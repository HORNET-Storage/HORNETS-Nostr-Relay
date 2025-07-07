package wallet

import (
	"fmt"
	"strconv"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
)

func GetWalletBalanceUSD(c *fiber.Ctx, store stores.Store) error {
	// Get the latest wallet balance
	latestBalance, err := store.GetStatsStore().GetLatestWalletBalance()
	if err != nil {
		logging.Infof("Error querying latest balance: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database query error",
		})
	}

	// Get the latest Bitcoin rate
	bitcoinRate, err := store.GetStatsStore().GetLatestBitcoinRate()
	if err != nil {
		logging.Infof("Error querying Bitcoin rate: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database query error",
		})
	}

	// Convert string rate to float64
	rateFloat, err := strconv.ParseFloat(bitcoinRate.Rate, 64)
	if err != nil {
		logging.Infof("Error converting Bitcoin rate to float64: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Rate conversion error",
		})
	}

	// Convert the balance to USD
	satoshis, err := strconv.ParseInt(latestBalance.Balance, 10, 64)
	if err != nil {
		logging.Infof("Error converting balance to int64: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Conversion error",
		})
	}

	usdBalance := SatoshiToUSD(rateFloat, satoshis)

	// Respond with the USD balance
	return c.JSON(fiber.Map{
		"balance_usd":    usdBalance,
		"latest_balance": satoshis,
	})
}

// Refactored updateWalletBalance function
func UpdateWalletBalance(c *fiber.Ctx, store stores.Store) error {
	var data map[string]interface{}

	// Parse the JSON body into the map
	if err := c.BodyParser(&data); err != nil {
		logging.Infof("Failed to parse JSON: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	// Print the received data
	logging.Info("Received data:", data)

	// Get the wallet name from request
	walletName, ok := data["wallet_name"].(string)
	if !ok {
		logging.Infof("Wallet name missing in request data: %v", data)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid or missing wallet name",
		})
	}

	// Get the configured wallet name, if it's empty use the one from the request
	expectedWalletName := viper.GetString("wallet.name")
	if expectedWalletName == "" {
		logging.Infof("No wallet name configured, using wallet name from request: %s", walletName)
		// Update the config with the wallet name from the request
		viper.Set("wallet.name", walletName)
		if err := viper.WriteConfig(); err != nil {
			logging.Infof("Warning: Failed to write wallet name to config: %v", err)
			// Continue processing even if writing to config fails
		}
	} else if walletName != expectedWalletName {
		logging.Infof("Received balance update from unknown wallet: %v, expected: %v", walletName, expectedWalletName)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid wallet name",
		})
	}

	// Extract and convert balance to string
	balanceRaw, ok := data["balance"]
	if !ok {
		logging.Infof("Balance not found in the data: %v", data)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Balance not found",
		})
	}
	balance := fmt.Sprintf("%v", balanceRaw)

	// Use the statistics store to update the wallet balance
	err := store.GetStatsStore().UpdateWalletBalance(walletName, balance)
	if err != nil {
		logging.Infof("Error updating wallet balance: %v", err)
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
