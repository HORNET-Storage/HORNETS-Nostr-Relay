// subscription.go

package subscription

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/nbd-wtf/go-nostr"

	"github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
)

// StorageInfo tracks current storage usage information for a subscriber
type StorageInfo struct {
	UsedBytes  int64     // Current bytes used by the subscriber
	TotalBytes int64     // Total bytes allocated to the subscriber
	UpdatedAt  time.Time // Last time storage information was updated
}

// SubscriptionManager handles all subscription-related operations including:
// - Subscriber management
// - NIP-88 event creation and updates
// - Storage tracking
// - Payment processing
type SubscriptionManager struct {
	store           stores.Store      // Interface to the storage layer
	relayPrivateKey *btcec.PrivateKey // Relay's private key for signing events
	// relayBTCAddress   string                 // Relay's Bitcoin address for payments
	relayDHTKey       string                 // Relay's DHT key
	subscriptionTiers []lib.SubscriptionTier // Available subscription tiers
}

// NewSubscriptionManager creates and initializes a new subscription manager
func NewSubscriptionManager(
	store stores.Store,
	relayPrivKey *btcec.PrivateKey,
	// relayBTCAddress string,
	relayDHTKey string,
	tiers []lib.SubscriptionTier,
) *SubscriptionManager {
	return &SubscriptionManager{
		store:           store,
		relayPrivateKey: relayPrivKey,
		// relayBTCAddress:   relayBTCAddress,
		relayDHTKey:       relayDHTKey,
		subscriptionTiers: tiers,
	}
}

// InitializeSubscriber creates a new subscriber or retrieves an existing one
// and creates their initial NIP-88 event. This is called when a user first
// connects to the relay.
func (m *SubscriptionManager) InitializeSubscriber(npub string) error {
	// Step 1: Get or create subscriber record
	subscriber, err := m.getOrCreateSubscriber(npub)
	if err != nil {
		return fmt.Errorf("failed to initialize subscriber: %v", err)
	}

	// Step 2: Create initial NIP-88 event with zero storage usage
	storageInfo := StorageInfo{
		UsedBytes:  0,
		TotalBytes: 0,
		UpdatedAt:  time.Now(),
	}

	// Step 3: Create the NIP-88 event
	return m.createOrUpdateNIP88Event(subscriber, "", time.Time{}, &storageInfo)
}

// ProcessPayment handles a new subscription payment. It:
// - Validates the payment amount against available tiers
// - Updates subscriber information
// - Creates a new subscription period
// - Updates the NIP-88 event
func (m *SubscriptionManager) ProcessPayment(
	npub string,
	transactionID string,
	amountSats int64,
) error {
	// Step 1: Find matching tier for payment amount
	tier, err := m.findMatchingTier(amountSats)
	if err != nil {
		return fmt.Errorf("error matching tier: %v", err)
	}

	// Step 2: Get existing subscriber
	subscriber, err := m.store.GetSubscriber(npub)
	if err != nil {
		return fmt.Errorf("subscriber not found: %v", err)
	}

	// Step 3: Verify transaction hasn't been processed before
	existingPeriod, err := m.store.GetSubscriberStore().GetSubscriptionByTransactionID(transactionID)
	if err == nil && existingPeriod != nil {
		return fmt.Errorf("transaction %s already processed", transactionID)
	}

	// Step 4: Calculate subscription period dates
	startDate := time.Now()
	endDate := m.calculateEndDate(subscriber.EndDate)
	storageLimit := m.calculateStorageLimit(tier.DataLimit)

	// Step 5: Initialize storage tracking
	storageInfo := StorageInfo{
		UsedBytes:  0, // Reset for new subscription
		TotalBytes: storageLimit,
		UpdatedAt:  time.Now(),
	}

	log.Printf("Updating NIP-88 event for subscriber %s with tier %s", npub, tier.DataLimit)
	// Step 6: Update NIP-88 event
	if err := m.createOrUpdateNIP88Event(subscriber, tier.DataLimit, endDate, &storageInfo); err != nil {
		log.Printf("Error updating NIP-88 event: %v", err)
	}

	// Step 7: Create subscription period record
	period := &lib.SubscriptionPeriod{
		TransactionID:     transactionID,
		Tier:              tier.DataLimit,
		StorageLimitBytes: storageLimit,
		StartDate:         startDate,
		EndDate:           endDate,
		PaymentAmount:     fmt.Sprintf("%d", amountSats),
	}

	if err := m.store.GetSubscriberStore().AddSubscriptionPeriod(npub, period); err != nil {
		return fmt.Errorf("failed to add subscription period: %v", err)
	}

	// Step 8: Update subscriber record
	subscriber.Tier = tier.DataLimit
	subscriber.StartDate = startDate
	subscriber.EndDate = endDate
	subscriber.LastTransactionID = transactionID

	if err := m.store.SaveSubscriber(subscriber); err != nil {
		return fmt.Errorf("failed to update subscriber: %v", err)
	}

	return nil
}

// UpdateStorageUsage updates the storage usage for a subscriber when they upload or delete files
// It updates both the subscriber store and the NIP-88 event
func (m *SubscriptionManager) UpdateStorageUsage(npub string, newBytes int64) error {
	// Step 1: Get current NIP-88 event to check current storage usage
	events, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{764},
		Tags: nostr.TagMap{
			"p": []string{npub},
		},
		Limit: 1,
	})
	if err != nil || len(events) == 0 {
		return fmt.Errorf("no NIP-88 event found for user")
	}

	currentEvent := events[0]

	// Step 2: Get current storage information
	storageInfo, err := m.extractStorageInfo(currentEvent)
	if err != nil {
		return fmt.Errorf("failed to extract storage info: %v", err)
	}

	// Step 3: Validate new storage usage
	newUsedBytes := storageInfo.UsedBytes + newBytes
	if newUsedBytes > storageInfo.TotalBytes {
		return fmt.Errorf("storage limit exceeded: would use %d of %d bytes",
			newUsedBytes, storageInfo.TotalBytes)
	}

	// Step 4: Update storage tracking
	storageInfo.UsedBytes = newUsedBytes
	storageInfo.UpdatedAt = time.Now()

	// Step 5: Get subscriber record
	subscriber, err := m.store.GetSubscriber(npub)
	if err != nil {
		return fmt.Errorf("failed to get subscriber: %v", err)
	}

	// Step 6: Update storage usage in subscriber store
	if err := m.store.GetSubscriberStore().UpdateStorageUsage(npub, newBytes); err != nil {
		return fmt.Errorf("failed to update storage usage: %v", err)
	}

	// Step 7: Get current subscription info from event tags
	activeTier := ""
	var expirationDate time.Time
	for _, tag := range currentEvent.Tags {
		if tag[0] == "active_subscription" && len(tag) >= 3 {
			activeTier = tag[1]
			timestamp, _ := strconv.ParseInt(tag[2], 10, 64)
			expirationDate = time.Unix(timestamp, 0)
			break
		}
	}

	// Step 8: Update NIP-88 event with new storage information
	return m.createOrUpdateNIP88Event(subscriber, activeTier, expirationDate, &storageInfo)
}

// Private helper methods

// getOrCreateSubscriber retrieves an existing subscriber or creates a new one
func (m *SubscriptionManager) getOrCreateSubscriber(npub string) (*lib.Subscriber, error) {
	subscriber, err := m.store.GetSubscriber(npub)
	if err == nil {
		return subscriber, nil
	}

	// Allocate a unique Bitcoin address for the new subscriber
	address, err := m.store.GetSubscriberStore().AllocateBitcoinAddress(npub)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate Bitcoin address: %v", err)
	}

	log.Println("User allocated address: ", address.Address)

	testAddress := "bc1qfjqax7sm9s5zcxwyq4r2shqlywh9re2l35mxa4"

	// Create new subscriber with default values
	newSubscriber := &lib.Subscriber{
		Npub:              npub,
		Tier:              "",
		StartDate:         time.Time{},
		EndDate:           time.Time{},
		Address:           testAddress,
		LastTransactionID: "",
	}

	if err := m.store.SaveSubscriber(newSubscriber); err != nil {
		return nil, err
	}

	return newSubscriber, nil
}

// createOrUpdateNIP88Event creates or updates a subscriber's NIP-88 event
func (m *SubscriptionManager) createOrUpdateNIP88Event(
	subscriber *lib.Subscriber,
	activeTier string,
	expirationDate time.Time,
	storageInfo *StorageInfo,
) error {
	// Step 1: Delete existing NIP-88 event if it exists
	existingEvents, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{764},
		Tags: nostr.TagMap{
			"p": []string{subscriber.Npub},
		},
		Limit: 1,
	})
	if err == nil && len(existingEvents) > 0 {
		if err := m.store.DeleteEvent(existingEvents[0].ID); err != nil {
			log.Printf("Warning: failed to delete existing NIP-88 event: %v", err)
		}
	}

	// Step 2: Prepare event tags
	tags := []nostr.Tag{
		{"subscription_duration", "1 month"},
		{"p", subscriber.Npub},
		{"subscription_status", m.getSubscriptionStatus(activeTier)},
		{"relay_bitcoin_address", subscriber.Address},
		{"relay_dht_key", m.relayDHTKey},
		// Add storage information tag
		{"storage",
			fmt.Sprintf("%d", storageInfo.UsedBytes),
			fmt.Sprintf("%d", storageInfo.TotalBytes),
			fmt.Sprintf("%d", storageInfo.UpdatedAt.Unix()),
		},
	}

	// Add available subscription tiers
	for _, tier := range m.subscriptionTiers {
		tags = append(tags, nostr.Tag{"subscription-tier", tier.DataLimit, tier.Price})
	}

	// Add active subscription info if applicable
	if activeTier != "" {
		tags = append(tags, nostr.Tag{
			"active_subscription",
			activeTier,
			fmt.Sprintf("%d", expirationDate.Unix()),
		})
	}

	// Step 3: Create new event
	event := &nostr.Event{
		PubKey:    hex.EncodeToString(m.relayPrivateKey.PubKey().SerializeCompressed()),
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      764,
		Tags:      tags,
		Content:   "",
	}

	// Generate event ID
	serializedEvent := event.Serialize()
	hash := sha256.Sum256(serializedEvent)
	event.ID = hex.EncodeToString(hash[:])

	// Sign event
	sig, err := schnorr.Sign(m.relayPrivateKey, hash[:])
	if err != nil {
		return fmt.Errorf("error signing event: %v", err)
	}
	event.Sig = hex.EncodeToString(sig.Serialize())

	// Step 4: Store the event
	return m.store.StoreEvent(event)
}

// Helper functions

// findMatchingTier finds the highest tier that matches the payment amount
func (m *SubscriptionManager) findMatchingTier(amountSats int64) (*lib.SubscriptionTier, error) {
	var bestMatch *lib.SubscriptionTier
	var bestPrice int64

	for _, tier := range m.subscriptionTiers {
		price := m.parseSats(tier.Price)
		if amountSats >= price && price > bestPrice {
			tierCopy := tier
			bestMatch = &tierCopy
			bestPrice = price
		}
	}

	if bestMatch == nil {
		return nil, fmt.Errorf("no matching tier for payment of %d sats", amountSats)
	}

	return bestMatch, nil
}

// calculateEndDate determines the subscription end date
func (m *SubscriptionManager) calculateEndDate(currentEnd time.Time) time.Time {
	if time.Now().Before(currentEnd) {
		return currentEnd.AddDate(0, 1, 0) // Extend by 1 month
	}
	return time.Now().AddDate(0, 1, 0) // Start new 1 month period
}

// calculateStorageLimit converts tier string to bytes
func (m *SubscriptionManager) calculateStorageLimit(tier string) int64 {
	switch tier {
	case "1 GB per month":
		return 1 * 1024 * 1024 * 1024
	case "5 GB per month":
		return 5 * 1024 * 1024 * 1024
	case "10 GB per month":
		return 10 * 1024 * 1024 * 1024
	default:
		return 0
	}
}

// getSubscriptionStatus returns the subscription status string
func (m *SubscriptionManager) getSubscriptionStatus(activeTier string) string {
	if activeTier == "" {
		return "inactive"
	}
	return "active"
}

// parseSats converts price string to satoshis
func (m *SubscriptionManager) parseSats(price string) int64 {
	var sats int64
	fmt.Sscanf(price, "%d", &sats)
	return sats
}

// extractStorageInfo gets storage information from NIP-88 event
func (m *SubscriptionManager) extractStorageInfo(event *nostr.Event) (StorageInfo, error) {
	var info StorageInfo

	for _, tag := range event.Tags {
		if tag[0] == "storage" && len(tag) >= 4 {
			used, err := strconv.ParseInt(tag[1], 10, 64)
			if err != nil {
				return info, fmt.Errorf("invalid used storage value: %v", err)
			}

			total, err := strconv.ParseInt(tag[2], 10, 64)
			if err != nil {
				return info, fmt.Errorf("invalid total storage value: %v", err)
			}

			updated, err := strconv.ParseInt(tag[3], 10, 64)
			if err != nil {
				return info, fmt.Errorf("invalid update timestamp: %v", err)
			}

			info.UsedBytes = used
			info.TotalBytes = total
			info.UpdatedAt = time.Unix(updated, 0)
			return info, nil
		}
	}

	// Return zero values if no storage tag found
	return StorageInfo{
		UsedBytes:  0,
		TotalBytes: 0,
		UpdatedAt:  time.Now(),
	}, nil
}

// package subscription

// import (
// 	"crypto/sha256"
// 	"encoding/hex"
// 	"fmt"
// 	"log"
// 	"strconv"
// 	"time"

// 	"github.com/btcsuite/btcd/btcec/v2"
// 	"github.com/btcsuite/btcd/btcec/v2/schnorr"
// 	"github.com/nbd-wtf/go-nostr"

// 	"github.com/HORNET-Storage/hornet-storage/lib"
// 	"github.com/HORNET-Storage/hornet-storage/lib/stores"
// )

// // StorageInfo tracks current storage usage information for a subscriber
// type StorageInfo struct {
// 	UsedBytes  int64     // Current bytes used by the subscriber
// 	TotalBytes int64     // Total bytes allocated to the subscriber
// 	UpdatedAt  time.Time // Last time storage information was updated
// }

// // SubscriptionManager handles all subscription-related operations including:
// // - Subscriber management
// // - NIP-88 event creation and updates
// // - Storage tracking
// // - Payment processing
// type SubscriptionManager struct {
// 	store           stores.Store      // Interface to the storage layer
// 	relayPrivateKey *btcec.PrivateKey // Relay's private key for signing events
// 	// relayBTCAddress   string                 // Relay's Bitcoin address for payments
// 	relayDHTKey       string                 // Relay's DHT key
// 	subscriptionTiers []lib.SubscriptionTier // Available subscription tiers
// }

// // NewSubscriptionManager creates and initializes a new subscription manager
// func NewSubscriptionManager(
// 	store stores.Store,
// 	relayPrivKey *btcec.PrivateKey,
// 	// relayBTCAddress string,
// 	relayDHTKey string,
// 	tiers []lib.SubscriptionTier,
// ) *SubscriptionManager {
// 	return &SubscriptionManager{
// 		store:           store,
// 		relayPrivateKey: relayPrivKey,
// 		// relayBTCAddress:   relayBTCAddress,
// 		relayDHTKey:       relayDHTKey,
// 		subscriptionTiers: tiers,
// 	}
// }

// // InitializeSubscriber creates a new subscriber or retrieves an existing one
// // and creates their initial NIP-88 event. This is called when a user first
// // connects to the relay.
// func (m *SubscriptionManager) InitializeSubscriber(npub string) error {
// 	// Step 1: Get or create subscriber record
// 	subscriber, err := m.getOrCreateSubscriber(npub)
// 	if err != nil {
// 		return fmt.Errorf("failed to initialize subscriber: %v", err)
// 	}

// 	// Step 2: Create initial NIP-88 event with zero storage usage
// 	storageInfo := StorageInfo{
// 		UsedBytes:  0,
// 		TotalBytes: 0,
// 		UpdatedAt:  time.Now(),
// 	}

// 	// Step 3: Create the NIP-88 event
// 	return m.createOrUpdateNIP88Event(subscriber, "", time.Time{}, &storageInfo)
// }

// // ProcessPayment handles a new subscription payment. It:
// // - Validates the payment amount against available tiers
// // - Updates subscriber information
// // - Creates a new subscription period
// // - Updates the NIP-88 event
// // ProcessPayment handles a new subscription payment and updates the NIP-88 event
// func (m *SubscriptionManager) ProcessPayment(npub string, transactionID string, amountSats int64) error {
// 	log.Printf("Starting ProcessPayment for npub: %s, transactionID: %s, amountSats: %d", npub, transactionID, amountSats)

// 	// Step 1: Match payment amount to a subscription tier
// 	tier, err := m.findMatchingTier(amountSats)
// 	if err != nil {
// 		log.Printf("Error matching tier for amount %d: %v", amountSats, err)
// 		return fmt.Errorf("error matching tier: %v", err)
// 	}
// 	log.Printf("Matched tier: %s for payment of %d sats", tier.DataLimit, amountSats)

// 	// Step 2: Retrieve the existing subscriber
// 	subscriber, err := m.store.GetSubscriber(npub)
// 	if err != nil {
// 		log.Printf("Error retrieving subscriber with npub %s: %v", npub, err)
// 		return fmt.Errorf("subscriber not found: %v", err)
// 	}
// 	log.Printf("Retrieved subscriber: %v", subscriber)

// 	// Step 3: Check if this transaction has already been processed
// 	existingPeriod, err := m.store.GetSubscriberStore().GetSubscriptionByTransactionID(transactionID)
// 	if err == nil && existingPeriod != nil {
// 		log.Printf("Transaction %s has already been processed for subscriber %s", transactionID, npub)
// 		return fmt.Errorf("transaction %s already processed", transactionID)
// 	}

// 	// Step 4: Calculate subscription period dates and storage limit
// 	startDate := time.Now()
// 	endDate := m.calculateEndDate(subscriber.EndDate)
// 	storageLimit := m.calculateStorageLimit(tier.DataLimit)
// 	log.Printf("Calculated subscription period: StartDate=%v, EndDate=%v, StorageLimit=%d bytes", startDate, endDate, storageLimit)

// 	// Step 5: Initialize storage tracking for new subscription period
// 	storageInfo := StorageInfo{
// 		UsedBytes:  0,
// 		TotalBytes: storageLimit,
// 		UpdatedAt:  time.Now(),
// 	}
// 	log.Printf("Initialized StorageInfo: %v", storageInfo)

// 	// Step 6: Create a new subscription period record
// 	period := &lib.SubscriptionPeriod{
// 		TransactionID:     transactionID,
// 		Tier:              tier.DataLimit,
// 		StorageLimitBytes: storageLimit,
// 		StartDate:         startDate,
// 		EndDate:           endDate,
// 		PaymentAmount:     fmt.Sprintf("%d", amountSats),
// 	}
// 	if err := m.store.GetSubscriberStore().AddSubscriptionPeriod(npub, period); err != nil {
// 		log.Printf("Error adding subscription period for subscriber %s: %v", npub, err)
// 		return fmt.Errorf("failed to add subscription period: %v", err)
// 	}
// 	log.Printf("Added subscription period: %v", period)

// 	// Step 7: Update the subscriber record with the new subscription information
// 	subscriber.Tier = tier.DataLimit
// 	subscriber.StartDate = startDate
// 	subscriber.EndDate = endDate
// 	subscriber.LastTransactionID = transactionID
// 	if err := m.store.SaveSubscriber(subscriber); err != nil {
// 		log.Printf("Error saving updated subscriber %s: %v", npub, err)
// 		return fmt.Errorf("failed to update subscriber: %v", err)
// 	}
// 	log.Printf("Updated subscriber record: %v", subscriber)

// 	// Step 8: Update the NIP-88 event for the subscriber
// 	if err := m.createOrUpdateNIP88Event(subscriber, tier.DataLimit, endDate, &storageInfo); err != nil {
// 		log.Printf("Error updating NIP-88 event for subscriber %s: %v", npub, err)
// 		return fmt.Errorf("failed to update NIP-88 event: %v", err)
// 	}
// 	log.Printf("Successfully updated NIP-88 event for subscriber %s with tier %s", npub, tier.DataLimit)

// 	log.Printf("ProcessPayment completed successfully for npub: %s, transactionID: %s", npub, transactionID)
// 	return nil
// }

// // UpdateStorageUsage updates the storage usage for a subscriber when they upload or delete files
// // It updates both the subscriber store and the NIP-88 event
// func (m *SubscriptionManager) UpdateStorageUsage(npub string, newBytes int64) error {
// 	// Step 1: Get current NIP-88 event to check current storage usage
// 	events, err := m.store.QueryEvents(nostr.Filter{
// 		Kinds: []int{764},
// 		Tags: nostr.TagMap{
// 			"p": []string{npub},
// 		},
// 		Limit: 1,
// 	})
// 	if err != nil || len(events) == 0 {
// 		return fmt.Errorf("no NIP-88 event found for user")
// 	}

// 	currentEvent := events[0]

// 	// Step 2: Get current storage information
// 	storageInfo, err := m.extractStorageInfo(currentEvent)
// 	if err != nil {
// 		return fmt.Errorf("failed to extract storage info: %v", err)
// 	}

// 	// Step 3: Validate new storage usage
// 	newUsedBytes := storageInfo.UsedBytes + newBytes
// 	if newUsedBytes > storageInfo.TotalBytes {
// 		return fmt.Errorf("storage limit exceeded: would use %d of %d bytes",
// 			newUsedBytes, storageInfo.TotalBytes)
// 	}

// 	// Step 4: Update storage tracking
// 	storageInfo.UsedBytes = newUsedBytes
// 	storageInfo.UpdatedAt = time.Now()

// 	// Step 5: Get subscriber record
// 	subscriber, err := m.store.GetSubscriber(npub)
// 	if err != nil {
// 		return fmt.Errorf("failed to get subscriber: %v", err)
// 	}

// 	// Step 6: Update storage usage in subscriber store
// 	if err := m.store.GetSubscriberStore().UpdateStorageUsage(npub, newBytes); err != nil {
// 		return fmt.Errorf("failed to update storage usage: %v", err)
// 	}

// 	// Step 7: Get current subscription info from event tags
// 	activeTier := ""
// 	var expirationDate time.Time
// 	for _, tag := range currentEvent.Tags {
// 		if tag[0] == "active_subscription" && len(tag) >= 3 {
// 			activeTier = tag[1]
// 			timestamp, _ := strconv.ParseInt(tag[2], 10, 64)
// 			expirationDate = time.Unix(timestamp, 0)
// 			break
// 		}
// 	}

// 	// Step 8: Update NIP-88 event with new storage information
// 	return m.createOrUpdateNIP88Event(subscriber, activeTier, expirationDate, &storageInfo)
// }

// // Private helper methods

// // getOrCreateSubscriber retrieves an existing subscriber or creates a new one
// func (m *SubscriptionManager) getOrCreateSubscriber(npub string) (*lib.Subscriber, error) {
// 	subscriber, err := m.store.GetSubscriber(npub)
// 	if err == nil {
// 		return subscriber, nil
// 	}

// 	// Allocate a unique Bitcoin address for the new subscriber
// 	address, err := m.store.GetSubscriberStore().AllocateBitcoinAddress(npub)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to allocate Bitcoin address: %v", err)
// 	}

// 	// Create new subscriber with allocated address
// 	newSubscriber := &lib.Subscriber{
// 		Npub:              npub,
// 		Tier:              "",
// 		StartDate:         time.Time{},
// 		EndDate:           time.Time{},
// 		Address:           address.Address, // Using allocated address
// 		LastTransactionID: "",
// 	}

// 	if err := m.store.SaveSubscriber(newSubscriber); err != nil {
// 		return nil, err
// 	}

// 	log.Printf("Created new subscriber %s with allocated address %s", npub, address.Address)
// 	return newSubscriber, nil
// }

// // createOrUpdateNIP88Event creates or updates a subscriber's NIP-88 event
// func (m *SubscriptionManager) createOrUpdateNIP88Event(
// 	subscriber *lib.Subscriber,
// 	activeTier string,
// 	expirationDate time.Time,
// 	storageInfo *StorageInfo,
// ) error {
// 	// Step 1: Delete existing NIP-88 event if it exists
// 	existingEvents, err := m.store.QueryEvents(nostr.Filter{
// 		Kinds: []int{764},
// 		Tags: nostr.TagMap{
// 			"p": []string{subscriber.Npub},
// 		},
// 		Limit: 1,
// 	})
// 	if err == nil && len(existingEvents) > 0 {
// 		if err := m.store.DeleteEvent(existingEvents[0].ID); err != nil {
// 			log.Printf("Warning: failed to delete existing NIP-88 event: %v", err)
// 		}
// 	}

// 	// Step 2: Prepare event tags with "paid" or "active" status
// 	subscriptionStatus := "inactive"
// 	if activeTier != "" {
// 		subscriptionStatus = "active"
// 	}
// 	tags := []nostr.Tag{
// 		{"subscription_duration", "1 month"},
// 		{"p", subscriber.Npub},
// 		{"subscription_status", subscriptionStatus},
// 		{"relay_bitcoin_address", subscriber.Address},
// 		{"relay_dht_key", m.relayDHTKey},
// 		{"storage", fmt.Sprintf("%d", storageInfo.UsedBytes),
// 			fmt.Sprintf("%d", storageInfo.TotalBytes),
// 			fmt.Sprintf("%d", storageInfo.UpdatedAt.Unix())},
// 	}
// 	if activeTier != "" {
// 		tags = append(tags, nostr.Tag{
// 			"active_subscription", activeTier, fmt.Sprintf("%d", expirationDate.Unix()),
// 		})
// 	}

// 	// Additional subscription tiers
// 	for _, tier := range m.subscriptionTiers {
// 		tags = append(tags, nostr.Tag{"subscription-tier", tier.DataLimit, tier.Price})
// 	}

// 	// Step 3: Create and sign the new event
// 	event := &nostr.Event{
// 		PubKey:    hex.EncodeToString(m.relayPrivateKey.PubKey().SerializeCompressed()),
// 		CreatedAt: nostr.Timestamp(time.Now().Unix()),
// 		Kind:      764,
// 		Tags:      tags,
// 		Content:   "",
// 	}
// 	serializedEvent := event.Serialize()
// 	hash := sha256.Sum256(serializedEvent)
// 	event.ID = hex.EncodeToString(hash[:])

// 	sig, err := schnorr.Sign(m.relayPrivateKey, hash[:])
// 	if err != nil {
// 		return fmt.Errorf("error signing event: %v", err)
// 	}
// 	event.Sig = hex.EncodeToString(sig.Serialize())

// 	// Step 4: Store the event and log status
// 	log.Printf("Storing NIP-88 event with ID: %s and subscription status: %s", event.ID, subscriptionStatus)
// 	return m.store.StoreEvent(event)
// }

// // Helper functions

// // findMatchingTier finds the highest tier that matches the payment amount
// func (m *SubscriptionManager) findMatchingTier(amountSats int64) (*lib.SubscriptionTier, error) {
// 	var bestMatch *lib.SubscriptionTier
// 	var bestPrice int64

// 	for _, tier := range m.subscriptionTiers {
// 		price := m.parseSats(tier.Price)
// 		if amountSats >= price && price > bestPrice {
// 			tierCopy := tier
// 			bestMatch = &tierCopy
// 			bestPrice = price
// 		}
// 	}

// 	if bestMatch == nil {
// 		return nil, fmt.Errorf("no matching tier for payment of %d sats", amountSats)
// 	}

// 	return bestMatch, nil
// }

// // calculateEndDate determines the subscription end date
// func (m *SubscriptionManager) calculateEndDate(currentEnd time.Time) time.Time {
// 	if time.Now().Before(currentEnd) {
// 		return currentEnd.AddDate(0, 1, 0) // Extend by 1 month
// 	}
// 	return time.Now().AddDate(0, 1, 0) // Start new 1 month period
// }

// // calculateStorageLimit converts tier string to bytes
// func (m *SubscriptionManager) calculateStorageLimit(tier string) int64 {
// 	switch tier {
// 	case "1 GB per month":
// 		return 1 * 1024 * 1024 * 1024
// 	case "5 GB per month":
// 		return 5 * 1024 * 1024 * 1024
// 	case "10 GB per month":
// 		return 10 * 1024 * 1024 * 1024
// 	default:
// 		return 0
// 	}
// }

// // getSubscriptionStatus returns the subscription status string
// // func (m *SubscriptionManager) getSubscriptionStatus(activeTier string) string {
// // 	if activeTier == "" {
// // 		return "inactive"
// // 	}
// // 	return "active"
// // }

// // parseSats converts price string to satoshis
// func (m *SubscriptionManager) parseSats(price string) int64 {
// 	var sats int64
// 	fmt.Sscanf(price, "%d", &sats)
// 	return sats
// }

// // extractStorageInfo gets storage information from NIP-88 event
// func (m *SubscriptionManager) extractStorageInfo(event *nostr.Event) (StorageInfo, error) {
// 	var info StorageInfo

// 	for _, tag := range event.Tags {
// 		if tag[0] == "storage" && len(tag) >= 4 {
// 			used, err := strconv.ParseInt(tag[1], 10, 64)
// 			if err != nil {
// 				return info, fmt.Errorf("invalid used storage value: %v", err)
// 			}

// 			total, err := strconv.ParseInt(tag[2], 10, 64)
// 			if err != nil {
// 				return info, fmt.Errorf("invalid total storage value: %v", err)
// 			}

// 			updated, err := strconv.ParseInt(tag[3], 10, 64)
// 			if err != nil {
// 				return info, fmt.Errorf("invalid update timestamp: %v", err)
// 			}

// 			info.UsedBytes = used
// 			info.TotalBytes = total
// 			info.UpdatedAt = time.Unix(updated, 0)
// 			return info, nil
// 		}
// 	}

// 	// Return zero values if no storage tag found
// 	return StorageInfo{
// 		UsedBytes:  0,
// 		TotalBytes: 0,
// 		UpdatedAt:  time.Now(),
// 	}, nil
// }
