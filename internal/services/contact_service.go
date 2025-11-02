package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	_ "time"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"

	"whatsapp-api/internal/models"
	"whatsapp-api/internal/repositories"
	"whatsapp-api/pkg/logger"
)

// ContactService handles WhatsApp contact operations
type ContactService struct {
	sessionManager *SessionManager
	contactRepo    *repositories.ContactRepository
	eventRepo      *repositories.EventRepository
	logger         *logger.Logger
}

// NewContactService creates a new contact service
func NewContactService(
	sessionManager *SessionManager,
	contactRepo *repositories.ContactRepository,
	eventRepo *repositories.EventRepository,
	logger *logger.Logger,
) *ContactService {
	return &ContactService{
		sessionManager: sessionManager,
		contactRepo:    contactRepo,
		eventRepo:      eventRepo,
		logger:         logger,
	}
}

// SyncContacts synchronizes contacts with WhatsApp
func (cs *ContactService) SyncContacts(ctx context.Context, sessionID uuid.UUID) (*models.ContactSyncResponse, error) {
	client, err := cs.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Get all contacts from WhatsApp store
	contacts, err := client.Store.Contacts.GetAllContacts()
	if err != nil {
		return nil, fmt.Errorf("failed to get contacts: %w", err)
	}

	syncedCount := 0
	failedCount := 0
	var syncedContacts []models.ContactInfo

	for jid, contact := range contacts {
		// Skip non-user contacts (groups, broadcast lists, etc.)
		if jid.Server != types.DefaultUserServer {
			continue
		}

		// Create contact model
		dbContact := &models.Contact{
			SessionID:    sessionID,
			JID:          jid.String(),
			Name:         contact.FullName,
			BusinessName: contact.BusinessName,
			PushName:     contact.PushName,
			PhoneNumber:  jid.User,
			IsBlocked:    false, // Will be updated separately
			IsEnterprise: contact.VerifiedName != nil,
			LastSeen:     nil, // Will be updated when presence is received
		}

		// Store or update contact in database
		if err := cs.contactRepo.Upsert(ctx, dbContact); err != nil {
			cs.logger.Error("Failed to store contact %s: %v", jid, err)
			failedCount++
			continue
		}

		syncedCount++
		syncedContacts = append(syncedContacts, models.ContactInfo{
			JID:          jid.String(),
			Name:         contact.FullName,
			BusinessName: contact.BusinessName,
			PushName:     contact.PushName,
			PhoneNumber:  jid.User,
		})
	}

	// Log event
	cs.logContactEvent(ctx, sessionID, "contacts_synced", map[string]interface{}{
		"total_count":  len(contacts),
		"synced_count": syncedCount,
		"failed_count": failedCount,
	})

	return &models.ContactSyncResponse{
		TotalContacts: len(contacts),
		SyncedCount:   syncedCount,
		FailedCount:   failedCount,
		Contacts:      syncedContacts,
	}, nil
}

// GetContacts retrieves all contacts for a session
func (cs *ContactService) GetContacts(ctx context.Context, sessionID uuid.UUID, filter *models.ContactFilter) ([]*models.Contact, error) {
	return cs.contactRepo.GetBySession(ctx, sessionID, filter)
}

// GetContact retrieves a specific contact
func (cs *ContactService) GetContact(ctx context.Context, sessionID uuid.UUID, jid string) (*models.Contact, error) {
	return cs.contactRepo.GetByJID(ctx, sessionID, jid)
}

// GetContactProfile retrieves detailed profile information for a contact
func (cs *ContactService) GetContactProfile(ctx context.Context, sessionID uuid.UUID, jid string) (*models.ContactProfile, error) {
	client, err := cs.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Parse JID
	contactJID, err := cs.parseJID(jid)
	if err != nil {
		return nil, fmt.Errorf("invalid JID: %w", err)
	}

	// Get user info from WhatsApp
	userInfoMap, err := client.GetUserInfo(ctx, []types.JID{contactJID})
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	userInfo, exists := userInfoMap[contactJID]
	if !exists {
		return nil, fmt.Errorf("user info not found")
	}

	// Get contact from local store
	contact, _ := client.Store.Contacts.GetContact(contactJID)

	// Get profile picture
	var profilePictureURL string
	profilePic, err := client.GetProfilePictureInfo(ctx, contactJID, &whatsmeow.GetProfilePictureParams{})
	if err == nil && profilePic != nil {
		profilePictureURL = profilePic.URL
	}

	// Get status/about
	var statusText string
	if userInfo.Status != nil {
		statusText = *userInfo.Status
	}

	// Build profile response
	profile := &models.ContactProfile{
		JID:            contactJID.String(),
		Name:           contact.FullName,
		BusinessName:   contact.BusinessName,
		PushName:       contact.PushName,
		PhoneNumber:    contactJID.User,
		About:          statusText,
		ProfilePicture: profilePictureURL,
		IsBlocked:      false, // Will be checked separately
		IsEnterprise:   userInfo.VerifiedName != nil,
		IsBusiness:     contact.BusinessName != "",
	}

	// If business account, add verified info
	if userInfo.VerifiedName != nil {
		profile.VerifiedName = &models.VerifiedBusinessInfo{
			Certificate: userInfo.VerifiedName.Certificate.String(),
			Details:     userInfo.VerifiedName.Details,
		}
	}

	// Update contact in database
	dbContact := &models.Contact{
		SessionID:      sessionID,
		JID:            contactJID.String(),
		Name:           contact.FullName,
		BusinessName:   contact.BusinessName,
		PushName:       contact.PushName,
		PhoneNumber:    contactJID.User,
		About:          &statusText,
		ProfilePicture: &profilePictureURL,
		IsEnterprise:   userInfo.VerifiedName != nil,
	}
	cs.contactRepo.Upsert(ctx, dbContact)

	return profile, nil
}

// GetProfilePicture retrieves the profile picture of a contact
func (cs *ContactService) GetProfilePicture(ctx context.Context, sessionID uuid.UUID, jid string) (*models.ProfilePictureResponse, error) {
	client, err := cs.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Parse JID
	contactJID, err := cs.parseJID(jid)
	if err != nil {
		return nil, fmt.Errorf("invalid JID: %w", err)
	}

	// Get profile picture info
	profilePic, err := client.GetProfilePictureInfo(ctx, contactJID, &whatsmeow.GetProfilePictureParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to get profile picture: %w", err)
	}

	if profilePic == nil {
		return nil, fmt.Errorf("no profile picture found")
	}

	// Download the actual image if needed
	// Note: This would require additional HTTP request to profilePic.URL
	// For now, we'll just return the URL

	return &models.ProfilePictureResponse{
		URL:        profilePic.URL,
		ID:         profilePic.ID,
		Type:       profilePic.Type,
		DirectPath: profilePic.DirectPath,
	}, nil
}

// CheckContactsExist checks if phone numbers are registered on WhatsApp
func (cs *ContactService) CheckContactsExist(ctx context.Context, sessionID uuid.UUID, phoneNumbers []string) (*models.ContactExistResponse, error) {
	client, err := cs.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Clean phone numbers
	cleanedNumbers := make([]string, 0, len(phoneNumbers))
	for _, phone := range phoneNumbers {
		cleaned := cs.cleanPhoneNumber(phone)
		if cleaned != "" {
			cleanedNumbers = append(cleanedNumbers, cleaned)
		}
	}

	// Check if numbers exist on WhatsApp
	results, err := client.IsOnWhatsApp(ctx, cleanedNumbers)
	if err != nil {
		return nil, fmt.Errorf("failed to check contacts: %w", err)
	}

	// Build response
	response := &models.ContactExistResponse{
		Results: make([]models.ContactExistResult, 0, len(results)),
	}

	for _, result := range results {
		existResult := models.ContactExistResult{
			PhoneNumber: result.Query,
			Exists:      result.IsIn,
		}

		if result.IsIn && result.JID != nil {
			existResult.JID = result.JID.String()
		}

		response.Results = append(response.Results, existResult)
	}

	// Log event
	cs.logContactEvent(ctx, sessionID, "contacts_checked", map[string]interface{}{
		"checked_count": len(cleanedNumbers),
		"exists_count":  len(response.Results),
	})

	return response, nil
}

// BlockContact blocks a contact
func (cs *ContactService) BlockContact(ctx context.Context, sessionID uuid.UUID, jid string) error {
	client, err := cs.sessionManager.GetClient(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return fmt.Errorf("session not logged in")
	}

	// Parse JID
	contactJID, err := cs.parseJID(jid)
	if err != nil {
		return fmt.Errorf("invalid JID: %w", err)
	}

	// Block contact
	_, err = client.UpdateBlocklist(ctx, contactJID, types.BlocklistChangeActionBlock)
	if err != nil {
		return fmt.Errorf("failed to block contact: %w", err)
	}

	// Update database
	contact, _ := cs.contactRepo.GetByJID(ctx, sessionID, jid)
	if contact != nil {
		contact.IsBlocked = true
		cs.contactRepo.Update(ctx, contact)
	}

	// Log event
	cs.logContactEvent(ctx, sessionID, "contact_blocked", map[string]interface{}{
		"jid": jid,
	})

	return nil
}

// UnblockContact unblocks a contact
func (cs *ContactService) UnblockContact(ctx context.Context, sessionID uuid.UUID, jid string) error {
	client, err := cs.sessionManager.GetClient(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return fmt.Errorf("session not logged in")
	}

	// Parse JID
	contactJID, err := cs.parseJID(jid)
	if err != nil {
		return fmt.Errorf("invalid JID: %w", err)
	}

	// Unblock contact
	_, err = client.UpdateBlocklist(ctx, contactJID, types.BlocklistChangeActionUnblock)
	if err != nil {
		return fmt.Errorf("failed to unblock contact: %w", err)
	}

	// Update database
	contact, _ := cs.contactRepo.GetByJID(ctx, sessionID, jid)
	if contact != nil {
		contact.IsBlocked = false
		cs.contactRepo.Update(ctx, contact)
	}

	// Log event
	cs.logContactEvent(ctx, sessionID, "contact_unblocked", map[string]interface{}{
		"jid": jid,
	})

	return nil
}

// GetBlockedContacts retrieves all blocked contacts
func (cs *ContactService) GetBlockedContacts(ctx context.Context, sessionID uuid.UUID) ([]*models.Contact, error) {
	client, err := cs.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Get blocklist from WhatsApp
	blocklist, err := client.GetBlocklist(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get blocklist: %w", err)
	}

	// Update local database
	for _, blockedJID := range blocklist.DHash {
		jidStr := blockedJID.String()
		contact, _ := cs.contactRepo.GetByJID(ctx, sessionID, jidStr)
		if contact == nil {
			// Create new contact entry if doesn't exist
			contact = &models.Contact{
				SessionID:   sessionID,
				JID:         jidStr,
				PhoneNumber: blockedJID.User,
				IsBlocked:   true,
			}
			cs.contactRepo.Create(ctx, contact)
		} else {
			// Update existing contact
			contact.IsBlocked = true
			cs.contactRepo.Update(ctx, contact)
		}
	}

	// Get all blocked contacts from database
	return cs.contactRepo.GetBlockedContacts(ctx, sessionID)
}

// SubscribePresence subscribes to presence updates for a contact
func (cs *ContactService) SubscribePresence(ctx context.Context, sessionID uuid.UUID, jid string) error {
	client, err := cs.sessionManager.GetClient(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return fmt.Errorf("session not logged in")
	}

	// Parse JID
	contactJID, err := cs.parseJID(jid)
	if err != nil {
		return fmt.Errorf("invalid JID: %w", err)
	}

	// Subscribe to presence
	if err := client.SubscribePresence(ctx, contactJID); err != nil {
		return fmt.Errorf("failed to subscribe to presence: %w", err)
	}

	// Log event
	cs.logContactEvent(ctx, sessionID, "presence_subscribed", map[string]interface{}{
		"jid": jid,
	})

	return nil
}

// GetPresence gets the current presence/online status of a contact
func (cs *ContactService) GetPresence(ctx context.Context, sessionID uuid.UUID, jid string) (*models.PresenceInfo, error) {
	// Get contact from database
	contact, err := cs.contactRepo.GetByJID(ctx, sessionID, jid)
	if err != nil {
		return nil, fmt.Errorf("contact not found: %w", err)
	}

	presence := &models.PresenceInfo{
		JID:       contact.JID,
		Available: false,
	}

	if contact.LastSeen != nil {
		presence.LastSeen = contact.LastSeen
	}

	// Note: Real-time presence would come from WhatsApp events
	// This just returns the last known status from database

	return presence, nil
}

// UpdateContactName updates the local name for a contact
func (cs *ContactService) UpdateContactName(ctx context.Context, sessionID uuid.UUID, jid string, name string) error {
	// Get contact from database
	contact, err := cs.contactRepo.GetByJID(ctx, sessionID, jid)
	if err != nil {
		return fmt.Errorf("contact not found: %w", err)
	}

	// Update name
	contact.Name = name
	if err := cs.contactRepo.Update(ctx, contact); err != nil {
		return fmt.Errorf("failed to update contact: %w", err)
	}

	// Log event
	cs.logContactEvent(ctx, sessionID, "contact_name_updated", map[string]interface{}{
		"jid":  jid,
		"name": name,
	})

	return nil
}

// DeleteContact deletes a contact from local storage
func (cs *ContactService) DeleteContact(ctx context.Context, sessionID uuid.UUID, jid string) error {
	// Delete from database
	if err := cs.contactRepo.Delete(ctx, sessionID, jid); err != nil {
		return fmt.Errorf("failed to delete contact: %w", err)
	}

	// Log event
	cs.logContactEvent(ctx, sessionID, "contact_deleted", map[string]interface{}{
		"jid": jid,
	})

	return nil
}

// GetCommonGroups gets groups shared with a contact
func (cs *ContactService) GetCommonGroups(ctx context.Context, sessionID uuid.UUID, jid string) ([]models.GroupInfo, error) {
	client, err := cs.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Parse JID
	contactJID, err := cs.parseJID(jid)
	if err != nil {
		return nil, fmt.Errorf("invalid JID: %w", err)
	}

	// Get all groups
	groups, err := client.GetJoinedGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get groups: %w", err)
	}

	// Find common groups
	var commonGroups []models.GroupInfo
	for _, group := range groups {
		// Check if contact is in this group
		for _, participant := range group.Participants {
			if participant.JID.String() == contactJID.String() {
				commonGroups = append(commonGroups, models.GroupInfo{
					JID:  group.JID.String(),
					Name: group.Name,
				})
				break
			}
		}
	}

	return commonGroups, nil
}

// GetBusinessCatalog gets the product catalog for a business contact
func (cs *ContactService) GetBusinessCatalog(ctx context.Context, sessionID uuid.UUID, jid string) (*models.BusinessCatalog, error) {
	client, err := cs.sessionManager.GetClient(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("session not logged in")
	}

	// Parse JID
	businessJID, err := cs.parseJID(jid)
	if err != nil {
		return nil, fmt.Errorf("invalid JID: %w", err)
	}

	// Get business profile
	profile, err := client.GetBusinessProfile(ctx, businessJID)
	if err != nil {
		return nil, fmt.Errorf("failed to get business profile: %w", err)
	}

	catalog := &models.BusinessCatalog{
		BusinessJID:  businessJID.String(),
		BusinessName: profile.Name,
		Description:  profile.Description,
		Category:     profile.Category,
		Email:        profile.Email,
		Website:      []string{profile.Website},
		Address:      profile.Address,
	}

	// Note: Product catalog would require additional WhatsApp Business API calls
	// This is a placeholder for the catalog structure

	return catalog, nil
}

// Helper functions

// parseJID parses a phone number or JID string into a WhatsApp JID
func (cs *ContactService) parseJID(input string) (types.JID, error) {
	// Clean the input
	cleaned := cs.cleanPhoneNumber(input)

	// Check if it's already a JID
	if strings.Contains(cleaned, "@") {
		return types.ParseJID(cleaned)
	}

	// It's a phone number, convert to JID
	return types.ParseJID(cleaned + "@s.whatsapp.net")
}

// cleanPhoneNumber cleans a phone number by removing special characters
func (cs *ContactService) cleanPhoneNumber(phone string) string {
	// Remove common formatting characters
	cleaned := strings.ReplaceAll(phone, "+", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "(", "")
	cleaned = strings.ReplaceAll(cleaned, ")", "")
	cleaned = strings.ReplaceAll(cleaned, ".", "")

	return cleaned
}

// logContactEvent logs a contact-related event
func (cs *ContactService) logContactEvent(ctx context.Context, sessionID uuid.UUID, eventType string, data map[string]interface{}) {
	event := &models.WhatsAppEvent{
		SessionID: sessionID,
		EventType: eventType,
		EventData: data,
	}

	if err := cs.eventRepo.Create(ctx, event); err != nil {
		cs.logger.Error("Failed to log contact event: %v", err)
	}
}

// ExportContactsVCard exports contacts as vCard format
func (cs *ContactService) ExportContactsVCard(ctx context.Context, sessionID uuid.UUID) (string, error) {
	contacts, err := cs.contactRepo.GetBySession(ctx, sessionID, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get contacts: %w", err)
	}

	var vcard strings.Builder
	for _, contact := range contacts {
		vcard.WriteString("BEGIN:VCARD\n")
		vcard.WriteString("VERSION:3.0\n")
		vcard.WriteString(fmt.Sprintf("FN:%s\n", contact.Name))
		if contact.PushName != "" {
			vcard.WriteString(fmt.Sprintf("NICKNAME:%s\n", contact.PushName))
		}
		vcard.WriteString(fmt.Sprintf("TEL:+%s\n", contact.PhoneNumber))
		if contact.About != nil && *contact.About != "" {
			vcard.WriteString(fmt.Sprintf("NOTE:%s\n", *contact.About))
		}
		if contact.BusinessName != "" {
			vcard.WriteString(fmt.Sprintf("ORG:%s\n", contact.BusinessName))
		}
		vcard.WriteString("END:VCARD\n")
	}

	// Encode to base64 for easy transport
	return base64.StdEncoding.EncodeToString([]byte(vcard.String())), nil
}
