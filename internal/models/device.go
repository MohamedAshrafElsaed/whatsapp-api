package models

import (
	"time"

	"gorm.io/gorm"
)

// WhatsAppDevice represents the device store for whatsmeow persistence
type WhatsAppDevice struct {
	JID                     string    `gorm:"type:varchar(255);primaryKey" json:"jid"`
	SessionID               *string   `gorm:"type:uuid;index:idx_devices_session_id" json:"session_id,omitempty"`
	UserID                  int       `gorm:"not null;index:idx_devices_user_id" json:"user_id"`
	RegistrationID          *uint32   `json:"registration_id,omitempty"`
	AdvSecretKey            []byte    `gorm:"type:bytea" json:"-"`
	NextPreKeyID            *uint32   `json:"next_pre_key_id,omitempty"`
	FirstUnuploadedPreKeyID *uint32   `json:"first_unuploaded_pre_key_id,omitempty"`
	AccountSignatureKey     []byte    `gorm:"type:bytea" json:"-"`
	AccountSignature        []byte    `gorm:"type:bytea" json:"-"`
	DeviceSignatureKey      []byte    `gorm:"type:bytea" json:"-"`
	DeviceSignature         []byte    `gorm:"type:bytea" json:"-"`
	IdentityKey             []byte    `gorm:"type:bytea" json:"-"`
	NoiseKey                []byte    `gorm:"type:bytea" json:"-"`
	Platform                *string   `gorm:"type:varchar(50)" json:"platform,omitempty"`
	BusinessName            *string   `gorm:"type:varchar(255)" json:"business_name,omitempty"`
	PushName                *string   `gorm:"type:varchar(255)" json:"push_name,omitempty"`
	CreatedAt               time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt               time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Relationships
	Session *WhatsAppSession `gorm:"foreignKey:SessionID;references:ID" json:"session,omitempty"`
}

// TableName specifies the table name for WhatsAppDevice
func (WhatsAppDevice) TableName() string {
	return "whatsapp_devices"
}

// BeforeCreate hook for WhatsAppDevice
func (d *WhatsAppDevice) BeforeCreate(tx *gorm.DB) error {
	// Validation can be added here if needed
	return nil
}

// BeforeUpdate hook for WhatsAppDevice
func (d *WhatsAppDevice) BeforeUpdate(tx *gorm.DB) error {
	// Validation can be added here if needed
	return nil
}

// IsComplete checks if the device has all necessary keys
func (d *WhatsAppDevice) IsComplete() bool {
	return d.IdentityKey != nil &&
		d.NoiseKey != nil &&
		d.AdvSecretKey != nil &&
		d.AccountSignatureKey != nil &&
		d.DeviceSignatureKey != nil
}

// HasPreKeys checks if the device has pre-key information
func (d *WhatsAppDevice) HasPreKeys() bool {
	return d.NextPreKeyID != nil && d.FirstUnuploadedPreKeyID != nil
}

// NeedsPreKeyUpload checks if pre-keys need to be uploaded
func (d *WhatsAppDevice) NeedsPreKeyUpload() bool {
	if !d.HasPreKeys() {
		return false
	}

	// If first unuploaded is greater than next, we need to upload
	return *d.FirstUnuploadedPreKeyID > *d.NextPreKeyID
}

// GetPhoneNumber extracts phone number from JID
func (d *WhatsAppDevice) GetPhoneNumber() string {
	// JID format is typically: phoneNumber@s.whatsapp.net
	if len(d.JID) > 0 {
		for i, c := range d.JID {
			if c == '@' {
				return d.JID[:i]
			}
		}
	}
	return d.JID
}

// IsBusinessAccount checks if this is a business account
func (d *WhatsAppDevice) IsBusinessAccount() bool {
	return d.BusinessName != nil && *d.BusinessName != ""
}

// DeviceIdentity represents the device identity for security purposes
type DeviceIdentity struct {
	JID               string    `json:"jid"`
	Platform          string    `json:"platform"`
	PushName          string    `json:"push_name"`
	BusinessName      string    `json:"business_name,omitempty"`
	RegistrationID    uint32    `json:"registration_id"`
	IsComplete        bool      `json:"is_complete"`
	HasPreKeys        bool      `json:"has_pre_keys"`
	NeedsPreKeyUpload bool      `json:"needs_pre_key_upload"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// ToDeviceIdentity converts WhatsAppDevice to DeviceIdentity (safe for public exposure)
func (d *WhatsAppDevice) ToDeviceIdentity() *DeviceIdentity {
	identity := &DeviceIdentity{
		JID:               d.JID,
		IsComplete:        d.IsComplete(),
		HasPreKeys:        d.HasPreKeys(),
		NeedsPreKeyUpload: d.NeedsPreKeyUpload(),
		CreatedAt:         d.CreatedAt,
		UpdatedAt:         d.UpdatedAt,
	}

	if d.Platform != nil {
		identity.Platform = *d.Platform
	}

	if d.PushName != nil {
		identity.PushName = *d.PushName
	}

	if d.BusinessName != nil {
		identity.BusinessName = *d.BusinessName
	}

	if d.RegistrationID != nil {
		identity.RegistrationID = *d.RegistrationID
	}

	return identity
}

// DeviceMetadata represents metadata about a device
type DeviceMetadata struct {
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

// ToDeviceMetadata converts WhatsAppDevice to DeviceMetadata
func (d *WhatsAppDevice) ToDeviceMetadata() *DeviceMetadata {
	metadata := &DeviceMetadata{
		JID:         d.JID,
		UserID:      d.UserID,
		PhoneNumber: d.GetPhoneNumber(),
		IsBusiness:  d.IsBusinessAccount(),
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	}

	if d.SessionID != nil {
		metadata.SessionID = *d.SessionID
	}

	if d.Platform != nil {
		metadata.Platform = *d.Platform
	}

	if d.PushName != nil {
		metadata.PushName = *d.PushName
	}

	if d.BusinessName != nil {
		metadata.BusinessName = *d.BusinessName
	}

	return metadata
}

// UpdateFromWhatsmeow updates device data from whatsmeow client
type UpdateFromWhatsmeow struct {
	RegistrationID          *uint32
	AdvSecretKey            []byte
	NextPreKeyID            *uint32
	FirstUnuploadedPreKeyID *uint32
	AccountSignatureKey     []byte
	AccountSignature        []byte
	DeviceSignatureKey      []byte
	DeviceSignature         []byte
	IdentityKey             []byte
	NoiseKey                []byte
	Platform                *string
	BusinessName            *string
	PushName                *string
}

// ApplyUpdate applies the update data to the device
func (d *WhatsAppDevice) ApplyUpdate(update *UpdateFromWhatsmeow) {
	if update.RegistrationID != nil {
		d.RegistrationID = update.RegistrationID
	}
	if update.AdvSecretKey != nil {
		d.AdvSecretKey = update.AdvSecretKey
	}
	if update.NextPreKeyID != nil {
		d.NextPreKeyID = update.NextPreKeyID
	}
	if update.FirstUnuploadedPreKeyID != nil {
		d.FirstUnuploadedPreKeyID = update.FirstUnuploadedPreKeyID
	}
	if update.AccountSignatureKey != nil {
		d.AccountSignatureKey = update.AccountSignatureKey
	}
	if update.AccountSignature != nil {
		d.AccountSignature = update.AccountSignature
	}
	if update.DeviceSignatureKey != nil {
		d.DeviceSignatureKey = update.DeviceSignatureKey
	}
	if update.DeviceSignature != nil {
		d.DeviceSignature = update.DeviceSignature
	}
	if update.IdentityKey != nil {
		d.IdentityKey = update.IdentityKey
	}
	if update.NoiseKey != nil {
		d.NoiseKey = update.NoiseKey
	}
	if update.Platform != nil {
		d.Platform = update.Platform
	}
	if update.BusinessName != nil {
		d.BusinessName = update.BusinessName
	}
	if update.PushName != nil {
		d.PushName = update.PushName
	}
}

// DeviceSummary represents a summary of all devices for a user
type DeviceSummary struct {
	UserID       int               `json:"user_id"`
	TotalDevices int64             `json:"total_devices"`
	Devices      []*DeviceMetadata `json:"devices"`
	CreatedAt    time.Time         `json:"created_at"`
}
