package web

import (
	"log"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func handleActivityData(c *fiber.Ctx) error {
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
			SUM(size) / 1024.0 as total_gb
		FROM (
			SELECT timestamp, size FROM kinds
			UNION ALL
			SELECT timestamp, size FROM photos
			UNION ALL
			SELECT timestamp, size FROM videos
			UNION ALL
			SELECT timestamp, size FROM git_nestrs
		)
		GROUP BY month
	`).Scan(&data).Error

	if err != nil {
		log.Println("Error fetching activity data:", err)
		return c.Status(500).SendString("Internal Server Error")
	}

	log.Printf("Fetched data: %+v", data)
	return c.JSON(data)
}

// package web

// import (
// 	"fmt"
// 	"log"
// 	"time"

// 	types "github.com/HORNET-Storage/hornet-storage/lib"
// 	"github.com/gofiber/fiber/v2"
// 	"github.com/spf13/viper"
// 	"gorm.io/driver/sqlite"
// 	"gorm.io/gorm"
// )

// func handleActivityData(c *fiber.Ctx) error {
// 	log.Println("Activity data request received")

// 	// Retrieve the database path from the config file using Viper
// 	dbPath := viper.GetString("relay_stats_db")
// 	if dbPath == "" {
// 		log.Fatal("Database path not found in config")
// 	}

// 	// Initialize the Gorm database
// 	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
// 	if err != nil {
// 		log.Printf("Failed to connect to the database: %v", err)
// 		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
// 	}

// 	// Query to get the total GBs per month
// 	var data []types.ActivityData
// 	err = db.Raw(`
// 		SELECT
// 			strftime('%Y-%m', timestamp) as month,
// 			SUM(size) / 1024.0 as total_gb
// 		FROM (
// 			SELECT timestamp, size FROM kinds
// 			UNION ALL
// 			SELECT timestamp, size FROM photos
// 			UNION ALL
// 			SELECT timestamp, size FROM videos
// 			UNION ALL
// 			SELECT timestamp, size FROM git_nestrs
// 		)
// 		GROUP BY month
// 	`).Scan(&data).Error

// 	if err != nil {
// 		log.Println("Error fetching activity data:", err)
// 		return c.Status(500).SendString("Internal Server Error")
// 	}

// 	// Generate data for all months from the beginning of the year to the current month
// 	fullYearData := generateFullYearActivityData(data)

// 	log.Printf("Fetched data: %+v", fullYearData)
// 	return c.JSON(fullYearData)
// }

// func generateFullYearActivityData(data []types.ActivityData) []types.ActivityData {
// 	currentYear := time.Now().Year()
// 	currentMonth := time.Now().Month()
// 	months := make(map[string]types.ActivityData)

// 	// Initialize months with zero counts up to the current month
// 	for month := 1; month <= int(currentMonth); month++ {
// 		monthStr := fmt.Sprintf("%d-%02d", currentYear, month)
// 		months[monthStr] = types.ActivityData{
// 			Month:   monthStr,
// 			TotalGB: 0,
// 		}
// 	}

// 	// Fill in the data for the months that have data
// 	for _, entry := range data {
// 		months[entry.Month] = entry
// 	}

// 	// Convert map to slice
// 	fullYearData := make([]types.ActivityData, 0, int(currentMonth))
// 	for month := 1; month <= int(currentMonth); month++ {
// 		monthStr := fmt.Sprintf("%d-%02d", currentYear, month)
// 		entry := months[monthStr]
// 		entry.Month = time.Month(month).String() // Convert month number to month name
// 		fullYearData = append(fullYearData, entry)
// 	}

// 	return fullYearData
// }
