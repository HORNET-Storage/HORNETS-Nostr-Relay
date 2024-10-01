package web

import (
	"fmt"
	"log"
	"strconv"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func getLatestWalletTransactions(c *fiber.Ctx) error {
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

	var transactions []types.WalletTransactions
	result := db.Order("date desc").Find(&transactions)

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
			log.Printf("No Bitcoin rate found, using default value")
			bitcoinRate.Rate = 0.0 // Set default rate
		} else {
			log.Printf("Error querying Bitcoin rate: %v", result.Error)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Database query error",
			})
		}
	}

	// Process each transaction to convert the value to USD
	for i, transaction := range transactions {
		value, err := strconv.ParseFloat(transaction.Value, 64)
		if err != nil {
			log.Printf("Error converting value to float64: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Conversion error",
			})
		}
		transactions[i].Value = fmt.Sprintf("%.8f", value)
	}

	// Respond with the transactions
	return c.JSON(transactions)
}