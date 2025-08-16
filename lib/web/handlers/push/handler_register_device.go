package push

import (
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/web/middleware"
	"github.com/gofiber/fiber/v2"
)

// RegisterDeviceRequest represents the request body for device registration
type RegisterDeviceRequest struct {
	DeviceToken string `json:"device_token" validate:"required"`
	Platform    string `json:"platform" validate:"required,oneof=ios android"`
}

// RegisterDeviceHandler handles push device registration
func RegisterDeviceHandler(store stores.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get authenticated pubkey from NIP-98 middleware
		pubkey, err := middleware.GetNIP98Pubkey(c)
		if err != nil {
			logging.Errorf("Failed to get NIP-98 pubkey: %v", err)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Authentication required",
			})
		}

		// Parse request body
		var req RegisterDeviceRequest
		if err := c.BodyParser(&req); err != nil {
			logging.Errorf("Failed to parse register device request: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}

		// Validate platform
		if req.Platform != "ios" && req.Platform != "android" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Platform must be 'ios' or 'android'",
			})
		}

		// Validate device token
		if req.DeviceToken == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Device token is required",
			})
		}

		// Register device
		statsStore := store.GetStatsStore()
		err = statsStore.RegisterPushDevice(pubkey, req.DeviceToken, req.Platform)
		if err != nil {
			logging.Errorf("Failed to register push device: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to register device",
			})
		}

		logging.Infof("Registered push device for pubkey %s: %s (%s)", pubkey, req.DeviceToken, req.Platform)

		return c.JSON(fiber.Map{
			"success": true,
			"message": "Device registered successfully",
		})
	}
}
