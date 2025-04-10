package web

import (
	"log"
	"strconv"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
)

// GetModerationNotifications retrieves all moderation notifications with pagination
func getModerationNotifications(c *fiber.Ctx, store stores.Store) error {
	log.Println("MOderations notification request made.")
	// Parse pagination parameters
	page, err := strconv.Atoi(c.Query("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(c.Query("limit", "10"))
	if err != nil || limit < 1 || limit > 100 {
		limit = 10
	}

	// Get notifications based on the filter type
	filterType := c.Query("filter", "all") // all, unread, user
	pubkey := c.Query("pubkey", "")

	var notifications []lib.ModerationNotification
	var metadata *lib.PaginationMetadata
	var fetchErr error

	switch filterType {
	case "unread":
		notifications, metadata, fetchErr = store.GetStatsStore().GetUnreadModerationNotifications(page, limit)
		if fetchErr != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to fetch notifications: " + fetchErr.Error(),
			})
		}

		// Return 204 No Content status with no body when there are no unread notifications
		if len(notifications) == 0 {
			return c.Status(fiber.StatusNoContent).Send(nil)
		}
	case "user":
		if pubkey == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Pubkey parameter is required for user filter",
			})
		}
		notifications, metadata, fetchErr = store.GetStatsStore().GetUserModerationNotifications(pubkey, page, limit)
		if fetchErr != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to fetch notifications: " + fetchErr.Error(),
			})
		}
	default: // "all"
		notifications, metadata, fetchErr = store.GetStatsStore().GetAllModerationNotifications(page, limit)
		if fetchErr != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to fetch notifications: " + fetchErr.Error(),
			})
		}
	}

	log.Println("Notifications: ", notifications)

	return c.JSON(fiber.Map{
		"notifications": notifications,
		"pagination":    metadata,
	})
}

// MarkNotificationAsRead marks a notification as read
func markNotificationAsRead(c *fiber.Ctx, store stores.Store) error {
	// Get notification ID from request body
	var req struct {
		ID uint `json:"id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	log.Printf("Notification %v has been read.", req.ID)

	if req.ID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Notification ID is required",
		})
	}

	// Mark notification as read
	if err := store.GetStatsStore().MarkNotificationAsRead(req.ID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to mark notification as read: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Notification marked as read",
	})
}

// MarkAllNotificationsAsRead marks all notifications as read for a user
func markAllNotificationsAsRead(c *fiber.Ctx, store stores.Store) error {
	// Get user pubkey from request body
	var req struct {
		Pubkey string `json:"pubkey"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.Pubkey == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "User pubkey is required",
		})
	}

	// Mark all notifications as read for the user
	if err := store.GetStatsStore().MarkAllNotificationsAsRead(req.Pubkey); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to mark notifications as read: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "All notifications marked as read",
	})
}

// GetModerationStats gets statistics about blocked content
func getModerationStats(c *fiber.Ctx, store stores.Store) error {
	// Get moderation statistics
	stats, err := store.GetStatsStore().GetModerationStats()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch moderation statistics: " + err.Error(),
		})
	}

	return c.JSON(stats)
}

// CreateModerationNotification creates a new moderation notification (for testing only)
func createModerationNotification(c *fiber.Ctx, store stores.Store) error {
	// This handler should only be used for testing
	var notification lib.ModerationNotification

	if err := c.BodyParser(&notification); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Set creation time if not provided
	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = time.Now()
	}

	// Make sure required fields are present
	if notification.PubKey == "" || notification.EventID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Pubkey and EventID are required",
		})
	}

	// Create the notification
	if err := store.GetStatsStore().CreateModerationNotification(&notification); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create notification: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Notification created successfully",
		"id":      notification.ID,
	})
}
