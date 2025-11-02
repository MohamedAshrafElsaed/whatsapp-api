// internal/database/migrations.go
package database

import (
	"database/sql"
	"fmt"
)

// RunMigrations runs all database migrations
func RunMigrations(db *sql.DB) error {
	// Enable UUID extension
	if err := enableExtensions(db); err != nil {
		return fmt.Errorf("failed to enable extensions: %w", err)
	}

	// Create tables
	if err := createTables(db); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	// Create indexes
	if err := createIndexes(db); err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	// Create functions and triggers
	if err := createFunctionsAndTriggers(db); err != nil {
		return fmt.Errorf("failed to create functions and triggers: %w", err)
	}

	// Create views
	if err := createViews(db); err != nil {
		return fmt.Errorf("failed to create views: %w", err)
	}

	return nil
}

func enableExtensions(db *sql.DB) error {
	query := `CREATE EXTENSION IF NOT EXISTS "uuid-ossp";`
	_, err := db.Exec(query)
	return err
}

func createTables(db *sql.DB) error {
	queries := []string{
		// WhatsApp sessions table
		`CREATE TABLE IF NOT EXISTS whatsapp_sessions (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			user_id INTEGER NOT NULL,
			session_name VARCHAR(255) NOT NULL,
			phone_number VARCHAR(20),
			jid VARCHAR(255) UNIQUE,
			status VARCHAR(50) NOT NULL DEFAULT 'pending',
			qr_code TEXT,
			qr_code_base64 TEXT,
			qr_generated_at TIMESTAMP,
			qr_expires_at TIMESTAMP,
			qr_retry_count INTEGER DEFAULT 0,
			connected_at TIMESTAMP,
			disconnected_at TIMESTAMP,
			last_seen TIMESTAMP,
			device_info JSONB,
			push_name VARCHAR(255),
			platform VARCHAR(50),
			is_active BOOLEAN DEFAULT true,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW(),
			deleted_at TIMESTAMP,
			CONSTRAINT unique_user_session UNIQUE(user_id, session_name),
			CONSTRAINT check_status CHECK (status IN ('pending', 'qr_ready', 'scanning', 'connected', 'disconnected', 'failed', 'expired'))
		)`,

		// Device store table
		`CREATE TABLE IF NOT EXISTS whatsapp_devices (
			jid VARCHAR(255) PRIMARY KEY,
			session_id UUID REFERENCES whatsapp_sessions(id) ON DELETE CASCADE,
			user_id INTEGER NOT NULL,
			registration_id INTEGER,
			adv_secret_key BYTEA,
			next_pre_key_id INTEGER,
			first_unuploaded_pre_key_id INTEGER,
			account_signature_key BYTEA,
			account_signature BYTEA,
			device_signature_key BYTEA,
			device_signature BYTEA,
			identity_key BYTEA,
			noise_key BYTEA,
			platform VARCHAR(50),
			business_name VARCHAR(255),
			push_name VARCHAR(255),
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW()
		)`,

		// Event logs table
		`CREATE TABLE IF NOT EXISTS whatsapp_events (
			id BIGSERIAL PRIMARY KEY,
			session_id UUID REFERENCES whatsapp_sessions(id) ON DELETE CASCADE,
			user_id INTEGER NOT NULL,
			event_type VARCHAR(100) NOT NULL,
			event_data JSONB,
			ip_address INET,
			user_agent TEXT,
			created_at TIMESTAMP DEFAULT NOW()
		)`,

		// Messages table
		`CREATE TABLE IF NOT EXISTS messages (
			id BIGSERIAL PRIMARY KEY,
			session_id UUID REFERENCES whatsapp_sessions(id) ON DELETE CASCADE,
			message_id VARCHAR(255) NOT NULL,
			to_jid VARCHAR(255) NOT NULL,
			from_jid VARCHAR(255) NOT NULL,
			message_type VARCHAR(50) NOT NULL,
			content TEXT,
			media_url TEXT,
			media_type VARCHAR(50),
			caption TEXT,
			file_name VARCHAR(255),
			status VARCHAR(50) DEFAULT 'pending',
			error_message TEXT,
			sent_at TIMESTAMP,
			delivered_at TIMESTAMP,
			read_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW(),
			UNIQUE(session_id, message_id)
		)`,

		// Groups table
		`CREATE TABLE IF NOT EXISTS groups (
			id BIGSERIAL PRIMARY KEY,
			session_id UUID REFERENCES whatsapp_sessions(id) ON DELETE CASCADE,
			group_jid VARCHAR(255) NOT NULL,
			name VARCHAR(255) NOT NULL,
			topic TEXT,
			owner_jid VARCHAR(255),
			created_at_wa TIMESTAMP,
			participant_count INTEGER DEFAULT 0,
			is_announce_only BOOLEAN DEFAULT false,
			is_locked BOOLEAN DEFAULT false,
			is_ephemeral BOOLEAN DEFAULT false,
			invite_link TEXT,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW(),
			UNIQUE(session_id, group_jid)
		)`,

		// Group participants table
		`CREATE TABLE IF NOT EXISTS group_participants (
			id BIGSERIAL PRIMARY KEY,
			group_id BIGINT REFERENCES groups(id) ON DELETE CASCADE,
			group_jid VARCHAR(255) NOT NULL,
			user_jid VARCHAR(255) NOT NULL,
			is_admin BOOLEAN DEFAULT false,
			is_super_admin BOOLEAN DEFAULT false,
			joined_at TIMESTAMP DEFAULT NOW(),
			UNIQUE(group_id, user_jid)
		)`,

		// Contacts table
		`CREATE TABLE IF NOT EXISTS contacts (
			id BIGSERIAL PRIMARY KEY,
			session_id UUID REFERENCES whatsapp_sessions(id) ON DELETE CASCADE,
			jid VARCHAR(255) NOT NULL,
			name VARCHAR(255),
			business_name VARCHAR(255),
			push_name VARCHAR(255),
			phone_number VARCHAR(20),
			about TEXT,
			profile_picture TEXT,
			is_blocked BOOLEAN DEFAULT false,
			is_enterprise BOOLEAN DEFAULT false,
			last_seen TIMESTAMP,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW(),
			UNIQUE(session_id, jid)
		)`,

		// Status/Stories table
		`CREATE TABLE IF NOT EXISTS statuses (
			id BIGSERIAL PRIMARY KEY,
			session_id UUID REFERENCES whatsapp_sessions(id) ON DELETE CASCADE,
			status_id VARCHAR(255) NOT NULL,
			type VARCHAR(50) NOT NULL,
			text TEXT,
			caption TEXT,
			media_url TEXT,
			contact_jid VARCHAR(255),
			contact_name VARCHAR(255),
			privacy VARCHAR(50) DEFAULT 'all_contacts',
			recipients TEXT[],
			view_count INTEGER DEFAULT 0,
			expires_at TIMESTAMP,
			posted_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT NOW(),
			UNIQUE(session_id, status_id)
		)`,

		// Status viewers table
		`CREATE TABLE IF NOT EXISTS status_viewers (
			id BIGSERIAL PRIMARY KEY,
			status_id VARCHAR(255) NOT NULL,
			viewer_jid VARCHAR(255) NOT NULL,
			viewer_name VARCHAR(255),
			viewed_at TIMESTAMP DEFAULT NOW(),
			UNIQUE(status_id, viewer_jid)
		)`,

		// Status mute settings
		`CREATE TABLE IF NOT EXISTS status_mutes (
			id BIGSERIAL PRIMARY KEY,
			session_id UUID REFERENCES whatsapp_sessions(id) ON DELETE CASCADE,
			contact_jid VARCHAR(255) NOT NULL,
			is_muted BOOLEAN DEFAULT true,
			created_at TIMESTAMP DEFAULT NOW(),
			UNIQUE(session_id, contact_jid)
		)`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query: %w\nQuery: %s", err, query)
		}
	}

	return nil
}

func createIndexes(db *sql.DB) error {
	indexes := []string{
		// Sessions indexes
		`CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON whatsapp_sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_status ON whatsapp_sessions(status)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_active ON whatsapp_sessions(user_id, is_active) WHERE is_active = true`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_connected ON whatsapp_sessions(user_id, status) WHERE status = 'connected'`,

		// Devices indexes
		`CREATE INDEX IF NOT EXISTS idx_devices_user_id ON whatsapp_devices(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_devices_session_id ON whatsapp_devices(session_id)`,

		// Events indexes
		`CREATE INDEX IF NOT EXISTS idx_events_session_id ON whatsapp_events(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_events_user_id ON whatsapp_events(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_events_created_at ON whatsapp_events(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_events_type ON whatsapp_events(event_type)`,

		// Messages indexes
		`CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_to_from ON messages(to_jid, from_jid)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_status ON messages(status)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at)`,

		// Groups indexes
		`CREATE INDEX IF NOT EXISTS idx_groups_session_id ON groups(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_groups_jid ON groups(group_jid)`,

		// Group participants indexes
		`CREATE INDEX IF NOT EXISTS idx_group_participants_group_id ON group_participants(group_id)`,
		`CREATE INDEX IF NOT EXISTS idx_group_participants_user_jid ON group_participants(user_jid)`,

		// Contacts indexes
		`CREATE INDEX IF NOT EXISTS idx_contacts_session_id ON contacts(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_jid ON contacts(jid)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_blocked ON contacts(session_id, is_blocked) WHERE is_blocked = true`,

		// Status indexes
		`CREATE INDEX IF NOT EXISTS idx_statuses_session_id ON statuses(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_statuses_expires_at ON statuses(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_status_viewers_status_id ON status_viewers(status_id)`,
		`CREATE INDEX IF NOT EXISTS idx_status_mutes_session_id ON status_mutes(session_id)`,
	}

	for _, index := range indexes {
		if _, err := db.Exec(index); err != nil {
			return fmt.Errorf("failed to create index: %w\nIndex: %s", err, index)
		}
	}

	return nil
}

func createFunctionsAndTriggers(db *sql.DB) error {
	// Function to check device limit
	checkDeviceLimitFunc := `
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
			AND id != NEW.id;
		
		IF active_count >= max_allowed THEN
			RAISE EXCEPTION 'Device limit exceeded. Maximum % devices allowed per user.', max_allowed;
		END IF;
		
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql;`

	// Function to update updated_at timestamp
	updateTimestampFunc := `
	CREATE OR REPLACE FUNCTION update_updated_at_column()
	RETURNS TRIGGER AS $$
	BEGIN
		NEW.updated_at = NOW();
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql;`

	// Execute function creation
	if _, err := db.Exec(checkDeviceLimitFunc); err != nil {
		return fmt.Errorf("failed to create check_device_limit function: %w", err)
	}

	if _, err := db.Exec(updateTimestampFunc); err != nil {
		return fmt.Errorf("failed to create update_updated_at_column function: %w", err)
	}

	// Create triggers
	triggers := []string{
		// Trigger to enforce device limit
		`DROP TRIGGER IF EXISTS enforce_device_limit ON whatsapp_sessions`,
		`CREATE TRIGGER enforce_device_limit
			BEFORE INSERT OR UPDATE ON whatsapp_sessions
			FOR EACH ROW
			WHEN (NEW.status IN ('pending', 'qr_ready', 'scanning', 'connected') AND NEW.is_active = true)
			EXECUTE FUNCTION check_device_limit()`,

		// Triggers to update updated_at
		`DROP TRIGGER IF EXISTS update_sessions_updated_at ON whatsapp_sessions`,
		`CREATE TRIGGER update_sessions_updated_at
			BEFORE UPDATE ON whatsapp_sessions
			FOR EACH ROW
			EXECUTE FUNCTION update_updated_at_column()`,

		`DROP TRIGGER IF EXISTS update_devices_updated_at ON whatsapp_devices`,
		`CREATE TRIGGER update_devices_updated_at
			BEFORE UPDATE ON whatsapp_devices
			FOR EACH ROW
			EXECUTE FUNCTION update_updated_at_column()`,

		`DROP TRIGGER IF EXISTS update_messages_updated_at ON messages`,
		`CREATE TRIGGER update_messages_updated_at
			BEFORE UPDATE ON messages
			FOR EACH ROW
			EXECUTE FUNCTION update_updated_at_column()`,

		`DROP TRIGGER IF EXISTS update_groups_updated_at ON groups`,
		`CREATE TRIGGER update_groups_updated_at
			BEFORE UPDATE ON groups
			FOR EACH ROW
			EXECUTE FUNCTION update_updated_at_column()`,

		`DROP TRIGGER IF EXISTS update_contacts_updated_at ON contacts`,
		`CREATE TRIGGER update_contacts_updated_at
			BEFORE UPDATE ON contacts
			FOR EACH ROW
			EXECUTE FUNCTION update_updated_at_column()`,
	}

	for _, trigger := range triggers {
		if _, err := db.Exec(trigger); err != nil {
			return fmt.Errorf("failed to create trigger: %w\nTrigger: %s", err, trigger)
		}
	}

	return nil
}

func createViews(db *sql.DB) error {
	// Create user device summary view
	view := `
	CREATE OR REPLACE VIEW user_device_summary AS
	SELECT 
		user_id,
		COUNT(*) FILTER (WHERE status = 'connected' AND is_active = true) as connected_devices,
		COUNT(*) FILTER (WHERE status IN ('pending', 'qr_ready', 'scanning') AND is_active = true) as pending_devices,
		COUNT(*) FILTER (WHERE is_active = true) as total_active_sessions,
		MAX(last_seen) as last_activity
	FROM whatsapp_sessions
	GROUP BY user_id;`

	if _, err := db.Exec(view); err != nil {
		return fmt.Errorf("failed to create view: %w", err)
	}

	return nil
}

// DropAllTables drops all tables (use with caution!)
func DropAllTables(db *sql.DB) error {
	tables := []string{
		"status_mutes",
		"status_viewers",
		"statuses",
		"contacts",
		"group_participants",
		"groups",
		"messages",
		"whatsapp_events",
		"whatsapp_devices",
		"whatsapp_sessions",
	}

	for _, table := range tables {
		query := fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", table)
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to drop table %s: %w", table, err)
		}
	}

	return nil
}
