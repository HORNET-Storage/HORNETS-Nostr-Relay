package web

import (
	"log"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
)

// Refactored getNotesMediaStorageData function
func getNotesMediaStorageData(c *fiber.Ctx, store stores.Store) error {
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
