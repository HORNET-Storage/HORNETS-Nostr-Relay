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

func handleTimeSeries(c *fiber.Ctx) error {
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

	log.Printf("Queried data: %+v", data)

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

// func handleTimeSeries(c *fiber.Ctx) error {
// 	log.Println("Time series request received")

// 	// Retrieve the database path from the config file using Viper
// 	dbPath := viper.GetString("relay_stats_db")
// 	if dbPath == "" {
// 		log.Fatal("Database path not found in config")
// 		return c.Status(fiber.StatusInternalServerError).SendString("Database configuration not found")
// 	}

// 	// Initialize the Gorm database
// 	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
// 	if err != nil {
// 		log.Printf("Failed to connect to the database: %v", err)
// 		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
// 	}

// 	// Calculate the date range for the last 6 months
// 	endDate := time.Now()
// 	startDate := endDate.AddDate(0, -6, 0).Format("2006-01") // Adjust to start exactly 6 months ago
// 	log.Printf("Start date: %s, End date: %s", startDate, endDate.Format("2006-01"))

// 	// Query data from the UserProfile table
// 	var data []types.TimeSeriesData
// 	err = db.Raw(`
//         SELECT
// 			strftime('%Y-%m', timestamp) as month,
// 			COUNT(*) as profiles,
// 			COUNT(CASE WHEN lightning_addr THEN 1 ELSE NULL END) as lightning_addr,
// 			COUNT(CASE WHEN dht_key THEN 1 ELSE NULL END) as dht_key,
// 			COUNT(CASE WHEN lightning_addr AND dht_key THEN 1 ELSE NULL END) as lightning_and_dht
// 		FROM user_profiles
// 		WHERE strftime('%Y-%m', timestamp) >= '2024-01' AND strftime('%Y-%m', timestamp) < '2024-07'
// 		GROUP BY month
// 		ORDER BY month ASC;
//     `, startDate, endDate.Format("2006-01")).Scan(&data).Error

// 	if err != nil {
// 		log.Println("Error fetching time series data:", err)
// 		return c.Status(500).SendString("Internal Server Error")
// 	}

// 	log.Printf("Queried data: %+v", data)

// 	// Generate a complete list of the last 6 months
// 	completeData := make([]types.TimeSeriesData, 6)
// 	for i := 0; i < 6; i++ {
// 		month := endDate.AddDate(0, -i, 0).Format("2006-01")
// 		completeData[5-i] = types.TimeSeriesData{Month: month} // Ensuring months are stored from oldest to newest
// 	}

// 	log.Printf("Complete list of months: %+v", completeData)

// 	// Merge queried data with the complete list
// 	dataMap := make(map[string]types.TimeSeriesData)
// 	for _, d := range data {
// 		dataMap[d.Month] = d
// 	}

// 	for i, cd := range completeData {
// 		if d, exists := dataMap[cd.Month]; exists {
// 			completeData[i] = d
// 		} else {
// 			// If it doesn't exist, it will keep the zero values but with the correct month set
// 			completeData[i].Month = cd.Month
// 		}
// 	}

// 	log.Printf("Fetched data for the last 6 months: %+v", completeData)
// 	return c.JSON(completeData)
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

// func handleTimeSeries(c *fiber.Ctx) error {
// 	log.Println("Time series request received")

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

// 	// Calculate the date range for the last 6 months
// 	endDate := time.Now()
// 	startDate := endDate.AddDate(0, -6, 0).Format("2006-01") // Adjust to start exactly 6 months ago
// 	log.Printf("Start date: %s, End date: %s", startDate, endDate.Format("2006-01"))

// 	// Query data from the UserProfile table
// 	var data []types.TimeSeriesData
// 	err = db.Raw(`
//         SELECT
// 			strftime('%Y-%m', timestamp) as month,
// 			COUNT(*) as profiles,
// 			COUNT(CASE WHEN lightning_addr THEN 1 ELSE NULL END) as lightning_addr,
// 			COUNT(CASE WHEN dht_key THEN 1 ELSE NULL END) as dht_key,
// 			COUNT(CASE WHEN lightning_addr AND dht_key THEN 1 ELSE NULL END) as lightning_and_dht
// 		FROM user_profiles
// 		WHERE strftime('%Y-%m', timestamp) >= '2024-01' AND strftime('%Y-%m', timestamp) < '2024-07'
// 		GROUP BY month
// 		ORDER BY month ASC;
//     `, startDate, endDate.Format("2006-01")).Scan(&data).Error

// 	if err != nil {
// 		log.Println("Error fetching time series data:", err)
// 		return c.Status(500).SendString("Internal Server Error")
// 	}

// 	// Generate a complete list of the last 6 months
// 	completeData := make([]types.TimeSeriesData, 6)
// 	for i := 0; i < 6; i++ {
// 		month := endDate.AddDate(0, -i, 0).Format("2006-01")
// 		completeData[5-i] = types.TimeSeriesData{Month: month} // Ensuring months are stored from oldest to newest
// 	}

// 	// Merge queried data with the complete list
// 	dataMap := make(map[string]types.TimeSeriesData)
// 	for _, d := range data {
// 		dataMap[d.Month] = d
// 	}

// 	for i, cd := range completeData {
// 		if d, exists := dataMap[cd.Month]; exists {
// 			completeData[i] = d
// 		}
// 		// If it doesn't exist, it will keep the zero values
// 	}

// 	log.Printf("Fetched data for the last 6 months: %+v", completeData)
// 	return c.JSON(completeData)
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

// func handleTimeSeries(c *fiber.Ctx) error {
// 	log.Println("Time series request received")

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

// 	// Calculate the date range for the last 6 months
// 	endDate := time.Now()
// 	startDate := endDate.AddDate(0, -6, 0).Format("2006-01")
// 	log.Printf("Start date: %s, End date: %s", startDate, endDate.Format("2006-01"))

// 	// Query data from the UserProfile table
// 	var data []types.TimeSeriesData
// 	err = db.Raw(`
//         SELECT
//             strftime('%Y-%m', timestamp) as month,
//             COUNT(*) as profiles,
//             COUNT(CASE WHEN lightning_addr THEN 1 ELSE NULL END) as lightning_addr,
//             COUNT(CASE WHEN dht_key THEN 1 ELSE NULL END) as dht_key,
//             COUNT(CASE WHEN lightning_addr AND dht_key THEN 1 ELSE NULL END) as lightning_and_dht
//         FROM user_profiles
//         WHERE strftime('%Y-%m', timestamp) >= ?
//         GROUP BY month
//         ORDER BY month ASC
//     `, startDate).Scan(&data).Error

// 	if err != nil {
// 		log.Println("Error fetching time series data:", err)
// 		return c.Status(500).SendString("Internal Server Error")
// 	}

// 	// Generate a complete list of the last 6 months
// 	completeData := make([]types.TimeSeriesData, 6)
// 	for i := 0; i < 6; i++ {
// 		month := endDate.AddDate(0, -i, 0).Format("2006-01")
// 		completeData[5-i] = types.TimeSeriesData{Month: month} // Reverse to order correctly from past to present
// 	}

// 	// Merge queried data with the complete list
// 	dataMap := make(map[string]types.TimeSeriesData)
// 	for _, d := range data {
// 		dataMap[d.Month] = d
// 	}

// 	for i, cd := range completeData {
// 		if d, exists := dataMap[cd.Month]; exists {
// 			completeData[i] = d
// 		}
// 		// If it doesn't exist, it will keep the zero values
// 	}

// 	log.Printf("Fetched data for the last 6 months: %+v", completeData)
// 	return c.JSON(completeData)
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

// func handleTimeSeries(c *fiber.Ctx) error {
// 	log.Println("Time series request received")

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

// 	// Query data from the UserProfile table
// 	var data []types.TimeSeriesData
// 	err = db.Raw(`
// 		SELECT
// 			strftime('%Y-%m', timestamp) as month,
// 			COUNT(*) as profiles,
// 			COUNT(CASE WHEN lightning_addr THEN 1 ELSE NULL END) as lightning_addr,
// 			COUNT(CASE WHEN dht_key THEN 1 ELSE NULL END) as dht_key,
// 			COUNT(CASE WHEN lightning_addr AND dht_key THEN 1 ELSE NULL END) as lightning_and_dht
// 		FROM user_profiles
// 		GROUP BY month
// 	`).Scan(&data).Error

// 	if err != nil {
// 		log.Println("Error fetching time series data:", err)
// 		return c.Status(500).SendString("Internal Server Error")
// 	}

// 	// Add a zero-data point for the month before the first month in the data
// 	if len(data) > 0 {
// 		firstMonth := data[0].Month
// 		firstMonthTime, err := time.Parse("2006-01", firstMonth)
// 		if err != nil {
// 			log.Println("Error parsing first month:", err)
// 			return c.Status(500).SendString("Internal Server Error")
// 		}
// 		previousMonth := firstMonthTime.AddDate(0, -6, 0)
// 		zeroPoint := types.TimeSeriesData{
// 			Month:           previousMonth.Format("2006-01"),
// 			Profiles:        0,
// 			LightningAddr:   0,
// 			DHTKey:          0,
// 			LightningAndDHT: 0,
// 		}
// 		data = append([]types.TimeSeriesData{zeroPoint}, data...)
// 	}

// 	log.Printf("Fetched data with zero point: %+v", data)
// 	return c.JSON(data)
// }

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

// func handleTimeSeries(c *fiber.Ctx) error {
// 	log.Println("Time series request received")

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

// 	// Query data from the UserProfile table
// 	var data []types.TimeSeriesData
// 	err = db.Raw(`
// 		SELECT
// 			strftime('%Y-%m', timestamp) as month,
// 			COUNT(*) as profiles,
// 			COUNT(CASE WHEN lightning_addr THEN 1 ELSE NULL END) as lightning_addr,
// 			COUNT(CASE WHEN dht_key THEN 1 ELSE NULL END) as dht_key,
// 			COUNT(CASE WHEN lightning_addr AND dht_key THEN 1 ELSE NULL END) as lightning_and_dht
// 		FROM user_profiles
// 		GROUP BY month
// 	`).Scan(&data).Error

// 	if err != nil {
// 		log.Println("Error fetching time series data:", err)
// 		return c.Status(500).SendString("Internal Server Error")
// 	}

// 	// Generate data for all months from the beginning of the year to the current month
// 	fullYearData := generateFullYearData(data)

// 	log.Printf("Fetched data: %+v", fullYearData)
// 	return c.JSON(fullYearData)
// }

// func generateFullYearData(data []types.TimeSeriesData) []types.TimeSeriesData {
// 	currentYear := time.Now().Year()
// 	currentMonth := time.Now().Month()
// 	months := make(map[string]types.TimeSeriesData)

// 	// Initialize months with zero counts up to the current month
// 	for month := 1; month <= int(currentMonth); month++ {
// 		monthStr := fmt.Sprintf("%d-%02d", currentYear, month)
// 		months[monthStr] = types.TimeSeriesData{
// 			Month:           monthStr,
// 			Profiles:        0,
// 			LightningAddr:   0,
// 			DHTKey:          0,
// 			LightningAndDHT: 0,
// 		}
// 	}

// 	// Fill in the data for the months that have data
// 	for _, entry := range data {
// 		months[entry.Month] = entry
// 	}

// 	// Convert map to slice
// 	fullYearData := make([]types.TimeSeriesData, 0, int(currentMonth))
// 	for month := 1; month <= int(currentMonth); month++ {
// 		monthStr := fmt.Sprintf("%d-%02d", currentYear, month)
// 		fullYearData = append(fullYearData, months[monthStr])
// 	}

// 	return fullYearData
// }
