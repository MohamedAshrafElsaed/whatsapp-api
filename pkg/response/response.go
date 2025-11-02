// pkg/response/response.go
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// StandardResponse represents a standard API response
type StandardResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Meta    *Meta       `json:"meta,omitempty"`
}

// Meta contains pagination and other metadata
type Meta struct {
	Page       int   `json:"page,omitempty"`
	PerPage    int   `json:"per_page,omitempty"`
	Total      int64 `json:"total,omitempty"`
	TotalPages int   `json:"total_pages,omitempty"`
}

// Success sends a successful response
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, StandardResponse{
		Success: true,
		Data:    data,
	})
}

// SuccessWithMessage sends a successful response with a message
func SuccessWithMessage(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, StandardResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// SuccessWithMeta sends a successful response with metadata
func SuccessWithMeta(c *gin.Context, data interface{}, meta *Meta) {
	c.JSON(http.StatusOK, StandardResponse{
		Success: true,
		Data:    data,
		Meta:    meta,
	})
}

// Created sends a successful creation response
func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, StandardResponse{
		Success: true,
		Message: "Resource created successfully",
		Data:    data,
	})
}

// NoContent sends a no content response
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// BadRequest sends a bad request error response
func BadRequest(c *gin.Context, error string) {
	c.JSON(http.StatusBadRequest, StandardResponse{
		Success: false,
		Error:   error,
	})
	c.Abort()
}

// Unauthorized sends an unauthorized error response
func Unauthorized(c *gin.Context, error string) {
	if error == "" {
		error = "Unauthorized"
	}
	c.JSON(http.StatusUnauthorized, StandardResponse{
		Success: false,
		Error:   error,
	})
	c.Abort()
}

// Forbidden sends a forbidden error response
func Forbidden(c *gin.Context, error string) {
	if error == "" {
		error = "Forbidden"
	}
	c.JSON(http.StatusForbidden, StandardResponse{
		Success: false,
		Error:   error,
	})
	c.Abort()
}

// NotFound sends a not found error response
func NotFound(c *gin.Context, error string) {
	if error == "" {
		error = "Resource not found"
	}
	c.JSON(http.StatusNotFound, StandardResponse{
		Success: false,
		Error:   error,
	})
	c.Abort()
}

// Conflict sends a conflict error response
func Conflict(c *gin.Context, error string) {
	c.JSON(http.StatusConflict, StandardResponse{
		Success: false,
		Error:   error,
	})
	c.Abort()
}

// UnprocessableEntity sends an unprocessable entity error response
func UnprocessableEntity(c *gin.Context, error string) {
	c.JSON(http.StatusUnprocessableEntity, StandardResponse{
		Success: false,
		Error:   error,
	})
	c.Abort()
}

// TooManyRequests sends a rate limit error response
func TooManyRequests(c *gin.Context, error string) {
	if error == "" {
		error = "Too many requests"
	}
	c.JSON(http.StatusTooManyRequests, StandardResponse{
		Success: false,
		Error:   error,
	})
	c.Abort()
}

// InternalError sends an internal server error response
func InternalError(c *gin.Context, error string) {
	if error == "" {
		error = "Internal server error"
	}
	c.JSON(http.StatusInternalServerError, StandardResponse{
		Success: false,
		Error:   error,
	})
	c.Abort()
}

// ServiceUnavailable sends a service unavailable error response
func ServiceUnavailable(c *gin.Context, error string) {
	if error == "" {
		error = "Service unavailable"
	}
	c.JSON(http.StatusServiceUnavailable, StandardResponse{
		Success: false,
		Error:   error,
	})
	c.Abort()
}

// Custom sends a custom response with specific status code
func Custom(c *gin.Context, statusCode int, success bool, message string, data interface{}, error string) {
	response := StandardResponse{
		Success: success,
	}

	if message != "" {
		response.Message = message
	}

	if data != nil {
		response.Data = data
	}

	if error != "" {
		response.Error = error
	}

	c.JSON(statusCode, response)
}

// ErrorResponse sends a generic error response
func ErrorResponse(c *gin.Context, statusCode int, error string) {
	c.JSON(statusCode, StandardResponse{
		Success: false,
		Error:   error,
	})
	c.Abort()
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationErrors sends validation error response
func ValidationErrors(c *gin.Context, errors []ValidationError) {
	c.JSON(http.StatusUnprocessableEntity, StandardResponse{
		Success: false,
		Error:   "Validation failed",
		Data:    gin.H{"validation_errors": errors},
	})
	c.Abort()
}
