package statistics

import (
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
)

// Refactored getMonthlyStorageStats function
func GetMonthlyStorageStats(c *fiber.Ctx, store stores.Store) error {
	logging.Info("Activity data request received")

	// Fetch the monthly storage stats using the statistics store
	data, err := store.GetStatsStore().FetchMonthlyStorageStats()
	if err != nil {
		logging.Infof("Error fetching activity data:%s", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Return the storage stats as JSON
	return c.JSON(data)
}

// Refactored getNotesMediaStorageData function
func GetNotesMediaStorageData(c *fiber.Ctx, store stores.Store) error {
	logging.Info("Bar chart data request received")

	// Fetch the notes and media storage data using the statistics store
	data, err := store.GetStatsStore().FetchNotesMediaStorageData()
	if err != nil {
		logging.Infof("Error fetching bar chart data:%s", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Return the data as JSON
	return c.JSON(data)
}
