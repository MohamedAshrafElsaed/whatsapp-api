package database

import (
	"context"
	"errors"
	"fmt"

	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	"gorm.io/gorm"
	"whatsapp-api/internal/models"
)

// PostgresStore implements the whatsmeow store.Store interface using PostgreSQL
type PostgresStore struct {
	db        *gorm.DB
	log       waLog.Logger
	jid       types.JID
	device    *models.WhatsAppDevice
	userID    int
	sessionID string
}

// NewPostgresStore creates a new PostgreSQL-backed store for whatsmeow
func NewPostgresStore(db *gorm.DB, jid types.JID, userID int, sessionID string, log waLog.Logger) *PostgresStore {
	return &PostgresStore{
		db:        db,
		log:       log,
		jid:       jid,
		userID:    userID,
		sessionID: sessionID,
	}
}

// GetDevice implements store.IdentityStore interface
func (s *PostgresStore) GetDevice() (*store.Device, error) {
	device := &models.WhatsAppDevice{}

	err := s.db.Where("jid = ? AND user_id = ?", s.jid.String(), s.userID).
		First(device).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	s.device = device

	// Convert to whatsmeow Device
	waDevice := &store.Device{
		Log: s.log,
	}

	// Set the basic info
	if device.RegistrationID != nil {
		waDevice.RegistrationID = *device.RegistrationID
	}

	if device.AdvSecretKey != nil {
		copy(waDevice.AdvSecretKey[:], device.AdvSecretKey)
	}

	if device.NextPreKeyID != nil {
		waDevice.NextPreKeyID = *device.NextPreKeyID
	}

	if device.FirstUnuploadedPreKeyID != nil {
		waDevice.FirstUnuploadedPreKeyID = *device.FirstUnuploadedPreKeyID
	}

	if device.AccountSignatureKey != nil {
		copy(waDevice.AccountSignatureKey[:], device.AccountSignatureKey)
	}

	if device.AccountSignature != nil {
		copy(waDevice.AccountSignature[:], device.AccountSignature)
	}

	if device.DeviceSignatureKey != nil {
		copy(waDevice.DeviceSignatureKey[:], device.DeviceSignatureKey)
	}

	if device.DeviceSignature != nil {
		copy(waDevice.DeviceSignature[:], device.DeviceSignature)
	}

	if device.IdentityKey != nil && len(device.IdentityKey) > 0 {
		waDevice.IdentityKey = &store.IdentityKey{}
		copy(waDevice.IdentityKey.Priv[:], device.IdentityKey[:32])
		copy(waDevice.IdentityKey.Pub[:], device.IdentityKey[32:])
	}

	if device.NoiseKey != nil && len(device.NoiseKey) > 0 {
		waDevice.NoiseKey = &store.NoiseKey{}
		copy(waDevice.NoiseKey.Priv[:], device.NoiseKey[:32])
		copy(waDevice.NoiseKey.Pub[:], device.NoiseKey[32:])
	}

	if device.Platform != nil {
		waDevice.Platform = *device.Platform
	}

	if device.BusinessName != nil {
		waDevice.BusinessName = *device.BusinessName
	}

	if device.PushName != nil {
		waDevice.PushName = *device.PushName
	}

	return waDevice, nil
}

// PutDevice implements store.IdentityStore interface
func (s *PostgresStore) PutDevice(device *store.Device) error {
	dbDevice := &models.WhatsAppDevice{
		JID:    s.jid.String(),
		UserID: s.userID,
	}

	sessionID := s.sessionID
	dbDevice.SessionID = &sessionID

	// Convert whatsmeow device to database model
	regID := device.RegistrationID
	dbDevice.RegistrationID = &regID

	if device.AdvSecretKey != [32]byte{} {
		dbDevice.AdvSecretKey = device.AdvSecretKey[:]
	}

	nextPreKey := device.NextPreKeyID
	dbDevice.NextPreKeyID = &nextPreKey

	firstUnuploaded := device.FirstUnuploadedPreKeyID
	dbDevice.FirstUnuploadedPreKeyID = &firstUnuploaded

	if device.AccountSignatureKey != [32]byte{} {
		dbDevice.AccountSignatureKey = device.AccountSignatureKey[:]
	}

	if device.AccountSignature != [64]byte{} {
		dbDevice.AccountSignature = device.AccountSignature[:]
	}

	if device.DeviceSignatureKey != [32]byte{} {
		dbDevice.DeviceSignatureKey = device.DeviceSignatureKey[:]
	}

	if device.DeviceSignature != [64]byte{} {
		dbDevice.DeviceSignature = device.DeviceSignature[:]
	}

	if device.IdentityKey != nil {
		identityKey := make([]byte, 64)
		copy(identityKey[:32], device.IdentityKey.Priv[:])
		copy(identityKey[32:], device.IdentityKey.Pub[:])
		dbDevice.IdentityKey = identityKey
	}

	if device.NoiseKey != nil {
		noiseKey := make([]byte, 64)
		copy(noiseKey[:32], device.NoiseKey.Priv[:])
		copy(noiseKey[32:], device.NoiseKey.Pub[:])
		dbDevice.NoiseKey = noiseKey
	}

	if device.Platform != "" {
		dbDevice.Platform = &device.Platform
	}

	if device.BusinessName != "" {
		dbDevice.BusinessName = &device.BusinessName
	}

	if device.PushName != "" {
		dbDevice.PushName = &device.PushName
	}

	// Upsert the device
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing models.WhatsAppDevice
		err := tx.Where("jid = ? AND user_id = ?", s.jid.String(), s.userID).
			First(&existing).Error

		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// Insert new device
				return tx.Create(dbDevice).Error
			}
			return err
		}

		// Update existing device
		return tx.Model(&existing).Updates(dbDevice).Error
	})

	if err != nil {
		return fmt.Errorf("failed to put device: %w", err)
	}

	s.device = dbDevice
	return nil
}

// DeleteDevice deletes the device from the store
func (s *PostgresStore) DeleteDevice() error {
	err := s.db.Where("jid = ? AND user_id = ?", s.jid.String(), s.userID).
		Delete(&models.WhatsAppDevice{}).Error

	if err != nil {
		return fmt.Errorf("failed to delete device: %w", err)
	}

	s.device = nil
	return nil
}

// PostgresContainer implements store.Container interface
type PostgresContainer struct {
	db  *gorm.DB
	log waLog.Logger
}

// NewPostgresContainer creates a new PostgreSQL-backed container for whatsmeow
func NewPostgresContainer(db *gorm.DB, log waLog.Logger) *PostgresContainer {
	return &PostgresContainer{
		db:  db,
		log: log,
	}
}

// GetFirstDevice returns the first device in the container (not typically used in multi-user scenario)
func (c *PostgresContainer) GetFirstDevice() (*store.Device, error) {
	var device models.WhatsAppDevice

	err := c.db.Order("created_at ASC").First(&device).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get first device: %w", err)
	}

	jid, err := types.ParseJID(device.JID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JID: %w", err)
	}

	sessionID := ""
	if device.SessionID != nil {
		sessionID = *device.SessionID
	}

	store := NewPostgresStore(c.db, jid, device.UserID, sessionID, c.log)
	return store.GetDevice()
}

// GetAllDevices returns all devices in the container
func (c *PostgresContainer) GetAllDevices() ([]*store.Device, error) {
	var devices []models.WhatsAppDevice

	err := c.db.Order("created_at ASC").Find(&devices).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get all devices: %w", err)
	}

	result := make([]*store.Device, 0, len(devices))
	for _, device := range devices {
		jid, err := types.ParseJID(device.JID)
		if err != nil {
			c.log.Warnf("Failed to parse JID %s: %v", device.JID, err)
			continue
		}

		sessionID := ""
		if device.SessionID != nil {
			sessionID = *device.SessionID
		}

		store := NewPostgresStore(c.db, jid, device.UserID, sessionID, c.log)
		waDevice, err := store.GetDevice()
		if err != nil {
			c.log.Warnf("Failed to get device for JID %s: %v", device.JID, err)
			continue
		}

		if waDevice != nil {
			result = append(result, waDevice)
		}
	}

	return result, nil
}

// GetDevice returns a device store for a specific JID
func (c *PostgresContainer) GetDevice(jid types.JID) (*store.Device, error) {
	var device models.WhatsAppDevice

	err := c.db.Where("jid = ?", jid.String()).First(&device).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	sessionID := ""
	if device.SessionID != nil {
		sessionID = *device.SessionID
	}

	store := NewPostgresStore(c.db, jid, device.UserID, sessionID, c.log)
	return store.GetDevice()
}

// NewDevice creates a new device store for the given JID
func (c *PostgresContainer) NewDevice(userID int, sessionID string) *store.Device {
	device := &store.Device{
		Log: c.log,
	}

	return device
}

// GetDeviceStore returns a PostgresStore for a specific session
func GetDeviceStore(db *gorm.DB, jid types.JID, userID int, sessionID string, log waLog.Logger) (*PostgresStore, error) {
	store := NewPostgresStore(db, jid, userID, sessionID, log)

	// Try to load existing device
	_, err := store.GetDevice()
	if err != nil {
		return nil, fmt.Errorf("failed to load device store: %w", err)
	}

	return store, nil
}

// CreateDeviceStore creates a new device store for a session
func CreateDeviceStore(ctx context.Context, db *gorm.DB, jid types.JID, userID int, sessionID string, log waLog.Logger) (*PostgresStore, error) {
	store := NewPostgresStore(db, jid, userID, sessionID, log)

	// Create a new device entry
	device := &store.Device{
		Log: log,
	}

	if err := store.PutDevice(device); err != nil {
		return nil, fmt.Errorf("failed to create device store: %w", err)
	}

	return store, nil
}

// DeleteDeviceStore deletes a device store
func DeleteDeviceStore(ctx context.Context, db *gorm.DB, jid string, userID int) error {
	err := db.Where("jid = ? AND user_id = ?", jid, userID).
		Delete(&models.WhatsAppDevice{}).Error

	if err != nil {
		return fmt.Errorf("failed to delete device store: %w", err)
	}

	return nil
}

// DeviceExists checks if a device exists for the given JID and user
func DeviceExists(db *gorm.DB, jid string, userID int) (bool, error) {
	var count int64
	err := db.Model(&models.WhatsAppDevice{}).
		Where("jid = ? AND user_id = ?", jid, userID).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("failed to check device existence: %w", err)
	}

	return count > 0, nil
}
