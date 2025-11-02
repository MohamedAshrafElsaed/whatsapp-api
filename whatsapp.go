package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/mdp/qrterminal/v3"
	qrcode "github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// SessionClient represents an active WhatsApp client session
type SessionClient struct {
	SessionID uuid.UUID
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
func (wsm *WebSocketManager) AddConnection(sessionID uuid.UUID, conn *websocket.Conn) {
	wsm.mu.Lock()
	defer wsm.mu.Unlock()

	connsInterface, _ := wsm.connections.LoadOrStore(sessionID, []*websocket.Conn{})
	conns := connsInterface.([]*websocket.Conn)
	conns = append(conns, conn)
	wsm.connections.Store(sessionID, conns)
}

// RemoveConnection removes a WebSocket connection
func (wsm *WebSocketManager) RemoveConnection(sessionID uuid.UUID, conn *websocket.Conn) {
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
func (wsm *WebSocketManager) SendToSession(sessionID uuid.UUID, message WebSocketMessage) {
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
	// Get container from database manager (already using PostgreSQL)
	container := ws.db.GetWhatsAppContainer()
	if container == nil {
		// Create a new PostgreSQL container if needed
		dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
			ws.cfg.DBHost, ws.cfg.DBPort, ws.cfg.DBUser, ws.cfg.DBPassword, ws.cfg.DBName, ws.cfg.DBSSLMode)

		dbLog := waLog.Stdout("WhatsApp", "INFO", true)
		var err error
		container, err = sqlstore.New(context.Background(), "postgres", dsn, dbLog)
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
		ws.db.UpdateSessionStatus(session.ID, StatusFailed)
		return nil, err
	}

	// Log event
	ws.db.CreateEvent(session.ID, userID, "session_created", map[string]interface{}{
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

	return nil
}

// connectClient connects a WhatsApp client
func (ws *WhatsAppService) connectClient(sc *SessionClient) {
	if err := sc.Client.Connect(); err != nil {
		log.Printf("Failed to connect client %s: %v", sc.SessionID, err)
		ws.db.UpdateSessionStatus(sc.SessionID, StatusFailed)
		ws.db.CreateEvent(sc.SessionID, sc.UserID, "connection_failed", map[string]interface{}{
			"error": err.Error(),
		})
	}
}

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

	device := container.NewDevice()
	if device == nil {
		log.Printf("Failed to create new device for session %s", session.ID)
		return nil
	}

	log.Printf("Created new WhatsApp device for session %s", session.ID)
	return device
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
		}
	})
}

// handleQREvent handles QR code events
func (ws *WhatsAppService) handleQREvent(sc *SessionClient, evt *events.QR) {
	log.Printf("QR event for session %s", sc.SessionID)

	// Update status
	ws.db.UpdateSessionStatus(sc.SessionID, StatusQRReady)

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
	ws.db.UpdateSessionQR(sc.SessionID, evt.Codes[0], qrBase64, ws.cfg.QRTimeout)

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

	jid := sc.Client.Store.ID.String()
	pushName := sc.Client.Store.PushName
	phoneNumber := sc.Client.Store.ID.User

	// Update session
	ws.db.SetSessionConnected(sc.SessionID, jid, phoneNumber, pushName, "WhatsApp Web")

	// Send WebSocket update
	ws.wsManager.SendToSession(sc.SessionID, WebSocketMessage{
		Type: "connected",
		Data: map[string]interface{}{
			"jid":       jid,
			"push_name": pushName,
		},
	})

	// Log event
	ws.db.CreateEvent(sc.SessionID, sc.UserID, "connected", map[string]interface{}{
		"jid":       jid,
		"push_name": pushName,
	})
}

// handleDisconnectedEvent handles disconnected events
func (ws *WhatsAppService) handleDisconnectedEvent(sc *SessionClient) {
	log.Printf("Disconnected event for session %s", sc.SessionID)

	ws.db.SetSessionDisconnected(sc.SessionID)

	ws.wsManager.SendToSession(sc.SessionID, WebSocketMessage{
		Type: "disconnected",
		Data: nil,
	})

	ws.db.CreateEvent(sc.SessionID, sc.UserID, "disconnected", nil)
}

// handleLoggedOutEvent handles logged out events
func (ws *WhatsAppService) handleLoggedOutEvent(sc *SessionClient) {
	log.Printf("Logged out event for session %s", sc.SessionID)

	ws.db.UpdateSessionStatus(sc.SessionID, StatusDisconnected)

	ws.sessions.Delete(sc.SessionID)
	close(sc.stopChan)

	ws.wsManager.SendToSession(sc.SessionID, WebSocketMessage{
		Type: "logged_out",
		Data: nil,
	})

	ws.db.CreateEvent(sc.SessionID, sc.UserID, "logged_out", nil)
}

// handlePairSuccess handles successful pairing
func (ws *WhatsAppService) handlePairSuccess(sc *SessionClient, evt *events.PairSuccess) {
	log.Printf("Pair success for session %s: JID=%s", sc.SessionID, evt.ID.String())

	jidStr := evt.ID.String()
	phoneNumber := evt.ID.User

	ws.db.SetSessionConnected(sc.SessionID, jidStr, phoneNumber, evt.BusinessName, evt.Platform)

	ws.wsManager.SendToSession(sc.SessionID, WebSocketMessage{
		Type: "pair_success",
		Data: map[string]interface{}{
			"jid":           jidStr,
			"business_name": evt.BusinessName,
			"platform":      evt.Platform,
		},
	})

	ws.db.CreateEvent(sc.SessionID, sc.UserID, "pair_success", map[string]interface{}{
		"jid":      jidStr,
		"platform": evt.Platform,
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

	ws.db.CreateEvent(sc.SessionID, sc.UserID, "message_received", map[string]interface{}{
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
func (ws *WhatsAppService) SendMessage(sessionID uuid.UUID, userID int, to string, content string) error {
	clientInterface, ok := ws.sessions.Load(sessionID)
	if !ok {
		return fmt.Errorf("session not found or not connected")
	}

	sc := clientInterface.(*SessionClient)

	if !sc.Client.IsConnected() {
		return fmt.Errorf("client not connected")
	}

	recipient, err := types.ParseJID(to)
	if err != nil {
		if to[0] != '+' {
			to = "+" + to
		}
		cleanNumber := "+"
		for _, char := range to[1:] {
			if char >= '0' && char <= '9' {
				cleanNumber += string(char)
			}
		}
		recipient = types.NewJID(cleanNumber[1:], types.DefaultUserServer)
	}

	message := &waE2E.Message{
		Conversation: proto.String(content),
	}

	resp, err := sc.Client.SendMessage(context.Background(), recipient, message)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

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
func (ws *WhatsAppService) GetQRCode(sessionID uuid.UUID, userID int) (string, error) {
	session, err := ws.db.GetSession(sessionID, userID)
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
func (ws *WhatsAppService) DeleteSession(sessionID uuid.UUID, userID int) error {
	if clientInterface, ok := ws.sessions.Load(sessionID); ok {
		sc := clientInterface.(*SessionClient)
		sc.Client.Disconnect()
		close(sc.stopChan)
		ws.sessions.Delete(sessionID)
	}

	return ws.db.DeleteSession(sessionID, userID)
}

// GetUserSessions gets all sessions for a user
func (ws *WhatsAppService) GetUserSessions(userID int) ([]WhatsAppSession, error) {
	return ws.db.GetUserSessions(userID)
}

// GetSessionStatus gets the status of a session
func (ws *WhatsAppService) GetSessionStatus(sessionID uuid.UUID, userID int) (*WhatsAppSession, error) {
	session, err := ws.db.GetSession(sessionID, userID)
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
func (ws *WhatsAppService) RestoreActiveSessions() error {
	// Implementation would restore sessions from database
	// For now, just log
	log.Println("Restoring active sessions...")
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
	ws.sessions.Range(func(key, value interface{}) bool {
		sc := value.(*SessionClient)
		sc.Client.Disconnect()
		return true
	})

	ws.containerMu.Lock()
	if ws.container != nil {
		ws.container.Close()
		ws.container = nil
	}
	ws.containerMu.Unlock()
}
