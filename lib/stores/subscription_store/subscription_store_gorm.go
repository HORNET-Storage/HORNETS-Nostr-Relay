package subscription_gorm

import (
	"fmt"
	"log"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type GormSubscriberStore struct {
	DB *gorm.DB
}

const (
	AddressStatusAvailable = "available"
	AddressStatusAllocated = "allocated"
	AddressStatusUsed      = "used"
)

// Convert storage strings to bytes
func parseStorageLimit(limit string) (int64, error) {
	var bytes int64
	switch limit {
	case "1 GB per month":
		bytes = 1 * 1024 * 1024 * 1024
	case "5 GB per month":
		bytes = 5 * 1024 * 1024 * 1024
	case "10 GB per month":
		bytes = 10 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unknown storage limit: %s", limit)
	}
	return bytes, nil
}

// InitStore initializes the GORM subscriber store
func (store *GormSubscriberStore) InitStore(basepath string, args ...interface{}) error {
	var err error

	// Initialize the database connection
	store.DB, err = gorm.Open(sqlite.Open(basepath), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}

	// Run migrations
	err = store.DB.AutoMigrate(
		&types.GormSubscriber{},
		&types.SubscriptionPeriod{},
		&types.FileUpload{},
		&types.SubscriberAddress{},
		&types.WalletAddress{},
	)
	if err != nil {
		return fmt.Errorf("failed to run migrations: %v", err)
	}

	return nil
}

// NewGormSubscriberStore creates a new instance of GormSubscriberStore
func NewGormSubscriberStore() *GormSubscriberStore {
	return &GormSubscriberStore{}
}

// SaveSubscriber saves or updates a subscriber
// For new subscribers:
// - Creates with default values (no tier/storage)
// For existing subscribers:
// - Updates with new tier and storage limits if provided
func (store *GormSubscriberStore) SaveSubscriber(subscriber *types.Subscriber) error {
	tx := store.DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var gormSubscriber types.GormSubscriber
	result := tx.Where("npub = ?", subscriber.Npub).First(&gormSubscriber)

	if result.Error == gorm.ErrRecordNotFound {
		// Creating new subscriber
		gormSubscriber = types.GormSubscriber{
			Npub:              subscriber.Npub,
			StorageUsedBytes:  0,
			StorageLimitBytes: 0, // No storage limit until subscription
			LastUpdated:       time.Now(),
		}

		// If tier is provided (unusual for new subscriber but possible)
		if subscriber.Tier != "" {
			storageLimitBytes, err := parseStorageLimit(subscriber.Tier)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("invalid tier for new subscriber: %v", err)
			}
			gormSubscriber.CurrentTier = subscriber.Tier
			gormSubscriber.StorageLimitBytes = storageLimitBytes
			gormSubscriber.StartDate = subscriber.StartDate
			gormSubscriber.EndDate = subscriber.EndDate
		}

		if err := tx.Create(&gormSubscriber).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to create subscriber: %v", err)
		}

		log.Printf("Created new subscriber: %s", subscriber.Npub)
	} else if result.Error != nil {
		tx.Rollback()
		return fmt.Errorf("failed to query subscriber: %v", result.Error)
	} else {
		// Updating existing subscriber
		if subscriber.Tier != "" {
			// Only update tier-related fields if a tier is provided
			storageLimitBytes, err := parseStorageLimit(subscriber.Tier)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("invalid tier for subscription update: %v", err)
			}

			gormSubscriber.CurrentTier = subscriber.Tier
			gormSubscriber.StorageLimitBytes = storageLimitBytes
			gormSubscriber.StartDate = subscriber.StartDate
			gormSubscriber.EndDate = subscriber.EndDate
		}

		gormSubscriber.LastUpdated = time.Now()

		if err := tx.Save(&gormSubscriber).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to update subscriber: %v", err)
		}

		log.Printf("Updated subscriber: %s", subscriber.Npub)
	}

	// Only create subscription period if tier and transaction ID are provided
	if subscriber.Tier != "" && subscriber.LastTransactionID != "" {
		storageLimitBytes, _ := parseStorageLimit(subscriber.Tier) // Error already checked above
		subscriptionPeriod := types.SubscriptionPeriod{
			SubscriberID:      gormSubscriber.ID,
			TransactionID:     subscriber.LastTransactionID,
			Tier:              subscriber.Tier,
			StorageLimitBytes: storageLimitBytes,
			StartDate:         subscriber.StartDate,
			EndDate:           subscriber.EndDate,
			PaymentAmount:     "", // Add payment amount when available
		}

		if err := tx.Create(&subscriptionPeriod).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to create subscription period: %v", err)
		}

		log.Printf("Created subscription period for %s: %s tier",
			subscriber.Npub, subscriber.Tier)
	}

	return tx.Commit().Error
}

// GetSubscriberStorageStats gets detailed storage statistics
func (store *GormSubscriberStore) GetSubscriberStorageStats(npub string) (*types.StorageStats, error) {
	var subscriber types.GormSubscriber
	if err := store.DB.Where("npub = ?", npub).First(&subscriber).Error; err != nil {
		return nil, err
	}

	var recentFiles []types.FileUpload
	if err := store.DB.Where("npub = ? AND deleted = ?", npub, false).
		Order("created_at desc").
		Limit(10).
		Find(&recentFiles).Error; err != nil {
		return nil, err
	}

	return &types.StorageStats{
		CurrentUsageBytes: subscriber.StorageUsedBytes,
		StorageLimitBytes: subscriber.StorageLimitBytes,
		UsagePercentage:   float64(subscriber.StorageUsedBytes) / float64(subscriber.StorageLimitBytes) * 100,
		SubscriptionEnd:   subscriber.EndDate,
		CurrentTier:       subscriber.CurrentTier,
		LastUpdated:       subscriber.LastUpdated,
		RecentFiles:       recentFiles,
	}, nil
}

// UpdateStorageUsage updates a subscriber's storage usage
func (store *GormSubscriberStore) UpdateStorageUsage(npub string, sizeBytes int64) error {
	tx := store.DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var subscriber types.GormSubscriber
	if err := tx.Where("npub = ?", npub).First(&subscriber).Error; err != nil {
		tx.Rollback()
		return err
	}

	newUsage := subscriber.StorageUsedBytes + sizeBytes
	if newUsage > subscriber.StorageLimitBytes {
		tx.Rollback()
		return fmt.Errorf("storage limit exceeded: would use %d of %d bytes", newUsage, subscriber.StorageLimitBytes)
	}

	if err := tx.Model(&subscriber).Updates(map[string]interface{}{
		"storage_used_bytes": newUsage,
		"last_updated":       time.Now(),
	}).Error; err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}

// TrackFileUpload records a new file upload
func (store *GormSubscriberStore) TrackFileUpload(upload *types.FileUpload) error {
	return store.DB.Create(upload).Error
}

// GetSubscriber retrieves a subscriber by npub
func (store *GormSubscriberStore) GetSubscriber(npub string) (*types.Subscriber, error) {
	var gormSubscriber types.GormSubscriber
	if err := store.DB.Where("npub = ?", npub).First(&gormSubscriber).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create a new subscriber with just the npub
			newSubscriber := &types.Subscriber{
				Npub: npub,
			}
			if err := store.SaveSubscriber(newSubscriber); err != nil {
				return nil, fmt.Errorf("failed to create new subscriber: %v", err)
			}
			return newSubscriber, nil
		}
		return nil, err
	}
	return gormSubscriber.ToSubscriber(), nil
}

// DeleteSubscriber removes a subscriber
func (store *GormSubscriberStore) DeleteSubscriber(npub string) error {
	return store.DB.Where("npub = ?", npub).Delete(&types.GormSubscriber{}).Error
}

// ListSubscribers returns all subscribers
func (store *GormSubscriberStore) ListSubscribers() ([]*types.Subscriber, error) {
	var gormSubscribers []types.GormSubscriber
	if err := store.DB.Find(&gormSubscribers).Error; err != nil {
		return nil, err
	}

	subscribers := make([]*types.Subscriber, len(gormSubscribers))
	for i, gs := range gormSubscribers {
		subscribers[i] = gs.ToSubscriber()
	}
	return subscribers, nil
}

// AddSubscriptionPeriod adds a new subscription period
func (store *GormSubscriberStore) AddSubscriptionPeriod(npub string, period *types.SubscriptionPeriod) error {
	var subscriber types.GormSubscriber
	if err := store.DB.Where("npub = ?", npub).First(&subscriber).Error; err != nil {
		return err
	}

	period.SubscriberID = subscriber.ID
	return store.DB.Create(period).Error
}

// GetSubscriptionPeriods retrieves all subscription periods for a subscriber
func (store *GormSubscriberStore) GetSubscriptionPeriods(npub string) ([]*types.SubscriptionPeriod, error) {
	var subscriber types.GormSubscriber
	if err := store.DB.Where("npub = ?", npub).First(&subscriber).Error; err != nil {
		return nil, err
	}

	var periods []*types.SubscriptionPeriod
	if err := store.DB.Where("subscriber_id = ?", subscriber.ID).
		Order("start_date desc").
		Find(&periods).Error; err != nil {
		return nil, err
	}

	return periods, nil
}

// GetActiveSubscription gets the current active subscription
func (store *GormSubscriberStore) GetActiveSubscription(npub string) (*types.SubscriptionPeriod, error) {
	var subscriber types.GormSubscriber
	if err := store.DB.Where("npub = ?", npub).First(&subscriber).Error; err != nil {
		return nil, err
	}

	var period types.SubscriptionPeriod
	if err := store.DB.Where("subscriber_id = ? AND end_date > ?", subscriber.ID, time.Now()).
		Order("end_date desc").
		First(&period).Error; err != nil {
		return nil, err
	}

	return &period, nil
}

func (store *GormSubscriberStore) GetSubscriberByAddress(address string) (*types.Subscriber, error) {
	// First find the subscriber_address record with explicit column selection
	var subscriberAddress struct {
		Address string
		Status  string
		Npub    string
	}

	err := store.DB.Table("subscriber_addresses").
		Select("address, status, npub").
		Where("address = ?", address).
		First(&subscriberAddress).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("address %s not allocated to any subscriber", address)
		}
		return nil, fmt.Errorf("error querying address: %v", err)
	}

	log.Printf("Found subscriber address record: address=%s, npub=%s, status=%s",
		subscriberAddress.Address, subscriberAddress.Npub, subscriberAddress.Status)

	// Verify we have a valid npub
	if subscriberAddress.Npub == "" {
		return nil, fmt.Errorf("address %s has no associated npub", address)
	}

	log.Println("Gorm Subscriber Npub: ", subscriberAddress.Npub)

	// Now get the subscriber using the npub
	var subscriber types.GormSubscriber
	if err := store.DB.Where("npub = ?", subscriberAddress.Npub).First(&subscriber).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("subscriber not found for npub: %s", subscriberAddress.Npub)
		}
		return nil, fmt.Errorf("error querying subscriber: %v", err)
	}

	// Create and return the subscriber
	return &types.Subscriber{
		Npub:      subscriber.Npub,
		Tier:      subscriber.CurrentTier,
		StartDate: subscriber.StartDate,
		EndDate:   subscriber.EndDate,
		Address:   address,
	}, nil
}

func (store *GormSubscriberStore) SaveSubscriberAddresses(address *types.WalletAddress) error {
	return store.DB.Create(address).Error
}

func (store *GormSubscriberStore) SaveSubscriberAddress(address *types.SubscriberAddress) error {
	// Check if the address already exists
	var existingAddress types.SubscriberAddress
	result := store.DB.Where("address = ?", address.Address).First(&existingAddress)

	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		log.Printf("Error querying existing address: %v", result.Error)
		return result.Error
	}

	// If the address already exists, log and skip the insert
	if result.RowsAffected > 0 {
		log.Printf("Address %s already exists, skipping save.", address.Address)
		return nil
	}

	// Set defaults if needed
	if address.Status == "" {
		address.Status = AddressStatusAvailable
	}
	if address.AllocatedAt == nil {
		now := time.Now()
		address.AllocatedAt = &now
	}
	address.Npub = "" // Explicitly set to NULL

	// Create the new address in the database
	if err := store.DB.Create(address).Error; err != nil {
		log.Printf("Error saving new address: %v", err)
		return err
	}

	log.Printf("Address %s saved successfully.", address.Address)
	return nil
}

func (store *GormSubscriberStore) AllocateBitcoinAddress(npub string) (*types.Address, error) {
	tx := store.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Modified query to handle NULL npub
	var subscriptionAddress types.SubscriberAddress
	err := tx.Where("status = ? AND (npub IS NULL OR npub = '')", AddressStatusAvailable).
		Order("id").
		First(&subscriptionAddress).Error

	if err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("no available addresses")
		}
		return nil, fmt.Errorf("failed to query available addresses: %v", err)
	}

	// Update the address fields
	now := time.Now()
	updates := map[string]interface{}{
		"status":       AddressStatusAllocated,
		"allocated_at": &now,
		"npub":         npub,
	}

	if err := tx.Model(&subscriptionAddress).Updates(updates).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update address allocation: %v", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &types.Address{
		Index:       subscriptionAddress.Index,
		Address:     subscriptionAddress.Address,
		WalletName:  subscriptionAddress.WalletName,
		Status:      subscriptionAddress.Status,
		AllocatedAt: subscriptionAddress.AllocatedAt,
		Npub:        npub,
	}, nil
}

func (store *GormSubscriberStore) AddressExists(address string) (bool, error) {
	var count int64
	err := store.DB.Model(&types.SubscriberAddress{}).
		Where("address = ?", address).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("failed to check address existence: %v", err)
	}

	return count > 0, nil
}

// GetSubscriptionByTransactionID retrieves a subscription by its transaction ID
func (store *GormSubscriberStore) GetSubscriptionByTransactionID(transactionID string) (*types.SubscriptionPeriod, error) {
	var period types.SubscriptionPeriod
	if err := store.DB.Where("transaction_id = ?", transactionID).First(&period).Error; err != nil {
		return nil, err
	}
	return &period, nil
}

// CheckStorageAvailability verifies if the subscriber has enough storage space
func (store *GormSubscriberStore) CheckStorageAvailability(npub string, requestedBytes int64) error {
	var subscriber types.GormSubscriber
	if err := store.DB.Where("npub = ?", npub).First(&subscriber).Error; err != nil {
		return err
	}

	// If no tier/storage limit is set, they can't upload
	if subscriber.StorageLimitBytes == 0 {
		return fmt.Errorf("no active subscription: storage limit not set")
	}

	if subscriber.EndDate.IsZero() || time.Now().After(subscriber.EndDate) {
		return fmt.Errorf("subscription expired or not yet activated")
	}

	newUsage := subscriber.StorageUsedBytes + requestedBytes
	if newUsage > subscriber.StorageLimitBytes {
		return fmt.Errorf("storage limit exceeded: would use %d of %d bytes",
			newUsage, subscriber.StorageLimitBytes)
	}

	return nil
}

// GetStorageUsage gets current storage usage
func (store *GormSubscriberStore) GetStorageUsage(npub string) (*types.StorageUsage, error) {
	var subscriber types.GormSubscriber
	if err := store.DB.Where("npub = ?", npub).First(&subscriber).Error; err != nil {
		return nil, err
	}

	return &types.StorageUsage{
		Npub:           subscriber.Npub,
		UsedBytes:      subscriber.StorageUsedBytes,
		AllocatedBytes: subscriber.StorageLimitBytes,
		LastUpdated:    subscriber.LastUpdated,
	}, nil
}

// GetFilesBySubscriber retrieves all files for a subscriber
func (store *GormSubscriberStore) GetFilesBySubscriber(npub string) ([]*types.FileUpload, error) {
	var files []*types.FileUpload
	if err := store.DB.Where("npub = ? AND deleted = ?", npub, false).
		Order("created_at desc").
		Find(&files).Error; err != nil {
		return nil, err
	}
	return files, nil
}

// GetRecentUploads gets the most recent uploads for a subscriber
func (store *GormSubscriberStore) GetRecentUploads(npub string, limit int) ([]*types.FileUpload, error) {
	var files []*types.FileUpload
	if err := store.DB.Where("npub = ? AND deleted = ?", npub, false).
		Order("created_at desc").
		Limit(limit).
		Find(&files).Error; err != nil {
		return nil, err
	}
	return files, nil
}

// DeleteFile marks a file as deleted and updates storage usage
func (store *GormSubscriberStore) DeleteFile(npub string, fileHash string) error {
	tx := store.DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var file types.FileUpload
	if err := tx.Where("npub = ? AND file_hash = ? AND deleted = ?", npub, fileHash, false).
		First(&file).Error; err != nil {
		tx.Rollback()
		return err
	}

	now := time.Now()
	if err := tx.Model(&file).Updates(map[string]interface{}{
		"deleted":    true,
		"deleted_at": &now,
	}).Error; err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Model(&types.GormSubscriber{}).
		Where("npub = ?", npub).
		UpdateColumn("storage_used_bytes", gorm.Expr("storage_used_bytes - ?", file.SizeBytes)).
		Error; err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}

func (store *GormSubscriberStore) DebugAddressDetails(address string) {
	var addr types.SubscriberAddress
	result := store.DB.Where("address = ?", address).First(&addr)

	if result.Error != nil {
		log.Printf("Error querying address details: %v", result.Error)
		return
	}

	log.Printf("=== Address Details ===")
	log.Printf("Address: %s", addr.Address)
	log.Printf("Status: %s", addr.Status)
	if addr.Npub != "" {
		log.Printf("Allocated to npub: %s", addr.Npub)
	} else {
		log.Printf("Not allocated to any npub")
	}
	if addr.AllocatedAt != nil {
		log.Printf("Allocated at: %v", *addr.AllocatedAt)
	}
	log.Printf("=====================")
}

func (store *GormSubscriberStore) DumpAddressTable() {
	var addresses []types.SubscriberAddress
	result := store.DB.Find(&addresses)

	if result.Error != nil {
		log.Printf("Error querying address table: %v", result.Error)
		return
	}

	log.Printf("=== Address Table Contents ===")
	log.Printf("Found %d addresses", len(addresses))
	for _, addr := range addresses {
		log.Printf("Address: %s, Status: %s, Npub: %v",
			addr.Address,
			addr.Status,
			addr.Npub)
	}
	log.Printf("============================")
}

// Add this debug select to check the address in the database directly
func (store *GormSubscriberStore) DebugAddressTableContent() {
	var results []struct {
		Address string
		Status  string
		Npub    *string
	}

	store.DB.Raw(`
        SELECT address, status, npub 
        FROM subscriber_addresses 
        WHERE status = 'allocated' 
        ORDER BY allocated_at DESC 
        LIMIT 5
    `).Scan(&results)

	log.Printf("=== Recent Allocated Addresses ===")
	for _, r := range results {
		log.Printf("Address: %s, Status: %s, Npub: %v",
			r.Address, r.Status, r.Npub)
	}
	log.Printf("=================================")
}
