package gorm

import (
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// GormStatisticsStore is a GORM-based implementation of the StatisticsStore interface.
type GormStatisticsStore struct {
	DB *gorm.DB
}

// InitStore initializes the GORM DB (can be swapped for another DB).
func (store *GormStatisticsStore) InitStore(basepath string, args ...interface{}) error {
	var err error
	store.DB, err = gorm.Open(sqlite.Open(basepath), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}

	// Auto migrate the schema
	err = store.DB.AutoMigrate(
		&types.Kind{},
		&types.FileInfo{},
		&types.UserProfile{},
		&types.User{},
		&types.WalletBalance{},
		&types.WalletTransactions{},
		&types.BitcoinRate{},
		&types.WalletAddress{},
		&types.UserChallenge{},
		&types.PendingTransaction{},
		&types.ActiveToken{},
	)
	if err != nil {
		return fmt.Errorf("failed to migrate database schema: %v", err)
	}

	return nil
}

// SaveBitcoinRate checks if the rate has changed and updates it in the database
func (store *GormStatisticsStore) SaveBitcoinRate(rate float64) error {
	// Query the latest Bitcoin rate
	var latestBitcoinRate types.BitcoinRate
	result := store.DB.Order("timestamp desc").First(&latestBitcoinRate)

	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		log.Printf("Error querying bitcoin rate: %v", result.Error)
		return result.Error
	}

	// If the rate is the same as the latest entry, no update needed
	if result.Error == nil && latestBitcoinRate.Rate == rate {
		log.Println("Rate is the same as the latest entry, no update needed")
		return nil
	}

	// Add the new rate
	newRate := types.BitcoinRate{
		Rate:      rate,
		Timestamp: time.Now(),
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
	result := store.DB.Where("timestamp >= ?", thirtyDaysAgo).Order("timestamp asc").Find(&bitcoinRates)

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
				NpubKey:       npubKey,
				LightningAddr: lightningAddr,
				DHTKey:        dhtKey,
				Timestamp:     createdAt,
			}
			return store.DB.Create(&userProfile).Error
		}
		return result.Error
	}

	// Update existing user profile
	userProfile.LightningAddr = lightningAddr
	userProfile.DHTKey = dhtKey
	userProfile.Timestamp = createdAt
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
		SELECT timestamp, size
		FROM kinds
		WHERE kind_number = ? AND timestamp >= date('now', '-12 months')
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
		month := row.Timestamp.Format("2006-01")
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
func (store *GormStatisticsStore) FindUserByNpub(npub string) (*types.User, error) {
	var user types.User
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
func (store *GormStatisticsStore) DeleteActiveToken(token string) error {
	result := store.DB.Where("token = ?", token).Delete(&types.ActiveToken{})
	if result.Error != nil {
		log.Printf("Failed to delete token: %v", result.Error)
		return result.Error
	}

	if result.RowsAffected == 0 {
		// Token wasn't found, but we'll still consider this a successful logout
		log.Printf("Token not found in ActiveTokens, but proceeding with logout")
	}

	return nil
}

// FetchMonthlyStorageStats retrieves the monthly storage stats (total GBs per month)
func (store *GormStatisticsStore) FetchMonthlyStorageStats() ([]types.ActivityData, error) {
	var data []types.ActivityData

	// Query to get the total GBs per month across different tables
	err := store.DB.Raw(`
		SELECT 
			strftime('%Y-%m', timestamp) as month,
			ROUND(SUM(size) / 1024.0, 3) as total_gb
		FROM (
			SELECT timestamp, size FROM kinds
			UNION ALL
			SELECT timestamp, size FROM file_info
		)
		GROUP BY month
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

	// Query to get the total GBs per month for notes and media
	err := store.DB.Raw(`
		SELECT 
			strftime('%Y-%m', timestamp) as month,
			ROUND(SUM(CASE WHEN kind_number IS NOT NULL THEN size ELSE 0 END) / 1024.0, 3) as notes_gb,  -- Convert to GB and round to 2 decimal places
			ROUND(SUM(CASE WHEN kind_number IS NULL THEN size ELSE 0 END) / 1024.0, 3) as media_gb  -- Convert to GB and round to 2 decimal places
		FROM (
			SELECT timestamp, size, kind_number FROM kinds
			UNION ALL
			SELECT timestamp, size, NULL as kind_number FROM file_info
		)
		GROUP BY month
	`).Scan(&data).Error

	if err != nil {
		log.Printf("Error fetching bar chart data: %v", err)
		return nil, err
	}

	return data, nil
}

// FetchProfilesTimeSeriesData retrieves the time series data for profiles over the last 6 months
func (store *GormStatisticsStore) FetchProfilesTimeSeriesData(startDate, endDate string) ([]types.TimeSeriesData, error) {
	var data []types.TimeSeriesData

	// Query to get profile data from the last 6 months
	err := store.DB.Raw(`
        SELECT
			strftime('%Y-%m', timestamp) as month,
			COUNT(*) as profiles,
			COUNT(CASE WHEN lightning_addr THEN 1 ELSE NULL END) as lightning_addr,
			COUNT(CASE WHEN dht_key THEN 1 ELSE NULL END) as dht_key,
			COUNT(CASE WHEN lightning_addr AND dht_key THEN 1 ELSE NULL END) as lightning_and_dht
		FROM user_profiles
		WHERE strftime('%Y-%m', timestamp) >= ? AND strftime('%Y-%m', timestamp) < ?
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
		Order("timestamp DESC").
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
		Timestamp:        time.Now(),
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
	pendingTransaction.Timestamp = time.Now()

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
	user := types.User{
		Password: string(hashedPassword),
		Npub:     npub,
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
	result := store.DB.Order("timestamp desc").Find(&pendingTransactions)
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
	result := store.DB.Order("timestamp desc").First(&latestBitcoinRate)

	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		log.Printf("Error querying bitcoin rate: %v", result.Error)
		return result.Error
	}

	if result.Error == nil && latestBitcoinRate.Rate == rate {
		// If the rate is the same as the latest entry, no update needed
		log.Println("Rate is the same as the latest entry, no update needed")
		return nil
	}

	// Add the new rate
	newRate := types.BitcoinRate{
		Rate:      rate,
		Timestamp: time.Now(),
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
	result := store.DB.Order("timestamp desc").First(&latestBalance)

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
		Balance:   balance,
		Timestamp: time.Now(),
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
	err := store.DB.Model(&types.User{}).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// AddressExists checks if a wallet address already exists in the database
func (store *GormStatisticsStore) AddressExists(address string) (bool, error) {
	var existingAddress types.WalletAddress
	result := store.DB.Where("address = ?", address).First(&existingAddress)
	if result.Error == nil {
		return true, nil
	}
	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		return false, result.Error
	}
	return false, nil
}

// SaveAddress saves a new wallet address to the database
func (store *GormStatisticsStore) SaveAddress(address *types.WalletAddress) error {
	return store.DB.Create(address).Error
}

// GetLatestWalletBalance retrieves the latest wallet balance from the database
func (store *GormStatisticsStore) GetLatestWalletBalance() (types.WalletBalance, error) {
	var latestBalance types.WalletBalance
	result := store.DB.Order("timestamp desc").First(&latestBalance)

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
	result := store.DB.Order("timestamp desc").First(&bitcoinRate)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			bitcoinRate.Rate = 0.0 // Default rate if not found
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
func (store *GormStatisticsStore) GetUserByID(userID uint) (types.User, error) {
	var user types.User
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
