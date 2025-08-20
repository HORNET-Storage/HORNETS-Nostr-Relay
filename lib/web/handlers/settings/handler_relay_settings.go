package settings

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind10411"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/subscription"
	"github.com/HORNET-Storage/hornet-storage/lib/transports/websocket"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
	"github.com/HORNET-Storage/hornet-storage/services/push"
)

// GetSettings returns the entire configuration
func GetSettings(c *fiber.Ctx) error {
	logging.Info("Get settings request received")

	// Use the new thread-safe helper function to get all settings
	// This avoids the concurrent map read/write error from viper.AllSettings()
	settings, err := config.GetAllSettingsAsMap()
	if err != nil {
		logging.Infof("Error getting settings: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to retrieve configuration",
		})
	}

	// Clean prefixed keys before sending to frontend
	cleanSettingsForFrontend(settings)

	// Log successful configuration sections retrieval
	logging.Infof("Successfully retrieved configuration sections: %v", func() []string {
		var sections []string
		for key := range settings {
			sections = append(sections, key)
		}
		return sections
	}())

	response := fiber.Map{
		"settings": settings,
	}

	return c.JSON(response)
}

// Helper function to get map keys
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// normalizeDataTypes ensures proper data types for specific configuration fields
func normalizeDataTypes(settings map[string]interface{}) {
	// Handle relay.supported_nips - should be []int, not []string
	if relay, ok := settings["relay"].(map[string]interface{}); ok {
		if supportedNipsRaw, ok := relay["supported_nips"]; ok {
			var normalizedNips []int

			switch v := supportedNipsRaw.(type) {
			case []interface{}:
				for _, item := range v {
					switch nipValue := item.(type) {
					case string:
						// Convert string to int
						if nipInt, err := strconv.Atoi(nipValue); err == nil {
							normalizedNips = append(normalizedNips, nipInt)
							logging.Infof("Converted NIP string '%s' to int %d", nipValue, nipInt)
						} else {
							logging.Infof("Warning: Could not convert NIP '%s' to integer: %v", nipValue, err)
						}
					case float64:
						// JSON numbers come as float64
						normalizedNips = append(normalizedNips, int(nipValue))
					case int:
						normalizedNips = append(normalizedNips, nipValue)
					default:
						logging.Infof("Warning: Unexpected NIP type %T: %v", nipValue, nipValue)
					}
				}
			case []string:
				// Convert all strings to ints
				for _, nipStr := range v {
					if nipInt, err := strconv.Atoi(nipStr); err == nil {
						normalizedNips = append(normalizedNips, nipInt)
						logging.Infof("Converted NIP string '%s' to int %d", nipStr, nipInt)
					} else {
						logging.Infof("Warning: Could not convert NIP '%s' to integer: %v", nipStr, err)
					}
				}
			case []int:
				normalizedNips = v // Already correct type
			}

			if len(normalizedNips) > 0 {
				relay["supported_nips"] = normalizedNips
				logging.Infof("Normalized supported_nips to integers: %v", normalizedNips)
			}
		}
	}
}

// validateModeSwitch validates mode changes, especially switching to subscription mode
func validateModeSwitch(currentSettings, newSettings *types.AllowedUsersSettings, store stores.Store) error {
	// Check if switching TO subscription mode from another mode
	if newSettings.Mode == "subscription" && currentSettings.Mode != "subscription" {
		logging.Infof("Mode switch detected: %s -> subscription. Checking Bitcoin address availability...", currentSettings.Mode)

		// First, check if wallet service is reachable
		walletHealthy, err := subscription.CheckWalletServiceHealth()
		if err != nil || !walletHealthy {
			return fmt.Errorf("cannot switch to subscription mode: wallet service is not available")
		}

		// Check if we have Bitcoin addresses in the pool
		statsStore := store.GetStatsStore()
		addressCount, err := statsStore.GetAvailableBitcoinAddressCount()
		if err != nil {
			return fmt.Errorf("failed to check Bitcoin address availability: %v", err)
		}

		// Check if existing users need addresses
		usersWithoutAddresses, err := statsStore.CountUsersWithoutBitcoinAddresses()
		if err != nil {
			return fmt.Errorf("failed to check users without addresses: %v", err)
		}

		// Define minimum buffer (e.g., 20% extra addresses or at least 100 addresses)
		bufferSize := int(float64(usersWithoutAddresses) * 0.2)
		if bufferSize < 100 {
			bufferSize = 100
		}

		requiredAddresses := usersWithoutAddresses + bufferSize

		if addressCount < requiredAddresses {
			addressesNeeded := requiredAddresses - addressCount
			logging.Infof("Insufficient Bitcoin addresses (%d needed + %d buffer = %d total required, but only %d available). Requesting %d more addresses...",
				usersWithoutAddresses, bufferSize, requiredAddresses, addressCount, addressesNeeded)

			// Request additional addresses from wallet service asynchronously
			go func() {
				subManager := subscription.GetGlobalManager()
				if subManager != nil {
					if err := subManager.RequestNewAddresses(addressesNeeded); err != nil {
						logging.Infof("Warning: Failed to request additional Bitcoin addresses: %v", err)
					} else {
						logging.Infof("Successfully requested %d additional Bitcoin addresses from wallet service", addressesNeeded)
					}
				}
			}()

			// Allow the mode switch to proceed - addresses will be generated in background
			logging.Infof("Mode switch proceeding - requested %d additional addresses, they will be available shortly", addressesNeeded)
		}

		logging.Infof("Mode switch validation passed: %d addresses available for %d users (with %d buffer)",
			addressCount, usersWithoutAddresses, bufferSize)
	}

	return nil
}

// UpdateSettings updates configuration values
func UpdateSettings(c *fiber.Ctx, store stores.Store) error {
	logging.Info("Update settings request received")

	var data map[string]interface{}
	if err := c.BodyParser(&data); err != nil {
		logging.Infof("Error parsing request body: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Extract settings from the request
	settings, ok := data["settings"].(map[string]interface{})
	if !ok {
		logging.Infof("ERROR: Settings data not found in request. Available keys: %v", getKeys(data))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Settings data expected",
		})
	}

	// NORMALIZATION: Ensure proper data types before saving
	normalizeDataTypes(settings)

	// Log the incoming settings for debugging
	logging.Infof("Received settings: %+v", settings)

	// CLEANUP: Convert incoming prefixed data to clean structure
	convertToCleanStructure(settings)
	logging.Infof("After cleanup: %+v", settings)

	// Check if allowed_users settings are being updated (affects subscription tiers)
	allowedUsersUpdated := false
	if _, exists := settings["allowed_users"]; exists {
		allowedUsersUpdated = true

		// VALIDATION: Check mode switch requirements before saving
		if allowedUsersInterface, ok := settings["allowed_users"].(map[string]interface{}); ok {
			// Get current config to compare mode changes
			currentConfig, err := config.GetConfig()
			if err != nil {
				logging.Infof("Error getting current config for validation: %v", err)
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "Failed to get current configuration for validation",
				})
			}

			// Convert the new settings to a proper struct for validation
			var newAllowedUsersSettings types.AllowedUsersSettings

			// Marshal to JSON and back to properly convert types
			jsonData, err := json.Marshal(allowedUsersInterface)
			if err != nil {
				logging.Infof("Error marshaling allowed_users settings: %v", err)
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "Invalid allowed_users settings format",
				})
			}

			if err := json.Unmarshal(jsonData, &newAllowedUsersSettings); err != nil {
				logging.Infof("Error unmarshaling allowed_users settings: %v", err)
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "Invalid allowed_users settings structure",
				})
			}

			// Validate the mode switch
			if err := validateModeSwitch(&currentConfig.AllowedUsersSettings, &newAllowedUsersSettings, store); err != nil {
				logging.Infof("Mode switch validation failed: %v", err)
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": fmt.Sprintf("Mode switch validation failed: %v", err),
				})
			}
		}

		// Set the last updated timestamp for allowed_users
		if allowedUsersMap, ok := settings["allowed_users"].(map[string]interface{}); ok {
			allowedUsersMap["last_updated"] = time.Now().Unix()
		}
	}

	// Check if relay settings are being updated (affects kind 10411 relay info)
	relaySettingsUpdated := false
	if _, exists := settings["relay"]; exists {
		relaySettingsUpdated = true
		logging.Info("Relay settings updated, will regenerate kind 10411 event...")
	}

	// Check if event_filtering settings are being updated (affects kind_whitelist, allow_unregistered_kinds, registered_kinds and supported_nips)
	eventFilteringUpdated := false
	if eventFilteringInterface, exists := settings["event_filtering"]; exists {
		eventFilteringUpdated = true

		// Extract event_filtering settings
		if eventFilteringMap, ok := eventFilteringInterface.(map[string]interface{}); ok {
			// Handle allow_unregistered_kinds field
			if allowUnregisteredInterface, exists := eventFilteringMap["allow_unregistered_kinds"]; exists {
				// Ensure it's a boolean
				switch v := allowUnregisteredInterface.(type) {
				case bool:
					logging.Infof("Allow unregistered kinds set to: %v", v)
				default:
					logging.Infof("WARNING: allow_unregistered_kinds should be boolean, got %T", allowUnregisteredInterface)
				}
			}

			// Handle registered_kinds field
			if registeredKindsInterface, exists := eventFilteringMap["registered_kinds"]; exists {
				var registeredKinds []int

				// Handle both slice and interface slice formats
				switch kinds := registeredKindsInterface.(type) {
				case []int:
					registeredKinds = kinds
				case []interface{}:
					for _, kind := range kinds {
						switch k := kind.(type) {
						case int:
							registeredKinds = append(registeredKinds, k)
						case float64:
							// JSON numbers come as float64
							registeredKinds = append(registeredKinds, int(k))
						case string:
							// Try to parse string to int
							if kindInt, err := strconv.Atoi(k); err == nil {
								registeredKinds = append(registeredKinds, kindInt)
							} else {
								logging.Infof("WARNING: Could not parse registered kind '%s' to int: %v", k, err)
							}
						default:
							logging.Infof("WARNING: Unexpected registered kind type %T: %v", k, k)
						}
					}
				default:
					logging.Infof("WARNING: Unexpected registered_kinds type: %T", registeredKindsInterface)
				}

				// Update the registered_kinds in the map
				if len(registeredKinds) > 0 {
					eventFilteringMap["registered_kinds"] = registeredKinds
					logging.Infof("Normalized registered_kinds to integers: %v", registeredKinds)
				}
			}

			// Handle kind_whitelist
			if kindWhitelistInterface, exists := eventFilteringMap["kind_whitelist"]; exists {
				// kind_whitelist is a slice/array, not a map
				var enabledKinds []string

				// Handle both slice and interface slice formats
				switch kindWhitelist := kindWhitelistInterface.(type) {
				case []string:
					enabledKinds = kindWhitelist
				case []interface{}:
					for _, kind := range kindWhitelist {
						if kindStr, ok := kind.(string); ok {
							enabledKinds = append(enabledKinds, kindStr)
						}
					}
				default:
					logging.Infof("DEBUG: Unexpected kind_whitelist type: %T", kindWhitelistInterface)
				}

				// Calculate supported NIPs from enabled kinds using config
				supportedNIPs, err := config.GetSupportedNIPsFromKinds(enabledKinds)
				if err != nil {
					logging.Infof("Error calculating supported NIPs from kinds: %v", err)
				} else {
					// Update supported_nips in relay settings (create relay section if it doesn't exist)
					if _, exists := settings["relay"]; !exists {
						settings["relay"] = make(map[string]interface{})
						logging.Infof("DEBUG: Created relay section in settings")
					}
					if relayMap, ok := settings["relay"].(map[string]interface{}); ok {
						relayMap["supported_nips"] = supportedNIPs
						logging.Infof("Updated supported_nips based on kind_whitelist: %v", supportedNIPs)
					}

					// Note: The supportedNIPs will be saved through the config.UpdateConfig call below
					// We don't need to call viper.Set directly anymore
				}
			}
		}
	}

	// Check if push notification settings are being updated
	pushNotificationsUpdated := false
	if _, exists := settings["push_notifications"]; exists {
		pushNotificationsUpdated = true
		logging.Info("Push notification settings updated, will reload push service...")
	}

	// Update each setting using thread-safe config functions
	// Use save=true on the last setting to persist all changes at once
	settingKeys := make([]string, 0, len(settings))
	for key := range settings {
		settingKeys = append(settingKeys, key)
	}

	for i, key := range settingKeys {
		value := settings[key]
		// Save on the last setting to persist all changes together
		shouldSave := (i == len(settingKeys)-1)

		logging.Infof("Setting %s = %v (type: %T, save: %v)", key, value, value, shouldSave)
		if err := config.UpdateConfig(key, value, shouldSave); err != nil {
			logging.Infof("Error updating config key %s: %v", key, err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": fmt.Sprintf("Failed to update setting %s", key),
			})
		}
	}

	// Config is already refreshed by SaveConfig(), so we can remove this
	if err := config.RefreshConfig(); err != nil {
		logging.Infof("Warning: Failed to refresh config cache: %v", err)
		// Don't fail the request, just log the warning
	}

	// If allowed_users settings were updated, update access control and trigger event regeneration
	if allowedUsersUpdated {
		logging.Info("Allowed users settings updated, updating access control and triggering event regeneration...")

		// Update the access control settings immediately
		if allowedUsersInterface, ok := settings["allowed_users"].(map[string]interface{}); ok {
			logging.Infof("Updating access control from settings: %+v", allowedUsersInterface)

			var newAllowedUsersSettings types.AllowedUsersSettings

			// Marshal to JSON and back to properly convert types
			jsonData, err := json.Marshal(allowedUsersInterface)
			if err == nil {
				if err := json.Unmarshal(jsonData, &newAllowedUsersSettings); err == nil {
					logging.Infof("Parsed settings - Mode: %s, Read: %s, Write: %s",
						newAllowedUsersSettings.Mode, newAllowedUsersSettings.Read, newAllowedUsersSettings.Write)

					// Update the global access control
					if err := websocket.UpdateAccessControlSettings(&newAllowedUsersSettings); err != nil {
						logging.Infof("Warning: Failed to update access control settings: %v", err)
					} else {
						logging.Info("Access control settings updated successfully")
					}
				} else {
					logging.Infof("Error unmarshaling allowed users settings: %v", err)
				}
			} else {
				logging.Infof("Error marshaling allowed users settings: %v", err)
			}
		} else {
			logging.Info("No allowed_users settings found in update")
		}

		// Schedule batch update of kind 11888 events after a short delay
		// This allows for multiple rapid setting changes to be batched together
		subscription.ScheduleBatchUpdateAfter(5 * time.Second)
	}

	// If either allowed_users, relay settings, or event filtering were updated, regenerate kind 10411 event
	if allowedUsersUpdated || relaySettingsUpdated || eventFilteringUpdated {
		// Regenerate kind 10411 event immediately in a goroutine
		if store != nil {
			go func() {
				var reason string
				var reasons []string
				if allowedUsersUpdated {
					reasons = append(reasons, "subscription tier")
				}
				if relaySettingsUpdated {
					reasons = append(reasons, "relay settings")
				}
				if eventFilteringUpdated {
					reasons = append(reasons, "event filtering")
				}
				if len(reasons) > 0 {
					reason = strings.Join(reasons, " and ") + " changes"
				} else {
					reason = "settings changes"
				}

				logging.Infof("Regenerating kind 10411 event due to %s...", reason)

				// Get the private and public keys from config (thread-safe)
				cfg, err := config.GetConfig()
				if err != nil {
					logging.Infof("Error getting config: %v", err)
					return
				}

				serializedPrivateKey := cfg.Relay.PrivateKey
				if len(serializedPrivateKey) <= 0 {
					logging.Infof("Error: No private key found in configuration")
					return
				}

				privateKey, publicKey, err := signing.DeserializePrivateKey(serializedPrivateKey)
				if err != nil {
					logging.Infof("Error deserializing private key: %v", err)
					return
				}

				// Use the existing store instance passed from the web server
				// This avoids the database lock issue
				if err := kind10411.CreateKind10411Event(privateKey, publicKey, store); err != nil {
					logging.Infof("Error regenerating kind 10411 event: %v", err)
				} else {
					logging.Infof("Successfully regenerated kind 10411 event")
				}
			}()
		} else {
			logging.Infof("Warning: Store not available, skipping kind 10411 regeneration")
		}
	}

	// If push notification settings were updated, reload the push service
	if pushNotificationsUpdated {
		logging.Info("Reloading push notification service with new configuration...")

		// Reload the service with new configuration
		statsStore := store.GetStatsStore()
		if err := push.ReloadGlobalPushService(statsStore); err != nil {
			logging.Infof("Warning: Failed to reload push notification service: %v", err)
			// Don't fail the request, just log the warning
			// The service will pick up the new config on next restart
		} else {
			logging.Info("Push notification service reloaded successfully")
		}
	}

	logging.Info("Settings updated successfully")
	return c.JSON(fiber.Map{
		"success": true,
		"message": "Settings updated successfully",
	})
}

// GetSettingValue returns a specific setting value
func GetSettingValue(c *fiber.Ctx) error {
	key := c.Params("key")
	if key == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Setting key required",
		})
	}

	// Use the thread-safe helper function to get a specific setting value
	value, err := config.GetSettingValue(key)
	if err != nil {
		logging.Infof("Error getting setting value for key %s: %v", key, err)
		// Return nil value instead of error for missing keys (backward compatibility)
		value = nil
	}

	return c.JSON(fiber.Map{
		"key":   key,
		"value": value,
	})
}

// UpdateSettingValue updates a specific setting value
func UpdateSettingValue(c *fiber.Ctx) error {
	key := c.Params("key")
	if key == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Setting key required",
		})
	}

	var data map[string]interface{}
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	value, ok := data["value"]
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Value required",
		})
	}

	// Update the setting using thread-safe config function
	if err := config.UpdateConfig(key, value, true); err != nil {
		logging.Infof("Error updating config: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to save setting",
		})
	}

	// Refresh the cached configuration
	if err := config.RefreshConfig(); err != nil {
		logging.Infof("Warning: Failed to refresh config cache: %v", err)
		// Don't fail the request, just log the warning
	}

	return c.JSON(fiber.Map{
		"success": true,
		"key":     key,
		"value":   value,
	})
}

// cleanSettingsForFrontend removes prefixed keys before sending to frontend
func cleanSettingsForFrontend(settings map[string]interface{}) {
	for _, sectionValue := range settings {
		if sectionMap, ok := sectionValue.(map[string]interface{}); ok {
			for subSectionName, subSectionValue := range sectionMap {
				if subSectionMap, ok := subSectionValue.(map[string]interface{}); ok {
					removePrefixedKeys(subSectionMap, subSectionName)
				}
			}
		}
	}
}

// convertToCleanStructure converts incoming prefixed data to clean structure
func convertToCleanStructure(settings map[string]interface{}) {
	for _, sectionValue := range settings {
		if sectionMap, ok := sectionValue.(map[string]interface{}); ok {
			for subSectionName, subSectionValue := range sectionMap {
				if subSectionMap, ok := subSectionValue.(map[string]interface{}); ok {
					convertPrefixedToClean(subSectionMap, subSectionName)
				}
			}
		}
	}
}

// removePrefixedKeys removes all prefixed keys, keeping only clean ones
func removePrefixedKeys(m map[string]interface{}, sectionName string) {
	var prefix string
	if sectionName == "text_filter" {
		prefix = "content_filter_"
	} else {
		prefix = sectionName + "_"
	}

	keysToDelete := []string{}
	for key := range m {
		if strings.HasPrefix(key, prefix) {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		delete(m, key)
	}
}

// convertPrefixedToClean converts prefixed keys to clean ones and removes prefixed versions
func convertPrefixedToClean(m map[string]interface{}, sectionName string) {
	var prefix string
	if sectionName == "text_filter" {
		prefix = "content_filter_"
	} else {
		prefix = sectionName + "_"
	}

	conversions := make(map[string]interface{})
	keysToDelete := []string{}

	// Find prefixed keys and prepare clean versions
	for key, value := range m {
		if strings.HasPrefix(key, prefix) {
			cleanKey := strings.TrimPrefix(key, prefix)
			// Only add if clean key doesn't already exist
			if _, exists := m[cleanKey]; !exists {
				conversions[cleanKey] = value
				logging.Infof("Converting %s -> %s", key, cleanKey)
			}
			keysToDelete = append(keysToDelete, key)
		}
	}

	// Add clean keys
	for cleanKey, value := range conversions {
		m[cleanKey] = value
	}

	// Remove prefixed keys
	for _, key := range keysToDelete {
		delete(m, key)
	}
}
