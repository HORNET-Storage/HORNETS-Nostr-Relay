package web

import (
	"log"

	gorm "github.com/HORNET-Storage/hornet-storage/lib/stores/stats_stores"
	"github.com/gofiber/fiber/v2"
)

// Refactored getPendingTransactions function
func getPendingTransactions(c *fiber.Ctx, store *gorm.GormStatisticsStore) error {
	log.Println("Request for unconfirmed transactions.")

	// Use the statistics store to retrieve pending transactions
	pendingTransactions, err := store.GetPendingTransactions()
	if err != nil {
		log.Printf("Error querying pending transactions: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database query error",
		})
	}

	// Respond with the pending transactions
	return c.JSON(pendingTransactions)
}
