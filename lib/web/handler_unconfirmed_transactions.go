package web

import (
	"encoding/json"
	"log"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	"github.com/gofiber/fiber/v2"
)

func getPendingTransactions(c *fiber.Ctx) error {
	log.Println("Request for unconfirmed transactions.")
	// Initialize the Gorm database
	db, err := graviton.InitGorm()
	if err != nil {
		log.Printf("Failed to connect to the database: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

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
