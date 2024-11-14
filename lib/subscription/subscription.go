// subscription.go

package subscription

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/golang-jwt/jwt/v4"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	"github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
)

// Address status constants
const (
	AddressStatusAvailable = "available"
	AddressStatusAllocated = "allocated"
	AddressStatusUsed      = "used"
)

// StorageInfo tracks current storage usage information for a subscriber
type StorageInfo struct {
	UsedBytes  int64     // Current bytes used by the subscriber
	TotalBytes int64     // Total bytes allocated to the subscriber
	UpdatedAt  time.Time // Last time storage information was updated
}

// SubscriptionManager handles all subscription-related operations through NIP-88 events
type SubscriptionManager struct {
	store             stores.Store           // Interface to handle minimal state storage
	relayPrivateKey   *btcec.PrivateKey      // Relay's private key for signing events
	relayDHTKey       string                 // Relay's DHT key
	subscriptionTiers []lib.SubscriptionTier // Available subscription tiers
}

// NewSubscriptionManager creates and initializes a new subscription manager
func NewSubscriptionManager(
	store stores.Store,
	relayPrivKey *btcec.PrivateKey,
	relayDHTKey string,
	tiers []lib.SubscriptionTier,
) *SubscriptionManager {
	return &SubscriptionManager{
		store:             store,
		relayPrivateKey:   relayPrivKey,
		relayDHTKey:       relayDHTKey,
		subscriptionTiers: tiers,
	}
}

// InitializeSubscriber creates a new subscriber or retrieves an existing one and creates their initial NIP-88 event.
func (m *SubscriptionManager) InitializeSubscriber(npub string) error {

	// Run address pool check in background
	go func() {
		if err := m.checkAddressPoolStatus(); err != nil {
			log.Printf("Warning: error checking address pool status: %v", err)
		}
	}()

	// Step 1: Allocate a Bitcoin address (if necessary)
	address, err := m.store.GetSubscriberStore().AllocateBitcoinAddress(npub)
	if err != nil {
		return fmt.Errorf("failed to allocate Bitcoin address: %v", err)
	}

	// Step 2: Create initial NIP-88 event with zero storage usage
	storageInfo := StorageInfo{
		UsedBytes:  0,
		TotalBytes: 0,
		UpdatedAt:  time.Now(),
	}

	// Step 3: Create the NIP-88 event
	return m.createNIP88EventIfNotExists(&lib.Subscriber{
		Npub:    npub,
		Address: address.Address,
	}, "", time.Time{}, &storageInfo)
}

// ProcessPayment handles a new subscription payment by updating the NIP-88 event and other relevant data
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

	// Step 2: Fetch NIP-88 event data to retrieve subscriber information
	events, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{888},
		Tags: nostr.TagMap{
			"p": []string{npub},
		},
		Limit: 1,
	})
	if err != nil || len(events) == 0 {
		return fmt.Errorf("no NIP-88 event found for user")
	}
	currentEvent := events[0]
	storageInfo, err := m.extractStorageInfo(currentEvent)
	if err != nil {
		return fmt.Errorf("failed to extract storage info: %v", err)
	}

	// Step 3: Calculate subscription period dates and storage limit
	createdAt := time.Unix(int64(currentEvent.CreatedAt), 0)
	endDate := m.calculateEndDate(createdAt)
	storageLimit := m.calculateStorageLimit(tier.DataLimit)

	// Step 4: Update storage information for new subscription
	storageInfo.TotalBytes = storageLimit
	storageInfo.UpdatedAt = time.Now()

	// Step 5: Update NIP-88 event with new subscription details
	address := getTagValue(currentEvent.Tags, "relay_bitcoin_address")
	if err := m.createOrUpdateNIP88Event(&lib.Subscriber{
		Npub:    npub,
		Address: address,
	}, tier.DataLimit, endDate, &storageInfo); err != nil {
		return fmt.Errorf("error updating NIP-88 event: %v", err)
	}

	log.Printf("Processed payment for subscriber %s with tier %s", npub, tier.DataLimit)
	return nil
}

// UpdateStorageUsage updates the storage usage for a subscriber by modifying the relevant NIP-88 event
func (m *SubscriptionManager) UpdateStorageUsage(npub string, newBytes int64) error {
	// Fetch current NIP-88 event data
	events, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{888},
		Tags: nostr.TagMap{
			"p": []string{npub},
		},
		Limit: 1,
	})
	if err != nil || len(events) == 0 {
		return fmt.Errorf("no NIP-88 event found for user")
	}
	currentEvent := events[0]

	// Extract and update storage information
	storageInfo, err := m.extractStorageInfo(currentEvent)
	if err != nil {
		return fmt.Errorf("failed to extract storage info: %v", err)
	}
	newUsedBytes := storageInfo.UsedBytes + newBytes
	if newUsedBytes > storageInfo.TotalBytes {
		return fmt.Errorf("storage limit exceeded: would use %d of %d bytes", newUsedBytes, storageInfo.TotalBytes)
	}
	storageInfo.UsedBytes = newUsedBytes
	storageInfo.UpdatedAt = time.Now()

	// Replacing `GetValue` and `GetUnixValue` calls with utility functions
	activeSubscription := getTagValue(currentEvent.Tags, "active_subscription")
	expirationTime := time.Unix(getTagUnixValue(currentEvent.Tags, "active_subscription"), 0)

	// Update NIP-88 event
	return m.createOrUpdateNIP88Event(&lib.Subscriber{
		Npub: npub,
	}, activeSubscription, expirationTime, &storageInfo)
}

// CheckStorageAvailability checks if a subscriber has enough available storage for a given number of bytes.
// It retrieves storage data from the user's NIP-88 event and validates against their current usage and limits.
func (m *SubscriptionManager) CheckStorageAvailability(npub string, requestedBytes int64) error {
	// Step 1: Fetch the user's NIP-88 event
	events, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{888},
		Tags: nostr.TagMap{
			"p": []string{npub},
		},
		Limit: 1,
	})
	if err != nil || len(events) == 0 {
		return fmt.Errorf("no NIP-88 event found for user: %s", npub)
	}
	currentEvent := events[0]

	// Step 2: Extract storage information from the event
	storageInfo, err := m.extractStorageInfo(currentEvent)
	if err != nil {
		return fmt.Errorf("failed to extract storage info: %v", err)
	}

	// Step 3: Check if the user has enough available storage
	newUsage := storageInfo.UsedBytes + requestedBytes
	if newUsage > storageInfo.TotalBytes {
		return fmt.Errorf("storage limit exceeded: would use %d of %d bytes", newUsage, storageInfo.TotalBytes)
	}

	// Step 4: Optionally, check if the subscription is still active
	for _, tag := range currentEvent.Tags {
		if tag[0] == "active_subscription" && len(tag) >= 3 {
			expirationTimestamp, err := strconv.ParseInt(tag[2], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid subscription expiration timestamp: %v", err)
			}
			expirationDate := time.Unix(expirationTimestamp, 0)
			if time.Now().After(expirationDate) {
				return fmt.Errorf("subscription has expired")
			}
			break
		}
	}

	return nil
}

// createOrUpdateNIP88Event creates or updates a subscriber's NIP-88 event
func (m *SubscriptionManager) createOrUpdateNIP88Event(
	subscriber *lib.Subscriber,
	activeTier string,
	expirationDate time.Time,
	storageInfo *StorageInfo,
) error {
	// Delete existing NIP-88 event if it exists
	existingEvents, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{888},
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

	// Prepare tags and create a new NIP-88 event
	tags := []nostr.Tag{
		{"subscription_duration", "1 month"},
		{"p", subscriber.Npub},
		{"subscription_status", m.getSubscriptionStatus(activeTier)},
		{"relay_bitcoin_address", subscriber.Address},
		{"relay_dht_key", m.relayDHTKey},
		{"storage", fmt.Sprintf("%d", storageInfo.UsedBytes), fmt.Sprintf("%d", storageInfo.TotalBytes), fmt.Sprintf("%d", storageInfo.UpdatedAt.Unix())},
	}

	// Fetch and add subscription_tier tags based on values from Viper
	rawTiers := viper.Get("subscription_tiers")
	if rawTiers != nil {
		if tiers, ok := rawTiers.([]interface{}); ok {
			for _, tier := range tiers {
				if tierMap, ok := tier.(map[string]interface{}); ok {
					dataLimit, okDataLimit := tierMap["data_limit"].(string)
					price, okPrice := tierMap["price"].(string)
					if okDataLimit && okPrice {
						priceInt, err := strconv.Atoi(price) // Convert string price to integer
						if err != nil {
							log.Printf("error converting price %s to integer: %v", price, err)
							continue
						}
						tags = append(tags, nostr.Tag{"subscription_tier", dataLimit, strconv.Itoa(priceInt)})
					} else {
						log.Printf("invalid data structure for tier: %v", tierMap)
					}
				} else {
					log.Printf("error asserting tier to map[string]interface{}: %v", tier)
				}
			}
		} else {
			log.Printf("error asserting subscription_tiers to []interface{}: %v", rawTiers)
		}
	}

	if activeTier != "" {
		tags = append(tags, nostr.Tag{
			"active_subscription", activeTier, fmt.Sprintf("%d", expirationDate.Unix()),
		})
	}

	event := &nostr.Event{
		PubKey:    hex.EncodeToString(m.relayPrivateKey.PubKey().SerializeCompressed()),
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      888,
		Tags:      tags,
		Content:   "",
	}

	// Sign and store the event
	serializedEvent := event.Serialize()
	hash := sha256.Sum256(serializedEvent)
	event.ID = hex.EncodeToString(hash[:])
	sig, err := schnorr.Sign(m.relayPrivateKey, hash[:])
	if err != nil {
		return fmt.Errorf("error signing event: %v", err)
	}
	event.Sig = hex.EncodeToString(sig.Serialize())

	return m.store.StoreEvent(event)
}

// createNIP88EventIfNotExists creates a new NIP-88 event for a subscriber if none exists
func (m *SubscriptionManager) createNIP88EventIfNotExists(
	subscriber *lib.Subscriber,
	activeTier string,
	expirationDate time.Time,
	storageInfo *StorageInfo,
) error {
	// Check if an existing NIP-88 event for the subscriber already exists
	existingEvents, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{888}, // Assuming 888 is the NIP-88 event kind
		Tags: nostr.TagMap{
			"p": []string{subscriber.Npub},
		},
		Limit: 1,
	})
	if err != nil {
		return fmt.Errorf("error querying existing NIP-88 events: %v", err)
	}

	// If an existing event is found, we skip creation
	if len(existingEvents) > 0 {
		log.Printf("NIP-88 event already exists for subscriber %s, skipping creation", subscriber.Npub)
		return nil
	}

	// Prepare tags for the new NIP-88 event
	tags := []nostr.Tag{
		{"subscription_duration", "1 month"},
		{"p", subscriber.Npub},
		{"subscription_status", m.getSubscriptionStatus(activeTier)},
		{"relay_bitcoin_address", subscriber.Address},
		{"relay_dht_key", m.relayDHTKey},
		{"storage", fmt.Sprintf("%d", storageInfo.UsedBytes), fmt.Sprintf("%d", storageInfo.TotalBytes), fmt.Sprintf("%d", storageInfo.UpdatedAt.Unix())},
	}

	// Fetch and add subscription_tier tags based on the values from Viper
	rawTiers := viper.Get("subscription_tiers")
	if rawTiers != nil {
		if tiers, ok := rawTiers.([]interface{}); ok {
			for _, tier := range tiers {
				if tierMap, ok := tier.(map[string]interface{}); ok {
					dataLimit, okDataLimit := tierMap["data_limit"].(string)
					price, okPrice := tierMap["price"].(string)
					if okDataLimit && okPrice {
						priceInt, err := strconv.Atoi(price) // Convert string price to integer
						if err != nil {
							log.Printf("error converting price %s to integer: %v", price, err)
							continue
						}
						tags = append(tags, nostr.Tag{"subscription_tier", dataLimit, strconv.Itoa(priceInt)})
					} else {
						log.Printf("invalid data structure for tier: %v", tierMap)
					}
				} else {
					log.Printf("error asserting tier to map[string]interface{}: %v", tier)
				}
			}
		} else {
			log.Printf("error asserting subscription_tiers to []interface{}: %v", rawTiers)
		}
	}

	if activeTier != "" {
		tags = append(tags, nostr.Tag{
			"active_subscription", activeTier, fmt.Sprintf("%d", expirationDate.Unix()),
		})
	}

	event := &nostr.Event{
		PubKey:    hex.EncodeToString(m.relayPrivateKey.PubKey().SerializeCompressed()),
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      888,
		Tags:      tags,
		Content:   "",
	}

	// Sign and store the new event
	serializedEvent := event.Serialize()
	hash := sha256.Sum256(serializedEvent)
	event.ID = hex.EncodeToString(hash[:])
	sig, err := schnorr.Sign(m.relayPrivateKey, hash[:])
	if err != nil {
		return fmt.Errorf("error signing event: %v", err)
	}
	event.Sig = hex.EncodeToString(sig.Serialize())

	return m.store.StoreEvent(event)
}

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

// checkAddressPoolStatus checks if we need to generate more addresses
func (m *SubscriptionManager) checkAddressPoolStatus() error {
	availableCount, err := m.store.GetSubscriberStore().CountAvailableAddresses()
	if err != nil {
		return fmt.Errorf("failed to count available addresses: %v", err)
	}

	log.Println("Available count: ", availableCount)

	// If we have less than 50% of addresses available, request more
	if availableCount < 50 {
		log.Printf("Address pool running low (%d available). Requesting 100 new addresses", availableCount)
		return m.requestNewAddresses(20)
	}

	return nil
}

// requestNewAddresses sends a request to the wallet to generate new addresses
func (m *SubscriptionManager) requestNewAddresses(count int) error {
	// Get API key from config
	apiKey := viper.GetString("wallet_api_key")

	// Generate JWT token using API key
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"api_key": apiKey,
		"exp":     time.Now().Add(time.Hour * 24).Unix(),
		"iat":     time.Now().Unix(),
	})

	// Sign token with API key
	tokenString, err := token.SignedString([]byte(apiKey))
	if err != nil {
		return fmt.Errorf("failed to generate token: %v", err)
	}

	reqBody := map[string]interface{}{
		"count": count,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %v", err)
	}

	// Prepare HMAC signature
	timestamp := time.Now().UTC().Format(time.RFC3339)
	message := apiKey + timestamp + string(jsonData)
	h := hmac.New(sha256.New, []byte(apiKey))
	h.Write([]byte(message))
	signature := hex.EncodeToString(h.Sum(nil))

	// Create request
	req, err := http.NewRequest("POST",
		"http://localhost:9003/generate-addresses",
		bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	// Add all required headers including the new JWT
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tokenString))
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Signature", signature)

	// Send request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wallet service returned status: %v", resp.Status)
	}

	// Just decode the response to verify it's valid JSON but we don't need to process it
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %v", err)
	}

	log.Printf("Successfully requested generation of %d addresses", count)
	return nil
}

// getSubscriptionStatus returns the subscription status string
func (m *SubscriptionManager) getSubscriptionStatus(activeTier string) string {
	if activeTier == "" {
		return "inactive"
	}
	return "active"
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

// calculateEndDate determines the subscription end date
func (m *SubscriptionManager) calculateEndDate(currentEnd time.Time) time.Time {
	if time.Now().Before(currentEnd) {
		return currentEnd.AddDate(0, 1, 0) // Extend by 1 month
	}
	return time.Now().AddDate(0, 1, 0) // Start new 1 month period
}

func getTagValue(tags []nostr.Tag, key string) string {
	for _, tag := range tags {
		if len(tag) > 1 && tag[0] == key {
			return tag[1]
		}
	}
	return ""
}

func getTagUnixValue(tags []nostr.Tag, key string) int64 {
	for _, tag := range tags {
		if len(tag) > 2 && tag[0] == key {
			unixTime, err := strconv.ParseInt(tag[2], 10, 64)
			if err == nil {
				return unixTime
			}
		}
	}
	return 0
}
