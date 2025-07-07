// utils.go - Utility functions and helpers

package subscription

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/spf13/viper"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
)

// normalizePubkey converts both npub and hex formats to a consistent format for comparison
// Uses the signing package for decoding and go-nostr nip19 for npub encoding
func normalizePubkey(pubkey string) (hex string, npub string, err error) {
	// Use the signing package's DecodeKey which handles both hex and bech32
	keyBytes, err := signing.DecodeKey(pubkey)
	if err != nil {
		return "", "", fmt.Errorf("invalid pubkey format: %v", err)
	}

	// Convert to hex format
	hexKey := fmt.Sprintf("%x", keyBytes)

	// Convert to npub format using go-nostr nip19 (consistent with rest of codebase)
	npubKey, err := nip19.EncodePublicKey(hexKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to encode as npub: %v", err)
	}

	return hexKey, npubKey, nil
}

// getRelayMode reads the current relay mode from configuration
func (m *SubscriptionManager) getRelayMode() string {
	// Load allowed users settings from config to get current mode
	var allowedUsersSettings types.AllowedUsersSettings
	if err := viper.UnmarshalKey("allowed_users", &allowedUsersSettings); err != nil {
		logging.Infof("[DEBUG] Warning: could not load allowed_users settings: %v", err)
		return "unknown"
	}

	mode := strings.ToLower(allowedUsersSettings.Mode)
	logging.Infof("[DEBUG] Creating storage info for npub: mode=%s", mode)

	// Map new access control modes to subscription system modes
	switch mode {
	case "public":
		return "public"
	case "subscription":
		return "subscription"
	case "invite-only":
		return "invite-only"
	case "only-me":
		return "only-me"

	default:
		logging.Infof("[DEBUG] Unknown mode '%s', defaulting to 'unknown'", mode)
		return "unknown"
	}
}

// getTagValue extracts a tag value by key from nostr event tags
func getTagValue(tags []nostr.Tag, key string) string {
	for _, tag := range tags {
		if len(tag) > 1 && tag[0] == key {
			return tag[1]
		}
	}
	return ""
}

// getTagUnixValue extracts a unix timestamp from a tag (typically from index 2)
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

// checkAddressPoolStatus checks if we need to generate more addresses
func (m *SubscriptionManager) checkAddressPoolStatus() error {
	availableCount, err := m.store.GetStatsStore().CountAvailableAddresses()
	if err != nil {
		return fmt.Errorf("failed to count available addresses: %v", err)
	}

	logging.Infof("Available count: %v", availableCount)

	// If we have less than 50% of addresses available, request more
	if availableCount < 50 {
		logging.Infof("Address pool running low (%d available). Requesting 100 new addresses", availableCount)
		return m.RequestNewAddresses(20)
	}

	return nil
}

// getSubscriptionStatus returns the subscription status string
func (m *SubscriptionManager) getSubscriptionStatus(activeTier string) string {
	if activeTier == "" {
		return "inactive"
	}
	return "active"
}

// findAppropriateTierForUser determines the appropriate tier for a user based on mode and current status
func (m *SubscriptionManager) findAppropriateTierForUser(pubkey string, currentTier *types.SubscriptionTier, allowedUsersSettings *types.AllowedUsersSettings) *types.SubscriptionTier {
	mode := strings.ToLower(allowedUsersSettings.Mode)

	logging.Infof("DEBUG: findAppropriateTierForUser called with mode='%s', pubkey='%s'", mode, pubkey)
	logging.Infof("DEBUG: Available tiers in findAppropriateTierForUser: %d", len(allowedUsersSettings.Tiers))

	switch mode {
	case "public", "free":
		// In public/free mode, assign users to the first available free tier
		logging.Infof("DEBUG: Looking for free tier (PriceSats <= 0)")
		for i := range allowedUsersSettings.Tiers {
			tier := &allowedUsersSettings.Tiers[i]
			logging.Infof("DEBUG: Checking tier %d: Name='%s', PriceSats=%d, MonthlyLimitBytes=%d",
				i, tier.Name, tier.PriceSats, tier.MonthlyLimitBytes)
			if tier.PriceSats <= 0 {
				logging.Infof("DEBUG: Found free tier: %s with %d bytes", tier.Name, tier.MonthlyLimitBytes)
				return tier
			}
		}
		// Fallback to basic free allocation
		logging.Infof("DEBUG: No free tier found in config, using fallback")
		return &types.SubscriptionTier{
			Name:              "Basic Free",
			MonthlyLimitBytes: 100 * 1024 * 1024, // 100 MB
			PriceSats:         0,
			Unlimited:         false,
		}

	case "subscription", "paid":
		// In subscription/paid mode, give users the basic free tier by default
		// They can upgrade by making payments
		for i := range allowedUsersSettings.Tiers {
			if allowedUsersSettings.Tiers[i].PriceSats <= 0 {
				return &allowedUsersSettings.Tiers[i]
			}
		}
		// No free tier configured, use minimal allocation
		return &types.SubscriptionTier{
			Name:              "Basic Free",
			MonthlyLimitBytes: 100 * 1024 * 1024, // 100 MB
			PriceSats:         0,
			Unlimited:         false,
		}

	case "invite-only", "exclusive":
		// In invite-only/exclusive mode, tier is manually assigned
		// Get the user's assigned tier from the allowed users table
		statsStore := m.store.GetStatsStore()
		if statsStore != nil {
			// Normalize the pubkey to ensure we're using the right format
			hexKey, _, err := normalizePubkey(pubkey)
			if err != nil {
				logging.Infof("DEBUG: Error normalizing pubkey %s: %v", pubkey, err)
			} else {
				logging.Infof("DEBUG: Looking up allowed user with hex key: %s", hexKey)
				allowedUser, err := statsStore.GetAllowedUser(hexKey)
				if err != nil {
					logging.Infof("DEBUG: Error getting allowed user: %v", err)
				} else if allowedUser == nil {
					logging.Infof("DEBUG: User %s not found in allowed users table", hexKey)
				} else {
					logging.Infof("DEBUG: Found allowed user %s with tier: '%s'", hexKey, allowedUser.Tier)
					if allowedUser.Tier != "" {
						// Find the tier in the configuration that matches the user's assigned tier
						for i := range allowedUsersSettings.Tiers {
							if allowedUsersSettings.Tiers[i].Name == allowedUser.Tier {
								logging.Infof("User %s assigned tier '%s' from allowed users table", pubkey, allowedUser.Tier)
								return &allowedUsersSettings.Tiers[i]
							}
						}
						logging.Infof("Warning: User %s has tier '%s' but it's not in current config", pubkey, allowedUser.Tier)
					}
				}
			}
		} else {
			logging.Infof("DEBUG: Stats store is nil")
		}

		// Fallback: if user is in allowed list but no tier found, give first available tier
		if m.isUserInAllowedLists(pubkey) {
			if len(allowedUsersSettings.Tiers) > 0 {
				logging.Infof("User %s in allowed list but no tier assigned, using first available tier", pubkey)
				return &allowedUsersSettings.Tiers[0]
			}
		}

		logging.Infof("DEBUG: User %s not in allowed lists or no tier available", pubkey)
		// User not in allowed lists or no tier available
		return nil

	case "only-me", "personal":
		// In only-me/personal mode, only the relay owner gets access
		// Return current tier if they have one, otherwise first available tier
		if currentTier != nil {
			return currentTier
		}
		if len(allowedUsersSettings.Tiers) > 0 {
			return &allowedUsersSettings.Tiers[0]
		}
		return nil

	default:
		logging.Infof("Unknown mode: %s", allowedUsersSettings.Mode)
		return currentTier
	}
}
