package wallet

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/subscription"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

// transactionDetails holds parsed transaction information
type transactionDetails struct {
	address  string
	date     time.Time
	output   string
	value    float64
	valueStr string
}

// Refactored getPendingTransactions function
func GetPendingTransactions(c *fiber.Ctx, store stores.Store) error {
	logging.Info("Request for unconfirmed transactions.")

	// Use the statistics store to retrieve pending transactions
	pendingTransactions, err := store.GetStatsStore().GetPendingTransactions()
	if err != nil {
		logging.Infof("Error querying pending transactions: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database query error",
		})
	}

	// Respond with the pending transactions
	return c.JSON(pendingTransactions)
}

// Refactored saveUnconfirmedTransaction function
func SaveUnconfirmedTransaction(c *fiber.Ctx, store stores.Store) error {
	var pendingTransaction types.PendingTransaction
	logging.Info("Saving unconfirmed transaction.")

	// Parse the JSON body into the struct with field mappings
	if err := c.BodyParser(&pendingTransaction); err != nil {
		logging.Infof("Failed to parse JSON: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	// Use the statistics store to save the pending transaction
	err := store.GetStatsStore().SaveUnconfirmedTransaction(&pendingTransaction)
	if err != nil {
		logging.Infof("Error saving pending transaction: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database save error",
		})
	}

	// Respond with success message
	return c.JSON(fiber.Map{
		"message": "Pending transaction saved successfully",
	})
}

// Refactored replaceTransaction function
func ReplaceTransaction(c *fiber.Ctx, store stores.Store) error {
	// Parse the JSON body into the ReplaceTransactionRequest struct
	var replaceRequest types.ReplaceTransactionRequest
	if err := c.BodyParser(&replaceRequest); err != nil {
		logging.Infof("Failed to parse replacement request: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	// Use the statistics store to replace the transaction
	err := store.GetStatsStore().ReplaceTransaction(replaceRequest)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Original transaction not found",
			})
		}
		logging.Infof("Error replacing transaction: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Error replacing transaction",
		})
	}

	// Respond with success
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Replacement transaction saved successfully",
		"txid":    replaceRequest.NewTxID,
	})
}

func GetLatestWalletTransactions(c *fiber.Ctx, store stores.Store) error {
	// Get the latest wallet transactions
	transactions, err := store.GetStatsStore().GetLatestWalletTransactions()
	if err != nil {
		logging.Infof("Error querying transactions: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database query error",
		})
	}

	// Process each transaction to convert the value to USD
	for i, transaction := range transactions {
		value, err := strconv.ParseFloat(transaction.Value, 64)
		if err != nil {
			logging.Infof("Error converting value to float64: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Conversion error",
			})
		}
		// You can adjust the format as needed, currently keeping the value as satoshis
		transactions[i].Value = fmt.Sprintf("%.8f", value)
	}

	// Respond with the transactions
	return c.JSON(transactions)
}

// updateWalletTransactions processes incoming wallet transactions
func UpdateWalletTransactions(c *fiber.Ctx, store stores.Store) error {
	var transactions []map[string]interface{}
	logging.Info("Transactions request received")

	// Parse the JSON body into the slice of maps
	if err := c.BodyParser(&transactions); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	// Validate wallet name
	expectedWalletName := ValidateWalletName(transactions)
	if expectedWalletName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Wallet name missing or invalid",
		})
	}

	// Get the global subscription manager
	subManager := subscription.GetGlobalManager()
	if subManager == nil {
		logging.Infof("Global subscription manager not initialized")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Subscription system not available",
		})
	}

	// Filter valid transactions
	validTransactions := make([]map[string]interface{}, 0)
	for _, tx := range transactions {
		walletName, ok := tx["wallet_name"].(string)
		if ok && walletName == expectedWalletName {
			validTransactions = append(validTransactions, tx)
		}
	}

	// Process transactions concurrently using a worker pool
	const numWorkers = 5
	jobs := make(chan map[string]interface{}, len(validTransactions))
	results := make(chan error, len(validTransactions))
	var wg sync.WaitGroup

	// Worker function to process transactions
	worker := func(jobs <-chan map[string]interface{}, results chan<- error) {
		for tx := range jobs {
			err := ProcessWalletTransaction(store, subManager, tx)
			results <- err
		}
		wg.Done()
	}

	// Start worker pool
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go worker(jobs, results)
	}

	// Enqueue jobs
	for _, tx := range validTransactions {
		jobs <- tx
	}
	close(jobs)

	// Wait for all workers to finish
	wg.Wait()
	close(results)

	// Collect results
	var processedCount, errorCount int
	for err := range results {
		if err != nil {
			logging.Infof("Error processing transaction: %v", err)
			errorCount++
		} else {
			processedCount++
		}
	}

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": fmt.Sprintf("Processed %d transactions successfully, %d errors", processedCount, errorCount),
	})
}

// processTransaction handles an individual transaction
func ProcessWalletTransaction(store stores.Store, subManager *subscription.SubscriptionManager, transaction map[string]interface{}) error {
	// Extract transaction details
	txDetails, err := ExtractTransactionDetails(transaction)
	if err != nil {
		return fmt.Errorf("failed to extract transaction details: %v", err)
	}

	// Process pending transaction
	txID := strings.Split(txDetails.address, ":")[0]
	if err := store.GetStatsStore().DeletePendingTransaction(txID); err != nil {
		logging.Infof("Warning: could not delete pending transaction: %v", err)
	}

	// Check if transaction already exists
	exists, err := store.GetStatsStore().TransactionExists(
		txDetails.address,
		txDetails.date,
		txDetails.output,
		txDetails.valueStr,
	)
	if err != nil {
		return fmt.Errorf("error checking existing transaction: %v", err)
	}
	if exists {
		return fmt.Errorf("transaction already processed")
	}

	// Save transaction record
	newTransaction := types.WalletTransactions{
		Address: txDetails.address,
		Date:    txDetails.date,
		Output:  txDetails.output,
		Value:   fmt.Sprintf("%.8f", txDetails.value),
	}
	if err := store.GetStatsStore().SaveWalletTransaction(newTransaction); err != nil {
		return fmt.Errorf("failed to save transaction: %v", err)
	}

	// After subscriber retrieval in processTransaction
	subscriber, err := store.GetStatsStore().GetSubscriberByAddress(txDetails.output)
	if err != nil {
		logging.Infof("Error: subscriber not found for address %s: %v", txDetails.output, err)
		return fmt.Errorf("subscriber not found for address %s: %v", txDetails.output, err)
	} else {
		logging.Infof("Subscriber retrieved: %v", subscriber)
	}

	// Check if subscriber.Npub is nil before dereferencing
	if subscriber.Npub == nil {
		logging.Infof("Warning: subscriber found for address %s but has nil Npub", txDetails.output)
		return fmt.Errorf("subscriber found but has nil npub for address: %s", txDetails.output)
	}

	// Convert BTC value to satoshis for subscription processing
	satoshis := int64(math.Round(txDetails.value * 100_000_000))

	// Process the subscription payment
	if err := subManager.ProcessPayment(*subscriber.Npub, txID, satoshis); err != nil {
		return fmt.Errorf("failed to process subscription: %v", err)
	}

	// Determine if this is a new subscriber or a renewal
	paidSubscriber, err := store.GetStatsStore().GetPaidSubscriberByNpub(*subscriber.Npub)
	isNewSubscriber := err != nil || paidSubscriber == nil // If error or nil, it's a new subscriber

	// Get the subscription tier
	// We'll extract the tier from paid subscriber data or try to determine from amount
	var tier string
	expirationDate := time.Now().Add(30 * 24 * time.Hour) // Default to 30 days

	if paidSubscriber != nil {
		tier = paidSubscriber.Tier
		expirationDate = paidSubscriber.ExpirationDate
	} else {
		// Try to determine the tier based on the payment amount
		settings, err := config.GetConfig()
		if err != nil {
			return fmt.Errorf("error getting config: %v", err)
		}

		for _, tierInfo := range settings.AllowedUsersSettings.Tiers {
			// Extract price in satoshis
			price := int64(tierInfo.PriceSats)
			// If payment amount is within 10% of the tier price, consider it that tier
			if satoshis >= price*9/10 && satoshis <= price*11/10 {
				tier = tierInfo.Name
				break
			}
		}

		if tier == "" {
			tier = "unknown" // Fallback if we couldn't determine the tier
		}
	}

	// Create payment notification
	notification := &types.PaymentNotification{
		PubKey:           *subscriber.Npub,
		TxID:             txID,
		Amount:           satoshis,
		SubscriptionTier: tier,
		IsNewSubscriber:  isNewSubscriber,
		ExpirationDate:   expirationDate,
		// CreatedAt and IsRead will be set automatically
	}

	if err := store.GetStatsStore().CreatePaymentNotification(notification); err != nil {
		logging.Infof("Warning: failed to create payment notification: %v", err)
		// Continue processing - non-fatal error
	} else {
		logging.Infof("Created payment notification for %s: %d sats, tier: %s",
			*subscriber.Npub, satoshis, tier)
	}

	logging.Infof("Successfully processed subscription payment for %s: %s sats",
		*subscriber.Npub, txDetails.valueStr)
	return nil
}

// initializeSubscriptionManager creates a new subscription manager instance
func InitializeSubscriptionManager(store stores.Store) (*subscription.SubscriptionManager, error) {
	// Load relay private key
	privateKey, _, err := signing.DeserializePrivateKey(viper.GetString("private_key"))
	if err != nil {
		return nil, fmt.Errorf("failed to load relay private key: %v", err)
	}

	settings, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("error getting config: %v", err)
	}

	// Log the tiers for debugging
	logging.Infof("Loading subscription tiers from relay_settings: %+v", settings.AllowedUsersSettings)
	for i, tier := range settings.AllowedUsersSettings.Tiers {
		logging.Infof("Tier %d: Name='%s', MonthlyLimitBytes='%d', Price='%d', Unlimited='%t'",
			i, tier.Name, tier.MonthlyLimitBytes, tier.PriceSats, tier.Unlimited)
	}

	// Create and return the subscription manager
	return subscription.NewSubscriptionManager(
		store,
		privateKey,
		viper.GetString("RelayDHTkey"),
		settings.AllowedUsersSettings.Tiers,
	), nil
}
