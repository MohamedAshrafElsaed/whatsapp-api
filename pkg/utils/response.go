package utils

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"whatsapp-api/internal/dto"
)

// Response is a generic response wrapper
type Response struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Details interface{} `json:"details,omitempty"`
}

// SuccessResponse sends a success response
func SuccessResponse(c *gin.Context, statusCode int, message string, data interface{}) {
	c.JSON(statusCode, Response{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// ErrorResponse sends an error response
func ErrorResponse(c *gin.Context, statusCode int, message string, details interface{}) {
	c.JSON(statusCode, Response{
		Success: false,
		Message: message,
		Details: details,
	})
}

// ErrorResponseWithError sends an error response with error details
func ErrorResponseWithError(c *gin.Context, statusCode int, message string, err error, details interface{}) {
	response := Response{
		Success: false,
		Message: message,
		Details: details,
	}

	if err != nil {
		response.Error = err.Error()
	}

	c.JSON(statusCode, response)
}

// ValidationErrorResponse sends a validation error response
func ValidationErrorResponse(c *gin.Context, errors interface{}) {
	c.JSON(http.StatusBadRequest, Response{
		Success: false,
		Message: "Validation failed",
		Details: errors,
	})
}

// NotFoundResponse sends a not found response
func NotFoundResponse(c *gin.Context, message string) {
	c.JSON(http.StatusNotFound, Response{
		Success: false,
		Message: message,
	})
}

// UnauthorizedResponse sends an unauthorized response
func UnauthorizedResponse(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, Response{
		Success: false,
		Message: message,
	})
}

// ForbiddenResponse sends a forbidden response
func ForbiddenResponse(c *gin.Context, message string) {
	c.JSON(http.StatusForbidden, Response{
		Success: false,
		Message: message,
	})
}

// ConflictResponse sends a conflict response
func ConflictResponse(c *gin.Context, message string, details interface{}) {
	c.JSON(http.StatusConflict, Response{
		Success: false,
		Message: message,
		Details: details,
	})
}

// InternalServerErrorResponse sends an internal server error response
func InternalServerErrorResponse(c *gin.Context, message string, err error) {
	response := Response{
		Success: false,
		Message: message,
	}

	// Only include error details in development
	if gin.Mode() == gin.DebugMode && err != nil {
		response.Error = err.Error()
	}

	c.JSON(http.StatusInternalServerError, response)
}

// BadRequestResponse sends a bad request response
func BadRequestResponse(c *gin.Context, message string, details interface{}) {
	c.JSON(http.StatusBadRequest, Response{
		Success: false,
		Message: message,
		Details: details,
	})
}

// CreatedResponse sends a created response
func CreatedResponse(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusCreated, Response{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// AcceptedResponse sends an accepted response
func AcceptedResponse(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusAccepted, Response{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// NoContentResponse sends a no content response
func NoContentResponse(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// OKResponse sends an OK response
func OKResponse(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Success: true,
		Data:    data,
	})
}

// OKResponseWithMessage sends an OK response with a message
func OKResponseWithMessage(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// PaginatedResponse sends a paginated response
func PaginatedResponse(c *gin.Context, data interface{}, meta *dto.PaginationMeta) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
		"meta":    meta,
	})
}

// ListResponse sends a list response
func ListResponse(c *gin.Context, data interface{}, total int64) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
		"total":   total,
	})
}

// CustomResponse sends a custom response
func CustomResponse(c *gin.Context, statusCode int, response interface{}) {
	c.JSON(statusCode, response)
}

// HealthResponse sends a health check response
func HealthResponse(c *gin.Context, status string, services map[string]dto.ServiceHealth) {
	c.JSON(http.StatusOK, dto.NewHealthResponse(status, services))
}

// ServiceUnavailableResponse sends a service unavailable response
func ServiceUnavailableResponse(c *gin.Context, message string) {
	c.JSON(http.StatusServiceUnavailable, Response{
		Success: false,
		Message: message,
	})
}

// TooManyRequestsResponse sends a too many requests response
func TooManyRequestsResponse(c *gin.Context, message string) {
	c.JSON(http.StatusTooManyRequests, Response{
		Success: false,
		Message: message,
	})
}

// GatewayTimeoutResponse sends a gateway timeout response
func GatewayTimeoutResponse(c *gin.Context, message string) {
	c.JSON(http.StatusGatewayTimeout, Response{
		Success: false,
		Message: message,
	})
}

// MethodNotAllowedResponse sends a method not allowed response
func MethodNotAllowedResponse(c *gin.Context, message string) {
	c.JSON(http.StatusMethodNotAllowed, Response{
		Success: false,
		Message: message,
	})
}

// UnsupportedMediaTypeResponse sends an unsupported media type response
func UnsupportedMediaTypeResponse(c *gin.Context, message string) {
	c.JSON(http.StatusUnsupportedMediaType, Response{
		Success: false,
		Message: message,
	})
}

// UnprocessableEntityResponse sends an unprocessable entity response
func UnprocessableEntityResponse(c *gin.Context, message string, details interface{}) {
	c.JSON(http.StatusUnprocessableEntity, Response{
		Success: false,
		Message: message,
		Details: details,
	})
}

// RedirectResponse sends a redirect response
func RedirectResponse(c *gin.Context, statusCode int, location string) {
	c.Redirect(statusCode, location)
}

// JSONResponse is a generic JSON response helper
func JSONResponse(c *gin.Context, statusCode int, success bool, message string, data interface{}, err error) {
	response := Response{
		Success: success,
		Message: message,
		Data:    data,
	}

	if err != nil && gin.Mode() == gin.DebugMode {
		response.Error = err.Error()
	}

	c.JSON(statusCode, response)
}

// AbortWithError aborts the request with an error response
func AbortWithError(c *gin.Context, statusCode int, message string, err error) {
	response := Response{
		Success: false,
		Message: message,
	}

	if err != nil && gin.Mode() == gin.DebugMode {
		response.Error = err.Error()
	}

	c.AbortWithStatusJSON(statusCode, response)
}

// AbortWithValidationError aborts with validation errors
func AbortWithValidationError(c *gin.Context, errors interface{}) {
	c.AbortWithStatusJSON(http.StatusBadRequest, Response{
		Success: false,
		Message: "Validation failed",
		Details: errors,
	})
}

// StreamJSON streams JSON response (for large responses)
func StreamJSON(c *gin.Context, data interface{}) error {
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Status(http.StatusOK)
	return c.Stream(func(w io.Writer) bool {
		return false
	})
}

// FileResponse sends a file response
func FileResponse(c *gin.Context, filepath string, filename string) {
	c.FileAttachment(filepath, filename)
}

// DownloadResponse sends a file for download
func DownloadResponse(c *gin.Context, data []byte, filename string, contentType string) {
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, contentType, data)
}

// ImageResponse sends an image response
func ImageResponse(c *gin.Context, data []byte, contentType string) {
	c.Data(http.StatusOK, contentType, data)
}

// TextResponse sends a plain text response
func TextResponse(c *gin.Context, statusCode int, text string) {
	c.String(statusCode, text)
}

// HTMLResponse sends an HTML response
func HTMLResponse(c *gin.Context, statusCode int, html string) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(statusCode, html)
}

// XMLResponse sends an XML response
func XMLResponse(c *gin.Context, statusCode int, data interface{}) {
	c.XML(statusCode, data)
}

// YAMLResponse sends a YAML response
func YAMLResponse(c *gin.Context, statusCode int, data interface{}) {
	c.YAML(statusCode, data)
}

// BindAndValidate binds and validates request data
func BindAndValidate(c *gin.Context, obj interface{}) error {
	if err := c.ShouldBindJSON(obj); err != nil {
		ValidationErrorResponse(c, err.Error())
		return err
	}

	// Check if the object has a Validate method
	if validator, ok := obj.(interface{ Validate() error }); ok {
		if err := validator.Validate(); err != nil {
			ValidationErrorResponse(c, err.Error())
			return err
		}
	}

	return nil
}

// BindQueryAndValidate binds and validates query parameters
func BindQueryAndValidate(c *gin.Context, obj interface{}) error {
	if err := c.ShouldBindQuery(obj); err != nil {
		ValidationErrorResponse(c, err.Error())
		return err
	}

	// Check if the object has a Validate method
	if validator, ok := obj.(interface{ Validate() error }); ok {
		if err := validator.Validate(); err != nil {
			ValidationErrorResponse(c, err.Error())
			return err
		}
	}

	return nil
}

// BindURIAndValidate binds and validates URI parameters
func BindURIAndValidate(c *gin.Context, obj interface{}) error {
	if err := c.ShouldBindUri(obj); err != nil {
		ValidationErrorResponse(c, err.Error())
		return err
	}

	// Check if the object has a Validate method
	if validator, ok := obj.(interface{ Validate() error }); ok {
		if err := validator.Validate(); err != nil {
			ValidationErrorResponse(c, err.Error())
			return err
		}
	}

	return nil
}

// GetStatusMessage returns a default message for a status code
func GetStatusMessage(statusCode int) string {
	switch statusCode {
	case http.StatusOK:
		return "Success"
	case http.StatusCreated:
		return "Created successfully"
	case http.StatusAccepted:
		return "Accepted"
	case http.StatusNoContent:
		return "No content"
	case http.StatusBadRequest:
		return "Bad request"
	case http.StatusUnauthorized:
		return "Unauthorized"
	case http.StatusForbidden:
		return "Forbidden"
	case http.StatusNotFound:
		return "Not found"
	case http.StatusConflict:
		return "Conflict"
	case http.StatusUnprocessableEntity:
		return "Unprocessable entity"
	case http.StatusTooManyRequests:
		return "Too many requests"
	case http.StatusInternalServerError:
		return "Internal server error"
	case http.StatusServiceUnavailable:
		return "Service unavailable"
	case http.StatusGatewayTimeout:
		return "Gateway timeout"
	default:
		return "Unknown error"
	}
}

// IsSuccessStatus checks if status code is successful (2xx)
func IsSuccessStatus(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}

// IsErrorStatus checks if status code is an error (4xx or 5xx)
func IsErrorStatus(statusCode int) bool {
	return statusCode >= 400
}
