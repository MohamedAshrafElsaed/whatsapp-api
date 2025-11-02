package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"whatsapp-api/internal/config"
)

// LoggerConfig contains logger middleware configuration
type LoggerConfig struct {
	SkipPaths       []string
	SkipMethods     []string
	LogRequestBody  bool
	LogResponseBody bool
	MaxBodySize     int
}

// RequestLogger logs HTTP requests with detailed information
func RequestLogger(cfg *config.Config) gin.HandlerFunc {
	return RequestLoggerWithConfig(LoggerConfig{
		SkipPaths: []string{
			"/health",
			"/metrics",
			"/favicon.ico",
		},
		LogRequestBody:  cfg.Server.Debug,
		LogResponseBody: false,
		MaxBodySize:     4096, // 4KB max for body logging
	})
}

// RequestLoggerWithConfig creates a logger middleware with custom config
func RequestLoggerWithConfig(config LoggerConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip logging for certain paths
		path := c.Request.URL.Path
		for _, skipPath := range config.SkipPaths {
			if path == skipPath {
				c.Next()
				return
			}
		}

		// Skip logging for certain methods
		method := c.Request.Method
		for _, skipMethod := range config.SkipMethods {
			if method == skipMethod {
				c.Next()
				return
			}
		}

		// Start timer
		start := time.Now()

		// Get request ID (if set by previous middleware)
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
			c.Header("X-Request-ID", requestID)
		}

		// Log request body if enabled
		var requestBody []byte
		if config.LogRequestBody && c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			// Restore body for handlers
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// Create response writer wrapper to capture response
		blw := &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		if config.LogResponseBody {
			c.Writer = blw
		}

		// Process request
		c.Next()

		// Calculate latency
		latency := time.Since(start)

		// Get client IP
		clientIP := GetClientIP(c)

		// Get user ID if available
		userID := 0
		if uid, exists := c.Get(string(UserIDKey)); exists {
			userID, _ = uid.(int)
		}

		// Build log entry
		logEntry := map[string]interface{}{
			"timestamp":  start.Format(time.RFC3339),
			"request_id": requestID,
			"method":     c.Request.Method,
			"path":       path,
			"query":      c.Request.URL.RawQuery,
			"status":     c.Writer.Status(),
			"latency_ms": latency.Milliseconds(),
			"client_ip":  clientIP,
			"user_agent": c.Request.UserAgent(),
			"user_id":    userID,
			"errors":     c.Errors.String(),
		}

		// Add request body if logged
		if config.LogRequestBody && len(requestBody) > 0 && len(requestBody) < config.MaxBodySize {
			logEntry["request_body"] = string(requestBody)
		}

		// Add response body if logged
		if config.LogResponseBody && blw.body.Len() > 0 && blw.body.Len() < config.MaxBodySize {
			logEntry["response_body"] = blw.body.String()
		}

		// Log based on status code
		statusCode := c.Writer.Status()
		logJSON, _ := json.Marshal(logEntry)

		if statusCode >= 500 {
			log.Printf("[ERROR] %s", string(logJSON))
		} else if statusCode >= 400 {
			log.Printf("[WARN] %s", string(logJSON))
		} else {
			log.Printf("[INFO] %s", string(logJSON))
		}
	}
}

// bodyLogWriter is a custom response writer that captures the response body
type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w bodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w bodyLogWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

// SimpleLogger provides basic request logging
func SimpleLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		method := c.Request.Method
		path := c.Request.URL.Path

		log.Printf("[%s] %s %s - %d (%v)",
			time.Now().Format("2006-01-02 15:04:05"),
			method,
			path,
			statusCode,
			latency,
		)
	}
}

// StructuredLogger provides JSON-formatted structured logging
func StructuredLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		end := time.Now()
		latency := end.Sub(start)

		logEntry := map[string]interface{}{
			"timestamp":  end.Format(time.RFC3339),
			"level":      getLogLevel(c.Writer.Status()),
			"method":     c.Request.Method,
			"path":       path,
			"query":      query,
			"status":     c.Writer.Status(),
			"latency_ns": latency.Nanoseconds(),
			"latency":    latency.String(),
			"client_ip":  c.ClientIP(),
			"user_agent": c.Request.UserAgent(),
		}

		// Add user ID if available
		if userID, exists := c.Get(string(UserIDKey)); exists {
			logEntry["user_id"] = userID
		}

		// Add errors if any
		if len(c.Errors) > 0 {
			logEntry["errors"] = c.Errors.String()
		}

		logJSON, _ := json.Marshal(logEntry)
		log.Println(string(logJSON))
	}
}

// getLogLevel returns log level based on status code
func getLogLevel(statusCode int) string {
	switch {
	case statusCode >= 500:
		return "ERROR"
	case statusCode >= 400:
		return "WARN"
	case statusCode >= 300:
		return "INFO"
	default:
		return "INFO"
	}
}

// RequestID middleware adds a unique request ID to each request
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}

		c.Header("X-Request-ID", requestID)
		c.Set("request_id", requestID)

		c.Next()
	}
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().Unix())
}

// ErrorLogger logs errors from the context
func ErrorLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) > 0 {
			for _, err := range c.Errors {
				logEntry := map[string]interface{}{
					"timestamp":  time.Now().Format(time.RFC3339),
					"level":      "ERROR",
					"error":      err.Error(),
					"error_type": err.Type,
					"path":       c.Request.URL.Path,
					"method":     c.Request.Method,
				}

				if userID, exists := c.Get(string(UserIDKey)); exists {
					logEntry["user_id"] = userID
				}

				if requestID, exists := c.Get("request_id"); exists {
					logEntry["request_id"] = requestID
				}

				logJSON, _ := json.Marshal(logEntry)
				log.Printf("[ERROR] %s", string(logJSON))
			}
		}
	}
}

// SlowRequestLogger logs requests that take longer than the specified duration
func SlowRequestLogger(threshold time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		latency := time.Since(start)

		if latency > threshold {
			logEntry := map[string]interface{}{
				"timestamp":  time.Now().Format(time.RFC3339),
				"level":      "WARN",
				"type":       "SLOW_REQUEST",
				"method":     c.Request.Method,
				"path":       c.Request.URL.Path,
				"latency":    latency.String(),
				"latency_ms": latency.Milliseconds(),
				"threshold":  threshold.String(),
				"status":     c.Writer.Status(),
			}

			if userID, exists := c.Get(string(UserIDKey)); exists {
				logEntry["user_id"] = userID
			}

			logJSON, _ := json.Marshal(logEntry)
			log.Printf("[WARN] %s", string(logJSON))
		}
	}
}

// AccessLogger logs all access attempts
func AccessLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		logEntry := map[string]interface{}{
			"timestamp":  time.Now().Format(time.RFC3339),
			"type":       "ACCESS",
			"method":     c.Request.Method,
			"path":       c.Request.URL.Path,
			"status":     c.Writer.Status(),
			"latency_ms": time.Since(start).Milliseconds(),
			"client_ip":  GetClientIP(c),
			"user_agent": GetUserAgent(c),
		}

		if userID, exists := c.Get(string(UserIDKey)); exists {
			logEntry["user_id"] = userID
		}

		if email, exists := c.Get(string(UserEmailKey)); exists {
			logEntry["user_email"] = email
		}

		logJSON, _ := json.Marshal(logEntry)
		log.Println(string(logJSON))
	}
}

// MetricsLogger logs request metrics
func MetricsLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start)

		// Log metrics
		logEntry := map[string]interface{}{
			"type":          "METRICS",
			"method":        c.Request.Method,
			"path":          c.Request.URL.Path,
			"status":        c.Writer.Status(),
			"duration_ms":   duration.Milliseconds(),
			"request_size":  c.Request.ContentLength,
			"response_size": c.Writer.Size(),
		}

		logJSON, _ := json.Marshal(logEntry)
		log.Println(string(logJSON))
	}
}

// DevelopmentLogger provides detailed logging for development
func DevelopmentLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// Log request
		log.Printf("→ [REQUEST] %s %s?%s", c.Request.Method, path, query)
		log.Printf("  Headers: %v", c.Request.Header)

		c.Next()

		latency := time.Since(start)

		// Log response
		statusColor := getStatusColor(c.Writer.Status())
		log.Printf("← [RESPONSE] %s %d %s (%v)",
			statusColor,
			c.Writer.Status(),
			path,
			latency,
		)

		if len(c.Errors) > 0 {
			log.Printf("  Errors: %s", c.Errors.String())
		}
	}
}

// getStatusColor returns ANSI color code based on status
func getStatusColor(status int) string {
	switch {
	case status >= 500:
		return "\033[31m" // Red
	case status >= 400:
		return "\033[33m" // Yellow
	case status >= 300:
		return "\033[36m" // Cyan
	case status >= 200:
		return "\033[32m" // Green
	default:
		return "\033[37m" // White
	}
}
