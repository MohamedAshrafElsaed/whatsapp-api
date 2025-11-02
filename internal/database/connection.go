package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"whatsapp-api/internal/config"
)

// Connection represents a database connection
type Connection struct {
	DB    *gorm.DB
	SqlDB *sql.DB
}

// NewConnection creates a new database connection
func NewConnection(cfg *config.Config) (*Connection, error) {
	// Build connection string
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=UTC",
		cfg.Database.Host,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Name,
		cfg.Database.Port,
		cfg.Database.SSLMode,
	)

	// Configure GORM
	gormConfig := &gorm.Config{
		SkipDefaultTransaction: true,
		PrepareStmt:            true,
	}

	// Set log level based on environment
	if cfg.Server.Environment == "production" {
		gormConfig.Logger = logger.Default.LogMode(logger.Error)
	} else {
		gormConfig.Logger = logger.Default.LogMode(logger.Info)
	}

	// Open connection with GORM
	db, err := gorm.Open(postgres.Open(dsn), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Get underlying SQL DB
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	// Configure connection pool
	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.Database.ConnMaxLifetime) * time.Second)
	sqlDB.SetConnMaxIdleTime(time.Duration(cfg.Database.ConnMaxIdleTime) * time.Second)

	// Test connection
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Connection{
		DB:    db,
		SqlDB: sqlDB,
	}, nil
}

// Close closes the database connection
func (c *Connection) Close() error {
	if c.SqlDB != nil {
		return c.SqlDB.Close()
	}
	return nil
}

// HealthCheck performs a health check on the database
func (c *Connection) HealthCheck() error {
	if c.SqlDB == nil {
		return fmt.Errorf("database connection is nil")
	}
	return c.SqlDB.Ping()
}

// BeginTransaction starts a new transaction
func (c *Connection) BeginTransaction() *gorm.DB {
	return c.DB.Begin()
}

// RollbackTransaction rolls back a transaction
func RollbackTransaction(tx *gorm.DB) {
	tx.Rollback()
}

// CommitTransaction commits a transaction
func CommitTransaction(tx *gorm.DB) error {
	return tx.Commit().Error
}

// WithTransaction executes a function within a transaction
func (c *Connection) WithTransaction(fn func(*gorm.DB) error) error {
	tx := c.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}

// GetDB returns the GORM database instance
func (c *Connection) GetDB() *gorm.DB {
	return c.DB
}

// GetSQLDB returns the sql.DB instance
func (c *Connection) GetSQLDB() *sql.DB {
	return c.SqlDB
}

// IsConnected checks if the database is connected
func (c *Connection) IsConnected() bool {
	if c.SqlDB == nil {
		return false
	}
	err := c.SqlDB.Ping()
	return err == nil
}
