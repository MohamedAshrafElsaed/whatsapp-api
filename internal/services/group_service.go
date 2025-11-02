// internal/services/group_service.go
package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"

	"whatsapp-api/internal/models"
	"whatsapp-api/internal/repositories"
	"whatsapp-api/pkg/logger"
)

// GroupService handles WhatsApp group operations
type GroupService struct {
	sessionManager *SessionManager
	groupRepo      *repositories.GroupRepository
	eventRepo      *repositories.EventRepository
	logger         *logger.Logger
}

// NewGroupService creates a new group service
func NewGroupService(
	sessionManager *SessionManager,
	groupRepo *repositories.GroupRepository,
	eventRepo *repositories.EventRepository,
	logger *logger.Logger,
) *GroupService {
	return &GroupService{
		sessionManager: sessionManager,
		groupRepo:      groupRepo,
		eventRepo:      eventRepo,
		logger:         logger,
	}
}

// CreateGroup creates a new WhatsApp group
func (gs *GroupService) CreateGroup(ctx context.Context, sessionID uuid.UUID, req *models.CreateGroupRequest) (*models.GroupResponse, error) {
	client, err := gs.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Parse participant JIDs
	participantJIDs := make([]types.JID, 0, len(req.Participants))
	for _, participant := range req.Participants {
		jid, err := gs.parseJID(participant)
		if err != nil {
			gs.logger.Warn("Invalid participant JID %s: %v", participant, err)
			continue
		}
		participantJIDs = append(participantJIDs, jid)
	}

	// Create group request
	createReq := whatsmeow.ReqCreateGroup{
		Name:         req.Name,
		Participants: participantJIDs,
	}

	// Set group settings
	if req.IsAnnounceOnly {
		createReq.GroupAnnounce.IsAnnounce = true
		createReq.GroupAnnounce.Set = true
	}

	if req.IsLocked {
		createReq.GroupLocked.IsLocked = true
		createReq.GroupLocked.Set = true
	}

	// Create the group
	groupInfo, err := client.CreateGroup(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create group: %w", err)
	}

	// Store group in database
	group := &models.Group{
		SessionID:        sessionID,
		GroupJID:         groupInfo.JID.String(),
		Name:             groupInfo.Name,
		Topic:            groupInfo.Topic,
		OwnerJID:         groupInfo.OwnerJID.String(),
		CreatedAt:        time.Unix(groupInfo.GroupCreated, 0),
		ParticipantCount: len(groupInfo.Participants),
		IsAnnounceOnly:   groupInfo.IsAnnounce,
		IsLocked:         groupInfo.IsLocked,
	}

	if err := gs.groupRepo.Create(ctx, group); err != nil {
		gs.logger.Error("Failed to store group: %v", err)
	}

	// Store participants
	for _, participant := range groupInfo.Participants {
		gs.groupRepo.AddParticipant(ctx, group.ID, &models.GroupParticipant{
			GroupID:      group.ID,
			UserJID:      participant.JID.String(),
			IsAdmin:      participant.IsAdmin,
			IsSuperAdmin: participant.IsSuperAdmin,
		})
	}

	// Log event
	gs.logGroupEvent(ctx, sessionID, "group_created", map[string]interface{}{
		"group_jid":    groupInfo.JID.String(),
		"name":         req.Name,
		"participants": len(participantJIDs),
	})

	return gs.convertGroupInfoToResponse(groupInfo), nil
}

// GetGroups retrieves all groups for a session
func (gs *GroupService) GetGroups(ctx context.Context, sessionID uuid.UUID) ([]*models.GroupResponse, error) {
	client, err := gs.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Get joined groups from WhatsApp
	groups, err := client.GetJoinedGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get groups: %w", err)
	}

	responses := make([]*models.GroupResponse, 0, len(groups))
	for _, group := range groups {
		// Store or update group in database
		dbGroup := &models.Group{
			SessionID:        sessionID,
			GroupJID:         group.JID.String(),
			Name:             group.Name,
			Topic:            group.Topic,
			OwnerJID:         group.OwnerJID.String(),
			CreatedAt:        time.Unix(group.GroupCreated, 0),
			ParticipantCount: len(group.Participants),
			IsAnnounceOnly:   group.IsAnnounce,
			IsLocked:         group.IsLocked,
		}

		if err := gs.groupRepo.Upsert(ctx, dbGroup); err != nil {
			gs.logger.Error("Failed to store group: %v", err)
		}

		responses = append(responses, gs.convertGroupInfoToResponse(group))
	}

	return responses, nil
}

// GetGroupInfo retrieves information about a specific group
func (gs *GroupService) GetGroupInfo(ctx context.Context, sessionID uuid.UUID, groupJID string) (*models.GroupDetailResponse, error) {
	client, err := gs.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Parse group JID
	jid, err := gs.parseJID(groupJID)
	if err != nil {
		return nil, fmt.Errorf("invalid group JID: %w", err)
	}

	// Get group info from WhatsApp
	groupInfo, err := client.GetGroupInfo(ctx, jid)
	if err != nil {
		return nil, fmt.Errorf("failed to get group info: %w", err)
	}

	// Build participant list
	participants := make([]models.GroupParticipantInfo, 0, len(groupInfo.Participants))
	for _, p := range groupInfo.Participants {
		participants = append(participants, models.GroupParticipantInfo{
			JID:          p.JID.String(),
			DisplayName:  p.DisplayName,
			IsAdmin:      p.IsAdmin,
			IsSuperAdmin: p.IsSuperAdmin,
			Error:        int(p.Error),
		})
	}

	return &models.GroupDetailResponse{
		JID:              groupInfo.JID.String(),
		Name:             groupInfo.Name,
		Topic:            groupInfo.Topic,
		OwnerJID:         groupInfo.OwnerJID.String(),
		CreatedAt:        time.Unix(groupInfo.GroupCreated, 0),
		ParticipantCount: len(groupInfo.Participants),
		Participants:     participants,
		IsAnnounceOnly:   groupInfo.IsAnnounce,
		IsLocked:         groupInfo.IsLocked,
		IsEphemeral:      groupInfo.IsEphemeral,
		InviteLink:       groupInfo.InviteLink,
	}, nil
}

// JoinGroup joins a group using an invite link
func (gs *GroupService) JoinGroup(ctx context.Context, sessionID uuid.UUID, inviteLink string) (*models.GroupResponse, error) {
	client, err := gs.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Extract invite code from link
	inviteCode := gs.extractInviteCode(inviteLink)
	if inviteCode == "" {
		return nil, fmt.Errorf("invalid invite link")
	}

	// Get group info from invite link
	groupInfo, err := client.GetGroupInfoFromLink(ctx, inviteCode)
	if err != nil {
		return nil, fmt.Errorf("failed to get group info from link: %w", err)
	}

	// Join the group
	jid, err := client.JoinGroupWithLink(ctx, inviteCode)
	if err != nil {
		return nil, fmt.Errorf("failed to join group: %w", err)
	}

	// Get full group info
	fullGroupInfo, err := client.GetGroupInfo(ctx, jid)
	if err != nil {
		// Use basic info if full info fails
		fullGroupInfo = groupInfo
	}

	// Store group in database
	group := &models.Group{
		SessionID:        sessionID,
		GroupJID:         jid.String(),
		Name:             fullGroupInfo.Name,
		Topic:            fullGroupInfo.Topic,
		OwnerJID:         fullGroupInfo.OwnerJID.String(),
		CreatedAt:        time.Unix(fullGroupInfo.GroupCreated, 0),
		ParticipantCount: len(fullGroupInfo.Participants),
		IsAnnounceOnly:   fullGroupInfo.IsAnnounce,
		IsLocked:         fullGroupInfo.IsLocked,
	}

	if err := gs.groupRepo.Create(ctx, group); err != nil {
		gs.logger.Error("Failed to store group: %v", err)
	}

	// Log event
	gs.logGroupEvent(ctx, sessionID, "group_joined", map[string]interface{}{
		"group_jid": jid.String(),
		"name":      fullGroupInfo.Name,
		"via":       "invite_link",
	})

	return gs.convertGroupInfoToResponse(fullGroupInfo), nil
}

// LeaveGroup leaves a group
func (gs *GroupService) LeaveGroup(ctx context.Context, sessionID uuid.UUID, groupJID string) error {
	client, err := gs.sessionManager.GetClient(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return fmt.Errorf("session not logged in")
	}

	// Parse group JID
	jid, err := gs.parseJID(groupJID)
	if err != nil {
		return fmt.Errorf("invalid group JID: %w", err)
	}

	// Leave the group
	if err := client.LeaveGroup(ctx, jid); err != nil {
		return fmt.Errorf("failed to leave group: %w", err)
	}

	// Remove from database
	if err := gs.groupRepo.DeleteByJID(ctx, sessionID, groupJID); err != nil {
		gs.logger.Error("Failed to remove group from database: %v", err)
	}

	// Log event
	gs.logGroupEvent(ctx, sessionID, "group_left", map[string]interface{}{
		"group_jid": groupJID,
	})

	return nil
}

// UpdateGroupInfo updates group information
func (gs *GroupService) UpdateGroupInfo(ctx context.Context, sessionID uuid.UUID, groupJID string, req *models.UpdateGroupRequest) error {
	client, err := gs.sessionManager.GetClient(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return fmt.Errorf("session not logged in")
	}

	// Parse group JID
	jid, err := gs.parseJID(groupJID)
	if err != nil {
		return fmt.Errorf("invalid group JID: %w", err)
	}

	// Update group name if provided
	if req.Name != "" {
		if err := client.SetGroupName(ctx, jid, req.Name); err != nil {
			return fmt.Errorf("failed to update group name: %w", err)
		}
	}

	// Update group description/topic if provided
	if req.Topic != "" {
		if err := client.SetGroupTopic(ctx, jid, "", "", req.Topic); err != nil {
			return fmt.Errorf("failed to update group topic: %w", err)
		}
	}

	// Update announce-only setting if provided
	if req.IsAnnounceOnly != nil {
		if err := client.SetGroupAnnounce(ctx, jid, *req.IsAnnounceOnly); err != nil {
			return fmt.Errorf("failed to update announce setting: %w", err)
		}
	}

	// Update locked setting if provided
	if req.IsLocked != nil {
		if err := client.SetGroupLocked(ctx, jid, *req.IsLocked); err != nil {
			return fmt.Errorf("failed to update locked setting: %w", err)
		}
	}

	// Log event
	gs.logGroupEvent(ctx, sessionID, "group_updated", map[string]interface{}{
		"group_jid": groupJID,
		"updates":   req,
	})

	return nil
}

// UpdateGroupPhoto updates the group profile picture
func (gs *GroupService) UpdateGroupPhoto(ctx context.Context, sessionID uuid.UUID, groupJID string, req *models.UpdateGroupPhotoRequest) error {
	client, err := gs.sessionManager.GetClient(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return fmt.Errorf("session not logged in")
	}

	// Parse group JID
	jid, err := gs.parseJID(groupJID)
	if err != nil {
		return fmt.Errorf("invalid group JID: %w", err)
	}

	// Get photo data
	var photoData []byte
	if req.PhotoURL != "" {
		// Download from URL
		resp, err := http.Get(req.PhotoURL)
		if err != nil {
			return fmt.Errorf("failed to download photo: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to download photo: status %d", resp.StatusCode)
		}

		photoData = make([]byte, resp.ContentLength)
		_, err = resp.Body.Read(photoData)
		if err != nil {
			return fmt.Errorf("failed to read photo: %w", err)
		}
	} else if req.PhotoBase64 != "" {
		// Decode from base64
		photoData, err = base64.StdEncoding.DecodeString(req.PhotoBase64)
		if err != nil {
			return fmt.Errorf("failed to decode base64: %w", err)
		}
	} else {
		return fmt.Errorf("no photo data provided")
	}

	// Set group photo
	pictureID, err := client.SetGroupPhoto(ctx, jid, photoData)
	if err != nil {
		return fmt.Errorf("failed to set group photo: %w", err)
	}

	// Log event
	gs.logGroupEvent(ctx, sessionID, "group_photo_updated", map[string]interface{}{
		"group_jid":  groupJID,
		"picture_id": pictureID,
	})

	return nil
}

// AddParticipants adds participants to a group
func (gs *GroupService) AddParticipants(ctx context.Context, sessionID uuid.UUID, groupJID string, req *models.GroupParticipantsRequest) (*models.GroupParticipantsResponse, error) {
	client, err := gs.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Parse group JID
	groupJid, err := gs.parseJID(groupJID)
	if err != nil {
		return nil, fmt.Errorf("invalid group JID: %w", err)
	}

	// Parse participant JIDs
	participantJIDs := make([]types.JID, 0, len(req.Participants))
	for _, participant := range req.Participants {
		jid, err := gs.parseJID(participant)
		if err != nil {
			gs.logger.Warn("Invalid participant JID %s: %v", participant, err)
			continue
		}
		participantJIDs = append(participantJIDs, jid)
	}

	// Add participants
	results, err := client.UpdateGroupParticipants(ctx, groupJid, participantJIDs, whatsmeow.ParticipantChangeAdd)
	if err != nil {
		return nil, fmt.Errorf("failed to add participants: %w", err)
	}

	// Build response
	response := &models.GroupParticipantsResponse{
		Success: make([]string, 0),
		Failed:  make([]models.FailedParticipant, 0),
	}

	for _, result := range results {
		if result.Error == 0 {
			response.Success = append(response.Success, result.JID.String())

			// Add to database
			gs.groupRepo.AddParticipant(ctx, 0, &models.GroupParticipant{
				GroupJID: groupJID,
				UserJID:  result.JID.String(),
				IsAdmin:  false,
			})
		} else {
			response.Failed = append(response.Failed, models.FailedParticipant{
				JID:   result.JID.String(),
				Error: gs.getParticipantErrorMessage(result.Error),
			})
		}
	}

	// Log event
	gs.logGroupEvent(ctx, sessionID, "participants_added", map[string]interface{}{
		"group_jid":    groupJID,
		"added_count":  len(response.Success),
		"failed_count": len(response.Failed),
	})

	return response, nil
}

// RemoveParticipants removes participants from a group
func (gs *GroupService) RemoveParticipants(ctx context.Context, sessionID uuid.UUID, groupJID string, req *models.GroupParticipantsRequest) (*models.GroupParticipantsResponse, error) {
	client, err := gs.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Parse group JID
	groupJid, err := gs.parseJID(groupJID)
	if err != nil {
		return nil, fmt.Errorf("invalid group JID: %w", err)
	}

	// Parse participant JIDs
	participantJIDs := make([]types.JID, 0, len(req.Participants))
	for _, participant := range req.Participants {
		jid, err := gs.parseJID(participant)
		if err != nil {
			gs.logger.Warn("Invalid participant JID %s: %v", participant, err)
			continue
		}
		participantJIDs = append(participantJIDs, jid)
	}

	// Remove participants
	results, err := client.UpdateGroupParticipants(ctx, groupJid, participantJIDs, whatsmeow.ParticipantChangeRemove)
	if err != nil {
		return nil, fmt.Errorf("failed to remove participants: %w", err)
	}

	// Build response
	response := &models.GroupParticipantsResponse{
		Success: make([]string, 0),
		Failed:  make([]models.FailedParticipant, 0),
	}

	for _, result := range results {
		if result.Error == 0 {
			response.Success = append(response.Success, result.JID.String())

			// Remove from database
			gs.groupRepo.RemoveParticipant(ctx, groupJID, result.JID.String())
		} else {
			response.Failed = append(response.Failed, models.FailedParticipant{
				JID:   result.JID.String(),
				Error: gs.getParticipantErrorMessage(result.Error),
			})
		}
	}

	// Log event
	gs.logGroupEvent(ctx, sessionID, "participants_removed", map[string]interface{}{
		"group_jid":     groupJID,
		"removed_count": len(response.Success),
		"failed_count":  len(response.Failed),
	})

	return response, nil
}

// PromoteParticipants promotes participants to admin
func (gs *GroupService) PromoteParticipants(ctx context.Context, sessionID uuid.UUID, groupJID string, req *models.GroupParticipantsRequest) (*models.GroupParticipantsResponse, error) {
	client, err := gs.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Parse group JID
	groupJid, err := gs.parseJID(groupJID)
	if err != nil {
		return nil, fmt.Errorf("invalid group JID: %w", err)
	}

	// Parse participant JIDs
	participantJIDs := make([]types.JID, 0, len(req.Participants))
	for _, participant := range req.Participants {
		jid, err := gs.parseJID(participant)
		if err != nil {
			gs.logger.Warn("Invalid participant JID %s: %v", participant, err)
			continue
		}
		participantJIDs = append(participantJIDs, jid)
	}

	// Promote participants
	results, err := client.UpdateGroupParticipants(ctx, groupJid, participantJIDs, whatsmeow.ParticipantChangePromote)
	if err != nil {
		return nil, fmt.Errorf("failed to promote participants: %w", err)
	}

	// Build response
	response := &models.GroupParticipantsResponse{
		Success: make([]string, 0),
		Failed:  make([]models.FailedParticipant, 0),
	}

	for _, result := range results {
		if result.Error == 0 {
			response.Success = append(response.Success, result.JID.String())

			// Update in database
			gs.groupRepo.UpdateParticipantRole(ctx, groupJID, result.JID.String(), true)
		} else {
			response.Failed = append(response.Failed, models.FailedParticipant{
				JID:   result.JID.String(),
				Error: gs.getParticipantErrorMessage(result.Error),
			})
		}
	}

	// Log event
	gs.logGroupEvent(ctx, sessionID, "participants_promoted", map[string]interface{}{
		"group_jid":      groupJID,
		"promoted_count": len(response.Success),
		"failed_count":   len(response.Failed),
	})

	return response, nil
}

// DemoteParticipants demotes participants from admin
func (gs *GroupService) DemoteParticipants(ctx context.Context, sessionID uuid.UUID, groupJID string, req *models.GroupParticipantsRequest) (*models.GroupParticipantsResponse, error) {
	client, err := gs.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Parse group JID
	groupJid, err := gs.parseJID(groupJID)
	if err != nil {
		return nil, fmt.Errorf("invalid group JID: %w", err)
	}

	// Parse participant JIDs
	participantJIDs := make([]types.JID, 0, len(req.Participants))
	for _, participant := range req.Participants {
		jid, err := gs.parseJID(participant)
		if err != nil {
			gs.logger.Warn("Invalid participant JID %s: %v", participant, err)
			continue
		}
		participantJIDs = append(participantJIDs, jid)
	}

	// Demote participants
	results, err := client.UpdateGroupParticipants(ctx, groupJid, participantJIDs, whatsmeow.ParticipantChangeDemote)
	if err != nil {
		return nil, fmt.Errorf("failed to demote participants: %w", err)
	}

	// Build response
	response := &models.GroupParticipantsResponse{
		Success: make([]string, 0),
		Failed:  make([]models.FailedParticipant, 0),
	}

	for _, result := range results {
		if result.Error == 0 {
			response.Success = append(response.Success, result.JID.String())

			// Update in database
			gs.groupRepo.UpdateParticipantRole(ctx, groupJID, result.JID.String(), false)
		} else {
			response.Failed = append(response.Failed, models.FailedParticipant{
				JID:   result.JID.String(),
				Error: gs.getParticipantErrorMessage(result.Error),
			})
		}
	}

	// Log event
	gs.logGroupEvent(ctx, sessionID, "participants_demoted", map[string]interface{}{
		"group_jid":     groupJID,
		"demoted_count": len(response.Success),
		"failed_count":  len(response.Failed),
	})

	return response, nil
}

// GetGroupInviteLink gets the invite link for a group
func (gs *GroupService) GetGroupInviteLink(ctx context.Context, sessionID uuid.UUID, groupJID string, reset bool) (string, error) {
	client, err := gs.sessionManager.GetClient(sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return "", fmt.Errorf("session not logged in")
	}

	// Parse group JID
	jid, err := gs.parseJID(groupJID)
	if err != nil {
		return "", fmt.Errorf("invalid group JID: %w", err)
	}

	// Get or reset invite link
	link, err := client.GetGroupInviteLink(ctx, jid, reset)
	if err != nil {
		return "", fmt.Errorf("failed to get invite link: %w", err)
	}

	// Log event
	gs.logGroupEvent(ctx, sessionID, "invite_link_retrieved", map[string]interface{}{
		"group_jid": groupJID,
		"reset":     reset,
	})

	return link, nil
}

// Helper functions

// parseJID parses a phone number or JID string into a WhatsApp JID
func (gs *GroupService) parseJID(input string) (types.JID, error) {
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
	if strings.HasSuffix(cleaned, "_group") || strings.HasPrefix(cleaned, "g_") {
		groupID := strings.TrimSuffix(cleaned, "_group")
		groupID = strings.TrimPrefix(groupID, "g_")
		return types.ParseJID(groupID + "@g.us")
	}

	// It's a phone number, convert to JID
	return types.ParseJID(cleaned + "@s.whatsapp.net")
}

// extractInviteCode extracts the invite code from a WhatsApp group link
func (gs *GroupService) extractInviteCode(link string) string {
	// Handle different link formats
	link = strings.TrimSpace(link)

	// Remove protocol
	link = strings.TrimPrefix(link, "https://")
	link = strings.TrimPrefix(link, "http://")

	// Remove domain
	link = strings.TrimPrefix(link, "chat.whatsapp.com/")
	link = strings.TrimPrefix(link, "wa.me/")

	// The remaining part should be the invite code
	parts := strings.Split(link, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return link
}

// convertGroupInfoToResponse converts WhatsApp group info to API response
func (gs *GroupService) convertGroupInfoToResponse(info *types.GroupInfo) *models.GroupResponse {
	return &models.GroupResponse{
		JID:              info.JID.String(),
		Name:             info.Name,
		Topic:            info.Topic,
		OwnerJID:         info.OwnerJID.String(),
		CreatedAt:        time.Unix(info.GroupCreated, 0),
		ParticipantCount: len(info.Participants),
		IsAnnounceOnly:   info.IsAnnounce,
		IsLocked:         info.IsLocked,
		IsEphemeral:      info.IsEphemeral,
	}
}

// getParticipantErrorMessage returns a human-readable error message for participant operations
func (gs *GroupService) getParticipantErrorMessage(errorCode types.GroupParticipantAddError) string {
	switch errorCode {
	case 403:
		return "Not authorized to add participant"
	case 404:
		return "Participant not found"
	case 408:
		return "Request timed out"
	case 409:
		return "Participant already in group"
	case 500:
		return "Internal server error"
	default:
		return fmt.Sprintf("Unknown error: %d", errorCode)
	}
}

// logGroupEvent logs a group-related event
func (gs *GroupService) logGroupEvent(ctx context.Context, sessionID uuid.UUID, eventType string, data map[string]interface{}) {
	event := &models.WhatsAppEvent{
		SessionID: sessionID,
		EventType: eventType,
		EventData: data,
	}

	if err := gs.eventRepo.Create(ctx, event); err != nil {
		gs.logger.Error("Failed to log group event: %v", err)
	}
}
