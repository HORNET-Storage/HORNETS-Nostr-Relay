package web

import (
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	kind411creator "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind411"
	"github.com/HORNET-Storage/hornet-storage/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/subscription"
	"github.com/gofiber/fiber/v2"
	jsoniter "github.com/json-iterator/go"
	"github.com/spf13/viper"
)

// Settings group registry maps config key prefixes to their respective types
var settingsRegistry = map[string]interface{}{
	"relay_settings":   types.RelaySettings{},
	"image_moderation": types.ImageModerationSettings{},
	"content_filter":   types.ContentFilterSettings{},
	"nest_feeder":      types.NestFeederSettings{},
	"ollama":           types.OllamaSettings{},
	"xnostr":           types.XNostrSettings{},
	"relay_info":       types.RelayInfoSettings{},
	"wallet":           types.WalletSettings{},
	"general":          types.GeneralSettings{},
	"query_cache":      map[string]interface{}{},
}

// Settings hooks for groups that need special handling after update
var settingsUpdateHooks = map[string]func(interface{}, stores.Store) error{
	"relay_settings": handleRelaySettingsUpdate,
	// Add more hooks as needed
}

// getConfigSettings handles GET requests for any settings group
func getConfigSettings(c *fiber.Ctx) error {
	groupName := c.Params("group")
	log.Printf("Get settings request received for group: %s", groupName)

	// Check if group exists in registry
	_, exists := settingsRegistry[groupName]
	if !exists {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Unknown settings group",
		})
	}

	// Special handling for relay settings
	if groupName == "relay_settings" {
		// Reuse existing relay settings handler
		return getRelaySettings(c)
	}

	// For other groups, fetch from Viper
	settings := fetchSettingsFromViper(groupName)
	return c.JSON(fiber.Map{
		groupName: settings,
	})
}

// updateConfigSettings handles POST requests to update any settings group
func updateConfigSettings(c *fiber.Ctx, store stores.Store) error {
	groupName := c.Params("group")
	log.Printf("Update settings request received for group: %s", groupName)

	// Check if group exists in registry
	groupType, exists := settingsRegistry[groupName]
	if !exists {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Unknown settings group",
		})
	}

	// Special handling for relay settings to maintain backward compatibility
	if groupName == "relay_settings" {
		return updateRelaySettings(c, store)
	}

	// Parse request body into a map
	var data map[string]interface{}
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	}

	// Extract the settings data from the request
	settingsData, ok := data[groupName]
	if !ok {
		return c.Status(fiber.StatusBadRequest).SendString(fmt.Sprintf("%s data expected", groupName))
	}

	// Validate and convert settings based on group type
	settings, err := validateAndConvertSettings(groupName, settingsData, groupType)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	}

	// Store in Viper
	err = storeSettingsInViper(groupName, settings)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to save settings")
	}

	// Run any post-update hooks
	if hook, hasHook := settingsUpdateHooks[groupName]; hasHook {
		if err := hook(settings, store); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}
	}

	return c.SendStatus(fiber.StatusOK)
}

// fetchSettingsFromViper retrieves settings for a group from Viper
func fetchSettingsFromViper(groupName string) interface{} {
	// For grouped settings like relay_settings
	if viper.IsSet(groupName) {
		return viper.Get(groupName)
	}

	// For flat settings with common prefixes
	prefix := groupName + "_"
	result := make(map[string]interface{})

	// For each key in Viper
	for _, key := range viper.AllKeys() {
		// If it belongs to this group
		if strings.HasPrefix(key, prefix) {
			// Add to result, removing the prefix
			shortKey := strings.TrimPrefix(key, prefix)
			result[shortKey] = viper.Get(key)
		} else if groupName == "relay_info" && strings.HasPrefix(key, "relay") && key != "relay_settings" && key != "relay_stats_db" {
			// Special case for relay_info which has keys starting with "relay"
			result[key] = viper.Get(key)
		} else if groupName == "general" {
			// General settings are those that don't fit in other categories
			if key == "port" || key == "private_key" || key == "proxy" ||
				key == "demo_mode" || key == "web" || key == "service_tag" ||
				key == "relay_stats_db" {
				result[key] = viper.Get(key)
			}
		}
	}

	// Special case for query_cache which is a direct key
	if groupName == "query_cache" && viper.IsSet("query_cache") {
		return viper.Get("query_cache")
	}

	return result
}

// storeSettingsInViper stores settings for a group in Viper
func storeSettingsInViper(groupName string, settings interface{}) error {
	// For grouped settings like relay_settings
	if groupName == "relay_settings" {
		viper.Set(groupName, settings)
		return viper.WriteConfig()
	}

	// For query_cache which is a direct key
	if groupName == "query_cache" {
		viper.Set(groupName, settings)
		return viper.WriteConfig()
	}

	// For flat settings
	settingsMap, err := convertToMap(settings)
	if err != nil {
		return err
	}

	// Special case for relay_info which has keys starting with "relay"
	if groupName == "relay_info" {
		for key, value := range settingsMap {
			viper.Set(key, value)
		}
		return viper.WriteConfig()
	}

	// Special case for general settings
	if groupName == "general" {
		for key, value := range settingsMap {
			viper.Set(key, value)
		}
		return viper.WriteConfig()
	}

	// For other groups with prefixed keys
	prefix := groupName + "_"
	for key, value := range settingsMap {
		viper.Set(prefix+key, value)
	}

	return viper.WriteConfig()
}

// validateAndConvertSettings validates and converts settings data to the correct type
func validateAndConvertSettings(groupName string, data interface{}, targetType interface{}) (interface{}, error) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	// Special handling for map types (like query_cache)
	if _, isMap := targetType.(map[string]interface{}); isMap {
		dataMap, ok := data.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid format for %s", groupName)
		}
		return dataMap, nil
	}

	// For struct types
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	// Create a new instance of the target type
	targetValue := reflect.New(reflect.TypeOf(targetType)).Interface()

	if err := json.Unmarshal(jsonData, targetValue); err != nil {
		return nil, err
	}

	// Extract the actual value from the pointer
	return reflect.ValueOf(targetValue).Elem().Interface(), nil
}

// convertToMap converts a struct to a map using JSON tags
func convertToMap(obj interface{}) (map[string]interface{}, error) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	jsonData, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(jsonData, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// handleRelaySettingsUpdate implements special handling for relay settings
func handleRelaySettingsUpdate(settings interface{}, store stores.Store) error {
	relaySettings, ok := settings.(types.RelaySettings)
	if !ok {
		return fmt.Errorf("invalid relay settings type")
	}

	// Get current settings to check if tiers have changed
	var currentRelaySettings types.RelaySettings
	err := viper.UnmarshalKey("relay_settings", &currentRelaySettings)
	if err != nil {
		return fmt.Errorf("error unmarshaling relay settings: %s", err)
	}

	// Check if tiers or free tier settings have changed
	needsKind411Update := tiersChanged(currentRelaySettings.SubscriptionTiers, relaySettings.SubscriptionTiers) ||
		currentRelaySettings.FreeTierEnabled != relaySettings.FreeTierEnabled ||
		currentRelaySettings.FreeTierLimit != relaySettings.FreeTierLimit

	// Update events if tier settings have changed
	if needsKind411Update {
		// Schedule batch update for all kind 888 events with 30-minute cooldown
		subscription.ScheduleBatchUpdateAfter(time.Minute * 30)
		log.Println("Scheduled batch update of kind 888 events with 30-minute cooldown")

		log.Println("Subscription tiers have changed, creating a new kind 411 event")

		serializedPrivateKey := viper.GetString("private_key")
		// Load private and public keys
		privateKey, publicKey, err := signing.DeserializePrivateKey(serializedPrivateKey)
		if err != nil {
			return fmt.Errorf("error loading keys: %s", err)
		}

		// Create kind 411 event using the provided store instance
		if err := kind411creator.CreateKind411Event(privateKey, publicKey, store); err != nil {
			return fmt.Errorf("error creating kind 411 event: %s", err)
		}
	}

	return nil
}
