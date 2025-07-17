package websocket

import (
	"sync"

	"github.com/HORNET-Storage/hornet-storage/lib/access"
	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics"
	types "github.com/HORNET-Storage/hornet-storage/lib/types"
)

var (
	globalAccessControl *access.AccessControl
	accessControlMutex  sync.RWMutex
)

// InitializeAccessControl initializes the global access control with the given statistics store
func InitializeAccessControl(statsStore statistics.StatisticsStore) error {
	accessControlMutex.Lock()
	defer accessControlMutex.Unlock()

	// Load allowed users settings from cached configuration
	cfg, err := config.GetConfig()
	if err != nil {
		logging.Infof("Error getting config, using default settings: %v", err)
		allowedUsersSettings := types.AllowedUsersSettings{
			Mode:  "only-me",
			Read:  "only-me",
			Write: "only-me",
		}
		globalAccessControl = access.NewAccessControl(statsStore, &allowedUsersSettings)
		return nil
	}

	allowedUsersSettings := cfg.AllowedUsersSettings

	// Create the access control instance
	globalAccessControl = access.NewAccessControl(statsStore, &allowedUsersSettings)

	logging.Infof("Access control initialized in %s mode", allowedUsersSettings.Mode)
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

	logging.Infof("UpdateAccessControlSettings called - Mode: %s, Read: %s, Write: %s",
		settings.Mode, settings.Read, settings.Write)

	if globalAccessControl == nil {
		logging.Infof("Warning: globalAccessControl is nil, cannot update settings")
		return nil // Not initialized yet
	}

	// Validate settings
	if err := globalAccessControl.ValidateSettings(settings); err != nil {
		logging.Infof("Settings validation failed: %v", err)
		return err
	}

	logging.Infof("After validation - Mode: %s, Read: %s, Write: %s",
		settings.Mode, settings.Read, settings.Write)

	// Update settings
	globalAccessControl.UpdateSettings(settings)

	logging.Infof("Access control settings successfully updated to %s mode with Read: %s, Write: %s",
		settings.Mode, settings.Read, settings.Write)
	return nil
}
