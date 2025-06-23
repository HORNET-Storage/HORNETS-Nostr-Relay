package settings

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
)

// GetSettings returns the entire configuration
func GetSettings(c *fiber.Ctx) error {
	log.Println("Get settings request received")

	// Return the entire config as JSON
	settings := viper.AllSettings()

	return c.JSON(fiber.Map{
		"settings": settings,
	})
}

// UpdateSettings updates configuration values
func UpdateSettings(c *fiber.Ctx) error {
	log.Println("Update settings request received")

	var data map[string]interface{}
	if err := c.BodyParser(&data); err != nil {
		log.Printf("Error parsing request body: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Extract settings from the request
	settings, ok := data["settings"].(map[string]interface{})
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Settings data expected",
		})
	}

	// Update each setting
	for key, value := range settings {
		log.Printf("Setting %s = %v", key, value)
		viper.Set(key, value)
	}

	// Save the configuration
	if err := viper.WriteConfig(); err != nil {
		log.Printf("Error writing config: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to save settings",
		})
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
