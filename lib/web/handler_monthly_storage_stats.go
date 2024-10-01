package web

import (
	"log"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func getMonthlyStorageStats(c *fiber.Ctx) error {
	log.Println("Activity data request received")

	// Retrieve the database path from the config file using Viper
	dbPath := viper.GetString("relay_stats_db")
	if dbPath == "" {
		log.Fatal("Database path not found in config")
	}

	// Initialize the Gorm database
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Printf("Failed to connect to the database: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Query to get the total GBs per month
	var data []types.ActivityData
	err = db.Raw(`
		SELECT 
			strftime('%Y-%m', timestamp) as month,
			ROUND(SUM(size) / 1024.0, 3) as total_gb
		FROM (
			SELECT timestamp, size FROM kinds
			UNION ALL
			SELECT timestamp, size FROM photos
			UNION ALL
			SELECT timestamp, size FROM videos
			UNION ALL
			SELECT timestamp, size FROM git_nestrs
			UNION ALL
			SELECT timestamp, size FROM audios
		)
		GROUP BY month
	`).Scan(&data).Error

	if err != nil {
		log.Println("Error fetching activity data:", err)
		return c.Status(500).SendString("Internal Server Error")
	}

	return c.JSON(data)
}
