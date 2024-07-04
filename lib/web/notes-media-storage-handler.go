package web

import (
	"log"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func handleBarChartData(c *fiber.Ctx) error {
	log.Println("Bar chart data request received")

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

	log.Printf("Fetched data: %+v", data)
	return c.JSON(data)
}

// package web

// import (
// 	"log"

// 	types "github.com/HORNET-Storage/hornet-storage/lib"
// 	"github.com/gofiber/fiber/v2"
// 	"github.com/spf13/viper"
// 	"gorm.io/driver/sqlite"
// 	"gorm.io/gorm"
// )

// func handleBarChartData(c *fiber.Ctx) error {
// 	log.Println("Bar chart data request received")

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

// 	// Query to get the total GBs per month for notes and media
// 	var data []types.BarChartData
// 	err = db.Raw(`
// 		SELECT
// 			strftime('%Y-%m', timestamp) as month,
// 			SUM(CASE WHEN kind_number IS NOT NULL THEN size ELSE 0 END) / 1024.0 as notes_gb,  -- Convert to GB
// 			SUM(CASE WHEN kind_number IS NULL THEN size ELSE 0 END) / 1024.0 as media_gb  -- Convert to GB
// 		FROM (
// 			SELECT timestamp, size, kind_number FROM kinds
// 			UNION ALL
// 			SELECT timestamp, size, NULL as kind_number FROM photos
// 			UNION ALL
// 			SELECT timestamp, size, NULL as kind_number FROM videos
// 			UNION ALL
// 			SELECT timestamp, size, NULL as kind_number FROM git_nestrs
// 		)
// 		GROUP BY month
// 	`).Scan(&data).Error

// 	if err != nil {
// 		log.Println("Error fetching bar chart data:", err)
// 		return c.Status(500).SendString("Internal Server Error")
// 	}

// 	log.Printf("Fetched data: %+v", data)
// 	return c.JSON(data)
// }

// package web

// import (
// 	"log"
// 	"time"

// 	types "github.com/HORNET-Storage/hornet-storage/lib"
// 	"github.com/gofiber/fiber/v2"
// 	"github.com/spf13/viper"
// 	"gorm.io/driver/sqlite"
// 	"gorm.io/gorm"
// )

// func handleBarChartData(c *fiber.Ctx) error {
// 	log.Println("Bar chart data request received")

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

// 	// Query to get the total GBs per month for notes and media
// 	var data []types.BarChartData
// 	err = db.Raw(`
// 		SELECT
// 			strftime('%Y-%m', timestamp) as month,
// 			SUM(CASE WHEN kind_number IS NOT NULL THEN size ELSE 0 END) / 1024.0 as notes_gb,  -- Convert to GB
// 			SUM(CASE WHEN kind_number IS NULL THEN size ELSE 0 END) / 1024.0 as media_gb  -- Convert to GB
// 		FROM (
// 			SELECT timestamp, size, kind_number FROM kinds
// 			UNION ALL
// 			SELECT timestamp, size, NULL as kind_number FROM photos
// 			UNION ALL
// 			SELECT timestamp, size, NULL as kind_number FROM videos
// 			UNION ALL
// 			SELECT timestamp, size, NULL as kind_number FROM git_nestrs
// 		)
// 		GROUP BY month
// 	`).Scan(&data).Error

// 	if err != nil {
// 		log.Println("Error fetching bar chart data:", err)
// 		return c.Status(500).SendString("Internal Server Error")
// 	}

// 	// Generate data for all months from the beginning of the year to the current month
// 	fullYearData := generateFullYearBarChartData(data)

// 	log.Printf("Fetched data: %+v", fullYearData)
// 	return c.JSON(fullYearData)
// }

// func generateFullYearBarChartData(data []types.BarChartData) []types.BarChartData {
// 	currentMonth := time.Now().Month()
// 	months := make(map[string]types.BarChartData)

// 	// Initialize months with zero counts up to the current month
// 	for month := 1; month <= int(currentMonth); month++ {
// 		monthStr := time.Month(month).String()
// 		months[monthStr] = types.BarChartData{
// 			Month:   monthStr,
// 			NotesGB: 0,
// 			MediaGB: 0,
// 		}
// 	}

// 	// Fill in the data for the months that have data
// 	for _, entry := range data {
// 		// Convert entry.Month from "YYYY-MM" to full month name
// 		if t, err := time.Parse("2006-01", entry.Month); err == nil {
// 			monthName := t.Month().String()
// 			entry.Month = monthName
// 		}
// 		monthData := months[entry.Month]
// 		monthData.NotesGB += entry.NotesGB
// 		monthData.MediaGB += entry.MediaGB
// 		months[entry.Month] = monthData
// 	}

// 	// Convert map to slice
// 	fullYearData := make([]types.BarChartData, 0, int(currentMonth))
// 	for month := 1; month <= int(currentMonth); month++ {
// 		monthStr := time.Month(month).String()
// 		fullYearData = append(fullYearData, months[monthStr])
// 	}

// 	return fullYearData
// }
