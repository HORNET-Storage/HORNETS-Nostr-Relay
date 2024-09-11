package web

import (
	"fmt"
	"log"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

func updateWalletBalance(c *fiber.Ctx) error {
	var data map[string]interface{}

	// Parse the JSON body into the map
	if err := c.BodyParser(&data); err != nil {
		log.Printf("Failed to parse JSON: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	// Print the received data
	log.Println("Received data:", data)

	// Get the expected wallet name from the configuration
	expectedWalletName := viper.GetString("wallet_name")

	if expectedWalletName == "" {
		log.Println("No expected wallet name set in configuration.")
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Check if the wallet name in the request matches the expected wallet name
	walletName, ok := data["wallet_name"].(string)
	if !ok || walletName != expectedWalletName {
		log.Printf("Received balance update from unknown wallet: %v", walletName)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid or missing wallet name",
		})
	}

	// Extract and convert balance to string
	balanceRaw, ok := data["balance"]
	if !ok {
		log.Printf("Balance not found in the data: %v", data)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Balance not found",
		})
	}

	balance := fmt.Sprintf("%v", balanceRaw)

	// Initialize the Gorm database
	db, err := graviton.InitGorm()
	if err != nil {
		log.Printf("Failed to connect to the database: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Query the latest wallet balance
	var latestBalance types.WalletBalance
	result := db.Order("timestamp desc").First(&latestBalance)

	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		log.Printf("Error querying latest balance: %v", result.Error)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database query error",
		})
	}

	if result.Error == nil && latestBalance.Balance == balance {
		// If the latest balance is the same, do not add a new entry
		return c.JSON(fiber.Map{
			"message": "Balance is the same as the latest entry, no update needed",
		})
	}

	// Add a new entry
	newBalance := types.WalletBalance{
		Balance:   balance,
		Timestamp: time.Now(),
	}
	if err := db.Create(&newBalance).Error; err != nil {
		log.Printf("Error saving new balance: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database save error",
		})
	}

	// Respond with the received data
	return c.JSON(data)
}
