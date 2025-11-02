package main

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ============= MODELS =============

type SessionStatus string

const (
	StatusPending      SessionStatus = "pending"
	StatusQRReady      SessionStatus = "qr_ready"
	StatusScanning     SessionStatus = "scanning"
	StatusConnected    SessionStatus = "connected"
	StatusDisconnected SessionStatus = "disconnected"
	StatusFailed       SessionStatus = "failed"
	StatusExpired      SessionStatus = "expired"
)

// WhatsAppSession represents a WhatsApp session in the database
type WhatsAppSession struct {
	ID             uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID         int            `gorm:"not null;index;index:idx_user_session,unique" json:"user_id"`
	SessionName    string         `gorm:"size:255;not null;index:idx_user_session,unique" json:"session_name"`
	PhoneNumber    *string        `gorm:"size:20" json:"phone_number,omitempty"`
	JID            *string        `gorm:"size:255;uniqueIndex" json:"jid,omitempty"`
	Status         SessionStatus  `gorm:"size:50;not null;default:'pending';index" json:"status"`
	QRCode         *string        `gorm:"type:text" json:"-"`
	QRCodeBase64   *string        `gorm:"type:text" json:"qr_code_base64,omitempty"`
	QRGeneratedAt  *time.Time     `json:"qr_generated_at,omitempty"`
	QRExpiresAt    *time.Time     `json:"qr_expires_at,omitempty"`
	QRRetryCount   int            `gorm:"default:0" json:"qr_retry_count"`
	ConnectedAt    *time.Time     `json:"connected_at,omitempty"`
	DisconnectedAt *time.Time     `json:"disconnected_at,omitempty"`
	LastSeen       *time.Time     `json:"last_seen,omitempty"`
	DeviceInfo     JSONB          `gorm:"type:jsonb" json:"device_info,omitempty"`
	PushName       *string        `gorm:"size:255" json:"push_name,omitempty"`
	Platform       *string        `gorm:"size:50" json:"platform,omitempty"`
	IsActive       bool           `gorm:"default:true;index" json:"is_active"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// WhatsAppEvent represents an event log
type WhatsAppEvent struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID uuid.UUID `gorm:"type:uuid;index" json:"session_id"`
	UserID    int       `gorm:"not null;index" json:"user_id"`
	EventType string    `gorm:"size:100;not null;index" json:"event_type"`
	EventData JSONB     `gorm:"type:jsonb" json:"event_data"`
	CreatedAt time.Time `json:"created_at"`
}

// JSONB type for PostgreSQL JSONB fields
type JSONB map[string]interface{}

func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return errors.New("unsupported type for JSONB")
	}

	return json.Unmarshal(data, j)
}

func (s *SessionStatus) Scan(value interface{}) error {
	if value == nil {
		*s = ""
		return nil
	}
	switch v := value.(type) {
	case string:
		*s = SessionStatus(v)
	case []byte:
		*s = SessionStatus(v)
	}
	return nil
}

func (s SessionStatus) Value() (driver.Value, error) {
	return string(s), nil
}

// ============= DATABASE MANAGER =============

type DatabaseManager struct {
	db          *gorm.DB
	sqlDB       *sqlstore.Container
	waContainer *sqlstore.Container
}

func (db *DatabaseManager) GetWhatsAppContainer() *sqlstore.Container {
	return db.waContainer
}

func NewDatabaseManager(cfg *Config) (*DatabaseManager, error) {
	// PostgreSQL connection string
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBSSLMode)

	// GORM connection
	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
		PrepareStmt: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// WhatsApp store container
	container, err := sqlstore.New(context.Background(), "postgres", dsn, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create WhatsApp store container: %w", err)
	}

	waContainer := container

	dm := &DatabaseManager{
		db:          gormDB,
		sqlDB:       container,
		waContainer: waContainer,
	}

	// Run migrations
	if err := dm.Migrate(); err != nil {
		return nil, err
	}

	return dm, nil
}

// Migrate creates all necessary tables and constraints
func (dm *DatabaseManager) Migrate() error {
	// Enable UUID extension
	dm.db.Exec("CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\"")

	// Auto migrate models
	if err := dm.db.AutoMigrate(&WhatsAppSession{}, &WhatsAppEvent{}); err != nil {
		return err
	}

	// Create device limit function
	dm.db.Exec(`
		CREATE OR REPLACE FUNCTION check_device_limit()
		RETURNS TRIGGER AS $$
		DECLARE
			active_count INTEGER;
			max_allowed INTEGER := 5;
		BEGIN
			SELECT COUNT(*) INTO active_count
			FROM whatsapp_sessions
			WHERE user_id = NEW.user_id
				AND is_active = true
				AND status IN ('connected', 'pending', 'qr_ready', 'scanning')
				AND id != NEW.id
				AND deleted_at IS NULL;
			
			IF active_count >= max_allowed THEN
				RAISE EXCEPTION 'Device limit exceeded. Maximum % devices allowed per user.', max_allowed;
			END IF;
			
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
	`)

	// Create trigger
	dm.db.Exec(`
		DROP TRIGGER IF EXISTS enforce_device_limit ON whatsapp_sessions;
		CREATE TRIGGER enforce_device_limit
			BEFORE INSERT OR UPDATE ON whatsapp_sessions
			FOR EACH ROW
			WHEN (NEW.status IN ('pending', 'qr_ready', 'scanning', 'connected') AND NEW.is_active = true)
			EXECUTE FUNCTION check_device_limit();
	`)

	// Create indexes
	dm.db.Exec("CREATE INDEX IF NOT EXISTS idx_sessions_user_status ON whatsapp_sessions(user_id, status) WHERE deleted_at IS NULL")
	dm.db.Exec("CREATE INDEX IF NOT EXISTS idx_events_session_created ON whatsapp_events(session_id, created_at DESC)")

	log.Println("Database migration completed")
	return nil
}

// ============= SESSION REPOSITORY =============

func (dm *DatabaseManager) CreateSession(userID int, sessionName string) (*WhatsAppSession, error) {
	session := &WhatsAppSession{
		ID:          uuid.New(),
		UserID:      userID,
		SessionName: sessionName,
		Status:      StatusPending,
		IsActive:    true,
	}

	if err := dm.db.Create(session).Error; err != nil {
		return nil, err
	}

	return session, nil
}

func (dm *DatabaseManager) GetSession(sessionID uuid.UUID, userID int) (*WhatsAppSession, error) {
	var session WhatsAppSession
	err := dm.db.Where("id = ? AND user_id = ?", sessionID, userID).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (dm *DatabaseManager) GetUserSessions(userID int) ([]WhatsAppSession, error) {
	var sessions []WhatsAppSession
	err := dm.db.Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("created_at DESC").
		Find(&sessions).Error
	return sessions, err
}

func (dm *DatabaseManager) UpdateSession(session *WhatsAppSession) error {
	return dm.db.Save(session).Error
}

func (dm *DatabaseManager) UpdateSessionStatus(sessionID uuid.UUID, status SessionStatus) error {
	return dm.db.Model(&WhatsAppSession{}).
		Where("id = ?", sessionID).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": time.Now(),
		}).Error
}

func (dm *DatabaseManager) DeleteSession(sessionID uuid.UUID, userID int) error {
	return dm.db.Where("id = ? AND user_id = ?", sessionID, userID).
		Delete(&WhatsAppSession{}).Error
}

func (dm *DatabaseManager) SetSessionConnected(sessionID uuid.UUID, jid, phoneNumber, pushName, platform string) error {
	now := time.Now()
	return dm.db.Model(&WhatsAppSession{}).
		Where("id = ?", sessionID).
		Updates(map[string]interface{}{
			"status":          StatusConnected,
			"jid":             jid,
			"phone_number":    phoneNumber,
			"push_name":       pushName,
			"platform":        platform,
			"connected_at":    now,
			"last_seen":       now,
			"disconnected_at": nil,
			"qr_code":         nil,
			"qr_code_base64":  nil,
		}).Error
}

func (dm *DatabaseManager) SetSessionDisconnected(sessionID uuid.UUID) error {
	now := time.Now()
	return dm.db.Model(&WhatsAppSession{}).
		Where("id = ?", sessionID).
		Updates(map[string]interface{}{
			"status":          StatusDisconnected,
			"disconnected_at": now,
			"last_seen":       now,
		}).Error
}

func (dm *DatabaseManager) UpdateSessionQR(sessionID uuid.UUID, qrCode, base64QR string, timeout time.Duration) error {
	now := time.Now()
	expiresAt := now.Add(timeout)

	return dm.db.Model(&WhatsAppSession{}).
		Where("id = ?", sessionID).
		Updates(map[string]interface{}{
			"status":          StatusQRReady,
			"qr_code":         qrCode,
			"qr_code_base64":  base64QR,
			"qr_generated_at": now,
			"qr_expires_at":   expiresAt,
			"qr_retry_count":  gorm.Expr("qr_retry_count + 1"),
		}).Error
}

func (dm *DatabaseManager) GetActiveSessionCount(userID int) (int64, error) {
	var count int64
	err := dm.db.Model(&WhatsAppSession{}).
		Where("user_id = ? AND is_active = true AND status IN ('connected', 'pending', 'qr_ready', 'scanning') AND deleted_at IS NULL", userID).
		Count(&count).Error
	return count, err
}

// ============= EVENT REPOSITORY =============

func (dm *DatabaseManager) CreateEvent(sessionID uuid.UUID, userID int, eventType string, data map[string]interface{}) error {
	event := &WhatsAppEvent{
		SessionID: sessionID,
		UserID:    userID,
		EventType: eventType,
		EventData: data,
		CreatedAt: time.Now(),
	}
	return dm.db.Create(event).Error
}

func (dm *DatabaseManager) GetSessionEvents(sessionID uuid.UUID, limit int) ([]WhatsAppEvent, error) {
	var events []WhatsAppEvent
	query := dm.db.Where("session_id = ?", sessionID).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&events).Error
	return events, err
}

// ============= DEVICE SUMMARY =============

type DeviceSummary struct {
	UserID           int              `json:"user_id"`
	MaxDevices       int              `json:"max_devices"`
	UsedDevices      int              `json:"used_devices"`
	AvailableSlots   int              `json:"available_slots"`
	ConnectedDevices int              `json:"connected_devices"`
	Sessions         []SessionSummary `json:"sessions"`
}

type SessionSummary struct {
	ID          uuid.UUID     `json:"id"`
	SessionName string        `json:"session_name"`
	Status      SessionStatus `json:"status"`
	PhoneNumber *string       `json:"phone_number,omitempty"`
	ConnectedAt *time.Time    `json:"connected_at,omitempty"`
	LastSeen    *time.Time    `json:"last_seen,omitempty"`
}

func (dm *DatabaseManager) GetUserDeviceSummary(userID int) (*DeviceSummary, error) {
	sessions, err := dm.GetUserSessions(userID)
	if err != nil {
		return nil, err
	}

	summary := &DeviceSummary{
		UserID:     userID,
		MaxDevices: 5,
		Sessions:   make([]SessionSummary, 0),
	}

	for _, session := range sessions {
		if session.IsActive {
			summary.UsedDevices++
			if session.Status == StatusConnected {
				summary.ConnectedDevices++
			}
		}

		summary.Sessions = append(summary.Sessions, SessionSummary{
			ID:          session.ID,
			SessionName: session.SessionName,
			Status:      session.Status,
			PhoneNumber: session.PhoneNumber,
			ConnectedAt: session.ConnectedAt,
			LastSeen:    session.LastSeen,
		})
	}

	summary.AvailableSlots = summary.MaxDevices - summary.UsedDevices
	return summary, nil
}

// ============= WHATSAPP DEVICE STORE =============

func (dm *DatabaseManager) GetWhatsAppDevice(jid types.JID) (*store.Device, error) {
	device, err := dm.sqlDB.GetDevice(context.Background(), jid)
	if err != nil {
		return nil, err
	}
	return device, nil
}

func (dm *DatabaseManager) GetAllDevices() ([]*store.Device, error) {
	devices, err := dm.sqlDB.GetAllDevices(context.Background())
	if err != nil {
		return nil, err
	}
	return devices, nil
}

func (dm *DatabaseManager) GetFirstDevice() (*store.Device, error) {
	device, err := dm.sqlDB.GetFirstDevice(context.Background())
	if err != nil {
		return nil, err
	}
	return device, nil
}

func (dm *DatabaseManager) PutDevice(device *store.Device) error {
	return dm.sqlDB.PutDevice(context.Background(), device)
}

func (dm *DatabaseManager) DeleteDevice(device *store.Device) error {
	return dm.sqlDB.DeleteDevice(context.Background(), device)
}

func (dm *DatabaseManager) Close() error {
	sqlDB, _ := dm.db.DB()
	if sqlDB != nil {
		sqlDB.Close()
	}
	return nil
}
