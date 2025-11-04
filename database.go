package main

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm/clause"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	_ "modernc.org/sqlite" // Pure Go SQLite driver (no CGO required)
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
	ID                string         `gorm:"type:char(36);primaryKey" json:"id"`
	UserID            int            `gorm:"not null;index;uniqueIndex:idx_user_session" json:"user_id"`
	SessionName       string         `gorm:"size:255;not null;uniqueIndex:idx_user_session" json:"session_name"`
	PhoneNumber       *string        `gorm:"size:20" json:"phone_number,omitempty"`
	JID               *string        `gorm:"column:j_id;size:255;uniqueIndex" json:"jid,omitempty"`
	Status            SessionStatus  `gorm:"size:50;not null;default:'pending';index" json:"status"`
	QRCode            *string        `gorm:"type:text" json:"-"`
	QRCodeBase64      *string        `gorm:"type:text" json:"qr_code_base64,omitempty"`
	QRGeneratedAt     *time.Time     `json:"qr_generated_at,omitempty"`
	QRExpiresAt       *time.Time     `json:"qr_expires_at,omitempty"`
	QRRetryCount      int            `gorm:"default:0" json:"qr_retry_count"`
	ConnectedAt       *time.Time     `json:"connected_at,omitempty"`
	DisconnectedAt    *time.Time     `json:"disconnected_at,omitempty"`
	LastSeen          *time.Time     `json:"last_seen,omitempty"`
	DeviceInfo        JSONData       `gorm:"type:json" json:"device_info,omitempty"`
	PushName          *string        `gorm:"size:255" json:"push_name,omitempty"`
	Platform          *string        `gorm:"size:50" json:"platform,omitempty"`
	IsActive          bool           `gorm:"default:true;index" json:"is_active"`
	IsBusinessAccount bool           `gorm:"default:false" json:"is_business_account"` // NEW FIELD
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

// WhatsAppContact represents a contact
type WhatsAppContact struct {
	ID            int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID        int       `gorm:"not null;index:idx_user_jid,unique" json:"user_id"`
	FullName      string    `gorm:"size:255" json:"full_name"`
	FirstName     string    `gorm:"size:100" json:"first_name"`
	LastName      string    `gorm:"size:155" json:"last_name"`
	JID           string    `gorm:"column:jid;size:255;not null;index:idx_user_jid,unique" json:"jid"`
	CountryCode   string    `gorm:"size:10" json:"country_code"`
	MobileNumber  string    `gorm:"size:50" json:"mobile_number"`
	GroupID       *int64    `gorm:"index" json:"group_id,omitempty"`      // NEW FIELD
	IsGroupMember bool      `gorm:"default:false" json:"is_group_member"` // NEW FIELD
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type WhatsAppGroup struct {
	ID               int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID           int       `gorm:"not null;index:idx_user_group,unique" json:"user_id"`
	SessionID        string    `gorm:"type:char(36);index" json:"session_id"`
	GroupJID         string    `gorm:"column:group_jid;size:255;not null;index:idx_user_group,unique" json:"group_jid"`
	GroupName        string    `gorm:"size:255" json:"group_name"`
	GroupSubject     *string   `gorm:"type:text" json:"group_subject,omitempty"`
	ParticipantCount int       `gorm:"default:0" json:"participant_count"`
	IsAnnouncement   bool      `gorm:"default:false" json:"is_announcement"`
	IsLocked         bool      `gorm:"default:false" json:"is_locked"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// BeforeCreate hook to generate UUID
func (s *WhatsAppSession) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	return nil
}

// WhatsAppEvent represents an event log
type WhatsAppEvent struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID string    `gorm:"type:char(36);index" json:"session_id"`
	UserID    int       `gorm:"not null;index" json:"user_id"`
	EventType string    `gorm:"size:100;not null;index" json:"event_type"`
	EventData JSONData  `gorm:"type:json" json:"event_data"`
	CreatedAt time.Time `json:"created_at"`
}

// JSONData type for MySQL JSON fields
type JSONData map[string]interface{}

func (j JSONData) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

func (j *JSONData) Scan(value interface{}) error {
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
		return errors.New("unsupported type for JSONData")
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
	// ========================================
	// Part 1: MySQL for Application Data
	// ========================================
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName)

	log.Printf("📊 Connecting to MySQL database...")
	log.Printf("   Host: %s:%s", cfg.DBHost, cfg.DBPort)
	log.Printf("   Database: %s", cfg.DBName)

	// GORM connection for application data
	gormDB, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
		PrepareStmt: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MySQL: %w", err)
	}

	// Configure connection pool
	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(50)
	sqlDB.SetMaxOpenConns(200)
	sqlDB.SetConnMaxLifetime(time.Hour)

	log.Println("   ✅ MySQL connected successfully")

	// ========================================
	// Part 2: SQLite for WhatsApp Store
	// ========================================
	log.Println("📱 Setting up WhatsApp store (SQLite)...")

	// Create data directory if it doesn't exist
	dataDir := "./data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// SQLite database path
	sqlitePath := filepath.Join(dataDir, "whatsapp_store.db")
	log.Printf("   Store location: %s", sqlitePath)

	// Create WhatsApp store container with SQLite
	container, err := sqlstore.New(context.Background(), "sqlite", sqlitePath+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create WhatsApp store: %w", err)
	}

	log.Println("   ✅ WhatsApp store initialized")

	dm := &DatabaseManager{
		db:          gormDB,
		sqlDB:       container,
		waContainer: container,
	}

	// Run migrations
	log.Println("🔧 Running database migrations...")
	if err := dm.Migrate(); err != nil {
		return nil, err
	}

	return dm, nil
}

// Migrate creates all necessary tables and constraints
// Replace the existing Migrate() function with this updated version:
func (dm *DatabaseManager) Migrate() error {
	// Auto migrate models - ADD WhatsAppGroup to the list
	if err := dm.db.AutoMigrate(&WhatsAppSession{}, &WhatsAppEvent{}, &WhatsAppContact{}, &WhatsAppGroup{}); err != nil {
		return err
	}

	// Add new columns to existing tables
	// Check if is_business_account exists, if not add it
	if !dm.db.Migrator().HasColumn(&WhatsAppSession{}, "is_business_account") {
		if err := dm.db.Migrator().AddColumn(&WhatsAppSession{}, "is_business_account"); err != nil {
			log.Printf("Warning: Failed to add is_business_account column: %v", err)
		}
	}

	// Check if group_id exists in contacts, if not add it
	if !dm.db.Migrator().HasColumn(&WhatsAppContact{}, "group_id") {
		if err := dm.db.Migrator().AddColumn(&WhatsAppContact{}, "group_id"); err != nil {
			log.Printf("Warning: Failed to add group_id column: %v", err)
		}
	}

	// Check if is_group_member exists, if not add it
	if !dm.db.Migrator().HasColumn(&WhatsAppContact{}, "is_group_member") {
		if err := dm.db.Migrator().AddColumn(&WhatsAppContact{}, "is_group_member"); err != nil {
			log.Printf("Warning: Failed to add is_group_member column: %v", err)
		}
	}

	// Create stored procedure for device limit check
	dm.db.Exec(`DROP PROCEDURE IF EXISTS check_device_limit;`)

	dm.db.Exec(`
		CREATE PROCEDURE check_device_limit(IN p_user_id INT, IN p_session_id CHAR(36))
		BEGIN
			DECLARE active_count INT;
			DECLARE max_allowed INT DEFAULT 5;
			
			SELECT COUNT(*) INTO active_count
			FROM whats_app_sessions
			WHERE user_id = p_user_id
				AND is_active = true
				AND status IN ('connected', 'pending', 'qr_ready', 'scanning')
				AND id != p_session_id
				AND deleted_at IS NULL;
			
			IF active_count >= max_allowed THEN
				SIGNAL SQLSTATE '45000' 
				SET MESSAGE_TEXT = 'Device limit exceeded. Maximum 5 devices allowed per user.';
			END IF;
		END;
	`)

	// Create trigger for INSERT
	dm.db.Exec(`DROP TRIGGER IF EXISTS enforce_device_limit_insert;`)

	dm.db.Exec(`
		CREATE TRIGGER enforce_device_limit_insert
		BEFORE INSERT ON whats_app_sessions
		FOR EACH ROW
		BEGIN
			IF NEW.status IN ('pending', 'qr_ready', 'scanning', 'connected') AND NEW.is_active = true THEN
				CALL check_device_limit(NEW.user_id, NEW.id);
			END IF;
		END;
	`)

	// Create trigger for UPDATE
	dm.db.Exec(`DROP TRIGGER IF EXISTS enforce_device_limit_update;`)

	dm.db.Exec(`
		CREATE TRIGGER enforce_device_limit_update
		BEFORE UPDATE ON whats_app_sessions
		FOR EACH ROW
		BEGIN
			IF NEW.status IN ('pending', 'qr_ready', 'scanning', 'connected') AND NEW.is_active = true THEN
				CALL check_device_limit(NEW.user_id, NEW.id);
			END IF;
		END;
	`)

	// Create indexes
	dm.db.Exec("CREATE INDEX IF NOT EXISTS idx_sessions_user_status ON whats_app_sessions(user_id, status)")
	dm.db.Exec("CREATE INDEX IF NOT EXISTS idx_events_session_created ON whats_app_events(session_id, created_at DESC)")
	dm.db.Exec("CREATE INDEX IF NOT EXISTS idx_groups_session ON whats_app_groups(session_id)")
	dm.db.Exec("CREATE INDEX IF NOT EXISTS idx_contacts_group ON whats_app_contacts(group_id)")

	log.Println("   ✅ Migrations completed")
	return nil
}

// ============= SESSION REPOSITORY =============

func (dm *DatabaseManager) CreateSession(userID int, sessionName string) (*WhatsAppSession, error) {
	sessionID := uuid.New()
	session := &WhatsAppSession{
		ID:          sessionID.String(),
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
	err := dm.db.Where("id = ? AND user_id = ?", sessionID.String(), userID).First(&session).Error
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
		Where("id = ?", sessionID.String()).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": time.Now(),
		}).Error
}

func (dm *DatabaseManager) DeleteSession(sessionID uuid.UUID, userID int) error {
	return dm.db.Where("id = ? AND user_id = ?", sessionID.String(), userID).
		Delete(&WhatsAppSession{}).Error
}

func (dm *DatabaseManager) SetSessionConnected(sessionID uuid.UUID, jid, phoneNumber, pushName, platform string) error {
	now := time.Now()

	updates := map[string]interface{}{
		"status":          StatusConnected,
		"j_id":            jid,
		"phone_number":    phoneNumber,
		"push_name":       pushName,
		"platform":        platform,
		"connected_at":    now,
		"last_seen":       now,
		"disconnected_at": nil,
		"qr_code":         nil,
		"qr_code_base64":  nil,
	}

	result := dm.db.Model(&WhatsAppSession{}).
		Where("id = ?", sessionID.String()).
		Select("*"). // ← Add this to force update all fields
		Updates(updates)

	if result.Error != nil {
		log.Printf("❌ Failed to update session %s: %v", sessionID.String(), result.Error)
		return result.Error
	}

	if result.RowsAffected == 0 {
		log.Printf("⚠️ No rows updated for session %s - record not found?", sessionID.String())
		return fmt.Errorf("session not found: %s", sessionID.String())
	}

	log.Printf("✅ Successfully updated session %s in database (rows affected: %d)", sessionID.String(), result.RowsAffected)
	return nil
}

func (dm *DatabaseManager) SetSessionDisconnected(sessionID uuid.UUID) error {
	now := time.Now()
	return dm.db.Model(&WhatsAppSession{}).
		Where("id = ?", sessionID.String()).
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
		Where("id = ?", sessionID.String()).
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
		SessionID: sessionID.String(),
		UserID:    userID,
		EventType: eventType,
		EventData: data,
		CreatedAt: time.Now(),
	}
	return dm.db.Create(event).Error
}

func (dm *DatabaseManager) GetSessionEvents(sessionID uuid.UUID, limit int) ([]WhatsAppEvent, error) {
	var events []WhatsAppEvent
	query := dm.db.Where("session_id = ?", sessionID.String()).Order("created_at DESC")
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

		// Parse UUID from string
		sessionUUID, _ := uuid.Parse(session.ID)
		summary.Sessions = append(summary.Sessions, SessionSummary{
			ID:          sessionUUID,
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

// ============= CONTACT REPOSITORY =============

func (dm *DatabaseManager) UpsertContact(contact *WhatsAppContact) error {
	return dm.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "jid"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"full_name", "first_name", "last_name",
			"country_code", "mobile_number",
			"group_id", "is_group_member", "updated_at",
		}),
	}).Create(contact).Error
}

func (dm *DatabaseManager) BulkUpsertContacts(contacts []WhatsAppContact) error {
	if len(contacts) == 0 {
		return nil
	}
	return dm.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "jid"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"full_name", "first_name", "last_name",
			"country_code", "mobile_number",
			"group_id", "is_group_member", "updated_at",
		}),
	}).Create(&contacts).Error
}

func (dm *DatabaseManager) GetUserContacts(userID int) ([]WhatsAppContact, error) {
	var contacts []WhatsAppContact
	err := dm.db.Where("user_id = ?", userID).
		Order("full_name ASC").
		Find(&contacts).Error
	return contacts, err
}

// ============= GROUP REPOSITORY (Add at the end of database.go) =============

func (dm *DatabaseManager) UpsertGroup(group *WhatsAppGroup) error {
	return dm.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "group_jid"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"session_id",
			"group_name",
			"group_subject",
			"participant_count",
			"is_announcement",
			"is_locked",
			"updated_at",
		}),
	}).Create(group).Error // ✅ CORRECT - updates on conflict
}

func (dm *DatabaseManager) GetUserGroups(userID int) ([]WhatsAppGroup, error) {
	var groups []WhatsAppGroup
	err := dm.db.Where("user_id = ?", userID).
		Order("group_name ASC").
		Find(&groups).Error
	return groups, err
}

func (dm *DatabaseManager) GetGroupByJID(userID int, groupJID string) (*WhatsAppGroup, error) {
	var group WhatsAppGroup
	err := dm.db.Where("user_id = ? AND group_jid = ?", userID, groupJID).
		First(&group).Error
	if err != nil {
		return nil, err
	}
	return &group, nil
}

func (dm *DatabaseManager) UpdateSessionBusinessAccount(sessionID uuid.UUID, isBusiness bool) error {
	return dm.db.Model(&WhatsAppSession{}).
		Where("id = ?", sessionID.String()).
		Update("is_business_account", isBusiness).Error
}
