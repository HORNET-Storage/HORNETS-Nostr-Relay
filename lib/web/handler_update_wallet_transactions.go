package web

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/subscription"
)

// updateWalletTransactions processes incoming wallet transactions
// This is the entry point for handling Bitcoin payments
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

	// Process each transaction
	for _, transaction := range transactions {
		// Skip transactions from different wallets
		walletName, ok := transaction["wallet_name"].(string)
		if !ok || walletName != expectedWalletName {
			continue
		}

		if err := processTransaction(store, subManager, transaction); err != nil {
			log.Printf("Error processing transaction: %v", err)
			// Continue processing other transactions even if one fails
			continue
		}
	}

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Transactions processed successfully",
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

	// Get subscriber by their Bitcoin address
	subscriber, err := store.GetSubscriberByAddress(txDetails.output)
	if err != nil {
		return fmt.Errorf("subscriber not found for address %s: %v", txDetails.output, err)
	}

	// Convert BTC value to satoshis for subscription processing
	satoshis := int64(txDetails.value * 100_000_000)

	// Process the subscription payment
	if err := subManager.ProcessPayment(subscriber.Npub, txID, satoshis); err != nil {
		return fmt.Errorf("failed to process subscription: %v", err)
	}

	log.Printf("Successfully processed subscription payment for %s: %s sats",
		subscriber.Npub, txDetails.valueStr)
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

// package web

// import (
// 	"crypto/sha256"
// 	"encoding/hex"
// 	"fmt"
// 	"log"
// 	"strconv"
// 	"strings"
// 	"time"

// 	"github.com/btcsuite/btcd/btcec/v2"
// 	"github.com/btcsuite/btcd/btcec/v2/schnorr"
// 	"github.com/gofiber/fiber/v2"
// 	"github.com/nbd-wtf/go-nostr"
// 	"github.com/spf13/viper"

// 	types "github.com/HORNET-Storage/hornet-storage/lib"
// 	"github.com/HORNET-Storage/hornet-storage/lib/signing"
// 	"github.com/HORNET-Storage/hornet-storage/lib/stores"
// )

// func updateWalletTransactions(c *fiber.Ctx, store stores.Store) error {
// 	var transactions []map[string]interface{}
// 	log.Println("Transactions request received")

// 	// Parse the JSON body into the slice of maps
// 	if err := c.BodyParser(&transactions); err != nil {
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
// 			"error": "Cannot parse JSON",
// 		})
// 	}

// 	// Get the expected wallet name from the configuration
// 	expectedWalletName := viper.GetString("wallet_name")

// 	// Set wallet name from first transaction if not set
// 	if expectedWalletName == "" && len(transactions) > 0 {
// 		walletName, ok := transactions[0]["wallet_name"].(string)
// 		if !ok {
// 			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
// 				"error": "Wallet name missing or invalid",
// 			})
// 		}
// 		viper.Set("wallet_name", walletName)
// 		expectedWalletName = walletName
// 	}

// 	for _, transaction := range transactions {
// 		walletName, ok := transaction["wallet_name"].(string)
// 		if !ok || walletName != expectedWalletName {
// 			continue // Skip processing if wallet name doesn't match
// 		}

// 		// Extract transaction details
// 		address, _ := transaction["address"].(string)
// 		dateStr, _ := transaction["date"].(string)
// 		date, err := time.Parse(time.RFC3339, dateStr)
// 		if err != nil {
// 			log.Printf("Error parsing date: %v", err)
// 			continue
// 		}
// 		output, _ := transaction["output"].(string)
// 		valueStr, _ := transaction["value"].(string)
// 		value, err := strconv.ParseFloat(valueStr, 64)
// 		if err != nil {
// 			log.Printf("Error parsing value: %v", err)
// 			continue
// 		}

// 		// Process pending transactions
// 		txID := strings.Split(address, ":")[0]
// 		err = store.GetStatsStore().DeletePendingTransaction(txID)
// 		if err != nil {
// 			continue
// 		}

// 		// Check for existing transactions
// 		exists, err := store.GetStatsStore().TransactionExists(address, date, output, valueStr)
// 		if err != nil {
// 			log.Printf("Error checking existing transactions: %v", err)
// 			continue
// 		}
// 		if exists {
// 			continue
// 		}

// 		// Create a new transaction
// 		newTransaction := types.WalletTransactions{
// 			Address: address,
// 			Date:    date,
// 			Output:  output,
// 			Value:   fmt.Sprintf("%.8f", value),
// 		}

// 		err = store.GetStatsStore().SaveWalletTransaction(newTransaction)
// 		if err != nil {
// 			log.Printf("Error saving new transaction: %v", err)
// 			continue
// 		}

// 		// Process subscription payments
// 		err = processSubscriptionPayment(store, transaction)
// 		if err != nil {
// 			log.Printf("Error processing subscription payment: %v", err)
// 		}
// 	}

// 	return c.JSON(fiber.Map{
// 		"status":  "success",
// 		"message": "Transactions processed successfully",
// 	})
// }

// func processSubscriptionPayment(store stores.Store, transaction map[string]interface{}) error {
// 	log.Printf("Processing transaction: %+v", transaction)

// 	// Get subscription tiers from config
// 	var subscriptionTiers []types.SubscriptionTier
// 	if err := viper.UnmarshalKey("subscription_tiers", &subscriptionTiers); err != nil {
// 		return fmt.Errorf("failed to fetch subscription tiers: %v", err)
// 	}

// 	// Log subscription tiers to confirm they are loaded correctly
// 	for _, tier := range subscriptionTiers {
// 		log.Printf("Loaded subscription tier: DataLimit=%s, Price=%s", tier.DataLimit, tier.Price)
// 	}

// 	// Extract and validate the Bitcoin address
// 	output, ok := transaction["output"].(string)
// 	if !ok {
// 		return fmt.Errorf("invalid output in transaction")
// 	}
// 	log.Printf("Looking up subscriber for address: %s", output)

// 	// Debug: Check if address exists in subscriber_addresses table
// 	if store, ok := store.(interface {
// 		AddressExists(string) (bool, error)
// 	}); ok {
// 		exists, err := store.AddressExists(output)
// 		if err != nil {
// 			log.Printf("Error checking address existence: %v", err)
// 		} else {
// 			log.Printf("Address %s exists in database: %v", output, exists)
// 		}
// 	}

// 	// Debug: Check address allocation status
// 	if store, ok := store.(interface {
// 		DebugAddressDetails(string)
// 	}); ok {
// 		log.Printf("Checking address details for: %s", output)
// 		store.DebugAddressDetails(output)
// 	}

// 	// Get subscriber details
// 	subscriber, err := store.GetSubscriberByAddress(output)
// 	if err != nil {
// 		log.Printf("Failed to find subscriber for address %s: %v", output, err)
// 		// Dump the subscriber_addresses table contents for debugging
// 		if store, ok := store.(interface {
// 			DumpAddressTable()
// 		}); ok {
// 			store.DumpAddressTable()
// 		}
// 		return fmt.Errorf("subscriber not found: %v", err)
// 	}

// 	// Validate and parse transaction details
// 	transactionID, valueStr, err := validateTransactionDetails(transaction, subscriber)
// 	if err != nil {
// 		return err
// 	}

// 	// Find matching tier for payment amount
// 	matchedTier, err := findMatchingTier(valueStr, subscriptionTiers)
// 	if err != nil {
// 		return err
// 	}
// 	if matchedTier == nil {
// 		log.Printf("Transaction value %v does not match any subscription tier for address: %s", valueStr, output)
// 		return nil
// 	}

// 	// Log to confirm the DataLimit value
// 	log.Printf("Matched tier data limit: %s", matchedTier.DataLimit)

// 	// Update subscription details
// 	newEndDate := calculateNewEndDate(subscriber.EndDate)
// 	storageLimitBytes, err := stores.ParseStorageLimit(matchedTier.DataLimit)
// 	if err != nil {
// 		return fmt.Errorf("failed to parse storage limit: %v", err)
// 	}

// 	// Update subscriber record
// 	if err := updateSubscriberRecord(store, subscriber, matchedTier, transactionID, newEndDate, storageLimitBytes); err != nil {
// 		return err
// 	}

// 	// Update NIP-88 event
// 	if err := updateNIP88Event(store, subscriber, matchedTier, newEndDate); err != nil {
// 		log.Printf("Warning: NIP-88 event update failed: %v", err)
// 		// Continue despite NIP-88 error as the subscription is already updated
// 	}

// 	log.Printf("Subscriber %s activated/extended on tier %s with transaction ID %s. New end date: %v",
// 		subscriber.Npub, matchedTier.DataLimit, transactionID, newEndDate)

// 	return nil
// }

// func validateTransactionDetails(transaction map[string]interface{}, subscriber *types.Subscriber) (string, string, error) {
// 	txAddressField, ok := transaction["address"].(string)
// 	if !ok || txAddressField == "" {
// 		return "", "", fmt.Errorf("transaction ID missing or invalid")
// 	}
// 	transactionID := strings.Split(txAddressField, ":")[0]

// 	if subscriber.LastTransactionID == transactionID {
// 		log.Printf("Transaction ID %s has already been processed for subscriber %s", transactionID, subscriber.Npub)
// 		return "", "", fmt.Errorf("transaction already processed")
// 	}

// 	valueStr, ok := transaction["value"].(string)
// 	if !ok {
// 		return "", "", fmt.Errorf("transaction value missing or invalid")
// 	}

// 	return transactionID, valueStr, nil
// }

// func findMatchingTier(valueStr string, tiers []types.SubscriptionTier) (*types.SubscriptionTier, error) {
// 	// Parse the BTC value as a float
// 	value, err := strconv.ParseFloat(valueStr, 64)
// 	if err != nil {
// 		return nil, fmt.Errorf("error parsing transaction value: %v", err)
// 	}

// 	// Convert the value from BTC to satoshis (1 BTC = 100,000,000 satoshis)
// 	paymentSats := int64(value * 100_000_000)
// 	log.Printf("Processing payment of %d satoshis", paymentSats)

// 	var bestMatch *types.SubscriptionTier
// 	var bestPrice int64 = 0

// 	for _, tier := range tiers {
// 		log.Printf("Checking tier: DataLimit=%s, Price=%s", tier.DataLimit, tier.Price)

// 		// Parse the tier price as an integer in satoshis
// 		tierPrice, err := strconv.ParseInt(tier.Price, 10, 64)
// 		if err != nil {
// 			log.Printf("Warning: invalid tier price configuration: %v", err)
// 			continue
// 		}

// 		// Check if the payment meets or exceeds the tier price, and if it’s the highest eligible price
// 		if paymentSats >= tierPrice && tierPrice > bestPrice {
// 			tierCopy := tier // Copy the struct to avoid pointer issues
// 			bestMatch = &tierCopy
// 			bestPrice = tierPrice
// 			log.Printf("Found matching tier: %s (price: %d sats)", tier.DataLimit, tierPrice)
// 		}
// 	}

// 	if bestMatch != nil {
// 		log.Printf("Selected tier: %s for payment of %d satoshis", bestMatch.DataLimit, paymentSats)
// 	} else {
// 		log.Printf("No matching tier found for payment of %d satoshis", paymentSats)
// 	}

// 	return bestMatch, nil
// }

// func calculateNewEndDate(currentEndDate time.Time) time.Time {
// 	if time.Now().Before(currentEndDate) {
// 		return currentEndDate.AddDate(0, 1, 0)
// 	}
// 	return time.Now().AddDate(0, 1, 0)
// }

// func updateSubscriberRecord(store stores.Store, subscriber *types.Subscriber, tier *types.SubscriptionTier,
// 	transactionID string, endDate time.Time, storageLimitBytes int64) error {

// 	log.Println("Updating subscriber: ", subscriber.Npub)
// 	subscriber.Tier = tier.DataLimit
// 	subscriber.StartDate = time.Now()
// 	subscriber.EndDate = endDate
// 	subscriber.LastTransactionID = transactionID

// 	if subscriberStore, ok := store.(stores.SubscriberStore); ok {
// 		period := &types.SubscriptionPeriod{
// 			TransactionID:     transactionID,
// 			Tier:              tier.DataLimit,
// 			StorageLimitBytes: storageLimitBytes,
// 			StartDate:         time.Now(),
// 			EndDate:           endDate,
// 			PaymentAmount:     tier.Price,
// 		}
// 		if err := subscriberStore.AddSubscriptionPeriod(subscriber.Npub, period); err != nil {
// 			return fmt.Errorf("failed to add subscription period: %v", err)
// 		}
// 	}

// 	err := store.DeleteSubscriber(subscriber.Npub)
// 	if err != nil {
// 		return fmt.Errorf("failed to delete subscriber: %v", err)
// 	}

// 	newSubscriberEntry := &types.Subscriber{
// 		Npub:              subscriber.Npub,
// 		Tier:              tier.DataLimit,
// 		StartDate:         time.Now(),
// 		EndDate:           endDate,
// 		Address:           subscriber.Address,
// 		LastTransactionID: transactionID,
// 	}

// 	return store.SaveSubscriber(newSubscriberEntry)
// }

// func updateNIP88Event(store stores.Store, subscriber *types.Subscriber, tier *types.SubscriptionTier, endDate time.Time) error {
// 	relayPrivKey, _, err := loadRelayPrivateKey()
// 	if err != nil {
// 		return fmt.Errorf("failed to load relay private key: %v", err)
// 	}

// 	return UpdateNIP88EventAfterPayment(relayPrivKey, subscriber.Npub, store, tier.DataLimit, endDate.Unix())
// }

// func UpdateNIP88EventAfterPayment(relayPrivKey *btcec.PrivateKey, userPubKey string, store stores.Store, tier string, expirationTimestamp int64) error {
// 	existingEvent, err := getExistingNIP88Event(store, userPubKey)
// 	if err != nil {
// 		return fmt.Errorf("error fetching existing NIP-88 event: %v", err)
// 	}
// 	if existingEvent == nil {
// 		return fmt.Errorf("no existing NIP-88 event found for user")
// 	}

// 	// Delete the existing event
// 	err = store.DeleteEvent(existingEvent.ID)
// 	if err != nil {
// 		return fmt.Errorf("error deleting existing NIP-88 event: %v", err)
// 	}

// 	subscriptionTiers := []types.SubscriptionTier{
// 		{DataLimit: "1 GB per month", Price: "8000"},
// 		{DataLimit: "5 GB per month", Price: "10000"},
// 		{DataLimit: "10 GB per month", Price: "15000"},
// 	}

// 	var relayAddress string
// 	for _, tag := range existingEvent.Tags {
// 		if tag[0] == "relay_bitcoin_address" && len(tag) > 1 {
// 			relayAddress = tag[1]
// 			break
// 		}
// 	}

// 	tags := []nostr.Tag{
// 		{"subscription_duration", "1 month"},
// 		{"p", userPubKey},
// 		{"subscription_status", "active"},
// 		{"relay_bitcoin_address", relayAddress},
// 		{"relay_dht_key", viper.GetString("RelayDHTkey")},
// 		{"active_subscription", tier, fmt.Sprintf("%d", expirationTimestamp)},
// 	}

// 	for _, tier := range subscriptionTiers {
// 		tags = append(tags, nostr.Tag{"subscription-tier", tier.DataLimit, tier.Price})
// 	}

// 	serializedPrivateKey, err := signing.SerializePrivateKey(relayPrivKey)
// 	if err != nil {
// 		log.Printf("failed to serialize private key")
// 	}

// 	event := &nostr.Event{
// 		PubKey:    *serializedPrivateKey,
// 		CreatedAt: nostr.Timestamp(time.Now().Unix()),
// 		Kind:      764,
// 		Tags:      tags,
// 		Content:   "",
// 	}

// 	// Generate the event ID
// 	serializedEvent := event.Serialize()
// 	hash := sha256.Sum256(serializedEvent)
// 	event.ID = hex.EncodeToString(hash[:])

// 	// Sign the event
// 	sig, err := schnorr.Sign(relayPrivKey, hash[:])
// 	if err != nil {
// 		return fmt.Errorf("error signing event: %v", err)
// 	}
// 	event.Sig = hex.EncodeToString(sig.Serialize())

// 	log.Println("Storing updated kind 764 event")

// 	// Store the event
// 	err = store.StoreEvent(event)
// 	if err != nil {
// 		return fmt.Errorf("failed to store NIP-88 event: %v", err)
// 	}

// 	log.Println("Kind 764 event successfully stored.")

// 	return nil
// }

// func getExistingNIP88Event(store stores.Store, userPubKey string) (*nostr.Event, error) {
// 	filter := nostr.Filter{
// 		Kinds: []int{764},
// 		Tags: nostr.TagMap{
// 			"p": []string{userPubKey},
// 		},
// 		Limit: 1,
// 	}

// 	events, err := store.QueryEvents(filter)
// 	if err != nil {
// 		return nil, err
// 	}

// 	if len(events) > 0 {
// 		return events[0], nil
// 	}

// 	return nil, nil
// }

// func loadRelayPrivateKey() (*btcec.PrivateKey, *btcec.PublicKey, error) {
// 	privateKey, publicKey, err := signing.DeserializePrivateKey(viper.GetString("priv_key"))
// 	if err != nil {
// 		return nil, nil, fmt.Errorf("error getting keys: %s", err)
// 	}

// 	return privateKey, publicKey, nil
// }
