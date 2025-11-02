package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"whatsapp-api/internal/config"
	"whatsapp-api/pkg/utils"
)

// ContextKey is a custom type for context keys
type ContextKey string

const (
	// UserIDKey is the context key for user ID
	UserIDKey ContextKey = "user_id"
	// UserEmailKey is the context key for user email
	UserEmailKey ContextKey = "user_email"
	// TokenKey is the context key for the JWT token
	TokenKey ContextKey = "token"
)

// Claims represents the JWT claims structure from Laravel
type Claims struct {
	UserID int    `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// AuthMiddleware validates JWT tokens from Laravel
func AuthMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			utils.ErrorResponse(c, http.StatusUnauthorized, "Authorization header required", nil)
			c.Abort()
			return
		}

		// Check if it's a Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid authorization header format", nil)
			c.Abort()
			return
		}

		tokenString := parts[1]

		// Parse and validate the token
		token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
			// Verify signing method
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(cfg.JWT.Secret), nil
		})

		if err != nil {
			if errors.Is(err, jwt.ErrTokenExpired) {
				utils.ErrorResponse(c, http.StatusUnauthorized, "Token has expired", nil)
			} else if errors.Is(err, jwt.ErrTokenNotValidYet) {
				utils.ErrorResponse(c, http.StatusUnauthorized, "Token not valid yet", nil)
			} else {
				utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid token", map[string]interface{}{
					"error": err.Error(),
				})
			}
			c.Abort()
			return
		}

		// Extract claims
		claims, ok := token.Claims.(*Claims)
		if !ok || !token.Valid {
			utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid token claims", nil)
			c.Abort()
			return
		}

		// Validate issuer
		if claims.Issuer != cfg.JWT.Issuer {
			utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid token issuer", nil)
			c.Abort()
			return
		}

		// Validate audience
		if !claims.VerifyAudience(cfg.JWT.Audience, true) {
			utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid token audience", nil)
			c.Abort()
			return
		}

		// Validate user ID
		if claims.UserID <= 0 {
			utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid user ID in token", nil)
			c.Abort()
			return
		}

		// Store user information in context
		c.Set(string(UserIDKey), claims.UserID)
		c.Set(string(UserEmailKey), claims.Email)
		c.Set(string(TokenKey), tokenString)

		// Also set in request context for use in services
		ctx := context.WithValue(c.Request.Context(), UserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, UserEmailKey, claims.Email)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// OptionalAuthMiddleware validates JWT tokens if present, but doesn't require them
func OptionalAuthMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			// No token provided, continue without authentication
			c.Next()
			return
		}

		// Token provided, validate it
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			// Invalid format but optional, continue
			c.Next()
			return
		}

		tokenString := parts[1]

		token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(cfg.JWT.Secret), nil
		})

		if err == nil && token.Valid {
			if claims, ok := token.Claims.(*Claims); ok {
				// Valid token, store user info
				c.Set(string(UserIDKey), claims.UserID)
				c.Set(string(UserEmailKey), claims.Email)
				c.Set(string(TokenKey), tokenString)

				ctx := context.WithValue(c.Request.Context(), UserIDKey, claims.UserID)
				ctx = context.WithValue(ctx, UserEmailKey, claims.Email)
				c.Request = c.Request.WithContext(ctx)
			}
		}

		c.Next()
	}
}

// GetUserID retrieves the user ID from the Gin context
func GetUserID(c *gin.Context) (int, error) {
	userID, exists := c.Get(string(UserIDKey))
	if !exists {
		return 0, fmt.Errorf("user ID not found in context")
	}

	id, ok := userID.(int)
	if !ok {
		return 0, fmt.Errorf("invalid user ID type in context")
	}

	return id, nil
}

// GetUserEmail retrieves the user email from the Gin context
func GetUserEmail(c *gin.Context) (string, error) {
	userEmail, exists := c.Get(string(UserEmailKey))
	if !exists {
		return "", fmt.Errorf("user email not found in context")
	}

	email, ok := userEmail.(string)
	if !ok {
		return "", fmt.Errorf("invalid user email type in context")
	}

	return email, nil
}

// GetToken retrieves the JWT token from the Gin context
func GetToken(c *gin.Context) (string, error) {
	token, exists := c.Get(string(TokenKey))
	if !exists {
		return "", fmt.Errorf("token not found in context")
	}

	tokenStr, ok := token.(string)
	if !ok {
		return "", fmt.Errorf("invalid token type in context")
	}

	return tokenStr, nil
}

// GetUserIDFromContext retrieves the user ID from the standard context
func GetUserIDFromContext(ctx context.Context) (int, error) {
	userID := ctx.Value(UserIDKey)
	if userID == nil {
		return 0, fmt.Errorf("user ID not found in context")
	}

	id, ok := userID.(int)
	if !ok {
		return 0, fmt.Errorf("invalid user ID type in context")
	}

	return id, nil
}

// MustGetUserID retrieves the user ID from the Gin context or panics
// Use this only when you're certain the middleware has been applied
func MustGetUserID(c *gin.Context) int {
	userID, err := GetUserID(c)
	if err != nil {
		panic(err)
	}
	return userID
}

// RequireAuth is a helper that returns 401 if user is not authenticated
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		_, exists := c.Get(string(UserIDKey))
		if !exists {
			utils.ErrorResponse(c, http.StatusUnauthorized, "Authentication required", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}

// ValidateToken validates a JWT token string without gin context
func ValidateToken(tokenString string, cfg *config.Config) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(cfg.JWT.Secret), nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Validate issuer
	if claims.Issuer != cfg.JWT.Issuer {
		return nil, fmt.Errorf("invalid token issuer")
	}

	// Validate audience
	if !claims.VerifyAudience(cfg.JWT.Audience, true) {
		return nil, fmt.Errorf("invalid token audience")
	}

	// Validate user ID
	if claims.UserID <= 0 {
		return nil, fmt.Errorf("invalid user ID in token")
	}

	return claims, nil
}

// ExtractTokenFromHeader extracts the JWT token from an Authorization header
func ExtractTokenFromHeader(authHeader string) (string, error) {
	if authHeader == "" {
		return "", fmt.Errorf("authorization header is empty")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid authorization header format")
	}

	if strings.ToLower(parts[0]) != "bearer" {
		return "", fmt.Errorf("authorization header must use Bearer scheme")
	}

	return parts[1], nil
}

// IsAuthenticated checks if the request is authenticated
func IsAuthenticated(c *gin.Context) bool {
	_, exists := c.Get(string(UserIDKey))
	return exists
}

// GetClientIP retrieves the client's IP address
func GetClientIP(c *gin.Context) string {
	// Check X-Forwarded-For header first
	forwarded := c.GetHeader("X-Forwarded-For")
	if forwarded != "" {
		// X-Forwarded-For can contain multiple IPs, get the first one
		ips := strings.Split(forwarded, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	realIP := c.GetHeader("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	// Fall back to RemoteAddr
	return c.ClientIP()
}

// GetUserAgent retrieves the user agent from the request
func GetUserAgent(c *gin.Context) string {
	return c.GetHeader("User-Agent")
}
