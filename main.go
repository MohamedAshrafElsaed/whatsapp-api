package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

// ============= CONFIGURATION =============

type Config struct {
	// App
	AppPort string
	AppEnv  string

	// Database
	DBHost     string
	DBPort     string
	DBName     string
	DBUser     string
	DBPassword string

	// JWT
	JWTSecret string
	JWTIssuer string

	// WhatsApp
	AutoReconnect     bool
	QRTimeout         time.Duration
	MaxDevicesPerUser int

	// CORS
	CORSAllowedOrigins string

	// Group sync settings
	GroupSyncDelay         time.Duration
	GroupSyncRetryAttempts int
}

func LoadConfig() (*Config, error) {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	cfg := &Config{
		// App
		AppPort: getEnv("APP_PORT", "8080"),
		AppEnv:  getEnv("APP_ENV", "development"),

		// Database
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "3306"),
		DBName:     getEnv("DB_NAME", "whatsapp_api"),
		DBUser:     getEnv("DB_USER", "root"),
		DBPassword: getEnv("DB_PASSWORD", ""),

		// JWT
		JWTSecret: getEnv("JWT_SECRET", ""),
		JWTIssuer: getEnv("JWT_ISSUER", ""),

		// WhatsApp
		AutoReconnect:     getEnv("WA_AUTO_RECONNECT", "true") == "true",
		QRTimeout:         parseDuration(getEnv("WA_QR_TIMEOUT", "30s"), 30*time.Second),
		MaxDevicesPerUser: parseInt(getEnv("MAX_DEVICES_PER_USER", "5"), 5),

		// CORS
		CORSAllowedOrigins: getEnv("CORS_ALLOWED_ORIGINS", "*"),

		GroupSyncDelay:         parseDuration(getEnv("GROUP_SYNC_DELAY", "2s"), 2*time.Second),
		GroupSyncRetryAttempts: parseInt(getEnv("GROUP_SYNC_RETRY_ATTEMPTS", "3"), 3),
	}

	// Validate required fields
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	if cfg.DBPassword == "" && cfg.AppEnv == "production" {
		return nil, fmt.Errorf("DB_PASSWORD is required in production")
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseDuration(s string, defaultValue time.Duration) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultValue
	}
	return d
}

func parseInt(s string, defaultValue int) int {
	var value int
	if _, err := fmt.Sscanf(s, "%d", &value); err != nil {
		return defaultValue
	}
	return value
}

// ============= MAIN =============

// ============= UPDATE MAIN FUNCTION (Replace main() in main.go) =============

func main() {
	// Load configuration
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Step 1: Test connection to MySQL server
	fmt.Println("\n🔍 Step 1: Testing connection to MySQL server...")
	fmt.Println("   Connecting to MySQL database...")

	// Initialize database
	log.Println("Initializing database...")
	db, err := NewDatabaseManager(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize WebSocket manager
	wsManager := NewWebSocketManager()

	// Initialize WhatsApp service
	log.Println("Initializing WhatsApp service...")
	whatsappService := NewWhatsAppService(cfg, db, wsManager)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start session health monitor
	whatsappService.StartSessionMonitor(ctx)
	defer whatsappService.StopSessionMonitor()

	// Restore active sessions
	if err := whatsappService.RestoreActiveSessions(); err != nil {
		log.Printf("Failed to restore active sessions: %v", err)
	}

	// Initialize API handlers
	handlers := NewAPIHandlers(whatsappService, db, wsManager, cfg)

	// Setup Gin router
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Apply middleware
	router.Use(LoggerMiddleware())
	router.Use(ErrorMiddleware())
	router.Use(CORSMiddleware(cfg.CORSAllowedOrigins))

	// Health check (no auth required)
	router.GET("/health", handlers.HealthCheck)

	v1 := router.Group("/api/v1")
	{
		// Protected routes (require JWT auth)
		protected := v1.Group("/", AuthMiddleware(cfg.JWTSecret))
		{
			// Session management
			protected.POST("/sessions", handlers.CreateSession)
			protected.GET("/sessions", handlers.GetSessions)
			protected.GET("/sessions/:session_id/qr", handlers.GetSessionQR)
			protected.GET("/sessions/:session_id/status", handlers.GetSessionStatus)
			protected.DELETE("/sessions/:session_id", handlers.DeleteSession)

			// NEW: Manual session refresh
			protected.POST("/sessions/:session_id/refresh", handlers.RefreshSession)

			// Messaging
			protected.POST("/sessions/:session_id/send", handlers.SendMessage)
			protected.POST("/sessions/:session_id/send-advanced", handlers.SendMessageAdvanced)

			// Device summary
			protected.GET("/devices/summary", handlers.GetDeviceSummary)

			// Account validation
			protected.POST("/validate-account", handlers.ValidateAccount)
		}

		// WebSocket endpoint (uses token query param)
		v1.GET("/sessions/:session_id/events", handlers.HandleWebSocket)
	}

	// Start server
	srv := &http.Server{
		Addr:         ":" + cfg.AppPort,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		log.Printf("Starting server on port %s", cfg.AppPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Cancel context to stop background tasks
	cancel()

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop session monitor
	whatsappService.StopSessionMonitor()

	// Cleanup WhatsApp resources
	whatsappService.Cleanup()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server shutdown complete")
}
