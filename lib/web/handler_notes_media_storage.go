package web

import (
	"log"

	gorm "github.com/HORNET-Storage/hornet-storage/lib/stores/stats_stores"
	"github.com/gofiber/fiber/v2"
)

// Refactored getNotesMediaStorageData function
func getNotesMediaStorageData(c *fiber.Ctx, store *gorm.GormStatisticsStore) error {
	log.Println("Bar chart data request received")

	// Fetch the notes and media storage data using the statistics store
	data, err := store.FetchNotesMediaStorageData()
	if err != nil {
		log.Println("Error fetching bar chart data:", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Return the data as JSON
	return c.JSON(data)
}
