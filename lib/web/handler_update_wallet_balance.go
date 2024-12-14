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

	// Get the wallet name from request
	walletName, ok := data["wallet_name"].(string)
	if !ok {
		log.Printf("Wallet name missing in request data: %v", data)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid or missing wallet name",
		})
	}

	// Get the configured wallet name, if it's empty use the one from the request
	expectedWalletName := viper.GetString("wallet_name")
	if expectedWalletName == "" {
		log.Printf("No wallet name configured, using wallet name from request: %s", walletName)
		// Update the config with the wallet name from the request
		viper.Set("wallet_name", walletName)
		if err := viper.WriteConfig(); err != nil {
			log.Printf("Warning: Failed to write wallet name to config: %v", err)
			// Continue processing even if writing to config fails
		}
	} else if walletName != expectedWalletName {
		log.Printf("Received balance update from unknown wallet: %v, expected: %v", walletName, expectedWalletName)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid wallet name",
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
