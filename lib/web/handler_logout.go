package web

import (
	"log"
	"strings"

	gorm "github.com/HORNET-Storage/hornet-storage/lib/stores/stats_stores"
	"github.com/gofiber/fiber/v2"
)

// Refactored logoutUser function
func logoutUser(c *fiber.Ctx, store *gorm.GormStatisticsStore) error {
	// Get the Authorization token
	token := c.Get("Authorization")
	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "No token provided",
		})
	}

	// Remove "Bearer " prefix if present
	token = strings.TrimPrefix(token, "Bearer ")

	// Delete the token from ActiveTokens using the statistics store
	if err := store.DeleteActiveToken(token); err != nil {
		log.Printf("Failed to delete token: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to logout",
		})
	}

	// Return a successful logout message
	return c.JSON(fiber.Map{
		"message": "Successfully logged out",
	})
}
