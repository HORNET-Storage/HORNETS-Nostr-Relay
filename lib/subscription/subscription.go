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
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/golang-jwt/jwt/v4"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
)

// Global subscription manager instance and update scheduling variables
var (
	globalManager      *SubscriptionManager
	globalManagerMutex sync.RWMutex

	// Variables for managing scheduled batch updates
	scheduledUpdateMutex sync.Mutex
	scheduledUpdateTimer *time.Timer
)

// InitGlobalManager initializes the global subscription manager instance
func InitGlobalManager(
	store stores.Store,
	relayPrivKey *btcec.PrivateKey,
	relayDHTKey string,
	tiers []types.SubscriptionTier,
) *SubscriptionManager {
	globalManagerMutex.Lock()
	defer globalManagerMutex.Unlock()

	globalManager = NewSubscriptionManager(store, relayPrivKey, relayDHTKey, tiers)
	log.Printf("Initialized global subscription manager with %d tiers", len(tiers))
	return globalManager
}

// GetGlobalManager returns the global subscription manager instance
// Returns nil if not initialized
func GetGlobalManager() *SubscriptionManager {
	globalManagerMutex.RLock()
	defer globalManagerMutex.RUnlock()

	return globalManager
}

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
	store             stores.Store             // Interface to handle minimal state storage
	relayPrivateKey   *btcec.PrivateKey        // Relay's private key for signing events
	relayDHTKey       string                   // Relay's DHT key
	subscriptionTiers []types.SubscriptionTier // Available subscription tiers
}

// NewSubscriptionManager creates and initializes a new subscription manager
// NewSubscriptionManager creates and initializes a new subscription manager
func NewSubscriptionManager(
	store stores.Store,
	relayPrivKey *btcec.PrivateKey,
	relayDHTKey string,
	tiers []types.SubscriptionTier,
) *SubscriptionManager {
	log.Printf("Initializing SubscriptionManager with tiers: %+v", tiers)

	// Log each tier in detail for debugging
	for i, tier := range tiers {
		log.Printf("DEBUG: Initial tier %d: MonthlyLimit='%s', PriceSats='%d'",
			i, tier.MonthlyLimit, tier.PriceSats)
	}

	// Validate tiers data
	validTiers := make([]types.SubscriptionTier, 0)
	for i, tier := range tiers {
		if tier.MonthlyLimit == "" {
			log.Printf("Warning: skipping tier %d with empty MonthlyLimit: '%s'", i, tier.MonthlyLimit)
			continue
		}
		validTiers = append(validTiers, tier)
		log.Printf("Validated tier %d: MonthlyLimit='%s', PriceSats='%d'",
			i, tier.MonthlyLimit, tier.PriceSats)
	}

	return &SubscriptionManager{
		store:             store,
		relayPrivateKey:   relayPrivKey,
		relayDHTKey:       relayDHTKey,
		subscriptionTiers: validTiers,
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

	// Step 2: Load allowed users settings to determine appropriate tier
	settings, err := config.GetConfig()
	if err != nil {
		log.Printf("Error getting config: %v", err)
		return fmt.Errorf("failed to get config: %v", err)
	}

	// Step 3: Determine appropriate tier for new user
	tierLimit := m.findAppropriateTierForUser(npub, "", &settings.AllowedUsersSettings)

	// Create initial NIP-88 event with storage usage based on assigned tier
	storageInfo := StorageInfo{
		UsedBytes: 0,
		UpdatedAt: time.Now(),
	}

	if tierLimit != "" {
		storageInfo.TotalBytes = m.calculateStorageLimit(tierLimit)
		log.Printf("Setting initial storage limit to %d bytes for tier: %s", storageInfo.TotalBytes, tierLimit)
	} else {
		storageInfo.TotalBytes = 0
		log.Printf("No tier assigned. Setting initial storage limit to 0 bytes")
	}

	// Set expiration date (1 month for all initial subscriptions)
	expirationDate := time.Now().AddDate(0, 1, 0)
	log.Printf("Setting tier limit: %s with expiration: %v", tierLimit, expirationDate)

	// Step 4: Create the NIP-88 event
	err = m.createNIP88EventIfNotExists(&types.Subscriber{
		Npub:    npub,
		Address: address.Address,
	}, tierLimit, expirationDate, &storageInfo)

	if err != nil {
		log.Printf("Error creating NIP-88 event: %v", err)
		return err
	}

	log.Printf("Successfully initialized subscriber %s with tier: %s", npub, tierLimit)
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
	var highestTier *types.SubscriptionTier
	var highestTierPriceSats int64

	for _, t := range m.subscriptionTiers {
		if t.PriceSats != 0 { // Skip free tier
			PriceSats := int64(t.PriceSats)
			if PriceSats > highestTierPriceSats {
				highestTierPriceSats = PriceSats
				highestTier = &types.SubscriptionTier{
					MonthlyLimit: t.MonthlyLimit,
					PriceSats:    t.PriceSats,
				}
			}
		}
	}

	// Handle payment greater than the highest tier PriceSats
	if highestTier != nil && totalAmount >= highestTierPriceSats && totalAmount > highestTierPriceSats {
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
				} else {
					// Set default expiration for new subscription
					expirationDate = time.Now().AddDate(0, 1, 0)
				}

				// Update the NIP-88 event to reflect the new credit
				if err := m.createOrUpdateNIP88Event(&types.Subscriber{
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

	// Validate that the paid tier has reasonable storage limits
	paidTierBytes := m.calculateStorageLimit(tier.MonthlyLimit)
	if paidTierBytes <= 0 {
		return fmt.Errorf("invalid paid tier storage limit: %s", tier.MonthlyLimit)
	}
	log.Printf("Processing payment for tier: %s (%d bytes)", tier.MonthlyLimit, paidTierBytes)

	// Get current expiration date from event
	expirationUnix := getTagUnixValue(currentEvent.Tags, "active_subscription")
	existingExpiration := time.Time{}
	if expirationUnix > 0 {
		existingExpiration = time.Unix(expirationUnix, 0)
	}

	// Always accumulate storage capacity for paid tiers
	prevBytes := storageInfo.TotalBytes
	newTierBytes := paidTierBytes

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
	err = m.createOrUpdateNIP88Event(&types.Subscriber{
		Npub:    npub,
		Address: address,
	}, tier.MonthlyLimit, endDate, &storageInfo)

	if err != nil {
		return fmt.Errorf("failed to update NIP-88 event: %v", err)
	}

	// Also update the paid subscribers table
	m.updatePaidSubscriberRecord(npub, tier.MonthlyLimit, endDate, &storageInfo)

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

	// Check if there are any sats leftover from this payment that could be credited
	tierPriceSats := int64(tier.PriceSats)
	if totalAmount > tierPriceSats {
		leftover := totalAmount - tierPriceSats
		log.Printf("Payment has %d sats leftover after purchasing tier", leftover)

		// Update credit with leftover amount
		if err := m.store.GetStatsStore().UpdateSubscriberCredit(npub, leftover); err != nil {
			log.Printf("Warning: failed to update credit with leftover amount: %v", err)
		} else {
			log.Printf("Added %d sats to credit for %s", leftover, npub)

			// Check if the updated credit can be used to purchase additional tier capacity
			_, err := m.checkAndApplyCredit(npub, address, &storageInfo, endDate)
			if err != nil {
				log.Printf("Warning: error checking credit for additional tier purchase: %v", err)
			}

			// Fetch the final credit amount to include in the NIP-88 event
			finalCredit, _ := m.store.GetStatsStore().GetSubscriberCredit(npub)

			// Final update of the NIP-88 event to include the latest credit
			if finalCredit > 0 {
				if err := m.createOrUpdateNIP88Event(&types.Subscriber{
					Npub:    npub,
					Address: address,
				}, tier.MonthlyLimit, endDate, &storageInfo); err != nil {
					log.Printf("Warning: failed to update final NIP-88 event with credit: %v", err)
				} else {
					log.Printf("Updated final NIP-88 event for %s with credit: %d sats", npub, finalCredit)
				}
			}
		}
	}

	// Add transaction processing log
	log.Printf("Successfully processed payment for %s: %d sats for tier %s",
		npub, amountSats, tier.MonthlyLimit)

	return nil
}

// processHighTierPayment handles payments that exceed the highest tier PriceSats by extending
// the subscription period and attempting to use any remainder for lower tiers
func (m *SubscriptionManager) processHighTierPayment(
	npub string,
	transactionID string,
	amountSats int64,
	highestTier *types.SubscriptionTier,
) error {
	log.Printf("Processing high-tier payment (tx: %s) for %s: %d sats for tier %s",
		transactionID, npub, amountSats, highestTier.MonthlyLimit)

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

	// Calculate full periods and remainder for highest tier
	highestTierPriceSats := int64(highestTier.PriceSats)
	fullPeriods := amountSats / highestTierPriceSats
	remainingSats := amountSats % highestTierPriceSats

	if fullPeriods < 1 {
		fullPeriods = 1 // Ensure at least one period
	}

	// Calculate the storage for one period of highest tier
	highestTierStorageBytes := m.calculateStorageLimit(highestTier.MonthlyLimit)

	// Calculate total new storage for all periods purchased
	totalNewStorage := highestTierStorageBytes * fullPeriods

	// Add the new storage to existing storage (accumulate instead of replace)
	prevBytes := storageInfo.TotalBytes
	storageInfo.TotalBytes += totalNewStorage
	storageInfo.UpdatedAt = time.Now()

	log.Printf("Upgrading storage from %d to %d bytes (adding %d bytes for %d periods of tier: %s)",
		prevBytes, storageInfo.TotalBytes, totalNewStorage, fullPeriods, highestTier.MonthlyLimit)

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
	err = m.createOrUpdateNIP88Event(&types.Subscriber{
		Npub:    npub,
		Address: address,
	}, highestTier.MonthlyLimit, endDate, &storageInfo)

	if err != nil {
		return fmt.Errorf("failed to update NIP-88 event: %v", err)
	}

	// Also update the paid subscribers table
	m.updatePaidSubscriberRecord(npub, highestTier.MonthlyLimit, endDate, &storageInfo)

	// Try to use remaining sats for lower tiers (cascading approach)
	// Sort tiers by PriceSats (descending)
	if remainingSats > 0 && len(m.subscriptionTiers) > 1 {
		log.Printf("Attempting to use remaining %d sats for lower tiers", remainingSats)

		// Create a sorted list of tiers by PriceSats (descending)
		type tierInfo struct {
			tier      types.SubscriptionTier
			PriceSats int64
		}

		sortedTiers := make([]tierInfo, 0)
		for _, tier := range m.subscriptionTiers {
			// Skip free tiers and the highest tier (already processed)
			if tier.PriceSats <= 0 || tier.MonthlyLimit == highestTier.MonthlyLimit {
				continue
			}

			PriceSats := int64(tier.PriceSats)
			if PriceSats > 0 {
				sortedTiers = append(sortedTiers, tierInfo{tier: tier, PriceSats: PriceSats})
			}
		}

		// Sort tiers by PriceSats (descending)
		for i := 0; i < len(sortedTiers)-1; i++ {
			for j := i + 1; j < len(sortedTiers); j++ {
				if sortedTiers[i].PriceSats < sortedTiers[j].PriceSats {
					sortedTiers[i], sortedTiers[j] = sortedTiers[j], sortedTiers[i]
				}
			}
		}

		// Get the lowest tier PriceSats for later comparison
		var lowestTierPriceSats int64 = highestTierPriceSats
		if len(sortedTiers) > 0 {
			lowestTierPriceSats = sortedTiers[len(sortedTiers)-1].PriceSats
		}

		// Try to use remaining sats for each tier
		for _, tierInfo := range sortedTiers {
			if remainingSats >= tierInfo.PriceSats {
				// We can afford this tier
				tierBytes := m.calculateStorageLimit(tierInfo.tier.MonthlyLimit)

				// Add storage
				storageInfo.TotalBytes += tierBytes

				log.Printf("Using %d sats for additional tier: %s (adding %d bytes)",
					tierInfo.PriceSats, tierInfo.tier.MonthlyLimit, tierBytes)

				// Subtract from remaining sats
				remainingSats -= tierInfo.PriceSats

				// If we run out of sats, break
				if remainingSats < lowestTierPriceSats {
					break
				}
			}
		}

		// Update the NIP-88 event with the additional storage
		if storageInfo.TotalBytes > totalNewStorage+prevBytes {
			err = m.createOrUpdateNIP88Event(&types.Subscriber{
				Npub:    npub,
				Address: address,
			}, highestTier.MonthlyLimit, endDate, &storageInfo)

			if err != nil {
				return fmt.Errorf("failed to update NIP-88 event with additional storage: %v", err)
			}

			// Update the paid subscribers table
			m.updatePaidSubscriberRecord(npub, highestTier.MonthlyLimit, endDate, &storageInfo)
		}
	}

	// Credit remainder if any
	if remainingSats > 0 {
		if err := m.store.GetStatsStore().UpdateSubscriberCredit(npub, remainingSats); err != nil {
			log.Printf("Warning: failed to save remainder credit of %d sats: %v", remainingSats, err)
		} else {
			log.Printf("Credited remainder of %d sats to user account", remainingSats)

			// Check if the stored credit can be used to purchase additional tier capacity
			_, err := m.checkAndApplyCredit(npub, address, &storageInfo, endDate)
			if err != nil {
				log.Printf("Warning: error checking credit for additional tier purchase: %v", err)
			}
		}
	}

	// Final update to ensure credit tag is included in NIP-88 event
	finalCredit, err := m.store.GetStatsStore().GetSubscriberCredit(npub)
	if err == nil && finalCredit > 0 {
		// One last update to ensure the credit is reflected in the NIP-88 event
		err = m.createOrUpdateNIP88Event(&types.Subscriber{
			Npub:    npub,
			Address: address,
		}, highestTier.MonthlyLimit, endDate, &storageInfo)

		if err != nil {
			log.Printf("Warning: failed to update final NIP-88 event with credit: %v", err)
		} else {
			log.Printf("Updated final NIP-88 event for %s with credit: %d sats", npub, finalCredit)
		}
	}

	log.Printf("Successfully processed high-tier payment: %d sats for %d months of %s tier",
		amountSats, fullPeriods, highestTier.MonthlyLimit)

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
	return m.createOrUpdateNIP88Event(&types.Subscriber{
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
	subscriber *types.Subscriber,
	activeTier string,
	expirationDate time.Time,
	storageInfo *StorageInfo,
) (*nostr.Event, error) {
	// Determine subscription status based on tier and expiration
	status := m.getSubscriptionStatus(activeTier)
	if activeTier != "" && !expirationDate.IsZero() && expirationDate.After(time.Now()) {
		status = "active"
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

	// Add tier information if tier is assigned
	if activeTier != "" {
		tags = append(tags, nostr.Tag{
			"active_subscription",
			activeTier,
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
	subscriber *types.Subscriber,
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
	subscriber *types.Subscriber,
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
func (m *SubscriptionManager) findMatchingTier(amountSats int64) (*types.SubscriptionTier, error) {
	if len(m.subscriptionTiers) == 0 {
		// Reload tiers from allowed_users settings
		var allowedUsersSettings types.AllowedUsersSettings
		if err := viper.UnmarshalKey("allowed_users", &allowedUsersSettings); err != nil {
			return nil, fmt.Errorf("no tiers available and failed to load allowed users settings: %v", err)
		}
		m.subscriptionTiers = allowedUsersSettings.Tiers
	}

	log.Printf("Finding tier for %d sats among %d tiers: %+v",
		amountSats, len(m.subscriptionTiers), m.subscriptionTiers)

	var bestMatch *types.SubscriptionTier
	var bestPriceSats int64

	for _, tier := range m.subscriptionTiers {
		if tier.MonthlyLimit == "" || tier.PriceSats == 0 {
			log.Printf("Warning: skipping invalid tier: MonthlyLimit='%s', PriceSats='%d'",
				tier.MonthlyLimit, tier.PriceSats)
			continue
		}

		PriceSats := int64(tier.PriceSats)
		log.Printf("Checking tier: MonthlyLimit='%s', PriceSats='%d' (%d sats)",
			tier.MonthlyLimit, tier.PriceSats, PriceSats)

		// Strict matching: Payment must be >= tier PriceSats exactly
		// No tolerance - exact matches only
		exactMatch := (amountSats >= PriceSats)

		// Must match exactly AND be better than current best match
		if exactMatch && PriceSats > bestPriceSats {
			bestMatch = &types.SubscriptionTier{
				MonthlyLimit: tier.MonthlyLimit,
				PriceSats:    tier.PriceSats,
			}
			bestPriceSats = PriceSats
			log.Printf("New best match: MonthlyLimit='%s', PriceSats='%d' (exact match)",
				bestMatch.MonthlyLimit, bestMatch.PriceSats)
		}
	}

	if bestMatch == nil {
		return nil, fmt.Errorf("no matching tier for payment of %d sats", amountSats)
	}

	log.Printf("Selected tier: MonthlyLimit='%s', PriceSats='%d'",
		bestMatch.MonthlyLimit, bestMatch.PriceSats)
	return bestMatch, nil
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
	apiKey := viper.GetString("external_services.wallet.key")

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

	walletAddress := config.GetExternalURL("wallet")

	// Create request
	req, err := http.NewRequest("POST",
		walletAddress+"/generate-addresses",
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
// based on current allowed users settings and updates it if necessary
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

	// Load allowed users settings to check last update timestamp
	var allowedUsersSettings types.AllowedUsersSettings
	if err := viper.UnmarshalKey("allowed_users", &allowedUsersSettings); err != nil {
		log.Printf("Error loading allowed users settings: %v", err)
		return event, nil // Return original event if we can't get settings
	}

	// Check if event was created/updated after the last settings change
	eventCreatedAt := time.Unix(int64(event.CreatedAt), 0)
	settingsUpdatedAt := time.Unix(allowedUsersSettings.LastUpdated, 0)

	// If event is newer than the last settings change, it's already up to date
	if allowedUsersSettings.LastUpdated > 0 && eventCreatedAt.After(settingsUpdatedAt) {
		log.Printf("Event was created/updated after the last settings change, no update needed")
		return event, nil
	}

	needsUpdate := false

	// Check if tier needs updating based on current mode
	expectedTier := m.findAppropriateTierForUser(pubkey, activeTier, &allowedUsersSettings)

	if expectedTier == "" {
		// User should not have access in current mode
		if allowedUsersSettings.Mode == "exclusive" {
			log.Printf("User %s no longer in allowed lists for exclusive mode, but keeping existing allocation", pubkey)
			// Don't remove existing allocations, just don't give new ones
		}
	} else if expectedTier != activeTier {
		// Tier has changed
		log.Printf("Expected tier changed for %s: %s -> %s", pubkey, activeTier, expectedTier)
		needsUpdate = true
	} else if status == "inactive" || storageInfo.TotalBytes == 0 {
		// User should have active allocation but doesn't
		log.Printf("User %s should have active %s tier but status is %s", pubkey, expectedTier, status)
		needsUpdate = true
	}

	if !needsUpdate {
		return event, nil
	}

	// Update the event based on expected tier
	if expectedTier != "" {
		// Set expiration if not set (monthly renewal)
		if expirationDate.IsZero() || expirationDate.Before(time.Now()) {
			expirationDate = time.Now().AddDate(0, 1, 0) // 1 month from now
		}

		// Calculate storage based on expected tier
		expectedTierBytes := m.calculateStorageLimit(expectedTier)

		// Don't reduce storage if they already have more (graceful transition)
		if storageInfo.TotalBytes < expectedTierBytes {
			storageInfo.TotalBytes = expectedTierBytes
		}

		// Update active tier
		activeTier = expectedTier
	}

	// Create a new updated event
	updatedEvent, err := m.createEvent(&types.Subscriber{
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

// checkAndApplyCredit checks if the subscriber's credit can be used to purchase any tier
// and applies it if possible. It returns the remaining credit and any error.
func (m *SubscriptionManager) checkAndApplyCredit(
	npub string,
	address string,
	storageInfo *StorageInfo,
	endDate time.Time,
) (int64, error) {
	// Get current credit
	credit, err := m.store.GetStatsStore().GetSubscriberCredit(npub)
	if err != nil {
		return 0, fmt.Errorf("failed to get subscriber credit: %v", err)
	}

	// If credit is too low, just return it
	if credit <= 0 {
		return credit, nil
	}

	log.Printf("Checking if credit of %d sats can be used for any tier", credit)

	// Try to find a tier that the credit can afford
	tier, err := m.findMatchingTier(credit)
	if err != nil {
		// No matching tier, just return the credit
		log.Printf("No tier found for credit of %d sats", credit)
		return credit, nil
	}

	// We found a tier! Apply it
	tierPriceSats := int64(tier.PriceSats)
	tierBytes := m.calculateStorageLimit(tier.MonthlyLimit)

	// Add storage
	prevBytes := storageInfo.TotalBytes
	storageInfo.TotalBytes += tierBytes
	storageInfo.UpdatedAt = time.Now()

	log.Printf("Using credit of %d sats for tier: %s (adding %d bytes to existing %d bytes, new total: %d bytes)",
		tierPriceSats, tier.MonthlyLimit, tierBytes, prevBytes, storageInfo.TotalBytes)

	// Update the NIP-88 event
	err = m.createOrUpdateNIP88Event(&types.Subscriber{
		Npub:    npub,
		Address: address,
	}, tier.MonthlyLimit, endDate, storageInfo)

	if err != nil {
		return credit, fmt.Errorf("failed to update NIP-88 event with credit-purchased tier: %v", err)
	}

	// Update paid subscriber record
	m.updatePaidSubscriberRecord(npub, tier.MonthlyLimit, endDate, storageInfo)

	// Update credit in database
	remainingCredit := credit - tierPriceSats
	if err := m.store.GetStatsStore().UpdateSubscriberCredit(npub, remainingCredit); err != nil {
		return remainingCredit, fmt.Errorf("failed to update credit after using for tier: %v", err)
	}

	log.Printf("Successfully used %d sats from credit for tier %s, remaining credit: %d",
		tierPriceSats, tier.MonthlyLimit, remainingCredit)

	// Check if remaining credit can be used for another tier recursively
	if remainingCredit > 0 {
		return m.checkAndApplyCredit(npub, address, storageInfo, endDate)
	}

	return remainingCredit, nil
}

// ScheduleBatchUpdateAfter schedules a batch update of all kind 888 events after
// the specified delay. If called multiple times, it cancels any previous scheduled update
// and restarts the timer with a new delay (sliding window approach).
func ScheduleBatchUpdateAfter(delay time.Duration) {
	scheduledUpdateMutex.Lock()
	defer scheduledUpdateMutex.Unlock()

	// Cancel any existing scheduled update
	if scheduledUpdateTimer != nil {
		scheduledUpdateTimer.Stop()
		log.Printf("Rescheduling batch update of kind 888 events (settings changed again)")
	} else {
		log.Printf("Scheduling batch update of kind 888 events in %v", delay)
	}

	// Schedule new update
	scheduledUpdateTimer = time.AfterFunc(delay, func() {
		scheduledUpdateMutex.Lock()
		scheduledUpdateTimer = nil
		scheduledUpdateMutex.Unlock()

		log.Printf("Starting batch update of kind 888 events after %v cooldown", delay)
		manager := GetGlobalManager()
		if manager != nil {
			if err := manager.BatchUpdateAllSubscriptionEvents(); err != nil {
				log.Printf("Error in batch update: %v", err)
			} else {
				log.Printf("Successfully completed batch update of kind 888 events")
			}
		}
	})
}

// BatchUpdateAllSubscriptionEvents processes all kind 888 events in batches
// to update storage allocations after allowed users settings have changed
func (m *SubscriptionManager) BatchUpdateAllSubscriptionEvents() error {
	log.Printf("Starting batch update of all kind 888 subscription events")

	// Get the current allowed users settings
	var allowedUsersSettings types.AllowedUsersSettings
	if err := viper.UnmarshalKey("allowed_users", &allowedUsersSettings); err != nil {
		return fmt.Errorf("failed to load allowed users settings: %v", err)
	}

	// Process events in batches of 50
	batchSize := 50
	processed := 0

	for {
		// Query the next batch of events
		filter := nostr.Filter{
			Kinds: []int{888},
			Limit: batchSize,
		}

		events, err := m.store.QueryEvents(filter)
		if err != nil {
			return fmt.Errorf("error querying events: %v", err)
		}

		// Exit if no more events
		if len(events) == 0 {
			break
		}

		// Process each event in the batch
		for _, event := range events {
			if err := m.processSingleSubscriptionEvent(event, &allowedUsersSettings); err != nil {
				log.Printf("Error processing event %s: %v", event.ID, err)
				// Continue with next event even if this one fails
			}

			processed++
		}

		log.Printf("Processed %d kind 888 events so far", processed)

		// If we received fewer events than requested, we've reached the end of available events
		if len(events) < batchSize {
			break
		}
	}

	log.Printf("Completed batch update, processed %d kind 888 events", processed)
	return nil
}

// processSingleSubscriptionEvent handles the storage adjustment logic for a single kind 888 event
// Only handling tier adjustments based on mode, as paid tiers are automatically updated when users purchase them
func (m *SubscriptionManager) processSingleSubscriptionEvent(event *nostr.Event, allowedUsersSettings *types.AllowedUsersSettings) error {
	// Extract pubkey
	pubkey := getTagValue(event.Tags, "p")
	if pubkey == "" {
		return fmt.Errorf("no pubkey found in event")
	}

	// Get storage info
	storageInfo, err := m.extractStorageInfo(event)
	if err != nil {
		return fmt.Errorf("failed to extract storage info: %v", err)
	}

	// Get subscription details
	activeTier := getTagValue(event.Tags, "active_subscription")
	expirationUnix := getTagUnixValue(event.Tags, "active_subscription")
	expirationDate := time.Unix(expirationUnix, 0)
	address := getTagValue(event.Tags, "relay_bitcoin_address")

	needsUpdate := false

	// Handle tier adjustments based on mode
	// Find appropriate tier for this user based on mode and current status
	newTierMonthlyLimit := m.findAppropriateTierForUser(pubkey, activeTier, allowedUsersSettings)

	if newTierMonthlyLimit == "" {
		log.Printf("No appropriate tier found for user %s in %s mode", pubkey, allowedUsersSettings.Mode)
		return nil
	}

	newTierBytes := m.calculateStorageLimit(newTierMonthlyLimit)
	oldTierBytes := storageInfo.TotalBytes

	// Only update if tier has actually changed
	if newTierMonthlyLimit != activeTier || newTierBytes != oldTierBytes {
		log.Printf("Updating tier for %s from %s (%d bytes) to %s (%d bytes)",
			pubkey, activeTier, oldTierBytes, newTierMonthlyLimit, newTierBytes)

		// Update storage allocation
		if newTierBytes > oldTierBytes {
			// Tier increased - add the difference
			storageInfo.TotalBytes = newTierBytes
		} else {
			// Tier decreased - set new limit but don't reduce below current usage
			if newTierBytes < storageInfo.UsedBytes {
				storageInfo.TotalBytes = storageInfo.UsedBytes // Can't go below current usage
				log.Printf("Warning: New tier limit (%d) is below current usage (%d) for %s",
					newTierBytes, storageInfo.UsedBytes, pubkey)
			} else {
				storageInfo.TotalBytes = newTierBytes
			}
		}

		// Update active tier
		activeTier = newTierMonthlyLimit
		needsUpdate = true
	}

	// Update the event if needed
	if needsUpdate {
		return m.createOrUpdateNIP88Event(&types.Subscriber{
			Npub:    pubkey,
			Address: address,
		}, activeTier, expirationDate, &storageInfo)
	}

	return nil
}

// InitDailyFreeSubscriptionRenewal sets up a daily job to refresh expired free tier subscriptions
// This should be called once when the application starts
func InitDailyFreeSubscriptionRenewal() {
	go func() {
		for {
			now := time.Now()
			nextRun := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 1, 0, 0, now.Location())
			delay := nextRun.Sub(now)

			log.Printf("Scheduled free tier renewal in %v (at %s)",
				delay, nextRun.Format("2006-01-02 15:04:05"))

			time.Sleep(delay)

			// Run the renewal
			manager := GetGlobalManager()
			if manager != nil {
				log.Printf("Starting daily free tier renewal process")
				if err := manager.RefreshExpiredFreeTierSubscriptions(); err != nil {
					log.Printf("Error in free tier renewal: %v", err)
				} else {
					log.Printf("Successfully completed daily free tier renewal")
				}
			}
		}
	}()
}

// RefreshExpiredFreeTierSubscriptions finds and refreshes all expired free tier subscriptions
func (m *SubscriptionManager) RefreshExpiredFreeTierSubscriptions() error {
	// Load allowed users settings to check for free mode
	var allowedUsersSettings types.AllowedUsersSettings
	if err := viper.UnmarshalKey("allowed_users", &allowedUsersSettings); err != nil {
		log.Printf("Error loading allowed users settings: %v", err)
		return fmt.Errorf("failed to load allowed users settings: %v", err)
	}

	// Only process free tier renewals in free mode
	if allowedUsersSettings.Mode != "free" {
		log.Printf("Skipping free tier renewal - not in free mode (current mode: %s)", allowedUsersSettings.Mode)
		return nil
	}

	log.Printf("Checking for expired free tier subscriptions to refresh")

	now := time.Now()
	batchSize := 50
	processed := 0
	refreshed := 0

	for {
		// Query all kind 888 events in batches
		filter := nostr.Filter{
			Kinds: []int{888},
			Limit: batchSize,
		}

		events, err := m.store.QueryEvents(filter)
		if err != nil {
			return fmt.Errorf("error querying events: %v", err)
		}

		// Exit if no more events
		if len(events) == 0 {
			break
		}

		for _, event := range events {
			processed++

			// Get user pubkey
			pubkey := getTagValue(event.Tags, "p")
			if pubkey == "" {
				continue
			}

			// Check if it's a free tier subscription (PriceSats = "0")
			activeTier := getTagValue(event.Tags, "active_subscription")
			isFreeTier := false
			for _, tier := range allowedUsersSettings.Tiers {
				if tier.MonthlyLimit == activeTier && tier.PriceSats <= 0 {
					isFreeTier = true
					break
				}
			}
			if !isFreeTier {
				continue
			}

			// Get Bitcoin address
			address := getTagValue(event.Tags, "relay_bitcoin_address")

			// Check expiration date
			expirationUnix := getTagUnixValue(event.Tags, "active_subscription")
			expirationDate := time.Unix(expirationUnix, 0)

			// Skip if not expired
			if !now.After(expirationDate) {
				continue
			}

			// Get current storage info
			storageInfo, err := m.extractStorageInfo(event)
			if err != nil {
				log.Printf("Warning: could not extract storage info for %s: %v", pubkey, err)
				continue
			}

			// Reset used storage to zero
			storageInfo.UsedBytes = 0

			// Set new expiration date
			newExpiration := now.AddDate(0, 1, 0)

			// Look for pending storage adjustments
			pendingTierLimit := ""
			for _, tag := range event.Tags {
				if tag[0] == "storage_adjustment_pending" && len(tag) > 1 {
					pendingTierLimit = tag[1]
					break
				}
			}

			// Apply pending adjustment if found
			if pendingTierLimit != "" {
				storageInfo.TotalBytes = m.calculateStorageLimit(pendingTierLimit)
				log.Printf("Applying pending adjustment for %s: new limit %s",
					pubkey, pendingTierLimit)
			}

			// Determine which tier to use (keep current free tier or use pending adjustment)
			tierToUse := activeTier
			if pendingTierLimit != "" {
				tierToUse = pendingTierLimit
			}

			// Update the NIP-88 event
			err = m.createOrUpdateNIP88Event(&types.Subscriber{
				Npub:    pubkey,
				Address: address,
			}, tierToUse, newExpiration, &storageInfo)

			if err != nil {
				log.Printf("Error refreshing free tier: %v", err)
			} else {
				refreshed++
				log.Printf("Refreshed free tier for %s until %s",
					pubkey, newExpiration.Format("2006-01-02"))
			}
		}

		// If we got fewer events than requested, we've reached the end
		if len(events) < batchSize {
			break
		}
	}

	log.Printf("Free tier refresh complete: processed %d events, refreshed %d subscriptions",
		processed, refreshed)
	return nil
}

// updatePaidSubscriberRecord is a helper method to update the PaidSubscriber table
// This should be called after successfully updating a NIP-88 event
func (m *SubscriptionManager) updatePaidSubscriberRecord(
	npub string,
	tier string,
	expirationDate time.Time,
	storageInfo *StorageInfo,
) {
	// Load allowed users settings to check if this is a free tier
	var allowedUsersSettings types.AllowedUsersSettings
	if err := viper.UnmarshalKey("allowed_users", &allowedUsersSettings); err != nil {
		log.Printf("Error loading allowed users settings: %v", err)
		return
	}

	// Skip free tier subscriptions (PriceSats = "0")
	for _, allowedTier := range allowedUsersSettings.Tiers {
		if allowedTier.MonthlyLimit == tier && allowedTier.PriceSats <= 0 {
			log.Printf("Skipping paid subscriber record update for free tier: %s", tier)
			return
		}
	}

	// Try to get the existing subscriber
	existingSubscriber, err := m.store.GetStatsStore().GetPaidSubscriberByNpub(npub)
	if err != nil {
		log.Printf("Warning: error checking for existing paid subscriber record: %v", err)
		return
	}

	// Create or update the paid subscriber record
	paidSubscriber := &types.PaidSubscriber{
		Npub:           npub,
		Tier:           tier,
		ExpirationDate: expirationDate,
		StorageBytes:   storageInfo.TotalBytes,
		UsedBytes:      storageInfo.UsedBytes,
	}

	// If subscriber already exists, keep the ID
	var updateErr error
	if existingSubscriber != nil {
		paidSubscriber.ID = existingSubscriber.ID
		updateErr = m.store.GetStatsStore().UpdatePaidSubscriber(paidSubscriber)
	} else {
		updateErr = m.store.GetStatsStore().SavePaidSubscriber(paidSubscriber)
	}

	if updateErr != nil {
		log.Printf("Warning: failed to update paid subscriber record: %v", updateErr)
	} else {
		log.Printf("Successfully updated paid subscriber record for %s", npub)
	}
}

// findAppropriateTierForUser determines the appropriate tier for a user based on mode and current status
func (m *SubscriptionManager) findAppropriateTierForUser(pubkey, currentTier string, allowedUsersSettings *types.AllowedUsersSettings) string {
	switch allowedUsersSettings.Mode {
	case "free":
		// In free mode, assign users to the first available free tier
		for _, tier := range allowedUsersSettings.Tiers {
			if tier.PriceSats <= 0 {
				return tier.MonthlyLimit
			}
		}
		// Fallback to basic free allocation
		return "100 MB per month"

	case "paid":
		// In paid mode, check if user has active subscription
		// If not, give them basic free tier if available
		if currentTier == "" || currentTier == "0" {
			// Look for free tier in paid mode
			for _, tier := range allowedUsersSettings.Tiers {
				if tier.PriceSats <= 0 {
					return tier.MonthlyLimit
				}
			}
			// No free tier available, use minimal allocation
			return "100 MB per month"
		}
		// User has active subscription, keep current tier
		return currentTier

	case "exclusive":
		// In exclusive mode, tier is manually assigned
		// Check if user is still in allowed lists
		if m.isUserInAllowedLists(pubkey) {
			if currentTier != "" {
				return currentTier
			}
			// User is allowed but no tier assigned, give basic tier
			for _, tier := range allowedUsersSettings.Tiers {
				return tier.MonthlyLimit // Use first available tier
			}
		}
		// User not in allowed lists, no tier
		return ""

	default:
		log.Printf("Unknown mode: %s", allowedUsersSettings.Mode)
		return currentTier
	}
}

// isUserInAllowedLists checks if a user is in the allowed read or write lists for exclusive mode
func (m *SubscriptionManager) isUserInAllowedLists(pubkey string) bool {
	// Check read list
	if allowed, err := m.store.GetStatsStore().IsNpubInAllowedReadList(pubkey); err == nil && allowed {
		return true
	}
	// Check write list
	if allowed, err := m.store.GetStatsStore().IsNpubInAllowedWriteList(pubkey); err == nil && allowed {
		return true
	}
	return false
}
