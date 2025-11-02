package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"whatsapp-api/internal/models"
)

// SessionRepository handles database operations for WhatsApp sessions
type SessionRepository struct {
	db *gorm.DB
}

// NewSessionRepository creates a new session repositories
func NewSessionRepository(db *gorm.DB) *SessionRepository {
	return &SessionRepository{
		db: db,
	}
}

// Create creates a new WhatsApp session
func (r *SessionRepository) Create(ctx context.Context, session *models.WhatsAppSession) error {
	return r.db.WithContext(ctx).Create(session).Error
}

// GetByID retrieves a session by ID
func (r *SessionRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.WhatsAppSession, error) {
	var session models.WhatsAppSession
	err := r.db.WithContext(ctx).
		Preload("Device").
		Where("id = ?", id).
		First(&session).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("session not found")
		}
		return nil, err
	}

	return &session, nil
}

// GetByIDAndUserID retrieves a session by ID and user ID
func (r *SessionRepository) GetByIDAndUserID(ctx context.Context, id uuid.UUID, userID int) (*models.WhatsAppSession, error) {
	var session models.WhatsAppSession
	err := r.db.WithContext(ctx).
		Preload("Device").
		Where("id = ? AND user_id = ?", id, userID).
		First(&session).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("session not found")
		}
		return nil, err
	}

	return &session, nil
}

// GetByJID retrieves a session by JID
func (r *SessionRepository) GetByJID(ctx context.Context, jid string, userID int) (*models.WhatsAppSession, error) {
	var session models.WhatsAppSession
	err := r.db.WithContext(ctx).
		Preload("Device").
		Where("jid = ? AND user_id = ?", jid, userID).
		First(&session).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("session not found")
		}
		return nil, err
	}

	return &session, nil
}

// GetByUserID retrieves all sessions for a user
func (r *SessionRepository) GetByUserID(ctx context.Context, userID int) ([]*models.WhatsAppSession, error) {
	var sessions []*models.WhatsAppSession
	err := r.db.WithContext(ctx).
		Preload("Device").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&sessions).Error

	if err != nil {
		return nil, err
	}

	return sessions, nil
}

// GetActiveByUserID retrieves all active sessions for a user
func (r *SessionRepository) GetActiveByUserID(ctx context.Context, userID int) ([]*models.WhatsAppSession, error) {
	var sessions []*models.WhatsAppSession
	err := r.db.WithContext(ctx).
		Preload("Device").
		Where("user_id = ? AND is_active = ?", userID, true).
		Order("created_at DESC").
		Find(&sessions).Error

	if err != nil {
		return nil, err
	}

	return sessions, nil
}

// GetConnectedByUserID retrieves all connected sessions for a user
func (r *SessionRepository) GetConnectedByUserID(ctx context.Context, userID int) ([]*models.WhatsAppSession, error) {
	var sessions []*models.WhatsAppSession
	err := r.db.WithContext(ctx).
		Preload("Device").
		Where("user_id = ? AND status = ? AND is_active = ?", userID, models.SessionStatusConnected, true).
		Order("connected_at DESC").
		Find(&sessions).Error

	if err != nil {
		return nil, err
	}

	return sessions, nil
}

// Update updates a session
func (r *SessionRepository) Update(ctx context.Context, session *models.WhatsAppSession) error {
	return r.db.WithContext(ctx).Save(session).Error
}

// UpdateStatus updates session status
func (r *SessionRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.SessionStatus) error {
	return r.db.WithContext(ctx).
		Model(&models.WhatsAppSession{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// UpdateLastSeen updates last seen timestamp
func (r *SessionRepository) UpdateLastSeen(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&models.WhatsAppSession{}).
		Where("id = ?", id).
		Update("last_seen", now).Error
}

// Delete soft deletes a session
func (r *SessionRepository) Delete(ctx context.Context, id uuid.UUID, userID int) error {
	return r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&models.WhatsAppSession{}).Error
}

// HardDelete permanently deletes a session
func (r *SessionRepository) HardDelete(ctx context.Context, id uuid.UUID, userID int) error {
	return r.db.WithContext(ctx).
		Unscoped().
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&models.WhatsAppSession{}).Error
}

// Deactivate deactivates a session
func (r *SessionRepository) Deactivate(ctx context.Context, id uuid.UUID, userID int) error {
	return r.db.WithContext(ctx).
		Model(&models.WhatsAppSession{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]interface{}{
			"is_active":       false,
			"status":          models.SessionStatusDisconnected,
			"disconnected_at": time.Now(),
		}).Error
}

// CountActiveByUserID counts active sessions for a user
func (r *SessionRepository) CountActiveByUserID(ctx context.Context, userID int) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.WhatsAppSession{}).
		Where("user_id = ? AND is_active = ? AND status IN ?",
			userID,
			true,
			[]models.SessionStatus{
				models.SessionStatusConnected,
				models.SessionStatusPending,
				models.SessionStatusQRReady,
				models.SessionStatusScanning,
			}).
		Count(&count).Error

	return count, err
}

// CountConnectedByUserID counts connected sessions for a user
func (r *SessionRepository) CountConnectedByUserID(ctx context.Context, userID int) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.WhatsAppSession{}).
		Where("user_id = ? AND is_active = ? AND status = ?", userID, true, models.SessionStatusConnected).
		Count(&count).Error

	return count, err
}

// GetSummary retrieves session summary for a user
func (r *SessionRepository) GetSummary(ctx context.Context, userID int, maxDevices int) (*models.SessionSummary, error) {
	summary := &models.SessionSummary{
		UserID:     userID,
		MaxDevices: maxDevices,
	}

	// Total sessions
	err := r.db.WithContext(ctx).
		Model(&models.WhatsAppSession{}).
		Where("user_id = ?", userID).
		Count(&summary.TotalSessions).Error
	if err != nil {
		return nil, err
	}

	// Connected count
	err = r.db.WithContext(ctx).
		Model(&models.WhatsAppSession{}).
		Where("user_id = ? AND status = ? AND is_active = ?", userID, models.SessionStatusConnected, true).
		Count(&summary.ConnectedCount).Error
	if err != nil {
		return nil, err
	}

	// Pending count (includes qr_ready and scanning)
	err = r.db.WithContext(ctx).
		Model(&models.WhatsAppSession{}).
		Where("user_id = ? AND status IN ? AND is_active = ?",
			userID,
			[]models.SessionStatus{
				models.SessionStatusPending,
				models.SessionStatusQRReady,
				models.SessionStatusScanning,
			},
			true).
		Count(&summary.PendingCount).Error
	if err != nil {
		return nil, err
	}

	// Disconnected count
	err = r.db.WithContext(ctx).
		Model(&models.WhatsAppSession{}).
		Where("user_id = ? AND status = ? AND is_active = ?", userID, models.SessionStatusDisconnected, true).
		Count(&summary.DisconnectedCount).Error
	if err != nil {
		return nil, err
	}

	// Last activity
	var lastSeen *time.Time
	err = r.db.WithContext(ctx).
		Model(&models.WhatsAppSession{}).
		Where("user_id = ?", userID).
		Select("MAX(last_seen)").
		Scan(&lastSeen).Error
	if err != nil {
		return nil, err
	}
	summary.LastActivity = lastSeen

	// Calculate available slots
	usedSlots := int(summary.ConnectedCount + summary.PendingCount)
	summary.AvailableSlots = maxDevices - usedSlots
	if summary.AvailableSlots < 0 {
		summary.AvailableSlots = 0
	}

	return summary, nil
}

// GetExpiredQRSessions retrieves sessions with expired QR codes
func (r *SessionRepository) GetExpiredQRSessions(ctx context.Context) ([]*models.WhatsAppSession, error) {
	var sessions []*models.WhatsAppSession
	now := time.Now()

	err := r.db.WithContext(ctx).
		Where("status = ? AND qr_expires_at < ? AND is_active = ?",
			models.SessionStatusQRReady,
			now,
			true).
		Find(&sessions).Error

	if err != nil {
		return nil, err
	}

	return sessions, nil
}

// GetInactiveSessions retrieves sessions that have been inactive for a given duration
func (r *SessionRepository) GetInactiveSessions(ctx context.Context, duration time.Duration) ([]*models.WhatsAppSession, error) {
	var sessions []*models.WhatsAppSession
	threshold := time.Now().Add(-duration)

	err := r.db.WithContext(ctx).
		Where("status = ? AND last_seen < ? AND is_active = ?",
			models.SessionStatusConnected,
			threshold,
			true).
		Find(&sessions).Error

	if err != nil {
		return nil, err
	}

	return sessions, nil
}

// ExistsForUser checks if a session with the given name exists for a user
func (r *SessionRepository) ExistsForUser(ctx context.Context, userID int, sessionName string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.WhatsAppSession{}).
		Where("user_id = ? AND session_name = ?", userID, sessionName).
		Count(&count).Error

	return count > 0, err
}

// GetBySessionName retrieves a session by name and user ID
func (r *SessionRepository) GetBySessionName(ctx context.Context, userID int, sessionName string) (*models.WhatsAppSession, error) {
	var session models.WhatsAppSession
	err := r.db.WithContext(ctx).
		Preload("Device").
		Where("user_id = ? AND session_name = ?", userID, sessionName).
		First(&session).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("session not found")
		}
		return nil, err
	}

	return &session, nil
}

// BulkUpdateStatus updates status for multiple sessions
func (r *SessionRepository) BulkUpdateStatus(ctx context.Context, ids []uuid.UUID, status models.SessionStatus) error {
	return r.db.WithContext(ctx).
		Model(&models.WhatsAppSession{}).
		Where("id IN ?", ids).
		Update("status", status).Error
}

// GetAllByStatus retrieves all sessions with a specific status
func (r *SessionRepository) GetAllByStatus(ctx context.Context, status models.SessionStatus) ([]*models.WhatsAppSession, error) {
	var sessions []*models.WhatsAppSession
	err := r.db.WithContext(ctx).
		Where("status = ?", status).
		Order("created_at DESC").
		Find(&sessions).Error

	if err != nil {
		return nil, err
	}

	return sessions, nil
}

// CleanupExpiredSessions marks expired QR sessions as expired
func (r *SessionRepository) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	result := r.db.WithContext(ctx).
		Model(&models.WhatsAppSession{}).
		Where("status = ? AND qr_expires_at < ?", models.SessionStatusQRReady, time.Now()).
		Updates(map[string]interface{}{
			"status":    models.SessionStatusExpired,
			"is_active": false,
		})

	return result.RowsAffected, result.Error
}

// CleanupInactiveSessions marks inactive sessions as disconnected
func (r *SessionRepository) CleanupInactiveSessions(ctx context.Context, duration time.Duration) (int64, error) {
	threshold := time.Now().Add(-duration)

	result := r.db.WithContext(ctx).
		Model(&models.WhatsAppSession{}).
		Where("status = ? AND last_seen < ? AND is_active = ?",
			models.SessionStatusConnected,
			threshold,
			true).
		Updates(map[string]interface{}{
			"status":          models.SessionStatusDisconnected,
			"disconnected_at": time.Now(),
		})

	return result.RowsAffected, result.Error
}
