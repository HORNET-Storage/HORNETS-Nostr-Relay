package web

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	gorm "github.com/HORNET-Storage/hornet-storage/lib/stores/stats_stores"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/gofiber/fiber/v2"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
)

func updateWalletTransactions(c *fiber.Ctx, store *gorm.GormStatisticsStore) error {
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

	// Set wallet name from first transaction if not set
	if expectedWalletName == "" && len(transactions) > 0 {
		walletName, ok := transactions[0]["wallet_name"].(string)
		if !ok {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Wallet name missing or invalid",
			})
		}
		viper.Set("wallet_name", walletName)
		expectedWalletName = walletName
	}

	// Initialize the Graviton store for subscription processing
	gravitonStore := &graviton.GravitonStore{}
	queryCache := viper.GetStringMapString("query_cache")
	err := gravitonStore.InitStore("gravitondb", queryCache)
	if err != nil {
		log.Fatal(err)
	}

	for _, transaction := range transactions {
		walletName, ok := transaction["wallet_name"].(string)
		if !ok || walletName != expectedWalletName {
			continue // Skip processing if wallet name doesn't match
		}

		// Extract transaction details
		address, _ := transaction["address"].(string)
		dateStr, _ := transaction["date"].(string)
		date, err := time.Parse(time.RFC3339, dateStr)
		if err != nil {
			log.Printf("Error parsing date: %v", err)
			continue
		}
		output, _ := transaction["output"].(string)
		valueStr, _ := transaction["value"].(string)
		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			log.Printf("Error parsing value: %v", err)
			continue
		}

		// Process pending transactions
		txID := strings.Split(address, ":")[0]
		err = store.DeletePendingTransaction(txID)
		if err != nil {
			continue
		}

		// Check for existing transactions
		exists, err := store.TransactionExists(address, date, output, valueStr)
		if err != nil {
			log.Printf("Error checking existing transactions: %v", err)
			continue
		}
		if exists {
			continue
		}

		// Create a new transaction
		newTransaction := types.WalletTransactions{
			Address: address,
			Date:    date,
			Output:  output,
			Value:   fmt.Sprintf("%.8f", value),
		}

		err = store.SaveWalletTransaction(newTransaction)
		if err != nil {
			log.Printf("Error saving new transaction: %v", err)
			continue
		}

		// Process subscription payments
		err = processSubscriptionPayment(gravitonStore, output, transaction)
		if err != nil {
			log.Printf("Error processing subscription payment: %v", err)
		}
	}

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Transactions processed successfully",
	})
}

// processSubscriptionPayment checks if a transaction corresponds to a valid subscription payment
func processSubscriptionPayment(store stores.Store, address string, transaction map[string]interface{}) error {
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
	newEndDate := time.Now().AddDate(0, 1, 0) // Set end date 1 month from now
	if time.Now().Before(subscriber.EndDate) {
		// If the current subscription is still active, extend from the current end date
		newEndDate = subscriber.EndDate.AddDate(0, 1, 0)
	}

	// Update subscriber's subscription details
	subscriber.Tier = matchedTier.DataLimit
	subscriber.StartDate = time.Now()
	subscriber.EndDate = newEndDate
	subscriber.LastTransactionID = transactionID

	err = store.SaveSubscriber(subscriber)
	if err != nil {
		return fmt.Errorf("failed to update subscriber: %v", err)
	}

	// Update the NIP-88 event
	relayPrivKey, _, err := loadRelayPrivateKey() // You need to implement this function
	if err != nil {
		return fmt.Errorf("failed to load relay private key: %v", err)
	}

	err = UpdateNIP88EventAfterPayment(relayPrivKey, subscriber.Npub, store, matchedTier.DataLimit, newEndDate.Unix())
	if err != nil {
		return fmt.Errorf("failed to update NIP-88 event: %v", err)
	}

	log.Printf("Subscriber %s activated/extended on tier %s with transaction ID %s. New end date: %v", subscriber.Npub, matchedTier.DataLimit, transactionID, newEndDate)
	return nil
}

func UpdateNIP88EventAfterPayment(relayPrivKey *btcec.PrivateKey, userPubKey string, store stores.Store, tier string, expirationTimestamp int64) error {
	existingEvent, err := getExistingNIP88Event(store, userPubKey)
	if err != nil {
		return fmt.Errorf("error fetching existing NIP-88 event: %v", err)
	}
	if existingEvent == nil {
		return fmt.Errorf("no existing NIP-88 event found for user")
	}

	// Delete the existing event
	err = store.DeleteEvent(existingEvent.ID)
	if err != nil {
		return fmt.Errorf("error deleting existing NIP-88 event: %v", err)
	}

	// Create a new event with updated status
	newEvent := *existingEvent
	newEvent.CreatedAt = nostr.Timestamp(time.Now().Unix())

	// Update the tags
	for i, tag := range newEvent.Tags {
		switch tag[0] {
		case "subscription_status":
			newEvent.Tags[i] = nostr.Tag{"subscription_status", "active"}
		case "active_subscription":
			newEvent.Tags[i] = nostr.Tag{"active_subscription", tier, fmt.Sprintf("%d", expirationTimestamp)}
		}
	}

	// Generate new ID and signature
	serializedEvent := newEvent.Serialize()
	hash := sha256.Sum256(serializedEvent)
	newEvent.ID = hex.EncodeToString(hash[:])

	sig, err := schnorr.Sign(relayPrivKey, hash[:])
	if err != nil {
		return fmt.Errorf("error signing updated event: %v", err)
	}
	newEvent.Sig = hex.EncodeToString(sig.Serialize())

	// Store the updated event
	err = store.StoreEvent(&newEvent)
	if err != nil {
		return fmt.Errorf("failed to store updated NIP-88 event: %v", err)
	}

	return nil
}

func getExistingNIP88Event(store stores.Store, userPubKey string) (*nostr.Event, error) {
	filter := nostr.Filter{
		Kinds: []int{88},
		Tags: nostr.TagMap{
			"p": []string{userPubKey},
		},
		Limit: 1,
	}

	events, err := store.QueryEvents(filter)
	if err != nil {
		return nil, err
	}

	if len(events) > 0 {
		return events[0], nil
	}

	return nil, nil
}

func loadRelayPrivateKey() (*btcec.PrivateKey, *btcec.PublicKey, error) {

	privateKey, publicKey, err := signing.DeserializePrivateKey(viper.GetString("priv_key"))
	if err != nil {
		return nil, nil, fmt.Errorf("error getting keys: %s", err)
	}

	return privateKey, publicKey, nil
}
