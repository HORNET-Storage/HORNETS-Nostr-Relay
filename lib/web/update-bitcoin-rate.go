package web

import (
	"log"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func handleBitcoinRate(c *fiber.Ctx) error {
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
	db, err := graviton.InitGorm()
	if err != nil {
		log.Printf("Failed to connect to the database: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Query the latest Bitcoin rate
	var bitcoinRate types.BitcoinRate
	result := db.First(&bitcoinRate)

	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		log.Printf("Error querying bitcoin rate: %v", result.Error)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database query error",
		})
	}

	if result.Error == nil && bitcoinRate.Rate == rate {
		// If the rate is the same, no update needed
		return c.JSON(fiber.Map{
			"message": "Rate is the same as the latest entry, no update needed",
		})
	}

	// Update or create the Bitcoin rate
	if result.Error == gorm.ErrRecordNotFound {
		// Create a new rate entry
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
	} else {
		// Update the existing rate
		bitcoinRate.Rate = rate
		bitcoinRate.Timestamp = time.Now()
		if err := db.Save(&bitcoinRate).Error; err != nil {
			log.Printf("Error updating rate: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Database update error",
			})
		}
	}

	// Respond with the received data
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Bitcoin rate updated successfully",
	})
}
