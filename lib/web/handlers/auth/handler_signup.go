package auth

import (
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
)

// Refactored signUpUser function
func SignUpUser(c *fiber.Ctx, store stores.Store) error {
	logging.Info("Sign-up request received")
	var signUpPayload struct {
		Npub     string `json:"npub"`
		Password string `json:"password"`
	}

	// Parse the JSON body into the struct
	if err := c.BodyParser(&signUpPayload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	// Use the statistics store to sign up the user
	err := store.GetStatsStore().SignUpUser(signUpPayload.Npub, signUpPayload.Password)
	if err != nil {
		logging.Infof("Failed to create user: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Respond with success message
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "User created successfully",
	})
}
