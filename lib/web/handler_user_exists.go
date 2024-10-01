package web

import (
	"log"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func checkUserExists(c *fiber.Ctx) error {
	log.Println("Checking if user exists...")
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

	var user types.User
	if err := db.First(&user).Error; err != nil {
		return c.JSON(fiber.Map{
			"exists":      false,
			"allowSignup": true,
			"message":     "No users found. Signup is allowed.",
		})
	}

	return c.JSON(fiber.Map{
		"exists":      true,
		"allowSignup": false,
		"message":     "User exists. Signup is not allowed.",
	})
}
