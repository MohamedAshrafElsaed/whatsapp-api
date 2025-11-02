// internal/handlers/session_handler.go
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"whatsapp-api/internal/models"
	"whatsapp-api/internal/repositories"
	"whatsapp-api/internal/services"
	"whatsapp-api/internal/websocket"
	"whatsapp-api/pkg/logger"
	"whatsapp-api/pkg/response"
)

// SessionHandler handles session-related HTTP requests
type SessionHandler struct {
	sessionManager *services.SessionManager
	sessionRepo    *repositories.SessionRepository
	eventRepo      *repositories.EventRepository
	wsManager      *websocket.Manager
	logger         *logger.Logger
}

// NewSessionHandler creates a new session handler
func NewSessionHandler(
	sessionManager *services.SessionManager,
	sessionRepo *repositories.SessionRepository,
	eventRepo *repositories.EventRepository,
	wsManager *websocket.Manager,
	logger *logger.Logger,
) *SessionHandler {
	return &SessionHandler{
		sessionManager: sessionManager,
		sessionRepo:    sessionRepo,
		eventRepo:      eventRepo,
		wsManager:      wsManager,
		logger:         logger,
	}
}

// CreateSession handles session creation requests
func (h *SessionHandler) CreateSession(c *gin.Context) {
	userID := c.GetInt("user_id")

	var req models.CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}

	// Validate session name
	if req.SessionName == "" {
		response.BadRequest(c, "Session name is required")
		return
	}

	// Create session
	session, err := h.sessionManager.CreateSession(c.Request.Context(), userID, req.SessionName)
	if err != nil {
		h.logger.Error("Failed to create session: %v", err)
		response.InternalError(c, "Failed to create session")
		return
	}

	// Log event
	event := &models.WhatsAppEvent{
		SessionID: session.ID,
		EventType: "session_created",
		EventData: map[string]interface{}{
			"session_name": req.SessionName,
			"user_id":      userID,
		},
	}
	h.eventRepo.Create(c.Request.Context(), event)

	response.Created(c, gin.H{
		"session_id":   session.ID,
		"session_name": session.SessionName,
		"status":       session.Status,
		"created_at":   session.CreatedAt,
	})
}

// ListSessions handles listing all user sessions
func (h *SessionHandler) ListSessions(c *gin.Context) {
	userID := c.GetInt("user_id")

	// Get all sessions for user
	sessions, err := h.sessionRepo.GetByUserID(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get sessions: %v", err)
		response.InternalError(c, "Failed to retrieve sessions")
		return
	}

	// Get device count
	count, err := h.sessionRepo.CountUserSessions(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to count sessions: %v", err)
	}

	// Build response
	sessionList := make([]gin.H, 0, len(sessions))
	connectedCount := 0
	for _, session := range sessions {
		if session.Status == models.StatusConnected {
			connectedCount++
		}

		sessionData := gin.H{
			"session_id":   session.ID,
			"session_name": session.SessionName,
			"status":       session.Status,
			"phone_number": session.PhoneNumber,
			"jid":          session.JID,
			"push_name":    session.PushName,
			"is_active":    session.IsActive,
			"connected_at": session.ConnectedAt,
			"last_seen":    session.LastSeen,
			"created_at":   session.CreatedAt,
		}
		sessionList = append(sessionList, sessionData)
	}

	response.Success(c, gin.H{
		"sessions": sessionList,
		"summary": gin.H{
			"total_sessions":  len(sessions),
			"connected":       connectedCount,
			"pending":         count - connectedCount,
			"max_devices":     5,
			"available_slots": 5 - count,
		},
	})
}

// GetSession handles getting a specific session
func (h *SessionHandler) GetSession(c *gin.Context) {
	userID := c.GetInt("user_id")
	sessionIDStr := c.Param("session_id")

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}

	// Get session
	session, err := h.sessionRepo.GetByID(c.Request.Context(), sessionID)
	if err != nil {
		response.NotFound(c, "Session not found")
		return
	}

	// Check ownership
	if session.UserID != userID {
		response.Forbidden(c, "Access denied")
		return
	}

	response.Success(c, gin.H{
		"session_id":      session.ID,
		"session_name":    session.SessionName,
		"status":          session.Status,
		"phone_number":    session.PhoneNumber,
		"jid":             session.JID,
		"push_name":       session.PushName,
		"platform":        session.Platform,
		"is_active":       session.IsActive,
		"connected_at":    session.ConnectedAt,
		"disconnected_at": session.DisconnectedAt,
		"last_seen":       session.LastSeen,
		"created_at":      session.CreatedAt,
		"updated_at":      session.UpdatedAt,
	})
}

// GenerateQR handles QR code generation requests
func (h *SessionHandler) GenerateQR(c *gin.Context) {
	userID := c.GetInt("user_id")
	sessionIDStr := c.Param("session_id")
	format := c.DefaultQuery("format", "json")

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}

	// Check session ownership
	session, err := h.sessionRepo.GetByID(c.Request.Context(), sessionID)
	if err != nil {
		response.NotFound(c, "Session not found")
		return
	}

	if session.UserID != userID {
		response.Forbidden(c, "Access denied")
		return
	}

	// Generate QR code
	qrData, err := h.sessionManager.GenerateQRCode(c.Request.Context(), sessionID)
	if err != nil {
		h.logger.Error("Failed to generate QR code: %v", err)
		response.BadRequest(c, err.Error())
		return
	}

	// Return based on format
	if format == "png" {
		// Return raw PNG image
		c.Header("Content-Type", "image/png")
		c.String(http.StatusOK, qrData.Base64)
	} else {
		// Return JSON response
		response.Success(c, gin.H{
			"qr_code":    "data:image/png;base64," + qrData.Base64,
			"qr_data":    qrData.Code,
			"expires_at": qrData.ExpiresAt,
			"timeout":    qrData.Timeout,
		})
	}

	// Log event
	event := &models.WhatsAppEvent{
		SessionID: sessionID,
		EventType: "qr_generated",
		EventData: map[string]interface{}{
			"format": format,
		},
	}
	h.eventRepo.Create(c.Request.Context(), event)
}

// GetStatus handles session status requests
func (h *SessionHandler) GetStatus(c *gin.Context) {
	userID := c.GetInt("user_id")
	sessionIDStr := c.Param("session_id")

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}

	// Get session
	session, err := h.sessionRepo.GetByID(c.Request.Context(), sessionID)
	if err != nil {
		response.NotFound(c, "Session not found")
		return
	}

	// Check ownership
	if session.UserID != userID {
		response.Forbidden(c, "Access denied")
		return
	}

	// Get client status
	client, _ := h.sessionManager.GetClient(sessionID)
	isConnected := false
	isLoggedIn := false

	if client != nil {
		isConnected = client.IsConnected()
		isLoggedIn = client.IsLoggedIn()
	}

	response.Success(c, gin.H{
		"session_id":   session.ID,
		"status":       session.Status,
		"is_connected": isConnected,
		"is_logged_in": isLoggedIn,
		"phone_number": session.PhoneNumber,
		"jid":          session.JID,
		"push_name":    session.PushName,
		"last_seen":    session.LastSeen,
	})
}

// Connect handles session connection requests
func (h *SessionHandler) Connect(c *gin.Context) {
	userID := c.GetInt("user_id")
	sessionIDStr := c.Param("session_id")

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}

	// Check session ownership
	session, err := h.sessionRepo.GetByID(c.Request.Context(), sessionID)
	if err != nil {
		response.NotFound(c, "Session not found")
		return
	}

	if session.UserID != userID {
		response.Forbidden(c, "Access denied")
		return
	}

	// Connect session
	if err := h.sessionManager.ConnectSession(c.Request.Context(), sessionID); err != nil {
		h.logger.Error("Failed to connect session: %v", err)
		response.InternalError(c, "Failed to connect session")
		return
	}

	response.Success(c, gin.H{
		"message":    "Session connecting",
		"session_id": sessionID,
	})
}

// Disconnect handles session disconnection requests
func (h *SessionHandler) Disconnect(c *gin.Context) {
	userID := c.GetInt("user_id")
	sessionIDStr := c.Param("session_id")

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}

	// Check session ownership
	session, err := h.sessionRepo.GetByID(c.Request.Context(), sessionID)
	if err != nil {
		response.NotFound(c, "Session not found")
		return
	}

	if session.UserID != userID {
		response.Forbidden(c, "Access denied")
		return
	}

	// Disconnect session
	if err := h.sessionManager.DisconnectSession(c.Request.Context(), sessionID); err != nil {
		h.logger.Error("Failed to disconnect session: %v", err)
		response.InternalError(c, "Failed to disconnect session")
		return
	}

	response.Success(c, gin.H{
		"message":    "Session disconnected",
		"session_id": sessionID,
	})
}

// DeleteSession handles session deletion requests
func (h *SessionHandler) DeleteSession(c *gin.Context) {
	userID := c.GetInt("user_id")
	sessionIDStr := c.Param("session_id")

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}

	// Check session ownership
	session, err := h.sessionRepo.GetByID(c.Request.Context(), sessionID)
	if err != nil {
		response.NotFound(c, "Session not found")
		return
	}

	if session.UserID != userID {
		response.Forbidden(c, "Access denied")
		return
	}

	// Delete session
	if err := h.sessionManager.DeleteSession(c.Request.Context(), sessionID); err != nil {
		h.logger.Error("Failed to delete session: %v", err)
		response.InternalError(c, "Failed to delete session")
		return
	}

	// Log event
	event := &models.WhatsAppEvent{
		SessionID: sessionID,
		EventType: "session_deleted",
		EventData: map[string]interface{}{
			"user_id": userID,
		},
	}
	h.eventRepo.Create(c.Request.Context(), event)

	response.Success(c, gin.H{
		"message": "Session deleted successfully",
	})
}
