package web

import (
	"log"
	"time"

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

	log.Println("Received data:", data)

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

	log.Println("Received relay settings JSON:", string(relaySettingsJSON))

	if err := json.Unmarshal(relaySettingsJSON, &relaySettings); err != nil {
		log.Println("Error unmarshaling relay settings:", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	var currentRelaySettings types.RelaySettings
	// Fetch settings from Viper
	err = viper.UnmarshalKey("relay_settings", &currentRelaySettings)
	if err != nil {
		log.Printf("Error unmarshaling relay settings: %s", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to fetch settings")
	}

	// Check if tiers or free tier settings have changed
	needsKind411Update := tiersChanged(currentRelaySettings.SubscriptionTiers, relaySettings.SubscriptionTiers) ||
		currentRelaySettings.FreeTierEnabled != relaySettings.FreeTierEnabled ||
		currentRelaySettings.FreeTierLimit != relaySettings.FreeTierLimit

	// Validate free tier settings
	if relaySettings.FreeTierEnabled && relaySettings.FreeTierLimit == "" {
		relaySettings.FreeTierLimit = "100 MB per month" // Set default if not provided
	}

	// Add timestamp to track when settings were last updated
	relaySettings.LastUpdated = time.Now().Unix()
	log.Printf("Setting LastUpdated timestamp to %d", relaySettings.LastUpdated)

	// Store new settings first
	viper.Set("relay_settings", relaySettings)
	if err := viper.WriteConfig(); err != nil {
		log.Printf("Error writing config: %s", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to update settings")
	}

	// Compare existing tiers with the new tiers
	if needsKind411Update {
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

	log.Printf("Stored relay settings (including free tier settings): %+v", relaySettings)

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

func getRelaySettings(c *fiber.Ctx) error {
	log.Println("Get relay settings request received")

	var relaySettings types.RelaySettings

	// Fetch settings from Viper
	err := viper.UnmarshalKey("relay_settings", &relaySettings)
	if err != nil {
		log.Printf("Error unmarshaling relay settings: %s", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to fetch settings")
	}

	log.Printf("Fetched relay settings 1: %+v", relaySettings)

	// Initialize arrays if nil
	if relaySettings.Protocol == nil {
		relaySettings.Protocol = []string{}
	}
	if relaySettings.Chunked == nil {
		relaySettings.Chunked = []string{}
	}
	// Initialize subscription tiers if nil
	if relaySettings.SubscriptionTiers == nil {
		log.Println("SubscriptionTiers is nil.")
		relaySettings.SubscriptionTiers = []types.SubscriptionTier{
			{DataLimit: "1 GB per month", Price: "8000"},
			{DataLimit: "5 GB per month", Price: "10000"},
			{DataLimit: "10 GB per month", Price: "15000"},
		}
	}

	if relaySettings.FreeTierLimit == "" {
		relaySettings.FreeTierLimit = "100 MB per month"
	}

	log.Printf("Fetched relay settings: %+v", relaySettings) // Using %+v for more detailed output

	return c.JSON(fiber.Map{
		"relay_settings": relaySettings,
	})
}
