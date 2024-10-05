package web

import (
	"log"
	"strconv"

	gorm "github.com/HORNET-Storage/hornet-storage/lib/stores/stats_stores"
	"github.com/gofiber/fiber/v2"
)

// Refactored getKindTrendData handler
func getKindTrendData(c *fiber.Ctx, store *gorm.GormStatisticsStore) error {
	log.Println("Kind trend data request received")
	kindNumberStr := c.Params("kindNumber")
	kindNumber, err := strconv.Atoi(kindNumberStr)
	if err != nil {
		log.Printf("Error converting kind number to integer: %v", err)
		return c.Status(fiber.StatusBadRequest).SendString("Invalid kind number")
	}

	// Fetch the kind trend data using the statistics store
	trendData, err := store.FetchKindTrendData(kindNumber)
	if err != nil {
		log.Println("Error fetching kind trend data:", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// If no data was found, return a 404
	if trendData == nil {
		return c.Status(fiber.StatusNotFound).SendString("No data found")
	}

	// Return the kind trend data
	return c.JSON(trendData)
}
