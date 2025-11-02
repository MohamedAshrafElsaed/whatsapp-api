package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	Server    ServerConfig
	Database  DatabaseConfig
	JWT       JWTConfig
	WhatsApp  WhatsAppConfig
	WebSocket WebSocketConfig
	CORS      CORSConfig
	RateLimit RateLimitConfig
	Logging   LoggingConfig
	QRCode    QRCodeConfig
	Session   SessionConfig
	Upload    UploadConfig
	Redis     RedisConfig
	Metrics   MetricsConfig
	Webhook   WebhookConfig
}

// ServerConfig contains server-related configuration
type ServerConfig struct {
	Port     string
	Env      string
	Debug    bool
	BasePath string
}

// DatabaseConfig contains database connection configuration
type DatabaseConfig struct {
	Host            string
	Port            string
	Name            string
	User            string
	Password        string
	SSLMode         string
	MaxIdleConns    int
	MaxOpenConns    int
	ConnMaxLifetime time.Duration
}

// JWTConfig contains JWT authentication configuration
type JWTConfig struct {
	Secret   string
	Issuer   string
	Audience string
	Expiry   time.Duration
}

// WhatsAppConfig contains WhatsApp-specific configuration
type WhatsAppConfig struct {
	AutoReconnect     bool
	QRTimeout         time.Duration
	QRMaxRetries      int
	MaxDevicesPerUser int
}

// WebSocketConfig contains WebSocket configuration
type WebSocketConfig struct {
	PingInterval    time.Duration
	PongTimeout     time.Duration
	WriteTimeout    time.Duration
	ReadBufferSize  int
	WriteBufferSize int
}

// CORSConfig contains CORS configuration
type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	AllowCredentials bool
	MaxAge           int
}

// RateLimitConfig contains rate limiting configuration
type RateLimitConfig struct {
	Enabled           bool
	RequestsPerMinute int
	Burst             int
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level  string
	Format string
	Output string
}

// QRCodeConfig contains QR code generation configuration
type QRCodeConfig struct {
	Size          int
	RecoveryLevel string
}

// SessionConfig contains session management configuration
type SessionConfig struct {
	CleanupInterval        time.Duration
	InactiveTimeout        time.Duration
	AutoDisconnectOnLogout bool
}

// UploadConfig contains file upload configuration
type UploadConfig struct {
	MaxSize          int64
	AllowedFileTypes []string
}

// RedisConfig contains Redis configuration
type RedisConfig struct {
	Enabled  bool
	Host     string
	Port     string
	Password string
	DB       int
}

// MetricsConfig contains metrics configuration
type MetricsConfig struct {
	Enabled bool
	Port    string
}

// WebhookConfig contains webhook configuration
type WebhookConfig struct {
	Enabled       bool
	URL           string
	Secret        string
	RetryAttempts int
	Timeout       time.Duration
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	// Load .env file if exists (ignore error if not found)
	_ = godotenv.Load()

	cfg := &Config{
		Server: ServerConfig{
			Port:     getEnv("APP_PORT", "8080"),
			Env:      getEnv("APP_ENV", "production"),
			Debug:    getEnvBool("APP_DEBUG", false),
			BasePath: getEnv("APP_BASE_PATH", "/api/v1"),
		},
		Database: DatabaseConfig{
			Host:            getEnv("DB_HOST", "localhost"),
			Port:            getEnv("DB_PORT", "5432"),
			Name:            getEnv("DB_NAME", "whatsapp_api"),
			User:            getEnv("DB_USER", "postgres"),
			Password:        getEnv("DB_PASSWORD", ""),
			SSLMode:         getEnv("DB_SSL_MODE", "disable"),
			MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 10),
			MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 100),
			ConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 3600) * time.Second,
		},
		JWT: JWTConfig{
			Secret:   getEnv("JWT_SECRET", ""),
			Issuer:   getEnv("JWT_ISSUER", "your-laravel-app"),
			Audience: getEnv("JWT_AUDIENCE", "whatsapp-api"),
			Expiry:   getEnvDuration("JWT_EXPIRY", 3600) * time.Second,
		},
		WhatsApp: WhatsAppConfig{
			AutoReconnect:     getEnvBool("WA_AUTO_RECONNECT", true),
			QRTimeout:         getEnvDuration("WA_QR_TIMEOUT", 30) * time.Second,
			QRMaxRetries:      getEnvInt("WA_QR_MAX_RETRIES", 5),
			MaxDevicesPerUser: getEnvInt("MAX_DEVICES_PER_USER", 5),
		},
		WebSocket: WebSocketConfig{
			PingInterval:    getEnvDuration("WS_PING_INTERVAL", 30) * time.Second,
			PongTimeout:     getEnvDuration("WS_PONG_TIMEOUT", 10) * time.Second,
			WriteTimeout:    getEnvDuration("WS_WRITE_TIMEOUT", 10) * time.Second,
			ReadBufferSize:  getEnvInt("WS_READ_BUFFER_SIZE", 1024),
			WriteBufferSize: getEnvInt("WS_WRITE_BUFFER_SIZE", 1024),
		},
		CORS: CORSConfig{
			AllowedOrigins:   getEnvSlice("CORS_ALLOWED_ORIGINS", []string{"*"}),
			AllowedMethods:   getEnvSlice("CORS_ALLOWED_METHODS", []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
			AllowedHeaders:   getEnvSlice("CORS_ALLOWED_HEADERS", []string{"Content-Type", "Authorization"}),
			AllowCredentials: getEnvBool("CORS_ALLOW_CREDENTIALS", true),
			MaxAge:           getEnvInt("CORS_MAX_AGE", 43200),
		},
		RateLimit: RateLimitConfig{
			Enabled:           getEnvBool("RATE_LIMIT_ENABLED", true),
			RequestsPerMinute: getEnvInt("RATE_LIMIT_REQUESTS_PER_MINUTE", 60),
			Burst:             getEnvInt("RATE_LIMIT_BURST", 10),
		},
		Logging: LoggingConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "json"),
			Output: getEnv("LOG_OUTPUT", "stdout"),
		},
		QRCode: QRCodeConfig{
			Size:          getEnvInt("QR_CODE_SIZE", 256),
			RecoveryLevel: getEnv("QR_CODE_RECOVERY_LEVEL", "medium"),
		},
		Session: SessionConfig{
			CleanupInterval:        getEnvDuration("SESSION_CLEANUP_INTERVAL", 3600) * time.Second,
			InactiveTimeout:        getEnvDuration("SESSION_INACTIVE_TIMEOUT", 86400) * time.Second,
			AutoDisconnectOnLogout: getEnvBool("SESSION_AUTO_DISCONNECT_ON_LOGOUT", true),
		},
		Upload: UploadConfig{
			MaxSize:          getEnvInt64("MAX_UPLOAD_SIZE", 16777216), // 16MB
			AllowedFileTypes: getEnvSlice("ALLOWED_FILE_TYPES", []string{"image/jpeg", "image/png"}),
		},
		Redis: RedisConfig{
			Enabled:  getEnvBool("REDIS_ENABLED", false),
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		Metrics: MetricsConfig{
			Enabled: getEnvBool("METRICS_ENABLED", false),
			Port:    getEnv("METRICS_PORT", "9090"),
		},
		Webhook: WebhookConfig{
			Enabled:       getEnvBool("WEBHOOK_ENABLED", false),
			URL:           getEnv("WEBHOOK_URL", ""),
			Secret:        getEnv("WEBHOOK_SECRET", ""),
			RetryAttempts: getEnvInt("WEBHOOK_RETRY_ATTEMPTS", 3),
			Timeout:       getEnvDuration("WEBHOOK_TIMEOUT", 30) * time.Second,
		},
	}

	// Validate required configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Database.Password == "" {
		return fmt.Errorf("DB_PASSWORD is required")
	}

	if c.JWT.Secret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}

	if c.Server.Port == "" {
		return fmt.Errorf("APP_PORT is required")
	}

	if c.WhatsApp.MaxDevicesPerUser < 1 || c.WhatsApp.MaxDevicesPerUser > 10 {
		return fmt.Errorf("MAX_DEVICES_PER_USER must be between 1 and 10")
	}

	return nil
}

// GetDSN returns the database connection string
func (c *DatabaseConfig) GetDSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode,
	)
}

// Helper functions to get environment variables with defaults

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue int) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return time.Duration(parsed)
		}
	}
	return time.Duration(defaultValue)
}

func getEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return defaultValue
}

// IsDevelopment returns true if the environment is development
func (c *Config) IsDevelopment() bool {
	return c.Server.Env == "development" || c.Server.Env == "dev"
}

// IsProduction returns true if the environment is production
func (c *Config) IsProduction() bool {
	return c.Server.Env == "production" || c.Server.Env == "prod"
}

// GetServerAddress returns the full server address
func (c *Config) GetServerAddress() string {
	return ":" + c.Server.Port
}
