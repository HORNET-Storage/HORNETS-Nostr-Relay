package access

import (
	"fmt"
	"strings"
	"time"

	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
)

// AccessControl handles permission checking for H.O.R.N.E.T Allowed Users
type AccessControl struct {
	statsStore statistics.StatisticsStore
	settings   *types.AllowedUsersSettings
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
	// Log the current settings being checked
	logging.Infof("Access check - Permission: %s, Current settings - Mode: %s, Read: %s, Write: %s",
		readOrWrite, ac.settings.Mode, ac.settings.Read, ac.settings.Write)

	// Everyone is allowed if all_users is set
	if readOrWrite == "all_users" {
		return nil
	}

	// Ensure we always use the hex encoded public key
	hex, err := sanitizePublicKey(npub)
	if err != nil {
		return err
	}

	// The owner is always allowed
	if ac.isOwner(npub) {
		logging.Infof("[ACCESS CONTROL] User %s is the relay owner, granting access", *hex)
		return nil
	}

	// Get the allowed user from the database
	logging.Infof("[ACCESS CONTROL] Looking up user %s in allowed_users table", *hex)
	user, err := ac.statsStore.GetAllowedUser(*hex)
	if err != nil {
		logging.Infof("[ACCESS CONTROL] Error looking up user %s: %v", *hex, err)
		return err
	}

	// User is not allowed if they don't exist
	if user == nil {
		logging.Infof("[ACCESS CONTROL] User %s not found in allowed_users table", *hex)
		return fmt.Errorf("user does not have permission to read")
	}
	
	logging.Infof("[ACCESS CONTROL] User %s found in allowed_users table with tier: %s", *hex, user.Tier)

	// Check if user has a paid tier if set to paid_users
	if readOrWrite == "paid_users" {
		logging.Infof("[ACCESS CONTROL] Checking paid subscriber status for user: %s", *hex)
		// Check if user exists in paid subscribers table
		paidSubscriber, err := ac.statsStore.GetPaidSubscriberByNpub(*hex)
		if err != nil {
			// Database error - log it but deny access
			logging.Infof("[ACCESS CONTROL] Error checking paid subscriber status: %v", err)
			return fmt.Errorf("user does not have permission")
		}

		if paidSubscriber == nil {
			logging.Infof("[ACCESS CONTROL] User %s not found in paid subscribers table", *hex)
			return fmt.Errorf("user does not have a paid subscription")
		}

		// Check if subscription is still valid
		if time.Now().After(paidSubscriber.ExpirationDate) {
			logging.Infof("[ACCESS CONTROL] User %s subscription expired on %v", *hex, paidSubscriber.ExpirationDate)
			return fmt.Errorf("user subscription has expired")
		}

		// Verify it's actually a paid tier (not a free tier that somehow got into the table)
		if paidSubscriber.Tier == "" {
			logging.Infof("[ACCESS CONTROL] User %s has empty tier", *hex)
			return fmt.Errorf("user does not have a valid subscription tier")
		}
		
		// Check if the tier is actually a paid tier by checking the current configuration
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
				logging.Infof("[ACCESS CONTROL] User %s has free/unpaid tier: %s (not valid for paid_users access)", *hex, paidSubscriber.Tier)
				return fmt.Errorf("user has a free tier, not a paid subscription")
			}
		} else {
			// Fallback: check if the tier name suggests it's free
			if strings.Contains(strings.ToLower(paidSubscriber.Tier), "free") || strings.Contains(strings.ToLower(paidSubscriber.Tier), "basic") {
				logging.Infof("[ACCESS CONTROL] User %s has free tier: %s (not valid for paid_users access)", *hex, paidSubscriber.Tier)
				return fmt.Errorf("user has a free tier, not a paid subscription")
			}
		}
		
		logging.Infof("[ACCESS CONTROL] User %s has valid paid subscription with tier: %s", *hex, paidSubscriber.Tier)
	}

	return nil
}

func (ac *AccessControl) AddAllowedUser(npub string, read bool, write bool, tier string, createdBy string) error {
	return ac.statsStore.AddAllowedUser(npub, tier, createdBy)
}

func (ac *AccessControl) RemoveAllowedUser(npub string) error {
	return ac.statsStore.RemoveAllowedUser(npub)
}

// Sanitizes the public key to ensure it is always the same hex format
func sanitizePublicKey(serializedPublicKey string) (hex *string, err error) {
	pubKey, err := signing.DeserializePublicKey(serializedPublicKey)
	if err != nil {
		return nil, err
	}

	hexKey, err := signing.SerializePublicKey(pubKey)
	if err != nil {
		return nil, err
	}

	return hexKey, nil
}

// Is the incoming pub key the owner of the relay
func (ac *AccessControl) isOwner(hex string) bool {
	// First check database for relay owner
	if ac.statsStore != nil {
		owner, err := ac.statsStore.GetRelayOwner()
		if err == nil && owner != nil {
			ownerHex, err := sanitizePublicKey(owner.Npub)
			if err == nil && hex == *ownerHex {
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

	logging.Infof("Write setting %s", write)
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

// UpdateSettings updates the access control settings
func (ac *AccessControl) UpdateSettings(settings *types.AllowedUsersSettings) {
	logging.Infof("Updating access control settings - Mode: %s, Read: %s, Write: %s",
		settings.Mode, settings.Read, settings.Write)
	ac.settings = settings
}
