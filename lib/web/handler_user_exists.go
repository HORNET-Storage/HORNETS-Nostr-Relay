package web

import (
	"log"

	gorm "github.com/HORNET-Storage/hornet-storage/lib/stores/stats_stores"
	"github.com/gofiber/fiber/v2"
)

func checkUserExists(c *fiber.Ctx, store *gorm.GormStatisticsStore) error {
	log.Println("Checking if user exists...")

	// Check if any user exists in the database using the store
	exists, err := store.UserExists()
	if err != nil {
		log.Printf("Error checking if user exists: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// If no users exist, allow signup
	if !exists {
		return c.JSON(fiber.Map{
			"exists":      false,
			"allowSignup": true,
			"message":     "No users found. Signup is allowed.",
		})
	}

	// If a user exists, disallow signup
	return c.JSON(fiber.Map{
		"exists":      true,
		"allowSignup": false,
		"message":     "User exists. Signup is not allowed.",
	})
}
