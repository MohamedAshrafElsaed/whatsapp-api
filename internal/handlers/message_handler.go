// internal/handlers/message_handler.go
package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"whatsapp-api/internal/models"
	"whatsapp-api/internal/services"
	"whatsapp-api/pkg/logger"
	"whatsapp-api/pkg/response"
)

// MessageHandler handles message-related HTTP requests
type MessageHandler struct {
	messageService *services.MessageService
	logger         *logger.Logger
}

// NewMessageHandler creates a new message handler
func NewMessageHandler(messageService *services.MessageService, logger *logger.Logger) *MessageHandler {
	return &MessageHandler{
		messageService: messageService,
		logger:         logger,
	}
}

// SendMessage handles sending text messages
func (h *MessageHandler) SendMessage(c *gin.Context) {
	var req models.SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}

	resp, err := h.messageService.SendTextMessage(c.Request.Context(), sessionID, &req)
	if err != nil {
		h.logger.Error("Failed to send message: %v", err)
		response.InternalError(c, "Failed to send message")
		return
	}

	response.Success(c, resp)
}

// SendImage handles sending image messages
func (h *MessageHandler) SendImage(c *gin.Context) {
	var req models.SendMediaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}

	resp, err := h.messageService.SendImageMessage(c.Request.Context(), sessionID, &req)
	if err != nil {
		h.logger.Error("Failed to send image: %v", err)
		response.InternalError(c, "Failed to send image")
		return
	}

	response.Success(c, resp)
}

// SendDocument handles sending document messages
func (h *MessageHandler) SendDocument(c *gin.Context) {
	var req models.SendMediaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}

	resp, err := h.messageService.SendDocumentMessage(c.Request.Context(), sessionID, &req)
	if err != nil {
		response.InternalError(c, "Failed to send document")
		return
	}

	response.Success(c, resp)
}

// SendAudio handles sending audio messages
func (h *MessageHandler) SendAudio(c *gin.Context) {
	var req models.SendMediaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}

	resp, err := h.messageService.SendAudioMessage(c.Request.Context(), sessionID, &req)
	if err != nil {
		response.InternalError(c, "Failed to send audio")
		return
	}

	response.Success(c, resp)
}

// SendVideo handles sending video messages
func (h *MessageHandler) SendVideo(c *gin.Context) {
	var req models.SendMediaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}

	resp, err := h.messageService.SendVideoMessage(c.Request.Context(), sessionID, &req)
	if err != nil {
		response.InternalError(c, "Failed to send video")
		return
	}

	response.Success(c, resp)
}

// SendLocation handles sending location messages
func (h *MessageHandler) SendLocation(c *gin.Context) {
	var req models.SendLocationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}

	resp, err := h.messageService.SendLocationMessage(c.Request.Context(), sessionID, &req)
	if err != nil {
		response.InternalError(c, "Failed to send location")
		return
	}

	response.Success(c, resp)
}

// SendContact handles sending contact messages
func (h *MessageHandler) SendContact(c *gin.Context) {
	var req models.SendContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}

	resp, err := h.messageService.SendContactMessage(c.Request.Context(), sessionID, &req)
	if err != nil {
		response.InternalError(c, "Failed to send contact")
		return
	}

	response.Success(c, resp)
}

// BroadcastMessage handles broadcasting messages
func (h *MessageHandler) BroadcastMessage(c *gin.Context) {
	var req models.BroadcastRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}

	resp, err := h.messageService.BroadcastMessage(c.Request.Context(), sessionID, &req)
	if err != nil {
		response.InternalError(c, "Failed to broadcast message")
		return
	}

	response.Success(c, resp)
}

// GetMessages retrieves messages for a session
func (h *MessageHandler) GetMessages(c *gin.Context) {
	sessionIDStr := c.Param("session_id")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}

	// Parse filter parameters
	filter := &models.MessageFilter{
		Limit:  20,
		Offset: 0,
	}

	messages, err := h.messageService.GetMessages(c.Request.Context(), sessionID, filter)
	if err != nil {
		response.InternalError(c, "Failed to get messages")
		return
	}

	response.Success(c, gin.H{"messages": messages})
}

// GetMessage retrieves a specific message
func (h *MessageHandler) GetMessage(c *gin.Context) {
	sessionIDStr := c.Param("session_id")
	messageID := c.Param("message_id")

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}

	message, err := h.messageService.GetMessage(c.Request.Context(), sessionID, messageID)
	if err != nil {
		response.NotFound(c, "Message not found")
		return
	}

	response.Success(c, message)
}

// MarkAsRead marks a message as read
func (h *MessageHandler) MarkAsRead(c *gin.Context) {
	sessionIDStr := c.Param("session_id")
	messageID := c.Param("message_id")

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}

	err = h.messageService.MarkMessageAsRead(c.Request.Context(), sessionID, messageID)
	if err != nil {
		response.InternalError(c, "Failed to mark message as read")
		return
	}

	response.Success(c, gin.H{"message": "Message marked as read"})
}
