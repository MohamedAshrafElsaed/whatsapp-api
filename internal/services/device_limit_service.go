package services

import (
	"context"
	"fmt"

	"whatsapp-api/internal/config"
	"whatsapp-api/internal/models"
	"whatsapp-api/internal/repositories"
)

// DeviceLimitService handles device limit checks and enforcement
type DeviceLimitService struct {
	sessionRepo *repositories.SessionRepository
	config      *config.Config
}

// NewDeviceLimitService creates a new device limit service
func NewDeviceLimitService(
	sessionRepo *repositories.SessionRepository,
	cfg *config.Config,
) *DeviceLimitService {
	return &DeviceLimitService{
		sessionRepo: sessionRepo,
		config:      cfg,
	}
}

// CanAddDevice checks if a user can add a new device
func (s *DeviceLimitService) CanAddDevice(ctx context.Context, userID int) (bool, error) {
	count, err := s.sessionRepo.CountActiveByUserID(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("failed to count active sessions: %w", err)
	}

	maxDevices := s.config.WhatsApp.MaxDevicesPerUser
	return count < int64(maxDevices), nil
}

// GetRemainingSlots returns the number of remaining device slots for a user
func (s *DeviceLimitService) GetRemainingSlots(ctx context.Context, userID int) (int, error) {
	count, err := s.sessionRepo.CountActiveByUserID(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to count active sessions: %w", err)
	}

	maxDevices := s.config.WhatsApp.MaxDevicesPerUser
	remaining := maxDevices - int(count)

	if remaining < 0 {
		remaining = 0
	}

	return remaining, nil
}

// GetUsedSlots returns the number of used device slots for a user
func (s *DeviceLimitService) GetUsedSlots(ctx context.Context, userID int) (int, error) {
	count, err := s.sessionRepo.CountActiveByUserID(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to count active sessions: %w", err)
	}

	return int(count), nil
}

// GetMaxDevices returns the maximum number of devices allowed per user
func (s *DeviceLimitService) GetMaxDevices() int {
	return s.config.WhatsApp.MaxDevicesPerUser
}

// ValidateDeviceLimit validates if adding a new device would exceed the limit
func (s *DeviceLimitService) ValidateDeviceLimit(ctx context.Context, userID int) error {
	canAdd, err := s.CanAddDevice(ctx, userID)
	if err != nil {
		return err
	}

	if !canAdd {
		return &DeviceLimitExceededError{
			UserID:     userID,
			MaxDevices: s.config.WhatsApp.MaxDevicesPerUser,
		}
	}

	return nil
}

// GetDeviceLimitInfo returns detailed device limit information for a user
func (s *DeviceLimitService) GetDeviceLimitInfo(ctx context.Context, userID int) (*DeviceLimitInfo, error) {
	activeCount, err := s.sessionRepo.CountActiveByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to count active sessions: %w", err)
	}

	connectedCount, err := s.sessionRepo.CountConnectedByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to count connected sessions: %w", err)
	}

	maxDevices := s.config.WhatsApp.MaxDevicesPerUser
	remaining := maxDevices - int(activeCount)
	if remaining < 0 {
		remaining = 0
	}

	return &DeviceLimitInfo{
		UserID:         userID,
		MaxDevices:     maxDevices,
		UsedSlots:      int(activeCount),
		ConnectedSlots: int(connectedCount),
		RemainingSlots: remaining,
		CanAddDevice:   remaining > 0,
	}, nil
}

// FindOldestSession finds the oldest session for a user (for potential replacement)
func (s *DeviceLimitService) FindOldestSession(ctx context.Context, userID int) (*models.WhatsAppSession, error) {
	sessions, err := s.sessionRepo.GetActiveByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active sessions: %w", err)
	}

	if len(sessions) == 0 {
		return nil, nil
	}

	// Find the oldest session by creation date
	oldest := sessions[0]
	for _, session := range sessions[1:] {
		if session.CreatedAt.Before(oldest.CreatedAt) {
			oldest = session
		}
	}

	return oldest, nil
}

// FindInactiveSession finds the most inactive session for a user
func (s *DeviceLimitService) FindInactiveSession(ctx context.Context, userID int) (*models.WhatsAppSession, error) {
	sessions, err := s.sessionRepo.GetActiveByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active sessions: %w", err)
	}

	if len(sessions) == 0 {
		return nil, nil
	}

	// Find the session with the oldest LastSeen
	var mostInactive *models.WhatsAppSession
	for _, session := range sessions {
		if session.LastSeen == nil {
			continue
		}

		if mostInactive == nil || (mostInactive.LastSeen != nil && session.LastSeen.Before(*mostInactive.LastSeen)) {
			mostInactive = session
		}
	}

	return mostInactive, nil
}

// GetDeviceLimitSuggestion suggests which session could be removed if limit is reached
func (s *DeviceLimitService) GetDeviceLimitSuggestion(ctx context.Context, userID int) (*DeviceLimitSuggestion, error) {
	canAdd, err := s.CanAddDevice(ctx, userID)
	if err != nil {
		return nil, err
	}

	if canAdd {
		return &DeviceLimitSuggestion{
			CanAddDevice: true,
			Reason:       "Slots available",
		}, nil
	}

	// Find inactive session
	inactiveSession, err := s.FindInactiveSession(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Find oldest session
	oldestSession, err := s.FindOldestSession(ctx, userID)
	if err != nil {
		return nil, err
	}

	suggestion := &DeviceLimitSuggestion{
		CanAddDevice: false,
		Reason:       "Device limit reached",
	}

	if inactiveSession != nil {
		suggestion.SuggestedSessionID = &inactiveSession.ID
		suggestion.SuggestedSessionName = &inactiveSession.SessionName
		suggestion.SuggestionReason = "Most inactive session"
	} else if oldestSession != nil {
		suggestion.SuggestedSessionID = &oldestSession.ID
		suggestion.SuggestedSessionName = &oldestSession.SessionName
		suggestion.SuggestionReason = "Oldest session"
	}

	return suggestion, nil
}

// CheckAndEnforceLimit checks the device limit and returns an error if exceeded
func (s *DeviceLimitService) CheckAndEnforceLimit(ctx context.Context, userID int) error {
	info, err := s.GetDeviceLimitInfo(ctx, userID)
	if err != nil {
		return err
	}

	if !info.CanAddDevice {
		return &DeviceLimitExceededError{
			UserID:     userID,
			MaxDevices: info.MaxDevices,
			UsedSlots:  info.UsedSlots,
		}
	}

	return nil
}

// IsWithinLimit checks if the user is within the device limit
func (s *DeviceLimitService) IsWithinLimit(ctx context.Context, userID int) (bool, error) {
	count, err := s.sessionRepo.CountActiveByUserID(ctx, userID)
	if err != nil {
		return false, err
	}

	return count < int64(s.config.WhatsApp.MaxDevicesPerUser), nil
}

// GetLimitPercentage returns the percentage of device slots used
func (s *DeviceLimitService) GetLimitPercentage(ctx context.Context, userID int) (float64, error) {
	count, err := s.sessionRepo.CountActiveByUserID(ctx, userID)
	if err != nil {
		return 0, err
	}

	maxDevices := s.config.WhatsApp.MaxDevicesPerUser
	if maxDevices == 0 {
		return 0, nil
	}

	percentage := (float64(count) / float64(maxDevices)) * 100
	return percentage, nil
}

// IsNearLimit checks if the user is near the device limit (>80%)
func (s *DeviceLimitService) IsNearLimit(ctx context.Context, userID int) (bool, error) {
	percentage, err := s.GetLimitPercentage(ctx, userID)
	if err != nil {
		return false, err
	}

	return percentage >= 80.0, nil
}

// DeviceLimitInfo represents device limit information
type DeviceLimitInfo struct {
	UserID         int  `json:"user_id"`
	MaxDevices     int  `json:"max_devices"`
	UsedSlots      int  `json:"used_slots"`
	ConnectedSlots int  `json:"connected_slots"`
	RemainingSlots int  `json:"remaining_slots"`
	CanAddDevice   bool `json:"can_add_device"`
}

// DeviceLimitSuggestion represents a suggestion for managing device limit
type DeviceLimitSuggestion struct {
	CanAddDevice         bool       `json:"can_add_device"`
	Reason               string     `json:"reason"`
	SuggestedSessionID   *uuid.UUID `json:"suggested_session_id,omitempty"`
	SuggestedSessionName *string    `json:"suggested_session_name,omitempty"`
	SuggestionReason     string     `json:"suggestion_reason,omitempty"`
}

// DeviceLimitExceededError represents a device limit exceeded error
type DeviceLimitExceededError struct {
	UserID     int
	MaxDevices int
	UsedSlots  int
}

// Error implements the error interface
func (e *DeviceLimitExceededError) Error() string {
	return fmt.Sprintf("device limit exceeded for user %d: using %d of %d devices",
		e.UserID, e.UsedSlots, e.MaxDevices)
}

// IsDeviceLimitError checks if an error is a device limit error
func IsDeviceLimitError(err error) bool {
	_, ok := err.(*DeviceLimitExceededError)
	return ok
}
