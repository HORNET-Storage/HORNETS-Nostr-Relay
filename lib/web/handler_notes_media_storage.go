package web

import (
	"log"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

func getNotesMediaStorageData(c *fiber.Ctx) error {
	log.Println("Bar chart data request received")

	// Retrieve the database path from the config file using Viper
	dbPath := viper.GetString("relay_stats_db")
	if dbPath == "" {
		log.Fatal("Database path not found in config")
	}

	// Retrieve the gorm db
	db := c.Locals("db").(*gorm.DB)
	var err error

	// Query to get the total GBs per month for notes and media
	var data []types.BarChartData
	err = db.Raw(`
		SELECT 
			strftime('%Y-%m', timestamp) as month,
			ROUND(SUM(CASE WHEN kind_number IS NOT NULL THEN size ELSE 0 END) / 1024.0, 3) as notes_gb,  -- Convert to GB and round to 2 decimal places
			ROUND(SUM(CASE WHEN kind_number IS NULL THEN size ELSE 0 END) / 1024.0, 3) as media_gb  -- Convert to GB and round to 2 decimal places
		FROM (
			SELECT timestamp, size, kind_number FROM kinds
			UNION ALL
			SELECT timestamp, size, NULL as kind_number FROM photos
			UNION ALL
			SELECT timestamp, size, NULL as kind_number FROM videos
			UNION ALL
			SELECT timestamp, size, NULL as kind_number FROM git_nestrs
			UNION ALL
			SELECT timestamp, size, NULL as kind_number FROM audios
		)
		GROUP BY month
	`).Scan(&data).Error

	if err != nil {
		log.Println("Error fetching bar chart data:", err)
		return c.Status(500).SendString("Internal Server Error")
	}

	return c.JSON(data)
}
