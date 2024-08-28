package web

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"gorm.io/gorm"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
)

func handleTransactions(c *fiber.Ctx) error {
	var transactions []map[string]interface{}
	log.Println("Transactions request received")

	// Parse the JSON body into the slice of maps
	if err := c.BodyParser(&transactions); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	// Get the expected wallet name from the configuration
	expectedWalletName := viper.GetString("wallet_name")

	// If the expected wallet name is not set, set it using the first transaction's wallet name
	if expectedWalletName == "" && len(transactions) > 0 {
		firstTransaction := transactions[0]
		walletName, ok := firstTransaction["wallet_name"].(string)
		if !ok {
			log.Println("Wallet name missing or invalid in the first transaction")
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Wallet name missing or invalid",
			})
		}

		// Set the expected wallet name in Viper
		viper.Set("wallet_name", walletName)
		expectedWalletName = walletName
		log.Printf("Setting wallet name in configuration: %s", expectedWalletName)

		// Optionally save the updated configuration to a file
		if err := viper.WriteConfig(); err != nil {
			log.Printf("Error saving updated configuration: %v", err)
		}
	}

	// Initialize the Graviton store
	store := &graviton.GravitonStore{}
	err := store.InitStore()
	if err != nil {
		log.Printf("Failed to initialize the Graviton store: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	for _, transaction := range transactions {
		walletName, ok := transaction["wallet_name"].(string)
		if !ok || walletName != expectedWalletName {
			log.Printf("Transaction from unknown wallet: %v", walletName)
			continue // Skip processing if wallet name doesn't match
		}

		address, ok := transaction["address"].(string)
		if !ok {
			log.Printf("Invalid address format: %v", transaction["address"])
			continue
		}

		// Check if the transaction matches a subscriber's address and update the subscription
		if err := processSubscriptionPayment(store, address, transaction); err != nil {
			log.Printf("Error processing subscription payment: %v", err)
			continue
		}

		dateStr, ok := transaction["date"].(string)
		if !ok {
			log.Printf("Invalid date format: %v", transaction["date"])
			continue
		}

		// Correct format for ISO 8601 datetime string with timezone
		date, err := time.Parse(time.RFC3339, dateStr)
		if err != nil {
			log.Printf("Error parsing date: %v", err)
			continue
		}

		output, ok := transaction["output"].(string)
		if !ok {
			log.Printf("Invalid output format: %v", transaction["output"])
			continue
		}

		valueStr, ok := transaction["value"].(string)
		if !ok {
			log.Printf("Invalid value format: %v", transaction["value"])
			continue
		}

		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			log.Printf("Error parsing value to float64: %v", err)
			continue
		}

		// Initialize the Gorm database
		db, err := graviton.InitGorm()
		if err != nil {
			log.Printf("Failed to connect to the database: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
		}

		var existingTransaction types.WalletTransactions
		result := db.Where("address = ? AND date = ? AND output = ? AND value = ?", address, date, output, valueStr).First(&existingTransaction)

		if result.Error == nil {
			// Transaction already exists, skip it
			log.Printf("Duplicate transaction found, skipping: %v", transaction)
			continue
		}

		if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
			log.Printf("Error querying transaction: %v", result.Error)
			continue
		}

		// Create a new transaction
		newTransaction := types.WalletTransactions{
			Address: address,
			Date:    date,
			Output:  output,
			Value:   fmt.Sprintf("%.8f", value),
		}
		if err := db.Create(&newTransaction).Error; err != nil {
			log.Printf("Error saving new transaction: %v", err)
			continue
		}
	}

	// Respond with a success message
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Transactions received and processed successfully",
	})
}

// processSubscriptionPayment checks if a transaction corresponds to a valid subscription payment
func processSubscriptionPayment(store *graviton.GravitonStore, address string, transaction map[string]interface{}) error {
	// Retrieve the subscription tiers from Viper
	var subscriptionTiers []types.SubscriptionTier
	err := viper.UnmarshalKey("subscription_tiers", &subscriptionTiers)
	if err != nil {
		return fmt.Errorf("failed to fetch subscription tiers: %v", err)
	}

	// Retrieve the subscriber associated with the address by finding their npub
	subscriber, err := store.GetSubscriberByAddress(address)
	if err != nil {
		return fmt.Errorf("subscriber not found: %v", err)
	}

	// Parse the transaction ID and value
	transactionID, ok := transaction["transaction_id"].(string)
	if !ok {
		return fmt.Errorf("transaction ID missing or invalid")
	}

	// Check if this transaction has already been processed
	if subscriber.LastTransactionID == transactionID {
		log.Printf("Transaction ID %s has already been processed for subscriber %s", transactionID, subscriber.Npub)
		return nil // Skip processing to avoid duplicate subscription updates
	}

	valueStr, ok := transaction["value"].(string)
	if !ok {
		return fmt.Errorf("transaction value missing or invalid")
	}

	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return fmt.Errorf("error parsing transaction value: %v", err)
	}

	// Check if the transaction value matches any subscription tier
	var matchedTier *types.SubscriptionTier
	for _, tier := range subscriptionTiers {
		// Convert tier.Price to float64
		tierPrice, err := strconv.ParseFloat(tier.Price, 64)
		if err != nil {
			return fmt.Errorf("error parsing tier price to float64: %v", err)
		}

		if value >= tierPrice {
			matchedTier = &tier
			break
		}
	}

	if matchedTier == nil {
		log.Printf("Transaction value %v does not match any subscription tier for address: %s", value, address)
		return nil // Payment too low or doesn't match any tier, skip
	}

	// Calculate the new subscription end date
	var newEndDate time.Time
	if time.Now().Before(subscriber.EndDate) {
		// If the current subscription is still active, extend from the current end date
		newEndDate = subscriber.EndDate.AddDate(0, 1, 0) // Extend by 1 month from the current end date
	} else {
		// If the subscription has expired, start from now
		newEndDate = time.Now().AddDate(0, 1, 0) // Set end date 1 month from now
	}

	// Update subscriber's subscription details
	subscriber.Tier = matchedTier.DataLimit
	subscriber.StartDate = time.Now()            // Update the start date to now
	subscriber.EndDate = newEndDate              // Set the new calculated end date
	subscriber.LastTransactionID = transactionID // Store the transaction ID to prevent duplicate processing

	err = store.SaveSubscriber(subscriber)
	if err != nil {
		return fmt.Errorf("failed to update subscriber: %v", err)
	}

	log.Printf("Subscriber %s activated/extended on tier %s with transaction ID %s. New end date: %v", subscriber.Npub, matchedTier.DataLimit, transactionID, newEndDate)
	return nil
}
