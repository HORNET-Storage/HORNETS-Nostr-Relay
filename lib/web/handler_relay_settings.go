package web

import (
	"log"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/gofiber/fiber/v2"
	jsoniter "github.com/json-iterator/go"
	"github.com/spf13/viper"
)

func updateRelaySettings(c *fiber.Ctx) error {
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

	// Update Viper configuration
	if err := updateViperConfig(relaySettings); err != nil {
		log.Printf("Error updating config: %s", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to update settings")
	}

	log.Println("Stored relay settings:", relaySettings)

	return c.SendStatus(fiber.StatusOK)
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

	// Ensure Protocol and Chunked are arrays
	if relaySettings.Protocol == nil {
		relaySettings.Protocol = []string{}
	}

	log.Println("Fetched relay settings:", relaySettings)

	return c.JSON(fiber.Map{
		"relay_settings": relaySettings,
	})
}
