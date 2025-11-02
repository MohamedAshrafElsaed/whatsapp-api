package dto

import (
	"time"

	"whatsapp-api/internal/models"
)

// CreateSessionRequest represents a request to create a new WhatsApp session
type CreateSessionRequest struct {
	SessionName string `json:"session_name" binding:"required,min=1,max=255" example:"Personal Phone"`
}

// Validate validates the create session request
func (r *CreateSessionRequest) Validate() error {
	if r.SessionName == "" {
		return ErrSessionNameRequired
	}

	if len(r.SessionName) > 255 {
		return ErrSessionNameTooLong
	}

	return nil
}

// UpdateSessionRequest represents a request to update a session
type UpdateSessionRequest struct {
	SessionName *string `json:"session_name,omitempty" binding:"omitempty,min=1,max=255" example:"Updated Name"`
	IsActive    *bool   `json:"is_active,omitempty" example:"true"`
}

// Validate validates the update session request
func (r *UpdateSessionRequest) Validate() error {
	if r.SessionName != nil && *r.SessionName == "" {
		return ErrSessionNameRequired
	}

	if r.SessionName != nil && len(*r.SessionName) > 255 {
		return ErrSessionNameTooLong
	}

	return nil
}

// GetQRCodeRequest represents a request to get QR code
type GetQRCodeRequest struct {
	Format string `form:"format" binding:"omitempty,oneof=json png base64" example:"json"`
}

// Validate validates the get QR code request
func (r *GetQRCodeRequest) Validate() error {
	if r.Format != "" && r.Format != "json" && r.Format != "png" && r.Format != "base64" {
		return ErrInvalidQRFormat
	}

	return nil
}

// GetSessionsRequest represents a request to get sessions with filters
type GetSessionsRequest struct {
	Status   string `form:"status" binding:"omitempty" example:"connected"`
	IsActive *bool  `form:"is_active" example:"true"`
	Limit    int    `form:"limit" binding:"omitempty,min=1,max=100" example:"10"`
	Offset   int    `form:"offset" binding:"omitempty,min=0" example:"0"`
	SortBy   string `form:"sort_by" binding:"omitempty,oneof=created_at updated_at last_seen" example:"created_at"`
	SortDir  string `form:"sort_dir" binding:"omitempty,oneof=asc desc" example:"desc"`
}

// Validate validates the get sessions request
func (r *GetSessionsRequest) Validate() error {
	if r.Status != "" {
		status := models.SessionStatus(r.Status)
		if !status.IsValid() {
			return ErrInvalidSessionStatus
		}
	}

	if r.Limit < 0 || r.Limit > 100 {
		r.Limit = 10
	}

	if r.Offset < 0 {
		r.Offset = 0
	}

	if r.SortBy == "" {
		r.SortBy = "created_at"
	}

	if r.SortDir == "" {
		r.SortDir = "desc"
	}

	return nil
}

// GetEventsRequest represents a request to get events with filters
type GetEventsRequest struct {
	SessionID  string   `form:"session_id" example:"uuid"`
	EventTypes []string `form:"event_types" example:"connected,disconnected"`
	StartDate  string   `form:"start_date" binding:"omitempty" example:"2024-01-01T00:00:00Z"`
	EndDate    string   `form:"end_date" binding:"omitempty" example:"2024-12-31T23:59:59Z"`
	Limit      int      `form:"limit" binding:"omitempty,min=1,max=100" example:"50"`
	Offset     int      `form:"offset" binding:"omitempty,min=0" example:"0"`
}

// Validate validates the get events request
func (r *GetEventsRequest) Validate() error {
	if r.Limit <= 0 || r.Limit > 100 {
		r.Limit = 50
	}

	if r.Offset < 0 {
		r.Offset = 0
	}

	return nil
}

// GetEventsFilter converts the request to an event filter
func (r *GetEventsRequest) GetEventsFilter(userID int) (*models.EventFilter, error) {
	filter := &models.EventFilter{
		UserID: &userID,
		Limit:  r.Limit,
		Offset: r.Offset,
	}

	if r.SessionID != "" {
		filter.SessionID = &r.SessionID
	}

	if len(r.EventTypes) > 0 {
		eventTypes := make([]models.EventType, 0, len(r.EventTypes))
		for _, et := range r.EventTypes {
			eventType := models.EventType(et)
			if eventType.IsValid() {
				eventTypes = append(eventTypes, eventType)
			}
		}
		filter.EventTypes = eventTypes
	}

	if r.StartDate != "" {
		startDate, err := time.Parse(time.RFC3339, r.StartDate)
		if err != nil {
			return nil, ErrInvalidDateFormat
		}
		filter.StartDate = &startDate
	}

	if r.EndDate != "" {
		endDate, err := time.Parse(time.RFC3339, r.EndDate)
		if err != nil {
			return nil, ErrInvalidDateFormat
		}
		filter.EndDate = &endDate
	}

	return filter, nil
}

// WebSocketConnectRequest represents WebSocket connection parameters
type WebSocketConnectRequest struct {
	Token string `form:"token" binding:"required" example:"jwt_token"`
}

// Validate validates the WebSocket connect request
func (r *WebSocketConnectRequest) Validate() error {
	if r.Token == "" {
		return ErrTokenRequired
	}

	return nil
}

// PaginationRequest represents common pagination parameters
type PaginationRequest struct {
	Page     int `form:"page" binding:"omitempty,min=1" example:"1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100" example:"10"`
}

// Validate validates pagination parameters
func (r *PaginationRequest) Validate() error {
	if r.Page < 1 {
		r.Page = 1
	}

	if r.PageSize < 1 || r.PageSize > 100 {
		r.PageSize = 10
	}

	return nil
}

// GetLimit returns the limit for the query
func (r *PaginationRequest) GetLimit() int {
	return r.PageSize
}

// GetOffset returns the offset for the query
func (r *PaginationRequest) GetOffset() int {
	return (r.Page - 1) * r.PageSize
}

// SearchRequest represents common search parameters
type SearchRequest struct {
	Query string `form:"q" example:"search term"`
	PaginationRequest
}

// Validate validates search parameters
func (r *SearchRequest) Validate() error {
	return r.PaginationRequest.Validate()
}

// DateRangeRequest represents date range filter
type DateRangeRequest struct {
	StartDate string `form:"start_date" binding:"omitempty" example:"2024-01-01"`
	EndDate   string `form:"end_date" binding:"omitempty" example:"2024-12-31"`
}

// Validate validates date range
func (r *DateRangeRequest) Validate() error {
	if r.StartDate != "" {
		if _, err := time.Parse("2006-01-02", r.StartDate); err != nil {
			return ErrInvalidDateFormat
		}
	}

	if r.EndDate != "" {
		if _, err := time.Parse("2006-01-02", r.EndDate); err != nil {
			return ErrInvalidDateFormat
		}
	}

	return nil
}

// GetStartDate returns parsed start date
func (r *DateRangeRequest) GetStartDate() (*time.Time, error) {
	if r.StartDate == "" {
		return nil, nil
	}

	date, err := time.Parse("2006-01-02", r.StartDate)
	if err != nil {
		return nil, err
	}

	return &date, nil
}

// GetEndDate returns parsed end date
func (r *DateRangeRequest) GetEndDate() (*time.Time, error) {
	if r.EndDate == "" {
		return nil, nil
	}

	date, err := time.Parse("2006-01-02", r.EndDate)
	if err != nil {
		return nil, err
	}

	// Set to end of day
	endOfDay := date.Add(24*time.Hour - time.Second)
	return &endOfDay, nil
}

// SendMessageRequest represents a request to send a message (for future use)
type SendMessageRequest struct {
	To      string `json:"to" binding:"required" example:"+1234567890"`
	Message string `json:"message" binding:"required" example:"Hello, World!"`
}

// Validate validates send message request
func (r *SendMessageRequest) Validate() error {
	if r.To == "" {
		return ErrRecipientRequired
	}

	if r.Message == "" {
		return ErrMessageRequired
	}

	return nil
}

// BulkActionRequest represents a bulk action request
type BulkActionRequest struct {
	SessionIDs []string `json:"session_ids" binding:"required,min=1" example:"[\"uuid1\",\"uuid2\"]"`
	Action     string   `json:"action" binding:"required,oneof=disconnect delete" example:"disconnect"`
}

// Validate validates bulk action request
func (r *BulkActionRequest) Validate() error {
	if len(r.SessionIDs) == 0 {
		return ErrSessionIDsRequired
	}

	if r.Action != "disconnect" && r.Action != "delete" {
		return ErrInvalidAction
	}

	return nil
}

// HealthCheckRequest represents a health check request
type HealthCheckRequest struct {
	Detailed bool `form:"detailed" example:"false"`
}

// Validation errors
var (
	ErrSessionNameRequired  = &ValidationError{Field: "session_name", Message: "session name is required"}
	ErrSessionNameTooLong   = &ValidationError{Field: "session_name", Message: "session name is too long (max 255 characters)"}
	ErrInvalidQRFormat      = &ValidationError{Field: "format", Message: "invalid QR code format (must be json, png, or base64)"}
	ErrInvalidSessionStatus = &ValidationError{Field: "status", Message: "invalid session status"}
	ErrInvalidDateFormat    = &ValidationError{Field: "date", Message: "invalid date format (use RFC3339 or YYYY-MM-DD)"}
	ErrTokenRequired        = &ValidationError{Field: "token", Message: "token is required"}
	ErrRecipientRequired    = &ValidationError{Field: "to", Message: "recipient is required"}
	ErrMessageRequired      = &ValidationError{Field: "message", Message: "message is required"}
	ErrSessionIDsRequired   = &ValidationError{Field: "session_ids", Message: "session IDs are required"}
	ErrInvalidAction        = &ValidationError{Field: "action", Message: "invalid action"}
)

// ValidationError represents a validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Error implements the error interface
func (e *ValidationError) Error() string {
	return e.Message
}

// ValidationErrors represents multiple validation errors
type ValidationErrors struct {
	Errors []*ValidationError `json:"errors"`
}

// Error implements the error interface
func (e *ValidationErrors) Error() string {
	if len(e.Errors) == 0 {
		return "validation failed"
	}
	return e.Errors[0].Message
}

// Add adds a validation error
func (e *ValidationErrors) Add(field, message string) {
	e.Errors = append(e.Errors, &ValidationError{
		Field:   field,
		Message: message,
	})
}

// HasErrors returns true if there are validation errors
func (e *ValidationErrors) HasErrors() bool {
	return len(e.Errors) > 0
}
