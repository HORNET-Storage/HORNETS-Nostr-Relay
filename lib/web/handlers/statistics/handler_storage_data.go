package statistics

import (
	"log"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
)

// Refactored getMonthlyStorageStats function
func GetMonthlyStorageStats(c *fiber.Ctx, store stores.Store) error {
	log.Println("Activity data request received")

	// Fetch the monthly storage stats using the statistics store
	data, err := store.GetStatsStore().FetchMonthlyStorageStats()
	if err != nil {
		log.Println("Error fetching activity data:", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Return the storage stats as JSON
	return c.JSON(data)
}

// Refactored getNotesMediaStorageData function
func GetNotesMediaStorageData(c *fiber.Ctx, store stores.Store) error {
	log.Println("Bar chart data request received")

	// Fetch the notes and media storage data using the statistics store
	data, err := store.GetStatsStore().FetchNotesMediaStorageData()
	if err != nil {
		log.Println("Error fetching bar chart data:", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Return the data as JSON
	return c.JSON(data)
}
