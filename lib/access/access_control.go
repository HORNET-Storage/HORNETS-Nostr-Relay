package access

import (
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
)

const (
	// accessCacheTTL is how long access check results are cached.
	accessCacheTTL = 30 * time.Second
)

// cachedResult stores an access check result with expiry.
type cachedResult struct {
	err       error
	expiresAt time.Time
}

// AccessControl handles permission checking for H.O.R.N.E.T Allowed Users
type AccessControl struct {
	statsStore statistics.StatisticsStore
	settings   *types.AllowedUsersSettings

	// accessCache caches IsAllowed results keyed by "permission:hex"
	accessCache sync.Map
}

// NewAccessControl creates a new access control instance
func NewAccessControl(statsStore statistics.StatisticsStore, settings *types.AllowedUsersSettings) *AccessControl {
	return &AccessControl{
		statsStore: statsStore,
		settings:   settings,
	}
}

func (ac *AccessControl) CanRead(npub string) error {
	return ac.IsAllowed(ac.settings.Read, npub)
}

func (ac *AccessControl) CanWrite(npub string) error {
	return ac.IsAllowed(ac.settings.Write, npub)
}

func (ac *AccessControl) IsAllowed(readOrWrite string, npub string) error {
	// Everyone is allowed if all_users is set
	if readOrWrite == "all_users" {
		return nil
	}

	// Validate that the public key is a valid 64-char hex string.
	// Per NIP-01, clients must send hex-encoded 32-byte pubkeys.
	if !isValidHexPubkey(npub) {
		return fmt.Errorf("invalid public key format: expected 64-character hex string")
	}
	hex := strings.ToLower(npub)

	// Check cache first
	cacheKey := readOrWrite + ":" + hex
	if cached, ok := ac.accessCache.Load(cacheKey); ok {
		entry := cached.(*cachedResult)
		if time.Now().Before(entry.expiresAt) {
			return entry.err
		}
		// Expired — delete and re-check
		ac.accessCache.Delete(cacheKey)
	}

	// Perform the actual access check
	result := ac.isAllowedUncached(readOrWrite, hex)

	// Cache the result
	ac.accessCache.Store(cacheKey, &cachedResult{
		err:       result,
		expiresAt: time.Now().Add(accessCacheTTL),
	})

	return result
}

// isAllowedUncached performs the actual DB-backed access check (no cache).
func (ac *AccessControl) isAllowedUncached(readOrWrite string, hex string) error {
	logging.Debugf("Access check - Permission: %s, Mode: %s", readOrWrite, ac.settings.Mode)

	// The owner is always allowed
	if ac.isOwner(hex) {
		logging.Debugf("[ACCESS CONTROL] User %s is the relay owner, granting access", hex)
		return nil
	}

	// Get the allowed user from the database
	user, err := ac.statsStore.GetAllowedUser(hex)
	if err != nil {
		logging.Debugf("[ACCESS CONTROL] Error looking up user %s: %v", hex, err)
		return err
	}

	// User is not allowed if they don't exist
	if user == nil {
		logging.Debugf("[ACCESS CONTROL] User %s not found in allowed_users table", hex)
		return fmt.Errorf("user does not have permission to read")
	}

	logging.Debugf("[ACCESS CONTROL] User %s found with tier: %s", hex, user.Tier)

	// Check if user has a paid tier if set to paid_users
	if readOrWrite == "paid_users" {
		logging.Debugf("[ACCESS CONTROL] Checking paid subscriber status for user: %s", hex)
		paidSubscriber, err := ac.statsStore.GetPaidSubscriberByNpub(hex)
		if err != nil {
			logging.Debugf("[ACCESS CONTROL] Error checking paid subscriber status: %v", err)
			return fmt.Errorf("user does not have permission")
		}

		if paidSubscriber == nil {
			logging.Debugf("[ACCESS CONTROL] User %s not found in paid subscribers table", hex)
			return fmt.Errorf("user does not have a paid subscription")
		}

		// Check if subscription is still valid
		if time.Now().After(paidSubscriber.ExpirationDate) {
			logging.Debugf("[ACCESS CONTROL] User %s subscription expired on %v", hex, paidSubscriber.ExpirationDate)
			return fmt.Errorf("user subscription has expired")
		}

		if paidSubscriber.Tier == "" {
			logging.Debugf("[ACCESS CONTROL] User %s has empty tier", hex)
			return fmt.Errorf("user does not have a valid subscription tier")
		}

		// Check if the tier is actually a paid tier
		cfg, err := config.GetConfig()
		if err == nil && cfg != nil {
			tierIsPaid := false
			for _, tier := range cfg.AllowedUsersSettings.Tiers {
				if tier.Name == paidSubscriber.Tier && tier.PriceSats > 0 {
					tierIsPaid = true
					break
				}
			}
			if !tierIsPaid {
				logging.Debugf("[ACCESS CONTROL] User %s has free/unpaid tier: %s", hex, paidSubscriber.Tier)
				return fmt.Errorf("user has a free tier, not a paid subscription")
			}
		} else {
			if strings.Contains(strings.ToLower(paidSubscriber.Tier), "free") || strings.Contains(strings.ToLower(paidSubscriber.Tier), "basic") {
				logging.Debugf("[ACCESS CONTROL] User %s has free tier: %s", hex, paidSubscriber.Tier)
				return fmt.Errorf("user has a free tier, not a paid subscription")
			}
		}

		logging.Debugf("[ACCESS CONTROL] User %s has valid paid subscription: %s", hex, paidSubscriber.Tier)
	}

	return nil
}

func (ac *AccessControl) AddAllowedUser(npub string, read bool, write bool, tier string, createdBy string) error {
	return ac.statsStore.AddAllowedUser(npub, tier, createdBy)
}

func (ac *AccessControl) RemoveAllowedUser(npub string) error {
	return ac.statsStore.RemoveAllowedUser(npub)
}

// isValidHexPubkey checks if s is a valid 64-character hex-encoded public key.
func isValidHexPubkey(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// Is the incoming pub key the owner of the relay
func (ac *AccessControl) isOwner(hex string) bool {
	// First check database for relay owner
	if ac.statsStore != nil {
		owner, err := ac.statsStore.GetRelayOwner()
		if err == nil && owner != nil {
			ownerHex := strings.ToLower(owner.Npub)
			if hex == ownerHex {
				return true
			}
		}
	}

	// Fallback to config-based owner (for backwards compatibility)
	_, err := config.GetConfig()
	if err != nil {
		return false
	}

	// Note: The relay public key is not in the Config struct,
	// so we need to get it from the relay settings
	// For now, we'll skip this check if we can't get the config
	// The database check above should be sufficient
	return false
}

// ValidateSettings validates the access control settings for consistency
func (ac *AccessControl) ValidateSettings(settings *types.AllowedUsersSettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
	}

	// Validate mode
	mode := strings.ToLower(settings.Mode)
	read := strings.ToLower(settings.Read)
	write := strings.ToLower(settings.Write)

	logging.Debugf("Write setting %s", write)
	// This ensures the correct options are selected for each mode and sets defaults when incorrect values are set
	// Not all read/write values are valid for each mode so this ensures that the read/write values are in line with the selected mode
	// mode: 		only-me, invite_only, public, subscription
	// read/write: 	all_users, paid_users, allowed_users, only-me

	switch mode {
	case "only-me":
		write = "only-me"
		switch read {
		case "only-me":
		case "all_users":
		case "allowed_users":
		default:
			read = "only-me"
		}
	case "invite-only":
		write = "allowed_users"
		switch read {
		case "all_users":
		case "allowed_users":
		default:
			read = "allowed_users"
		}
	case "public":
		write = "all_users"
		read = "all_users"
	case "subscription":
		write = "paid_users"
		switch read {
		case "all_users":
		case "paid_users":
		default:
			read = "paid_users"
		}
	default:
		mode = "only-me"
		read = "only-me"
		write = "only-me"
	}

	settings.Mode = mode
	settings.Read = read
	settings.Write = write

	return nil
}

// GetSettings returns the current access control settings
func (ac *AccessControl) GetSettings() *types.AllowedUsersSettings {
	return ac.settings
}

// UpdateSettings updates the access control settings and invalidates the access cache
func (ac *AccessControl) UpdateSettings(settings *types.AllowedUsersSettings) {
	logging.Infof("Updating access control settings - Mode: %s, Read: %s, Write: %s",
		settings.Mode, settings.Read, settings.Write)
	ac.settings = settings
	// Invalidate all cached access results since settings changed
	ac.InvalidateCache()
}

// InvalidateCache clears all cached access check results.
// Call this when allowed users, subscriptions, or settings change.
func (ac *AccessControl) InvalidateCache() {
	ac.accessCache.Range(func(key, value interface{}) bool {
		ac.accessCache.Delete(key)
		return true
	})
}
