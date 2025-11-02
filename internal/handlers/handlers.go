// internal/handlers/handlers.go
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"whatsapp-api/internal/models"
	"whatsapp-api/internal/repositories"
	"whatsapp-api/internal/services"
	"whatsapp-api/internal/websocket"
	"whatsapp-api/pkg/logger"
	"whatsapp-api/pkg/response"
)

// GroupHandler handles group-related requests
type GroupHandler struct {
	groupService *services.GroupService
	logger       *logger.Logger
}

func NewGroupHandler(groupService *services.GroupService, logger *logger.Logger) *GroupHandler {
	return &GroupHandler{groupService: groupService, logger: logger}
}

func (h *GroupHandler) CreateGroup(c *gin.Context) {
	var req models.CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	sessionID, _ := uuid.Parse(req.SessionID)
	resp, err := h.groupService.CreateGroup(c.Request.Context(), sessionID, &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Created(c, resp)
}

func (h *GroupHandler) GetGroups(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	groups, err := h.groupService.GetGroups(c.Request.Context(), sessionID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"groups": groups})
}

func (h *GroupHandler) GetGroupInfo(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	groupJID := c.Param("group_id")
	info, err := h.groupService.GetGroupInfo(c.Request.Context(), sessionID, groupJID)
	if err != nil {
		response.NotFound(c, "Group not found")
		return
	}
	response.Success(c, info)
}

func (h *GroupHandler) JoinGroup(c *gin.Context) {
	var req models.JoinGroupRequest
	c.ShouldBindJSON(&req)
	sessionID, _ := uuid.Parse(req.SessionID)
	resp, err := h.groupService.JoinGroup(c.Request.Context(), sessionID, req.InviteLink)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *GroupHandler) LeaveGroup(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	groupJID := c.Param("group_id")
	err := h.groupService.LeaveGroup(c.Request.Context(), sessionID, groupJID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Left group successfully"})
}

func (h *GroupHandler) UpdateGroupInfo(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	groupJID := c.Param("group_id")
	var req models.UpdateGroupRequest
	c.ShouldBindJSON(&req)
	err := h.groupService.UpdateGroupInfo(c.Request.Context(), sessionID, groupJID, &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Group updated"})
}

func (h *GroupHandler) UpdateGroupPhoto(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	groupJID := c.Param("group_id")
	var req models.UpdateGroupPhotoRequest
	c.ShouldBindJSON(&req)
	err := h.groupService.UpdateGroupPhoto(c.Request.Context(), sessionID, groupJID, &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Photo updated"})
}

func (h *GroupHandler) AddParticipants(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	groupJID := c.Param("group_id")
	var req models.GroupParticipantsRequest
	c.ShouldBindJSON(&req)
	resp, err := h.groupService.AddParticipants(c.Request.Context(), sessionID, groupJID, &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *GroupHandler) RemoveParticipants(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	groupJID := c.Param("group_id")
	var req models.GroupParticipantsRequest
	c.ShouldBindJSON(&req)
	resp, err := h.groupService.RemoveParticipants(c.Request.Context(), sessionID, groupJID, &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *GroupHandler) PromoteParticipants(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	groupJID := c.Param("group_id")
	var req models.GroupParticipantsRequest
	c.ShouldBindJSON(&req)
	resp, err := h.groupService.PromoteParticipants(c.Request.Context(), sessionID, groupJID, &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *GroupHandler) DemoteParticipants(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	groupJID := c.Param("group_id")
	var req models.GroupParticipantsRequest
	c.ShouldBindJSON(&req)
	resp, err := h.groupService.DemoteParticipants(c.Request.Context(), sessionID, groupJID, &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *GroupHandler) GetInviteLink(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	groupJID := c.Param("group_id")
	reset := c.Query("reset") == "true"
	link, err := h.groupService.GetGroupInviteLink(c.Request.Context(), sessionID, groupJID, reset)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"invite_link": link})
}

// ContactHandler handles contact-related requests
type ContactHandler struct {
	contactService *services.ContactService
	logger         *logger.Logger
}

func NewContactHandler(contactService *services.ContactService, logger *logger.Logger) *ContactHandler {
	return &ContactHandler{contactService: contactService, logger: logger}
}

func (h *ContactHandler) SyncContacts(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	resp, err := h.contactService.SyncContacts(c.Request.Context(), sessionID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *ContactHandler) GetContacts(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	contacts, err := h.contactService.GetContacts(c.Request.Context(), sessionID, nil)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"contacts": contacts})
}

func (h *ContactHandler) GetContact(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	jid := c.Param("jid")
	contact, err := h.contactService.GetContact(c.Request.Context(), sessionID, jid)
	if err != nil {
		response.NotFound(c, "Contact not found")
		return
	}
	response.Success(c, contact)
}

func (h *ContactHandler) GetProfile(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	jid := c.Param("jid")
	profile, err := h.contactService.GetContactProfile(c.Request.Context(), sessionID, jid)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.Success(c, profile)
}

func (h *ContactHandler) GetProfilePicture(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	jid := c.Param("jid")
	pic, err := h.contactService.GetProfilePicture(c.Request.Context(), sessionID, jid)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.Success(c, pic)
}

func (h *ContactHandler) CheckContactsExist(c *gin.Context) {
	var req models.CheckContactsRequest
	c.ShouldBindJSON(&req)
	sessionID, _ := uuid.Parse(req.SessionID)
	resp, err := h.contactService.CheckContactsExist(c.Request.Context(), sessionID, req.PhoneNumbers)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *ContactHandler) BlockContact(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	jid := c.Param("jid")
	err := h.contactService.BlockContact(c.Request.Context(), sessionID, jid)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Contact blocked"})
}

func (h *ContactHandler) UnblockContact(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	jid := c.Param("jid")
	err := h.contactService.UnblockContact(c.Request.Context(), sessionID, jid)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Contact unblocked"})
}

func (h *ContactHandler) GetBlockedContacts(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	contacts, err := h.contactService.GetBlockedContacts(c.Request.Context(), sessionID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"blocked_contacts": contacts})
}

func (h *ContactHandler) SubscribePresence(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	jid := c.Param("jid")
	err := h.contactService.SubscribePresence(c.Request.Context(), sessionID, jid)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Subscribed to presence"})
}

func (h *ContactHandler) GetPresence(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	jid := c.Param("jid")
	presence, err := h.contactService.GetPresence(c.Request.Context(), sessionID, jid)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.Success(c, presence)
}

func (h *ContactHandler) GetCommonGroups(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	jid := c.Param("jid")
	groups, err := h.contactService.GetCommonGroups(c.Request.Context(), sessionID, jid)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"groups": groups})
}

// StatusHandler handles status-related requests
type StatusHandler struct {
	statusService *services.StatusService
	logger        *logger.Logger
}

func NewStatusHandler(statusService *services.StatusService, logger *logger.Logger) *StatusHandler {
	return &StatusHandler{statusService: statusService, logger: logger}
}

func (h *StatusHandler) PostTextStatus(c *gin.Context) {
	var req models.PostStatusRequest
	c.ShouldBindJSON(&req)
	sessionID, _ := uuid.Parse(req.SessionID)
	resp, err := h.statusService.PostTextStatus(c.Request.Context(), sessionID, &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Created(c, resp)
}

func (h *StatusHandler) PostImageStatus(c *gin.Context) {
	var req models.PostMediaStatusRequest
	c.ShouldBindJSON(&req)
	sessionID, _ := uuid.Parse(req.SessionID)
	resp, err := h.statusService.PostImageStatus(c.Request.Context(), sessionID, &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Created(c, resp)
}

func (h *StatusHandler) PostVideoStatus(c *gin.Context) {
	var req models.PostMediaStatusRequest
	c.ShouldBindJSON(&req)
	sessionID, _ := uuid.Parse(req.SessionID)
	resp, err := h.statusService.PostVideoStatus(c.Request.Context(), sessionID, &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Created(c, resp)
}

func (h *StatusHandler) GetMyStatuses(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	statuses, err := h.statusService.GetMyStatuses(c.Request.Context(), sessionID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"statuses": statuses})
}

func (h *StatusHandler) GetContactStatuses(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	statuses, err := h.statusService.GetContactStatuses(c.Request.Context(), sessionID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"statuses": statuses})
}

func (h *StatusHandler) ViewStatus(c *gin.Context) {
	var req models.ViewStatusRequest
	c.ShouldBindJSON(&req)
	sessionID, _ := uuid.Parse(req.SessionID)
	err := h.statusService.ViewStatus(c.Request.Context(), sessionID, req.StatusID, req.ContactJID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Status viewed"})
}

func (h *StatusHandler) DeleteStatus(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	statusID := c.Param("status_id")
	err := h.statusService.DeleteStatus(c.Request.Context(), sessionID, statusID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Status deleted"})
}

func (h *StatusHandler) GetPrivacy(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	privacy, err := h.statusService.GetStatusPrivacy(c.Request.Context(), sessionID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, privacy)
}

func (h *StatusHandler) UpdatePrivacy(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	var req models.UpdateStatusPrivacyRequest
	c.ShouldBindJSON(&req)
	err := h.statusService.UpdateStatusPrivacy(c.Request.Context(), sessionID, &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Privacy updated"})
}

func (h *StatusHandler) GetViewers(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	statusID := c.Param("status_id")
	viewers, err := h.statusService.GetStatusViewers(c.Request.Context(), sessionID, statusID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"viewers": viewers})
}

func (h *StatusHandler) MuteUpdates(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	contactJID := c.Param("contact_jid")
	err := h.statusService.MuteStatusUpdates(c.Request.Context(), sessionID, contactJID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Updates muted"})
}

func (h *StatusHandler) UnmuteUpdates(c *gin.Context) {
	sessionID, _ := uuid.Parse(c.Param("session_id"))
	contactJID := c.Param("contact_jid")
	err := h.statusService.UnmuteStatusUpdates(c.Request.Context(), sessionID, contactJID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Updates unmuted"})
}

// DeviceHandler handles device management
type DeviceHandler struct {
	sessionRepo *repositories.SessionRepository
	deviceRepo  *repositories.DeviceRepository
	logger      *logger.Logger
}

func NewDeviceHandler(sessionRepo *repositories.SessionRepository, deviceRepo *repositories.DeviceRepository, logger *logger.Logger) *DeviceHandler {
	return &DeviceHandler{sessionRepo: sessionRepo, deviceRepo: deviceRepo, logger: logger}
}

func (h *DeviceHandler) GetDeviceSummary(c *gin.Context) {
	userID := c.GetInt("user_id")
	sessions, _ := h.sessionRepo.GetByUserID(c.Request.Context(), userID)
	count, _ := h.sessionRepo.CountUserSessions(c.Request.Context(), userID)

	response.Success(c, gin.H{
		"user_id":           userID,
		"max_devices":       5,
		"used_devices":      count,
		"available_slots":   5 - count,
		"connected_devices": len(sessions),
		"devices":           sessions,
	})
}

func (h *DeviceHandler) GetDeviceLimit(c *gin.Context) {
	userID := c.GetInt("user_id")
	count, _ := h.sessionRepo.CountUserSessions(c.Request.Context(), userID)

	response.Success(c, gin.H{
		"limit":     5,
		"used":      count,
		"available": 5 - count,
	})
}

// WebSocketHandler handles WebSocket connections
type WebSocketHandler struct {
	wsManager   *websocket.Manager
	sessionRepo *repositories.SessionRepository
	logger      *logger.Logger
	upgrader    websocket.Upgrader
}

func NewWebSocketHandler(wsManager *websocket.Manager, sessionRepo *repositories.SessionRepository, logger *logger.Logger) *WebSocketHandler {
	return &WebSocketHandler{
		wsManager:   wsManager,
		sessionRepo: sessionRepo,
		logger:      logger,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (h *WebSocketHandler) HandleConnection(c *gin.Context) {
	sessionIDStr := c.Param("session_id")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}

	// Get user ID from context
	userID := c.GetInt("user_id")

	// Verify session ownership
	session, err := h.sessionRepo.GetByID(c.Request.Context(), sessionID)
	if err != nil || session.UserID != userID {
		response.Forbidden(c, "Access denied")
		return
	}

	// Upgrade to WebSocket
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("Failed to upgrade to WebSocket: %v", err)
		return
	}

	// Register client with WebSocket manager
	client := h.wsManager.RegisterClient(conn, userID, &sessionID)
	defer h.wsManager.UnregisterClient(client)

	// Start client message pumps
	go client.WritePump()
	client.ReadPump()
}
