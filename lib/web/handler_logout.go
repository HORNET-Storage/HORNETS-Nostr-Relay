package web

import (
	"log"
	"strings"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func logoutUser(c *fiber.Ctx) error {
	token := c.Get("Authorization")
	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "No token provided",
		})
	}

	// Remove "Bearer " prefix if present
	token = strings.TrimPrefix(token, "Bearer ")

	// Initialize the database connection
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

	// Delete the token from ActiveTokens
	result := db.Where("token = ?", token).Delete(&types.ActiveToken{})
	if result.Error != nil {
		log.Printf("Failed to delete token: %v", result.Error)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to logout",
		})
	}

	if result.RowsAffected == 0 {
		// Token wasn't found, but we'll still consider this a successful logout
		log.Printf("Token not found in ActiveTokens, but proceeding with logout")
	}

	return c.JSON(fiber.Map{
		"message": "Successfully logged out",
	})
}
