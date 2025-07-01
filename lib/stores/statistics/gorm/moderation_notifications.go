package gorm

import (
	"fmt"
	"math"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib"
)

// CreateModerationNotification creates a new moderation notification
func (store *GormStatisticsStore) CreateModerationNotification(notification *lib.ModerationNotification) error {
	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = time.Now()
	}
	return store.DB.Create(notification).Error
}

// GetAllModerationNotifications retrieves all moderation notifications with pagination
func (store *GormStatisticsStore) GetAllModerationNotifications(page, limit int) ([]lib.ModerationNotification, *lib.PaginationMetadata, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	var total int64
	offset := (page - 1) * limit

	// Count total notifications
	if err := store.DB.Model(&lib.ModerationNotification{}).Count(&total).Error; err != nil {
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

	// Retrieve notifications
	var notifications []lib.ModerationNotification
	err := store.DB.Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&notifications).Error

	if err != nil {
		return nil, nil, err
	}

	return notifications, metadata, nil
}

// GetUserModerationNotifications retrieves moderation notifications for a specific user with pagination
func (store *GormStatisticsStore) GetUserModerationNotifications(pubkey string, page, limit int) ([]lib.ModerationNotification, *lib.PaginationMetadata, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	var total int64
	offset := (page - 1) * limit

	// Count total notifications for this user
	if err := store.DB.Model(&lib.ModerationNotification{}).Where("pub_key = ?", pubkey).Count(&total).Error; err != nil {
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

	// Retrieve notifications for this user
	var notifications []lib.ModerationNotification
	err := store.DB.Where("pub_key = ?", pubkey).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&notifications).Error

	if err != nil {
		return nil, nil, err
	}

	return notifications, metadata, nil
}

// GetUnreadModerationNotifications retrieves all unread moderation notifications with pagination
func (store *GormStatisticsStore) GetUnreadModerationNotifications(page, limit int) ([]lib.ModerationNotification, *lib.PaginationMetadata, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	var total int64
	offset := (page - 1) * limit

	// Count total unread notifications
	if err := store.DB.Model(&lib.ModerationNotification{}).Where("is_read = ?", false).Count(&total).Error; err != nil {
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

	// Retrieve unread notifications
	var notifications []lib.ModerationNotification
	err := store.DB.Where("is_read = ?", false).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&notifications).Error

	if err != nil {
		return nil, nil, err
	}

	return notifications, metadata, nil
}

// MarkNotificationAsRead marks a notification as read by its ID
func (store *GormStatisticsStore) MarkNotificationAsRead(id uint) error {
	result := store.DB.Model(&lib.ModerationNotification{}).
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

// MarkAllNotificationsAsRead marks all notifications for a user as read
func (store *GormStatisticsStore) MarkAllNotificationsAsRead(pubkey string) error {
	result := store.DB.Model(&lib.ModerationNotification{}).
		Where("pub_key = ?", pubkey).
		Update("is_read", true)

	if result.Error != nil {
		return result.Error
	}

	return nil
}

// DeleteModerationNotification deletes a notification by its ID
func (store *GormStatisticsStore) DeleteModerationNotification(id uint) error {
	result := store.DB.Delete(&lib.ModerationNotification{}, id)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("notification with ID %d not found", id)
	}

	return nil
}

// GetModerationStats gets statistics about blocked content
func (store *GormStatisticsStore) GetModerationStats() (*lib.ModerationStats, error) {
	stats := &lib.ModerationStats{}

	// Get total blocked content count
	totalBlocked, err := store.GetBlockedContentCount()
	if err != nil {
		return nil, err
	}
	stats.TotalBlocked = totalBlocked

	// Get blocked content count for today
	todayBlocked, err := store.GetTodayBlockedContentCount()
	if err != nil {
		return nil, err
	}
	stats.TotalBlockedToday = todayBlocked

	// Get blocked content by type
	byContentType, err := store.GetBlockedContentByType()
	if err != nil {
		return nil, err
	}
	stats.ByContentType = byContentType

	// Get top users with blocked content
	byUser, err := store.GetBlockedContentByUser(5) // Get top 5 users
	if err != nil {
		return nil, err
	}
	stats.ByUser = byUser

	// Get recent blocking reasons
	recentReasons, err := store.GetRecentBlockingReasons(5) // Get 5 most recent reasons
	if err != nil {
		return nil, err
	}
	stats.RecentReasons = recentReasons

	return stats, nil
}

// GetBlockedContentCount gets the total count of blocked events
func (store *GormStatisticsStore) GetBlockedContentCount() (int, error) {
	var count int64
	err := store.DB.Model(&lib.ModerationNotification{}).Count(&count).Error
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// GetTodayBlockedContentCount gets the count of content blocked today
func (store *GormStatisticsStore) GetTodayBlockedContentCount() (int, error) {
	var count int64

	// Get start of today
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	err := store.DB.Model(&lib.ModerationNotification{}).
		Where("created_at >= ?", startOfDay).
		Count(&count).Error

	if err != nil {
		return 0, err
	}

	return int(count), nil
}

// GetBlockedContentByType gets the count of blocked content by type
func (store *GormStatisticsStore) GetBlockedContentByType() ([]lib.TypeStat, error) {
	var results []struct {
		ContentType string
		Count       int
	}

	err := store.DB.Model(&lib.ModerationNotification{}).
		Select("content_type, COUNT(*) as count").
		Group("content_type").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	// Convert to TypeStat
	stats := make([]lib.TypeStat, len(results))
	for i, result := range results {
		stats[i] = lib.TypeStat{
			Type:  result.ContentType,
			Count: result.Count,
		}
	}

	return stats, nil
}

// GetBlockedContentByUser gets the count of blocked content by user
func (store *GormStatisticsStore) GetBlockedContentByUser(limit int) ([]lib.UserStat, error) {
	var results []struct {
		PubKey string
		Count  int
	}

	err := store.DB.Model(&lib.ModerationNotification{}).
		Select("pub_key, COUNT(*) as count").
		Group("pub_key").
		Order("count DESC").
		Limit(limit).
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	// Convert to UserStat
	stats := make([]lib.UserStat, len(results))
	for i, result := range results {
		stats[i] = lib.UserStat{
			PubKey: result.PubKey,
			Count:  result.Count,
		}
	}

	return stats, nil
}

// GetRecentBlockingReasons gets the most recent blocking reasons
func (store *GormStatisticsStore) GetRecentBlockingReasons(limit int) ([]string, error) {
	var notifications []lib.ModerationNotification

	err := store.DB.Model(&lib.ModerationNotification{}).
		Select("reason").
		Order("created_at DESC").
		Limit(limit).
		Find(&notifications).Error

	if err != nil {
		return nil, err
	}

	// Extract reasons
	reasons := make([]string, len(notifications))
	for i, notification := range notifications {
		reasons[i] = notification.Reason
	}

	return reasons, nil
}
