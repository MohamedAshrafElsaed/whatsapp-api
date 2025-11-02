package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// EventType represents the type of WhatsApp event
type EventType string

const (
	// Connection Events
	EventTypeConnected    EventType = "connected"
	EventTypeDisconnected EventType = "disconnected"
	EventTypeReconnecting EventType = "reconnecting"
	EventTypeLoggedOut    EventType = "logged_out"

	// QR Events
	EventTypeQRGenerated EventType = "qr_generated"
	EventTypeQRScanned   EventType = "qr_scanned"
	EventTypeQRExpired   EventType = "qr_expired"

	// Session Events
	EventTypeSessionCreated EventType = "session_created"
	EventTypeSessionDeleted EventType = "session_deleted"
	EventTypeSessionUpdated EventType = "session_updated"

	// Authentication Events
	EventTypePairSuccess EventType = "pair_success"
	EventTypePairError   EventType = "pair_error"

	// Error Events
	EventTypeError       EventType = "error"
	EventTypeStreamError EventType = "stream_error"
	EventTypeClientError EventType = "client_error"

	// Status Events
	EventTypeStatusChange EventType = "status_change"
	EventTypePresence     EventType = "presence"

	// Message Events (for future use)
	EventTypeMessageReceived EventType = "message_received"
	EventTypeMessageSent     EventType = "message_sent"
	EventTypeMessageError    EventType = "message_error"

	// Device Events
	EventTypeDeviceAdded   EventType = "device_added"
	EventTypeDeviceRemoved EventType = "device_removed"

	// App State Events
	EventTypeAppStateSync EventType = "app_state_sync"

	// Keep Alive Events
	EventTypeKeepAlive EventType = "keep_alive"
	EventTypePingPong  EventType = "ping_pong"
)

// IsValid checks if the event type is valid
func (e EventType) IsValid() bool {
	switch e {
	case EventTypeConnected, EventTypeDisconnected, EventTypeReconnecting, EventTypeLoggedOut,
		EventTypeQRGenerated, EventTypeQRScanned, EventTypeQRExpired,
		EventTypeSessionCreated, EventTypeSessionDeleted, EventTypeSessionUpdated,
		EventTypePairSuccess, EventTypePairError,
		EventTypeError, EventTypeStreamError, EventTypeClientError,
		EventTypeStatusChange, EventTypePresence,
		EventTypeMessageReceived, EventTypeMessageSent, EventTypeMessageError,
		EventTypeDeviceAdded, EventTypeDeviceRemoved,
		EventTypeAppStateSync, EventTypeKeepAlive, EventTypePingPong:
		return true
	default:
		return false
	}
}

// IsCritical checks if the event is critical
func (e EventType) IsCritical() bool {
	return e == EventTypeError || e == EventTypeStreamError ||
		e == EventTypeClientError || e == EventTypePairError ||
		e == EventTypeLoggedOut
}

// EventData stores the event payload
type EventData map[string]interface{}

// Scan implements sql.Scanner interface for EventData
func (e *EventData) Scan(value interface{}) error {
	if value == nil {
		*e = make(EventData)
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return gorm.ErrInvalidData
	}

	return json.Unmarshal(bytes, e)
}

// Value implements driver.Valuer interface for EventData
func (e EventData) Value() (driver.Value, error) {
	if e == nil || len(e) == 0 {
		return nil, nil
	}
	return json.Marshal(e)
}

// Get retrieves a value from EventData
func (e EventData) Get(key string) interface{} {
	if e == nil {
		return nil
	}
	return e[key]
}

// GetString retrieves a string value from EventData
func (e EventData) GetString(key string) string {
	if v, ok := e[key].(string); ok {
		return v
	}
	return ""
}

// GetInt retrieves an int value from EventData
func (e EventData) GetInt(key string) int {
	if v, ok := e[key].(float64); ok {
		return int(v)
	}
	return 0
}

// GetBool retrieves a bool value from EventData
func (e EventData) GetBool(key string) bool {
	if v, ok := e[key].(bool); ok {
		return v
	}
	return false
}

// Set sets a value in EventData
func (e EventData) Set(key string, value interface{}) {
	if e == nil {
		e = make(EventData)
	}
	e[key] = value
}

// WhatsAppEvent represents an event log in the database
type WhatsAppEvent struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID *string   `gorm:"type:uuid;index:idx_events_session_id" json:"session_id,omitempty"`
	UserID    int       `gorm:"not null;index:idx_events_user_id" json:"user_id"`
	EventType EventType `gorm:"type:varchar(100);not null;index:idx_events_type" json:"event_type"`
	EventData EventData `gorm:"type:jsonb" json:"event_data,omitempty"`
	IPAddress *string   `gorm:"type:inet" json:"ip_address,omitempty"`
	UserAgent *string   `gorm:"type:text" json:"user_agent,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime;index:idx_events_created_at" json:"created_at"`

	// Relationships
	Session *WhatsAppSession `gorm:"foreignKey:SessionID;references:ID" json:"session,omitempty"`
}

// TableName specifies the table name for WhatsAppEvent
func (WhatsAppEvent) TableName() string {
	return "whatsapp_events"
}

// BeforeCreate hook for WhatsAppEvent
func (e *WhatsAppEvent) BeforeCreate(tx *gorm.DB) error {
	if !e.EventType.IsValid() {
		return gorm.ErrInvalidData
	}
	return nil
}

// IsCritical checks if the event is critical
func (e *WhatsAppEvent) IsCritical() bool {
	return e.EventType.IsCritical()
}

// GetAge returns the age of the event
func (e *WhatsAppEvent) GetAge() time.Duration {
	return time.Since(e.CreatedAt)
}

// IsRecent checks if the event occurred within the given duration
func (e *WhatsAppEvent) IsRecent(duration time.Duration) bool {
	return e.GetAge() < duration
}

// EventBuilder helps build event data
type EventBuilder struct {
	sessionID *string
	userID    int
	eventType EventType
	data      EventData
	ipAddress *string
	userAgent *string
}

// NewEventBuilder creates a new EventBuilder
func NewEventBuilder(userID int, eventType EventType) *EventBuilder {
	return &EventBuilder{
		userID:    userID,
		eventType: eventType,
		data:      make(EventData),
	}
}

// WithSessionID sets the session ID
func (b *EventBuilder) WithSessionID(sessionID string) *EventBuilder {
	b.sessionID = &sessionID
	return b
}

// WithData sets the event data
func (b *EventBuilder) WithData(data EventData) *EventBuilder {
	b.data = data
	return b
}

// WithDataField adds a single field to event data
func (b *EventBuilder) WithDataField(key string, value interface{}) *EventBuilder {
	if b.data == nil {
		b.data = make(EventData)
	}
	b.data[key] = value
	return b
}

// WithIPAddress sets the IP address
func (b *EventBuilder) WithIPAddress(ip string) *EventBuilder {
	b.ipAddress = &ip
	return b
}

// WithUserAgent sets the user agent
func (b *EventBuilder) WithUserAgent(ua string) *EventBuilder {
	b.userAgent = &ua
	return b
}

// Build creates the WhatsAppEvent
func (b *EventBuilder) Build() *WhatsAppEvent {
	return &WhatsAppEvent{
		SessionID: b.sessionID,
		UserID:    b.userID,
		EventType: b.eventType,
		EventData: b.data,
		IPAddress: b.ipAddress,
		UserAgent: b.userAgent,
	}
}

// EventFilter represents filters for querying events
type EventFilter struct {
	UserID     *int
	SessionID  *string
	EventTypes []EventType
	StartDate  *time.Time
	EndDate    *time.Time
	Limit      int
	Offset     int
}

// EventStatistics represents statistics about events
type EventStatistics struct {
	UserID         int                 `json:"user_id"`
	TotalEvents    int64               `json:"total_events"`
	EventsByType   map[EventType]int64 `json:"events_by_type"`
	CriticalEvents int64               `json:"critical_events"`
	LastEvent      *time.Time          `json:"last_event,omitempty"`
	FirstEvent     *time.Time          `json:"first_event,omitempty"`
}

// ConnectionEvent represents a specific connection event data
type ConnectionEvent struct {
	SessionID   string    `json:"session_id"`
	JID         string    `json:"jid"`
	PhoneNumber string    `json:"phone_number"`
	PushName    string    `json:"push_name"`
	Platform    string    `json:"platform"`
	ConnectedAt time.Time `json:"connected_at"`
	Reason      string    `json:"reason,omitempty"`
	WasExpected bool      `json:"was_expected,omitempty"`
}

// ToEventData converts ConnectionEvent to EventData
func (c *ConnectionEvent) ToEventData() EventData {
	data := make(EventData)
	data["session_id"] = c.SessionID
	data["jid"] = c.JID
	data["phone_number"] = c.PhoneNumber
	data["push_name"] = c.PushName
	data["platform"] = c.Platform
	data["connected_at"] = c.ConnectedAt
	if c.Reason != "" {
		data["reason"] = c.Reason
	}
	data["was_expected"] = c.WasExpected
	return data
}

// QREvent represents a QR code event data
type QREvent struct {
	SessionID  string    `json:"session_id"`
	QRCode     string    `json:"qr_code"`
	ExpiresAt  time.Time `json:"expires_at"`
	RetryCount int       `json:"retry_count"`
}

// ToEventData converts QREvent to EventData
func (q *QREvent) ToEventData() EventData {
	data := make(EventData)
	data["session_id"] = q.SessionID
	data["qr_code"] = q.QRCode
	data["expires_at"] = q.ExpiresAt
	data["retry_count"] = q.RetryCount
	return data
}

// ErrorEvent represents an error event data
type ErrorEvent struct {
	SessionID string `json:"session_id,omitempty"`
	Error     string `json:"error"`
	ErrorCode string `json:"error_code,omitempty"`
	Stack     string `json:"stack,omitempty"`
	Context   string `json:"context,omitempty"`
}

// ToEventData converts ErrorEvent to EventData
func (e *ErrorEvent) ToEventData() EventData {
	data := make(EventData)
	if e.SessionID != "" {
		data["session_id"] = e.SessionID
	}
	data["error"] = e.Error
	if e.ErrorCode != "" {
		data["error_code"] = e.ErrorCode
	}
	if e.Stack != "" {
		data["stack"] = e.Stack
	}
	if e.Context != "" {
		data["context"] = e.Context
	}
	return data
}
