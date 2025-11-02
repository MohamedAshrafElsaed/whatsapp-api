package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"whatsapp-api/internal/config"
	"whatsapp-api/internal/dto"
	"whatsapp-api/internal/models"
)

// WebSocketManager manages WebSocket connections
type WebSocketManager struct {
	clients    map[string]*WebSocketClient
	broadcast  chan *BroadcastMessage
	register   chan *WebSocketClient
	unregister chan *WebSocketClient
	mu         sync.RWMutex
	config     *config.Config
}

// WebSocketClient represents a connected WebSocket client
type WebSocketClient struct {
	ID        string
	UserID    int
	SessionID *uuid.UUID
	Conn      *websocket.Conn
	Send      chan []byte
	Manager   *WebSocketManager
	mu        sync.Mutex
}

// BroadcastMessage represents a message to be broadcast
type BroadcastMessage struct {
	Type      BroadcastType
	UserID    *int
	SessionID *uuid.UUID
	Data      interface{}
}

// BroadcastType represents the type of broadcast
type BroadcastType int

const (
	BroadcastToAll BroadcastType = iota
	BroadcastToUser
	BroadcastToSession
)

// NewWebSocketManager creates a new WebSocket manager
func NewWebSocketManager(cfg *config.Config) *WebSocketManager {
	return &WebSocketManager{
		clients:    make(map[string]*WebSocketClient),
		broadcast:  make(chan *BroadcastMessage, 256),
		register:   make(chan *WebSocketClient),
		unregister: make(chan *WebSocketClient),
		config:     cfg,
	}
}

// Run starts the WebSocket manager
func (m *WebSocketManager) Run(ctx context.Context) {
	log.Println("WebSocket manager started")

	for {
		select {
		case <-ctx.Done():
			log.Println("WebSocket manager stopping...")
			m.shutdown()
			return

		case client := <-m.register:
			m.registerClient(client)

		case client := <-m.unregister:
			m.unregisterClient(client)

		case message := <-m.broadcast:
			m.handleBroadcast(message)
		}
	}
}

// registerClient registers a new WebSocket client
func (m *WebSocketManager) registerClient(client *WebSocketClient) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.clients[client.ID] = client
	log.Printf("WebSocket client registered: %s (UserID: %d)", client.ID, client.UserID)
}

// unregisterClient unregisters a WebSocket client
func (m *WebSocketManager) unregisterClient(client *WebSocketClient) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.clients[client.ID]; ok {
		delete(m.clients, client.ID)
		close(client.Send)
		log.Printf("WebSocket client unregistered: %s (UserID: %d)", client.ID, client.UserID)
	}
}

// handleBroadcast handles broadcasting messages
func (m *WebSocketManager) handleBroadcast(message *BroadcastMessage) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	switch message.Type {
	case BroadcastToAll:
		m.broadcastToAll(message.Data)
	case BroadcastToUser:
		if message.UserID != nil {
			m.broadcastToUser(*message.UserID, message.Data)
		}
	case BroadcastToSession:
		if message.SessionID != nil {
			m.broadcastToSession(*message.SessionID, message.Data)
		}
	}
}

// broadcastToAll broadcasts to all connected clients
func (m *WebSocketManager) broadcastToAll(data interface{}) {
	message, err := m.encodeMessage(data)
	if err != nil {
		log.Printf("Failed to encode broadcast message: %v", err)
		return
	}

	for _, client := range m.clients {
		select {
		case client.Send <- message:
		default:
			// Client's send channel is full, skip
			log.Printf("Failed to send to client %s (channel full)", client.ID)
		}
	}
}

// broadcastToUser broadcasts to all clients of a specific user
func (m *WebSocketManager) broadcastToUser(userID int, data interface{}) {
	message, err := m.encodeMessage(data)
	if err != nil {
		log.Printf("Failed to encode broadcast message: %v", err)
		return
	}

	count := 0
	for _, client := range m.clients {
		if client.UserID == userID {
			select {
			case client.Send <- message:
				count++
			default:
				log.Printf("Failed to send to client %s (channel full)", client.ID)
			}
		}
	}

	log.Printf("Broadcast to user %d: %d clients", userID, count)
}

// broadcastToSession broadcasts to all clients watching a specific session
func (m *WebSocketManager) broadcastToSession(sessionID uuid.UUID, data interface{}) {
	message, err := m.encodeMessage(data)
	if err != nil {
		log.Printf("Failed to encode broadcast message: %v", err)
		return
	}

	count := 0
	for _, client := range m.clients {
		if client.SessionID != nil && *client.SessionID == sessionID {
			select {
			case client.Send <- message:
				count++
			default:
				log.Printf("Failed to send to client %s (channel full)", client.ID)
			}
		}
	}

	log.Printf("Broadcast to session %s: %d clients", sessionID, count)
}

// encodeMessage encodes data to JSON
func (m *WebSocketManager) encodeMessage(data interface{}) ([]byte, error) {
	return json.Marshal(data)
}

// SendToUser sends a message to a specific user
func (m *WebSocketManager) SendToUser(userID int, data interface{}) {
	m.broadcast <- &BroadcastMessage{
		Type:   BroadcastToUser,
		UserID: &userID,
		Data:   data,
	}
}

// SendToSession sends a message to all clients watching a session
func (m *WebSocketManager) SendToSession(sessionID uuid.UUID, data interface{}) {
	m.broadcast <- &BroadcastMessage{
		Type:      BroadcastToSession,
		SessionID: &sessionID,
		Data:      data,
	}
}

// SendToAll sends a message to all connected clients
func (m *WebSocketManager) SendToAll(data interface{}) {
	m.broadcast <- &BroadcastMessage{
		Type: BroadcastToAll,
		Data: data,
	}
}

// NewClient creates a new WebSocket client
func (m *WebSocketManager) NewClient(conn *websocket.Conn, userID int, sessionID *uuid.UUID) *WebSocketClient {
	return &WebSocketClient{
		ID:        generateClientID(),
		UserID:    userID,
		SessionID: sessionID,
		Conn:      conn,
		Send:      make(chan []byte, 256),
		Manager:   m,
	}
}

// RegisterClient registers a client with the manager
func (m *WebSocketManager) RegisterClient(client *WebSocketClient) {
	m.register <- client
}

// UnregisterClient unregisters a client from the manager
func (m *WebSocketManager) UnregisterClient(client *WebSocketClient) {
	m.unregister <- client
}

// GetClientCount returns the number of connected clients
func (m *WebSocketManager) GetClientCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.clients)
}

// GetUserClientCount returns the number of clients for a specific user
func (m *WebSocketManager) GetUserClientCount(userID int) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, client := range m.clients {
		if client.UserID == userID {
			count++
		}
	}
	return count
}

// GetSessionClientCount returns the number of clients watching a session
func (m *WebSocketManager) GetSessionClientCount(sessionID uuid.UUID) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, client := range m.clients {
		if client.SessionID != nil && *client.SessionID == sessionID {
			count++
		}
	}
	return count
}

// shutdown gracefully shuts down the WebSocket manager
func (m *WebSocketManager) shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Close all client connections
	for _, client := range m.clients {
		client.Close()
	}

	// Clear clients map
	m.clients = make(map[string]*WebSocketClient)

	log.Println("WebSocket manager shut down")
}

// SendQRCode sends a QR code to a user
func (m *WebSocketManager) SendQRCode(userID int, sessionID uuid.UUID, qrCode, qrData string, expiresAt time.Time) {
	message := dto.NewWebSocketMessage("qr_code", sessionID.String(), &dto.QRCodeWebSocketData{
		QRCode:    qrCode,
		ExpiresAt: expiresAt,
	})

	m.SendToUser(userID, message)
}

// SendStatusChange sends a status change notification
func (m *WebSocketManager) SendStatusChange(userID int, sessionID uuid.UUID, oldStatus, newStatus models.SessionStatus) {
	message := dto.NewWebSocketMessage("status_change", sessionID.String(), &dto.StatusChangeWebSocketData{
		OldStatus: oldStatus,
		NewStatus: newStatus,
		Timestamp: time.Now(),
	})

	m.SendToUser(userID, message)
}

// SendConnected sends a connected notification
func (m *WebSocketManager) SendConnected(userID int, sessionID uuid.UUID, jid, phoneNumber, pushName string) {
	message := dto.NewWebSocketMessage("connected", sessionID.String(), &dto.ConnectionWebSocketData{
		JID:         jid,
		PhoneNumber: phoneNumber,
		PushName:    pushName,
		ConnectedAt: time.Now(),
	})

	m.SendToUser(userID, message)
}

// SendDisconnected sends a disconnected notification
func (m *WebSocketManager) SendDisconnected(userID int, sessionID uuid.UUID) {
	message := dto.NewWebSocketMessage("disconnected", sessionID.String(), map[string]interface{}{
		"timestamp": time.Now(),
	})

	m.SendToUser(userID, message)
}

// SendError sends an error notification
func (m *WebSocketManager) SendError(userID int, sessionID uuid.UUID, errorMsg string) {
	message := dto.NewWebSocketMessage("error", sessionID.String(), map[string]interface{}{
		"error":     errorMsg,
		"timestamp": time.Now(),
	})

	m.SendToUser(userID, message)
}

// WebSocketClient methods

// ReadPump reads messages from the WebSocket connection
func (c *WebSocketClient) ReadPump() {
	defer func() {
		c.Manager.UnregisterClient(c)
		c.Conn.Close()
	}()

	c.Conn.SetReadDeadline(time.Now().Add(c.Manager.config.WebSocket.PongTimeout))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(c.Manager.config.WebSocket.PongTimeout))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Handle incoming messages if needed
		c.handleMessage(message)
	}
}

// WritePump writes messages to the WebSocket connection
func (c *WebSocketClient) WritePump() {
	ticker := time.NewTicker(c.Manager.config.WebSocket.PingInterval)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(c.Manager.config.WebSocket.WriteTimeout))
			if !ok {
				// Channel closed
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("WebSocket write error: %v", err)
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(c.Manager.config.WebSocket.WriteTimeout))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage handles incoming WebSocket messages
func (c *WebSocketClient) handleMessage(message []byte) {
	// Parse and handle incoming messages if needed
	// For now, this is a placeholder
	log.Printf("Received message from client %s: %s", c.ID, string(message))
}

// Close closes the WebSocket connection
func (c *WebSocketClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Conn != nil {
		c.Conn.Close()
	}
}

// SendMessage sends a message to the client
func (c *WebSocketClient) SendMessage(data interface{}) error {
	message, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	select {
	case c.Send <- message:
		return nil
	default:
		return fmt.Errorf("client send channel is full")
	}
}

// Helper functions

// generateClientID generates a unique client ID
func generateClientID() string {
	return fmt.Sprintf("client_%d_%s", time.Now().UnixNano(), uuid.New().String()[:8])
}

// IsConnected checks if the client is connected
func (c *WebSocketClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn != nil
}

// GetUserID returns the client's user ID
func (c *WebSocketClient) GetUserID() int {
	return c.UserID
}

// GetSessionID returns the client's session ID
func (c *WebSocketClient) GetSessionID() *uuid.UUID {
	return c.SessionID
}

// SetSessionID sets the client's session ID
func (c *WebSocketClient) SetSessionID(sessionID uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SessionID = &sessionID
}
