// internal/services/message_service.go
package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"whatsapp-api/internal/models"
	"whatsapp-api/internal/repositories"
	"whatsapp-api/pkg/logger"
)

// MessageService handles WhatsApp messaging operations
type MessageService struct {
	sessionManager *SessionManager
	messageRepo    *repositories.MessageRepository
	eventRepo      *repositories.EventRepository
	logger         *logger.Logger
}

// NewMessageService creates a new message service
func NewMessageService(
	sessionManager *SessionManager,
	messageRepo *repositories.MessageRepository,
	eventRepo *repositories.EventRepository,
	logger *logger.Logger,
) *MessageService {
	return &MessageService{
		sessionManager: sessionManager,
		messageRepo:    messageRepo,
		eventRepo:      eventRepo,
		logger:         logger,
	}
}

// SendTextMessage sends a text message
func (ms *MessageService) SendTextMessage(ctx context.Context, sessionID uuid.UUID, req *models.SendMessageRequest) (*models.MessageResponse, error) {
	client, err := ms.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Parse recipient JID
	recipientJID, err := ms.parseJID(req.To)
	if err != nil {
		return nil, fmt.Errorf("invalid recipient: %w", err)
	}

	// Build message
	message := &waE2E.Message{
		Conversation: proto.String(req.Text),
	}

	// Add quoted message if replying
	if req.QuotedMessageID != "" {
		message.ExtendedTextMessage = &waE2E.ExtendedTextMessage{
			Text: proto.String(req.Text),
			ContextInfo: &waE2E.ContextInfo{
				QuotedMessage: &waE2E.Message{
					Conversation: proto.String(""), // Will be filled by WhatsApp
				},
				StanzaID:    proto.String(req.QuotedMessageID),
				Participant: proto.String(req.To),
			},
		}
		message.Conversation = nil
	}

	// Send message
	resp, err := client.SendMessage(ctx, recipientJID, message)
	if err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	// Store message in database
	messageRecord := &models.Message{
		SessionID: sessionID,
		MessageID: string(resp.ID),
		To:        req.To,
		From:      client.Store.ID.String(),
		Type:      "text",
		Content:   req.Text,
		Status:    "sent",
		SentAt:    resp.Timestamp,
	}

	if err := ms.messageRepo.Create(ctx, messageRecord); err != nil {
		ms.logger.Error("Failed to store message: %v", err)
	}

	// Log event
	ms.logMessageEvent(ctx, sessionID, "text_sent", map[string]interface{}{
		"to":         req.To,
		"message_id": resp.ID,
		"timestamp":  resp.Timestamp,
	})

	return &models.MessageResponse{
		MessageID: string(resp.ID),
		Timestamp: resp.Timestamp,
		Status:    "sent",
	}, nil
}

// SendImageMessage sends an image message
func (ms *MessageService) SendImageMessage(ctx context.Context, sessionID uuid.UUID, req *models.SendMediaRequest) (*models.MessageResponse, error) {
	client, err := ms.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Parse recipient JID
	recipientJID, err := ms.parseJID(req.To)
	if err != nil {
		return nil, fmt.Errorf("invalid recipient: %w", err)
	}

	// Download or decode media
	mediaData, err := ms.getMediaData(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get media data: %w", err)
	}

	// Upload media to WhatsApp
	uploadResp, err := client.Upload(ctx, mediaData, whatsmeow.MediaImage)
	if err != nil {
		return nil, fmt.Errorf("failed to upload media: %w", err)
	}

	// Build image message
	message := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			URL:           proto.String(uploadResp.URL),
			DirectPath:    proto.String(uploadResp.DirectPath),
			MediaKey:      uploadResp.MediaKey,
			FileEncSHA256: uploadResp.FileEncSHA256,
			FileSHA256:    uploadResp.FileSHA256,
			FileLength:    proto.Uint64(uploadResp.FileLength),
			Mimetype:      proto.String(req.MimeType),
			Caption:       proto.String(req.Caption),
		},
	}

	// Send message
	resp, err := client.SendMessage(ctx, recipientJID, message)
	if err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	// Store message
	messageRecord := &models.Message{
		SessionID: sessionID,
		MessageID: string(resp.ID),
		To:        req.To,
		From:      client.Store.ID.String(),
		Type:      "image",
		MediaURL:  &uploadResp.URL,
		Caption:   &req.Caption,
		Status:    "sent",
		SentAt:    resp.Timestamp,
	}

	if err := ms.messageRepo.Create(ctx, messageRecord); err != nil {
		ms.logger.Error("Failed to store message: %v", err)
	}

	// Log event
	ms.logMessageEvent(ctx, sessionID, "image_sent", map[string]interface{}{
		"to":         req.To,
		"message_id": resp.ID,
		"caption":    req.Caption,
	})

	return &models.MessageResponse{
		MessageID: string(resp.ID),
		Timestamp: resp.Timestamp,
		Status:    "sent",
	}, nil
}

// SendDocumentMessage sends a document message
func (ms *MessageService) SendDocumentMessage(ctx context.Context, sessionID uuid.UUID, req *models.SendMediaRequest) (*models.MessageResponse, error) {
	client, err := ms.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Parse recipient JID
	recipientJID, err := ms.parseJID(req.To)
	if err != nil {
		return nil, fmt.Errorf("invalid recipient: %w", err)
	}

	// Download or decode media
	mediaData, err := ms.getMediaData(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get media data: %w", err)
	}

	// Upload media to WhatsApp
	uploadResp, err := client.Upload(ctx, mediaData, whatsmeow.MediaDocument)
	if err != nil {
		return nil, fmt.Errorf("failed to upload media: %w", err)
	}

	// Build document message
	message := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           proto.String(uploadResp.URL),
			DirectPath:    proto.String(uploadResp.DirectPath),
			MediaKey:      uploadResp.MediaKey,
			FileEncSHA256: uploadResp.FileEncSHA256,
			FileSHA256:    uploadResp.FileSHA256,
			FileLength:    proto.Uint64(uploadResp.FileLength),
			Mimetype:      proto.String(req.MimeType),
			FileName:      proto.String(req.FileName),
			Title:         proto.String(req.FileName),
		},
	}

	// Send message
	resp, err := client.SendMessage(ctx, recipientJID, message)
	if err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	// Store message
	messageRecord := &models.Message{
		SessionID: sessionID,
		MessageID: string(resp.ID),
		To:        req.To,
		From:      client.Store.ID.String(),
		Type:      "document",
		MediaURL:  &uploadResp.URL,
		FileName:  &req.FileName,
		Status:    "sent",
		SentAt:    resp.Timestamp,
	}

	if err := ms.messageRepo.Create(ctx, messageRecord); err != nil {
		ms.logger.Error("Failed to store message: %v", err)
	}

	return &models.MessageResponse{
		MessageID: string(resp.ID),
		Timestamp: resp.Timestamp,
		Status:    "sent",
	}, nil
}

// SendAudioMessage sends an audio message
func (ms *MessageService) SendAudioMessage(ctx context.Context, sessionID uuid.UUID, req *models.SendMediaRequest) (*models.MessageResponse, error) {
	client, err := ms.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Parse recipient JID
	recipientJID, err := ms.parseJID(req.To)
	if err != nil {
		return nil, fmt.Errorf("invalid recipient: %w", err)
	}

	// Download or decode media
	mediaData, err := ms.getMediaData(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get media data: %w", err)
	}

	// Upload media to WhatsApp
	uploadResp, err := client.Upload(ctx, mediaData, whatsmeow.MediaAudio)
	if err != nil {
		return nil, fmt.Errorf("failed to upload media: %w", err)
	}

	// Build audio message
	message := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           proto.String(uploadResp.URL),
			DirectPath:    proto.String(uploadResp.DirectPath),
			MediaKey:      uploadResp.MediaKey,
			FileEncSHA256: uploadResp.FileEncSHA256,
			FileSHA256:    uploadResp.FileSHA256,
			FileLength:    proto.Uint64(uploadResp.FileLength),
			Mimetype:      proto.String(req.MimeType),
			Seconds:       proto.Uint32(uint32(req.Duration)),
			PTT:           proto.Bool(req.IsVoiceNote),
		},
	}

	// Send message
	resp, err := client.SendMessage(ctx, recipientJID, message)
	if err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	// Store message
	messageRecord := &models.Message{
		SessionID: sessionID,
		MessageID: string(resp.ID),
		To:        req.To,
		From:      client.Store.ID.String(),
		Type:      "audio",
		MediaURL:  &uploadResp.URL,
		Status:    "sent",
		SentAt:    resp.Timestamp,
	}

	if err := ms.messageRepo.Create(ctx, messageRecord); err != nil {
		ms.logger.Error("Failed to store message: %v", err)
	}

	return &models.MessageResponse{
		MessageID: string(resp.ID),
		Timestamp: resp.Timestamp,
		Status:    "sent",
	}, nil
}

// SendVideoMessage sends a video message
func (ms *MessageService) SendVideoMessage(ctx context.Context, sessionID uuid.UUID, req *models.SendMediaRequest) (*models.MessageResponse, error) {
	client, err := ms.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Parse recipient JID
	recipientJID, err := ms.parseJID(req.To)
	if err != nil {
		return nil, fmt.Errorf("invalid recipient: %w", err)
	}

	// Download or decode media
	mediaData, err := ms.getMediaData(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get media data: %w", err)
	}

	// Upload media to WhatsApp
	uploadResp, err := client.Upload(ctx, mediaData, whatsmeow.MediaVideo)
	if err != nil {
		return nil, fmt.Errorf("failed to upload media: %w", err)
	}

	// Build video message
	message := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			URL:           proto.String(uploadResp.URL),
			DirectPath:    proto.String(uploadResp.DirectPath),
			MediaKey:      uploadResp.MediaKey,
			FileEncSHA256: uploadResp.FileEncSHA256,
			FileSHA256:    uploadResp.FileSHA256,
			FileLength:    proto.Uint64(uploadResp.FileLength),
			Mimetype:      proto.String(req.MimeType),
			Caption:       proto.String(req.Caption),
			Seconds:       proto.Uint32(uint32(req.Duration)),
		},
	}

	// Send message
	resp, err := client.SendMessage(ctx, recipientJID, message)
	if err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	// Store message
	messageRecord := &models.Message{
		SessionID: sessionID,
		MessageID: string(resp.ID),
		To:        req.To,
		From:      client.Store.ID.String(),
		Type:      "video",
		MediaURL:  &uploadResp.URL,
		Caption:   &req.Caption,
		Status:    "sent",
		SentAt:    resp.Timestamp,
	}

	if err := ms.messageRepo.Create(ctx, messageRecord); err != nil {
		ms.logger.Error("Failed to store message: %v", err)
	}

	return &models.MessageResponse{
		MessageID: string(resp.ID),
		Timestamp: resp.Timestamp,
		Status:    "sent",
	}, nil
}

// SendLocationMessage sends a location message
func (ms *MessageService) SendLocationMessage(ctx context.Context, sessionID uuid.UUID, req *models.SendLocationRequest) (*models.MessageResponse, error) {
	client, err := ms.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Parse recipient JID
	recipientJID, err := ms.parseJID(req.To)
	if err != nil {
		return nil, fmt.Errorf("invalid recipient: %w", err)
	}

	// Build location message
	message := &waE2E.Message{
		LocationMessage: &waE2E.LocationMessage{
			DegreesLatitude:  proto.Float64(req.Latitude),
			DegreesLongitude: proto.Float64(req.Longitude),
			Name:             proto.String(req.Name),
			Address:          proto.String(req.Address),
		},
	}

	// Send message
	resp, err := client.SendMessage(ctx, recipientJID, message)
	if err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	// Store message
	messageRecord := &models.Message{
		SessionID: sessionID,
		MessageID: string(resp.ID),
		To:        req.To,
		From:      client.Store.ID.String(),
		Type:      "location",
		Status:    "sent",
		SentAt:    resp.Timestamp,
	}

	if err := ms.messageRepo.Create(ctx, messageRecord); err != nil {
		ms.logger.Error("Failed to store message: %v", err)
	}

	return &models.MessageResponse{
		MessageID: string(resp.ID),
		Timestamp: resp.Timestamp,
		Status:    "sent",
	}, nil
}

// SendContactMessage sends a contact message
func (ms *MessageService) SendContactMessage(ctx context.Context, sessionID uuid.UUID, req *models.SendContactRequest) (*models.MessageResponse, error) {
	client, err := ms.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Parse recipient JID
	recipientJID, err := ms.parseJID(req.To)
	if err != nil {
		return nil, fmt.Errorf("invalid recipient: %w", err)
	}

	// Build vCard
	vcard := fmt.Sprintf("BEGIN:VCARD\nVERSION:3.0\nFN:%s\nTEL:%s\nEND:VCARD", req.DisplayName, req.PhoneNumber)

	// Build contact message
	message := &waE2E.Message{
		ContactMessage: &waE2E.ContactMessage{
			DisplayName: proto.String(req.DisplayName),
			Vcard:       proto.String(vcard),
		},
	}

	// Send message
	resp, err := client.SendMessage(ctx, recipientJID, message)
	if err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	// Store message
	messageRecord := &models.Message{
		SessionID: sessionID,
		MessageID: string(resp.ID),
		To:        req.To,
		From:      client.Store.ID.String(),
		Type:      "contact",
		Status:    "sent",
		SentAt:    resp.Timestamp,
	}

	if err := ms.messageRepo.Create(ctx, messageRecord); err != nil {
		ms.logger.Error("Failed to store message: %v", err)
	}

	return &models.MessageResponse{
		MessageID: string(resp.ID),
		Timestamp: resp.Timestamp,
		Status:    "sent",
	}, nil
}

// BroadcastMessage sends a message to multiple recipients
func (ms *MessageService) BroadcastMessage(ctx context.Context, sessionID uuid.UUID, req *models.BroadcastRequest) (*models.BroadcastResponse, error) {
	client, err := ms.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	results := make([]models.BroadcastResult, 0, len(req.Recipients))
	successCount := 0
	failureCount := 0

	// Send to each recipient
	for _, recipient := range req.Recipients {
		// Parse recipient JID
		recipientJID, err := ms.parseJID(recipient)
		if err != nil {
			results = append(results, models.BroadcastResult{
				Recipient: recipient,
				Status:    "failed",
				Error:     err.Error(),
			})
			failureCount++
			continue
		}

		// Build message
		message := &waE2E.Message{
			Conversation: proto.String(req.Text),
		}

		// Send message
		resp, err := client.SendMessage(ctx, recipientJID, message)
		if err != nil {
			results = append(results, models.BroadcastResult{
				Recipient: recipient,
				Status:    "failed",
				Error:     err.Error(),
			})
			failureCount++
		} else {
			results = append(results, models.BroadcastResult{
				Recipient: recipient,
				MessageID: string(resp.ID),
				Status:    "sent",
			})
			successCount++
		}

		// Small delay between messages to avoid rate limiting
		time.Sleep(500 * time.Millisecond)
	}

	// Log event
	ms.logMessageEvent(ctx, sessionID, "broadcast_sent", map[string]interface{}{
		"total":   len(req.Recipients),
		"success": successCount,
		"failure": failureCount,
		"text":    req.Text,
	})

	return &models.BroadcastResponse{
		TotalRecipients: len(req.Recipients),
		SuccessCount:    successCount,
		FailureCount:    failureCount,
		Results:         results,
	}, nil
}

// GetMessages retrieves messages for a session
func (ms *MessageService) GetMessages(ctx context.Context, sessionID uuid.UUID, filter *models.MessageFilter) ([]*models.Message, error) {
	return ms.messageRepo.GetBySession(ctx, sessionID, filter)
}

// GetMessage retrieves a specific message
func (ms *MessageService) GetMessage(ctx context.Context, sessionID uuid.UUID, messageID string) (*models.Message, error) {
	return ms.messageRepo.GetByID(ctx, sessionID, messageID)
}

// MarkMessageAsRead marks a message as read
func (ms *MessageService) MarkMessageAsRead(ctx context.Context, sessionID uuid.UUID, messageID string) error {
	client, err := ms.sessionManager.GetClient(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return fmt.Errorf("session not logged in")
	}

	// Get message from database to get chat JID
	message, err := ms.messageRepo.GetByID(ctx, sessionID, messageID)
	if err != nil {
		return fmt.Errorf("message not found: %w", err)
	}

	// Parse chat JID
	chatJID, err := ms.parseJID(message.From)
	if err != nil {
		return fmt.Errorf("invalid chat JID: %w", err)
	}

	// Mark as read
	messageIDs := []types.MessageID{types.MessageID(messageID)}
	err = client.MarkRead(ctx, messageIDs, time.Now(), chatJID, types.EmptyJID)
	if err != nil {
		return fmt.Errorf("failed to mark as read: %w", err)
	}

	// Update message status in database
	message.Status = "read"
	return ms.messageRepo.Update(ctx, message)
}

// Helper functions

// parseJID parses a phone number or JID string into a WhatsApp JID
func (ms *MessageService) parseJID(input string) (types.JID, error) {
	// Remove any non-numeric characters from phone numbers
	cleaned := strings.ReplaceAll(input, "+", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "(", "")
	cleaned = strings.ReplaceAll(cleaned, ")", "")

	// Check if it's already a JID
	if strings.Contains(cleaned, "@") {
		return types.ParseJID(cleaned)
	}

	// Check if it's a group
	if strings.HasPrefix(cleaned, "g_") {
		groupID := strings.TrimPrefix(cleaned, "g_")
		return types.ParseJID(groupID + "@g.us")
	}

	// It's a phone number, convert to JID
	return types.ParseJID(cleaned + "@s.whatsapp.net")
}

// getMediaData retrieves media data from URL or base64
func (ms *MessageService) getMediaData(req *models.SendMediaRequest) ([]byte, error) {
	if req.MediaURL != "" {
		// Download from URL
		resp, err := http.Get(req.MediaURL)
		if err != nil {
			return nil, fmt.Errorf("failed to download media: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to download media: status %d", resp.StatusCode)
		}

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read media: %w", err)
		}

		// Auto-detect mime type if not provided
		if req.MimeType == "" {
			req.MimeType = http.DetectContentType(data)
		}

		// Auto-detect filename if not provided
		if req.FileName == "" && req.MediaURL != "" {
			req.FileName = filepath.Base(req.MediaURL)
			if req.FileName == "" || req.FileName == "." {
				ext := mime.TypeByExtension(req.MimeType)
				if ext == "" {
					ext = ".bin"
				}
				req.FileName = fmt.Sprintf("file_%d%s", time.Now().Unix(), ext)
			}
		}

		return data, nil
	} else if req.MediaBase64 != "" {
		// Decode from base64
		data, err := base64.StdEncoding.DecodeString(req.MediaBase64)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64: %w", err)
		}

		// Auto-detect mime type if not provided
		if req.MimeType == "" {
			req.MimeType = http.DetectContentType(data)
		}

		return data, nil
	}

	return nil, fmt.Errorf("no media data provided")
}

// logMessageEvent logs a message event
func (ms *MessageService) logMessageEvent(ctx context.Context, sessionID uuid.UUID, eventType string, data map[string]interface{}) {
	event := &models.WhatsAppEvent{
		SessionID: sessionID,
		EventType: eventType,
		EventData: data,
	}

	if err := ms.eventRepo.Create(ctx, event); err != nil {
		ms.logger.Error("Failed to log message event: %v", err)
	}
}
