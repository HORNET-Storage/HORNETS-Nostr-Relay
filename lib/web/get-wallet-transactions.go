package web

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func handleLatestTransactions(c *fiber.Ctx) error {
	// Initialize the Gorm database
	db, err := graviton.InitGorm()
	if err != nil {
		log.Printf("Failed to connect to the database: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Get the latest 10 transactions
	var transactions []types.WalletTransactions
	result := db.Order("date desc").Limit(10).Find(&transactions)

	if result.Error != nil {
		log.Printf("Error querying transactions: %v", result.Error)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database query error",
		})
	}

	// Get the latest Bitcoin rate
	var bitcoinRate types.BitcoinRate
	result = db.Order("timestamp desc").First(&bitcoinRate)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{
				"error": "No Bitcoin rate found",
			})
		}
		log.Printf("Error querying Bitcoin rate: %v", result.Error)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database query error",
		})
	}

	// Process each transaction to convert the value to USD
	for i, transaction := range transactions {
		satoshis, err := strconv.ParseInt(transaction.Value, 10, 64)
		if err != nil {
			log.Printf("Error converting value to int64: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Conversion error",
			})
		}
		transactions[i].Value = fmt.Sprintf("%.2f", satoshiToUSD(bitcoinRate.Rate, satoshis))
	}

	// Respond with the transactions
	return c.JSON(transactions)
}
