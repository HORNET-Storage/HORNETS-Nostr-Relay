package web

import (
	"log"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
)

func getBitcoinRatesLast30Days(c *fiber.Ctx, store stores.Store) error {
	// Query Bitcoin rates using the statistics store
	bitcoinRates, err := store.GetStatsStore().GetBitcoinRatesLast30Days()
	if err != nil {
		log.Printf("Error querying Bitcoin rates: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database query error",
		})
	}

	// Respond with the Bitcoin rates
	return c.JSON(bitcoinRates)
}
