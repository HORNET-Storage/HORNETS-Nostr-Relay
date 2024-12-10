package gorm

import (
	"fmt"
	"log"
	"math"
	"sort"
	"sync"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BatchSize defines how many records to insert in a single transaction
const BatchSize = 50

// GormStatisticsStore is a GORM-based implementation of the StatisticsStore interface.
type GormStatisticsStore struct {
	DB    *gorm.DB
	mutex sync.RWMutex
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
	)
	if err != nil {
		return fmt.Errorf("failed to migrate database schema: %v", err)
	}

	var result map[string]interface{}
	store.DB.Raw("SELECT column_name, data_type FROM information_schema.columns WHERE table_name = 'bitcoin_rates'").Scan(&result)
	log.Printf("Bitcoin rates table schema: %+v", result)

	// Create indexes after table creation
	err = store.DB.Exec("CREATE INDEX ON active_tokens (user_id)").Error
	if err != nil {
		log.Printf("Warning: failed to create user_id index: %v", err)
	}

	err = store.DB.Exec("CREATE UNIQUE INDEX ON active_tokens (token)").Error
	if err != nil {
		log.Printf("Warning: failed to create token index: %v", err)
	}

	return nil
}

func (store *GormStatisticsStore) AllocateBitcoinAddress(npub string) (*types.Address, error) {
	tx := store.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Step 1: Check if the npub already has an allocated address
	var existingAddressRecord types.SubscriberAddress
	err := tx.Where("npub = ?", npub).First(&existingAddressRecord).Error
	if err == nil {
		// If an existing record is found, return it
		return &types.Address{
			IndexHornets: existingAddressRecord.IndexHornets,
			Address:      existingAddressRecord.Address,
			WalletName:   existingAddressRecord.WalletName,
			Status:       existingAddressRecord.Status,
			AllocatedAt:  existingAddressRecord.AllocatedAt,
			Npub:         npub,
		}, nil
	} else if err != gorm.ErrRecordNotFound {
		// If another error occurred (not record not found), rollback and return error
		tx.Rollback()
		return nil, fmt.Errorf("failed to query existing address for npub: %v", err)
	}

	// Step 2: Allocate a new address if no existing address is found
	var addressRecord types.SubscriberAddress
	err = tx.Where("status = ? AND (npub IS NULL OR npub = '')", AddressStatusAvailable).
		Order("id").
		First(&addressRecord).Error

	if err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("no available addresses")
		}
		return nil, fmt.Errorf("failed to query available addresses: %v", err)
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":       AddressStatusAllocated,
		"allocated_at": &now,
		"npub":         npub,
	}

	if err := tx.Model(&addressRecord).Updates(updates).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update address allocation: %v", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &types.Address{
		IndexHornets: addressRecord.IndexHornets,
		Address:      addressRecord.Address,
		WalletName:   addressRecord.WalletName,
		Status:       addressRecord.Status,
		AllocatedAt:  addressRecord.AllocatedAt,
		Npub:         npub,
	}, nil
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

// GetBitcoinRatesLast30Days retrieves Bitcoin rates for the last 30 days
func (store *GormStatisticsStore) GetBitcoinRatesLast30Days() ([]types.BitcoinRate, error) {
	// Calculate the date 30 days ago
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)

	// Query the Bitcoin rates for the last 30 days
	var bitcoinRates []types.BitcoinRate
	result := store.DB.Where("timestamp_hornets >= ?", thirtyDaysAgo).Order("timestamp_hornets asc").Find(&bitcoinRates)

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
	kindStr := fmt.Sprintf("kind%d", event.Kind)

	var relaySettings types.RelaySettings
	if err := viper.UnmarshalKey("relay_settings", &relaySettings); err != nil {
		log.Printf("Error unmarshaling relay settings: %v", err)
		return err
	}

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

		err := store.UpsertUserProfile(npubKey, lightningAddr, dhtKey, time.Unix(int64(event.CreatedAt), 0))
		if err != nil {
			log.Printf("Error upserting user profile: %v", err)
			return err
		}
	}

	// If the event kind matches relay settings, store it in the database
	if contains(relaySettings.KindWhitelist, kindStr) {
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
		if err := store.DB.Create(&kind).Error; err != nil {
			return err
		}
	}

	return nil
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

// SaveFile saves the file (photo, video, audio, or misc) based on its type and processing mode (smart or unlimited).
func (store *GormStatisticsStore) SaveFile(root string, hash string, fileName string, mimeType string, leafCount int, size int64) error {
	file := types.FileInfo{
		Root:      root,
		Hash:      hash,
		FileName:  fileName,
		MimeType:  mimeType,
		LeafCount: leafCount,
		Size:      size,
	}
	return store.DB.Create(&file).Error
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
	if err := store.DB.Create(userChallenge).Error; err != nil {
		log.Printf("Failed to save user challenge: %v", err)
		return err
	}
	return nil
}

// DeleteActiveToken deletes the given token from the ActiveTokens table
func (store *GormStatisticsStore) DeleteActiveToken(userID uint) error {
	result := store.DB.Where("user_id = ?", userID).Delete(&types.ActiveToken{})
	if result.Error != nil {
		log.Printf("Failed to delete tokens for user %d: %v", userID, result.Error)
		return result.Error
	}

	if result.RowsAffected == 0 {
		// No tokens found for this user, but we'll still consider this successful
		log.Printf("No tokens found for user %d, but proceeding with cleanup", userID)
	} else {
		log.Printf("Successfully deleted %d tokens for user %d", result.RowsAffected, userID)
	}

	return nil
}

func (store *GormStatisticsStore) FindUserByToken(token string) (*types.AdminUser, error) {
	var activeToken types.ActiveToken
	if err := store.DB.Where("token = ? AND expires_at > NOW()", token).First(&activeToken).Error; err != nil {
		return nil, err
	}

	var user types.AdminUser
	if err := store.DB.First(&user, activeToken.UserID).Error; err != nil {
		return nil, err
	}

	return &user, nil
}

// FetchMonthlyStorageStats retrieves the monthly storage stats (total GBs per month)
func (store *GormStatisticsStore) FetchMonthlyStorageStats() ([]types.ActivityData, error) {
	var data []types.ActivityData

	// Simpler query without UNION and strftime
	err := store.DB.Raw(`
        SELECT 
            timestamp_hornets as month,
            ROUND(SUM(size) / 1024.0, 3) as total_gb
        FROM kinds
        GROUP BY timestamp_hornets
    `).Scan(&data).Error

	if err != nil {
		log.Printf("Error fetching monthly storage stats: %v", err)
		return nil, err
	}

	return data, nil
}

// FetchNotesMediaStorageData retrieves the total GBs per month for notes and media
func (store *GormStatisticsStore) FetchNotesMediaStorageData() ([]types.BarChartData, error) {
	var data []types.BarChartData

	// Simplified query using ImmuDB compatible syntax
	err := store.DB.Raw(`
        SELECT 
            timestamp_hornets as month,
            ROUND((SELECT SUM(size) FROM kinds WHERE kind_number IS NOT NULL) / 1024.0, 3) as notes_gb,
            ROUND((SELECT SUM(size) FROM file_info) / 1024.0, 3) as media_gb
        FROM kinds k
        GROUP BY timestamp_hornets
    `).Scan(&data).Error

	if err != nil {
		log.Printf("Error fetching bar chart data: %v", err)
		return nil, err
	}

	// Post-process the timestamps into month format if needed
	for i := range data {
		if t, err := time.Parse(time.RFC3339, data[i].Month); err == nil {
			data[i].Month = t.Format("2006-01")
		}
	}

	return data, nil
}

// FetchProfilesTimeSeriesData retrieves the time series data for profiles over the last 6 months
func (store *GormStatisticsStore) FetchProfilesTimeSeriesData(startDate, endDate string) ([]types.TimeSeriesData, error) {
	var data []types.TimeSeriesData

	// Query to get profile data from the last 6 months
	err := store.DB.Raw(`
        SELECT
			strftime('%Y-%m', timestamp_hornets) as month,
			COUNT(*) as profiles,
			COUNT(CASE WHEN lightning_addr THEN 1 ELSE NULL END) as lightning_addr,
			COUNT(CASE WHEN dht_key THEN 1 ELSE NULL END) as dht_key,
			COUNT(CASE WHEN lightning_addr AND dht_key THEN 1 ELSE NULL END) as lightning_and_dht
		FROM user_profiles
		WHERE strftime('%Y-%m', timestamp_hornets) >= ? AND strftime('%Y-%m', timestamp_hornets) <= ?
		GROUP BY month
		ORDER BY month ASC;
    `, startDate, endDate).Scan(&data).Error

	if err != nil {
		log.Printf("Error fetching time series data: %v", err)
		return nil, err
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
	// Query the latest Bitcoin rate
	var latestBitcoinRate types.BitcoinRate
	result := store.DB.Order("timestamp_hornets desc").First(&latestBitcoinRate)

	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		log.Printf("Error querying bitcoin rate: %v", result.Error)
		return result.Error
	}

	// Convert current rate to string for comparison
	rateStr := fmt.Sprintf("%.8f", rate)

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
	if err := store.DB.Create(&newRate).Error; err != nil {
		log.Printf("Error saving new rate: %v", err)
		return err
	}

	log.Println("Bitcoin rate updated successfully")
	return nil
}

// UpdateWalletBalance updates the wallet balance if it's new
func (store *GormStatisticsStore) UpdateWalletBalance(walletName, balance string) error {
	// Query the latest wallet balance
	var latestBalance types.WalletBalance
	result := store.DB.Order("timestamp_hornets desc").First(&latestBalance)

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

	if err := store.DB.Create(&newBalance).Error; err != nil {
		log.Printf("Error saving new balance: %v", err)
		return err
	}

	log.Println("Wallet balance updated successfully")
	return nil
}

// SaveWalletTransaction saves a new wallet transaction to the database
func (store *GormStatisticsStore) SaveWalletTransaction(tx types.WalletTransactions) error {
	return store.DB.Create(&tx).Error
}

// DeletePendingTransaction deletes a pending transaction from the database by TxID
func (store *GormStatisticsStore) DeletePendingTransaction(txID string) error {
	var pendingTx types.PendingTransaction
	result := store.DB.Where("tx_id = ?", txID).First(&pendingTx)
	if result.Error == nil {
		return store.DB.Delete(&pendingTx).Error
	}
	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		log.Printf("Error querying pending transaction with TxID %s: %v", txID, result.Error)
		return result.Error
	}
	log.Printf("No pending transaction found with TxID %s", txID)
	return nil
}

// ExistingTransactionExists checks if a transaction already exists in the database
func (store *GormStatisticsStore) TransactionExists(address string, date time.Time, output string, value string) (bool, error) {
	var existingTransaction types.WalletTransactions
	result := store.DB.Where("address = ? AND date = ? AND output = ? AND value = ?", address, date, output, value).First(&existingTransaction)

	// Handle "Record Not Found" case without raising an error
	if result.Error == gorm.ErrRecordNotFound {
		return false, nil // No transaction exists, but it's not an error
	}

	// If there’s another error, return it
	if result.Error != nil {
		return false, result.Error
	}

	// If no error, the transaction exists
	return true, nil
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

	const maxRetries = 3

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

	const maxRetries = 3

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

	return store.DB.Create(activeToken).Error
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
	// Query the active tokens table for a matching token that has not expired
	if err := store.DB.Where("token = ? AND expires_at > ?", token, time.Now()).First(&activeToken).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GetSubscriberByAddress retrieves subscriber information by Bitcoin address
func (store *GormStatisticsStore) GetSubscriberByAddress(address string) (*types.SubscriberAddress, error) {
	var subscriber types.SubscriberAddress

	err := store.DB.Where("address = ?", address).First(&subscriber).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("no subscriber found for address: %s", address)
		}
		return nil, fmt.Errorf("failed to query subscriber: %v", err)
	}

	return &subscriber, nil
}

// SaveSubscriberAddress saves or updates a subscriber address in the database
func (store *GormStatisticsStore) SaveSubscriberAddress(address *types.SubscriberAddress) error {
	return store.SaveSubscriberAddressBatch([]*types.SubscriberAddress{address})
}
