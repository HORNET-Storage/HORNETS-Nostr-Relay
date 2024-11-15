package web

import (
	"log"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	kind411creator "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind411"
	"github.com/HORNET-Storage/hornet-storage/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
	jsoniter "github.com/json-iterator/go"
	"github.com/spf13/viper"
)

func updateRelaySettings(c *fiber.Ctx, store stores.Store) error {
	log.Println("Relay settings request received")
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	var data map[string]interface{}
	if err := c.BodyParser(&data); err != nil {
		return c.Status(400).SendString(err.Error())
	}

	relaySettingsData, ok := data["relay_settings"]
	if !ok {
		log.Println("Relay settings data not provided")
		return c.Status(400).SendString("Relay settings data expected")
	}

	var relaySettings types.RelaySettings
	relaySettingsJSON, err := json.Marshal(relaySettingsData)
	if err != nil {
		log.Println("Error marshaling relay settings:", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	if err := json.Unmarshal(relaySettingsJSON, &relaySettings); err != nil {
		log.Println("Error unmarshaling relay settings:", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Apply logic for boolean flags
	applyBooleanFlags(&relaySettings)

	// Fetch existing subscription tiers from Viper
	var existingTiers []types.SubscriptionTier
	if err := viper.UnmarshalKey("subscription_tiers", &existingTiers); err != nil {
		log.Println("Error fetching existing subscription tiers:", err)
	}

	// Compare existing tiers with the new tiers
	if tiersChanged(existingTiers, relaySettings.SubscriptionTiers) {
		log.Println("Subscription tiers have changed, creating a new kind 411 event")

		serializedPrivateKey := viper.GetString("private_key")
		// Load private and public keys
		privateKey, publicKey, err := signing.DeserializePrivateKey(serializedPrivateKey) // Assume a function to load private and public keys
		if err != nil {
			log.Println("Error loading keys:", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load keys")
		}

		// Create kind 411 event using the provided store instance
		if err := kind411creator.CreateKind411Event(privateKey, publicKey, store); err != nil {
			log.Println("Error creating kind 411 event:", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to create kind 411 event")
		}
	}

	// Update Viper configuration
	if err := updateViperConfig(relaySettings); err != nil {
		log.Printf("Error updating config: %s", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to update settings")
	}

	log.Println("Stored relay settings:", relaySettings)

	return c.SendStatus(fiber.StatusOK)
}

// Function to compare existing tiers with new tiers
func tiersChanged(existing, new []types.SubscriptionTier) bool {
	if len(existing) != len(new) {
		return true
	}

	for i := range existing {
		if existing[i].DataLimit != new[i].DataLimit || existing[i].Price != new[i].Price {
			return true
		}
	}

	return false
}

func applyBooleanFlags(settings *types.RelaySettings) {
	if !settings.IsKindsActive {
		settings.Kinds = []string{}
		settings.DynamicKinds = []string{}
	}
	if !settings.IsPhotosActive {
		settings.Photos = []string{}
	}
	if !settings.IsVideosActive {
		settings.Videos = []string{}
	}
	if !settings.IsGitNestrActive {
		settings.GitNestr = []string{}
	}
	if !settings.IsAudioActive {
		settings.Audio = []string{}
	}

	if settings.AppBuckets == nil {
		settings.AppBuckets = []string{}
	}

	log.Println("Dynamic app buckets: ", settings.DynamicAppBuckets)

	if settings.DynamicAppBuckets == nil {
		settings.DynamicAppBuckets = []string{}
	}

	if settings.Mode == "smart" {
		settings.DynamicKinds = []string{}
	}
}

func updateViperConfig(settings types.RelaySettings) error {
	viper.Set("relay_settings.Mode", settings.Mode)
	viper.Set("relay_settings.IsKindsActive", settings.IsKindsActive)
	viper.Set("relay_settings.IsPhotosActive", settings.IsPhotosActive)
	viper.Set("relay_settings.IsVideosActive", settings.IsVideosActive)
	viper.Set("relay_settings.IsGitNestrActive", settings.IsGitNestrActive)
	viper.Set("relay_settings.IsAudioActive", settings.IsAudioActive)
	viper.Set("relay_settings.IsFileStorageActive", settings.IsFileStorageActive)
	viper.Set("relay_settings.Kinds", settings.Kinds)
	viper.Set("relay_settings.DynamicKinds", settings.DynamicKinds)
	viper.Set("relay_settings.Photos", settings.Photos)
	viper.Set("relay_settings.Videos", settings.Videos)
	viper.Set("relay_settings.GitNestr", settings.GitNestr)
	viper.Set("relay_settings.Audio", settings.Audio)
	viper.Set("relay_settings.Protocol", settings.Protocol)
	viper.Set("relay_settings.AppBuckets", settings.AppBuckets)
	viper.Set("relay_settings.DynamicAppBuckets", settings.DynamicAppBuckets)
	viper.Set("subscription_tiers", settings.SubscriptionTiers)

	return viper.WriteConfig()
}

func getRelaySettings(c *fiber.Ctx) error {
	log.Println("Get relay settings request received")

	var relaySettings types.RelaySettings

	// Fetch settings from Viper
	err := viper.UnmarshalKey("relay_settings", &relaySettings)
	if err != nil {
		log.Printf("Error unmarshaling relay settings: %s", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to fetch settings")
	}

	// Initialize empty slices if nil
	if relaySettings.Protocol == nil {
		relaySettings.Protocol = []string{}
	}

	if relaySettings.AppBuckets == nil {
		relaySettings.AppBuckets = []string{}
	}

	if relaySettings.DynamicAppBuckets == nil {
		relaySettings.DynamicAppBuckets = []string{}
	}

	// Get subscription tiers
	var subscriptionTiers []types.SubscriptionTier
	if err := viper.UnmarshalKey("subscription_tiers", &subscriptionTiers); err != nil {
		log.Printf("Error unmarshaling subscription tiers: %s", err)
	}
	relaySettings.SubscriptionTiers = subscriptionTiers

	log.Println("Fetched relay settings:", relaySettings)

	return c.JSON(fiber.Map{
		"relay_settings": relaySettings,
	})
}
