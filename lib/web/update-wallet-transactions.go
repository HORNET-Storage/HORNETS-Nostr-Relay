package web

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"gorm.io/gorm"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
)

func handleTransactions(c *fiber.Ctx) error {
	var transactions []map[string]interface{}
	log.Println("Transactions request received")

	// Parse the JSON body into the slice of maps
	if err := c.BodyParser(&transactions); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	// Get the expected wallet name from the configuration
	expectedWalletName := viper.GetString("wallet_name")

	// If the expected wallet name is not set, set it using the first transaction's wallet name
	if expectedWalletName == "" && len(transactions) > 0 {
		firstTransaction := transactions[0]
		walletName, ok := firstTransaction["wallet_name"].(string)
		if !ok {
			log.Println("Wallet name missing or invalid in the first transaction")
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Wallet name missing or invalid",
			})
		}

		// Set the expected wallet name in Viper
		viper.Set("wallet_name", walletName)
		expectedWalletName = walletName
		log.Printf("Setting wallet name in configuration: %s", expectedWalletName)

		// Optionally save the updated configuration to a file
		if err := viper.WriteConfig(); err != nil {
			log.Printf("Error saving updated configuration: %v", err)
		}
	}

	for _, transaction := range transactions {
		walletName, ok := transaction["wallet_name"].(string)
		if !ok || walletName != expectedWalletName {
			log.Printf("Transaction from unknown wallet: %v", walletName)
			continue // Skip processing if wallet name doesn't match
		}

		address, ok := transaction["address"].(string)
		if !ok {
			log.Printf("Invalid address format: %v", transaction["address"])
			continue
		}

		dateStr, ok := transaction["date"].(string)
		if !ok {
			log.Printf("Invalid date format: %v", transaction["date"])
			continue
		}

		// Correct format for ISO 8601 datetime string with timezone
		date, err := time.Parse(time.RFC3339, dateStr)
		if err != nil {
			log.Printf("Error parsing date: %v", err)
			continue
		}

		output, ok := transaction["output"].(string)
		if !ok {
			log.Printf("Invalid output format: %v", transaction["output"])
			continue
		}

		valueStr, ok := transaction["value"].(string)
		if !ok {
			log.Printf("Invalid value format: %v", transaction["value"])
			continue
		}

		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			log.Printf("Error parsing value to float64: %v", err)
			continue
		}

		// Initialize the Gorm database
		db, err := graviton.InitGorm()
		if err != nil {
			log.Printf("Failed to connect to the database: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
		}

		var existingTransaction types.WalletTransactions
		result := db.Where("address = ? AND date = ? AND output = ? AND value = ?", address, date, output, valueStr).First(&existingTransaction)

		if result.Error == nil {
			// Transaction already exists, skip it
			log.Printf("Duplicate transaction found, skipping: %v", transaction)
			continue
		}

		if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
			log.Printf("Error querying transaction: %v", result.Error)
			continue
		}

		// Create a new transaction
		newTransaction := types.WalletTransactions{
			Address: address,
			Date:    date,
			Output:  output,
			Value:   fmt.Sprintf("%.8f", value),
		}
		if err := db.Create(&newTransaction).Error; err != nil {
			log.Printf("Error saving new transaction: %v", err)
			continue
		}
	}

	// Respond with a success message
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Transactions received and processed successfully",
	})
}
