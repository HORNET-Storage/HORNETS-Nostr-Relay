// manager.go - Core subscription manager and types

package subscription

import (
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
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

// Address status constants
const (
	AddressStatusAvailable = "available"
	AddressStatusAllocated = "allocated"
	AddressStatusUsed      = "used"
)

// StorageInfo tracks current storage usage information for a subscriber
type StorageInfo struct {
	UsedBytes   int64     // Current bytes used by the subscriber
	TotalBytes  int64     // Total bytes allocated to the subscriber (0 for unlimited)
	IsUnlimited bool      // True if storage is unlimited
	UpdatedAt   time.Time // Last time storage information was updated
}

// SubscriptionManager handles all subscription-related operations through NIP-88 events
type SubscriptionManager struct {
	store             stores.Store             // Interface to handle minimal state storage
	relayPrivateKey   *btcec.PrivateKey        // Relay's private key for signing events
	relayDHTKey       string                   // Relay's DHT key
	subscriptionTiers []types.SubscriptionTier // Available subscription tiers
}

// NewSubscriptionManager creates and initializes a new subscription manager
func NewSubscriptionManager(
	store stores.Store,
	relayPrivKey *btcec.PrivateKey,
	relayDHTKey string,
	tiers []types.SubscriptionTier,
) *SubscriptionManager {
	logging.Infof("Initializing SubscriptionManager with %d tiers", len(tiers))

	// Log each tier in detail for debugging
	for i, tier := range tiers {
		logging.Infof("DEBUG: Initial tier %d: Name='%s', MonthlyLimitBytes=%d, PriceSats=%d, Unlimited=%t",
			i, tier.Name, tier.MonthlyLimitBytes, tier.PriceSats, tier.Unlimited)
	}

	// Validate tiers data
	validTiers := make([]types.SubscriptionTier, 0)
	for i, tier := range tiers {
		if tier.MonthlyLimitBytes <= 0 && !tier.Unlimited {
			logging.Infof("Warning: skipping tier %d ('%s') with invalid MonthlyLimitBytes: %d", i, tier.Name, tier.MonthlyLimitBytes)
			continue
		}
		validTiers = append(validTiers, tier)
		logging.Infof("Validated tier %d: Name='%s', MonthlyLimitBytes=%d, PriceSats=%d, Unlimited=%t",
			i, tier.Name, tier.MonthlyLimitBytes, tier.PriceSats, tier.Unlimited)
	}

	logging.Infof("SubscriptionManager initialized with %d valid tiers (from %d total)", len(validTiers), len(tiers))

	return &SubscriptionManager{
		store:             store,
		relayPrivateKey:   relayPrivKey,
		relayDHTKey:       relayDHTKey,
		subscriptionTiers: validTiers,
	}
}

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
	logging.Infof("Initialized global subscription manager with %d tiers", len(tiers))
	return globalManager
}

// GetGlobalManager returns the global subscription manager instance
// Returns nil if not initialized
func GetGlobalManager() *SubscriptionManager {
	globalManagerMutex.RLock()
	defer globalManagerMutex.RUnlock()

	return globalManager
}
