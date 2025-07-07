package statistics

import (
	"strconv"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
)

// Refactored getKindData handler
func GetKindData(c *fiber.Ctx, store stores.Store) error {
	logging.Info("Kind data request received")

	// Fetch the kind data using the statistics store
	kindData, err := store.GetStatsStore().FetchKindData()
	if err != nil {
		logging.Infof("Error fetching kind data:%s", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Return the aggregated kind data
	return c.JSON(kindData)
}

// Refactored getKindTrendData handler
func GetKindTrendData(c *fiber.Ctx, store stores.Store) error {
	logging.Info("Kind trend data request received")
	kindNumberStr := c.Params("kindNumber")
	kindNumber, err := strconv.Atoi(kindNumberStr)
	if err != nil {
		logging.Infof("Error converting kind number to integer: %v", err)
		return c.Status(fiber.StatusBadRequest).SendString("Invalid kind number")
	}

	// Fetch the kind trend data using the statistics store
	trendData, err := store.GetStatsStore().FetchKindTrendData(kindNumber)
	if err != nil {
		logging.Infof("Error fetching kind trend data:%s", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// If no data was found, return a 404
	if trendData == nil {
		return c.Status(fiber.StatusNotFound).SendString("No data found")
	}

	// Return the kind trend data
	return c.JSON(trendData)
}
