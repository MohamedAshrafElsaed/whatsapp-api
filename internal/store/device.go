// internal/store/device.go
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"

	"whatsapp-api/internal/config"
	"whatsapp-api/pkg/logger"
)

// Device represents a WhatsApp device store using PostgreSQL
type Device struct {
	*store.Device
	SessionID uuid.UUID
	UserID    int
	db        *sql.DB
	logger    *logger.Logger
	mu        sync.RWMutex
}

// NewPostgresDevice creates a new PostgreSQL-based device store
func NewPostgresDevice(cfg *config.Config, sessionID uuid.UUID, log *logger.Logger) (*Device, error) {
	// Create PostgreSQL connection string
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Name,
		cfg.Database.SSLMode,
	)

	// Open database connection
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Create SQLStore container
	container := sqlstore.NewWithDB(db, "postgres", log)

	// Initialize tables if needed
	if err := container.Upgrade(); err != nil {
		return nil, fmt.Errorf("failed to upgrade database: %w", err)
	}

	// Get device from store or create new one
	deviceStore, err := container.GetDevice(types.EmptyJID)
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	return &Device{
		Device:    deviceStore,
		SessionID: sessionID,
		db:        db,
		logger:    log,
	}, nil
}

// SaveDevice saves device information to custom tables
func (d *Device) SaveDevice() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Save to whatsmeow tables (already handled by base Device)
	if err := d.Device.Save(); err != nil {
		return fmt.Errorf("failed to save device to whatsmeow store: %w", err)
	}

	// Also save to our custom whatsapp_devices table for tracking
	query := `
		INSERT INTO whatsapp_devices (
			jid, session_id, user_id, 
			registration_id, platform, 
			business_name, push_name,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (jid) 
		DO UPDATE SET 
			session_id = EXCLUDED.session_id,
			registration_id = EXCLUDED.registration_id,
			platform = EXCLUDED.platform,
			business_name = EXCLUDED.business_name,
			push_name = EXCLUDED.push_name,
			updated_at = EXCLUDED.updated_at
	`

	var platform string
	if d.Platform != "" {
		platform = d.Platform.String()
	}

	_, err := d.db.Exec(
		query,
		d.ID.String(),
		d.SessionID,
		d.UserID,
		d.RegistrationID,
		platform,
		d.BusinessName,
		d.PushName,
		time.Now(),
		time.Now(),
	)

	if err != nil {
		return fmt.Errorf("failed to save device to custom table: %w", err)
	}

	return nil
}

// UpdateSession updates session information when device connects
func (d *Device) UpdateSession(status string, phoneNumber *string) error {
	query := `
		UPDATE whatsapp_sessions 
		SET 
			status = $1,
			phone_number = $2,
			jid = $3,
			push_name = $4,
			platform = $5,
			device_info = $6,
			connected_at = $7,
			last_seen = $8,
			updated_at = $9
		WHERE id = $10
	`

	deviceInfo := map[string]interface{}{
		"registration_id": d.RegistrationID,
		"platform":        d.Platform.String(),
		"business_name":   d.BusinessName,
		"push_name":       d.PushName,
	}

	deviceInfoJSON, _ := json.Marshal(deviceInfo)
	now := time.Now()

	_, err := d.db.Exec(
		query,
		status,
		phoneNumber,
		d.ID.String(),
		d.PushName,
		d.Platform.String(),
		deviceInfoJSON,
		now,
		now,
		now,
		d.SessionID,
	)

	return err
}

// GetAllDevices retrieves all devices for a user
func GetAllDevices(db *sql.DB, userID int) ([]*DeviceInfo, error) {
	query := `
		SELECT 
			d.jid, d.session_id, d.registration_id,
			d.platform, d.business_name, d.push_name,
			s.session_name, s.status, s.connected_at
		FROM whatsapp_devices d
		JOIN whatsapp_sessions s ON d.session_id = s.id
		WHERE d.user_id = $1 AND s.is_active = true
		ORDER BY s.created_at DESC
	`

	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []*DeviceInfo
	for rows.Next() {
		var dev DeviceInfo
		var platform sql.NullString
		var businessName sql.NullString
		var pushName sql.NullString
		var connectedAt sql.NullTime

		err := rows.Scan(
			&dev.JID,
			&dev.SessionID,
			&dev.RegistrationID,
			&platform,
			&businessName,
			&pushName,
			&dev.SessionName,
			&dev.Status,
			&connectedAt,
		)
		if err != nil {
			return nil, err
		}

		if platform.Valid {
			dev.Platform = platform.String
		}
		if businessName.Valid {
			dev.BusinessName = businessName.String
		}
		if pushName.Valid {
			dev.PushName = pushName.String
		}
		if connectedAt.Valid {
			dev.ConnectedAt = &connectedAt.Time
		}

		devices = append(devices, &dev)
	}

	return devices, nil
}

// DeviceInfo represents device information
type DeviceInfo struct {
	JID            string     `json:"jid"`
	SessionID      uuid.UUID  `json:"session_id"`
	SessionName    string     `json:"session_name"`
	RegistrationID uint32     `json:"registration_id"`
	Platform       string     `json:"platform"`
	BusinessName   string     `json:"business_name"`
	PushName       string     `json:"push_name"`
	Status         string     `json:"status"`
	ConnectedAt    *time.Time `json:"connected_at"`
}

// DeleteDevice removes device from database
func (d *Device) DeleteDevice() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Delete from whatsmeow tables
	if err := d.Device.Delete(); err != nil {
		return fmt.Errorf("failed to delete from whatsmeow store: %w", err)
	}

	// Delete from our custom table
	query := `DELETE FROM whatsapp_devices WHERE jid = $1`
	_, err := d.db.Exec(query, d.ID.String())
	if err != nil {
		return fmt.Errorf("failed to delete from custom table: %w", err)
	}

	return nil
}

// Close closes the database connection
func (d *Device) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// IsConnected checks if device is connected
func (d *Device) IsConnected() bool {
	return d.ID != nil && d.ID.String() != ""
}

// GetJID returns the device JID
func (d *Device) GetJID() string {
	if d.ID != nil {
		return d.ID.String()
	}
	return ""
}

// GetPhoneNumber extracts phone number from JID
func (d *Device) GetPhoneNumber() string {
	if d.ID != nil {
		return d.ID.User
	}
	return ""
}

// UpdateLastSeen updates the last seen timestamp
func (d *Device) UpdateLastSeen() error {
	query := `
		UPDATE whatsapp_sessions 
		SET last_seen = $1 
		WHERE id = $2
	`
	_, err := d.db.Exec(query, time.Now(), d.SessionID)
	return err
}

// LogEvent logs a device event
func (d *Device) LogEvent(eventType string, data map[string]interface{}) error {
	query := `
		INSERT INTO whatsapp_events (
			session_id, user_id, event_type, 
			event_data, created_at
		) VALUES ($1, $2, $3, $4, $5)
	`

	eventDataJSON, _ := json.Marshal(data)
	_, err := d.db.Exec(
		query,
		d.SessionID,
		d.UserID,
		eventType,
		eventDataJSON,
		time.Now(),
	)

	return err
}

// GetDeviceBySessionID retrieves device by session ID
func GetDeviceBySessionID(db *sql.DB, sessionID uuid.UUID) (*DeviceInfo, error) {
	query := `
		SELECT 
			d.jid, d.session_id, d.registration_id,
			d.platform, d.business_name, d.push_name,
			s.session_name, s.status, s.connected_at
		FROM whatsapp_devices d
		JOIN whatsapp_sessions s ON d.session_id = s.id
		WHERE d.session_id = $1
		LIMIT 1
	`

	var dev DeviceInfo
	var platform sql.NullString
	var businessName sql.NullString
	var pushName sql.NullString
	var connectedAt sql.NullTime

	err := db.QueryRow(query, sessionID).Scan(
		&dev.JID,
		&dev.SessionID,
		&dev.RegistrationID,
		&platform,
		&businessName,
		&pushName,
		&dev.SessionName,
		&dev.Status,
		&connectedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if platform.Valid {
		dev.Platform = platform.String
	}
	if businessName.Valid {
		dev.BusinessName = businessName.String
	}
	if pushName.Valid {
		dev.PushName = pushName.String
	}
	if connectedAt.Valid {
		dev.ConnectedAt = &connectedAt.Time
	}

	return &dev, nil
}
