package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"whatsapp-api/internal/config"
)

// CORSMiddleware handles Cross-Origin Resource Sharing
func CORSMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Check if origin is allowed
		if isOriginAllowed(origin, cfg.CORS.AllowedOrigins) {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		} else if len(cfg.CORS.AllowedOrigins) == 1 && cfg.CORS.AllowedOrigins[0] == "*" {
			// Allow all origins if configured
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		}

		// Set allowed methods
		if len(cfg.CORS.AllowedMethods) > 0 {
			c.Writer.Header().Set("Access-Control-Allow-Methods", strings.Join(cfg.CORS.AllowedMethods, ", "))
		}

		// Set allowed headers
		if len(cfg.CORS.AllowedHeaders) > 0 {
			c.Writer.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.CORS.AllowedHeaders, ", "))
		}

		// Set credentials flag
		if cfg.CORS.AllowCredentials {
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		// Set max age for preflight cache
		if cfg.CORS.MaxAge > 0 {
			c.Writer.Header().Set("Access-Control-Max-Age", string(rune(cfg.CORS.MaxAge)))
		}

		// Set exposed headers (for clients to read)
		c.Writer.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Type, Authorization")

		// Handle preflight requests
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// isOriginAllowed checks if an origin is in the allowed list
func isOriginAllowed(origin string, allowedOrigins []string) bool {
	if origin == "" {
		return false
	}

	for _, allowed := range allowedOrigins {
		if allowed == "*" {
			return true
		}

		// Exact match
		if allowed == origin {
			return true
		}

		// Wildcard subdomain match (e.g., *.example.com)
		if strings.HasPrefix(allowed, "*.") {
			domain := strings.TrimPrefix(allowed, "*.")
			if strings.HasSuffix(origin, domain) {
				return true
			}
		}
	}

	return false
}

// SecureHeaders adds security headers to responses
func SecureHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prevent MIME type sniffing
		c.Writer.Header().Set("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking
		c.Writer.Header().Set("X-Frame-Options", "DENY")

		// Enable XSS protection
		c.Writer.Header().Set("X-XSS-Protection", "1; mode=block")

		// Referrer policy
		c.Writer.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Content Security Policy (basic)
		c.Writer.Header().Set("Content-Security-Policy", "default-src 'self'")

		// Strict Transport Security (only if HTTPS)
		if c.Request.TLS != nil {
			c.Writer.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		c.Next()
	}
}

// NoCache adds headers to prevent caching
func NoCache() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		c.Writer.Header().Set("Pragma", "no-cache")
		c.Writer.Header().Set("Expires", "0")
		c.Next()
	}
}

// CacheControl adds cache control headers
func CacheControl(maxAge int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if maxAge > 0 {
			c.Writer.Header().Set("Cache-Control", "public, max-age="+string(rune(maxAge)))
		} else {
			c.Writer.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		}
		c.Next()
	}
}

// AddVaryHeader adds Vary header to response
func AddVaryHeader(headers ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(headers) > 0 {
			c.Writer.Header().Set("Vary", strings.Join(headers, ", "))
		}
		c.Next()
	}
}

// ContentTypeJSON ensures the response content type is JSON
func ContentTypeJSON() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		c.Next()
	}
}

// AllowWebSocket allows WebSocket upgrade requests
func AllowWebSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if this is a WebSocket upgrade request
		if c.GetHeader("Upgrade") == "websocket" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", c.GetHeader("Origin"))
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Sec-WebSocket-Protocol, Sec-WebSocket-Version, Sec-WebSocket-Key")
		}
		c.Next()
	}
}

// PreflightHandler handles CORS preflight requests
func PreflightHandler(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == "OPTIONS" {
			origin := c.Request.Header.Get("Origin")

			if isOriginAllowed(origin, cfg.CORS.AllowedOrigins) {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			} else if len(cfg.CORS.AllowedOrigins) == 1 && cfg.CORS.AllowedOrigins[0] == "*" {
				c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
			}

			c.Writer.Header().Set("Access-Control-Allow-Methods", strings.Join(cfg.CORS.AllowedMethods, ", "))
			c.Writer.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.CORS.AllowedHeaders, ", "))

			if cfg.CORS.AllowCredentials {
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if cfg.CORS.MaxAge > 0 {
				c.Writer.Header().Set("Access-Control-Max-Age", string(rune(cfg.CORS.MaxAge)))
			}

			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// CustomCORS creates a custom CORS middleware with specific settings
func CustomCORS(allowedOrigins []string, allowedMethods []string, allowedHeaders []string, allowCredentials bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		if isOriginAllowed(origin, allowedOrigins) {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		}

		if len(allowedMethods) > 0 {
			c.Writer.Header().Set("Access-Control-Allow-Methods", strings.Join(allowedMethods, ", "))
		}

		if len(allowedHeaders) > 0 {
			c.Writer.Header().Set("Access-Control-Allow-Headers", strings.Join(allowedHeaders, ", "))
		}

		if allowCredentials {
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		c.Writer.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// OriginValidator validates the origin against a custom function
func OriginValidator(validator func(string) bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		if origin != "" && !validator(origin) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Origin not allowed",
			})
			return
		}

		c.Next()
	}
}

// AllowOrigins is a helper to create a simple CORS middleware
func AllowOrigins(origins ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		if isOriginAllowed(origin, origins) {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
