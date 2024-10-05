package web

import (
	"log"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
)

// Refactored updateBitcoinRate function
func updateBitcoinRate(c *fiber.Ctx, store stores.Store) error {
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

	// Extract the rate from the received data
	rateRaw, ok := data["rate"]
	if !ok {
		log.Printf("Rate not found in the data: %v", data)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Rate not found",
		})
	}

	// Validate the rate format
	rate, ok := rateRaw.(float64)
	if !ok {
		log.Printf("Invalid rate format: %v", rateRaw)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid rate format",
		})
	}

	// Use the statistics store to update the Bitcoin rate
	err := store.GetStatsStore().UpdateBitcoinRate(rate)
	if err != nil {
		log.Printf("Error updating Bitcoin rate: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database save error",
		})
	}

	// Respond with the success message
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Bitcoin rate updated successfully",
	})
}
