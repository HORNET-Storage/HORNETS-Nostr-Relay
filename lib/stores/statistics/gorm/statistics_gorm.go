package gorm

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/subscription"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/dag"
)

// BatchSize defines how many records to insert in a single transaction
const BatchSize = 50

// GormStatisticsStore is a GORM-based implementation of the StatisticsStore interface.
type GormStatisticsStore struct {
	DB    *gorm.DB
	mutex sync.RWMutex

	// Function-specific mutexes to avoid global locking
	walletBalanceMutex sync.RWMutex
	bitcoinRateMutex   sync.RWMutex
	walletTxMutex      sync.RWMutex
	eventKindMutex     sync.RWMutex
	addressMutex       sync.RWMutex
}

const (
	AddressStatusAvailable = "available"
	AddressStatusAllocated = "allocated"
)

// Generic Init for gorm
func (store *GormStatisticsStore) Init() error {

	err := store.DB.AutoMigrate(
		&types.Kind{},
		&types.FileInfo{},
		&types.UserProfile{},
		&types.AdminUser{},
		&types.WalletBalance{},
		&types.WalletTransactions{},
		&types.BitcoinRate{},
		&types.WalletAddress{},
		&types.UserChallenge{},
		&types.PendingTransaction{},
		&types.ActiveToken{},
		&types.SubscriberAddress{},
		&types.FileTag{},
		&types.PaidSubscriber{},
		&types.ModerationNotification{},
		&types.PaymentNotification{},
		&types.ReportNotification{}, // Add ReportNotification to be migrated
		&types.AllowedReadNpub{},    // NEW: Add NPUB access control tables
		&types.AllowedWriteNpub{},   // NEW: Add NPUB access control tables
	)
	if err != nil {
		return fmt.Errorf("failed to migrate database schema: %v", err)
	}

	return nil
}

func (store *GormStatisticsStore) AllocateBitcoinAddress(npub string) (*types.Address, error) {
	// Use a dedicated mutex for address allocation
	store.addressMutex.Lock()
	defer store.addressMutex.Unlock()

	// Maximum retries for database operations
	const maxRetries = 8 // High retry count for this critical operation

	var addressToReturn *types.Address
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Use timeout context
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second) // Extended timeout
		defer cancel()

		// Execute transaction with explicit context timeout
		err = store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// Step 1: Check if the npub already has an allocated address
			var existingAddressRecord types.SubscriberAddress
			err := tx.Where("npub = ?", npub).First(&existingAddressRecord).Error
			if err == nil {
				// If an existing record is found, prepare it to return
				addressToReturn = &types.Address{
					IndexHornets: existingAddressRecord.IndexHornets,
					Address:      existingAddressRecord.Address,
					WalletName:   existingAddressRecord.WalletName,
					Status:       existingAddressRecord.Status,
					AllocatedAt:  existingAddressRecord.AllocatedAt,
					Npub:         npub,
				}
				return nil
			} else if err != gorm.ErrRecordNotFound {
				// If another error occurred (not record not found), return error
				return fmt.Errorf("failed to query existing address for npub: %v", err)
			}

			// Step 2: Allocate a new address if no existing address is found
			var addressRecord types.SubscriberAddress
			err = tx.Where("status = ? AND (npub IS NULL OR npub = '')", AddressStatusAvailable).
				Order("id").
				First(&addressRecord).Error

			if err != nil {
				if err == gorm.ErrRecordNotFound {
					return fmt.Errorf("no available addresses")
				}
				return fmt.Errorf("failed to query available addresses: %v", err)
			}

			// Step 3: Store the address allocation in a local variable to return
			// Step 3: Update the address record with the npub
			now := time.Now()
			updates := map[string]interface{}{
				"status":       AddressStatusAllocated,
				"allocated_at": &now,
				"npub":         npub,
			}

			if err := tx.Model(&addressRecord).Updates(updates).Error; err != nil {
				return fmt.Errorf("failed to update address record: %v", err)
			}

			// Store the address allocation in a local variable to return
			addressToReturn = &types.Address{
				IndexHornets: addressRecord.IndexHornets,
				Address:      addressRecord.Address,
				WalletName:   addressRecord.WalletName,
				Status:       AddressStatusAllocated,
				AllocatedAt:  &now,
				Npub:         npub,
			}

			return nil
		})

		if err == nil {
			log.Printf("Successfully allocated address for npub %s after %d attempts", npub, attempt+1)
			return addressToReturn, nil
		}

		// If this is a database lock error, retry with longer exponential backoff
		if strings.Contains(err.Error(), "database is locked") ||
			strings.Contains(err.Error(), "busy") ||
			strings.Contains(err.Error(), "tx read conflict") {
			backoffTime := time.Duration(1000*(1<<attempt)) * time.Millisecond // Base of 1 second for address allocation
			log.Printf("Database lock detected when allocating address, retrying in %v: %v", backoffTime, err)
			time.Sleep(backoffTime)
			continue
		}

		// For other errors, break the loop and return the error
		break
	}

	return nil, fmt.Errorf("failed to allocate address after %d attempts: %v", maxRetries, err)
}

// CountAvailableAddresses counts the number of available addresses in the database
func (store *GormStatisticsStore) CountAvailableAddresses() (int64, error) {
	var count int64
	err := store.DB.Model(&types.SubscriberAddress{}).
		Where("status = ?", AddressStatusAvailable).
		Count(&count).Error

	if err != nil {
		return 0, fmt.Errorf("failed to count available addresses: %v", err)
	}

	return count, nil
}

// SaveBitcoinRate checks if the rate has changed and updates it in the database
func (store *GormStatisticsStore) SaveBitcoinRate(rate float64) error {
	// Query the latest Bitcoin rate
	var latestBitcoinRate types.BitcoinRate
	result := store.DB.Order("timestamp_hornets desc").First(&latestBitcoinRate)

	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		log.Printf("Error querying bitcoin rate: %v", result.Error)
		return result.Error
	}

	// Convert current rate to string for comparison
	rateStr := fmt.Sprintf("%.8f", rate)

	// If the rate is the same as the latest entry, no update needed
	if result.Error == nil && latestBitcoinRate.Rate == rateStr {
		log.Println("Rate is the same as the latest entry, no update needed")
		return nil
	}

	// Add the new rate
	newRate := types.BitcoinRate{
		Rate:             rateStr,
		TimestampHornets: time.Now(),
	}
	if err := store.DB.Create(&newRate).Error; err != nil {
		log.Printf("Error saving new rate: %v", err)
		return err
	}

	log.Println("Bitcoin rate updated successfully")
	return nil
}

// GetBitcoinRates for the last x days
func (store *GormStatisticsStore) GetBitcoinRates(days int) ([]types.BitcoinRate, error) {
	timePassed := time.Now().AddDate(0, 0, days)

	// Query the Bitcoin rates for the last x days
	var bitcoinRates []types.BitcoinRate
	result := store.DB.Where("timestamp_hornets >= ?", timePassed).Order("timestamp_hornets asc").Find(&bitcoinRates)

	if result.Error != nil {
		log.Printf("Error querying Bitcoin rates: %v", result.Error)
		return nil, result.Error
	}

	return bitcoinRates, nil
}

// SavePendingTransaction saves an unconfirmed transaction to the database.
func (store *GormStatisticsStore) SavePendingTransaction(transaction types.PendingTransaction) error {
	return store.DB.Create(&transaction).Error
}

// GetPendingTransactionByID retrieves a pending transaction by ID.
func (store *GormStatisticsStore) GetPendingTransactionByID(id string) (*types.PendingTransaction, error) {
	var transaction types.PendingTransaction
	result := store.DB.First(&transaction, "id = ?", id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &transaction, nil
}

// SaveEventKind stores event kind information in the database.
func (store *GormStatisticsStore) SaveEventKind(event *nostr.Event) error {
	// Use event-specific mutex to prevent concurrent access
	store.eventKindMutex.Lock()
	defer store.eventKindMutex.Unlock()

	kindStr := fmt.Sprintf("kind%d", event.Kind)

	// Maximum retries for database operations
	const maxRetries = 6 // Increased from 3 to handle persistent locks

	settings, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get config: %v", err)
	}

	// Try multiple times with backoff
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Use timeout context for the transaction
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Increased from 5s to 10s
		defer cancel()

		// Start a database transaction
		err = store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// Handle user profile creation or update if the event is of kind 0
			if event.Kind == 0 {
				var contentData map[string]interface{}
				if err := jsoniter.Unmarshal([]byte(event.Content), &contentData); err != nil {
					log.Println("No lightningAddr or dhtKey keys in event content, proceeding with default values.")
					contentData = map[string]interface{}{}
				}

				npubKey := event.PubKey
				lightningAddr := false
				dhtKey := false

				if nip05, ok := contentData["nip05"].(string); ok && nip05 != "" {
					lightningAddr = true
				}

				if dht, ok := contentData["dht-key"].(string); ok && dht != "" {
					dhtKey = true
				}

				// Use the same transaction for profile update
				if err := store.upsertUserProfileTx(tx, npubKey, lightningAddr, dhtKey, time.Unix(int64(event.CreatedAt), 0)); err != nil {
					log.Printf("Error upserting user profile: %v", err)
					return err
				}
			}

			// If the event kind matches relay settings, store it in the database
			if contains(settings.EventFiltering.KindWhitelist, kindStr) {
				// Check if event already exists to avoid duplicates
				var count int64
				if err := tx.Model(&types.Kind{}).Where("event_id = ?", event.ID).Count(&count).Error; err != nil {
					return err
				}

				if count > 0 {
					// Event already exists, skip insertion
					log.Printf("Event %s already exists in the database, skipping", event.ID)
					return nil
				}

				sizeBytes := len(event.ID) + len(event.PubKey) + len(event.Content) + len(event.Sig)
				for _, tag := range event.Tags {
					for _, t := range tag {
						sizeBytes += len(t)
					}
				}
				sizeMB := float64(sizeBytes) / (1024 * 1024)

				kind := types.Kind{
					KindNumber: event.Kind,
					EventID:    event.ID,
					Size:       sizeMB,
				}
				if err := tx.Create(&kind).Error; err != nil {
					return err
				}
			}

			return nil
		})

		if err == nil {
			// Success - no need to retry
			return nil
		}

		// If this is a database lock error, retry
		if strings.Contains(err.Error(), "database is locked") ||
			strings.Contains(err.Error(), "busy") ||
			strings.Contains(err.Error(), "tx read conflict") {
			backoffTime := time.Duration(100*(1<<attempt)) * time.Millisecond
			log.Printf("Database lock detected when saving event kind, retrying in %v: %v", backoffTime, err)
			time.Sleep(backoffTime)
			continue
		}

		// For other errors, break and return the error
		break
	}

	return err
}

// upsertUserProfileTx is an internal helper function for SaveEventKind
// that uses the provided transaction to upsert a user profile
func (store *GormStatisticsStore) upsertUserProfileTx(tx *gorm.DB, npubKey string, lightningAddr, dhtKey bool, createdAt time.Time) error {
	var userProfile types.UserProfile
	result := tx.Where("npub_key = ?", npubKey).First(&userProfile)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// Create new user profile
			userProfile = types.UserProfile{
				NpubKey:          npubKey,
				LightningAddr:    lightningAddr,
				DHTKey:           dhtKey,
				TimestampHornets: createdAt,
			}
			return tx.Create(&userProfile).Error
		}
		return result.Error
	}

	// Update existing user profile
	userProfile.LightningAddr = lightningAddr
	userProfile.DHTKey = dhtKey
	userProfile.TimestampHornets = createdAt
	return tx.Save(&userProfile).Error
}

// UpsertUserProfile inserts or updates the user profile in the database.
func (store *GormStatisticsStore) UpsertUserProfile(npubKey string, lightningAddr, dhtKey bool, createdAt time.Time) error {
	var userProfile types.UserProfile
	result := store.DB.Where("npub_key = ?", npubKey).First(&userProfile)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// Create new user profile
			userProfile = types.UserProfile{
				NpubKey:          npubKey,
				LightningAddr:    lightningAddr,
				DHTKey:           dhtKey,
				TimestampHornets: createdAt,
			}
			return store.DB.Create(&userProfile).Error
		}
		return result.Error
	}

	// Update existing user profile
	userProfile.LightningAddr = lightningAddr
	userProfile.DHTKey = dhtKey
	userProfile.TimestampHornets = createdAt
	return store.DB.Save(&userProfile).Error
}

// Utility function to check if an item exists in a slice
func contains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

// SaveFile saves the file (photo, video, audio, or misc) based on its type and processing mode (whitelist or blacklist).
func (store *GormStatisticsStore) SaveFile(root string, hash string, fileName string, mimeType string, leafCount int, size int64) error {
	file := types.FileInfo{
		Root:      root,
		Hash:      hash,
		FileName:  fileName,
		MimeType:  mimeType,
		LeafCount: leafCount,
		Size:      size,
	}

	// Use FirstOrCreate to avoid UNIQUE constraint violations on hash
	result := store.DB.Where(types.FileInfo{Hash: hash}).FirstOrCreate(&file)
	return result.Error
}

func (store *GormStatisticsStore) QueryFiles(criteria map[string]interface{}) ([]types.FileInfo, error) {
	var files []types.FileInfo
	query := store.DB.Model(&types.FileInfo{})

	// Apply each criteria to the query
	for key, value := range criteria {
		query = query.Where(key+" = ?", value)
	}

	err := query.Find(&files).Error
	if err != nil {
		log.Printf("Error querying files: %v", err)
		return nil, err
	}

	return files, nil
}

func (store *GormStatisticsStore) SaveTags(root string, leaf *merkle_dag.DagLeaf) error {
	for key, value := range leaf.AdditionalData {
		tag := types.FileTag{
			Root:  root,
			Key:   key,
			Value: value,
		}

		// Use pointer and specify search conditions for FirstOrCreate
		result := store.DB.Where(&types.FileTag{
			Root:  root,
			Key:   key,
			Value: value,
		}).FirstOrCreate(&tag)

		if result.Error != nil {
			return result.Error
		}
	}

	return nil
}

func (store *GormStatisticsStore) QueryTags(tags map[string]string) ([]string, error) {
	query := store.DB.Model(&types.FileTag{})

	for key, value := range tags {
		query = query.Where("key = ? AND value = ?", key, value)
	}

	var roots []string
	err := query.Distinct("root").Pluck("root", &roots).Error
	if err != nil {
		log.Printf("Error querying file tags: %v", err)
		return nil, err
	}

	return roots, nil
}

func (store *GormStatisticsStore) DeleteEventByID(eventID string) error {
	// Delete the event from the Kind table in the GORM database
	if err := store.DB.Delete(&types.Kind{}, "event_id = ?", eventID).Error; err != nil {
		return err
	}
	return nil
}

// FetchKindData retrieves and aggregates kind data from the database
func (store *GormStatisticsStore) FetchKindData() ([]types.AggregatedKindData, error) {
	var kinds []types.Kind
	if err := store.DB.Find(&kinds).Error; err != nil {
		log.Println("Error fetching kinds:", err)
		return nil, err
	}

	log.Println("Stats DB kinds: ", kinds)

	aggregatedData := make(map[int]types.AggregatedKindData)

	// Aggregate the data by kind number
	for _, kind := range kinds {
		if data, exists := aggregatedData[kind.KindNumber]; exists {
			data.KindCount++
			data.TotalSize += kind.Size
			aggregatedData[kind.KindNumber] = data
		} else {
			aggregatedData[kind.KindNumber] = types.AggregatedKindData{
				KindNumber: kind.KindNumber,
				KindCount:  1,
				TotalSize:  kind.Size,
			}
		}
	}

	// Convert the map to a slice
	result := []types.AggregatedKindData{}
	for _, data := range aggregatedData {
		result = append(result, data)
	}

	// Sort the result by TotalSize in descending order
	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalSize > result[j].TotalSize
	})

	return result, nil
}

// FetchKindTrendData fetches and aggregates kind trend data for the past 12 months
func (store *GormStatisticsStore) FetchKindTrendData(kindNumber int) ([]types.MonthlyKindData, error) {
	var data []types.KindData
	query := `
		SELECT timestamp_hornets, size
		FROM kinds
		WHERE kind_number = ? AND timestamp_hornets >= date('now', '-12 months')
	`
	err := store.DB.Raw(query, kindNumber).Scan(&data).Error
	if err != nil {
		log.Println("Error fetching kind data:", err)
		return nil, err
	}

	if len(data) == 0 {
		log.Println("No data found for the specified kind number and time range")
		return nil, nil
	}

	// Aggregate data by month
	monthlyData := make(map[string]float64)
	for _, row := range data {
		month := row.TimestampHornets.Format("2006-01")
		monthlyData[month] += row.Size
	}

	// Convert the map to a slice of MonthlyKindData
	var result []types.MonthlyKindData
	for month, totalSize := range monthlyData {
		result = append(result, types.MonthlyKindData{Month: month, TotalSize: totalSize})
	}

	// Sort the result by month in ascending order
	sort.Slice(result, func(i, j int) bool {
		return result[i].Month < result[j].Month
	})

	return result, nil
}

// FindUserByNpub finds a user by their npub (public key)
func (store *GormStatisticsStore) FindUserByNpub(npub string) (*types.AdminUser, error) {
	var user types.AdminUser
	if err := store.DB.Where("npub = ?", npub).First(&user).Error; err != nil {
		log.Printf("User not found: %v", err)
		return nil, err
	}
	return &user, nil
}

// ComparePasswords compares the hashed password with the input password
func (store *GormStatisticsStore) ComparePasswords(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}

// SaveUserChallenge saves the user challenge in the database
func (store *GormStatisticsStore) SaveUserChallenge(userChallenge *types.UserChallenge) error {
	// Check if challenge already exists
	var count int64
	if err := store.DB.Model(&types.UserChallenge{}).
		Where("challenge = ? AND expired = false", userChallenge.Challenge).
		Count(&count).Error; err != nil {
		log.Printf("Failed to check existing challenge: %v", err)
		return err
	}

	if count > 0 {
		return fmt.Errorf("active challenge already exists")
	}

	// Save the new challenge
	if err := store.DB.Create(userChallenge).Error; err != nil {
		log.Printf("Failed to save user challenge: %v", err)
		return err
	}

	return nil
}

// DeleteActiveToken deletes the given token from the ActiveTokens table
func (store *GormStatisticsStore) DeleteActiveToken(userID uint) error {
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		err := store.DB.Transaction(func(tx *gorm.DB) error {
			result := tx.Exec("DELETE FROM active_tokens WHERE user_id = ?", userID)
			if result.Error != nil {
				return result.Error
			}

			if result.RowsAffected == 0 {
				log.Printf("No tokens found for user %d", userID)
			} else {
				log.Printf("Successfully deleted %d tokens for user %d", result.RowsAffected, userID)
			}

			return nil
		})

		if err == nil {
			return nil
		}

		// If it's not a read conflict, return the error
		if !strings.Contains(err.Error(), "tx read conflict") {
			return err
		}

		// If this was the last retry, return the error
		if i == maxRetries-1 {
			log.Printf("Failed to delete tokens after %d retries: %v", maxRetries, err)
			return err
		}

		// Wait before retrying
		time.Sleep(time.Millisecond * time.Duration(100*(i+1)))
	}

	return nil
}

func (store *GormStatisticsStore) FindUserByToken(token string) (*types.AdminUser, error) {
	var activeToken types.ActiveToken

	// Get current time in RFC3339 format
	nowStr := time.Now().Format(time.RFC3339)

	// Query using string comparison
	if err := store.DB.Where("token = ? AND expires_at > ?", token, nowStr).First(&activeToken).Error; err != nil {
		return nil, err
	}

	// Parse the stored expiry time and verify it's still valid
	expiryTime, err := time.Parse(time.RFC3339, activeToken.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse expiry time: %v", err)
	}

	if time.Now().After(expiryTime) {
		return nil, gorm.ErrRecordNotFound
	}

	var user types.AdminUser
	if err := store.DB.First(&user, activeToken.UserID).Error; err != nil {
		return nil, err
	}

	return &user, nil
}

// FetchMonthlyStorageStats retrieves the monthly storage stats (total GBs per month)
func (store *GormStatisticsStore) FetchMonthlyStorageStats() ([]types.ActivityData, error) {
	var data []struct {
		Month time.Time `gorm:"column:month"`
		Size  float64   `gorm:"column:size"`
	}

	err := store.DB.Raw(`
		SELECT timestamp_hornets as month, size
		FROM kinds
	`).Scan(&data).Error
	if err != nil {
		return nil, err
	}

	// Group and calculate in Go
	monthlyData := make(map[string]float64)
	for _, d := range data {
		key := d.Month.Format("2006-01")
		monthlyData[key] += d.Size / 1024.0
	}

	result := make([]types.ActivityData, 0, len(monthlyData))
	for month, size := range monthlyData {
		result = append(result, types.ActivityData{
			Month:   month,
			TotalGB: math.Round(size*1000) / 1000,
		})
	}

	return result, nil
}

// FetchNotesMediaStorageData retrieves the total GBs per month for notes and media
func (store *GormStatisticsStore) FetchNotesMediaStorageData() ([]types.BarChartData, error) {
	// Get notes data from kinds table
	var notesData []struct {
		Month time.Time `gorm:"column:month"`
		Size  float64   `gorm:"column:size"`
	}

	err := store.DB.Raw(`
        SELECT timestamp_hornets as month, size
        FROM kinds
    `).Scan(&notesData).Error
	if err != nil {
		return nil, err
	}

	// Get media data from file_infos table
	var mediaData []struct {
		Month    time.Time `gorm:"column:month"`
		MimeType string    `gorm:"column:mime_type"`
		Size     int64     `gorm:"column:size"`
	}

	err = store.DB.Raw(`
        SELECT timestamp_hornets as month, mime_type, size
        FROM file_infos
    `).Scan(&mediaData).Error
	if err != nil {
		return nil, err
	}

	// Group notes by month
	notesMonthData := make(map[string]float64)
	for _, d := range notesData {
		key := d.Month.Format("2006-01")
		notesMonthData[key] += d.Size / 1024.0 // Convert to GB
	}

	// Group media by month
	mediaMonthData := make(map[string]float64)
	for _, d := range mediaData {
		key := d.Month.Format("2006-01")
		mediaMonthData[key] += float64(d.Size) / (1024.0 * 1024.0 * 1024.0) // Convert bytes to GB
	}

	// Combine all months from both datasets
	allMonths := make(map[string]bool)
	for month := range notesMonthData {
		allMonths[month] = true
	}
	for month := range mediaMonthData {
		allMonths[month] = true
	}

	// Convert to BarChartData
	result := make([]types.BarChartData, 0, len(allMonths))
	for month := range allMonths {
		result = append(result, types.BarChartData{
			Month:   month,
			NotesGB: notesMonthData[month],
			MediaGB: mediaMonthData[month],
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Month < result[j].Month
	})

	return result, nil
}

// FetchProfilesTimeSeriesData retrieves the time series data for profiles over the last 6 months
// with cumulative totals rather than monthly counts
func (store *GormStatisticsStore) FetchProfilesTimeSeriesData(startDate, endDate string) ([]types.TimeSeriesData, error) {
	var rawData []struct {
		Month     string `gorm:"column:month"`
		Profiles  int    `gorm:"column:total"`
		Lightning int    `gorm:"column:lightning"`
		DHTKey    int    `gorm:"column:dht"`
		Both      int    `gorm:"column:both"`
	}

	log.Printf("FetchProfilesTimeSeriesData: startDate=%s, endDate=%s", startDate, endDate)

	// First, let's make sure we include all months in our range, even if there's no data for a month
	// Then calculate the cumulative totals for each month
	err := store.DB.Raw(`
   WITH date_range AS (
       SELECT
           date(?) as start_date,
           date(?) as end_date
   ),
   all_months AS (
       WITH RECURSIVE months(dt) AS (
           SELECT date(start_date, 'start of month') FROM date_range
           UNION ALL
           SELECT date(dt, '+1 month') FROM months
           WHERE dt < date((SELECT end_date FROM date_range), 'start of month')
       )
       SELECT strftime('%Y-%m', dt) as month FROM months
   ),
   data_by_month AS (
       SELECT
           strftime('%Y-%m', timestamp_hornets) as month,
           COUNT(*) as monthly_total,
           SUM(CASE WHEN lightning_addr = 1 THEN 1 ELSE 0 END) as monthly_lightning,
           SUM(CASE WHEN dht_key = 1 THEN 1 ELSE 0 END) as monthly_dht,
           SUM(CASE WHEN lightning_addr = 1 AND dht_key = 1 THEN 1 ELSE 0 END) as monthly_both
       FROM user_profiles
       WHERE timestamp_hornets >= ? AND timestamp_hornets < ?
       GROUP BY month
   )
   SELECT
       am.month,
       (SELECT SUM(monthly_total) FROM data_by_month WHERE month <= am.month) as total,
       (SELECT SUM(monthly_lightning) FROM data_by_month WHERE month <= am.month) as lightning,
       (SELECT SUM(monthly_dht) FROM data_by_month WHERE month <= am.month) as dht,
       (SELECT SUM(monthly_both) FROM data_by_month WHERE month <= am.month) as both
   FROM all_months am
   ORDER BY am.month
`, startDate, endDate, startDate, endDate).Scan(&rawData).Error

	if err != nil {
		return nil, fmt.Errorf("error fetching time series data: %v", err)
	}

	data := make([]types.TimeSeriesData, len(rawData))
	for i, raw := range rawData {
		data[i] = types.TimeSeriesData{
			Month:           raw.Month,
			Profiles:        raw.Profiles,
			LightningAddr:   raw.Lightning,
			DHTKey:          raw.DHTKey,
			LightningAndDHT: raw.Both,
		}
	}

	return data, nil
}

func (store *GormStatisticsStore) FetchWalletAddresses() ([]types.WalletAddress, error) {
	var walletAddresses []types.WalletAddress

	// Query the wallet addresses from the database
	if err := store.DB.Find(&walletAddresses).Error; err != nil {
		log.Printf("Error fetching wallet addresses: %v", err)
		return nil, err
	}

	return walletAddresses, nil
}

// FetchKindCount retrieves the count of kinds from the database
func (store *GormStatisticsStore) FetchKindCount() (int, error) {
	var count int64
	err := store.DB.Model(&types.Kind{}).Count(&count).Error
	return int(count), err
}

// FetchFileCountByType retrieves the count of stored files for a specific mime type
func (store *GormStatisticsStore) FetchFileCountByType(mimeType string) (int, error) {
	var count int64
	err := store.DB.Model(&types.FileInfo{}).Where("mime_type = ?", mimeType).Count(&count).Error
	return int(count), err
}

func (store *GormStatisticsStore) FetchFilesByType(mimeType string, page int, pageSize int) ([]types.FileInfo, *types.PaginationMetadata, error) {
	var total int64

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	offset := (page - 1) * pageSize

	result := store.DB.Model(&types.FileInfo{}).Where("mime_type = ?", mimeType).Count(&total)
	if result.Error != nil {
		return nil, nil, result.Error
	}

	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))

	var files []types.FileInfo
	result = store.DB.Where("mime_type = ?", mimeType).
		Order("timestamp_hornets DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&files)

	if result.Error != nil {
		return nil, nil, result.Error
	}

	metaData := &types.PaginationMetadata{
		CurrentPage: page,
		PageSize:    pageSize,
		TotalItems:  total,
		TotalPages:  totalPages,
		HasNext:     page < totalPages,
		HasPrevious: page > 1,
	}

	return files, metaData, nil
}

// ReplaceTransaction handles replacing a pending transaction with a new one
func (store *GormStatisticsStore) ReplaceTransaction(replaceRequest types.ReplaceTransactionRequest) error {
	// Delete the original pending transaction
	var originalPendingTransaction types.PendingTransaction
	if err := store.DB.Where("tx_id = ?", replaceRequest.OriginalTxID).First(&originalPendingTransaction).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Printf("No pending transaction found with TxID %s", replaceRequest.OriginalTxID)
			return gorm.ErrRecordNotFound
		}
		log.Printf("Error querying original transaction with TxID %s: %v", replaceRequest.OriginalTxID, err)
		return err
	}

	if err := store.DB.Delete(&originalPendingTransaction).Error; err != nil {
		log.Printf("Error deleting pending transaction with TxID %s: %v", replaceRequest.OriginalTxID, err)
		return err
	}
	log.Printf("Deleted original pending transaction with TxID %s", replaceRequest.OriginalTxID)

	// Save the new pending transaction
	newPendingTransaction := types.PendingTransaction{
		TxID:             replaceRequest.NewTxID,
		FeeRate:          replaceRequest.NewFeeRate,
		Amount:           replaceRequest.Amount,
		RecipientAddress: replaceRequest.RecipientAddress,
		TimestampHornets: time.Now(),
	}

	if err := store.DB.Create(&newPendingTransaction).Error; err != nil {
		log.Printf("Error saving new pending transaction: %v", err)
		return err
	}

	return nil
}

// SaveUnconfirmedTransaction saves an unconfirmed transaction to the database
func (store *GormStatisticsStore) SaveUnconfirmedTransaction(pendingTransaction *types.PendingTransaction) error {
	// Ensure Timestamp is populated
	pendingTransaction.TimestampHornets = time.Now()

	// Save the pending transaction to the database
	if err := store.DB.Create(pendingTransaction).Error; err != nil {
		log.Printf("Error saving pending transaction: %v", err)
		return err
	}

	return nil
}

// SignUpUser registers a new user in the database
func (store *GormStatisticsStore) SignUpUser(npub string, password string) error {
	// Check if npub already exists
	var count int64
	if err := store.DB.Model(&types.AdminUser{}).Where("npub = ?", npub).Count(&count).Error; err != nil {
		log.Printf("Failed to check existing user: %v", err)
		return err
	}
	if count > 0 {
		return fmt.Errorf("user with npub %s already exists", npub)
	}

	// Hash the password before saving it
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Failed to hash password: %v", err)
		return err
	}

	// Create the user object
	user := types.AdminUser{
		Pass: string(hashedPassword),
		Npub: npub,
	}

	// Save the user in the database
	if err := store.DB.Create(&user).Error; err != nil {
		log.Printf("Failed to create user: %v", err)
		return err
	}

	return nil
}

// GetPendingTransactions retrieves all pending transactions from the database
func (store *GormStatisticsStore) GetPendingTransactions() ([]types.PendingTransaction, error) {
	var pendingTransactions []types.PendingTransaction

	// Query all pending transactions ordered by timestamp (descending)
	result := store.DB.Order("timestamp_hornets desc").Find(&pendingTransactions)
	if result.Error != nil {
		log.Printf("Error querying pending transactions: %v", result.Error)
		return nil, result.Error
	}

	return pendingTransactions, nil
}

// UpdateBitcoinRate checks the latest rate and updates the database if it's new
func (store *GormStatisticsStore) UpdateBitcoinRate(rate float64) error {
	// Use Bitcoin rate-specific mutex
	store.bitcoinRateMutex.Lock()
	defer store.bitcoinRateMutex.Unlock()

	log.Printf("Updating Bitcoin rate to %.8f", rate)

	// Maximum retries for database operations
	const maxRetries = 6 // Increased from 3 to handle persistent locks

	// Convert current rate to string for comparison
	rateStr := fmt.Sprintf("%.8f", rate)

	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Use timeout context for the transaction with increased timeout
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Increased from 5s to 10s
		defer cancel()

		// Start a database transaction with a timeout
		err = store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// Query the latest Bitcoin rate
			var latestBitcoinRate types.BitcoinRate
			result := tx.Order("timestamp_hornets desc").First(&latestBitcoinRate)

			if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
				log.Printf("Error querying bitcoin rate: %v", result.Error)
				return result.Error
			}

			if result.Error == nil && latestBitcoinRate.Rate == rateStr {
				// If the rate is the same as the latest entry, no update needed
				log.Println("Rate is the same as the latest entry, no update needed")
				return nil
			}

			// Add the new rate
			newRate := types.BitcoinRate{
				Rate:             rateStr,
				TimestampHornets: time.Now(),
			}

			if err := tx.Create(&newRate).Error; err != nil {
				log.Printf("Error saving new rate: %v", err)
				return err
			}

			return nil
		})

		if err == nil {
			log.Println("Bitcoin rate updated successfully")
			return nil
		}

		// If this is a database lock error, retry with increased backoff
		if strings.Contains(err.Error(), "database is locked") ||
			strings.Contains(err.Error(), "busy") ||
			strings.Contains(err.Error(), "tx read conflict") {
			// Longer exponential backoff with base of 500ms instead of 100ms
			backoffTime := time.Duration(500*(1<<attempt)) * time.Millisecond
			log.Printf("Database lock detected when updating Bitcoin rate, retrying in %v: %v", backoffTime, err)
			time.Sleep(backoffTime)
			continue
		}

		// For other errors, break and return the error
		break
	}

	if err != nil {
		log.Printf("Failed to update Bitcoin rate after %d attempts: %v", maxRetries, err)
	}

	return err
}

// UpdateWalletBalance updates the wallet balance if it's new
func (store *GormStatisticsStore) UpdateWalletBalance(walletName, balance string) error {
	// Use wallet-specific mutex to prevent concurrent updates to wallet balance
	store.walletBalanceMutex.Lock()
	defer store.walletBalanceMutex.Unlock()

	log.Printf("Updating wallet balance for %s to %s", walletName, balance)

	// Maximum retries for database operations
	const maxRetries = 6 // Increased from 3 to handle persistent locks

	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Use timeout context for the transaction with increased timeout
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Increased from 5s to 10s
		defer cancel()

		// Start a database transaction with a timeout
		err = store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// Query the latest wallet balance
			var latestBalance types.WalletBalance
			result := tx.Order("timestamp_hornets desc").First(&latestBalance)

			if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
				log.Printf("Error querying latest balance: %v", result.Error)
				return result.Error
			}

			// If the balance is the same as the latest entry, no update needed
			if result.Error == nil && latestBalance.Balance == balance {
				log.Println("Balance is the same as the latest entry, no update needed")
				return nil
			}

			// Add a new balance entry
			newBalance := types.WalletBalance{
				Balance:          balance,
				TimestampHornets: time.Now(),
			}

			if err := tx.Create(&newBalance).Error; err != nil {
				log.Printf("Error saving new balance: %v", err)
				return err
			}

			return nil
		})

		if err == nil {
			log.Println("Wallet balance updated successfully")
			return nil
		}

		// If this is a database lock error, retry with increased backoff
		if strings.Contains(err.Error(), "database is locked") ||
			strings.Contains(err.Error(), "busy") ||
			strings.Contains(err.Error(), "tx read conflict") {
			// Longer exponential backoff with base of 500ms instead of 100ms
			backoffTime := time.Duration(500*(1<<attempt)) * time.Millisecond
			log.Printf("Database lock detected when updating wallet balance, retrying in %v: %v", backoffTime, err)
			time.Sleep(backoffTime)
			continue
		}

		// For other errors, break and return the error
		break
	}

	if err != nil {
		log.Printf("Failed to update wallet balance after %d attempts: %v", maxRetries, err)
		return err
	}

	return nil
}

// SaveWalletTransaction saves a new wallet transaction to the database
func (store *GormStatisticsStore) SaveWalletTransaction(tx types.WalletTransactions) error {
	// Use wallet transaction specific mutex
	store.walletTxMutex.Lock()
	defer store.walletTxMutex.Unlock()

	// Maximum retries for database operations
	const maxRetries = 6 // Increased from 3 to handle persistent locks

	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Use timeout context with increased timeout
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Increased from 5s to 10s
		defer cancel()

		// Start a database transaction with timeout
		err = store.DB.WithContext(ctx).Transaction(func(txDB *gorm.DB) error {
			return txDB.Create(&tx).Error
		})

		if err == nil {
			return nil
		}

		log.Printf("Attempt %d: Error saving wallet transaction: %v", attempt+1, err)

		// Enhanced exponential backoff
		if attempt < maxRetries-1 {
			backoffDuration := time.Millisecond * time.Duration(500*(1<<attempt)) // Base increased to 500ms
			log.Printf("Retrying in %v...", backoffDuration)
			time.Sleep(backoffDuration)
		}
	}

	return fmt.Errorf("failed to save wallet transaction after %d attempts: %v", maxRetries, err)
}

// DeletePendingTransaction deletes a pending transaction from the database by TxID
func (store *GormStatisticsStore) DeletePendingTransaction(txID string) error {
	// Use wallet transaction specific mutex
	store.walletTxMutex.Lock()
	defer store.walletTxMutex.Unlock()

	// Maximum retries for database operations
	const maxRetries = 6 // Increased from 3 to handle persistent locks

	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Use timeout context with increased timeout
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Increased from 5s to 10s
		defer cancel()

		// Execute operation with transaction
		err = store.DB.WithContext(ctx).Transaction(func(txDB *gorm.DB) error {
			var pendingTx types.PendingTransaction
			result := txDB.Where("tx_id = ?", txID).First(&pendingTx)

			if result.Error == nil {
				return txDB.Delete(&pendingTx).Error
			}

			if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
				log.Printf("Error querying pending transaction with TxID %s: %v", txID, result.Error)
				return result.Error
			}

			log.Printf("No pending transaction found with TxID %s", txID)
			return nil
		})

		if err == nil {
			return nil
		}

		log.Printf("Attempt %d: Error deleting pending transaction: %v", attempt+1, err)

		// Enhanced exponential backoff
		if attempt < maxRetries-1 {
			backoffDuration := time.Millisecond * time.Duration(500*(1<<attempt)) // Base increased to 500ms
			log.Printf("Retrying in %v...", backoffDuration)
			time.Sleep(backoffDuration)
		}
	}

	return fmt.Errorf("failed to delete pending transaction after %d attempts: %v", maxRetries, err)
}

// TransactionExists checks if a transaction already exists in the database
func (store *GormStatisticsStore) TransactionExists(address string, date time.Time, output string, value string) (bool, error) {
	// Use a read lock for this operation - since we're only reading
	store.walletTxMutex.RLock()
	defer store.walletTxMutex.RUnlock()

	// Maximum retries for database operations
	const maxRetries = 6 // Increased from 3 to handle persistent locks

	var exists bool
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Use timeout context with increased timeout
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Increased from 5s to 10s
		defer cancel()

		// Query with timeout context
		var existingTransaction types.WalletTransactions
		result := store.DB.WithContext(ctx).
			Where("address = ? AND date = ? AND output = ? AND value = ?",
				address, date, output, value).
			First(&existingTransaction)

		// Handle "Record Not Found" case without raising an error
		if result.Error == gorm.ErrRecordNotFound {
			exists = false
			return false, nil // No transaction exists, but it's not an error
		}

		// If there's another error, we'll retry
		if result.Error != nil {
			err = result.Error
			log.Printf("Attempt %d: Error checking transaction existence: %v", attempt+1, err)

			// Enhanced exponential backoff
			if attempt < maxRetries-1 {
				backoffDuration := time.Millisecond * time.Duration(500*(1<<attempt)) // Base increased to 500ms
				log.Printf("Retrying in %v...", backoffDuration)
				time.Sleep(backoffDuration)
				continue
			}

			return false, fmt.Errorf("failed to check transaction existence after %d attempts: %v", maxRetries, err)
		}

		// If no error, the transaction exists
		exists = true
		return true, nil
	}

	// This should not be reached due to the returns above, but just in case
	return exists, err
}

// UserExists checks if any user exists in the database
func (store *GormStatisticsStore) UserExists() (bool, error) {
	var count int64
	err := store.DB.Model(&types.AdminUser{}).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// AddressExists checks if a given Bitcoin address exists in the database
func (store *GormStatisticsStore) SubscriberAddressExists(address string) (bool, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	var count int64
	err := store.DB.Model(&types.SubscriberAddress{}).
		Where("address = ?", address).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("failed to check subscriber address existence: %v", err)
	}

	return count > 0, nil
}

func (store *GormStatisticsStore) WalletAddressExists(address string) (bool, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	var count int64
	err := store.DB.Model(&types.WalletAddress{}).
		Where("address = ?", address).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("failed to check wallet address existence: %v", err)
	}

	return count > 0, nil
}

func (store *GormStatisticsStore) AddressExists(address string) (bool, error) {
	subscriberExists, err := store.SubscriberAddressExists(address)
	if err != nil {
		return false, err
	}

	walletExists, err := store.WalletAddressExists(address)
	if err != nil {
		return false, err
	}

	return subscriberExists || walletExists, nil
}

func (store *GormStatisticsStore) SaveAddressBatch(addresses []*types.WalletAddress) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	const maxRetries = 6 // Increased from 3 to handle persistent locks

	// Split addresses into batches
	for i := 0; i < len(addresses); i += BatchSize {
		end := i + BatchSize
		if end > len(addresses) {
			end = len(addresses)
		}
		batch := addresses[i:end]

		// Retry logic for each batch
		for retry := 0; retry < maxRetries; retry++ {
			err := store.DB.Transaction(func(tx *gorm.DB) error {
				// Use clause.OnConflict to handle duplicate addresses
				result := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "address"}},
					DoNothing: true,
				}).Create(&batch)

				if result.Error != nil {
					return result.Error
				}
				return nil
			})

			if err == nil {
				break
			}

			if retry == maxRetries-1 {
				log.Printf("Failed to save batch after %d retries: %v", maxRetries, err)
				return err
			}

			// Exponential backoff
			time.Sleep(time.Millisecond * time.Duration(100*(1<<retry)))
		}
	}
	return nil
}

// SaveSubscriberAddressBatch handles batch saving of subscriber addresses
func (store *GormStatisticsStore) SaveSubscriberAddressBatch(addresses []*types.SubscriberAddress) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	const maxRetries = 6 // Increased from 3 to handle persistent locks

	// Split addresses into batches
	for i := 0; i < len(addresses); i += BatchSize {
		end := i + BatchSize
		if end > len(addresses) {
			end = len(addresses)
		}
		batch := addresses[i:end]

		// Retry logic for each batch
		for retry := 0; retry < maxRetries; retry++ {
			err := store.DB.Transaction(func(tx *gorm.DB) error {
				// Use clause.OnConflict to handle duplicate addresses
				result := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "address"}},
					DoNothing: true,
				}).Create(&batch)

				if result.Error != nil {
					return result.Error
				}
				return nil
			})

			if err == nil {
				break
			}

			if retry == maxRetries-1 {
				log.Printf("Failed to save subscriber batch after %d retries: %v", maxRetries, err)
				return err
			}

			// Exponential backoff
			time.Sleep(time.Millisecond * time.Duration(100*(1<<retry)))
		}
	}
	return nil
}

// SaveAddress saves a new wallet address to the database
func (store *GormStatisticsStore) SaveAddress(address *types.WalletAddress) error {
	return store.SaveAddressBatch([]*types.WalletAddress{address})
}

// GetLatestWalletBalance retrieves the latest wallet balance from the database
func (store *GormStatisticsStore) GetLatestWalletBalance() (types.WalletBalance, error) {
	var latestBalance types.WalletBalance
	result := store.DB.Order("timestamp_hornets desc").First(&latestBalance)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			latestBalance.Balance = "0" // Default balance if not found
			return latestBalance, nil
		}
		return types.WalletBalance{}, result.Error
	}
	return latestBalance, nil
}

// GetLatestBitcoinRate retrieves the latest Bitcoin rate from the database
func (store *GormStatisticsStore) GetLatestBitcoinRate() (types.BitcoinRate, error) {
	var bitcoinRate types.BitcoinRate

	// Simpler query without complex ordering
	result := store.DB.Select("*").
		Table("bitcoin_rates").
		Limit(1).
		Find(&bitcoinRate)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			bitcoinRate.Rate = "0.00000000" // Default rate if not found
			return bitcoinRate, nil
		}
		return types.BitcoinRate{}, result.Error
	}
	return bitcoinRate, nil
}

// GetUserChallenge retrieves a valid user challenge from the database
func (store *GormStatisticsStore) GetUserChallenge(challenge string) (types.UserChallenge, error) {
	var userChallenge types.UserChallenge
	if err := store.DB.Where("challenge = ? AND expired = ?", challenge, false).First(&userChallenge).Error; err != nil {
		return userChallenge, err
	}
	return userChallenge, nil
}

// MarkChallengeExpired marks the given challenge as expired
func (store *GormStatisticsStore) MarkChallengeExpired(userChallenge *types.UserChallenge) error {
	return store.DB.Model(userChallenge).Update("expired", true).Error
}

// GetUserByID retrieves a user by their ID
func (store *GormStatisticsStore) GetUserByID(userID uint) (types.AdminUser, error) {
	var user types.AdminUser
	if err := store.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		return user, err
	}
	return user, nil
}

// StoreActiveToken saves the generated active JWT token in the database
func (store *GormStatisticsStore) StoreActiveToken(activeToken *types.ActiveToken) error {
	// First try to delete any existing tokens for this user
	if err := store.DeleteActiveToken(activeToken.UserID); err != nil {
		log.Printf("Warning: failed to delete existing tokens: %v", err)
		// Continue anyway as we still want to try creating the new token
	}

	// Create a copy with string timestamp
	tokenToStore := &types.ActiveToken{
		UserID:    activeToken.UserID,
		Token:     activeToken.Token,
		ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339), // Format new expiry time
	}

	// Try to create the new token
	return store.DB.Create(tokenToStore).Error
}

// GetLatestWalletTransactions retrieves all wallet transactions ordered by date descending
func (store *GormStatisticsStore) GetLatestWalletTransactions() ([]types.WalletTransactions, error) {
	var transactions []types.WalletTransactions
	result := store.DB.Order("date desc").Find(&transactions)
	if result.Error != nil {
		return nil, result.Error
	}
	return transactions, nil
}

// IsActiveToken checks if a token is active and not expired
func (store *GormStatisticsStore) IsActiveToken(token string) (bool, error) {
	var activeToken types.ActiveToken

	// Convert current time to the same string format
	nowStr := time.Now().Format(time.RFC3339)

	// Query using string comparison
	if err := store.DB.Where("token = ? AND expires_at > ?", token, nowStr).First(&activeToken).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, err
	}

	// Parse the stored expiry time and compare
	expiryTime, err := time.Parse(time.RFC3339, activeToken.ExpiresAt)
	if err != nil {
		return false, fmt.Errorf("failed to parse expiry time: %v", err)
	}

	return time.Now().Before(expiryTime), nil
}

// GetSubscriberByAddress retrieves subscriber information by Bitcoin address
func (store *GormStatisticsStore) GetSubscriberByAddress(address string) (*types.SubscriberAddress, error) {
	// Use address-specific mutex for reading subscribers
	store.addressMutex.RLock()
	defer store.addressMutex.RUnlock()

	// Maximum retries for database operations
	const maxRetries = 6 // Increased from 3 to handle persistent locks

	var subscriber types.SubscriberAddress
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Use timeout context
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Increased from 5s to 10s
		defer cancel()

		err = store.DB.WithContext(ctx).Where("address = ?", address).First(&subscriber).Error

		if err == nil {
			return &subscriber, nil
		}

		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("no subscriber found for address: %s", address)
		}

		log.Printf("Attempt %d: Error querying subscriber by address: %v", attempt+1, err)

		// Exponential backoff for retries
		if attempt < maxRetries-1 {
			backoffDuration := time.Millisecond * time.Duration(500*(1<<attempt)) // Base increased to 500ms
			log.Printf("Retrying in %v...", backoffDuration)
			time.Sleep(backoffDuration)
		}
	}

	return nil, fmt.Errorf("failed to query subscriber after %d attempts: %v", maxRetries, err)
}

// GetSubscriberByNpub retrieves subscriber information by Npub
func (store *GormStatisticsStore) GetSubscriberByNpub(npub string) (*types.SubscriberAddress, error) {
	// Use address-specific mutex for reading subscribers
	store.addressMutex.RLock()
	defer store.addressMutex.RUnlock()

	// Maximum retries for database operations
	const maxRetries = 6

	var subscriber types.SubscriberAddress
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Use timeout context
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err = store.DB.WithContext(ctx).Where("npub = ?", npub).First(&subscriber).Error

		if err == nil {
			return &subscriber, nil
		}

		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("no subscriber found for npub: %s", npub)
		}

		log.Printf("Attempt %d: Error querying subscriber by npub: %v", attempt+1, err)

		// Exponential backoff for retries
		if attempt < maxRetries-1 {
			backoffDuration := time.Millisecond * time.Duration(500*(1<<attempt))
			log.Printf("Retrying in %v...", backoffDuration)
			time.Sleep(backoffDuration)
		}
	}

	return nil, fmt.Errorf("failed to query subscriber after %d attempts: %v", maxRetries, err)
}

// GetSubscriberCredit retrieves the credit amount for a subscriber by npub
func (store *GormStatisticsStore) GetSubscriberCredit(npub string) (int64, error) {
	subscriber, err := store.GetSubscriberByNpub(npub)
	if err != nil {
		return 0, err
	}
	return subscriber.CreditSats, nil
}

// UpdateSubscriberCredit updates the credit amount for a subscriber
func (store *GormStatisticsStore) UpdateSubscriberCredit(npub string, creditSats int64) error {
	// Use address-specific mutex for updating subscriber credit
	store.addressMutex.Lock()
	defer store.addressMutex.Unlock()

	// Maximum retries for database operations
	const maxRetries = 6

	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Use timeout context
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err = store.DB.WithContext(ctx).Model(&types.SubscriberAddress{}).
			Where("npub = ?", npub).
			Update("credit_sats", creditSats).Error

		if err == nil {
			log.Printf("Updated credit for npub %s to %d sats", npub, creditSats)
			return nil
		}

		log.Printf("Attempt %d: Error updating subscriber credit: %v", attempt+1, err)

		// Exponential backoff for retries
		if attempt < maxRetries-1 {
			backoffDuration := time.Millisecond * time.Duration(500*(1<<attempt))
			log.Printf("Retrying in %v...", backoffDuration)
			time.Sleep(backoffDuration)
		}
	}

	return fmt.Errorf("failed to update subscriber credit after %d attempts: %v", maxRetries, err)
}

// SaveSubscriberAddress saves or updates a subscriber address in the database
func (store *GormStatisticsStore) SaveSubscriberAddress(address *types.SubscriberAddress) error {
	return store.SaveSubscriberAddressBatch([]*types.SubscriberAddress{address})
}

// GetPaidSubscribers retrieves all paid subscribers from the database
func (store *GormStatisticsStore) GetPaidSubscribers() ([]types.PaidSubscriber, error) {
	var subscribers []types.PaidSubscriber

	// Query all paid subscribers
	if err := store.DB.Find(&subscribers).Error; err != nil {
		log.Printf("Error fetching paid subscribers: %v", err)
		return nil, err
	}

	// Filter out expired subscriptions
	var activeSubscribers []types.PaidSubscriber
	now := time.Now()

	for _, sub := range subscribers {
		if sub.ExpirationDate.After(now) {
			activeSubscribers = append(activeSubscribers, sub)
		}
	}

	return activeSubscribers, nil
}

// GetPaidSubscriberByNpub retrieves a specific paid subscriber by their npub
func (store *GormStatisticsStore) GetPaidSubscriberByNpub(npub string) (*types.PaidSubscriber, error) {
	var subscriber types.PaidSubscriber

	if err := store.DB.Where("npub = ?", npub).First(&subscriber).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // Subscriber not found
		}
		return nil, err
	}

	return &subscriber, nil
}

// SavePaidSubscriber creates a new paid subscriber record
func (store *GormStatisticsStore) SavePaidSubscriber(subscriber *types.PaidSubscriber) error {
	return store.DB.Create(subscriber).Error
}

// UpdatePaidSubscriber updates an existing paid subscriber record
func (store *GormStatisticsStore) UpdatePaidSubscriber(subscriber *types.PaidSubscriber) error {
	return store.DB.Save(subscriber).Error
}

// DeletePaidSubscriber deletes a paid subscriber record by npub
func (store *GormStatisticsStore) DeletePaidSubscriber(npub string) error {
	return store.DB.Where("npub = ?", npub).Delete(&types.PaidSubscriber{}).Error
}

// NPUB access control management implementation

// IsNpubInAllowedReadList checks if an NPUB is in the allowed read list
func (store *GormStatisticsStore) IsNpubInAllowedReadList(npub string) (bool, error) {
	var count int64
	err := store.DB.Model(&types.AllowedReadNpub{}).Where("npub = ?", npub).Count(&count).Error
	return count > 0, err
}

// IsNpubInAllowedWriteList checks if an NPUB is in the allowed write list
func (store *GormStatisticsStore) IsNpubInAllowedWriteList(npub string) (bool, error) {
	var count int64
	err := store.DB.Model(&types.AllowedWriteNpub{}).Where("npub = ?", npub).Count(&count).Error
	return count > 0, err
}

// AddNpubToReadList adds an NPUB to the allowed read list
func (store *GormStatisticsStore) AddNpubToReadList(npub, tierName, addedBy string) error {
	allowedNpub := types.AllowedReadNpub{
		Npub:     npub,
		TierName: tierName,
		AddedBy:  addedBy,
	}
	return store.DB.Create(&allowedNpub).Error
}

// AddNpubToWriteList adds an NPUB to the allowed write list and updates the corresponding kind 888 event
func (store *GormStatisticsStore) AddNpubToWriteList(npub, tierName, addedBy string) error {
	log.Printf("[DEBUG] AddNpubToWriteList called with npub='%s', tierName='%s', addedBy='%s'", npub, tierName, addedBy)
	
	// First, add to the database
	allowedNpub := types.AllowedWriteNpub{
		Npub:     npub,
		TierName: tierName,
		AddedBy:  addedBy,
	}
	if err := store.DB.Create(&allowedNpub).Error; err != nil {
		return err
	}

	// Then, update the kind 888 event to reflect the new tier assignment
	subManager := subscription.GetGlobalManager()
	if subManager != nil {
		if err := subManager.UpdateNpubSubscriptionEvent(npub, tierName); err != nil {
			log.Printf("Warning: Failed to update kind 888 event for npub %s: %v", npub, err)
			// Don't return error since the database update succeeded
			// The kind 888 event can be updated later via batch update
		}
	} else {
		log.Printf("Warning: Global subscription manager not available, kind 888 event for npub %s will not be updated", npub)
	}

	return nil
}

// RemoveNpubFromReadList removes an NPUB from the allowed read list
func (store *GormStatisticsStore) RemoveNpubFromReadList(npub string) error {
	return store.DB.Where("npub = ?", npub).Delete(&types.AllowedReadNpub{}).Error
}

// RemoveNpubFromWriteList removes an NPUB from the allowed write list
func (store *GormStatisticsStore) RemoveNpubFromWriteList(npub string) error {
	return store.DB.Where("npub = ?", npub).Delete(&types.AllowedWriteNpub{}).Error
}

// GetAllowedReadNpubs retrieves paginated allowed read NPUBs
func (store *GormStatisticsStore) GetAllowedReadNpubs(page, pageSize int) ([]types.AllowedReadNpub, *types.PaginationMetadata, error) {
	var total int64
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}

	offset := (page - 1) * pageSize

	// Count total records
	if err := store.DB.Model(&types.AllowedReadNpub{}).Count(&total).Error; err != nil {
		return nil, nil, err
	}

	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))

	// Get paginated records
	var npubs []types.AllowedReadNpub
	if err := store.DB.Order("added_at DESC").Limit(pageSize).Offset(offset).Find(&npubs).Error; err != nil {
		return nil, nil, err
	}

	metadata := &types.PaginationMetadata{
		CurrentPage: page,
		PageSize:    pageSize,
		TotalItems:  total,
		TotalPages:  totalPages,
		HasNext:     page < totalPages,
		HasPrevious: page > 1,
	}

	return npubs, metadata, nil
}

// GetAllowedWriteNpubs retrieves paginated allowed write NPUBs
func (store *GormStatisticsStore) GetAllowedWriteNpubs(page, pageSize int) ([]types.AllowedWriteNpub, *types.PaginationMetadata, error) {
	var total int64
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}

	offset := (page - 1) * pageSize

	// Count total records
	if err := store.DB.Model(&types.AllowedWriteNpub{}).Count(&total).Error; err != nil {
		return nil, nil, err
	}

	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))

	// Get paginated records
	var npubs []types.AllowedWriteNpub
	if err := store.DB.Order("added_at DESC").Limit(pageSize).Offset(offset).Find(&npubs).Error; err != nil {
		return nil, nil, err
	}

	metadata := &types.PaginationMetadata{
		CurrentPage: page,
		PageSize:    pageSize,
		TotalItems:  total,
		TotalPages:  totalPages,
		HasNext:     page < totalPages,
		HasPrevious: page > 1,
	}

	return npubs, metadata, nil
}

// GetNpubTierFromReadList gets the tier for an NPUB from the read list
func (store *GormStatisticsStore) GetNpubTierFromReadList(npub string) (string, error) {
	var allowedNpub types.AllowedReadNpub
	err := store.DB.Where("npub = ?", npub).First(&allowedNpub).Error
	if err != nil {
		return "", err
	}
	return allowedNpub.TierName, nil
}

// GetNpubTierFromWriteList gets the tier for an NPUB from the write list
func (store *GormStatisticsStore) GetNpubTierFromWriteList(npub string) (string, error) {
	var allowedNpub types.AllowedWriteNpub
	err := store.DB.Where("npub = ?", npub).First(&allowedNpub).Error
	if err != nil {
		return "", err
	}
	return allowedNpub.TierName, nil
}

// BulkAddNpubsToReadList adds multiple NPUBs to the read list in a batch
func (store *GormStatisticsStore) BulkAddNpubsToReadList(npubs []types.AllowedReadNpub) error {
	return store.DB.Transaction(func(tx *gorm.DB) error {
		for _, npub := range npubs {
			if err := tx.Create(&npub).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// BulkAddNpubsToWriteList adds multiple NPUBs to the write list in a batch
func (store *GormStatisticsStore) BulkAddNpubsToWriteList(npubs []types.AllowedWriteNpub) error {
	return store.DB.Transaction(func(tx *gorm.DB) error {
		for _, npub := range npubs {
			if err := tx.Create(&npub).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
