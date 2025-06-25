package settings

import (
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"

	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind411"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/subscription"
)

// GetSettings returns the entire configuration
func GetSettings(c *fiber.Ctx) error {
	log.Println("Get settings request received")

	// Return the entire config as JSON
	settings := viper.AllSettings()

	// Log the complete settings structure being sent to frontend
	log.Printf("=== SETTINGS RESPONSE START ===")
	log.Printf("Settings structure being sent to frontend:")

	// Log each major section
	if subscriptions, ok := settings["subscriptions"]; ok {
		log.Printf("subscriptions: %+v", subscriptions)
	} else {
		log.Printf("subscriptions: NOT FOUND")
	}

	if allowedUsers, ok := settings["allowed_users"]; ok {
		log.Printf("allowed_users: %+v", allowedUsers)
	} else {
		log.Printf("allowed_users: NOT FOUND")
	}

	if eventFiltering, ok := settings["event_filtering"]; ok {
		log.Printf("event_filtering: %+v", eventFiltering)
	} else {
		log.Printf("event_filtering: NOT FOUND")
	}

	if contentFiltering, ok := settings["content_filtering"]; ok {
		log.Printf("content_filtering: %+v", contentFiltering)
	} else {
		log.Printf("content_filtering: NOT FOUND")
	}

	if relay, ok := settings["relay"]; ok {
		log.Printf("relay: %+v", relay)
	} else {
		log.Printf("relay: NOT FOUND")
	}

	if server, ok := settings["server"]; ok {
		log.Printf("server: %+v", server)
	} else {
		log.Printf("server: NOT FOUND")
	}

	if externalServices, ok := settings["external_services"]; ok {
		log.Printf("external_services: %+v", externalServices)
	} else {
		log.Printf("external_services: NOT FOUND")
	}

	if logging, ok := settings["logging"]; ok {
		log.Printf("logging: %+v", logging)
	} else {
		log.Printf("logging: NOT FOUND")
	}

	log.Printf("Total settings keys: %d", len(settings))
	log.Printf("All top-level keys: %v", getKeys(settings))
	log.Printf("=== SETTINGS RESPONSE END ===")

	response := fiber.Map{
		"settings": settings,
	}

	log.Printf("Final response structure: %+v", response)

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
							log.Printf("Converted NIP string '%s' to int %d", nipValue, nipInt)
						} else {
							log.Printf("Warning: Could not convert NIP '%s' to integer: %v", nipValue, err)
						}
					case float64:
						// JSON numbers come as float64
						normalizedNips = append(normalizedNips, int(nipValue))
					case int:
						normalizedNips = append(normalizedNips, nipValue)
					default:
						log.Printf("Warning: Unexpected NIP type %T: %v", nipValue, nipValue)
					}
				}
			case []string:
				// Convert all strings to ints
				for _, nipStr := range v {
					if nipInt, err := strconv.Atoi(nipStr); err == nil {
						normalizedNips = append(normalizedNips, nipInt)
						log.Printf("Converted NIP string '%s' to int %d", nipStr, nipInt)
					} else {
						log.Printf("Warning: Could not convert NIP '%s' to integer: %v", nipStr, err)
					}
				}
			case []int:
				normalizedNips = v // Already correct type
			}

			if len(normalizedNips) > 0 {
				relay["supported_nips"] = normalizedNips
				log.Printf("Normalized supported_nips to integers: %v", normalizedNips)
			}
		}
	}
}

// UpdateSettings updates configuration values
func UpdateSettings(c *fiber.Ctx, store stores.Store) error {
	log.Println("Update settings request received")

	var data map[string]interface{}
	if err := c.BodyParser(&data); err != nil {
		log.Printf("Error parsing request body: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	log.Printf("=== UPDATE SETTINGS REQUEST START ===")
	log.Printf("Raw request data: %+v", data)

	// Extract settings from the request
	settings, ok := data["settings"].(map[string]interface{})
	if !ok {
		log.Printf("ERROR: Settings data not found in request. Available keys: %v", getKeys(data))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Settings data expected",
		})
	}

	log.Printf("Extracted settings from request: %+v", settings)
	log.Printf("Settings keys being updated: %v", getKeys(settings))

	// NORMALIZATION: Ensure proper data types before saving
	normalizeDataTypes(settings)

	// Check if allowed_users settings are being updated (affects subscription tiers)
	allowedUsersUpdated := false
	if _, exists := settings["allowed_users"]; exists {
		allowedUsersUpdated = true
		// Set the last updated timestamp for allowed_users
		if allowedUsersMap, ok := settings["allowed_users"].(map[string]interface{}); ok {
			allowedUsersMap["last_updated"] = time.Now().Unix()
		}
	}

	// Check if relay settings are being updated (affects kind 411 relay info)
	relaySettingsUpdated := false
	if _, exists := settings["relay"]; exists {
		relaySettingsUpdated = true
		log.Println("Relay settings updated, will regenerate kind 411 event...")
	}

	// Update each setting
	for key, value := range settings {
		log.Printf("Setting %s = %v (type: %T)", key, value, value)
		viper.Set(key, value)
	}

	log.Printf("=== UPDATE SETTINGS REQUEST END ===")

	// Save the configuration
	if err := viper.WriteConfig(); err != nil {
		log.Printf("Error writing config: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to save settings",
		})
	}

	// If allowed_users settings were updated, trigger event regeneration
	if allowedUsersUpdated {
		log.Println("Allowed users settings updated, triggering event regeneration...")

		// Schedule batch update of kind 888 events after a short delay
		// This allows for multiple rapid setting changes to be batched together
		subscription.ScheduleBatchUpdateAfter(5 * time.Second)
	}

	// If either allowed_users or relay settings were updated, regenerate kind 411 event
	if allowedUsersUpdated || relaySettingsUpdated {
		// Regenerate kind 411 event immediately in a goroutine
		if store != nil {
			go func() {
				var reason string
				if allowedUsersUpdated && relaySettingsUpdated {
					reason = "subscription tier and relay settings changes"
				} else if allowedUsersUpdated {
					reason = "subscription tier changes"
				} else {
					reason = "relay settings changes"
				}

				log.Printf("Regenerating kind 411 event due to %s...", reason)

				// Get the private and public keys from viper (same way as main.go does)
				serializedPrivateKey := viper.GetString("relay.private_key")
				if len(serializedPrivateKey) <= 0 {
					log.Printf("Error: No private key found in configuration")
					return
				}

				privateKey, publicKey, err := signing.DeserializePrivateKey(serializedPrivateKey)
				if err != nil {
					log.Printf("Error deserializing private key: %v", err)
					return
				}

				// Use the existing store instance passed from the web server
				// This avoids the database lock issue
				if err := kind411.CreateKind411Event(privateKey, publicKey, store); err != nil {
					log.Printf("Error regenerating kind 411 event: %v", err)
				} else {
					log.Printf("Successfully regenerated kind 411 event")
				}
			}()
		} else {
			log.Printf("Warning: Store not available, skipping kind 411 regeneration")
		}
	}

	log.Println("Settings updated successfully")
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

	value := viper.Get(key)
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

	// Update the setting
	viper.Set(key, value)

	// Save the configuration
	if err := viper.WriteConfig(); err != nil {
		log.Printf("Error writing config: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to save setting",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"key":     key,
		"value":   value,
	})
}
