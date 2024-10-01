package web

import (
	"log"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func updateBitcoinRate(c *fiber.Ctx) error {
	var data map[string]interface{}

	// Parse the JSON body into the map
	if err := c.BodyParser(&data); err != nil {
		log.Printf("Failed to parse JSON: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	// Print the received data
	log.Println("Received data:", data)

	rateRaw, ok := data["rate"]
	if !ok {
		log.Printf("Rate not found in the data: %v", data)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Rate not found",
		})
	}

	rate, ok := rateRaw.(float64)
	if !ok {
		log.Printf("Invalid rate format: %v", rateRaw)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid rate format",
		})
	}

	// Initialize the Gorm database
	dbPath := viper.GetString("relay_stats_db")
	if dbPath == "" {
		log.Fatal("Database path not found in config")
	}

	// Initialize the Gorm database
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Printf("Failed to connect to the database: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Query the latest Bitcoin rate
	var latestBitcoinRate types.BitcoinRate
	result := db.Order("timestamp desc").First(&latestBitcoinRate)

	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		log.Printf("Error querying bitcoin rate: %v", result.Error)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database query error",
		})
	}

	if result.Error == nil && latestBitcoinRate.Rate == rate {
		// If the rate is the same as the latest entry, no update needed
		return c.JSON(fiber.Map{
			"message": "Rate is the same as the latest entry, no update needed",
		})
	}

	// Add the new rate
	newRate := types.BitcoinRate{
		Rate:      rate,
		Timestamp: time.Now(),
	}
	if err := db.Create(&newRate).Error; err != nil {
		log.Printf("Error saving new rate: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database save error",
		})
	}

	// Respond with the received data
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Bitcoin rate updated successfully",
	})
}
