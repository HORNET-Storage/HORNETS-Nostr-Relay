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
	"strings"
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
	freeTierEnabled   bool
	freeTierLimit     string
}

// NewSubscriptionManager creates and initializes a new subscription manager
// NewSubscriptionManager creates and initializes a new subscription manager
func NewSubscriptionManager(
	store stores.Store,
	relayPrivKey *btcec.PrivateKey,
	relayDHTKey string,
	tiers []lib.SubscriptionTier,
) *SubscriptionManager {
	log.Printf("Initializing SubscriptionManager with tiers: %+v", tiers)

	// Log each tier in detail for debugging
	for i, tier := range tiers {
		log.Printf("DEBUG: Initial tier %d: DataLimit='%s', Price='%s'",
			i, tier.DataLimit, tier.Price)
	}

	// Load relay settings first to get free tier configuration
	var settings lib.RelaySettings
	if err := viper.UnmarshalKey("relay_settings", &settings); err != nil {
		log.Printf("Error loading relay settings: %v", err)
	}

	// Validate tiers data
	validTiers := make([]lib.SubscriptionTier, 0)

	// If free tier is enabled, add it as the first tier
	if settings.FreeTierEnabled && settings.FreeTierLimit != "" {
		freeTier := lib.SubscriptionTier{
			DataLimit: settings.FreeTierLimit,
			Price:     "0",
		}
		validTiers = append(validTiers, freeTier)
		log.Printf("Added free tier: DataLimit='%s'", settings.FreeTierLimit)
	}

	// Validate and add paid tiers
	for i, tier := range tiers {
		if tier.DataLimit == "" || tier.Price == "" {
			log.Printf("Warning: skipping tier %d with empty fields: DataLimit='%s', Price='%s'",
				i, tier.DataLimit, tier.Price)
			continue
		}
		// Skip if it's a free tier (price = "0") and we already have a free tier
		if tier.Price == "0" && settings.FreeTierEnabled {
			log.Printf("Skipping duplicate free tier at index %d", i)
			continue
		}
		validTiers = append(validTiers, tier)
		log.Printf("Validated tier %d: DataLimit='%s', Price='%s'",
			i, tier.DataLimit, tier.Price)
	}

	if len(validTiers) == 0 {
		log.Printf("Warning: no valid tiers found, checking relay settings directly")
		// Fallback to loading from settings directly
		if len(settings.SubscriptionTiers) > 0 {
			// If free tier is enabled, filter out any existing free tiers from settings
			if settings.FreeTierEnabled {
				for _, tier := range settings.SubscriptionTiers {
					if tier.Price != "0" {
						validTiers = append(validTiers, tier)
					}
				}
				// Add free tier at the beginning
				freeTier := lib.SubscriptionTier{
					DataLimit: settings.FreeTierLimit,
					Price:     "0",
				}
				validTiers = append([]lib.SubscriptionTier{freeTier}, validTiers...)
			} else {
				validTiers = settings.SubscriptionTiers
			}
			log.Printf("Loaded tiers from settings: %+v", validTiers)
		}
	}

	return &SubscriptionManager{
		store:             store,
		relayPrivateKey:   relayPrivKey,
		relayDHTKey:       relayDHTKey,
		subscriptionTiers: validTiers,
		freeTierEnabled:   settings.FreeTierEnabled,
		freeTierLimit:     settings.FreeTierLimit,
	}
}

// InitializeSubscriber creates a new subscriber or retrieves an existing one and creates their initial NIP-88 event.
func (m *SubscriptionManager) InitializeSubscriber(npub string) error {
	log.Printf("Initializing subscriber for npub: %s", npub)

	// Run address pool check in background
	go func() {
		if err := m.checkAddressPoolStatus(); err != nil {
			log.Printf("Warning: error checking address pool status: %v", err)
		}
	}()

	log.Println("Address Pool checked Going to allocate address")

	// Step 1: Allocate a Bitcoin address (if necessary)
	address, err := m.store.GetStatsStore().AllocateBitcoinAddress(npub)
	if err != nil {
		log.Printf("Error allocating bitcoin address: %v", err)
		return fmt.Errorf("failed to allocate Bitcoin address: %v", err)
	}
	log.Printf("Successfully allocated address: %s", address.Address)

	// Step 2: Create initial NIP-88 event with storage usage based on free tier
	storageInfo := StorageInfo{
		UsedBytes: 0,
		UpdatedAt: time.Now(),
	}

	// Set initial storage limit based on free tier status
	if m.freeTierEnabled {
		storageInfo.TotalBytes = m.calculateStorageLimit(m.freeTierLimit)
		log.Printf("Free tier enabled. Setting initial storage limit to %d bytes", storageInfo.TotalBytes)
	} else {
		storageInfo.TotalBytes = 0
		log.Printf("Free tier disabled. Setting initial storage limit to 0 bytes")
	}

	// Step 3: Create the NIP-88 event with appropriate tier information
	var tierLimit string
	var expirationDate time.Time

	if m.freeTierEnabled {
		tierLimit = m.freeTierLimit
		expirationDate = time.Now().AddDate(0, 1, 0) // 1 month from now
		log.Printf("Setting free tier limit: %s with expiration: %v", tierLimit, expirationDate)
	}

	// Step 4: Create the NIP-88 event
	err = m.createNIP88EventIfNotExists(&lib.Subscriber{
		Npub:    npub,
		Address: address.Address,
	}, tierLimit, expirationDate, &storageInfo)

	if err != nil {
		log.Printf("Error creating NIP-88 event: %v", err)
		return err
	}

	log.Printf("Successfully initialized subscriber %s with free tier status: %v", npub, m.freeTierEnabled)
	return nil
}

// ProcessPayment handles a new subscription payment by updating the NIP-88 event and other relevant data
func (m *SubscriptionManager) ProcessPayment(
	npub string,
	transactionID string,
	amountSats int64,
) error {
	log.Printf("Processing payment of %d sats for %s", amountSats, npub)

	// Validate payment amount
	if amountSats <= 0 {
		return fmt.Errorf("invalid payment amount: %d", amountSats)
	}

	// Get current credit and add to payment for processing
	var totalAmount = amountSats
	creditSats, err := m.store.GetStatsStore().GetSubscriberCredit(npub)
	if err == nil && creditSats > 0 {
		totalAmount = amountSats + creditSats
		log.Printf("Adding existing credit of %d sats to payment (total: %d)",
			creditSats, totalAmount)
	}

	// Get available tiers and find the highest tier
	var highestTier *lib.SubscriptionTier
	var highestTierPrice int64

	for _, t := range m.subscriptionTiers {
		if t.Price != "0" { // Skip free tier
			price := m.parseSats(t.Price)
			if price > highestTierPrice {
				highestTierPrice = price
				highestTier = &lib.SubscriptionTier{
					DataLimit: t.DataLimit,
					Price:     t.Price,
				}
			}
		}
	}

	// Handle payment greater than the highest tier price
	if highestTier != nil && totalAmount >= highestTierPrice && totalAmount > highestTierPrice {
		// If we have credit, reset it since we're using it
		if creditSats > 0 {
			if err := m.store.GetStatsStore().UpdateSubscriberCredit(npub, 0); err != nil {
				log.Printf("Warning: failed to reset credit: %v", err)
			}
		}
		return m.processHighTierPayment(npub, transactionID, totalAmount, highestTier)
	}

	// Try to find matching tier
	tier, err := m.findMatchingTier(totalAmount)
	if err != nil {
		// No matching tier found, add to credit
		if strings.Contains(err.Error(), "no matching tier") {
			// If we already had credit, add the new payment to it
			newCredit := creditSats + amountSats

			if err := m.store.GetStatsStore().UpdateSubscriberCredit(npub, newCredit); err != nil {
				return fmt.Errorf("failed to update credit: %v", err)
			}

			log.Printf("Added %d sats to credit for %s (total credit: %d)",
				amountSats, npub, newCredit)

			// Update the NIP-88 event to reflect the new credit amount
			events, err := m.store.QueryEvents(nostr.Filter{
				Kinds: []int{888},
				Tags:  nostr.TagMap{"p": []string{npub}},
				Limit: 1,
			})
			if err == nil && len(events) > 0 {
				currentEvent := events[0]

				// Extract current info
				storageInfo, err := m.extractStorageInfo(currentEvent)
				if err != nil {
					log.Printf("Warning: could not extract storage info: %v", err)
					return nil
				}

				// Get address and current tier information
				address := getTagValue(currentEvent.Tags, "relay_bitcoin_address")
				activeTier := getTagValue(currentEvent.Tags, "active_subscription")

				// Get expiration date if any
				var expirationDate time.Time
				if expirationUnix := getTagUnixValue(currentEvent.Tags, "active_subscription"); expirationUnix > 0 {
					expirationDate = time.Unix(expirationUnix, 0)
				} else if m.freeTierEnabled {
					// Set default expiration for free tier
					expirationDate = time.Now().AddDate(0, 1, 0)
				}

				// Update the NIP-88 event to reflect the new credit
				if err := m.createOrUpdateNIP88Event(&lib.Subscriber{
					Npub:    npub,
					Address: address,
				}, activeTier, expirationDate, &storageInfo); err != nil {
					log.Printf("Warning: failed to update NIP-88 event with credit: %v", err)
				} else {
					log.Printf("Updated NIP-88 event for %s with credit: %d sats", npub, newCredit)
				}
			}

			return nil
		}
		return err
	}

	// We have a matching tier - reset credit if we used it
	if creditSats > 0 {
		if err := m.store.GetStatsStore().UpdateSubscriberCredit(npub, 0); err != nil {
			log.Printf("Warning: failed to reset credit after using: %v", err)
		}
	}

	log.Printf("Found matching tier: %+v", tier)

	// Fetch current NIP-88 event to get existing state
	events, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{888},
		Tags:  nostr.TagMap{"p": []string{npub}},
		Limit: 1,
	})
	if err != nil || len(events) == 0 {
		return fmt.Errorf("no NIP-88 event found for user")
	}
	currentEvent := events[0]

	// Extract current storage info
	storageInfo, err := m.extractStorageInfo(currentEvent)
	if err != nil {
		return fmt.Errorf("failed to extract storage info: %v", err)
	}

	// If free tier is enabled, ensure paid tier provides more storage
	if m.freeTierEnabled {
		freeTierBytes := m.calculateStorageLimit(m.freeTierLimit)
		paidTierBytes := m.calculateStorageLimit(tier.DataLimit)

		if paidTierBytes <= freeTierBytes {
			return fmt.Errorf("paid tier (%s) must provide more storage than free tier (%s)",
				tier.DataLimit, m.freeTierLimit)
		}
		log.Printf("Paid tier provides more storage than free tier (%d > %d bytes)",
			paidTierBytes, freeTierBytes)
	}

	// Get current expiration date from event
	expirationUnix := getTagUnixValue(currentEvent.Tags, "active_subscription")
	existingExpiration := time.Time{}
	if expirationUnix > 0 {
		existingExpiration = time.Unix(expirationUnix, 0)
	}

	// Always accumulate storage capacity for paid tiers
	prevBytes := storageInfo.TotalBytes
	newTierBytes := m.calculateStorageLimit(tier.DataLimit)

	// Add new tier capacity to existing capacity
	storageInfo.TotalBytes += newTierBytes
	log.Printf("Accumulating storage: adding %d bytes to existing %d bytes (new total: %d bytes)",
		newTierBytes, prevBytes, storageInfo.TotalBytes)

	// Calculate new expiration date - add one month from current expiration
	// If existing subscription is valid, extend it by 1 month
	var endDate time.Time
	if existingExpiration.After(time.Now()) {
		endDate = existingExpiration.AddDate(0, 1, 0)
		log.Printf("Extending subscription expiration from %s to %s",
			existingExpiration.Format("2006-01-02"), endDate.Format("2006-01-02"))
	} else {
		// If expired or no previous subscription, start fresh
		endDate = time.Now().AddDate(0, 1, 0)
		log.Printf("Setting new subscription expiration to %s", endDate.Format("2006-01-02"))
	}

	storageInfo.UpdatedAt = time.Now()

	// Get address from current event
	address := getTagValue(currentEvent.Tags, "relay_bitcoin_address")

	// Update the NIP-88 event
	err = m.createOrUpdateNIP88Event(&lib.Subscriber{
		Npub:    npub,
		Address: address,
	}, tier.DataLimit, endDate, &storageInfo)

	if err != nil {
		return fmt.Errorf("failed to update NIP-88 event: %v", err)
	}

	// Verify the update
	updatedEvents, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{888},
		Tags:  nostr.TagMap{"p": []string{npub}},
		Limit: 1,
	})
	if err != nil || len(updatedEvents) == 0 {
		log.Printf("Warning: couldn't verify NIP-88 event update")
	} else {
		log.Printf("Updated NIP-88 event status: %s",
			getTagValue(updatedEvents[0].Tags, "subscription_status"))
	}

	// Add transaction processing log
	log.Printf("Successfully processed payment for %s: %d sats for tier %s",
		npub, amountSats, tier.DataLimit)

	return nil
}

// processHighTierPayment handles payments that exceed the highest tier price by extending
// the subscription period and crediting any remainder
func (m *SubscriptionManager) processHighTierPayment(
	npub string,
	_ string, // transactionID not used but kept for API consistency
	amountSats int64,
	highestTier *lib.SubscriptionTier,
) error {
	// Fetch current NIP-88 event to get existing state
	events, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{888},
		Tags:  nostr.TagMap{"p": []string{npub}},
		Limit: 1,
	})
	if err != nil || len(events) == 0 {
		return fmt.Errorf("no NIP-88 event found for user")
	}
	currentEvent := events[0]

	// Extract current storage info and address
	storageInfo, err := m.extractStorageInfo(currentEvent)
	if err != nil {
		return fmt.Errorf("failed to extract storage info: %v", err)
	}

	address := getTagValue(currentEvent.Tags, "relay_bitcoin_address")

	// Calculate full periods and remainder
	tierPrice, _ := strconv.ParseInt(highestTier.Price, 10, 64)
	fullPeriods := amountSats / tierPrice
	remainder := amountSats % tierPrice

	if fullPeriods < 1 {
		fullPeriods = 1 // Ensure at least one period
	}

	// Set highest tier storage limit
	prevBytes := storageInfo.TotalBytes
	storageInfo.TotalBytes = m.calculateStorageLimit(highestTier.DataLimit)
	storageInfo.UpdatedAt = time.Now()

	log.Printf("Upgrading storage from %d to %d bytes (tier: %s)",
		prevBytes, storageInfo.TotalBytes, highestTier.DataLimit)

	// Calculate end date based on multiple periods
	var endDate time.Time

	// If subscription hasn't expired, extend from current end date
	if existingEndDate := getTagUnixValue(currentEvent.Tags, "active_subscription"); existingEndDate > 0 {
		endTime := time.Unix(existingEndDate, 0)
		if endTime.After(time.Now()) {
			// Extend from current end date
			endDate = endTime.AddDate(0, int(fullPeriods), 0)
			log.Printf("Extending existing subscription by %d months (from %s to %s)",
				fullPeriods, endTime.Format("2006-01-02"), endDate.Format("2006-01-02"))
		} else {
			// Expired - start fresh from now
			endDate = time.Now().AddDate(0, int(fullPeriods), 0)
			log.Printf("Existing subscription expired, starting new %d month subscription",
				fullPeriods)
		}
	} else {
		// No existing subscription, start from now
		endDate = time.Now().AddDate(0, int(fullPeriods), 0)
		log.Printf("Starting new %d month subscription", fullPeriods)
	}

	// Update the NIP-88 event with extended period
	err = m.createOrUpdateNIP88Event(&lib.Subscriber{
		Npub:    npub,
		Address: address,
	}, highestTier.DataLimit, endDate, &storageInfo)

	if err != nil {
		return fmt.Errorf("failed to update NIP-88 event: %v", err)
	}

	// Credit remainder if any
	if remainder > 0 {
		if err := m.store.GetStatsStore().UpdateSubscriberCredit(npub, remainder); err != nil {
			log.Printf("Warning: failed to save remainder credit of %d sats: %v", remainder, err)
		} else {
			log.Printf("Credited remainder of %d sats to user account", remainder)
		}
	}

	log.Printf("Successfully processed high-tier payment: %d sats for %d months of %s tier",
		amountSats, fullPeriods, highestTier.DataLimit)

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

// createEvent is a helper function to create a NIP-88 event with common logic
func (m *SubscriptionManager) createEvent(
	subscriber *lib.Subscriber,
	activeTier string,
	expirationDate time.Time,
	storageInfo *StorageInfo,
) (*nostr.Event, error) {
	// Determine subscription status
	var status string
	if m.freeTierEnabled {
		status = "active" // Always active if free tier is enabled
	} else {
		status = m.getSubscriptionStatus(activeTier)
	}

	// Get current credit for the subscriber
	creditSats, err := m.store.GetStatsStore().GetSubscriberCredit(subscriber.Npub)
	if err != nil {
		log.Printf("Warning: could not get credit for subscriber: %v", err)
		creditSats = 0
	}

	// Prepare tags with free tier consideration
	tags := []nostr.Tag{
		{"subscription_duration", "1 month"},
		{"p", subscriber.Npub},
		{"subscription_status", status},
		{"relay_bitcoin_address", subscriber.Address},
		{"relay_dht_key", m.relayDHTKey},
		{"storage", fmt.Sprintf("%d", storageInfo.UsedBytes),
			fmt.Sprintf("%d", storageInfo.TotalBytes),
			fmt.Sprintf("%d", storageInfo.UpdatedAt.Unix())},
	}

	// Add credit information if there is any
	if creditSats > 0 {
		tags = append(tags, nostr.Tag{
			"credit", fmt.Sprintf("%d", creditSats),
		})
	}

	// Add tier information
	if activeTier != "" || m.freeTierEnabled {
		tierToUse := activeTier
		if m.freeTierEnabled && activeTier == "" {
			tierToUse = m.freeTierLimit
		}
		tags = append(tags, nostr.Tag{
			"active_subscription",
			tierToUse,
			fmt.Sprintf("%d", expirationDate.Unix()),
		})
	}

	// Create the event
	event := &nostr.Event{
		PubKey:    hex.EncodeToString(m.relayPrivateKey.PubKey().SerializeCompressed()),
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      888,
		Tags:      tags,
		Content:   "",
	}

	// Sign the event
	serializedEvent := event.Serialize()
	hash := sha256.Sum256(serializedEvent)
	event.ID = hex.EncodeToString(hash[:])
	sig, err := schnorr.Sign(m.relayPrivateKey, hash[:])
	if err != nil {
		return nil, fmt.Errorf("error signing event: %v", err)
	}
	event.Sig = hex.EncodeToString(sig.Serialize())

	return event, nil
}

// createOrUpdateNIP88Event creates or updates a subscriber's NIP-88 event
func (m *SubscriptionManager) createOrUpdateNIP88Event(
	subscriber *lib.Subscriber,
	activeTier string,
	expirationDate time.Time,
	storageInfo *StorageInfo,
) error {
	log.Printf("Creating/updating NIP-88 event for %s with tier %s",
		subscriber.Npub, activeTier)

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

	// Create new event
	event, err := m.createEvent(subscriber, activeTier, expirationDate, storageInfo)
	if err != nil {
		return err
	}

	return m.store.StoreEvent(event)
}

// createNIP88EventIfNotExists creates a new NIP-88 event for a subscriber if none exists
func (m *SubscriptionManager) createNIP88EventIfNotExists(
	subscriber *lib.Subscriber,
	activeTier string,
	expirationDate time.Time,
	storageInfo *StorageInfo,
) error {
	log.Printf("Checking for existing NIP-88 event for subscriber %s", subscriber.Npub)

	// Check for existing event
	existingEvents, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{888},
		Tags: nostr.TagMap{
			"p": []string{subscriber.Npub},
		},
		Limit: 1,
	})
	if err != nil {
		log.Printf("Error querying events: %v", err)
		return fmt.Errorf("error querying existing NIP-88 events: %v", err)
	}

	if len(existingEvents) > 0 {
		log.Printf("NIP-88 event already exists for subscriber %s, skipping creation", subscriber.Npub)
		return nil
	}

	log.Printf("Creating new NIP-88 event for subscriber %s", subscriber.Npub)
	log.Printf("Subscriber Address: %s", subscriber.Address)

	// Create new event
	event, err := m.createEvent(subscriber, activeTier, expirationDate, storageInfo)
	if err != nil {
		return err
	}

	log.Println("Subscription Event before storing: ", event.String())

	// Store and verify
	if err := m.store.StoreEvent(event); err != nil {
		return fmt.Errorf("error storing event: %v", err)
	}

	// Verification
	storedEvents, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{888},
		Tags: nostr.TagMap{
			"p": []string{subscriber.Npub},
		},
		Limit: 1,
	})
	if err != nil {
		log.Printf("Error verifying stored event: %v", err)
	} else {
		log.Printf("Verified stored event. Found %d events", len(storedEvents))
		if len(storedEvents) > 0 {
			log.Printf("Event details: %+v", storedEvents[0])
		}
	}

	return nil
}

// findMatchingTier finds the highest tier that matches the payment amount
func (m *SubscriptionManager) findMatchingTier(amountSats int64) (*lib.SubscriptionTier, error) {
	if len(m.subscriptionTiers) == 0 {
		// Reload tiers if empty
		var settings lib.RelaySettings
		if err := viper.UnmarshalKey("relay_settings", &settings); err != nil {
			return nil, fmt.Errorf("no tiers available and failed to load settings: %v", err)
		}
		m.subscriptionTiers = settings.SubscriptionTiers
	}

	log.Printf("Finding tier for %d sats among %d tiers: %+v",
		amountSats, len(m.subscriptionTiers), m.subscriptionTiers)

	var bestMatch *lib.SubscriptionTier
	var bestPrice int64

	for _, tier := range m.subscriptionTiers {
		if tier.DataLimit == "" || tier.Price == "" {
			log.Printf("Warning: skipping invalid tier: DataLimit='%s', Price='%s'",
				tier.DataLimit, tier.Price)
			continue
		}

		price := m.parseSats(tier.Price)
		log.Printf("Checking tier: DataLimit='%s', Price='%s' (%d sats)",
			tier.DataLimit, tier.Price, price)

		if amountSats >= price && price > bestPrice {
			bestMatch = &lib.SubscriptionTier{
				DataLimit: tier.DataLimit,
				Price:     tier.Price,
			}
			bestPrice = price
			log.Printf("New best match: DataLimit='%s', Price='%s'",
				bestMatch.DataLimit, bestMatch.Price)
		}
	}

	if bestMatch == nil {
		return nil, fmt.Errorf("no matching tier for payment of %d sats", amountSats)
	}

	log.Printf("Selected tier: DataLimit='%s', Price='%s'",
		bestMatch.DataLimit, bestMatch.Price)
	return bestMatch, nil
}

// parseSats converts price string to satoshis
func (m *SubscriptionManager) parseSats(price string) int64 {
	var sats int64
	if _, err := fmt.Sscanf(price, "%d", &sats); err != nil {
		log.Printf("Warning: could not parse price '%s': %v", price, err)
		return 0
	}
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
	availableCount, err := m.store.GetStatsStore().CountAvailableAddresses()
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
	log.Printf("Calculating storage limit for tier: %s", tier)

	var amount int64
	var unit string

	// Parse the string to get amount and unit (MB or GB)
	_, err := fmt.Sscanf(tier, "%d %2s per month", &amount, &unit)
	if err != nil {
		log.Printf("Warning: could not parse storage limit from tier '%s': %v", tier, err)
		return 0
	}

	// Convert to bytes based on unit
	var bytes int64
	switch strings.ToUpper(unit) {
	case "MB":
		bytes = amount * 1024 * 1024 // MB to bytes
		log.Printf("Converted %d MB to %d bytes", amount, bytes)
	case "GB":
		bytes = amount * 1024 * 1024 * 1024 // GB to bytes
		log.Printf("Converted %d GB to %d bytes", amount, bytes)
	default:
		log.Printf("Warning: unknown unit '%s' in tier '%s'", unit, tier)
		return 0
	}

	return bytes
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

// CheckAndUpdateSubscriptionEvent checks if a kind 888 event needs to be updated
// based on current free tier settings and updates it if necessary
func (m *SubscriptionManager) CheckAndUpdateSubscriptionEvent(event *nostr.Event) (*nostr.Event, error) {
	// Only process kind 888 events
	if event.Kind != 888 {
		return event, nil
	}

	log.Printf("Checking kind 888 event for updates based on free tier status")

	// Get the pubkey from the p tag
	var pubkey string
	for _, tag := range event.Tags {
		if tag[0] == "p" && len(tag) > 1 {
			pubkey = tag[1]
			break
		}
	}

	if pubkey == "" {
		return event, fmt.Errorf("no pubkey found in kind 888 event")
	}

	// Get subscription status
	var status string
	for _, tag := range event.Tags {
		if tag[0] == "subscription_status" && len(tag) > 1 {
			status = tag[1]
			break
		}
	}

	// Extract current storage info
	storageInfo, err := m.extractStorageInfo(event)
	if err != nil {
		return event, fmt.Errorf("failed to extract storage info: %v", err)
	}

	// Extract active subscription tier
	var activeTier string
	var expirationDate time.Time
	for _, tag := range event.Tags {
		if tag[0] == "active_subscription" && len(tag) > 1 {
			activeTier = tag[1]
			if len(tag) > 2 {
				expirationTimeUnix, err := strconv.ParseInt(tag[2], 10, 64)
				if err == nil {
					expirationDate = time.Unix(expirationTimeUnix, 0)
				}
			}
			break
		}
	}

	// Get bitcoin address from event
	var address string
	for _, tag := range event.Tags {
		if tag[0] == "relay_bitcoin_address" && len(tag) > 1 {
			address = tag[1]
			break
		}
	}

	// Load relay settings to check last update timestamp
	var settings lib.RelaySettings
	if err := viper.UnmarshalKey("relay_settings", &settings); err != nil {
		log.Printf("Error loading relay settings: %v", err)
		return event, nil // Return original event if we can't get settings
	}

	// Check if event was created/updated after the last settings change
	eventCreatedAt := time.Unix(int64(event.CreatedAt), 0)
	settingsUpdatedAt := time.Unix(settings.LastUpdated, 0)

	// If event is newer than the last settings change, it's already up to date
	if settings.LastUpdated > 0 && eventCreatedAt.After(settingsUpdatedAt) {
		log.Printf("Event was created/updated after the last settings change, no update needed")
		return event, nil
	}

	needsUpdate := false

	// Check if free tier is enabled and the event needs updating
	if m.freeTierEnabled {
		// If subscription is inactive, or total bytes is 0, or no active subscription
		if status == "inactive" || storageInfo.TotalBytes == 0 || activeTier == "" {
			log.Printf("Free tier is enabled, updating kind 888 event for pubkey %s", pubkey)
			needsUpdate = true
		} else if activeTier == m.freeTierLimit && status != "active" {
			// Free tier is active but status is not set correctly
			log.Printf("Free tier entry exists but status is not active, updating for pubkey %s", pubkey)
			needsUpdate = true
		} else if activeTier == m.freeTierLimit {
			// If it's a free tier and the free tier limit changed
			freeTierBytes := m.calculateStorageLimit(m.freeTierLimit)
			expectedFreeTierBytes := m.calculateStorageLimit(settings.FreeTierLimit)

			if freeTierBytes != expectedFreeTierBytes {
				log.Printf("Free tier limit changed from %d to %d bytes, updating for pubkey %s",
					storageInfo.TotalBytes, expectedFreeTierBytes, pubkey)
				needsUpdate = true
			}
		}
	} else {
		// If free tier is disabled but the user has the free tier set as active
		if activeTier == m.freeTierLimit && storageInfo.TotalBytes > 0 {
			// This is a free tier user but free tier is now disabled
			log.Printf("Free tier is disabled but user has free tier, keeping their existing allocation")
			// We don't change anything here - let them keep what they have until they pay
		}
	}

	if !needsUpdate {
		return event, nil
	}

	// Update the event
	if m.freeTierEnabled {
		// Set free tier expiration if not set
		if expirationDate.IsZero() || expirationDate.Before(time.Now()) {
			expirationDate = time.Now().AddDate(0, 1, 0) // 1 month from now
		}

		// Calculate storage based on free tier
		freeTierBytes := m.calculateStorageLimit(m.freeTierLimit)

		// Don't reduce storage if they already have more
		if storageInfo.TotalBytes < freeTierBytes {
			storageInfo.TotalBytes = freeTierBytes
		}

		// Set active tier to free tier if not set
		if activeTier == "" {
			activeTier = m.freeTierLimit
		}
	}

	// Create a new updated event
	updatedEvent, err := m.createEvent(&lib.Subscriber{
		Npub:    pubkey,
		Address: address,
	}, activeTier, expirationDate, &storageInfo)

	if err != nil {
		return event, fmt.Errorf("failed to create updated event: %v", err)
	}

	// Store the updated event
	if err := m.store.StoreEvent(updatedEvent); err != nil {
		return event, fmt.Errorf("failed to store updated event: %v", err)
	}

	log.Printf("Successfully updated kind 888 event for pubkey %s", pubkey)
	return updatedEvent, nil
}
