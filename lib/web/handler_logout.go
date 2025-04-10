package web

import (
	"log"
	"strings"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
)

// Refactored logoutUser function
func logoutUser(c *fiber.Ctx, store stores.Store) error {
	// In demo mode, just return success without checking tokens
	if viper.GetBool("demo_mode") {
		return c.JSON(fiber.Map{
			"message": "Successfully logged out from demo mode",
		})
	}

	// Get the Authorization token
	token := c.Get("Authorization")
	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "No token provided",
		})
	}

	// Remove "Bearer " prefix if present
	token = strings.TrimPrefix(token, "Bearer ")

	// Find the user associated with this token
	user, err := store.GetStatsStore().FindUserByToken(token)
	if err != nil {
		log.Printf("Failed to find user for token during logout: %v", err)
		// Still return success as we want to log out anyway
		return c.JSON(fiber.Map{
			"message": "Successfully logged out",
		})
	}

	// If we found the user, delete all their active tokens
	if user != nil {
		if err := store.GetStatsStore().DeleteActiveToken(user.ID); err != nil {
			log.Printf("Failed to delete tokens for user %d: %v", user.ID, err)
			// Still return success as we want to log out anyway
		}
	}

	// Return a successful logout message
	return c.JSON(fiber.Map{
		"message": "Successfully logged out",
	})
}
