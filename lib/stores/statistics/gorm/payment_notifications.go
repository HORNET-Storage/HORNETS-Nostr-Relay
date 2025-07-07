package gorm

import (
	"fmt"
	"math"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
)

// CreatePaymentNotification creates a new payment notification
func (store *GormStatisticsStore) CreatePaymentNotification(notification *lib.PaymentNotification) error {
	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = time.Now()
	}
	return store.DB.Create(notification).Error
}

// GetAllPaymentNotifications retrieves all payment notifications with pagination
func (store *GormStatisticsStore) GetAllPaymentNotifications(page, limit int) ([]lib.PaymentNotification, *lib.PaginationMetadata, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	var total int64
	offset := (page - 1) * limit

	// Count total notifications
	if err := store.DB.Model(&lib.PaymentNotification{}).Count(&total).Error; err != nil {
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
	var notifications []lib.PaymentNotification
	err := store.DB.Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&notifications).Error

	if err != nil {
		return nil, nil, err
	}

	return notifications, metadata, nil
}

// GetUserPaymentNotifications retrieves payment notifications for a specific user with pagination
func (store *GormStatisticsStore) GetUserPaymentNotifications(pubkey string, page, limit int) ([]lib.PaymentNotification, *lib.PaginationMetadata, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	var total int64
	offset := (page - 1) * limit

	// Count total notifications for this user
	if err := store.DB.Model(&lib.PaymentNotification{}).Where("pub_key = ?", pubkey).Count(&total).Error; err != nil {
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
	var notifications []lib.PaymentNotification
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

// GetUnreadPaymentNotifications retrieves all unread payment notifications with pagination
func (store *GormStatisticsStore) GetUnreadPaymentNotifications(page, limit int) ([]lib.PaymentNotification, *lib.PaginationMetadata, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	var total int64
	offset := (page - 1) * limit

	// Count total unread notifications
	if err := store.DB.Model(&lib.PaymentNotification{}).Where("is_read = ?", false).Count(&total).Error; err != nil {
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
	var notifications []lib.PaymentNotification
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

// MarkPaymentNotificationAsRead marks a notification as read by its ID
func (store *GormStatisticsStore) MarkPaymentNotificationAsRead(id uint) error {
	result := store.DB.Model(&lib.PaymentNotification{}).
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

// MarkAllPaymentNotificationsAsRead marks all notifications as read
func (store *GormStatisticsStore) MarkAllPaymentNotificationsAsRead() error {
	result := store.DB.Model(&lib.PaymentNotification{}).
		Where("is_read = ?", false). // Only update notifications that are currently unread
		Update("is_read", true)

	if result.Error != nil {
		return result.Error
	}

	// Log the number of rows affected to verify the update
	logging.Infof("Marked %d payment notifications as read\n", result.RowsAffected)

	return nil
}

// DeletePaymentNotification deletes a notification by its ID
func (store *GormStatisticsStore) DeletePaymentNotification(id uint) error {
	result := store.DB.Delete(&lib.PaymentNotification{}, id)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("notification with ID %d not found", id)
	}

	return nil
}

// GetPaymentStats gets statistics about payments and subscriptions
func (store *GormStatisticsStore) GetPaymentStats() (*lib.PaymentStats, error) {
	stats := &lib.PaymentStats{}

	// Get total revenue
	totalRevenue, err := store.GetTotalRevenue()
	if err != nil {
		return nil, err
	}
	stats.TotalRevenue = totalRevenue

	// Get revenue for today
	todayRevenue, err := store.GetTodayRevenue()
	if err != nil {
		return nil, err
	}
	stats.RevenueToday = todayRevenue

	// Get active subscribers count
	activeSubscribers, err := store.GetActiveSubscribersCount()
	if err != nil {
		return nil, err
	}
	stats.ActiveSubscribers = activeSubscribers

	// Get new subscribers today
	newSubscribers, err := store.GetNewSubscribersToday()
	if err != nil {
		return nil, err
	}
	stats.NewSubscribersToday = newSubscribers

	// Get revenue by tier
	byTier, err := store.GetRevenueByTier()
	if err != nil {
		return nil, err
	}
	stats.ByTier = byTier

	// Get recent transactions
	recentTransactions, err := store.GetRecentTransactions(5) // Get 5 most recent transactions
	if err != nil {
		return nil, err
	}
	stats.RecentTransactions = recentTransactions

	return stats, nil
}

// GetTotalRevenue gets the total all-time revenue in satoshis
func (store *GormStatisticsStore) GetTotalRevenue() (int64, error) {
	var total int64
	err := store.DB.Model(&lib.PaymentNotification{}).
		Select("COALESCE(SUM(amount), 0)").
		Row().
		Scan(&total)

	return total, err
}

// GetTodayRevenue gets the revenue received today in satoshis
func (store *GormStatisticsStore) GetTodayRevenue() (int64, error) {
	var total int64

	// Get start of today
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	err := store.DB.Model(&lib.PaymentNotification{}).
		Select("COALESCE(SUM(amount), 0)").
		Where("created_at >= ?", startOfDay).
		Row().
		Scan(&total)

	return total, err
}

// GetActiveSubscribersCount gets the count of active subscribers
func (store *GormStatisticsStore) GetActiveSubscribersCount() (int, error) {
	var count int64
	err := store.DB.Model(&lib.PaidSubscriber{}).
		Where("expiration_date > ?", time.Now()).
		Count(&count).Error

	return int(count), err
}

// GetNewSubscribersToday gets the count of new subscribers today
func (store *GormStatisticsStore) GetNewSubscribersToday() (int, error) {
	var count int64

	// Get start of today
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	err := store.DB.Model(&lib.PaymentNotification{}).
		Where("is_new_subscriber = ? AND created_at >= ?", true, startOfDay).
		Count(&count).Error

	return int(count), err
}

// GetRevenueByTier gets the count and revenue for each tier
func (store *GormStatisticsStore) GetRevenueByTier() ([]lib.TierStat, error) {
	var results []struct {
		Tier    string
		Count   int
		Revenue int64
	}

	err := store.DB.Model(&lib.PaymentNotification{}).
		Select("subscription_tier as tier, COUNT(*) as count, SUM(amount) as revenue").
		Group("subscription_tier").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	// Convert to TierStat
	stats := make([]lib.TierStat, len(results))
	for i, result := range results {
		stats[i] = lib.TierStat{
			Tier:    result.Tier,
			Count:   result.Count,
			Revenue: result.Revenue,
		}
	}

	return stats, nil
}

// GetRecentTransactions gets the most recent transactions
func (store *GormStatisticsStore) GetRecentTransactions(limit int) ([]lib.TxSummary, error) {
	var notifications []lib.PaymentNotification

	err := store.DB.
		Order("created_at DESC").
		Limit(limit).
		Find(&notifications).Error

	if err != nil {
		return nil, err
	}

	// Convert to TxSummary
	transactions := make([]lib.TxSummary, len(notifications))
	for i, notification := range notifications {
		transactions[i] = lib.TxSummary{
			PubKey: notification.PubKey,
			Amount: notification.Amount,
			Tier:   notification.SubscriptionTier,
			Date:   notification.CreatedAt,
		}
	}

	return transactions, nil
}
