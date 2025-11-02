package services

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"

	"github.com/skip2/go-qrcode"
	"whatsapp-api/internal/config"
)

// QRGeneratorService handles QR code generation
type QRGeneratorService struct {
	config *config.Config
}

// NewQRGeneratorService creates a new QR generator service
func NewQRGeneratorService(cfg *config.Config) *QRGeneratorService {
	return &QRGeneratorService{
		config: cfg,
	}
}

// QRCodeData represents QR code data in different formats
type QRCodeData struct {
	Raw     string `json:"raw"`
	Base64  string `json:"base64"`
	PNG     []byte `json:"-"`
	DataURI string `json:"data_uri"`
}

// GenerateQRCode generates a QR code from the given data
func (s *QRGeneratorService) GenerateQRCode(data string) (*QRCodeData, error) {
	if data == "" {
		return nil, fmt.Errorf("QR code data cannot be empty")
	}

	size := s.config.QRCode.Size
	if size <= 0 {
		size = 256 // Default size
	}

	// Get recovery level
	recoveryLevel := s.getRecoveryLevel()

	// Generate QR code as PNG
	qrCode, err := qrcode.New(data, recoveryLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to create QR code: %w", err)
	}

	// Set the size
	qrCode.DisableBorder = false

	// Generate PNG bytes
	pngBytes, err := qrCode.PNG(size)
	if err != nil {
		return nil, fmt.Errorf("failed to generate QR code PNG: %w", err)
	}

	// Generate base64 string
	base64String := base64.StdEncoding.EncodeToString(pngBytes)

	// Generate data URI
	dataURI := fmt.Sprintf("data:image/png;base64,%s", base64String)

	return &QRCodeData{
		Raw:     data,
		Base64:  base64String,
		PNG:     pngBytes,
		DataURI: dataURI,
	}, nil
}

// GenerateQRCodePNG generates a QR code as PNG bytes
func (s *QRGeneratorService) GenerateQRCodePNG(data string) ([]byte, error) {
	if data == "" {
		return nil, fmt.Errorf("QR code data cannot be empty")
	}

	size := s.config.QRCode.Size
	if size <= 0 {
		size = 256
	}

	recoveryLevel := s.getRecoveryLevel()

	pngBytes, err := qrcode.Encode(data, recoveryLevel, size)
	if err != nil {
		return nil, fmt.Errorf("failed to generate QR code PNG: %w", err)
	}

	return pngBytes, nil
}

// GenerateQRCodeBase64 generates a QR code as base64 string
func (s *QRGeneratorService) GenerateQRCodeBase64(data string) (string, error) {
	pngBytes, err := s.GenerateQRCodePNG(data)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(pngBytes), nil
}

// GenerateQRCodeDataURI generates a QR code as data URI
func (s *QRGeneratorService) GenerateQRCodeDataURI(data string) (string, error) {
	base64String, err := s.GenerateQRCodeBase64(data)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("data:image/png;base64,%s", base64String), nil
}

// GenerateQRCodeWithCustomSize generates a QR code with custom size
func (s *QRGeneratorService) GenerateQRCodeWithCustomSize(data string, size int) (*QRCodeData, error) {
	if data == "" {
		return nil, fmt.Errorf("QR code data cannot be empty")
	}

	if size <= 0 {
		size = 256
	}

	recoveryLevel := s.getRecoveryLevel()

	qrCode, err := qrcode.New(data, recoveryLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to create QR code: %w", err)
	}

	pngBytes, err := qrCode.PNG(size)
	if err != nil {
		return nil, fmt.Errorf("failed to generate QR code PNG: %w", err)
	}

	base64String := base64.StdEncoding.EncodeToString(pngBytes)
	dataURI := fmt.Sprintf("data:image/png;base64,%s", base64String)

	return &QRCodeData{
		Raw:     data,
		Base64:  base64String,
		PNG:     pngBytes,
		DataURI: dataURI,
	}, nil
}

// getRecoveryLevel returns the QR code recovery level based on config
func (s *QRGeneratorService) getRecoveryLevel() qrcode.RecoveryLevel {
	switch s.config.QRCode.RecoveryLevel {
	case "low":
		return qrcode.Low
	case "medium":
		return qrcode.Medium
	case "high":
		return qrcode.High
	case "highest":
		return qrcode.Highest
	default:
		return qrcode.Medium
	}
}

// ValidateQRData validates QR code data
func (s *QRGeneratorService) ValidateQRData(data string) error {
	if data == "" {
		return fmt.Errorf("QR code data cannot be empty")
	}

	if len(data) > 4296 {
		return fmt.Errorf("QR code data too long (max 4296 characters)")
	}

	return nil
}

// GenerateQRCodeSVG generates a QR code as SVG string (using custom implementation)
func (s *QRGeneratorService) GenerateQRCodeSVG(data string) (string, error) {
	if data == "" {
		return "", fmt.Errorf("QR code data cannot be empty")
	}

	size := s.config.QRCode.Size
	if size <= 0 {
		size = 256
	}

	recoveryLevel := s.getRecoveryLevel()

	// Generate QR code
	qrCode, err := qrcode.New(data, recoveryLevel)
	if err != nil {
		return "", fmt.Errorf("failed to create QR code: %w", err)
	}

	// Convert to SVG (simplified implementation)
	// For production, you might want to use a proper SVG library
	svgString := s.convertToSVG(qrCode, size)

	return svgString, nil
}

// convertToSVG converts QR code to SVG string (simplified)
func (s *QRGeneratorService) convertToSVG(qr *qrcode.QRCode, size int) string {
	// This is a simplified SVG conversion
	// For production use, consider using a dedicated library
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`,
		size, size, size, size)
	svg += `<rect width="100%" height="100%" fill="white"/>`

	// Here you would add the actual QR code modules
	// This is a placeholder implementation

	svg += `</svg>`
	return svg
}

// GenerateMultipleFormats generates QR code in multiple formats
func (s *QRGeneratorService) GenerateMultipleFormats(data string) (map[string]interface{}, error) {
	qrData, err := s.GenerateQRCode(data)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"raw":      qrData.Raw,
		"base64":   qrData.Base64,
		"data_uri": qrData.DataURI,
		"size":     s.config.QRCode.Size,
	}, nil
}

// QRCodeOptions represents options for QR code generation
type QRCodeOptions struct {
	Size            int
	RecoveryLevel   string
	BorderSize      int
	ForegroundColor string
	BackgroundColor string
}

// GenerateQRCodeWithOptions generates a QR code with custom options
func (s *QRGeneratorService) GenerateQRCodeWithOptions(data string, options *QRCodeOptions) (*QRCodeData, error) {
	if data == "" {
		return nil, fmt.Errorf("QR code data cannot be empty")
	}

	size := options.Size
	if size <= 0 {
		size = s.config.QRCode.Size
	}

	// Get recovery level
	var recoveryLevel qrcode.RecoveryLevel
	switch options.RecoveryLevel {
	case "low":
		recoveryLevel = qrcode.Low
	case "medium":
		recoveryLevel = qrcode.Medium
	case "high":
		recoveryLevel = qrcode.High
	case "highest":
		recoveryLevel = qrcode.Highest
	default:
		recoveryLevel = s.getRecoveryLevel()
	}

	// Create QR code
	qrCode, err := qrcode.New(data, recoveryLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to create QR code: %w", err)
	}

	// Apply custom options
	if options.ForegroundColor != "" {
		// Parse and set foreground color (requires color parsing)
		// This is a placeholder - implement color parsing if needed
	}

	if options.BackgroundColor != "" {
		// Parse and set background color
		// This is a placeholder - implement color parsing if needed
	}

	// Generate PNG
	pngBytes, err := qrCode.PNG(size)
	if err != nil {
		return nil, fmt.Errorf("failed to generate QR code PNG: %w", err)
	}

	base64String := base64.StdEncoding.EncodeToString(pngBytes)
	dataURI := fmt.Sprintf("data:image/png;base64,%s", base64String)

	return &QRCodeData{
		Raw:     data,
		Base64:  base64String,
		PNG:     pngBytes,
		DataURI: dataURI,
	}, nil
}

// GetQRCodeSize returns the configured QR code size
func (s *QRGeneratorService) GetQRCodeSize() int {
	return s.config.QRCode.Size
}

// GetRecoveryLevel returns the configured recovery level
func (s *QRGeneratorService) GetRecoveryLevel() string {
	return s.config.QRCode.RecoveryLevel
}

// EncodeToBase64 encodes bytes to base64 string
func (s *QRGeneratorService) EncodeToBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// DecodeFromBase64 decodes base64 string to bytes
func (s *QRGeneratorService) DecodeFromBase64(encoded string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(encoded)
}

// CreateDataURI creates a data URI from bytes
func (s *QRGeneratorService) CreateDataURI(data []byte, mimeType string) string {
	base64String := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64String)
}

// OptimizeQRCodeSize optimizes QR code size based on data length
func (s *QRGeneratorService) OptimizeQRCodeSize(dataLength int) int {
	// Simple optimization: larger data = larger QR code
	switch {
	case dataLength < 50:
		return 128
	case dataLength < 100:
		return 192
	case dataLength < 200:
		return 256
	case dataLength < 500:
		return 384
	default:
		return 512
	}
}

// ResizeQRCode resizes an existing QR code PNG
func (s *QRGeneratorService) ResizeQRCode(pngData []byte, newSize int) ([]byte, error) {
	// Decode the PNG
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, fmt.Errorf("failed to decode PNG: %w", err)
	}

	// For now, return the original - implement proper resizing if needed
	// This would require image manipulation libraries like "github.com/nfnt/resize"
	_ = img
	_ = newSize

	return pngData, nil
}

// ValidateQRCodeSize validates the requested QR code size
func (s *QRGeneratorService) ValidateQRCodeSize(size int) error {
	if size < 32 {
		return fmt.Errorf("QR code size too small (minimum 32)")
	}

	if size > 2048 {
		return fmt.Errorf("QR code size too large (maximum 2048)")
	}

	return nil
}
