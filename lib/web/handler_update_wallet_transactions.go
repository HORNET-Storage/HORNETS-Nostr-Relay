package web

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/subscription"
)

// updateWalletTransactions processes incoming wallet transactions
func updateWalletTransactions(c *fiber.Ctx, store stores.Store) error {
	var transactions []map[string]interface{}
	log.Println("Transactions request received")

	// Parse the JSON body into the slice of maps
	if err := c.BodyParser(&transactions); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	// Validate wallet name
	expectedWalletName := validateWalletName(transactions)
	if expectedWalletName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Wallet name missing or invalid",
		})
	}

	// Initialize subscription manager
	subManager, err := initializeSubscriptionManager(store)
	if err != nil {
		log.Printf("Failed to initialize subscription manager: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to initialize subscription system",
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
			err := processTransaction(store, subManager, tx)
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
			log.Printf("Error processing transaction: %v", err)
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
func processTransaction(store stores.Store, subManager *subscription.SubscriptionManager, transaction map[string]interface{}) error {
	// Extract transaction details
	txDetails, err := extractTransactionDetails(transaction)
	if err != nil {
		return fmt.Errorf("failed to extract transaction details: %v", err)
	}

	// Process pending transaction
	txID := strings.Split(txDetails.address, ":")[0]
	if err := store.GetStatsStore().DeletePendingTransaction(txID); err != nil {
		log.Printf("Warning: could not delete pending transaction: %v", err)
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
		log.Printf("Error: subscriber not found for address %s: %v", txDetails.output, err)
		return fmt.Errorf("subscriber not found for address %s: %v", txDetails.output, err)
	} else {
		log.Printf("Subscriber retrieved: %v", subscriber)
	}

	// Convert BTC value to satoshis for subscription processing
	satoshis := int64(txDetails.value * 100_000_000)

	// Process the subscription payment
	if err := subManager.ProcessPayment(*subscriber.Npub, txID, satoshis); err != nil {
		return fmt.Errorf("failed to process subscription: %v", err)
	}

	log.Printf("Successfully processed subscription payment for %s: %s sats",
		*subscriber.Npub, txDetails.valueStr)
	return nil
}

// transactionDetails holds parsed transaction information
type transactionDetails struct {
	address  string
	date     time.Time
	output   string
	value    float64
	valueStr string
}

// extractTransactionDetails parses and validates transaction data
func extractTransactionDetails(transaction map[string]interface{}) (*transactionDetails, error) {
	address, ok := transaction["address"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid address")
	}

	dateStr, ok := transaction["date"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid date")
	}
	date, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		return nil, fmt.Errorf("error parsing date: %v", err)
	}

	output, ok := transaction["output"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid output")
	}

	valueStr, ok := transaction["value"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid value")
	}
	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return nil, fmt.Errorf("error parsing value: %v", err)
	}

	return &transactionDetails{
		address:  address,
		date:     date,
		output:   output,
		value:    value,
		valueStr: valueStr,
	}, nil
}

// validateWalletName ensures the wallet name is valid and consistent
func validateWalletName(transactions []map[string]interface{}) string {
	expectedWalletName := viper.GetString("wallet_name")

	// Set wallet name from first transaction if not set
	if expectedWalletName == "" && len(transactions) > 0 {
		if walletName, ok := transactions[0]["wallet_name"].(string); ok {
			viper.Set("wallet_name", walletName)
			expectedWalletName = walletName
		}
	}

	return expectedWalletName
}

// initializeSubscriptionManager creates a new subscription manager instance
func initializeSubscriptionManager(store stores.Store) (*subscription.SubscriptionManager, error) {
	// Load relay private key
	privateKey, _, err := signing.DeserializePrivateKey(viper.GetString("private_key"))
	if err != nil {
		return nil, fmt.Errorf("failed to load relay private key: %v", err)
	}

	// Get subscription tiers from config
	var subscriptionTiers []types.SubscriptionTier
	if err := viper.UnmarshalKey("subscription_tiers", &subscriptionTiers); err != nil {
		return nil, fmt.Errorf("failed to load subscription tiers: %v", err)
	}

	// Create and return the subscription manager
	return subscription.NewSubscriptionManager(
		store,
		privateKey,
		viper.GetString("RelayDHTkey"),
		subscriptionTiers,
	), nil
}
