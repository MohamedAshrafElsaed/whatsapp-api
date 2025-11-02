// ⚠️⚠️⚠️ WARNING: JWT AUTHENTICATION DISABLED FOR TESTING ⚠️⚠️⚠️
// This version has JWT authentication bypassed for testing purposes.
// All requests will use user_id = 1
// DO NOT USE IN PRODUCTION!
// To restore JWT authentication, uncomment the original code blocks
// in AuthMiddleware() and validateWebSocketToken() functions.
// ================================================================

package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// ============= MIDDLEWARE =============

// AuthMiddleware validates JWT tokens from Laravel
// ⚠️ WARNING: JWT AUTHENTICATION DISABLED FOR TESTING ⚠️
func AuthMiddleware(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ========================================
		// JWT AUTHENTICATION BYPASSED FOR TESTING
		// ========================================
		// Using a default test user ID = 1
		// Remove this block and uncomment the original code for production

		c.Set("user_id", 1) // Default test user ID
		c.Set("claims", jwt.MapClaims{
			"user_id":   float64(1),
			"test_mode": true,
		})
		log.Println("⚠️ JWT AUTH BYPASSED - TEST MODE - User ID: 1")
		c.Next()
		return

		/* ORIGINAL JWT VALIDATION CODE - UNCOMMENT FOR PRODUCTION
		// Get token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Authorization header missing",
			})
			c.Abort()
			return
		}

		// Extract token
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Invalid authorization format",
			})
			c.Abort()
			return
		}

		// Parse and validate token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			// Validate signing method
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(jwtSecret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Invalid or expired token",
			})
			c.Abort()
			return
		}

		// Extract claims
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Invalid token claims",
			})
			c.Abort()
			return
		}

		// Get user ID
		userIDFloat, ok := claims["user_id"].(float64)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "User ID not found in token",
			})
			c.Abort()
			return
		}

		userID := int(userIDFloat)

		// Store in context
		c.Set("user_id", userID)
		c.Set("claims", claims)

		c.Next()
		*/
	}
}

// CORSMiddleware handles CORS headers
func CORSMiddleware(allowedOrigins string) gin.HandlerFunc {
	origins := strings.Split(allowedOrigins, ",")

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Check if origin is allowed
		for _, allowedOrigin := range origins {
			if allowedOrigin == "*" || allowedOrigin == origin {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				break
			}
		}

		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// LoggerMiddleware logs HTTP requests
func LoggerMiddleware() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf("%s - [%s] \"%s %s %s %d %s \"%s\" %s\"\n",
			param.ClientIP,
			param.TimeStamp.Format(time.RFC1123),
			param.Method,
			param.Path,
			param.Request.Proto,
			param.StatusCode,
			param.Latency,
			param.Request.UserAgent(),
			param.ErrorMessage,
		)
	})
}

// ErrorMiddleware handles panic recovery
func ErrorMiddleware() gin.HandlerFunc {
	return gin.Recovery()
}

// ============= HANDLERS =============

type APIHandlers struct {
	whatsappService *WhatsAppService
	db              *DatabaseManager
	wsManager       *WebSocketManager
	cfg             *Config
}

func NewAPIHandlers(ws *WhatsAppService, db *DatabaseManager, wsm *WebSocketManager, cfg *Config) *APIHandlers {
	return &APIHandlers{
		whatsappService: ws,
		db:              db,
		wsManager:       wsm,
		cfg:             cfg,
	}
}

// CreateSession creates a new WhatsApp session
func (h *APIHandlers) CreateSession(c *gin.Context) {
	userID := c.GetInt("user_id")

	var req struct {
		SessionName string `json:"session_name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Create session
	session, err := h.whatsappService.CreateSession(userID, req.SessionName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data": gin.H{
			"session_id":   session.ID,
			"user_id":      session.UserID,
			"session_name": session.SessionName,
			"status":       session.Status,
			"created_at":   session.CreatedAt,
		},
	})
}

// GetSessions gets all sessions for the authenticated user
func (h *APIHandlers) GetSessions(c *gin.Context) {
	userID := c.GetInt("user_id")

	// Get sessions
	sessions, err := h.whatsappService.GetUserSessions(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Get summary
	summary, err := h.db.GetUserDeviceSummary(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Format response
	sessionList := make([]gin.H, 0, len(sessions))
	for _, session := range sessions {
		sessionList = append(sessionList, gin.H{
			"id":           session.ID,
			"session_name": session.SessionName,
			"status":       session.Status,
			"phone_number": session.PhoneNumber,
			"jid":          session.JID,
			"push_name":    session.PushName,
			"platform":     session.Platform,
			"connected_at": session.ConnectedAt,
			"last_seen":    session.LastSeen,
			"is_active":    session.IsActive,
			"created_at":   session.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"sessions": sessionList,
			"summary": gin.H{
				"total_sessions":  len(sessions),
				"connected":       summary.ConnectedDevices,
				"pending":         summary.UsedDevices - summary.ConnectedDevices,
				"max_devices":     h.cfg.MaxDevicesPerUser,
				"available_slots": summary.AvailableSlots,
			},
		},
	})
}

// GetSessionQR gets the QR code for a session
func (h *APIHandlers) GetSessionQR(c *gin.Context) {
	userID := c.GetInt("user_id")
	sessionIDStr := c.Param("session_id")
	format := c.DefaultQuery("format", "json")

	// Parse session ID
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid session ID",
		})
		return
	}

	// Get session
	session, err := h.db.GetSession(sessionID, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Session not found",
		})
		return
	}

	// Check if QR is available
	if session.Status != StatusPending && session.Status != StatusQRReady {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   fmt.Sprintf("QR code not available for status: %s", session.Status),
		})
		return
	}

	// Get QR code
	qrCode, err := h.whatsappService.GetQRCode(sessionID, userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Return based on format
	if format == "png" {
		// Decode base64 and return as PNG
		data := strings.TrimPrefix(qrCode, "data:image/png;base64,")
		decoded, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to decode QR image",
			})
			return
		}

		c.Data(http.StatusOK, "image/png", decoded)
	} else {
		// Return as JSON
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"qr_code":    qrCode,
				"expires_at": session.QRExpiresAt,
			},
		})
	}
}

// GetSessionStatus gets the status of a session
func (h *APIHandlers) GetSessionStatus(c *gin.Context) {
	userID := c.GetInt("user_id")
	sessionIDStr := c.Param("session_id")

	// Parse session ID
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid session ID",
		})
		return
	}

	// Get session status
	session, err := h.whatsappService.GetSessionStatus(sessionID, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Session not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"session_id":   session.ID,
			"status":       session.Status,
			"phone_number": session.PhoneNumber,
			"jid":          session.JID,
			"push_name":    session.PushName,
			"last_seen":    session.LastSeen,
			"connected_at": session.ConnectedAt,
		},
	})
}

// DeleteSession deletes a session
func (h *APIHandlers) DeleteSession(c *gin.Context) {
	userID := c.GetInt("user_id")
	sessionIDStr := c.Param("session_id")

	// Parse session ID
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid session ID",
		})
		return
	}

	// Delete session
	if err := h.whatsappService.DeleteSession(sessionID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Session deleted successfully",
	})
}

// GetDeviceSummary gets device summary for a user
func (h *APIHandlers) GetDeviceSummary(c *gin.Context) {
	userID := c.GetInt("user_id")

	// Get summary
	summary, err := h.db.GetUserDeviceSummary(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    summary,
	})
}

// SendMessage sends a WhatsApp message
func (h *APIHandlers) SendMessage(c *gin.Context) {
	userID := c.GetInt("user_id")
	sessionIDStr := c.Param("session_id")

	var req struct {
		To      string `json:"to" binding:"required"`
		Message string `json:"message" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Parse session ID
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid session ID",
		})
		return
	}

	// Send message
	if err := h.whatsappService.SendMessage(sessionID, userID, req.To, req.Message); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Message sent successfully",
	})
}

// WebSocket upgrader
var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Configure based on your needs
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// HandleWebSocket handles WebSocket connections for real-time updates
func (h *APIHandlers) HandleWebSocket(c *gin.Context) {
	sessionIDStr := c.Param("session_id")
	token := c.Query("token")

	// Validate token
	userID, err := h.validateWebSocketToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Invalid token",
		})
		return
	}

	// Parse session ID
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid session ID",
		})
		return
	}

	// Verify user owns this session
	session, err := h.db.GetSession(sessionID, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Session not found",
		})
		return
	}

	// Upgrade to WebSocket
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade to WebSocket: %v", err)
		return
	}
	defer conn.Close()

	// Add connection to manager
	h.wsManager.AddConnection(sessionID, conn)
	defer h.wsManager.RemoveConnection(sessionID, conn)

	// Send initial status
	conn.WriteJSON(WebSocketMessage{
		Type: "status",
		Data: map[string]interface{}{
			"session_id": session.ID,
			"status":     session.Status,
			"connected":  session.Status == StatusConnected,
		},
	})

	// Keep connection alive
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	done := make(chan struct{})

	// Read messages (for ping/pong)
	go func() {
		defer close(done)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	// Write ping messages
	for {
		select {
		case <-ticker.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

// validateWebSocketToken validates JWT token for WebSocket
// ⚠️ WARNING: JWT VALIDATION DISABLED FOR TESTING ⚠️
func (h *APIHandlers) validateWebSocketToken(tokenString string) (int, error) {
	// ========================================
	// JWT VALIDATION BYPASSED FOR TESTING
	// ========================================
	log.Println("⚠️ WebSocket JWT BYPASSED - TEST MODE - Returning User ID: 1")
	return 1, nil // Always return user ID 1 for testing

	/* ORIGINAL JWT VALIDATION CODE - UNCOMMENT FOR PRODUCTION
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(h.cfg.JWTSecret), nil
	})

	if err != nil || !token.Valid {
		return 0, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, fmt.Errorf("invalid claims")
	}

	userIDFloat, ok := claims["user_id"].(float64)
	if !ok {
		return 0, fmt.Errorf("user_id not found")
	}

	return int(userIDFloat), nil
	*/
}

// Health check endpoint
func (h *APIHandlers) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"status":  "healthy",
		"time":    time.Now(),
	})
}
