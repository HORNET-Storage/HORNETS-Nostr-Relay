package push

import (
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/web/middleware"
	"github.com/gofiber/fiber/v2"
	"github.com/nbd-wtf/go-nostr"

	pushService "github.com/HORNET-Storage/hornet-storage/services/push"
)

// TestNotificationRequest represents the request body for test notification
type TestNotificationRequest struct {
	Message string `json:"message,omitempty"`
	Title   string `json:"title,omitempty"`
}

// TestNotificationHandler handles sending test push notifications
func TestNotificationHandler(store stores.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get authenticated pubkey from NIP-98 middleware
		pubkey, err := middleware.GetNIP98Pubkey(c)
		if err != nil {
			logging.Errorf("Failed to get NIP-98 pubkey: %v", err)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Authentication required",
			})
		}

		// Parse request body (optional)
		var req TestNotificationRequest
		if err := c.BodyParser(&req); err != nil {
			// If parsing fails, use defaults
			req.Title = "Test Notification"
			req.Message = "This is a test push notification from HORNETS Relay"
		}

		// Set defaults if not provided
		if req.Title == "" {
			req.Title = "Test Notification"
		}
		if req.Message == "" {
			req.Message = "This is a test push notification from HORNETS Relay"
		}

		// Get devices for this user
		statsStore := store.GetStatsStore()
		devices, err := statsStore.GetPushDevicesByPubkey(pubkey)
		if err != nil {
			logging.Errorf("Failed to get devices for pubkey %s: %v", pubkey, err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to get user devices",
			})
		}

		if len(devices) == 0 {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "No devices registered for this user",
			})
		}

		// Get push service
		ps := pushService.GetGlobalPushService()
		if ps == nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "Push notification service not available",
			})
		}

		// Create a fake test event
		// Using a special test pubkey that the notification service recognizes
		testEvent := &nostr.Event{
			ID:      "test_notification_" + pubkey,
			PubKey:  "0000000000000000000000000000000000000000000000000000000000000000",
			Kind:    1808, // Audio note
			Content: req.Message,
			Tags:    nostr.Tags{nostr.Tag{"p", pubkey}},
		}

		// Process the test event
		ps.ProcessEvent(testEvent)

		logging.Infof("Sent test notification to %d devices for pubkey %s", len(devices), pubkey)

		return c.JSON(fiber.Map{
			"success":      true,
			"message":      "Test notification sent successfully",
			"device_count": len(devices),
		})
	}
}
