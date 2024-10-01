package web

import (
	"log"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	"github.com/gofiber/fiber/v2"
)

func saveUnconfirmedTransaction(c *fiber.Ctx) error {
	var pendingTransaction types.PendingTransaction
	log.Println("Saving unconfirmed transactions.")

	// Parse the JSON body into the struct with field mappings
	if err := c.BodyParser(&pendingTransaction); err != nil {
		log.Printf("Failed to parse JSON: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	// Ensure Timestamp is populated
	pendingTransaction.Timestamp = time.Now()

	// Initialize the Gorm database
	db, err := graviton.InitGorm()
	if err != nil {
		log.Printf("Failed to connect to the database: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Save the pending transaction to the database
	if err := db.Create(&pendingTransaction).Error; err != nil {
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
