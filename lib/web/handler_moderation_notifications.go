package web

import (
	"log"
	"strconv"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
	"github.com/nbd-wtf/go-nostr"
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

// GetBlockedEvent retrieves a blocked event by its ID
func getBlockedEvent(c *fiber.Ctx, store stores.Store) error {
	// Get event ID from the URL parameters
	eventID := c.Params("id")
	if eventID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Event ID is required",
		})
	}

	// Check if the event is actually blocked
	isBlocked, err := store.IsEventBlocked(eventID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to check event status: " + err.Error(),
		})
	}

	if !isBlocked {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Event is not currently blocked",
		})
	}

	// Query the event from the store
	filter := nostr.Filter{
		IDs: []string{eventID},
	}
	events, err := store.QueryEvents(filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to retrieve event: " + err.Error(),
		})
	}

	if len(events) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Event not found",
		})
	}

	// Get the moderation notification for this event to include the reason
	var notification *lib.ModerationNotification
	notifications, _, err := store.GetStatsStore().GetAllModerationNotifications(1, 100)
	if err == nil {
		for _, n := range notifications {
			if n.EventID == eventID {
				notification = &n
				break
			}
		}
	}

	log.Println("Blocked event: ", events[0])

	// Return the event along with its moderation details
	return c.JSON(fiber.Map{
		"event":              events[0],
		"moderation_details": notification,
	})
}

// UnblockEvent handles requests to unblock incorrectly flagged content
func unblockEvent(c *fiber.Ctx, store stores.Store) error {
	// Get event ID from request body
	var req struct {
		EventID string `json:"event_id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.EventID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Event ID is required",
		})
	}

	// Check if the event is actually blocked
	isBlocked, err := store.IsEventBlocked(req.EventID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to check event status: " + err.Error(),
		})
	}

	if !isBlocked {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Event is not currently blocked",
		})
	}

	// Unblock the event
	if err := store.UnmarkEventBlocked(req.EventID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to unblock event: " + err.Error(),
		})
	}

	// Find and delete the corresponding moderation notification
	notifications, _, err := store.GetStatsStore().GetAllModerationNotifications(1, 100)
	if err == nil {
		for _, notification := range notifications {
			if notification.EventID == req.EventID {
				store.GetStatsStore().DeleteModerationNotification(notification.ID)
				break
			}
		}
	}

	return c.JSON(fiber.Map{
		"success":  true,
		"message":  "Event unblocked successfully",
		"event_id": req.EventID,
	})
}

// DeleteModeratedEvent permanently deletes a moderated event from the relay
func deleteModeratedEvent(c *fiber.Ctx, store stores.Store) error {
	// Get event ID from the URL parameters
	eventID := c.Params("id")
	if eventID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Event ID is required",
		})
	}

	// Check if the event is actually blocked
	isBlocked, err := store.IsEventBlocked(eventID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to check event status: " + err.Error(),
		})
	}

	if !isBlocked {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Event is not currently blocked",
		})
	}

	// Delete the event from the database
	if err := store.DeleteEvent(eventID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete event: " + err.Error(),
		})
	}

	// Remove from blocked list
	if err := store.UnmarkEventBlocked(eventID); err != nil {
		// Log but don't fail - the main deletion was successful
		log.Printf("Error removing event %s from blocked list: %v", eventID, err)
	}

	// Find and delete the corresponding moderation notification
	notifications, _, err := store.GetStatsStore().GetAllModerationNotifications(1, 100)
	if err == nil {
		for _, notification := range notifications {
			if notification.EventID == eventID {
				store.GetStatsStore().DeleteModerationNotification(notification.ID)
				break
			}
		}
	}

	// Return success response
	return c.JSON(fiber.Map{
		"success":  true,
		"message":  "Event permanently deleted",
		"event_id": eventID,
	})
}
