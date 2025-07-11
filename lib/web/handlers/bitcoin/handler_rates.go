package bitcoin

import (
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
)

func GetBitcoinRatesLast30Days(c *fiber.Ctx, store stores.Store) error {
	bitcoinRates, err := store.GetStatsStore().GetBitcoinRates(-30)
	if err != nil {
		logging.Infof("Error querying Bitcoin rates: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database query error",
		})
	}

	return c.JSON(bitcoinRates)
}

func UpdateBitcoinRate(c *fiber.Ctx, store stores.Store) error {
	var data map[string]interface{}

	if err := c.BodyParser(&data); err != nil {
		logging.Infof("Failed to parse JSON: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	rateRaw, ok := data["rate"]
	if !ok {
		logging.Infof("Rate not found in the data: %v", data)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Rate not found",
		})
	}

	rate, ok := rateRaw.(float64)
	if !ok {
		logging.Infof("Invalid rate format: %v", rateRaw)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid rate format",
		})
	}
	err := store.GetStatsStore().UpdateBitcoinRate(rate)
	if err != nil {
		logging.Infof("Error updating Bitcoin rate: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database save error",
		})
	}

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Bitcoin rate updated successfully",
	})
}
