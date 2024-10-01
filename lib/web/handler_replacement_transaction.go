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

func replaceTransaction(c *fiber.Ctx) error {
	// Parse the JSON body into the ReplaceTransactionRequest struct
	var replaceRequest types.ReplaceTransactionRequest
	if err := c.BodyParser(&replaceRequest); err != nil {
		log.Printf("Failed to parse replacement request: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
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

	// Delete the original pending transaction
	var originalPendingTransaction types.PendingTransaction
	if err := db.Where("tx_id = ?", replaceRequest.OriginalTxID).First(&originalPendingTransaction).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Printf("No pending transaction found with TxID %s", replaceRequest.OriginalTxID)
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Original transaction not found",
			})
		}
		log.Printf("Error querying original transaction with TxID %s: %v", replaceRequest.OriginalTxID, err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	if err := db.Delete(&originalPendingTransaction).Error; err != nil {
		log.Printf("Error deleting pending transaction with TxID %s: %v", replaceRequest.OriginalTxID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Error deleting original transaction",
		})
	}
	log.Printf("Deleted original pending transaction with TxID %s", replaceRequest.OriginalTxID)

	// Save the new pending transaction
	newPendingTransaction := types.PendingTransaction{
		TxID:             replaceRequest.NewTxID, // Save the new transaction ID
		FeeRate:          replaceRequest.NewFeeRate,
		Amount:           replaceRequest.Amount,
		RecipientAddress: replaceRequest.RecipientAddress,
		Timestamp:        time.Now(),
	}

	if err := db.Create(&newPendingTransaction).Error; err != nil {
		log.Printf("Error saving new pending transaction: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Error saving new transaction",
		})
	}

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Replacement transaction saved successfully",
		"txid":    newPendingTransaction.TxID,
	})
}
