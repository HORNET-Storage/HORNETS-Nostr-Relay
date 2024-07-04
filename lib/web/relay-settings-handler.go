package web

import (
	"log"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/gofiber/fiber/v2"
	jsoniter "github.com/json-iterator/go"
	"github.com/spf13/viper"
)

func handleRelaySettings(c *fiber.Ctx) error {
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

	// Check boolean flags and set corresponding arrays to empty if false
	if !relaySettings.IsKindsActive {
		relaySettings.Kinds = []string{}
		relaySettings.DynamicKinds = []string{}
	}
	if !relaySettings.IsPhotosActive {
		relaySettings.Photos = []string{}
	}
	if !relaySettings.IsVideosActive {
		relaySettings.Videos = []string{}
	}
	if !relaySettings.IsGitNestrActive {
		relaySettings.GitNestr = []string{}
	}
	if !relaySettings.IsAudioActive {
		relaySettings.Audio = []string{}
	}
	if relaySettings.Mode == "smart" {
		relaySettings.DynamicKinds = []string{}
	}

	// Store in Viper
	viper.Set("relay_settings", relaySettings)

	// Save the changes to the configuration file
	if err := viper.WriteConfig(); err != nil {
		log.Printf("Error writing config: %s", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to update settings")
	}

	log.Println("Stored relay settings:", relaySettings)

	return c.SendStatus(fiber.StatusOK)
}

func handleGetRelaySettings(c *fiber.Ctx) error {
	log.Println("Get relay settings request received")

	var relaySettings types.RelaySettings

	// Fetch settings from Viper
	err := viper.UnmarshalKey("relay_settings", &relaySettings)
	if err != nil {
		log.Printf("Error unmarshaling relay settings: %s", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to fetch settings")
	}

	// Ensure Protocol and Chunked are arrays
	if relaySettings.Protocol == nil {
		relaySettings.Protocol = []string{}
	}
	if relaySettings.Chunked == nil {
		relaySettings.Chunked = []string{}
	}

	log.Println("Fetched relay settings:", relaySettings)

	return c.JSON(fiber.Map{
		"relay_settings": relaySettings,
	})
}
