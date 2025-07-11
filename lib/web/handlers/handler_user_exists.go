package handlers

import (
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
)

func CheckUserExists(c *fiber.Ctx, store stores.Store) error {
	logging.Info("Checking if user exists...")

	// Check if any user exists in the database using the store
	exists, err := store.GetStatsStore().UserExists()
	if err != nil {
		logging.Infof("Error checking if user exists: %v", err)
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
