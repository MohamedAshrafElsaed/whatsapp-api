package repositories

import (
	"context"
	"time"

	"gorm.io/gorm"
	"whatsapp-api/internal/models"
)

// EventRepository handles database operations for WhatsApp events
type EventRepository struct {
	db *gorm.DB
}

// NewEventRepository creates a new event repositories
func NewEventRepository(db *gorm.DB) *EventRepository {
	return &EventRepository{
		db: db,
	}
}

// Create creates a new WhatsApp event
func (r *EventRepository) Create(ctx context.Context, event *models.WhatsAppEvent) error {
	return r.db.WithContext(ctx).Create(event).Error
}

// CreateBatch creates multiple events in batch
func (r *EventRepository) CreateBatch(ctx context.Context, events []*models.WhatsAppEvent) error {
	if len(events) == 0 {
		return nil
	}

	batchSize := 100
	return r.db.WithContext(ctx).CreateInBatches(events, batchSize).Error
}

// GetByID retrieves an event by ID
func (r *EventRepository) GetByID(ctx context.Context, id int64) (*models.WhatsAppEvent, error) {
	var event models.WhatsAppEvent
	err := r.db.WithContext(ctx).
		Preload("Session").
		Where("id = ?", id).
		First(&event).Error

	if err != nil {
		return nil, err
	}

	return &event, nil
}

// GetBySessionID retrieves all events for a session
func (r *EventRepository) GetBySessionID(ctx context.Context, sessionID string, limit, offset int) ([]*models.WhatsAppEvent, error) {
	var events []*models.WhatsAppEvent
	query := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Find(&events).Error
	if err != nil {
		return nil, err
	}

	return events, nil
}

// GetByUserID retrieves all events for a user
func (r *EventRepository) GetByUserID(ctx context.Context, userID int, limit, offset int) ([]*models.WhatsAppEvent, error) {
	var events []*models.WhatsAppEvent
	query := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Find(&events).Error
	if err != nil {
		return nil, err
	}

	return events, nil
}

// GetByFilter retrieves events by filter
func (r *EventRepository) GetByFilter(ctx context.Context, filter *models.EventFilter) ([]*models.WhatsAppEvent, error) {
	var events []*models.WhatsAppEvent
	query := r.db.WithContext(ctx)

	if filter.UserID != nil {
		query = query.Where("user_id = ?", *filter.UserID)
	}

	if filter.SessionID != nil {
		query = query.Where("session_id = ?", *filter.SessionID)
	}

	if len(filter.EventTypes) > 0 {
		query = query.Where("event_type IN ?", filter.EventTypes)
	}

	if filter.StartDate != nil {
		query = query.Where("created_at >= ?", *filter.StartDate)
	}

	if filter.EndDate != nil {
		query = query.Where("created_at <= ?", *filter.EndDate)
	}

	query = query.Order("created_at DESC")

	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	} else {
		query = query.Limit(100) // Default limit
	}

	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	err := query.Find(&events).Error
	if err != nil {
		return nil, err
	}

	return events, nil
}

// CountByFilter counts events by filter
func (r *EventRepository) CountByFilter(ctx context.Context, filter *models.EventFilter) (int64, error) {
	var count int64
	query := r.db.WithContext(ctx).Model(&models.WhatsAppEvent{})

	if filter.UserID != nil {
		query = query.Where("user_id = ?", *filter.UserID)
	}

	if filter.SessionID != nil {
		query = query.Where("session_id = ?", *filter.SessionID)
	}

	if len(filter.EventTypes) > 0 {
		query = query.Where("event_type IN ?", filter.EventTypes)
	}

	if filter.StartDate != nil {
		query = query.Where("created_at >= ?", *filter.StartDate)
	}

	if filter.EndDate != nil {
		query = query.Where("created_at <= ?", *filter.EndDate)
	}

	err := query.Count(&count).Error
	return count, err
}

// GetByEventType retrieves events by type
func (r *EventRepository) GetByEventType(ctx context.Context, userID int, eventType models.EventType, limit int) ([]*models.WhatsAppEvent, error) {
	var events []*models.WhatsAppEvent
	query := r.db.WithContext(ctx).
		Where("user_id = ? AND event_type = ?", userID, eventType).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	} else {
		query = query.Limit(50)
	}

	err := query.Find(&events).Error
	if err != nil {
		return nil, err
	}

	return events, nil
}

// GetCriticalEvents retrieves critical events
func (r *EventRepository) GetCriticalEvents(ctx context.Context, userID int, limit int) ([]*models.WhatsAppEvent, error) {
	criticalTypes := []models.EventType{
		models.EventTypeError,
		models.EventTypeStreamError,
		models.EventTypeClientError,
		models.EventTypePairError,
		models.EventTypeLoggedOut,
	}

	var events []*models.WhatsAppEvent
	query := r.db.WithContext(ctx).
		Where("user_id = ? AND event_type IN ?", userID, criticalTypes).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	} else {
		query = query.Limit(50)
	}

	err := query.Find(&events).Error
	if err != nil {
		return nil, err
	}

	return events, nil
}

// GetRecentEvents retrieves recent events
func (r *EventRepository) GetRecentEvents(ctx context.Context, userID int, duration time.Duration, limit int) ([]*models.WhatsAppEvent, error) {
	threshold := time.Now().Add(-duration)

	var events []*models.WhatsAppEvent
	query := r.db.WithContext(ctx).
		Where("user_id = ? AND created_at >= ?", userID, threshold).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	} else {
		query = query.Limit(50)
	}

	err := query.Find(&events).Error
	if err != nil {
		return nil, err
	}

	return events, nil
}

// GetStatistics retrieves event statistics for a user
func (r *EventRepository) GetStatistics(ctx context.Context, userID int) (*models.EventStatistics, error) {
	stats := &models.EventStatistics{
		UserID:       userID,
		EventsByType: make(map[models.EventType]int64),
	}

	// Total events
	err := r.db.WithContext(ctx).
		Model(&models.WhatsAppEvent{}).
		Where("user_id = ?", userID).
		Count(&stats.TotalEvents).Error
	if err != nil {
		return nil, err
	}

	// Events by type
	type eventTypeCount struct {
		EventType models.EventType
		Count     int64
	}

	var typeCounts []eventTypeCount
	err = r.db.WithContext(ctx).
		Model(&models.WhatsAppEvent{}).
		Select("event_type, COUNT(*) as count").
		Where("user_id = ?", userID).
		Group("event_type").
		Scan(&typeCounts).Error
	if err != nil {
		return nil, err
	}

	for _, tc := range typeCounts {
		stats.EventsByType[tc.EventType] = tc.Count
	}

	// Critical events count
	criticalTypes := []models.EventType{
		models.EventTypeError,
		models.EventTypeStreamError,
		models.EventTypeClientError,
		models.EventTypePairError,
		models.EventTypeLoggedOut,
	}

	err = r.db.WithContext(ctx).
		Model(&models.WhatsAppEvent{}).
		Where("user_id = ? AND event_type IN ?", userID, criticalTypes).
		Count(&stats.CriticalEvents).Error
	if err != nil {
		return nil, err
	}

	// Last event time
	var lastEvent *time.Time
	err = r.db.WithContext(ctx).
		Model(&models.WhatsAppEvent{}).
		Where("user_id = ?", userID).
		Select("MAX(created_at)").
		Scan(&lastEvent).Error
	if err != nil {
		return nil, err
	}
	stats.LastEvent = lastEvent

	// First event time
	var firstEvent *time.Time
	err = r.db.WithContext(ctx).
		Model(&models.WhatsAppEvent{}).
		Where("user_id = ?", userID).
		Select("MIN(created_at)").
		Scan(&firstEvent).Error
	if err != nil {
		return nil, err
	}
	stats.FirstEvent = firstEvent

	return stats, nil
}

// GetSessionStatistics retrieves event statistics for a session
func (r *EventRepository) GetSessionStatistics(ctx context.Context, sessionID string) (*models.EventStatistics, error) {
	stats := &models.EventStatistics{
		EventsByType: make(map[models.EventType]int64),
	}

	// Total events
	err := r.db.WithContext(ctx).
		Model(&models.WhatsAppEvent{}).
		Where("session_id = ?", sessionID).
		Count(&stats.TotalEvents).Error
	if err != nil {
		return nil, err
	}

	// Events by type
	type eventTypeCount struct {
		EventType models.EventType
		Count     int64
	}

	var typeCounts []eventTypeCount
	err = r.db.WithContext(ctx).
		Model(&models.WhatsAppEvent{}).
		Select("event_type, COUNT(*) as count").
		Where("session_id = ?", sessionID).
		Group("event_type").
		Scan(&typeCounts).Error
	if err != nil {
		return nil, err
	}

	for _, tc := range typeCounts {
		stats.EventsByType[tc.EventType] = tc.Count
	}

	// Critical events count
	criticalTypes := []models.EventType{
		models.EventTypeError,
		models.EventTypeStreamError,
		models.EventTypeClientError,
		models.EventTypePairError,
		models.EventTypeLoggedOut,
	}

	err = r.db.WithContext(ctx).
		Model(&models.WhatsAppEvent{}).
		Where("session_id = ? AND event_type IN ?", sessionID, criticalTypes).
		Count(&stats.CriticalEvents).Error
	if err != nil {
		return nil, err
	}

	return stats, nil
}

// DeleteOlderThan deletes events older than the specified duration
func (r *EventRepository) DeleteOlderThan(ctx context.Context, duration time.Duration) (int64, error) {
	threshold := time.Now().Add(-duration)

	result := r.db.WithContext(ctx).
		Where("created_at < ?", threshold).
		Delete(&models.WhatsAppEvent{})

	return result.RowsAffected, result.Error
}

// DeleteBySessionID deletes all events for a session
func (r *EventRepository) DeleteBySessionID(ctx context.Context, sessionID string) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&models.WhatsAppEvent{})

	return result.RowsAffected, result.Error
}

// DeleteByUserID deletes all events for a user
func (r *EventRepository) DeleteByUserID(ctx context.Context, userID int) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&models.WhatsAppEvent{})

	return result.RowsAffected, result.Error
}

// CountByUserID counts events for a user
func (r *EventRepository) CountByUserID(ctx context.Context, userID int) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.WhatsAppEvent{}).
		Where("user_id = ?", userID).
		Count(&count).Error

	return count, err
}

// CountBySessionID counts events for a session
func (r *EventRepository) CountBySessionID(ctx context.Context, sessionID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.WhatsAppEvent{}).
		Where("session_id = ?", sessionID).
		Count(&count).Error

	return count, err
}

// CountByEventType counts events by type for a user
func (r *EventRepository) CountByEventType(ctx context.Context, userID int, eventType models.EventType) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.WhatsAppEvent{}).
		Where("user_id = ? AND event_type = ?", userID, eventType).
		Count(&count).Error

	return count, err
}

// GetLatestByType retrieves the latest event of a specific type
func (r *EventRepository) GetLatestByType(ctx context.Context, userID int, eventType models.EventType) (*models.WhatsAppEvent, error) {
	var event models.WhatsAppEvent
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND event_type = ?", userID, eventType).
		Order("created_at DESC").
		First(&event).Error

	if err != nil {
		return nil, err
	}

	return &event, nil
}

// GetEventsInDateRange retrieves events within a date range
func (r *EventRepository) GetEventsInDateRange(ctx context.Context, userID int, start, end time.Time, limit int) ([]*models.WhatsAppEvent, error) {
	var events []*models.WhatsAppEvent
	query := r.db.WithContext(ctx).
		Where("user_id = ? AND created_at BETWEEN ? AND ?", userID, start, end).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&events).Error
	if err != nil {
		return nil, err
	}

	return events, nil
}

// CleanupOldEvents removes events older than specified days
func (r *EventRepository) CleanupOldEvents(ctx context.Context, days int) (int64, error) {
	threshold := time.Now().AddDate(0, 0, -days)

	result := r.db.WithContext(ctx).
		Where("created_at < ?", threshold).
		Delete(&models.WhatsAppEvent{})

	return result.RowsAffected, result.Error
}

// GetEventTypeCounts gets counts for all event types
func (r *EventRepository) GetEventTypeCounts(ctx context.Context, userID int) (map[models.EventType]int64, error) {
	type eventTypeCount struct {
		EventType models.EventType
		Count     int64
	}

	var typeCounts []eventTypeCount
	err := r.db.WithContext(ctx).
		Model(&models.WhatsAppEvent{}).
		Select("event_type, COUNT(*) as count").
		Where("user_id = ?", userID).
		Group("event_type").
		Scan(&typeCounts).Error

	if err != nil {
		return nil, err
	}

	result := make(map[models.EventType]int64)
	for _, tc := range typeCounts {
		result[tc.EventType] = tc.Count
	}

	return result, nil
}
