package repositories

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"whatsapp-api/internal/models"
)

// DeviceRepository handles database operations for WhatsApp devices
type DeviceRepository struct {
	db *gorm.DB
}

// NewDeviceRepository creates a new device repositories
func NewDeviceRepository(db *gorm.DB) *DeviceRepository {
	return &DeviceRepository{
		db: db,
	}
}

// Create creates a new WhatsApp device
func (r *DeviceRepository) Create(ctx context.Context, device *models.WhatsAppDevice) error {
	return r.db.WithContext(ctx).Create(device).Error
}

// GetByJID retrieves a device by JID
func (r *DeviceRepository) GetByJID(ctx context.Context, jid string) (*models.WhatsAppDevice, error) {
	var device models.WhatsAppDevice
	err := r.db.WithContext(ctx).
		Preload("Session").
		Where("jid = ?", jid).
		First(&device).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("device not found")
		}
		return nil, err
	}

	return &device, nil
}

// GetByJIDAndUserID retrieves a device by JID and user ID
func (r *DeviceRepository) GetByJIDAndUserID(ctx context.Context, jid string, userID int) (*models.WhatsAppDevice, error) {
	var device models.WhatsAppDevice
	err := r.db.WithContext(ctx).
		Preload("Session").
		Where("jid = ? AND user_id = ?", jid, userID).
		First(&device).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("device not found")
		}
		return nil, err
	}

	return &device, nil
}

// GetBySessionID retrieves a device by session ID
func (r *DeviceRepository) GetBySessionID(ctx context.Context, sessionID string) (*models.WhatsAppDevice, error) {
	var device models.WhatsAppDevice
	err := r.db.WithContext(ctx).
		Preload("Session").
		Where("session_id = ?", sessionID).
		First(&device).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("device not found")
		}
		return nil, err
	}

	return &device, nil
}

// GetByUserID retrieves all devices for a user
func (r *DeviceRepository) GetByUserID(ctx context.Context, userID int) ([]*models.WhatsAppDevice, error) {
	var devices []*models.WhatsAppDevice
	err := r.db.WithContext(ctx).
		Preload("Session").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&devices).Error

	if err != nil {
		return nil, err
	}

	return devices, nil
}

// Update updates a device
func (r *DeviceRepository) Update(ctx context.Context, device *models.WhatsAppDevice) error {
	return r.db.WithContext(ctx).Save(device).Error
}

// UpdateFields updates specific fields of a device
func (r *DeviceRepository) UpdateFields(ctx context.Context, jid string, userID int, fields map[string]interface{}) error {
	return r.db.WithContext(ctx).
		Model(&models.WhatsAppDevice{}).
		Where("jid = ? AND user_id = ?", jid, userID).
		Updates(fields).Error
}

// Delete deletes a device
func (r *DeviceRepository) Delete(ctx context.Context, jid string, userID int) error {
	return r.db.WithContext(ctx).
		Where("jid = ? AND user_id = ?", jid, userID).
		Delete(&models.WhatsAppDevice{}).Error
}

// DeleteBySessionID deletes a device by session ID
func (r *DeviceRepository) DeleteBySessionID(ctx context.Context, sessionID string) error {
	return r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&models.WhatsAppDevice{}).Error
}

// Exists checks if a device exists
func (r *DeviceRepository) Exists(ctx context.Context, jid string, userID int) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.WhatsAppDevice{}).
		Where("jid = ? AND user_id = ?", jid, userID).
		Count(&count).Error

	return count > 0, err
}

// ExistsByJID checks if a device exists by JID only
func (r *DeviceRepository) ExistsByJID(ctx context.Context, jid string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.WhatsAppDevice{}).
		Where("jid = ?", jid).
		Count(&count).Error

	return count > 0, err
}

// CountByUserID counts devices for a user
func (r *DeviceRepository) CountByUserID(ctx context.Context, userID int) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.WhatsAppDevice{}).
		Where("user_id = ?", userID).
		Count(&count).Error

	return count, err
}

// GetDeviceIdentities retrieves device identities for a user
func (r *DeviceRepository) GetDeviceIdentities(ctx context.Context, userID int) ([]*models.DeviceIdentity, error) {
	var devices []*models.WhatsAppDevice
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&devices).Error

	if err != nil {
		return nil, err
	}

	identities := make([]*models.DeviceIdentity, 0, len(devices))
	for _, device := range devices {
		identities = append(identities, device.ToDeviceIdentity())
	}

	return identities, nil
}

// GetDeviceMetadataList retrieves device metadata for a user
func (r *DeviceRepository) GetDeviceMetadataList(ctx context.Context, userID int) ([]*models.DeviceMetadata, error) {
	var devices []*models.WhatsAppDevice
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&devices).Error

	if err != nil {
		return nil, err
	}

	metadata := make([]*models.DeviceMetadata, 0, len(devices))
	for _, device := range devices {
		metadata = append(metadata, device.ToDeviceMetadata())
	}

	return metadata, nil
}

// GetDeviceSummary retrieves device summary for a user
func (r *DeviceRepository) GetDeviceSummary(ctx context.Context, userID int) (*models.DeviceSummary, error) {
	var devices []*models.WhatsAppDevice
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at ASC").
		Find(&devices).Error

	if err != nil {
		return nil, err
	}

	summary := &models.DeviceSummary{
		UserID:       userID,
		TotalDevices: int64(len(devices)),
		Devices:      make([]*models.DeviceMetadata, 0, len(devices)),
	}

	for _, device := range devices {
		summary.Devices = append(summary.Devices, device.ToDeviceMetadata())
	}

	if len(devices) > 0 {
		summary.CreatedAt = devices[0].CreatedAt
	}

	return summary, nil
}

// GetIncompleteDevices retrieves devices that don't have all required keys
func (r *DeviceRepository) GetIncompleteDevices(ctx context.Context) ([]*models.WhatsAppDevice, error) {
	var devices []*models.WhatsAppDevice
	err := r.db.WithContext(ctx).
		Where("identity_key IS NULL OR noise_key IS NULL OR adv_secret_key IS NULL").
		Order("created_at DESC").
		Find(&devices).Error

	if err != nil {
		return nil, err
	}

	return devices, nil
}

// GetDevicesNeedingPreKeyUpload retrieves devices that need pre-key upload
func (r *DeviceRepository) GetDevicesNeedingPreKeyUpload(ctx context.Context) ([]*models.WhatsAppDevice, error) {
	var devices []*models.WhatsAppDevice
	err := r.db.WithContext(ctx).
		Where("next_pre_key_id IS NOT NULL AND first_unuploaded_pre_key_id IS NOT NULL").
		Where("first_unuploaded_pre_key_id > next_pre_key_id").
		Order("created_at DESC").
		Find(&devices).Error

	if err != nil {
		return nil, err
	}

	return devices, nil
}

// GetBusinessAccounts retrieves all business accounts
func (r *DeviceRepository) GetBusinessAccounts(ctx context.Context, userID int) ([]*models.WhatsAppDevice, error) {
	var devices []*models.WhatsAppDevice
	err := r.db.WithContext(ctx).
		Preload("Session").
		Where("user_id = ? AND business_name IS NOT NULL AND business_name != ''", userID).
		Order("created_at DESC").
		Find(&devices).Error

	if err != nil {
		return nil, err
	}

	return devices, nil
}

// UpdatePreKeyInfo updates pre-key information for a device
func (r *DeviceRepository) UpdatePreKeyInfo(ctx context.Context, jid string, userID int, nextPreKeyID, firstUnuploadedPreKeyID uint32) error {
	return r.db.WithContext(ctx).
		Model(&models.WhatsAppDevice{}).
		Where("jid = ? AND user_id = ?", jid, userID).
		Updates(map[string]interface{}{
			"next_pre_key_id":             nextPreKeyID,
			"first_unuploaded_pre_key_id": firstUnuploadedPreKeyID,
		}).Error
}

// UpdateIdentityKeys updates identity keys for a device
func (r *DeviceRepository) UpdateIdentityKeys(ctx context.Context, jid string, userID int, identityKey, noiseKey []byte) error {
	return r.db.WithContext(ctx).
		Model(&models.WhatsAppDevice{}).
		Where("jid = ? AND user_id = ?", jid, userID).
		Updates(map[string]interface{}{
			"identity_key": identityKey,
			"noise_key":    noiseKey,
		}).Error
}

// UpdateSignatures updates signature keys for a device
func (r *DeviceRepository) UpdateSignatures(ctx context.Context, jid string, userID int, update *models.UpdateFromWhatsmeow) error {
	updates := make(map[string]interface{})

	if update.AccountSignatureKey != nil {
		updates["account_signature_key"] = update.AccountSignatureKey
	}
	if update.AccountSignature != nil {
		updates["account_signature"] = update.AccountSignature
	}
	if update.DeviceSignatureKey != nil {
		updates["device_signature_key"] = update.DeviceSignatureKey
	}
	if update.DeviceSignature != nil {
		updates["device_signature"] = update.DeviceSignature
	}

	if len(updates) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).
		Model(&models.WhatsAppDevice{}).
		Where("jid = ? AND user_id = ?", jid, userID).
		Updates(updates).Error
}

// BulkDelete deletes multiple devices
func (r *DeviceRepository) BulkDelete(ctx context.Context, jids []string, userID int) error {
	return r.db.WithContext(ctx).
		Where("jid IN ? AND user_id = ?", jids, userID).
		Delete(&models.WhatsAppDevice{}).Error
}

// GetAllDevices retrieves all devices (admin use)
func (r *DeviceRepository) GetAllDevices(ctx context.Context, limit, offset int) ([]*models.WhatsAppDevice, error) {
	var devices []*models.WhatsAppDevice
	query := r.db.WithContext(ctx).
		Preload("Session").
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Find(&devices).Error
	if err != nil {
		return nil, err
	}

	return devices, nil
}

// GetTotalDeviceCount gets total count of all devices
func (r *DeviceRepository) GetTotalDeviceCount(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.WhatsAppDevice{}).
		Count(&count).Error

	return count, err
}

// CleanupOrphanedDevices removes devices without associated sessions
func (r *DeviceRepository) CleanupOrphanedDevices(ctx context.Context) (int64, error) {
	result := r.db.WithContext(ctx).
		Exec(`
			DELETE FROM whatsapp_devices 
			WHERE session_id IS NOT NULL 
			AND session_id NOT IN (SELECT id::text FROM whatsapp_sessions WHERE deleted_at IS NULL)
		`)

	return result.RowsAffected, result.Error
}

// Upsert creates or updates a device
func (r *DeviceRepository) Upsert(ctx context.Context, device *models.WhatsAppDevice) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.WhatsAppDevice
		err := tx.Where("jid = ? AND user_id = ?", device.JID, device.UserID).
			First(&existing).Error

		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// Insert new device
				return tx.Create(device).Error
			}
			return err
		}

		// Update existing device
		return tx.Model(&existing).Updates(device).Error
	})
}
