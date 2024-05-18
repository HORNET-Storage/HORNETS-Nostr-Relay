package web

import (
	"fmt"
	"log"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func handleTimeSeries(c *fiber.Ctx) error {
	log.Println("Time series request received")

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

	// Query data from the UserProfile table
	var data []types.TimeSeriesData
	err = db.Raw(`
		SELECT 
			strftime('%Y-%m', timestamp) as month,
			COUNT(*) as profiles,
			COUNT(CASE WHEN lightning_addr THEN 1 ELSE NULL END) as lightning_addr,
			COUNT(CASE WHEN dht_key THEN 1 ELSE NULL END) as dht_key,
			COUNT(CASE WHEN lightning_addr AND dht_key THEN 1 ELSE NULL END) as lightning_and_dht
		FROM user_profiles
		GROUP BY month
	`).Scan(&data).Error

	if err != nil {
		log.Println("Error fetching time series data:", err)
		return c.Status(500).SendString("Internal Server Error")
	}

	// Generate data for all months from the beginning of the year to the current month
	fullYearData := generateFullYearData(data)

	log.Printf("Fetched data: %+v", fullYearData)
	return c.JSON(fullYearData)
}

func generateFullYearData(data []types.TimeSeriesData) []types.TimeSeriesData {
	currentYear := time.Now().Year()
	currentMonth := time.Now().Month()
	months := make(map[string]types.TimeSeriesData)

	// Initialize months with zero counts up to the current month
	for month := 1; month <= int(currentMonth); month++ {
		monthStr := fmt.Sprintf("%d-%02d", currentYear, month)
		months[monthStr] = types.TimeSeriesData{
			Month:           monthStr,
			Profiles:        0,
			LightningAddr:   0,
			DHTKey:          0,
			LightningAndDHT: 0,
		}
	}

	// Fill in the data for the months that have data
	for _, entry := range data {
		months[entry.Month] = entry
	}

	// Convert map to slice
	fullYearData := make([]types.TimeSeriesData, 0, int(currentMonth))
	for month := 1; month <= int(currentMonth); month++ {
		monthStr := fmt.Sprintf("%d-%02d", currentYear, month)
		fullYearData = append(fullYearData, months[monthStr])
	}

	return fullYearData
}
