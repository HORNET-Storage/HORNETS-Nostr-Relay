package web

import (
	"log"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Refactored replaceTransaction function
func replaceTransaction(c *fiber.Ctx, store stores.Store) error {
	// Parse the JSON body into the ReplaceTransactionRequest struct
	var replaceRequest types.ReplaceTransactionRequest
	if err := c.BodyParser(&replaceRequest); err != nil {
		log.Printf("Failed to parse replacement request: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	// Use the statistics store to replace the transaction
	err := store.GetStatsStore().ReplaceTransaction(replaceRequest)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Original transaction not found",
			})
		}
		log.Printf("Error replacing transaction: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Error replacing transaction",
		})
	}

	// Respond with success
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Replacement transaction saved successfully",
		"txid":    replaceRequest.NewTxID,
	})
}
