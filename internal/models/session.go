package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SessionStatus represents the status of a WhatsApp session
type SessionStatus string

const (
	SessionStatusPending      SessionStatus = "pending"
	SessionStatusQRReady      SessionStatus = "qr_ready"
	SessionStatusScanning     SessionStatus = "scanning"
	SessionStatusConnected    SessionStatus = "connected"
	SessionStatusDisconnected SessionStatus = "disconnected"
	SessionStatusFailed       SessionStatus = "failed"
	SessionStatusExpired      SessionStatus = "expired"
)

// IsValid checks if the session status is valid
func (s SessionStatus) IsValid() bool {
	switch s {
	case SessionStatusPending, SessionStatusQRReady, SessionStatusScanning,
		SessionStatusConnected, SessionStatusDisconnected, SessionStatusFailed,
		SessionStatusExpired:
		return true
	default:
		return false
	}
}

// IsActive checks if the session is in an active state
func (s SessionStatus) IsActive() bool {
	return s == SessionStatusConnected || s == SessionStatusPending ||
		s == SessionStatusQRReady || s == SessionStatusScanning
}

// DeviceInfo stores additional device information
type DeviceInfo struct {
	Platform      string `json:"platform,omitempty"`
	AppVersion    string `json:"app_version,omitempty"`
	OSVersion     string `json:"os_version,omitempty"`
	DeviceModel   string `json:"device_model,omitempty"`
	Manufacturer  string `json:"manufacturer,omitempty"`
	WAVersion     string `json:"wa_version,omitempty"`
	MCC           string `json:"mcc,omitempty"`
	MNC           string `json:"mnc,omitempty"`
	OsName        string `json:"os_name,omitempty"`
	OsBuildNumber string `json:"os_build_number,omitempty"`
}

// Scan implements sql.Scanner interface for DeviceInfo
func (d *DeviceInfo) Scan(value interface{}) error {
	if value == nil {
		*d = DeviceInfo{}
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return gorm.ErrInvalidData
	}

	return json.Unmarshal(bytes, d)
}

// Value implements driver.Valuer interface for DeviceInfo
func (d DeviceInfo) Value() (driver.Value, error) {
	if d == (DeviceInfo{}) {
		return nil, nil
	}
	return json.Marshal(d)
}

// WhatsAppSession represents a WhatsApp session in the database
type WhatsAppSession struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID         int            `gorm:"not null;index:idx_sessions_user_id" json:"user_id"`
	SessionName    string         `gorm:"type:varchar(255);not null" json:"session_name"`
	PhoneNumber    *string        `gorm:"type:varchar(20)" json:"phone_number,omitempty"`
	JID            *string        `gorm:"type:varchar(255);uniqueIndex" json:"jid,omitempty"`
	Status         SessionStatus  `gorm:"type:varchar(50);not null;default:'pending';index:idx_sessions_status" json:"status"`
	QRCode         *string        `gorm:"type:text" json:"qr_code,omitempty"`
	QRCodeBase64   *string        `gorm:"type:text" json:"qr_code_base64,omitempty"`
	QRGeneratedAt  *time.Time     `json:"qr_generated_at,omitempty"`
	QRExpiresAt    *time.Time     `json:"qr_expires_at,omitempty"`
	QRRetryCount   int            `gorm:"default:0" json:"qr_retry_count"`
	ConnectedAt    *time.Time     `json:"connected_at,omitempty"`
	DisconnectedAt *time.Time     `json:"disconnected_at,omitempty"`
	LastSeen       *time.Time     `json:"last_seen,omitempty"`
	DeviceInfo     *DeviceInfo    `gorm:"type:jsonb" json:"device_info,omitempty"`
	PushName       *string        `gorm:"type:varchar(255)" json:"push_name,omitempty"`
	Platform       *string        `gorm:"type:varchar(50)" json:"platform,omitempty"`
	IsActive       bool           `gorm:"default:true;index:idx_sessions_active" json:"is_active"`
	CreatedAt      time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	// Relationships
	Device *WhatsAppDevice `gorm:"foreignKey:SessionID;references:ID" json:"device,omitempty"`
	Events []WhatsAppEvent `gorm:"foreignKey:SessionID;references:ID" json:"events,omitempty"`
}

// TableName specifies the table name for WhatsAppSession
func (WhatsAppSession) TableName() string {
	return "whatsapp_sessions"
}

// BeforeCreate hook to generate UUID if not set
func (s *WhatsAppSession) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

// IsQRExpired checks if the QR code has expired
func (s *WhatsAppSession) IsQRExpired() bool {
	if s.QRExpiresAt == nil {
		return true
	}
	return time.Now().After(*s.QRExpiresAt)
}

// CanGenerateNewQR checks if a new QR code can be generated
func (s *WhatsAppSession) CanGenerateNewQR(maxRetries int) bool {
	return s.QRRetryCount < maxRetries
}

// UpdateLastSeen updates the last seen timestamp
func (s *WhatsAppSession) UpdateLastSeen() {
	now := time.Now()
	s.LastSeen = &now
}

// SetConnected marks the session as connected
func (s *WhatsAppSession) SetConnected(jid, phoneNumber, pushName string, deviceInfo *DeviceInfo) {
	now := time.Now()
	s.Status = SessionStatusConnected
	s.JID = &jid
	s.PhoneNumber = &phoneNumber
	s.PushName = &pushName
	s.ConnectedAt = &now
	s.LastSeen = &now
	s.DeviceInfo = deviceInfo
	s.DisconnectedAt = nil
	s.QRCode = nil
	s.QRCodeBase64 = nil
	s.QRGeneratedAt = nil
	s.QRExpiresAt = nil
	s.QRRetryCount = 0
}

// SetDisconnected marks the session as disconnected
func (s *WhatsAppSession) SetDisconnected() {
	now := time.Now()
	s.Status = SessionStatusDisconnected
	s.DisconnectedAt = &now
}

// SetQRCode sets the QR code data
func (s *WhatsAppSession) SetQRCode(qrCode, qrCodeBase64 string, timeout time.Duration) {
	now := time.Now()
	expiresAt := now.Add(timeout)

	s.QRCode = &qrCode
	s.QRCodeBase64 = &qrCodeBase64
	s.QRGeneratedAt = &now
	s.QRExpiresAt = &expiresAt
	s.Status = SessionStatusQRReady
	s.QRRetryCount++
}

// ClearQRCode clears the QR code data
func (s *WhatsAppSession) ClearQRCode() {
	s.QRCode = nil
	s.QRCodeBase64 = nil
	s.QRGeneratedAt = nil
	s.QRExpiresAt = nil
}

// SetScanning marks the session as scanning
func (s *WhatsAppSession) SetScanning() {
	s.Status = SessionStatusScanning
}

// SetFailed marks the session as failed
func (s *WhatsAppSession) SetFailed() {
	s.Status = SessionStatusFailed
}

// SetExpired marks the session as expired
func (s *WhatsAppSession) SetExpired() {
	s.Status = SessionStatusExpired
}

// Deactivate deactivates the session
func (s *WhatsAppSession) Deactivate() {
	s.IsActive = false
	s.SetDisconnected()
}

// GetConnectionDuration returns the connection duration if connected
func (s *WhatsAppSession) GetConnectionDuration() *time.Duration {
	if s.ConnectedAt == nil {
		return nil
	}

	var end time.Time
	if s.DisconnectedAt != nil {
		end = *s.DisconnectedAt
	} else {
		end = time.Now()
	}

	duration := end.Sub(*s.ConnectedAt)
	return &duration
}

// IsInactive checks if the session has been inactive for the given duration
func (s *WhatsAppSession) IsInactive(duration time.Duration) bool {
	if s.LastSeen == nil {
		return false
	}
	return time.Since(*s.LastSeen) > duration
}

// SessionSummary represents a summary of sessions for a user
type SessionSummary struct {
	UserID            int        `json:"user_id"`
	TotalSessions     int64      `json:"total_sessions"`
	ConnectedCount    int64      `json:"connected_count"`
	PendingCount      int64      `json:"pending_count"`
	DisconnectedCount int64      `json:"disconnected_count"`
	MaxDevices        int        `json:"max_devices"`
	AvailableSlots    int        `json:"available_slots"`
	LastActivity      *time.Time `json:"last_activity,omitempty"`
}

// CanAddNewDevice checks if the user can add a new device
func (s *SessionSummary) CanAddNewDevice() bool {
	return s.AvailableSlots > 0
}

// GetUsedSlots returns the number of used device slots
func (s *SessionSummary) GetUsedSlots() int {
	return int(s.ConnectedCount + s.PendingCount)
}
