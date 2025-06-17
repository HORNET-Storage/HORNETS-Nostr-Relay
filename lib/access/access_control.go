package access

import (
	"fmt"
	"log"
	"strings"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics"
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

// UpdateSettings updates the access control settings
func (ac *AccessControl) UpdateSettings(settings *types.AllowedUsersSettings) {
	ac.settings = settings
}

// CanRead checks if an NPUB has read access based on current mode and settings
func (ac *AccessControl) CanRead(npub string) (bool, error) {
	if ac.settings == nil {
		return false, fmt.Errorf("access control settings not initialized")
	}

	// If read access is disabled, deny all read access
	if !ac.settings.ReadAccess.Enabled {
		return false, nil
	}

	switch strings.ToLower(ac.settings.Mode) {
	case "free":
		return ac.canReadFreeMode(npub)
	case "paid":
		return ac.canReadPaidMode(npub)
	case "exclusive":
		return ac.canReadExclusiveMode(npub)
	default:
		log.Printf("Unknown access control mode: %s", ac.settings.Mode)
		return false, fmt.Errorf("unknown access control mode: %s", ac.settings.Mode)
	}
}

// CanWrite checks if an NPUB has write access based on current mode and settings
func (ac *AccessControl) CanWrite(npub string) (bool, error) {
	if ac.settings == nil {
		return false, fmt.Errorf("access control settings not initialized")
	}

	// If write access is disabled, deny all write access
	if !ac.settings.WriteAccess.Enabled {
		return false, nil
	}

	switch strings.ToLower(ac.settings.Mode) {
	case "free":
		return ac.canWriteFreeMode(npub)
	case "paid":
		return ac.canWritePaidMode(npub)
	case "exclusive":
		return ac.canWriteExclusiveMode(npub)
	default:
		log.Printf("Unknown access control mode: %s", ac.settings.Mode)
		return false, fmt.Errorf("unknown access control mode: %s", ac.settings.Mode)
	}
}

// GetUserTier gets the tier assignment for an NPUB based on current mode
func (ac *AccessControl) GetUserTier(npub string) (string, error) {
	if ac.settings == nil {
		return "", fmt.Errorf("access control settings not initialized")
	}

	switch strings.ToLower(ac.settings.Mode) {
	case "free":
		return ac.getUserTierFreeMode(npub)
	case "paid":
		return ac.getUserTierPaidMode(npub)
	case "exclusive":
		return ac.getUserTierExclusiveMode(npub)
	default:
		return "", fmt.Errorf("unknown access control mode: %s", ac.settings.Mode)
	}
}

// Mode-specific read access methods

func (ac *AccessControl) canReadFreeMode(_ string) (bool, error) {
	// In free mode, read access scope determines permissions
	switch strings.ToLower(ac.settings.ReadAccess.Scope) {
	case "all_users", "":
		// Everyone can read in free mode
		return true, nil
	default:
		log.Printf("Unknown read access scope for free mode: %s", ac.settings.ReadAccess.Scope)
		return true, nil // Default to open access for free mode
	}
}

func (ac *AccessControl) canReadPaidMode(npub string) (bool, error) {
	switch strings.ToLower(ac.settings.ReadAccess.Scope) {
	case "all_users":
		// Anyone can read, regardless of payment status
		return true, nil
	case "paid_users":
		// Only paid subscribers can read
		return ac.isPaidSubscriber(npub)
	default:
		log.Printf("Unknown read access scope for paid mode: %s", ac.settings.ReadAccess.Scope)
		return false, nil
	}
}

func (ac *AccessControl) canReadExclusiveMode(npub string) (bool, error) {
	switch strings.ToLower(ac.settings.ReadAccess.Scope) {
	case "all_users":
		// Anyone can read (displays warning about public access)
		return true, nil
	case "allowed_users":
		// Only NPUBs in the allowed read list can read
		return ac.statsStore.IsNpubInAllowedReadList(npub)
	default:
		log.Printf("Unknown read access scope for exclusive mode: %s", ac.settings.ReadAccess.Scope)
		return false, nil
	}
}

// Mode-specific write access methods

func (ac *AccessControl) canWriteFreeMode(_ string) (bool, error) {
	// In free mode, write access is open to all users when enabled
	return true, nil
}

func (ac *AccessControl) canWritePaidMode(npub string) (bool, error) {
	// In paid mode, only paid subscribers can write
	return ac.isPaidSubscriber(npub)
}

func (ac *AccessControl) canWriteExclusiveMode(npub string) (bool, error) {
	// In exclusive mode, only NPUBs in the allowed write list can write
	return ac.statsStore.IsNpubInAllowedWriteList(npub)
}

// Mode-specific tier assignment methods

func (ac *AccessControl) getUserTierFreeMode(_ string) (string, error) {
	// In free mode, tiers are assigned based on free tier configuration
	// For now, return the first available free tier or "basic"
	if len(ac.settings.Tiers) > 0 {
		return "basic", nil // Default to basic tier for free users
	}
	return "basic", nil
}

func (ac *AccessControl) getUserTierPaidMode(npub string) (string, error) {
	// In paid mode, tier comes from subscription system
	subscriber, err := ac.statsStore.GetPaidSubscriberByNpub(npub)
	if err != nil {
		return "", err
	}
	if subscriber == nil {
		return "", fmt.Errorf("no paid subscription found for npub: %s", npub)
	}
	return subscriber.Tier, nil
}

func (ac *AccessControl) getUserTierExclusiveMode(npub string) (string, error) {
	// In exclusive mode, tier is manually assigned and stored in NPUB lists
	// Check both read and write lists for tier assignment
	tier, err := ac.statsStore.GetNpubTierFromReadList(npub)
	if err == nil && tier != "" {
		return tier, nil
	}

	tier, err = ac.statsStore.GetNpubTierFromWriteList(npub)
	if err == nil && tier != "" {
		return tier, nil
	}

	return "", fmt.Errorf("no tier assignment found for npub: %s", npub)
}

// Helper methods

func (ac *AccessControl) isPaidSubscriber(npub string) (bool, error) {
	subscriber, err := ac.statsStore.GetPaidSubscriberByNpub(npub)
	if err != nil {
		return false, err
	}
	return subscriber != nil, nil
}

// Bulk operations for NPUB management

// AddNpubToReadList adds an NPUB to the allowed read list with tier assignment
func (ac *AccessControl) AddNpubToReadList(npub, tierName, addedBy string) error {
	return ac.statsStore.AddNpubToReadList(npub, tierName, addedBy)
}

// AddNpubToWriteList adds an NPUB to the allowed write list with tier assignment
func (ac *AccessControl) AddNpubToWriteList(npub, tierName, addedBy string) error {
	return ac.statsStore.AddNpubToWriteList(npub, tierName, addedBy)
}

// RemoveNpubFromReadList removes an NPUB from the allowed read list
func (ac *AccessControl) RemoveNpubFromReadList(npub string) error {
	return ac.statsStore.RemoveNpubFromReadList(npub)
}

// RemoveNpubFromWriteList removes an NPUB from the allowed write list
func (ac *AccessControl) RemoveNpubFromWriteList(npub string) error {
	return ac.statsStore.RemoveNpubFromWriteList(npub)
}

// BulkImportReadNpubs imports multiple NPUBs to the read list
func (ac *AccessControl) BulkImportReadNpubs(npubs []types.AllowedReadNpub) error {
	return ac.statsStore.BulkAddNpubsToReadList(npubs)
}

// BulkImportWriteNpubs imports multiple NPUBs to the write list
func (ac *AccessControl) BulkImportWriteNpubs(npubs []types.AllowedWriteNpub) error {
	return ac.statsStore.BulkAddNpubsToWriteList(npubs)
}

// GetAllowedReadNpubs retrieves paginated allowed read NPUBs
func (ac *AccessControl) GetAllowedReadNpubs(page, pageSize int) ([]types.AllowedReadNpub, *types.PaginationMetadata, error) {
	return ac.statsStore.GetAllowedReadNpubs(page, pageSize)
}

// GetAllowedWriteNpubs retrieves paginated allowed write NPUBs
func (ac *AccessControl) GetAllowedWriteNpubs(page, pageSize int) ([]types.AllowedWriteNpub, *types.PaginationMetadata, error) {
	return ac.statsStore.GetAllowedWriteNpubs(page, pageSize)
}

// ValidateSettings validates the access control settings for consistency
func (ac *AccessControl) ValidateSettings(settings *types.AllowedUsersSettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
	}

	// Validate mode
	mode := strings.ToLower(settings.Mode)
	if mode != "free" && mode != "paid" && mode != "exclusive" {
		return fmt.Errorf("invalid mode: %s, must be 'free', 'paid', or 'exclusive'", settings.Mode)
	}

	// Validate read access scope based on mode
	if settings.ReadAccess.Enabled {
		switch mode {
		case "free":
			// Free mode supports "all_users" scope
			if settings.ReadAccess.Scope != "" && strings.ToLower(settings.ReadAccess.Scope) != "all_users" {
				return fmt.Errorf("free mode read access scope must be 'all_users' or empty")
			}
		case "paid":
			// Paid mode supports "all_users" or "paid_users"
			scope := strings.ToLower(settings.ReadAccess.Scope)
			if scope != "all_users" && scope != "paid_users" {
				return fmt.Errorf("paid mode read access scope must be 'all_users' or 'paid_users'")
			}
		case "exclusive":
			// Exclusive mode supports "all_users" or "allowed_users"
			scope := strings.ToLower(settings.ReadAccess.Scope)
			if scope != "all_users" && scope != "allowed_users" {
				return fmt.Errorf("exclusive mode read access scope must be 'all_users' or 'allowed_users'")
			}
		}
	}

	// Write access validation: scope is mode-dependent and handled automatically
	// Free: "all_users" when enabled
	// Paid: "paid_users" when enabled
	// Exclusive: "allowed_users" when enabled

	return nil
}

// GetAccessSummary returns a summary of current access control configuration
func (ac *AccessControl) GetAccessSummary() map[string]interface{} {
	if ac.settings == nil {
		return map[string]interface{}{
			"error": "settings not initialized",
		}
	}

	summary := map[string]interface{}{
		"mode":          ac.settings.Mode,
		"read_enabled":  ac.settings.ReadAccess.Enabled,
		"read_scope":    ac.settings.ReadAccess.Scope,
		"write_enabled": ac.settings.WriteAccess.Enabled,
		"tier_count":    len(ac.settings.Tiers),
		"last_updated":  ac.settings.LastUpdated,
	}

	// Add mode-specific information
	switch strings.ToLower(ac.settings.Mode) {
	case "free":
		summary["description"] = "Open access with configurable read/write permissions"
	case "paid":
		summary["description"] = "Bitcoin payment-gated access with automatic tier assignment"
	case "exclusive":
		summary["description"] = "Invitation-based access with manual NPUB curation"
	}

	return summary
}
