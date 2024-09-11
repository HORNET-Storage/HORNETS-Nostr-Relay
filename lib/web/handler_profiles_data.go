package web

import (
	"log"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func getProfilesTimeSeriesData(c *fiber.Ctx) error {
	log.Println("Time series request received")

	// Retrieve the database path from the config file using Viper
	dbPath := viper.GetString("relay_stats_db")
	if dbPath == "" {
		log.Fatal("Database path not found in config")
		return c.Status(fiber.StatusInternalServerError).SendString("Database configuration not found")
	}

	// Initialize the Gorm database
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Printf("Failed to connect to the database: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Calculate the date range for the last 6 months
	endDate := time.Now()
	startDate := endDate.AddDate(0, -6, 0).Format("2006-01") // Adjust to start exactly 6 months ago
	log.Printf("Start date: %s, End date: %s", startDate, endDate.Format("2006-01"))

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
		WHERE strftime('%Y-%m', timestamp) >= '2024-02' AND strftime('%Y-%m', timestamp) < '2024-08'
		GROUP BY month
		ORDER BY month ASC;
    `, startDate, endDate.Format("2006-01")).Scan(&data).Error

	if err != nil {
		log.Println("Error fetching time series data:", err)
		return c.Status(500).SendString("Internal Server Error")
	}

	completeData := make([]types.TimeSeriesData, 6)
	currentMonth := time.Now()

	// Ensure the loop starts from 0 and goes to 5, generating a full 6 months.
	for i := 0; i < 6; i++ {
		// Subtract i months from the current month.
		// Time.Date gives more control and avoids issues at month boundaries.
		month := time.Date(currentMonth.Year(), currentMonth.Month()-time.Month(i), 1, 0, 0, 0, 0, currentMonth.Location())
		// Format the time as "YYYY-MM".
		formattedMonth := month.Format("2006-01")
		// Store the months in reverse order to list from oldest to newest.
		completeData[5-i] = types.TimeSeriesData{Month: formattedMonth}
	}

	log.Printf("Complete list of months: %+v", completeData)

	// Merge queried data with the complete list
	dataMap := make(map[string]types.TimeSeriesData)
	for _, d := range data {
		dataMap[d.Month] = d
	}

	for i, cd := range completeData {
		if d, exists := dataMap[cd.Month]; exists {
			completeData[i] = d
		} else {
			// If it doesn't exist, it will keep the zero values but with the correct month set
			completeData[i].Month = cd.Month
		}
	}

	log.Printf("Fetched data for the last 6 months: %+v", completeData)
	return c.JSON(completeData)
}
