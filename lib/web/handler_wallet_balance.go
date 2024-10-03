package web

import (
	"log"
	"strconv"

	gorm "github.com/HORNET-Storage/hornet-storage/lib/stores/stats_stores"
	"github.com/gofiber/fiber/v2"
)

func getWalletBalanceUSD(c *fiber.Ctx, store *gorm.GormStatisticsStore) error {
	// Get the latest wallet balance
	latestBalance, err := store.GetLatestWalletBalance()
	if err != nil {
		log.Printf("Error querying latest balance: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database query error",
		})
	}

	// Get the latest Bitcoin rate
	bitcoinRate, err := store.GetLatestBitcoinRate()
	if err != nil {
		log.Printf("Error querying Bitcoin rate: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database query error",
		})
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
