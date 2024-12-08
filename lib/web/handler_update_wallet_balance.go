package web

import (
	"fmt"
	"log"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
)

// Refactored updateWalletBalance function
func updateWalletBalance(c *fiber.Ctx, store stores.Store) error {
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

	// Use the statistics store to update the wallet balance
	err := store.GetStatsStore().UpdateWalletBalance(walletName, balance)
	if err != nil {
		log.Printf("Error updating wallet balance: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database save error",
		})
	}

	// Respond with the received data
	return c.JSON(fiber.Map{
		"message": "success",
		"balance": balance,
	})
}
