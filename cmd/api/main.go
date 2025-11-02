// cmd/api/main.go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"whatsapp-api/internal/config"
	"whatsapp-api/internal/database"
	"whatsapp-api/internal/handlers"
	"whatsapp-api/internal/middleware"
	"whatsapp-api/internal/repositories"
	"whatsapp-api/internal/services"
	"whatsapp-api/internal/websocket"
	"whatsapp-api/pkg/logger"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		fmt.Printf("Warning: .env file not found: %v\n", err)
	}

	// Initialize logger
	log := logger.New(os.Getenv("LOG_LEVEL"))
	log.Info("Starting WhatsApp API Server...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration: %v", err)
	}

	// Initialize database connection
	db, err := database.NewConnection(cfg)
	if err != nil {
		log.Fatal("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := database.RunMigrations(db.DB); err != nil {
		log.Fatal("Failed to run migrations: %v", err)
	}

	// Initialize repositories
	sessionRepo := repositories.NewSessionRepository(db, log)
	deviceRepo := repositories.NewDeviceRepository(db, log)
	eventRepo := repositories.NewEventRepository(db, log)
	messageRepo := repositories.NewMessageRepository(db, log)
	groupRepo := repositories.NewGroupRepository(db, log)
	contactRepo := repositories.NewContactRepository(db, log)
	statusRepo := repositories.NewStatusRepository(db, log)

	// Initialize WebSocket manager
	wsManager := websocket.NewManager(cfg, log)
	go wsManager.Run()

	// Initialize services
	sessionManager := services.NewSessionManager(
		cfg,
		sessionRepo,
		deviceRepo,
		eventRepo,
		wsManager,
		log,
	)

	// Start session manager
	ctx := context.Background()
	if err := sessionManager.Start(ctx); err != nil {
		log.Error("Failed to start session manager: %v", err)
	}

	messageService := services.NewMessageService(
		sessionManager,
		messageRepo,
		eventRepo,
		log,
	)

	groupService := services.NewGroupService(
		sessionManager,
		groupRepo,
		eventRepo,
		log,
	)

	contactService := services.NewContactService(
		sessionManager,
		contactRepo,
		eventRepo,
		log,
	)

	statusService := services.NewStatusService(
		sessionManager,
		statusRepo,
		eventRepo,
		log,
	)

	// Initialize handlers
	sessionHandler := handlers.NewSessionHandler(
		sessionManager,
		sessionRepo,
		eventRepo,
		wsManager,
		log,
	)

	messageHandler := handlers.NewMessageHandler(
		messageService,
		log,
	)

	groupHandler := handlers.NewGroupHandler(
		groupService,
		log,
	)

	contactHandler := handlers.NewContactHandler(
		contactService,
		log,
	)

	statusHandler := handlers.NewStatusHandler(
		statusService,
		log,
	)

	deviceHandler := handlers.NewDeviceHandler(
		sessionRepo,
		deviceRepo,
		log,
	)

	websocketHandler := handlers.NewWebSocketHandler(
		wsManager,
		sessionRepo,
		log,
	)

	// Setup Gin router
	router := setupRouter(
		cfg,
		sessionHandler,
		messageHandler,
		groupHandler,
		contactHandler,
		statusHandler,
		deviceHandler,
		websocketHandler,
		log,
	)

	// Start HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start server in goroutine
	go func() {
		log.Info("Server starting on port %d", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop session manager
	sessionManager.Stop()

	// Stop WebSocket manager
	wsManager.Stop()

	// Shutdown HTTP server
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("Server forced to shutdown: %v", err)
	}

	log.Info("Server shutdown complete")
}

func setupRouter(
	cfg *config.Config,
	sessionHandler *handlers.SessionHandler,
	messageHandler *handlers.MessageHandler,
	groupHandler *handlers.GroupHandler,
	contactHandler *handlers.ContactHandler,
	statusHandler *handlers.StatusHandler,
	deviceHandler *handlers.DeviceHandler,
	websocketHandler *handlers.WebSocketHandler,
	log *logger.Logger,
) *gin.Engine {
	// Set Gin mode
	if cfg.Server.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Global middleware
	router.Use(gin.Recovery())
	router.Use(middleware.Logger(log))

	// CORS configuration
	corsConfig := cors.Config{
		AllowOrigins:     cfg.Server.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
	router.Use(cors.New(corsConfig))

	// Health check endpoints
	router.GET("/health", handleHealthCheck)
	router.GET("/ready", handleReadyCheck(sessionHandler))

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Apply auth middleware to all v1 routes
		v1.Use(middleware.JWTAuth(cfg.JWT.Secret, log))

		// Apply rate limiting
		v1.Use(middleware.RateLimiter(cfg.RateLimit.RequestsPerMinute))

		// Session management endpoints
		sessions := v1.Group("/sessions")
		{
			sessions.POST("", sessionHandler.CreateSession)
			sessions.GET("", sessionHandler.ListSessions)
			sessions.GET("/:session_id", sessionHandler.GetSession)
			sessions.GET("/:session_id/qr", sessionHandler.GenerateQR)
			sessions.GET("/:session_id/status", sessionHandler.GetStatus)
			sessions.DELETE("/:session_id", sessionHandler.DeleteSession)
			sessions.POST("/:session_id/connect", sessionHandler.Connect)
			sessions.POST("/:session_id/disconnect", sessionHandler.Disconnect)
		}

		// Message endpoints
		messages := v1.Group("/messages")
		{
			messages.POST("/send", messageHandler.SendMessage)
			messages.POST("/send/image", messageHandler.SendImage)
			messages.POST("/send/document", messageHandler.SendDocument)
			messages.POST("/send/audio", messageHandler.SendAudio)
			messages.POST("/send/video", messageHandler.SendVideo)
			messages.POST("/send/location", messageHandler.SendLocation)
			messages.POST("/send/contact", messageHandler.SendContact)
			messages.POST("/broadcast", messageHandler.BroadcastMessage)
			messages.GET("/:session_id", messageHandler.GetMessages)
			messages.GET("/:session_id/:message_id", messageHandler.GetMessage)
			messages.POST("/:session_id/:message_id/read", messageHandler.MarkAsRead)
		}

		// Group endpoints
		groups := v1.Group("/groups")
		{
			groups.POST("/create", groupHandler.CreateGroup)
			groups.GET("/:session_id", groupHandler.GetGroups)
			groups.GET("/:session_id/:group_id", groupHandler.GetGroupInfo)
			groups.POST("/join", groupHandler.JoinGroup)
			groups.POST("/:session_id/:group_id/leave", groupHandler.LeaveGroup)
			groups.PUT("/:session_id/:group_id", groupHandler.UpdateGroupInfo)
			groups.PUT("/:session_id/:group_id/photo", groupHandler.UpdateGroupPhoto)
			groups.POST("/:session_id/:group_id/participants/add", groupHandler.AddParticipants)
			groups.POST("/:session_id/:group_id/participants/remove", groupHandler.RemoveParticipants)
			groups.POST("/:session_id/:group_id/participants/promote", groupHandler.PromoteParticipants)
			groups.POST("/:session_id/:group_id/participants/demote", groupHandler.DemoteParticipants)
			groups.GET("/:session_id/:group_id/invite-link", groupHandler.GetInviteLink)
		}

		// Contact endpoints
		contacts := v1.Group("/contacts")
		{
			contacts.POST("/:session_id/sync", contactHandler.SyncContacts)
			contacts.GET("/:session_id", contactHandler.GetContacts)
			contacts.GET("/:session_id/:jid", contactHandler.GetContact)
			contacts.GET("/:session_id/:jid/profile", contactHandler.GetProfile)
			contacts.GET("/:session_id/:jid/picture", contactHandler.GetProfilePicture)
			contacts.POST("/check", contactHandler.CheckContactsExist)
			contacts.POST("/:session_id/:jid/block", contactHandler.BlockContact)
			contacts.POST("/:session_id/:jid/unblock", contactHandler.UnblockContact)
			contacts.GET("/:session_id/blocked", contactHandler.GetBlockedContacts)
			contacts.POST("/:session_id/:jid/presence", contactHandler.SubscribePresence)
			contacts.GET("/:session_id/:jid/presence", contactHandler.GetPresence)
			contacts.GET("/:session_id/:jid/common-groups", contactHandler.GetCommonGroups)
		}

		// Status endpoints
		status := v1.Group("/status")
		{
			status.POST("/text", statusHandler.PostTextStatus)
			status.POST("/image", statusHandler.PostImageStatus)
			status.POST("/video", statusHandler.PostVideoStatus)
			status.GET("/:session_id/mine", statusHandler.GetMyStatuses)
			status.GET("/:session_id/contacts", statusHandler.GetContactStatuses)
			status.POST("/view", statusHandler.ViewStatus)
			status.DELETE("/:session_id/:status_id", statusHandler.DeleteStatus)
			status.GET("/:session_id/privacy", statusHandler.GetPrivacy)
			status.PUT("/:session_id/privacy", statusHandler.UpdatePrivacy)
			status.GET("/:session_id/:status_id/viewers", statusHandler.GetViewers)
			status.POST("/:session_id/mute/:contact_jid", statusHandler.MuteUpdates)
			status.POST("/:session_id/unmute/:contact_jid", statusHandler.UnmuteUpdates)
		}

		// Device management endpoints
		devices := v1.Group("/devices")
		{
			devices.GET("/summary", deviceHandler.GetDeviceSummary)
			devices.GET("/limit", deviceHandler.GetDeviceLimit)
		}

		// WebSocket endpoint (auth handled internally)
		v1.GET("/ws/:session_id", websocketHandler.HandleConnection)
	}

	// 404 handler
	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Endpoint not found",
		})
	})

	return router
}

// Health check handlers
func handleHealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"time":   time.Now().Unix(),
	})
}

func handleReadyCheck(sessionHandler *handlers.SessionHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check database connection
		if sessionHandler == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not_ready",
				"error":  "Service not initialized",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "ready",
			"time":   time.Now().Unix(),
		})
	}
}
