package web

import (
	"log"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	"github.com/gofiber/fiber/v2"
)

func getBitcoinRatesLast30Days(c *fiber.Ctx) error {
	// Initialize the Gorm database
	db, err := graviton.InitGorm()
	if err != nil {
		log.Printf("Failed to connect to the database: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Calculate the date 30 days ago
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)

	// Query the Bitcoin rates for the last 30 days
	var bitcoinRates []types.BitcoinRate
	result := db.Where("timestamp >= ?", thirtyDaysAgo).Order("timestamp asc").Find(&bitcoinRates)

	if result.Error != nil {
		log.Printf("Error querying Bitcoin rates: %v", result.Error)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database query error",
		})
	}

	// Respond with the Bitcoin rates
	return c.JSON(bitcoinRates)
}
