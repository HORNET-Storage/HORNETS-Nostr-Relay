package web

import (
	"log"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func getRelayCount(c *fiber.Ctx) error {
	log.Println("Relay count request received")

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
		Audio    []string `mapstructure:"audio"`
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
		"audio":    0, // Add audio to response data
		"misc":     0, // Add misc to response data
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

	responseData["audio"], err = getAudioCounts(db) // Add audio counts
	if err != nil {
		log.Printf("Error getting audio counts: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Error getting audio counts")
	}

	responseData["misc"], err = getMiscCounts(db) // Add misc counts
	if err != nil {
		log.Printf("Error getting misc counts: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Error getting misc counts")
	}

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

func getAudioCounts(db *gorm.DB) (int, error) {
	var count int64
	err := db.Model(&types.Audio{}).Count(&count).Error
	return int(count), err
}

func getMiscCounts(db *gorm.DB) (int, error) {
	var count int64
	err := db.Model(&types.Misc{}).Count(&count).Error
	return int(count), err
}
