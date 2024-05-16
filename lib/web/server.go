package web

import (
	"fmt"
	"log"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	jsoniter "github.com/json-iterator/go"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func StartServer() error {
	app := fiber.New()

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	// Dedicated routes for each handler
	app.Post("/relaycount", handleRelayCount)
	app.Post("/relay-settings", handleRelaySettings)
	app.Post("/timeseries", handleTimeSeries)

	return app.Listen(":5000")
}

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
func handleRelayCount(c *fiber.Ctx) error {
	log.Println("Relay count request received")
	// var json = jsoniter.ConfigCompatibleWithStandardLibrary

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

	// Retrieve relay settings from the config file using Viper
	var relaySettings struct {
		Kinds    []string `mapstructure:"kinds"`
		Photos   []string `mapstructure:"photos"`
		Videos   []string `mapstructure:"videos"`
		GitNestr []string `mapstructure:"gitNestr"`
	}
	if err := viper.UnmarshalKey("relay_settings", &relaySettings); err != nil {
		log.Fatalf("Error unmarshaling relay settings: %v", err)
	}

	// Initialize the response data
	responseData := map[string]int{
		"kinds":    0,
		"photos":   0,
		"videos":   0,
		"gitNestr": 0,
	}

	// Aggregate counts for each category
	responseData["kinds"], err = getKindCounts(db)
	if err != nil {
		log.Printf("Error getting kind counts: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Error getting kind counts")
	}

	responseData["photos"], err = getPhotoCounts(db)
	if err != nil {
		log.Printf("Error getting photo counts: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Error getting photo counts")
	}

	responseData["videos"], err = getVideoCounts(db)
	if err != nil {
		log.Printf("Error getting video counts: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Error getting video counts")
	}

	responseData["gitNestr"], err = getGitNestrCounts(db, relaySettings.GitNestr)
	if err != nil {
		log.Printf("Error getting gitNestr counts: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Error getting gitNestr counts")
	}

	log.Printf("Response Data: %+v", responseData)
	return c.JSON(responseData)
}

func getKindCounts(db *gorm.DB) (int, error) {
	var count int64
	err := db.Model(&types.Kind{}).Count(&count).Error
	return int(count), err
}

func getPhotoCounts(db *gorm.DB) (int, error) {
	var count int64
	err := db.Model(&types.Photo{}).Count(&count).Error
	return int(count), err
}

func getVideoCounts(db *gorm.DB) (int, error) {
	var count int64
	err := db.Model(&types.Video{}).Count(&count).Error
	return int(count), err
}

func getGitNestrCounts(db *gorm.DB, gitNestr []string) (int, error) {
	var count int64
	err := db.Model(&types.GitNestr{}).Where("git_type IN ?", gitNestr).Count(&count).Error
	return int(count), err
}

func handleRelaySettings(c *fiber.Ctx) error {
	log.Println("Relay settings request received")
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	var data map[string]interface{}
	if err := c.BodyParser(&data); err != nil {
		return c.Status(400).SendString(err.Error())
	}

	relaySettingsData, ok := data["relay_settings"]
	if !ok {
		log.Println("Relay settings data not provided")
		return c.Status(400).SendString("Relay settings data expected")
	}

	var relaySettings types.RelaySettings
	relaySettingsJSON, err := json.Marshal(relaySettingsData)
	if err != nil {
		log.Println("Error marshaling relay settings:", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	log.Println("Received relay settings JSON:", string(relaySettingsJSON))

	if err := json.Unmarshal(relaySettingsJSON, &relaySettings); err != nil {
		log.Println("Error unmarshaling relay settings:", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Store in Viper
	viper.Set("relay_settings", relaySettings)

	// Save the changes to the configuration file
	if err := viper.WriteConfig(); err != nil {
		log.Printf("Error writing config: %s", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to update settings")
	}

	log.Println("Stored relay settings:", relaySettings)

	return c.SendStatus(fiber.StatusOK)
}
