package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/mdp/qrterminal/v3"
	"github.com/nyaruka/phonenumbers"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	"io"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ============= BRANDING CONFIGURATION =============
// These values determine how the connection appears in WhatsApp

const (
	// ClientName is the name shown in WhatsApp linked devices
	ClientName = "WA Sender Pro"

	// ClientShortName is a shorter version of the name
	ClientShortName = "WA Sender"

	// ClientVersion is the version shown
	ClientVersion = "1.0.0"

	// ClientPlatformType determines the icon shown in WhatsApp
	// Use store.Chrome for Chrome icon, store.Firefox for Firefox icon, etc.
	ClientPlatformType = "Chrome"

	// ClientOSName is the operating system name shown
	ClientOSName = "Windows"

	// ClientOSVersion is the OS version
	ClientOSVersion = "10"
)

// SessionClient represents an active WhatsApp client session
type SessionClient struct {
	SessionID string
	UserID    int
	Client    *whatsmeow.Client
	Device    *store.Device
	QRChannel chan string
	stopChan  chan struct{}
	mu        sync.Mutex
}

// WebSocketManager manages WebSocket connections for real-time updates
type WebSocketManager struct {
	connections sync.Map // sessionID -> []*websocket.Conn
	mu          sync.RWMutex
}

// WebSocketMessage represents a message sent through WebSocket
type WebSocketMessage struct {
	Type      string                 `json:"type"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
}

// NewWebSocketManager creates a new WebSocket manager
func NewWebSocketManager() *WebSocketManager {
	return &WebSocketManager{}
}

// AddConnection adds a WebSocket connection for a session
func (wsm *WebSocketManager) AddConnection(sessionID string, conn *websocket.Conn) {
	wsm.mu.Lock()
	defer wsm.mu.Unlock()

	connsInterface, _ := wsm.connections.LoadOrStore(sessionID, []*websocket.Conn{})
	conns := connsInterface.([]*websocket.Conn)
	conns = append(conns, conn)
	wsm.connections.Store(sessionID, conns)
}

// RemoveConnection removes a WebSocket connection
func (wsm *WebSocketManager) RemoveConnection(sessionID string, conn *websocket.Conn) {
	wsm.mu.Lock()
	defer wsm.mu.Unlock()

	connsInterface, exists := wsm.connections.Load(sessionID)
	if !exists {
		return
	}

	conns := connsInterface.([]*websocket.Conn)
	for i, c := range conns {
		if c == conn {
			conns = append(conns[:i], conns[i+1:]...)
			break
		}
	}

	if len(conns) > 0 {
		wsm.connections.Store(sessionID, conns)
	} else {
		wsm.connections.Delete(sessionID)
	}
}

// SendToSession sends a message to all connections for a session
func (wsm *WebSocketManager) SendToSession(sessionID string, message WebSocketMessage) {
	connsInterface, exists := wsm.connections.Load(sessionID)
	if !exists {
		return
	}

	message.Timestamp = time.Now()
	conns := connsInterface.([]*websocket.Conn)

	for _, conn := range conns {
		go func(c *websocket.Conn) {
			c.WriteJSON(message)
		}(conn)
	}
}

// WhatsAppService manages WhatsApp connections and sessions
type WhatsAppService struct {
	cfg         *Config
	db          *DatabaseManager
	sessions    sync.Map // sessionID -> *SessionClient
	wsManager   *WebSocketManager
	container   *sqlstore.Container
	containerMu sync.RWMutex
	monitorCtx  context.Context    // ADD THIS
	monitorStop context.CancelFunc // ADD THIS
}

// NewWhatsAppService creates a new WhatsApp service
func NewWhatsAppService(cfg *Config, db *DatabaseManager, wsm *WebSocketManager) *WhatsAppService {
	ws := &WhatsAppService{
		cfg:       cfg,
		db:        db,
		wsManager: wsm,
	}

	// Initialize WhatsApp SQL store container
	if err := ws.initializeContainer(); err != nil {
		log.Printf("Failed to initialize WhatsApp container: %v", err)
	}

	return ws
}

// initializeContainer initializes the WhatsApp SQL store container
func (ws *WhatsAppService) initializeContainer() error {
	// Get container from database manager (already using MySQL)
	container := ws.db.GetWhatsAppContainer()
	if container == nil {
		// Create a new MySQL container if needed
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s",
			ws.cfg.DBUser, ws.cfg.DBPassword, ws.cfg.DBHost, ws.cfg.DBPort, ws.cfg.DBName)

		dbLog := waLog.Stdout("WhatsApp", "INFO", true)
		var err error
		container, err = sqlstore.New(context.Background(), "mysql", dsn, dbLog)
		if err != nil {
			return fmt.Errorf("failed to create SQL store container: %w", err)
		}
	}

	ws.containerMu.Lock()
	ws.container = container
	ws.containerMu.Unlock()

	log.Printf("WhatsApp SQL store container initialized with PostgreSQL")
	return nil
}

// CreateSession creates a new WhatsApp session
func (ws *WhatsAppService) CreateSession(userID int, sessionName string) (*WhatsAppSession, error) {
	// Check device limit
	count, err := ws.db.GetActiveSessionCount(userID)
	if err != nil {
		return nil, err
	}

	if int(count) >= ws.cfg.MaxDevicesPerUser {
		return nil, fmt.Errorf("device limit reached: %d/%d", count, ws.cfg.MaxDevicesPerUser)
	}

	// Create session in database
	session, err := ws.db.CreateSession(userID, sessionName)
	if err != nil {
		return nil, err
	}

	// Initialize WhatsApp client
	if err := ws.InitializeClient(session); err != nil {
		sessionUUID, _ := uuid.Parse(session.ID)
		ws.db.UpdateSessionStatus(sessionUUID, StatusFailed)
		return nil, err
	}

	// Log event
	sessionUUID, _ := uuid.Parse(session.ID)
	ws.db.CreateEvent(sessionUUID, userID, "session_created", map[string]interface{}{
		"session_name": sessionName,
	})

	return session, nil
}

// InitializeClient initializes a WhatsApp client for a session
func (ws *WhatsAppService) InitializeClient(session *WhatsAppSession) error {
	// Create device store
	deviceStore := ws.createDeviceStore(session)

	if deviceStore == nil {
		return fmt.Errorf("failed to create device store for session %s", session.ID)
	}

	// Set up logger
	clientLog := waLog.Stdout("Client", "INFO", true)

	// Create WhatsApp client
	client := whatsmeow.NewClient(deviceStore, clientLog)
	client.EnableAutoReconnect = ws.cfg.AutoReconnect

	// ============= SET CLIENT PUSH NAME =============
	// This is the name that appears in WhatsApp at the top of the connection
	// and in the "Linked Devices" list
	client.Store.PushName = ClientName // "WA Sender Pro"

	// Create session client
	sessionClient := &SessionClient{
		SessionID: session.ID,
		UserID:    session.UserID,
		Client:    client,
		Device:    deviceStore,
		QRChannel: make(chan string, 1),
		stopChan:  make(chan struct{}),
	}

	// Register event handlers
	ws.registerEventHandlers(sessionClient)

	// Store session client
	ws.sessions.Store(session.ID, sessionClient)

	// Connect client
	go ws.connectClient(sessionClient)

	log.Printf("🚀 Initialized WhatsApp client '%s' for session %s", ClientName, session.ID)

	return nil
}

// connectClient connects a WhatsApp client
func (ws *WhatsAppService) connectClient(sc *SessionClient) {
	if err := sc.Client.Connect(); err != nil {
		log.Printf("Failed to connect client %s: %v", sc.SessionID, err)
		sessionUUID, _ := uuid.Parse(sc.SessionID)
		ws.db.UpdateSessionStatus(sessionUUID, StatusFailed)
		ws.db.CreateEvent(sessionUUID, sc.UserID, "connection_failed", map[string]interface{}{
			"error": err.Error(),
		})
	}
}

// createDeviceStore creates a device store for WhatsApp
// createDeviceStore creates a device store for WhatsApp
func (ws *WhatsAppService) createDeviceStore(session *WhatsAppSession) *store.Device {
	ws.containerMu.RLock()
	container := ws.container
	ws.containerMu.RUnlock()

	if container == nil {
		log.Printf("WhatsApp container not initialized for session %s", session.ID)
		if err := ws.initializeContainer(); err != nil {
			log.Printf("Failed to initialize container: %v", err)
			return nil
		}
		ws.containerMu.RLock()
		container = ws.container
		ws.containerMu.RUnlock()

		if container == nil {
			return nil
		}
	}

	// Check if device already exists for this session (by JID)
	if session.JID != nil && *session.JID != "" {
		jid, err := types.ParseJID(*session.JID)
		if err == nil {
			existingDevice, err := container.GetDevice(context.Background(), jid)
			if err == nil && existingDevice != nil {
				log.Printf("♻️  Reusing existing device for session %s (JID: %s)", session.ID, *session.JID)
				return existingDevice
			}
		}
	}

	// Create new device if none exists
	device := container.NewDevice()
	if device == nil {
		log.Printf("Failed to create new device for session %s", session.ID)
		return nil
	}

	// ============= CONFIGURE DEVICE BRANDING =============
	// This sets how the device appears in WhatsApp's linked devices list

	// Set platform type (this determines the icon shown in WhatsApp)
	// Available options: Chrome, Firefox, Safari, Edge, Opera, IE, Desktop, etc.
	if device.Platform == "" {
		device.Platform = ClientPlatformType
	}

	device.Platform = ClientPlatformType

	log.Printf("✅ Created WhatsApp device '%s' for session %s", ClientName, session.ID)
	return device
}

// GetSessionClient gets a session client from memory
func (ws *WhatsAppService) GetSessionClient(sessionID string) (*SessionClient, error) {
	clientInterface, ok := ws.sessions.Load(sessionID)
	if !ok {
		// Try to restore from database
		log.Printf("⚠️  Session %s not in memory, attempting to restore...", sessionID)

		sessionUUID, err := uuid.Parse(sessionID)
		if err != nil {
			return nil, fmt.Errorf("invalid session ID")
		}

		// Get session from database
		session, err := ws.db.GetSession(sessionUUID, 0) // userID doesn't matter for restore
		if err != nil {
			return nil, fmt.Errorf("session not found in database: %w", err)
		}

		// Only restore if session was previously connected
		if session.Status != StatusConnected && session.JID == nil {
			return nil, fmt.Errorf("session %s is not connected (status: %s)", sessionID, session.Status)
		}

		// Try to restore this single session
		if err := ws.restoreSingleSession(session); err != nil {
			return nil, fmt.Errorf("failed to restore session: %w", err)
		}

		// Try to load again
		clientInterface, ok = ws.sessions.Load(sessionID)
		if !ok {
			return nil, fmt.Errorf("failed to restore session %s", sessionID)
		}
	}

	return clientInterface.(*SessionClient), nil
}

// restoreSingleSession restores a single session
func (ws *WhatsAppService) restoreSingleSession(session *WhatsAppSession) error {
	if session.JID == nil || *session.JID == "" {
		return fmt.Errorf("session has no JID")
	}

	// Parse JID
	jid, err := types.ParseJID(*session.JID)
	if err != nil {
		return fmt.Errorf("invalid JID: %w", err)
	}

	// Get device from store
	device, err := ws.db.GetWhatsAppDevice(jid)
	if err != nil {
		return fmt.Errorf("device not found in store: %w", err)
	}

	// Create client
	clientLog := waLog.Stdout("Client", "INFO", true)
	client := whatsmeow.NewClient(device, clientLog)
	client.EnableAutoReconnect = ws.cfg.AutoReconnect

	// Set push name
	if client.Store.PushName == "" {
		client.Store.PushName = ClientName
	}

	// Create session client
	sessionClient := &SessionClient{
		SessionID: session.ID,
		UserID:    session.UserID,
		Client:    client,
		Device:    device,
		QRChannel: make(chan string, 1),
		stopChan:  make(chan struct{}),
	}

	// Register event handlers
	ws.registerEventHandlers(sessionClient)

	// Store in memory
	ws.sessions.Store(session.ID, sessionClient)

	// Connect
	go ws.connectClient(sessionClient)

	log.Printf("✅ Restored session %s", session.ID)
	return nil
}

// registerEventHandlers registers WhatsApp event handlers
func (ws *WhatsAppService) registerEventHandlers(sc *SessionClient) {
	sc.Client.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.QR:
			ws.handleQREvent(sc, v)
		case *events.Connected:
			ws.handleConnectedEvent(sc, v)
		case *events.Disconnected:
			ws.handleDisconnectedEvent(sc)
		case *events.LoggedOut:
			ws.handleLoggedOutEvent(sc)
		case *events.Message:
			ws.handleMessageEvent(sc, v)
		case *events.Receipt:
			ws.handleReceiptEvent(sc, v)
		case *events.PairSuccess:
			ws.handlePairSuccess(sc, v)
		case *events.HistorySync: // ← Add this
			ws.handleHistorySync(sc, v)
		}
	})
}

// handleHistorySync handles history sync to update push name
func (ws *WhatsAppService) handleHistorySync(sc *SessionClient, evt *events.HistorySync) {
	// Get push names from history sync
	pushnames := evt.Data.GetPushnames()
	if len(pushnames) == 0 {
		return
	}

	log.Printf("📇 Syncing %d contacts for session %s", len(pushnames), sc.SessionID)

	myJID := sc.Client.Store.ID
	contacts := make([]WhatsAppContact, 0, len(pushnames))

	for _, pn := range pushnames {
		jid := pn.GetID()
		pushName := pn.GetPushname()

		// Update our own push name
		if myJID != nil && pn.GetID() == myJID.User {
			if pushName != "" {
				sessionUUID, _ := uuid.Parse(sc.SessionID)
				err := ws.db.db.Model(&WhatsAppSession{}).
					Where("id = ?", sessionUUID.String()).
					Update("push_name", pushName).Error
				if err == nil {
					log.Printf("✅ Updated user's push name in DB: %s", pushName)
				}
			}
			continue // Don't add ourselves to contacts
		}

		// Parse and add contact
		contact := parseContact(jid, pushName, sc.UserID)
		contacts = append(contacts, *contact)
	}

	// Bulk insert contacts
	if len(contacts) > 0 {
		if err := ws.db.BulkUpsertContacts(contacts); err != nil {
			log.Printf("❌ Failed to save contacts: %v", err)
		} else {
			log.Printf("✅ Saved %d contacts for user %d", len(contacts), sc.UserID)
		}
	}
}

// handleQREvent handles QR code events
func (ws *WhatsAppService) handleQREvent(sc *SessionClient, evt *events.QR) {
	log.Printf("QR event for session %s", sc.SessionID)

	// Update status
	sessionUUID, _ := uuid.Parse(sc.SessionID)
	ws.db.UpdateSessionStatus(sessionUUID, StatusQRReady)

	// Generate QR code as base64 image
	qrPNG, err := qrcode.Encode(evt.Codes[0], qrcode.Medium, 256)
	if err != nil {
		log.Printf("Failed to generate QR code: %v", err)
		return
	}

	qrBase64 := fmt.Sprintf("data:image/png;base64,%s", base64.StdEncoding.EncodeToString(qrPNG))

	// Store QR code
	select {
	case sc.QRChannel <- qrBase64:
	default:
		<-sc.QRChannel
		sc.QRChannel <- qrBase64
	}

	// Update database with QR
	ws.db.UpdateSessionQR(sessionUUID, evt.Codes[0], qrBase64, ws.cfg.QRTimeout)

	// Send WebSocket update
	ws.wsManager.SendToSession(sc.SessionID, WebSocketMessage{
		Type: "qr_ready",
		Data: map[string]interface{}{
			"qr_code": qrBase64,
		},
	})

	// Print to terminal for debugging
	qrterminal.GenerateWithConfig(evt.Codes[0], qrterminal.Config{
		Level:     qrterminal.L,
		Writer:    log.Writer(),
		BlackChar: qrterminal.WHITE,
		WhiteChar: qrterminal.BLACK,
		QuietZone: 1,
	})
}

// handleConnectedEvent handles connected events
func (ws *WhatsAppService) handleConnectedEvent(sc *SessionClient, evt *events.Connected) {
	log.Printf("Connected event for session %s", sc.SessionID)

	sessionUUID, _ := uuid.Parse(sc.SessionID)

	// ============= ENSURE PUSH NAME IS SET =============
	if sc.Client.Store.PushName == "" {
		sc.Client.Store.PushName = ClientName
	}

	// Send presence to ensure WhatsApp registers our push name
	go func() {
		time.Sleep(2 * time.Second)
		ctx := context.Background()
		if err := sc.Client.SendPresence(ctx, types.PresenceAvailable); err != nil {
			log.Printf("⚠️  Failed to send presence for session %s: %v", sc.SessionID, err)
		} else {
			log.Printf("✅ Sent presence with push name '%s' for session %s",
				sc.Client.Store.PushName, sc.SessionID)
		}
	}()

	// Only update if we have the info
	if sc.Client.Store.ID != nil {
		jid := sc.Client.Store.ID.String()
		phoneNumber := sc.Client.Store.ID.User
		platform := sc.Client.Store.Platform

		if jid != "" && phoneNumber != "" {
			ws.db.db.Model(&WhatsAppSession{}).
				Where("id = ?", sessionUUID.String()).
				Updates(map[string]interface{}{
					"jid":          jid,
					"phone_number": phoneNumber,
					"platform":     platform,
					"status":       StatusConnected,
					"connected_at": time.Now(),
					"last_seen":    time.Now(),
				})

			log.Printf("📱 Connected - Device: '%s', JID: %s, Platform: %s", ClientName, jid, platform)
		}
	}

	// Send WebSocket update
	ws.wsManager.SendToSession(sc.SessionID, WebSocketMessage{
		Type: "connected",
		Data: map[string]interface{}{
			"session_id": sc.SessionID,
			"push_name":  sc.Client.Store.PushName,
		},
	})

	// Log event
	ws.db.CreateEvent(sessionUUID, sc.UserID, "connected", map[string]interface{}{
		"push_name": sc.Client.Store.PushName,
	})

	// ============= NEW: SYNC GROUPS AND DETECT BUSINESS ACCOUNT =============
	// Run in background to avoid blocking the connection event
	go func() {
		// Wait a bit for connection to stabilize
		time.Sleep(3 * time.Second)

		// Detect if this is a business account
		ws.detectBusinessAccount(sc)

		// Sync all groups
		ws.syncUserGroups(sc)
	}()
}

// handleDisconnectedEvent handles disconnected events
func (ws *WhatsAppService) handleDisconnectedEvent(sc *SessionClient) {
	log.Printf("Disconnected event for session %s", sc.SessionID)

	sessionUUID, _ := uuid.Parse(sc.SessionID)
	ws.db.SetSessionDisconnected(sessionUUID)

	ws.wsManager.SendToSession(sc.SessionID, WebSocketMessage{
		Type: "disconnected",
		Data: nil,
	})

	ws.db.CreateEvent(sessionUUID, sc.UserID, "disconnected", nil)
}

// handleLoggedOutEvent handles logged out events
func (ws *WhatsAppService) handleLoggedOutEvent(sc *SessionClient) {
	log.Printf("Logged out event for session %s", sc.SessionID)

	sessionUUID, _ := uuid.Parse(sc.SessionID)
	ws.db.UpdateSessionStatus(sessionUUID, StatusDisconnected)

	ws.sessions.Delete(sc.SessionID)
	close(sc.stopChan)

	ws.wsManager.SendToSession(sc.SessionID, WebSocketMessage{
		Type: "logged_out",
		Data: nil,
	})

	ws.db.CreateEvent(sessionUUID, sc.UserID, "logged_out", nil)
}

// handlePairSuccess handles successful pairing
func (ws *WhatsAppService) handlePairSuccess(sc *SessionClient, evt *events.PairSuccess) {
	log.Printf("✅ Pair success for session %s: JID=%s", sc.SessionID, evt.ID.String())

	jidStr := evt.ID.String()
	phoneNumber := evt.ID.User

	// ============= SET CUSTOM PUSH NAME =============
	// Override the push name with our custom name
	sc.Client.Store.PushName = ClientName
	userPushName := evt.BusinessName
	if userPushName == "" {
		userPushName = ClientName // Fallback to our brand name
	}

	// Save the updated push name to the database
	sessionUUID, _ := uuid.Parse(sc.SessionID)
	ws.db.SetSessionConnected(sessionUUID, jidStr, phoneNumber, userPushName, evt.Platform)

	log.Printf("📱 Set push name to '%s' for session %s", ClientName, sc.SessionID)

	ws.wsManager.SendToSession(sc.SessionID, WebSocketMessage{
		Type: "pair_success",
		Data: map[string]interface{}{
			"jid":           jidStr,
			"push_name":     userPushName,
			"business_name": evt.BusinessName,
			"platform":      evt.Platform,
		},
	})

	ws.db.CreateEvent(sessionUUID, sc.UserID, "pair_success", map[string]interface{}{
		"jid":       jidStr,
		"push_name": userPushName,
		"platform":  evt.Platform,
	})
}

// handleMessageEvent handles message events
func (ws *WhatsAppService) handleMessageEvent(sc *SessionClient, evt *events.Message) {
	content := ws.extractMessageContent(evt.Message)
	messageType := ws.getMessageType(evt.Message)

	ws.wsManager.SendToSession(sc.SessionID, WebSocketMessage{
		Type: "message",
		Data: map[string]interface{}{
			"message_id": evt.Info.ID,
			"from":       evt.Info.Sender.String(),
			"content":    content,
			"type":       messageType,
			"timestamp":  evt.Info.Timestamp,
		},
	})

	sessionUUID, _ := uuid.Parse(sc.SessionID)
	ws.db.CreateEvent(sessionUUID, sc.UserID, "message_received", map[string]interface{}{
		"message_id": evt.Info.ID,
		"from":       evt.Info.Sender.String(),
		"type":       messageType,
	})
}

// handleReceiptEvent handles receipt events
func (ws *WhatsAppService) handleReceiptEvent(sc *SessionClient, evt *events.Receipt) {
	ws.wsManager.SendToSession(sc.SessionID, WebSocketMessage{
		Type: "receipt",
		Data: map[string]interface{}{
			"message_id": evt.MessageIDs[0],
			"status":     string(evt.Type),
			"timestamp":  evt.Timestamp,
		},
	})
}

// SendMessage sends a WhatsApp message
func (ws *WhatsAppService) SendMessage(sessionID string, userID int, to string, content string) error {
	// Use the new helper that auto-restores if needed
	sc, err := ws.GetSessionClient(sessionID)
	if err != nil {
		return err
	}

	if !sc.Client.IsConnected() {
		return fmt.Errorf("client not connected")
	}

	var recipient types.JID

	// Try to parse as JID first (e.g., 201097154916@s.whatsapp.net)
	if strings.Contains(to, "@") {
		recipient, err = types.ParseJID(to)
		if err != nil {
			return fmt.Errorf("invalid JID format: %w", err)
		}
	} else {
		// Clean the phone number - remove + and any non-digit characters
		cleanNumber := ""
		for _, char := range to {
			if char >= '0' && char <= '9' {
				cleanNumber += string(char)
			}
		}

		// Validate that we have a number
		if cleanNumber == "" {
			return fmt.Errorf("invalid phone number format")
		}

		// Verify the number is on WhatsApp and get the proper JID
		// This is the KEY FIX - it ensures we get the correct JID format from WhatsApp
		resp, err := sc.Client.IsOnWhatsApp(context.Background(), []string{"+" + cleanNumber})
		if err != nil {
			return fmt.Errorf("failed to verify WhatsApp number: %w", err)
		}

		if len(resp) == 0 {
			return fmt.Errorf("unable to verify phone number")
		}

		if !resp[0].IsIn {
			return fmt.Errorf("phone number %s is not registered on WhatsApp", cleanNumber)
		}

		// Use the JID returned by WhatsApp - this handles both regular JIDs and LIDs
		recipient = resp[0].JID

		log.Printf("📱 Verified number %s -> JID: %s", cleanNumber, recipient.String())
	}

	message := &waE2E.Message{
		Conversation: proto.String(content),
	}

	resp, err := sc.Client.SendMessage(context.Background(), recipient, message)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	log.Printf("✅ Message sent successfully to %s (ID: %s)", recipient.String(), resp.ID)

	ws.wsManager.SendToSession(sessionID, WebSocketMessage{
		Type: "message_sent",
		Data: map[string]interface{}{
			"message_id": resp.ID,
			"to":         recipient.String(),
			"timestamp":  resp.Timestamp,
		},
	})

	return nil
}

// GetQRCode gets the QR code for a session
func (ws *WhatsAppService) GetQRCode(sessionID string, userID int) (string, error) {
	sessionUUID, err := uuid.Parse(sessionID)
	if err != nil {
		return "", fmt.Errorf("invalid session ID")
	}

	session, err := ws.db.GetSession(sessionUUID, userID)
	if err != nil {
		return "", err
	}

	if session.QRCodeBase64 != nil && *session.QRCodeBase64 != "" {
		if session.QRExpiresAt != nil && session.QRExpiresAt.Before(time.Now()) {
			return "", fmt.Errorf("QR code expired")
		}
		return *session.QRCodeBase64, nil
	}

	clientInterface, ok := ws.sessions.Load(sessionID)
	if !ok {
		return "", fmt.Errorf("session not initialized")
	}

	sc := clientInterface.(*SessionClient)
	select {
	case qr := <-sc.QRChannel:
		sc.QRChannel <- qr
		return qr, nil
	default:
		return "", fmt.Errorf("QR code not available")
	}
}

// DeleteSession deletes a WhatsApp session
func (ws *WhatsAppService) DeleteSession(sessionID string, userID int) error {
	if clientInterface, ok := ws.sessions.Load(sessionID); ok {
		sc := clientInterface.(*SessionClient)
		sc.Client.Disconnect()
		close(sc.stopChan)
		ws.sessions.Delete(sessionID)
	}

	sessionUUID, err := uuid.Parse(sessionID)
	if err != nil {
		return fmt.Errorf("invalid session ID")
	}
	return ws.db.DeleteSession(sessionUUID, userID)
}

// GetUserSessions gets all sessions for a user
func (ws *WhatsAppService) GetUserSessions(userID int) ([]WhatsAppSession, error) {
	return ws.db.GetUserSessions(userID)
}

// GetSessionStatus gets the status of a session
func (ws *WhatsAppService) GetSessionStatus(sessionID string, userID int) (*WhatsAppSession, error) {
	sessionUUID, err := uuid.Parse(sessionID)
	if err != nil {
		return nil, fmt.Errorf("invalid session ID")
	}

	session, err := ws.db.GetSession(sessionUUID, userID)
	if err != nil {
		return nil, err
	}

	if clientInterface, ok := ws.sessions.Load(sessionID); ok {
		sc := clientInterface.(*SessionClient)
		if sc.Client.IsConnected() {
			session.Status = StatusConnected
		} else {
			session.Status = StatusDisconnected
		}
		now := time.Now()
		session.LastSeen = &now
	}

	return session, nil
}

// RestoreActiveSessions restores active sessions on startup
// RestoreActiveSessions restores active sessions on startup
func (ws *WhatsAppService) RestoreActiveSessions() error {
	log.Println("🔄 Restoring active sessions from database...")

	// Get all devices from WhatsApp store
	devices, err := ws.db.GetAllDevices()
	if err != nil {
		log.Printf("Failed to get devices from store: %v", err)
		return err
	}

	if len(devices) == 0 {
		log.Println("   ℹ️  No devices found to restore")
		return nil
	}

	log.Printf("   Found %d device(s) in WhatsApp store", len(devices))

	restoredCount := 0
	for _, device := range devices {
		if device.ID == nil {
			log.Printf("   ⚠️  Skipping device with nil ID")
			continue
		}

		// Find matching session in database
		jidStr := device.ID.String()
		var session WhatsAppSession
		err := ws.db.db.Where("j_id = ? AND status IN ('connected', 'qr_ready', 'pending')", jidStr).
			First(&session).Error

		if err != nil {
			log.Printf("   ⚠️  No active session found for JID %s, skipping", jidStr)
			continue
		}

		// Check if session is already loaded in memory
		if _, exists := ws.sessions.Load(session.ID); exists {
			log.Printf("   ℹ️  Session %s already loaded, skipping", session.ID)
			continue
		}

		log.Printf("   🔄 Restoring session: %s (JID: %s)", session.SessionName, jidStr)

		// Create client with existing device
		clientLog := waLog.Stdout("Client", "INFO", true)
		client := whatsmeow.NewClient(device, clientLog)
		client.EnableAutoReconnect = ws.cfg.AutoReconnect

		// Set push name
		if client.Store.PushName == "" {
			client.Store.PushName = ClientName
		}

		// Create session client
		sessionClient := &SessionClient{
			SessionID: session.ID,
			UserID:    session.UserID,
			Client:    client,
			Device:    device,
			QRChannel: make(chan string, 1),
			stopChan:  make(chan struct{}),
		}

		// Register event handlers
		ws.registerEventHandlers(sessionClient)

		// Store session client in memory
		ws.sessions.Store(session.ID, sessionClient)

		// Connect client
		go ws.connectClient(sessionClient)

		restoredCount++
		log.Printf("   ✅ Restored session %s", session.SessionName)
	}

	if restoredCount > 0 {
		log.Printf("✅ Successfully restored %d session(s)", restoredCount)
	} else {
		log.Println("   ℹ️  No sessions needed restoration")
	}

	return nil
}

// extractMessageContent extracts content from a WhatsApp message
func (ws *WhatsAppService) extractMessageContent(msg *waE2E.Message) string {
	if msg.GetConversation() != "" {
		return msg.GetConversation()
	}
	if msg.GetExtendedTextMessage() != nil {
		return msg.GetExtendedTextMessage().GetText()
	}
	if msg.GetImageMessage() != nil {
		return "[Image]"
	}
	if msg.GetVideoMessage() != nil {
		return "[Video]"
	}
	if msg.GetAudioMessage() != nil {
		return "[Audio]"
	}
	if msg.GetDocumentMessage() != nil {
		return "[Document]"
	}
	return "[Unknown Message Type]"
}

// getMessageType gets the type of a WhatsApp message
func (ws *WhatsAppService) getMessageType(msg *waE2E.Message) string {
	if msg.GetConversation() != "" || msg.GetExtendedTextMessage() != nil {
		return "text"
	}
	if msg.GetImageMessage() != nil {
		return "image"
	}
	if msg.GetVideoMessage() != nil {
		return "video"
	}
	if msg.GetAudioMessage() != nil {
		return "audio"
	}
	if msg.GetDocumentMessage() != nil {
		return "document"
	}
	return "unknown"
}

// Cleanup cleans up resources
func (ws *WhatsAppService) Cleanup() {
	// Stop monitor if running
	ws.StopSessionMonitor()

	// Disconnect all sessions
	ws.sessions.Range(func(key, value interface{}) bool {
		sc := value.(*SessionClient)
		sc.Client.Disconnect()
		return true
	})

	// Close container
	ws.containerMu.Lock()
	if ws.container != nil {
		ws.container.Close()
		ws.container = nil
	}
	ws.containerMu.Unlock()
}

func parseContact(jid, pushName string, userID int) *WhatsAppContact {
	// Extract phone number from JID
	phoneNumber := ""
	if idx := strings.Index(jid, "@"); idx > 0 {
		phoneNumber = jid[:idx]
		if colonIdx := strings.Index(phoneNumber, ":"); colonIdx > 0 {
			phoneNumber = phoneNumber[:colonIdx]
		}
	}

	// Parse country code dynamically using phonenumbers library
	countryCode := ""
	mobileNumber := phoneNumber

	if phoneNumber != "" {
		// Parse the phone number (assume international format)
		num, err := phonenumbers.Parse("+"+phoneNumber, "")
		if err == nil {
			countryCode = fmt.Sprintf("%d", num.GetCountryCode())
			mobileNumber = fmt.Sprintf("%d", num.GetNationalNumber())
		}
	}

	// Parse name into first/last
	firstName := ""
	lastName := ""
	fullName := strings.TrimSpace(pushName)

	if fullName != "" {
		parts := strings.Fields(fullName)
		if len(parts) > 0 {
			firstName = parts[0]
			if len(parts) > 1 {
				lastName = strings.Join(parts[1:], " ")
			}
		}
	}

	return &WhatsAppContact{
		UserID:       userID,
		FullName:     fullName,
		FirstName:    firstName,
		LastName:     lastName,
		JID:          jid,
		CountryCode:  countryCode,
		MobileNumber: mobileNumber,
	}
}

// syncUserGroups syncs all user's WhatsApp groups to the database
func (ws *WhatsAppService) syncUserGroups(sc *SessionClient) {
	log.Printf("📱 Starting group sync for session %s", sc.SessionID)
	ctx := context.Background()
	groups, err := sc.Client.GetJoinedGroups(ctx)
	if err != nil {
		log.Printf("❌ Failed to fetch groups for session %s: %v", sc.SessionID, err)
		return
	}
	if len(groups) == 0 {
		log.Printf("ℹ️  No groups found for session %s", sc.SessionID)
		return
	}
	log.Printf("📊 Found %d groups for session %s (will use %v delay between requests)",
		len(groups), sc.SessionID, ws.cfg.GroupSyncDelay)

	successCount := 0
	errorCount := 0
	rateLimitCount := 0

	for i, groupInfo := range groups {
		if i > 0 {
			time.Sleep(ws.cfg.GroupSyncDelay)
		}
		err := ws.processGroupWithRetry(sc, groupInfo, ws.cfg.GroupSyncRetryAttempts)
		if err != nil {
			errorCount++
			if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "rate-overlimit") {
				rateLimitCount++
				log.Printf("⏸️  Rate limited on group %s, waiting 30 seconds...", groupInfo.JID.String())
				time.Sleep(30 * time.Second)
			} else {
				log.Printf("❌ Failed to process group %s: %v", groupInfo.JID.String(), err)
			}
		} else {
			successCount++
		}
		if (i+1)%10 == 0 {
			log.Printf("📊 Progress: %d/%d groups processed", i+1, len(groups))
		}
	}
	log.Printf("✅ Group sync completed for session %s: %d successful, %d failed (%d rate-limited)",
		sc.SessionID, successCount, errorCount, rateLimitCount)

	sessionUUID, _ := uuid.Parse(sc.SessionID)
	ws.db.CreateEvent(sessionUUID, sc.UserID, "groups_synced", map[string]interface{}{
		"total_groups": len(groups),
		"successful":   successCount,
		"failed":       errorCount,
		"rate_limited": rateLimitCount,
	})
}

// processGroup processes a single group and its participants
func (ws *WhatsAppService) processGroup(sc *SessionClient, groupInfo *types.GroupInfo) error {
	ctx := context.Background()
	fullGroupInfo, err := sc.Client.GetGroupInfo(ctx, groupInfo.JID)
	if err != nil {
		if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "rate-overlimit") {
			return fmt.Errorf("rate limited: %w", err)
		}
		return fmt.Errorf("failed to get full group info: %w", err)
	}
	group := &WhatsAppGroup{
		UserID:           sc.UserID,
		SessionID:        sc.SessionID,
		GroupJID:         groupInfo.JID.String(),
		GroupName:        fullGroupInfo.Name,
		GroupSubject:     &fullGroupInfo.Topic,
		ParticipantCount: len(fullGroupInfo.Participants),
		IsAnnouncement:   fullGroupInfo.IsAnnounce,
		IsLocked:         fullGroupInfo.IsLocked,
	}
	if err := ws.db.UpsertGroup(group); err != nil {
		return fmt.Errorf("failed to save group: %w", err)
	}
	savedGroup, err := ws.db.GetGroupByJID(sc.UserID, groupInfo.JID.String())
	if err != nil {
		return fmt.Errorf("failed to retrieve saved group: %w", err)
	}
	if len(fullGroupInfo.Participants) > 0 {
		participants := make([]WhatsAppContact, 0, len(fullGroupInfo.Participants))
		for _, participant := range fullGroupInfo.Participants {
			jidStr := participant.JID.String()
			pushName := participant.DisplayName
			if pushName == "" {
				pushName = participant.JID.User
			}
			contact := parseContact(jidStr, pushName, sc.UserID)
			contact.GroupID = &savedGroup.ID
			contact.IsGroupMember = true
			participants = append(participants, *contact)
		}
		if err := ws.db.BulkUpsertContacts(participants); err != nil {
			log.Printf("⚠️  Failed to save participants for group %s: %v", fullGroupInfo.Name, err)
		} else {
			log.Printf("👥 Saved %d participants for group %s", len(participants), fullGroupInfo.Name)
		}
	}
	log.Printf("✅ Processed group: %s (%d participants)", fullGroupInfo.Name, len(fullGroupInfo.Participants))
	return nil
}

// detectBusinessAccount checks if the connected account is a business account
func (ws *WhatsAppService) detectBusinessAccount(sc *SessionClient) {
	sessionUUID, _ := uuid.Parse(sc.SessionID)

	// Check if business name is set in the store
	isBusiness := sc.Client.Store.BusinessName != ""

	// Update database
	if err := ws.db.UpdateSessionBusinessAccount(sessionUUID, isBusiness); err != nil {
		log.Printf("❌ Failed to update business account status for session %s: %v",
			sc.SessionID, err)
		return
	}

	if isBusiness {
		log.Printf("🏢 Business account detected for session %s: %s",
			sc.SessionID, sc.Client.Store.BusinessName)

		// Log event
		ws.db.CreateEvent(sessionUUID, sc.UserID, "business_account_detected", map[string]interface{}{
			"business_name": sc.Client.Store.BusinessName,
		})
	} else {
		log.Printf("👤 Personal account detected for session %s", sc.SessionID)
	}
}

// ============= MEDIA UPLOAD HELPER =============

// uploadMedia uploads media to WhatsApp servers
func (ws *WhatsAppService) uploadMedia(sc *SessionClient, mediaData []byte, mediaType whatsmeow.MediaType) (*whatsmeow.UploadResponse, error) {
	ctx := context.Background()

	log.Printf("📤 Uploading media of type %s (%d bytes)", mediaType, len(mediaData))

	resp, err := sc.Client.Upload(ctx, mediaData, mediaType)
	if err != nil {
		return nil, fmt.Errorf("failed to upload media: %w", err)
	}

	log.Printf("✅ Media uploaded successfully - URL: %s", resp.URL)
	return &resp, nil
}

// ============= IMAGE MESSAGE =============

// SendImageMessage sends an image message with optional caption
func (ws *WhatsAppService) SendImageMessage(sessionID string, userID int, to string, imageData []byte, caption string) error {
	sc, err := ws.GetSessionClient(sessionID)
	if err != nil {
		return err
	}

	if !sc.Client.IsConnected() {
		return fmt.Errorf("client not connected")
	}

	// Validate recipient
	recipient, err := ws.validateAndGetRecipient(sc, to)
	if err != nil {
		return err
	}

	// Upload image
	uploaded, err := ws.uploadMedia(sc, imageData, whatsmeow.MediaImage)
	if err != nil {
		return err
	}

	// Detect MIME type
	mimeType := http.DetectContentType(imageData)

	// Create image message
	imageMsg := &waE2E.ImageMessage{
		Caption:       proto.String(caption),
		Mimetype:      proto.String(mimeType),
		URL:           &uploaded.URL,
		DirectPath:    &uploaded.DirectPath,
		MediaKey:      uploaded.MediaKey,
		FileEncSHA256: uploaded.FileEncSHA256,
		FileSHA256:    uploaded.FileSHA256,
		FileLength:    &uploaded.FileLength,
	}

	message := &waE2E.Message{
		ImageMessage: imageMsg,
	}

	// Send message
	ctx := context.Background()
	resp, err := sc.Client.SendMessage(ctx, recipient, message)
	if err != nil {
		return fmt.Errorf("failed to send image message: %w", err)
	}

	log.Printf("✅ Image message sent to %s (ID: %s)", recipient.String(), resp.ID)

	// Send WebSocket notification
	ws.wsManager.SendToSession(sessionID, WebSocketMessage{
		Type: "message_sent",
		Data: map[string]interface{}{
			"message_id": resp.ID,
			"to":         recipient.String(),
			"type":       "image",
			"timestamp":  resp.Timestamp,
		},
	})

	return nil
}

// ============= VIDEO MESSAGE =============

// SendVideoMessage sends a video message with optional caption
func (ws *WhatsAppService) SendVideoMessage(sessionID string, userID int, to string, videoData []byte, caption string) error {
	sc, err := ws.GetSessionClient(sessionID)
	if err != nil {
		return err
	}

	if !sc.Client.IsConnected() {
		return fmt.Errorf("client not connected")
	}

	// Validate recipient
	recipient, err := ws.validateAndGetRecipient(sc, to)
	if err != nil {
		return err
	}

	// Upload video
	uploaded, err := ws.uploadMedia(sc, videoData, whatsmeow.MediaVideo)
	if err != nil {
		return err
	}

	// Detect MIME type
	mimeType := http.DetectContentType(videoData)
	if mimeType == "application/octet-stream" {
		mimeType = "video/mp4" // Default to mp4
	}

	// Create video message
	videoMsg := &waE2E.VideoMessage{
		Caption:       proto.String(caption),
		Mimetype:      proto.String(mimeType),
		URL:           &uploaded.URL,
		DirectPath:    &uploaded.DirectPath,
		MediaKey:      uploaded.MediaKey,
		FileEncSHA256: uploaded.FileEncSHA256,
		FileSHA256:    uploaded.FileSHA256,
		FileLength:    &uploaded.FileLength,
	}

	message := &waE2E.Message{
		VideoMessage: videoMsg,
	}

	// Send message
	ctx := context.Background()
	resp, err := sc.Client.SendMessage(ctx, recipient, message)
	if err != nil {
		return fmt.Errorf("failed to send video message: %w", err)
	}

	log.Printf("✅ Video message sent to %s (ID: %s)", recipient.String(), resp.ID)

	ws.wsManager.SendToSession(sessionID, WebSocketMessage{
		Type: "message_sent",
		Data: map[string]interface{}{
			"message_id": resp.ID,
			"to":         recipient.String(),
			"type":       "video",
			"timestamp":  resp.Timestamp,
		},
	})

	return nil
}

// ============= AUDIO MESSAGE =============

// SendAudioMessage sends an audio message (voice note or audio file)
func (ws *WhatsAppService) SendAudioMessage(sessionID string, userID int, to string, audioData []byte, isVoice bool) error {
	sc, err := ws.GetSessionClient(sessionID)
	if err != nil {
		return err
	}

	if !sc.Client.IsConnected() {
		return fmt.Errorf("client not connected")
	}

	// Validate recipient
	recipient, err := ws.validateAndGetRecipient(sc, to)
	if err != nil {
		return err
	}

	// Upload audio
	uploaded, err := ws.uploadMedia(sc, audioData, whatsmeow.MediaAudio)
	if err != nil {
		return err
	}

	// Detect MIME type
	mimeType := http.DetectContentType(audioData)
	if mimeType == "application/octet-stream" {
		mimeType = "audio/ogg; codecs=opus" // Default for voice notes
	}

	// Create audio message
	audioMsg := &waE2E.AudioMessage{
		Mimetype:      proto.String(mimeType),
		URL:           &uploaded.URL,
		DirectPath:    &uploaded.DirectPath,
		MediaKey:      uploaded.MediaKey,
		FileEncSHA256: uploaded.FileEncSHA256,
		FileSHA256:    uploaded.FileSHA256,
		FileLength:    &uploaded.FileLength,
		PTT:           proto.Bool(isVoice), // PTT = Push To Talk (voice note)
	}

	message := &waE2E.Message{
		AudioMessage: audioMsg,
	}

	// Send message
	ctx := context.Background()
	resp, err := sc.Client.SendMessage(ctx, recipient, message)
	if err != nil {
		return fmt.Errorf("failed to send audio message: %w", err)
	}

	audioType := "audio"
	if isVoice {
		audioType = "voice"
	}

	log.Printf("✅ %s message sent to %s (ID: %s)", audioType, recipient.String(), resp.ID)

	ws.wsManager.SendToSession(sessionID, WebSocketMessage{
		Type: "message_sent",
		Data: map[string]interface{}{
			"message_id": resp.ID,
			"to":         recipient.String(),
			"type":       audioType,
			"timestamp":  resp.Timestamp,
		},
	})

	return nil
}

// ============= DOCUMENT MESSAGE =============

// SendDocumentMessage sends a document with filename and MIME type
func (ws *WhatsAppService) SendDocumentMessage(sessionID string, userID int, to string, docData []byte, filename, mimetype string) error {
	sc, err := ws.GetSessionClient(sessionID)
	if err != nil {
		return err
	}

	if !sc.Client.IsConnected() {
		return fmt.Errorf("client not connected")
	}

	// Validate recipient
	recipient, err := ws.validateAndGetRecipient(sc, to)
	if err != nil {
		return err
	}

	// Upload document
	uploaded, err := ws.uploadMedia(sc, docData, whatsmeow.MediaDocument)
	if err != nil {
		return err
	}

	// Auto-detect MIME type if not provided
	if mimetype == "" {
		mimetype = http.DetectContentType(docData)
		if mimetype == "application/octet-stream" {
			// Try to guess from filename extension
			ext := filepath.Ext(filename)
			mimetype = mime.TypeByExtension(ext)
			if mimetype == "" {
				mimetype = "application/octet-stream"
			}
		}
	}

	// Set default filename if not provided
	if filename == "" {
		filename = "document"
	}

	// Create document message
	docMsg := &waE2E.DocumentMessage{
		FileName:      proto.String(filename),
		Mimetype:      proto.String(mimetype),
		URL:           &uploaded.URL,
		DirectPath:    &uploaded.DirectPath,
		MediaKey:      uploaded.MediaKey,
		FileEncSHA256: uploaded.FileEncSHA256,
		FileSHA256:    uploaded.FileSHA256,
		FileLength:    &uploaded.FileLength,
	}

	message := &waE2E.Message{
		DocumentMessage: docMsg,
	}

	// Send message
	ctx := context.Background()
	resp, err := sc.Client.SendMessage(ctx, recipient, message)
	if err != nil {
		return fmt.Errorf("failed to send document message: %w", err)
	}

	log.Printf("✅ Document message sent to %s (ID: %s, file: %s)", recipient.String(), resp.ID, filename)

	ws.wsManager.SendToSession(sessionID, WebSocketMessage{
		Type: "message_sent",
		Data: map[string]interface{}{
			"message_id": resp.ID,
			"to":         recipient.String(),
			"type":       "document",
			"filename":   filename,
			"timestamp":  resp.Timestamp,
		},
	})

	return nil
}

// ============= HELPER FUNCTIONS =============

// validateAndGetRecipient validates and returns the recipient JID
func (ws *WhatsAppService) validateAndGetRecipient(sc *SessionClient, to string) (types.JID, error) {
	var recipient types.JID
	var err error

	// Try to parse as JID first (e.g., 201097154916@s.whatsapp.net)
	if strings.Contains(to, "@") {
		recipient, err = types.ParseJID(to)
		if err != nil {
			return types.JID{}, fmt.Errorf("invalid JID format: %w", err)
		}
	} else {
		// Clean the phone number - remove + and any non-digit characters
		cleanNumber := ""
		for _, char := range to {
			if char >= '0' && char <= '9' {
				cleanNumber += string(char)
			}
		}

		if cleanNumber == "" {
			return types.JID{}, fmt.Errorf("invalid phone number format")
		}

		// Verify the number is on WhatsApp
		resp, err := sc.Client.IsOnWhatsApp(context.Background(), []string{"+" + cleanNumber})
		if err != nil {
			return types.JID{}, fmt.Errorf("failed to verify WhatsApp number: %w", err)
		}

		if len(resp) == 0 || !resp[0].IsIn {
			return types.JID{}, fmt.Errorf("phone number %s is not registered on WhatsApp", cleanNumber)
		}

		recipient = resp[0].JID
		log.Printf("📱 Verified number %s -> JID: %s", cleanNumber, recipient.String())
	}

	return recipient, nil
}

// downloadMediaFromURL downloads media from a URL
func (ws *WhatsAppService) downloadMediaFromURL(url string, maxSize int64) ([]byte, error) {
	log.Printf("📥 Downloading media from URL: %s", url)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download media: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download media: HTTP %d", resp.StatusCode)
	}

	// Check content length
	if resp.ContentLength > maxSize {
		return nil, fmt.Errorf("file too large: %d bytes (max %d bytes)", resp.ContentLength, maxSize)
	}

	// Read with size limit
	limitedReader := io.LimitReader(resp.Body, maxSize)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read media data: %w", err)
	}

	log.Printf("✅ Downloaded %d bytes from URL", len(data))
	return data, nil
}

func (ws *WhatsAppService) processGroupWithRetry(sc *SessionClient, groupInfo *types.GroupInfo, maxRetries int) error {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			waitTime := time.Duration(5*(1<<uint(attempt-1))) * time.Second
			log.Printf("🔄 Retry attempt %d/%d for group %s after %v",
				attempt+1, maxRetries, groupInfo.JID.String(), waitTime)
			time.Sleep(waitTime)
		}
		err := ws.processGroup(sc, groupInfo)
		if err == nil {
			return nil
		}
		lastErr = err
		if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "rate-overlimit") {
			continue
		}
		return err
	}
	return fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

func (ws *WhatsAppService) StartSessionMonitor(ctx context.Context) {
	ws.monitorCtx, ws.monitorStop = context.WithCancel(ctx)
	go ws.sessionMonitorLoop()
	log.Println("✅ Session health monitor started")
}

func (ws *WhatsAppService) StopSessionMonitor() {
	if ws.monitorStop != nil {
		ws.monitorStop()
		log.Println("🛑 Session health monitor stopped")
	}
}
func (ws *WhatsAppService) sessionMonitorLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Initial check after 10 seconds
	initialTimer := time.NewTimer(10 * time.Second)
	defer initialTimer.Stop()

	for {
		select {
		case <-ws.monitorCtx.Done():
			return
		case <-initialTimer.C:
			ws.checkAllSessionHealth()
		case <-ticker.C:
			ws.checkAllSessionHealth()
		}
	}
}

func (ws *WhatsAppService) checkAllSessionHealth() {
	log.Println("🔍 Checking health of all active sessions...")

	checkedCount := 0
	reconnectedCount := 0
	failedCount := 0

	// Get all connected sessions from database
	var sessions []WhatsAppSession
	err := ws.db.db.Where("status = ? AND deleted_at IS NULL", StatusConnected).
		Find(&sessions).Error
	if err != nil {
		log.Printf("❌ Failed to fetch sessions for health check: %v", err)
		return
	}

	for _, session := range sessions {
		checkedCount++

		// Update last_seen timestamp
		sessionUUID, _ := uuid.Parse(session.ID)
		now := time.Now()
		ws.db.db.Model(&WhatsAppSession{}).
			Where("id = ?", session.ID).
			Update("last_seen", now)

		// Check if session exists in memory
		clientInterface, exists := ws.sessions.Load(session.ID)
		if !exists {
			// Session not in memory but should be connected, try to restore
			log.Printf("⚠️ Session %s not in memory, attempting restoration...", session.SessionName)
			if err := ws.restoreSingleSession(&session); err != nil {
				log.Printf("❌ Failed to restore session %s: %v", session.SessionName, err)
				failedCount++
				ws.db.UpdateSessionStatus(sessionUUID, StatusDisconnected)
			} else {
				log.Printf("✅ Successfully restored session %s", session.SessionName)
				reconnectedCount++
			}
			continue
		}

		// Check if client is actually connected
		sc := clientInterface.(*SessionClient)
		if !sc.Client.IsConnected() {
			log.Printf("⚠️ Session %s is disconnected, attempting reconnection...", session.SessionName)

			// Try to reconnect
			if err := ws.reconnectSession(sc); err != nil {
				log.Printf("❌ Failed to reconnect session %s: %v", session.SessionName, err)
				failedCount++
				ws.db.UpdateSessionStatus(sessionUUID, StatusDisconnected)

				// Send WebSocket notification
				ws.wsManager.SendToSession(session.ID, WebSocketMessage{
					Type: "session_health",
					Data: map[string]interface{}{
						"status":    "disconnected",
						"error":     err.Error(),
						"timestamp": time.Now(),
					},
				})
			} else {
				log.Printf("✅ Successfully reconnected session %s", session.SessionName)
				reconnectedCount++

				// Send WebSocket notification
				ws.wsManager.SendToSession(session.ID, WebSocketMessage{
					Type: "session_health",
					Data: map[string]interface{}{
						"status":    "reconnected",
						"timestamp": time.Now(),
					},
				})
			}
		}
	}

	if checkedCount > 0 {
		log.Printf("🔍 Health check complete: %d checked, %d reconnected, %d failed",
			checkedCount, reconnectedCount, failedCount)
	}
}

// reconnectSession attempts to reconnect a disconnected session
func (ws *WhatsAppService) reconnectSession(sc *SessionClient) error {
	// Disconnect first if needed
	if sc.Client.IsConnected() {
		sc.Client.Disconnect()
		time.Sleep(1 * time.Second)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Attempt to connect
	if err := sc.Client.Connect(); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	// Wait for connection to be established
	connChan := make(chan bool, 1)
	go func() {
		for i := 0; i < 30; i++ {
			if sc.Client.IsConnected() {
				connChan <- true
				return
			}
			time.Sleep(1 * time.Second)
		}
		connChan <- false
	}()

	select {
	case connected := <-connChan:
		if !connected {
			return fmt.Errorf("connection timeout")
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("connection timeout")
	}
}

// RefreshSession manually refreshes a session by disconnecting and reconnecting
func (ws *WhatsAppService) RefreshSession(sessionID string, userID int) error {
	// Validate session ID
	sessionUUID, err := uuid.Parse(sessionID)
	if err != nil {
		return fmt.Errorf("invalid session ID")
	}

	// Check if user owns the session
	session, err := ws.db.GetSession(sessionUUID, userID)
	if err != nil {
		return fmt.Errorf("session not found or unauthorized")
	}

	log.Printf("🔄 Manual refresh requested for session %s", session.SessionName)

	// Check if session exists in memory
	clientInterface, exists := ws.sessions.Load(sessionID)
	if !exists {
		// Session not in memory, try to restore it
		log.Printf("📱 Session %s not in memory, attempting restoration...", session.SessionName)

		// Only restore if it has a JID (was previously connected)
		if session.JID == nil || *session.JID == "" {
			return fmt.Errorf("session was never connected")
		}

		if err := ws.restoreSingleSession(session); err != nil {
			// Update status to disconnected
			ws.db.UpdateSessionStatus(sessionUUID, StatusDisconnected)
			return fmt.Errorf("failed to restore session: %w", err)
		}

		// Try to load again after restoration
		clientInterface, exists = ws.sessions.Load(sessionID)
		if !exists {
			return fmt.Errorf("failed to load session after restoration")
		}
	}

	// Get the session client
	sc := clientInterface.(*SessionClient)

	// Force disconnect
	log.Printf("🔌 Disconnecting session %s...", session.SessionName)
	sc.Client.Disconnect()
	time.Sleep(2 * time.Second)

	// Attempt reconnection
	log.Printf("🔌 Reconnecting session %s...", session.SessionName)
	if err := ws.reconnectSession(sc); err != nil {
		// Update status to disconnected
		ws.db.UpdateSessionStatus(sessionUUID, StatusDisconnected)

		// Log event
		ws.db.CreateEvent(sessionUUID, userID, "refresh_failed", map[string]interface{}{
			"error": err.Error(),
		})

		return fmt.Errorf("failed to reconnect: %w", err)
	}

	// Update status to connected
	ws.db.UpdateSessionStatus(sessionUUID, StatusConnected)

	// Log event
	ws.db.CreateEvent(sessionUUID, userID, "refresh_success", nil)

	// Send WebSocket notification
	ws.wsManager.SendToSession(sessionID, WebSocketMessage{
		Type: "session_refreshed",
		Data: map[string]interface{}{
			"status":    "connected",
			"timestamp": time.Now(),
		},
	})

	log.Printf("✅ Successfully refreshed session %s", session.SessionName)
	return nil
}
