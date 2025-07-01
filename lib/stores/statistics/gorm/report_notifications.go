package gorm

import (
	"fmt"
	"math"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib"
	"gorm.io/gorm"
)

// CreateReportNotification creates a new report notification
func (store *GormStatisticsStore) CreateReportNotification(notification *lib.ReportNotification) error {
	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = time.Now()
	}
	return store.DB.Create(notification).Error
}

// GetReportNotificationByEventID finds a report notification by event ID
func (store *GormStatisticsStore) GetReportNotificationByEventID(eventID string) (*lib.ReportNotification, error) {
	var notification lib.ReportNotification
	result := store.DB.Where("event_id = ?", eventID).First(&notification)
	if result.Error != nil {
		if result.RowsAffected == 0 {
			return nil, nil // No notification found, not an error
		}
		return nil, result.Error
	}
	return &notification, nil
}

// UpdateReportCount increments the report count for an event
func (store *GormStatisticsStore) UpdateReportCount(eventID string) error {
	result := store.DB.Model(&lib.ReportNotification{}).
		Where("event_id = ?", eventID).
		Update("report_count", gorm.Expr("report_count + 1"))

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("no notification found for event ID %s", eventID)
	}
	return nil
}

// GetAllReportNotifications retrieves all report notifications with pagination
func (store *GormStatisticsStore) GetAllReportNotifications(page, limit int) ([]lib.ReportNotification, *lib.PaginationMetadata, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	var total int64
	offset := (page - 1) * limit

	// Count total notifications
	if err := store.DB.Model(&lib.ReportNotification{}).Count(&total).Error; err != nil {
		return nil, nil, err
	}

	// Calculate pagination metadata
	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	metadata := &lib.PaginationMetadata{
		CurrentPage: page,
		PageSize:    limit,
		TotalItems:  total,
		TotalPages:  totalPages,
		HasNext:     page < totalPages,
		HasPrevious: page > 1,
	}

	// Retrieve notifications, order by report count (most reported first), then by creation date
	var notifications []lib.ReportNotification
	err := store.DB.Order("report_count DESC, created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&notifications).Error

	if err != nil {
		return nil, nil, err
	}

	return notifications, metadata, nil
}

// GetUnreadReportNotifications retrieves all unread report notifications with pagination
func (store *GormStatisticsStore) GetUnreadReportNotifications(page, limit int) ([]lib.ReportNotification, *lib.PaginationMetadata, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	var total int64
	offset := (page - 1) * limit

	// Count total unread notifications
	if err := store.DB.Model(&lib.ReportNotification{}).Where("is_read = ?", false).Count(&total).Error; err != nil {
		return nil, nil, err
	}

	// Calculate pagination metadata
	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	metadata := &lib.PaginationMetadata{
		CurrentPage: page,
		PageSize:    limit,
		TotalItems:  total,
		TotalPages:  totalPages,
		HasNext:     page < totalPages,
		HasPrevious: page > 1,
	}

	// Retrieve unread notifications, order by report count (most reported first), then by creation date
	var notifications []lib.ReportNotification
	err := store.DB.Where("is_read = ?", false).
		Order("report_count DESC, created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&notifications).Error

	if err != nil {
		return nil, nil, err
	}

	return notifications, metadata, nil
}

// MarkReportNotificationAsRead marks a report notification as read by its ID
func (store *GormStatisticsStore) MarkReportNotificationAsRead(id uint) error {
	result := store.DB.Model(&lib.ReportNotification{}).
		Where("id = ?", id).
		Update("is_read", true)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("notification with ID %d not found", id)
	}

	return nil
}

// MarkAllReportNotificationsAsRead marks all report notifications as read
func (store *GormStatisticsStore) MarkAllReportNotificationsAsRead() error {
	return store.DB.Model(&lib.ReportNotification{}).
		Where("is_read = ?", false).
		Update("is_read", true).Error
}

// DeleteReportNotificationByEventID deletes a report notification by event ID
func (store *GormStatisticsStore) DeleteReportNotificationByEventID(eventID string) error {
	result := store.DB.Where("event_id = ?", eventID).Delete(&lib.ReportNotification{})
	return result.Error
}

// GetReportStats gets statistics about reported content
func (store *GormStatisticsStore) GetReportStats() (*lib.ReportStats, error) {
	stats := &lib.ReportStats{}

	// Get total reported content count
	totalReported, err := store.GetTotalReported()
	if err != nil {
		return nil, err
	}
	stats.TotalReported = totalReported

	// Get reported content count for today
	todayReported, err := store.GetTodayReportedCount()
	if err != nil {
		return nil, err
	}
	stats.TotalReportedToday = todayReported

	// Get reported content by type
	byReportType, err := store.GetReportsByType()
	if err != nil {
		return nil, err
	}
	stats.ByReportType = byReportType

	// Get most reported content
	mostReported, err := store.GetMostReportedContent(5) // Get top 5 most reported
	if err != nil {
		return nil, err
	}
	stats.MostReported = mostReported

	return stats, nil
}

// GetTotalReported gets the total count of reported events
func (store *GormStatisticsStore) GetTotalReported() (int, error) {
	var count int64
	err := store.DB.Model(&lib.ReportNotification{}).Count(&count).Error
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// GetTodayReportedCount gets the count of content reported today
func (store *GormStatisticsStore) GetTodayReportedCount() (int, error) {
	var count int64

	// Get start of today
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	err := store.DB.Model(&lib.ReportNotification{}).
		Where("created_at >= ?", startOfDay).
		Count(&count).Error

	if err != nil {
		return 0, err
	}

	return int(count), nil
}

// GetReportsByType gets the count of reported content by type
func (store *GormStatisticsStore) GetReportsByType() ([]lib.TypeStat, error) {
	var results []struct {
		ReportType string
		Count      int
	}

	err := store.DB.Model(&lib.ReportNotification{}).
		Select("report_type as ReportType, COUNT(*) as Count").
		Group("report_type").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	// Convert to TypeStat
	stats := make([]lib.TypeStat, len(results))
	for i, result := range results {
		stats[i] = lib.TypeStat{
			Type:  result.ReportType,
			Count: result.Count,
		}
	}

	return stats, nil
}

// GetMostReportedContent gets content with highest report counts
func (store *GormStatisticsStore) GetMostReportedContent(limit int) ([]lib.ReportSummary, error) {
	var notifications []lib.ReportNotification

	err := store.DB.Order("report_count DESC").
		Limit(limit).
		Find(&notifications).Error

	if err != nil {
		return nil, err
	}

	// Convert to ReportSummary
	summaries := make([]lib.ReportSummary, len(notifications))
	for i, notification := range notifications {
		summaries[i] = lib.ReportSummary{
			EventID:     notification.EventID,
			PubKey:      notification.PubKey,
			ReportCount: notification.ReportCount,
			ReportType:  notification.ReportType,
			CreatedAt:   notification.CreatedAt,
		}
	}

	return summaries, nil
}
