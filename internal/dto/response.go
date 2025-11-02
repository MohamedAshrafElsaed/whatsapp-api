package dto

import (
	"time"

	"github.com/google/uuid"
	"whatsapp-api/internal/models"
)

// SessionResponse represents a WhatsApp session response
type SessionResponse struct {
	ID             uuid.UUID            `json:"id"`
	UserID         int                  `json:"user_id"`
	SessionName    string               `json:"session_name"`
	PhoneNumber    *string              `json:"phone_number,omitempty"`
	JID            *string              `json:"jid,omitempty"`
	Status         models.SessionStatus `json:"status"`
	QRGeneratedAt  *time.Time           `json:"qr_generated_at,omitempty"`
	QRExpiresAt    *time.Time           `json:"qr_expires_at,omitempty"`
	QRRetryCount   int                  `json:"qr_retry_count"`
	ConnectedAt    *time.Time           `json:"connected_at,omitempty"`
	DisconnectedAt *time.Time           `json:"disconnected_at,omitempty"`
	LastSeen       *time.Time           `json:"last_seen,omitempty"`
	DeviceInfo     *models.DeviceInfo   `json:"device_info,omitempty"`
	PushName       *string              `json:"push_name,omitempty"`
	Platform       *string              `json:"platform,omitempty"`
	IsActive       bool                 `json:"is_active"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
}

// FromSession converts a session model to response
func (r *SessionResponse) FromSession(session *models.WhatsAppSession) {
	r.ID = session.ID
	r.UserID = session.UserID
	r.SessionName = session.SessionName
	r.PhoneNumber = session.PhoneNumber
	r.JID = session.JID
	r.Status = session.Status
	r.QRGeneratedAt = session.QRGeneratedAt
	r.QRExpiresAt = session.QRExpiresAt
	r.QRRetryCount = session.QRRetryCount
	r.ConnectedAt = session.ConnectedAt
	r.DisconnectedAt = session.DisconnectedAt
	r.LastSeen = session.LastSeen
	r.DeviceInfo = session.DeviceInfo
	r.PushName = session.PushName
	r.Platform = session.Platform
	r.IsActive = session.IsActive
	r.CreatedAt = session.CreatedAt
	r.UpdatedAt = session.UpdatedAt
}

// SessionListResponse represents a list of sessions
type SessionListResponse struct {
	Sessions []*SessionResponse `json:"sessions"`
	Summary  *SessionSummary    `json:"summary"`
	Meta     *PaginationMeta    `json:"meta,omitempty"`
}

// SessionSummary represents session summary information
type SessionSummary struct {
	TotalSessions     int64      `json:"total_sessions"`
	ConnectedCount    int64      `json:"connected_count"`
	PendingCount      int64      `json:"pending_count"`
	DisconnectedCount int64      `json:"disconnected_count"`
	MaxDevices        int        `json:"max_devices"`
	AvailableSlots    int        `json:"available_slots"`
	LastActivity      *time.Time `json:"last_activity,omitempty"`
}

// FromModelSummary converts model summary to response
func (s *SessionSummary) FromModelSummary(model *models.SessionSummary) {
	s.TotalSessions = model.TotalSessions
	s.ConnectedCount = model.ConnectedCount
	s.PendingCount = model.PendingCount
	s.DisconnectedCount = model.DisconnectedCount
	s.MaxDevices = model.MaxDevices
	s.AvailableSlots = model.AvailableSlots
	s.LastActivity = model.LastActivity
}

// QRCodeResponse represents a QR code response
type QRCodeResponse struct {
	QRCode    string    `json:"qr_code"`
	QRData    string    `json:"qr_data"`
	ExpiresAt time.Time `json:"expires_at"`
	Format    string    `json:"format,omitempty"`
}

// SessionStatusResponse represents session status
type SessionStatusResponse struct {
	SessionID      uuid.UUID            `json:"session_id"`
	Status         models.SessionStatus `json:"status"`
	PhoneNumber    *string              `json:"phone_number,omitempty"`
	JID            *string              `json:"jid,omitempty"`
	LastSeen       *time.Time           `json:"last_seen,omitempty"`
	ConnectedAt    *time.Time           `json:"connected_at,omitempty"`
	DisconnectedAt *time.Time           `json:"disconnected_at,omitempty"`
	IsActive       bool                 `json:"is_active"`
}

// DeviceResponse represents a device response
type DeviceResponse struct {
	JID          string    `json:"jid"`
	SessionID    string    `json:"session_id"`
	UserID       int       `json:"user_id"`
	PhoneNumber  string    `json:"phone_number"`
	Platform     string    `json:"platform,omitempty"`
	PushName     string    `json:"push_name,omitempty"`
	BusinessName string    `json:"business_name,omitempty"`
	IsBusiness   bool      `json:"is_business"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// FromDeviceMetadata converts device metadata to response
func (r *DeviceResponse) FromDeviceMetadata(metadata *models.DeviceMetadata) {
	r.JID = metadata.JID
	r.SessionID = metadata.SessionID
	r.UserID = metadata.UserID
	r.PhoneNumber = metadata.PhoneNumber
	r.Platform = metadata.Platform
	r.PushName = metadata.PushName
	r.BusinessName = metadata.BusinessName
	r.IsBusiness = metadata.IsBusiness
	r.CreatedAt = metadata.CreatedAt
	r.UpdatedAt = metadata.UpdatedAt
}

// DeviceSummaryResponse represents device summary
type DeviceSummaryResponse struct {
	UserID           int               `json:"user_id"`
	MaxDevices       int               `json:"max_devices"`
	UsedDevices      int               `json:"used_devices"`
	AvailableSlots   int               `json:"available_slots"`
	ConnectedDevices int               `json:"connected_devices"`
	Devices          []*DeviceResponse `json:"devices"`
}

// EventResponse represents an event response
type EventResponse struct {
	ID        int64            `json:"id"`
	SessionID *string          `json:"session_id,omitempty"`
	UserID    int              `json:"user_id"`
	EventType models.EventType `json:"event_type"`
	EventData models.EventData `json:"event_data,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
}

// FromEvent converts an event model to response
func (r *EventResponse) FromEvent(event *models.WhatsAppEvent) {
	r.ID = event.ID
	r.SessionID = event.SessionID
	r.UserID = event.UserID
	r.EventType = event.EventType
	r.EventData = event.EventData
	r.CreatedAt = event.CreatedAt
}

// EventListResponse represents a list of events
type EventListResponse struct {
	Events []*EventResponse `json:"events"`
	Meta   *PaginationMeta  `json:"meta,omitempty"`
}

// EventStatisticsResponse represents event statistics
type EventStatisticsResponse struct {
	UserID         int                        `json:"user_id"`
	TotalEvents    int64                      `json:"total_events"`
	EventsByType   map[models.EventType]int64 `json:"events_by_type"`
	CriticalEvents int64                      `json:"critical_events"`
	LastEvent      *time.Time                 `json:"last_event,omitempty"`
	FirstEvent     *time.Time                 `json:"first_event,omitempty"`
}

// FromModelStatistics converts model statistics to response
func (r *EventStatisticsResponse) FromModelStatistics(stats *models.EventStatistics) {
	r.UserID = stats.UserID
	r.TotalEvents = stats.TotalEvents
	r.EventsByType = stats.EventsByType
	r.CriticalEvents = stats.CriticalEvents
	r.LastEvent = stats.LastEvent
	r.FirstEvent = stats.FirstEvent
}

// PaginationMeta represents pagination metadata
type PaginationMeta struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalPages int   `json:"total_pages"`
	TotalCount int64 `json:"total_count"`
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status    string                   `json:"status"`
	Timestamp time.Time                `json:"timestamp"`
	Services  map[string]ServiceHealth `json:"services,omitempty"`
	Version   string                   `json:"version,omitempty"`
}

// ServiceHealth represents individual service health
type ServiceHealth struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// WebSocketMessage represents a WebSocket message
type WebSocketMessage struct {
	Type      string      `json:"type"`
	SessionID string      `json:"session_id,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

// WebSocketEventData represents WebSocket event data
type WebSocketEventData struct {
	Event     models.EventType `json:"event"`
	SessionID string           `json:"session_id"`
	Status    string           `json:"status,omitempty"`
	Message   string           `json:"message,omitempty"`
	Data      interface{}      `json:"data,omitempty"`
}

// QRCodeWebSocketData represents QR code WebSocket data
type QRCodeWebSocketData struct {
	QRCode    string    `json:"qr_code"`
	ExpiresAt time.Time `json:"expires_at"`
}

// StatusChangeWebSocketData represents status change WebSocket data
type StatusChangeWebSocketData struct {
	OldStatus models.SessionStatus `json:"old_status"`
	NewStatus models.SessionStatus `json:"new_status"`
	Timestamp time.Time            `json:"timestamp"`
}

// ConnectionWebSocketData represents connection WebSocket data
type ConnectionWebSocketData struct {
	JID         string    `json:"jid"`
	PhoneNumber string    `json:"phone_number"`
	PushName    string    `json:"push_name"`
	ConnectedAt time.Time `json:"connected_at"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Error   string      `json:"error,omitempty"`
	Details interface{} `json:"details,omitempty"`
}

// SuccessResponse represents a success response
type SuccessResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// DeleteResponse represents a delete operation response
type DeleteResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	SessionID string `json:"session_id"`
}

// BulkActionResponse represents a bulk action response
type BulkActionResponse struct {
	Success      bool     `json:"success"`
	Message      string   `json:"message"`
	TotalCount   int      `json:"total_count"`
	SuccessCount int      `json:"success_count"`
	FailedCount  int      `json:"failed_count"`
	Failed       []string `json:"failed,omitempty"`
}

// StatsResponse represents general statistics
type StatsResponse struct {
	TotalUsers        int64   `json:"total_users"`
	TotalSessions     int64   `json:"total_sessions"`
	ConnectedSessions int64   `json:"connected_sessions"`
	TotalDevices      int64   `json:"total_devices"`
	TotalEvents       int64   `json:"total_events"`
	Uptime            string  `json:"uptime"`
	MemoryUsage       string  `json:"memory_usage,omitempty"`
	CPUUsage          float64 `json:"cpu_usage,omitempty"`
}

// ConnectionInfoResponse represents connection information
type ConnectionInfoResponse struct {
	Connected      bool       `json:"connected"`
	Authenticated  bool       `json:"authenticated"`
	JID            *string    `json:"jid,omitempty"`
	PhoneNumber    *string    `json:"phone_number,omitempty"`
	PushName       *string    `json:"push_name,omitempty"`
	LastSeen       *time.Time `json:"last_seen,omitempty"`
	ConnectionTime *time.Time `json:"connection_time,omitempty"`
}

// MessageResponse represents a message response (for future use)
type MessageResponse struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Status    string    `json:"status"`
	From      string    `json:"from"`
	To        string    `json:"to"`
}

// TokenValidationResponse represents token validation result
type TokenValidationResponse struct {
	Valid  bool   `json:"valid"`
	UserID int    `json:"user_id,omitempty"`
	Email  string `json:"email,omitempty"`
	Error  string `json:"error,omitempty"`
}

// APIInfoResponse represents API information
type APIInfoResponse struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Environment string   `json:"environment"`
	Endpoints   []string `json:"endpoints,omitempty"`
}

// Helper functions to create responses

// NewSuccessResponse creates a new success response
func NewSuccessResponse(message string, data interface{}) *SuccessResponse {
	return &SuccessResponse{
		Success: true,
		Message: message,
		Data:    data,
	}
}

// NewErrorResponse creates a new error response
func NewErrorResponse(message string, err error, details interface{}) *ErrorResponse {
	resp := &ErrorResponse{
		Success: false,
		Message: message,
		Details: details,
	}

	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}

// NewPaginationMeta creates pagination metadata
func NewPaginationMeta(page, pageSize int, totalCount int64) *PaginationMeta {
	totalPages := int(totalCount) / pageSize
	if int(totalCount)%pageSize > 0 {
		totalPages++
	}

	return &PaginationMeta{
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
		TotalCount: totalCount,
	}
}

// NewWebSocketMessage creates a new WebSocket message
func NewWebSocketMessage(msgType string, sessionID string, data interface{}) *WebSocketMessage {
	return &WebSocketMessage{
		Type:      msgType,
		SessionID: sessionID,
		Data:      data,
		Timestamp: time.Now(),
	}
}

// NewHealthResponse creates a new health response
func NewHealthResponse(status string, services map[string]ServiceHealth) *HealthResponse {
	return &HealthResponse{
		Status:    status,
		Timestamp: time.Now(),
		Services:  services,
		Version:   "1.0.0",
	}
}

// SessionsFromModels converts multiple session models to responses
func SessionsFromModels(sessions []*models.WhatsAppSession) []*SessionResponse {
	responses := make([]*SessionResponse, len(sessions))
	for i, session := range sessions {
		resp := &SessionResponse{}
		resp.FromSession(session)
		responses[i] = resp
	}
	return responses
}

// DevicesFromMetadata converts multiple device metadata to responses
func DevicesFromMetadata(metadata []*models.DeviceMetadata) []*DeviceResponse {
	responses := make([]*DeviceResponse, len(metadata))
	for i, meta := range metadata {
		resp := &DeviceResponse{}
		resp.FromDeviceMetadata(meta)
		responses[i] = resp
	}
	return responses
}

// EventsFromModels converts multiple event models to responses
func EventsFromModels(events []*models.WhatsAppEvent) []*EventResponse {
	responses := make([]*EventResponse, len(events))
	for i, event := range events {
		resp := &EventResponse{}
		resp.FromEvent(event)
		responses[i] = resp
	}
	return responses
}
