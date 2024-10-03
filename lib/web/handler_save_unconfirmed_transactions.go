package web

import (
	"log"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	gorm "github.com/HORNET-Storage/hornet-storage/lib/stores/stats_stores"
	"github.com/gofiber/fiber/v2"
)

// Refactored saveUnconfirmedTransaction function
func saveUnconfirmedTransaction(c *fiber.Ctx, store *gorm.GormStatisticsStore) error {
	var pendingTransaction types.PendingTransaction
	log.Println("Saving unconfirmed transaction.")

	// Parse the JSON body into the struct with field mappings
	if err := c.BodyParser(&pendingTransaction); err != nil {
		log.Printf("Failed to parse JSON: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	// Use the statistics store to save the pending transaction
	err := store.SaveUnconfirmedTransaction(&pendingTransaction)
	if err != nil {
		log.Printf("Error saving pending transaction: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database save error",
		})
	}

	// Respond with success message
	return c.JSON(fiber.Map{
		"message": "Pending transaction saved successfully",
	})
}
