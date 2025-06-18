package web

import (
	"log"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
	jsoniter "github.com/json-iterator/go"
	"github.com/spf13/viper"
)

func updateRelaySettings(c *fiber.Ctx, _ stores.Store) error {
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

	// Add timestamp to track when settings were last updated
	relaySettings.LastUpdated = time.Now().Unix()
	log.Printf("Setting LastUpdated timestamp to %d", relaySettings.LastUpdated)

	// Store new settings
	viper.Set("relay_settings", relaySettings)
	if err := viper.WriteConfig(); err != nil {
		log.Printf("Error writing config: %s", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to update settings")
	}

	log.Printf("Stored relay settings: %+v", relaySettings)

	return c.SendStatus(fiber.StatusOK)
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

	// Get subscription tiers from allowed_users instead of relay_settings
	// Note: Subscription tiers are now managed through allowed_users settings

	// Initialize moderation mode if not set
	if relaySettings.ModerationMode == "" {
		relaySettings.ModerationMode = "strict" // Default to strict mode
	}

	log.Printf("Fetched relay settings: %+v", relaySettings) // Using %+v for more detailed output

	return c.JSON(fiber.Map{
		"relay_settings": relaySettings,
	})
}
