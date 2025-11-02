// internal/services/session_manager.go
package services

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"whatsapp-api/internal/config"
	"whatsapp-api/internal/models"
	"whatsapp-api/internal/repositories"
	"whatsapp-api/internal/store"
	"whatsapp-api/internal/websocket"
	"whatsapp-api/pkg/logger"
)

// SessionManager manages WhatsApp sessions
type SessionManager struct {
	config      *config.Config
	sessionRepo *repositories.SessionRepository
	deviceRepo  *repositories.DeviceRepository
	eventRepo   *repositories.EventRepository
	wsManager   *websocket.Manager
	clients     map[uuid.UUID]*whatsmeow.Client
	devices     map[uuid.UUID]*store.Device
	mu          sync.RWMutex
	logger      *logger.Logger
	stopChan    chan struct{}
}

// NewSessionManager creates a new session manager
func NewSessionManager(
	cfg *config.Config,
	sessionRepo *repositories.SessionRepository,
	deviceRepo *repositories.DeviceRepository,
	eventRepo *repositories.EventRepository,
	wsManager *websocket.Manager,
	log *logger.Logger,
) *SessionManager {
	return &SessionManager{
		config:      cfg,
		sessionRepo: sessionRepo,
		deviceRepo:  deviceRepo,
		eventRepo:   eventRepo,
		wsManager:   wsManager,
		clients:     make(map[uuid.UUID]*whatsmeow.Client),
		devices:     make(map[uuid.UUID]*store.Device),
		logger:      log,
		stopChan:    make(chan struct{}),
	}
}

// Start starts the session manager
func (sm *SessionManager) Start(ctx context.Context) error {
	sm.logger.Info("Starting session manager")

	// Load and connect all active sessions
	sessions, err := sm.sessionRepo.GetActiveSessions(ctx)
	if err != nil {
		return fmt.Errorf("failed to get active sessions: %w", err)
	}

	for _, session := range sessions {
		if session.Status == models.StatusConnected {
			go sm.connectSession(ctx, session.ID)
		}
	}

	// Start cleanup routine
	go sm.cleanupRoutine(ctx)

	return nil
}

// Stop stops the session manager
func (sm *SessionManager) Stop() {
	sm.logger.Info("Stopping session manager")
	close(sm.stopChan)

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Disconnect all clients
	for id, client := range sm.clients {
		if client.IsConnected() {
			client.Disconnect()
		}
		delete(sm.clients, id)
		delete(sm.devices, id)
	}
}

// CreateSession creates a new WhatsApp session
func (sm *SessionManager) CreateSession(ctx context.Context, userID int, name string) (*models.WhatsAppSession, error) {
	// Check device limit
	count, err := sm.sessionRepo.CountUserSessions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to count user sessions: %w", err)
	}

	if count >= sm.config.WhatsApp.MaxDevicesPerUser {
		return nil, fmt.Errorf("device limit reached (max: %d)", sm.config.WhatsApp.MaxDevicesPerUser)
	}

	// Create session in database
	session := &models.WhatsAppSession{
		UserID:      userID,
		SessionName: name,
		Status:      models.StatusPending,
		IsActive:    true,
	}

	if err := sm.sessionRepo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Initialize WhatsApp client
	if err := sm.initializeClient(ctx, session); err != nil {
		session.Status = models.StatusFailed
		sm.sessionRepo.Update(ctx, session)
		return nil, fmt.Errorf("failed to initialize client: %w", err)
	}

	return session, nil
}

// initializeClient initializes a WhatsApp client for a session
func (sm *SessionManager) initializeClient(ctx context.Context, session *models.WhatsAppSession) error {
	// Create device store
	deviceStore, err := store.NewPostgresDevice(sm.config, session.ID, sm.logger)
	if err != nil {
		return fmt.Errorf("failed to create device store: %w", err)
	}

	// Create WhatsApp client
	client := whatsmeow.NewClient(deviceStore, nil)

	// Add event handlers
	client.AddEventHandler(sm.createEventHandler(session))

	// Store client and device
	sm.mu.Lock()
	sm.clients[session.ID] = client
	sm.devices[session.ID] = deviceStore
	sm.mu.Unlock()

	return nil
}

// GetClient returns a WhatsApp client for a session
func (sm *SessionManager) GetClient(sessionID uuid.UUID) (*whatsmeow.Client, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	client, exists := sm.clients[sessionID]
	if !exists {
		return nil, fmt.Errorf("client not found for session %s", sessionID)
	}

	return client, nil
}

// GenerateQRCode generates a QR code for session login
func (sm *SessionManager) GenerateQRCode(ctx context.Context, sessionID uuid.UUID) (*models.QRCodeData, error) {
	client, err := sm.GetClient(sessionID)
	if err != nil {
		// Try to initialize client if not found
		session, err := sm.sessionRepo.GetByID(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("session not found: %w", err)
		}

		if err := sm.initializeClient(ctx, session); err != nil {
			return nil, fmt.Errorf("failed to initialize client: %w", err)
		}

		client, err = sm.GetClient(sessionID)
		if err != nil {
			return nil, err
		}
	}

	// Check if already logged in
	if client.IsLoggedIn() {
		return nil, fmt.Errorf("already logged in")
	}

	// Generate QR channel
	qrChan, err := client.GetQRChannel(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get QR channel: %w", err)
	}

	// Connect to WhatsApp
	go func() {
		if err := client.Connect(); err != nil {
			sm.logger.Error("Failed to connect to WhatsApp: %v", err)
		}
	}()

	// Wait for QR code
	select {
	case evt := <-qrChan:
		switch evt.Event {
		case "code":
			// Generate QR code image
			qrPNG, err := qrcode.Encode(evt.Code, qrcode.Medium, 256)
			if err != nil {
				return nil, fmt.Errorf("failed to generate QR code image: %w", err)
			}

			qrBase64 := base64.StdEncoding.EncodeToString(qrPNG)

			// Update session with QR code
			session, err := sm.sessionRepo.GetByID(ctx, sessionID)
			if err != nil {
				return nil, err
			}

			now := time.Now()
			session.QRCode = &evt.Code
			session.QRCodeBase64 = &qrBase64
			session.QRGeneratedAt = &now
			expiry := now.Add(time.Duration(evt.Timeout))
			session.QRExpiresAt = &expiry
			session.Status = models.StatusQRReady

			if err := sm.sessionRepo.Update(ctx, session); err != nil {
				return nil, fmt.Errorf("failed to update session: %w", err)
			}

			// Send WebSocket event
			sm.wsManager.BroadcastToSession(sessionID, websocket.EventQRCode, map[string]interface{}{
				"code":       evt.Code,
				"base64":     qrBase64,
				"timeout":    evt.Timeout.Seconds(),
				"expires_at": expiry,
			})

			return &models.QRCodeData{
				Code:      evt.Code,
				Base64:    qrBase64,
				Timeout:   int(evt.Timeout.Seconds()),
				ExpiresAt: expiry,
			}, nil

		default:
			return nil, fmt.Errorf("unexpected QR channel event: %s", evt.Event)
		}

	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("timeout waiting for QR code")

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// connectSession connects an existing session
func (sm *SessionManager) connectSession(ctx context.Context, sessionID uuid.UUID) error {
	session, err := sm.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// Initialize client if not exists
	sm.mu.RLock()
	client, exists := sm.clients[sessionID]
	sm.mu.RUnlock()

	if !exists {
		if err := sm.initializeClient(ctx, session); err != nil {
			return fmt.Errorf("failed to initialize client: %w", err)
		}
		client = sm.clients[sessionID]
	}

	// Check if already connected
	if client.IsConnected() {
		return nil
	}

	// Connect to WhatsApp
	if err := client.Connect(); err != nil {
		session.Status = models.StatusDisconnected
		sm.sessionRepo.Update(ctx, session)
		return fmt.Errorf("failed to connect: %w", err)
	}

	return nil
}

// DisconnectSession disconnects a session
func (sm *SessionManager) DisconnectSession(ctx context.Context, sessionID uuid.UUID) error {
	client, err := sm.GetClient(sessionID)
	if err != nil {
		return err
	}

	// Disconnect from WhatsApp
	client.Disconnect()

	// Update session status
	session, err := sm.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}

	now := time.Now()
	session.Status = models.StatusDisconnected
	session.DisconnectedAt = &now

	return sm.sessionRepo.Update(ctx, session)
}

// DeleteSession deletes a session
func (sm *SessionManager) DeleteSession(ctx context.Context, sessionID uuid.UUID) error {
	// Disconnect if connected
	if client, err := sm.GetClient(sessionID); err == nil {
		client.Disconnect()
	}

	// Remove from memory
	sm.mu.Lock()
	delete(sm.clients, sessionID)
	delete(sm.devices, sessionID)
	sm.mu.Unlock()

	// Soft delete from database
	return sm.sessionRepo.Delete(ctx, sessionID)
}

// SendMessage sends a WhatsApp message
func (sm *SessionManager) SendMessage(ctx context.Context, sessionID uuid.UUID, to string, text string) (*types.MessageID, error) {
	client, err := sm.GetClient(sessionID)
	if err != nil {
		return nil, err
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in")
	}

	// Parse JID
	jid, err := types.ParseJID(to)
	if err != nil {
		return nil, fmt.Errorf("invalid JID: %w", err)
	}

	// Create message
	message := &waE2E.Message{
		Conversation: proto.String(text),
	}

	// Send message
	resp, err := client.SendMessage(ctx, jid, message)
	if err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	// Store event
	event := &models.WhatsAppEvent{
		SessionID: sessionID,
		EventType: "message_sent",
		EventData: map[string]interface{}{
			"to":         to,
			"text":       text,
			"message_id": resp.ID,
			"timestamp":  resp.Timestamp,
		},
	}
	sm.eventRepo.Create(ctx, event)

	messageID := resp.ID
	return &messageID, nil
}

// createEventHandler creates an event handler for WhatsApp events
func (sm *SessionManager) createEventHandler(session *models.WhatsAppSession) func(evt interface{}) {
	return func(evt interface{}) {
		ctx := context.Background()

		switch v := evt.(type) {
		case *events.Connected:
			sm.handleConnected(ctx, session, v)
		case *events.Disconnected:
			sm.handleDisconnected(ctx, session, v)
		case *events.QR:
			sm.handleQR(ctx, session, v)
		case *events.QRScannedWithoutMultidevice:
			sm.handleQRScanned(ctx, session)
		case *events.LoggedOut:
			sm.handleLoggedOut(ctx, session, v)
		case *events.Message:
			sm.handleMessage(ctx, session, v)
		case *events.Receipt:
			sm.handleReceipt(ctx, session, v)
		case *events.Presence:
			sm.handlePresence(ctx, session, v)
		case *events.ChatPresence:
			sm.handleChatPresence(ctx, session, v)
		case *events.HistorySync:
			sm.handleHistorySync(ctx, session, v)
		case *events.AppStateSyncComplete:
			sm.handleAppStateSync(ctx, session, v)
		}
	}
}

// Event handlers

func (sm *SessionManager) handleConnected(ctx context.Context, session *models.WhatsAppSession, evt *events.Connected) {
	sm.logger.Info("Session %s connected", session.ID)

	// Update session
	now := time.Now()
	session.Status = models.StatusConnected
	session.ConnectedAt = &now
	session.JID = &evt.ID.String()
	session.PushName = &evt.PushName

	// Get device info
	client := sm.clients[session.ID]
	if client != nil && client.Store != nil && client.Store.ID != nil {
		platform := string(client.Store.Platform)
		session.Platform = &platform

		deviceInfo := map[string]interface{}{
			"platform":      platform,
			"push_name":     evt.PushName,
			"wa_version":    client.Store.WaVersion,
			"business_name": client.Store.BusinessName,
		}
		deviceInfoJSON, _ := json.Marshal(deviceInfo)
		session.DeviceInfo = deviceInfoJSON
	}

	sm.sessionRepo.Update(ctx, session)

	// Send WebSocket event
	sm.wsManager.BroadcastToSession(session.ID, websocket.EventConnected, map[string]interface{}{
		"jid":       evt.ID.String(),
		"push_name": evt.PushName,
	})

	// Store event
	event := &models.WhatsAppEvent{
		SessionID: session.ID,
		EventType: "connected",
		EventData: map[string]interface{}{
			"jid":       evt.ID.String(),
			"push_name": evt.PushName,
		},
	}
	sm.eventRepo.Create(ctx, event)
}

func (sm *SessionManager) handleDisconnected(ctx context.Context, session *models.WhatsAppSession, evt *events.Disconnected) {
	sm.logger.Info("Session %s disconnected", session.ID)

	// Update session
	now := time.Now()
	session.Status = models.StatusDisconnected
	session.DisconnectedAt = &now

	sm.sessionRepo.Update(ctx, session)

	// Send WebSocket event
	sm.wsManager.BroadcastToSession(session.ID, websocket.EventDisconnected, nil)

	// Store event
	event := &models.WhatsAppEvent{
		SessionID: session.ID,
		EventType: "disconnected",
	}
	sm.eventRepo.Create(ctx, event)
}

func (sm *SessionManager) handleQR(ctx context.Context, session *models.WhatsAppSession, evt *events.QR) {
	sm.logger.Debug("QR code received for session %s", session.ID)

	// This is handled in GenerateQRCode method
	// We don't need to do anything here as the QR channel handles it
}

func (sm *SessionManager) handleQRScanned(ctx context.Context, session *models.WhatsAppSession) {
	sm.logger.Info("QR code scanned for session %s", session.ID)

	// Update session status
	session.Status = models.StatusScanning
	sm.sessionRepo.Update(ctx, session)

	// Send WebSocket event
	sm.wsManager.BroadcastToSession(session.ID, "qr_scanned", nil)
}

func (sm *SessionManager) handleLoggedOut(ctx context.Context, session *models.WhatsAppSession, evt *events.LoggedOut) {
	sm.logger.Info("Session %s logged out: %s", session.ID, evt.Reason)

	// Update session
	session.Status = models.StatusDisconnected
	session.IsActive = false

	sm.sessionRepo.Update(ctx, session)

	// Send WebSocket event
	sm.wsManager.BroadcastToSession(session.ID, "logged_out", map[string]interface{}{
		"reason": evt.Reason.String(),
	})

	// Store event
	event := &models.WhatsAppEvent{
		SessionID: session.ID,
		EventType: "logged_out",
		EventData: map[string]interface{}{
			"reason": evt.Reason.String(),
		},
	}
	sm.eventRepo.Create(ctx, event)
}

func (sm *SessionManager) handleMessage(ctx context.Context, session *models.WhatsAppSession, evt *events.Message) {
	sm.logger.Debug("Message received for session %s", session.ID)

	// Extract message info
	messageData := map[string]interface{}{
		"id":        evt.Info.ID,
		"from":      evt.Info.Sender.String(),
		"chat":      evt.Info.Chat.String(),
		"timestamp": evt.Info.Timestamp,
		"is_group":  evt.Info.IsGroup,
	}

	// Extract text based on message type
	if evt.Message.GetConversation() != "" {
		messageData["text"] = evt.Message.GetConversation()
	} else if evt.Message.GetExtendedTextMessage() != nil {
		messageData["text"] = evt.Message.GetExtendedTextMessage().GetText()
	}

	// Send WebSocket event
	sm.wsManager.BroadcastToSession(session.ID, "message_received", messageData)

	// Store event
	event := &models.WhatsAppEvent{
		SessionID: session.ID,
		EventType: "message_received",
		EventData: messageData,
	}
	sm.eventRepo.Create(ctx, event)
}

func (sm *SessionManager) handleReceipt(ctx context.Context, session *models.WhatsAppSession, evt *events.Receipt) {
	sm.logger.Debug("Receipt received for session %s", session.ID)

	// Store event
	event := &models.WhatsAppEvent{
		SessionID: session.ID,
		EventType: "receipt",
		EventData: map[string]interface{}{
			"message_ids": evt.MessageIDs,
			"type":        evt.Type.String(),
			"from":        evt.SourceString(),
		},
	}
	sm.eventRepo.Create(ctx, event)
}

func (sm *SessionManager) handlePresence(ctx context.Context, session *models.WhatsAppSession, evt *events.Presence) {
	// Store presence event if needed
	sm.logger.Debug("Presence update for session %s: %s is %s", session.ID, evt.From, evt.State)
}

func (sm *SessionManager) handleChatPresence(ctx context.Context, session *models.WhatsAppSession, evt *events.ChatPresence) {
	// Store chat presence event if needed
	sm.logger.Debug("Chat presence for session %s: %s in %s", session.ID, evt.State, evt.Chat)
}

func (sm *SessionManager) handleHistorySync(ctx context.Context, session *models.WhatsAppSession, evt *events.HistorySync) {
	sm.logger.Info("History sync for session %s: %d conversations", session.ID, len(evt.Data.Conversations))

	// You can process history sync here if needed
}

func (sm *SessionManager) handleAppStateSync(ctx context.Context, session *models.WhatsAppSession, evt *events.AppStateSyncComplete) {
	sm.logger.Info("App state sync complete for session %s: %s", session.ID, evt.Name)
}

// cleanupRoutine runs periodic cleanup tasks
func (sm *SessionManager) cleanupRoutine(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sm.cleanup(ctx)
		case <-sm.stopChan:
			return
		case <-ctx.Done():
			return
		}
	}
}

// cleanup performs cleanup tasks
func (sm *SessionManager) cleanup(ctx context.Context) {
	// Clean expired QR codes
	if err := sm.sessionRepo.CleanExpiredQRCodes(ctx); err != nil {
		sm.logger.Error("Failed to clean expired QR codes: %v", err)
	}

	// Reconnect disconnected sessions
	sessions, err := sm.sessionRepo.GetDisconnectedSessions(ctx)
	if err != nil {
		sm.logger.Error("Failed to get disconnected sessions: %v", err)
		return
	}

	for _, session := range sessions {
		if session.IsActive {
			go sm.connectSession(ctx, session.ID)
		}
	}
}
