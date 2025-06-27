package access

import (
	"fmt"
	"strings"

	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
	"github.com/spf13/viper"
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

func (ac *AccessControl) CanWrite(npub string) error {
	if ac.settings.Read == "all_users" {
		return nil
	}

	hex, err := sanitizePublicKey(npub)
	if err != nil {
		return err
	}

	if isOwner(npub) {
		return nil
	}

	user, err := ac.statsStore.GetAllowedUser(*hex)
	if err != nil {
		return err
	}

	if !user.Write {
		return fmt.Errorf("user does not have permission to write")
	}

	return nil
}

func (ac *AccessControl) CanRead(npub string) error {
	if ac.settings.Read == "all_users" {
		return nil
	}

	hex, err := sanitizePublicKey(npub)
	if err != nil {
		return err
	}

	if isOwner(npub) {
		return nil
	}

	user, err := ac.statsStore.GetAllowedUser(*hex)
	if err != nil {
		return err
	}

	if !user.Read {
		return fmt.Errorf("user does not have permission to read")
	}

	return nil
}

func (ac *AccessControl) AddAllowedUser(npub string, read bool, write bool, tier string, createdBy string) error {
	return ac.statsStore.AddAllowedUser(npub, read, write, tier, createdBy)
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

// Is the incomming pub key the owner of the relay
func isOwner(hex string) bool {
	ownerKey := viper.GetString("relay.public_key")
	ownerHex, err := sanitizePublicKey(ownerKey)
	if err != nil {
		return false
	}

	if hex != *ownerHex {
		return false
	}

	return true
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

	// This ensures the correct options are selected for each mode and sets defaults when incorrect values are set
	// Not all read/write values are valid for each mode so this ensures that the read/write values are in line with the selected mode
	// mode: 		only_me, invite_only, public, subscription
	// read/write: 	all_users, paid_users, allowed_users, only_me

	switch mode {
	case "only_me":
		write = "only_me"
		switch read {
		case "only_me":
		case "all_users":
		case "allowed_users":
		default:
			read = "only_me"
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
		mode = "only_me"
		read = "only_me"
		write = "only_me"
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
	ac.settings = settings
}
