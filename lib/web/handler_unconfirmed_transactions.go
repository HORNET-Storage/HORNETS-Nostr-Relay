package web

import (
	"encoding/json"
	"log"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func getPendingTransactions(c *fiber.Ctx) error {
	log.Println("Request for unconfirmed transactions.")

	// Retrieve the gorm db
	db := c.Locals("db").(*gorm.DB)
	var err error

	// Query all pending transactions
	var pendingTransactions []types.PendingTransaction
	result := db.Order("timestamp desc").Find(&pendingTransactions)

	if result.Error != nil {
		log.Printf("Error querying pending transactions: %v", result.Error)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database query error",
		})
	}

	pendingTransactionsJson, err := json.Marshal(pendingTransactions)
	if err != nil {
		log.Printf("Error marshalling pending transactions: %v", err)
	}

	log.Println("transactions: ", string(pendingTransactionsJson))

	// Respond with the pending transactions
	return c.JSON(pendingTransactions)
}
