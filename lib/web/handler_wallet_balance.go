package web

import (
	"log"
	"strconv"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func getWalletBalanceUSD(c *fiber.Ctx) error {
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

	// Get the latest wallet balance
	var latestBalance types.WalletBalance
	result := db.Order("timestamp desc").First(&latestBalance)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			log.Printf("No wallet balance found, using default value")
			latestBalance.Balance = "0" // Set default balance
		} else {
			log.Printf("Error querying latest balance: %v", result.Error)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Database query error",
			})
		}
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

	// Convert the balance to USD
	satoshis, err := strconv.ParseInt(latestBalance.Balance, 10, 64)
	if err != nil {
		log.Printf("Error converting balance to int64: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Conversion error",
		})
	}

	usdBalance := satoshiToUSD(bitcoinRate.Rate, satoshis)

	// Respond with the USD balance
	return c.JSON(fiber.Map{
		"balance_usd":    usdBalance,
		"latest_balance": satoshis,
	})
}
