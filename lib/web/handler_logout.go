package web

import (
	"log"
	"strings"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	"github.com/gofiber/fiber/v2"
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
	db, err := graviton.InitGorm()
	if err != nil {
		log.Printf("Failed to connect to the database: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Internal server error",
		})
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
