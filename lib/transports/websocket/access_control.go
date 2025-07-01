package websocket

import (
	"log"
	"sync"

	"github.com/HORNET-Storage/hornet-storage/lib/access"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics"
	types "github.com/HORNET-Storage/hornet-storage/lib/types"
	"github.com/spf13/viper"
)

var (
	globalAccessControl *access.AccessControl
	accessControlMutex  sync.RWMutex
)

// InitializeAccessControl initializes the global access control with the given statistics store
func InitializeAccessControl(statsStore statistics.StatisticsStore) error {
	accessControlMutex.Lock()
	defer accessControlMutex.Unlock()

	// Load allowed users settings from configuration
	var allowedUsersSettings types.AllowedUsersSettings
	if err := viper.UnmarshalKey("allowed_users", &allowedUsersSettings); err != nil {
		// If no settings found, use default free mode
		log.Printf("No allowed users settings found, using default free mode: %v", err)
		allowedUsersSettings = types.AllowedUsersSettings{
			Mode:  "only-me",
			Read:  "only-me",
			Write: "only-me",
		}
	}

	// Create the access control instance
	globalAccessControl = access.NewAccessControl(statsStore, &allowedUsersSettings)

	log.Printf("Access control initialized in %s mode", allowedUsersSettings.Mode)
	return nil
}

// GetAccessControl returns the global access control instance
func GetAccessControl() *access.AccessControl {
	accessControlMutex.RLock()
	defer accessControlMutex.RUnlock()
	return globalAccessControl
}

// UpdateAccessControlSettings updates the access control settings
func UpdateAccessControlSettings(settings *types.AllowedUsersSettings) error {
	accessControlMutex.Lock()
	defer accessControlMutex.Unlock()

	if globalAccessControl == nil {
		return nil // Not initialized yet
	}

	// Validate settings
	if err := globalAccessControl.ValidateSettings(settings); err != nil {
		return err
	}

	// Update settings
	globalAccessControl.UpdateSettings(settings)

	log.Printf("Access control settings updated to %s mode", settings.Mode)
	return nil
}
