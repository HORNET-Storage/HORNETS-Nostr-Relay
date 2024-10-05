package web

import (
	"log"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
)

// Refactored getKindData handler
func getKindData(c *fiber.Ctx, store stores.Store) error {
	log.Println("Kind data request received")

	// Fetch the kind data using the statistics store
	kindData, err := store.GetStatsStore().FetchKindData()
	if err != nil {
		log.Println("Error fetching kind data:", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Return the aggregated kind data
	return c.JSON(kindData)
}
