package push

import (
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/web/middleware"
	"github.com/gofiber/fiber/v2"
)

// UnregisterDeviceRequest represents the request body for device unregistration
type UnregisterDeviceRequest struct {
	DeviceToken string `json:"device_token" validate:"required"`
}

// UnregisterDeviceHandler handles push device unregistration
func UnregisterDeviceHandler(store stores.Store) fiber.Handler {
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
		var req UnregisterDeviceRequest
		if err := c.BodyParser(&req); err != nil {
			logging.Errorf("Failed to parse unregister device request: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}

		// Validate device token
		if req.DeviceToken == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Device token is required",
			})
		}

		// Unregister device
		statsStore := store.GetStatsStore()
		err = statsStore.UnregisterPushDevice(pubkey, req.DeviceToken)
		if err != nil {
			logging.Errorf("Failed to unregister push device: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to unregister device",
			})
		}

		logging.Infof("Unregistered push device for pubkey %s: %s", pubkey, req.DeviceToken)

		return c.JSON(fiber.Map{
			"success": true,
			"message": "Device unregistered successfully",
		})
	}
}
