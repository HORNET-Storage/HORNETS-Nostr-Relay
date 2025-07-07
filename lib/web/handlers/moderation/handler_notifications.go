package moderation

import (
	"strconv"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
	"github.com/nbd-wtf/go-nostr"
)

func GetModerationNotifications(c *fiber.Ctx, store stores.Store) error {
	page, err := strconv.Atoi(c.Query("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(c.Query("limit", "10"))
	if err != nil || limit < 1 || limit > 100 {
		limit = 10
	}

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

	return c.JSON(fiber.Map{
		"notifications": notifications,
		"pagination":    metadata,
	})
}

func MarkNotificationAsRead(c *fiber.Ctx, store stores.Store) error {
	var req struct {
		ID uint `json:"id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	logging.Infof("Notification %v has been read.", req.ID)

	if req.ID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Notification ID is required",
		})
	}

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

func MarkAllNotificationsAsRead(c *fiber.Ctx, store stores.Store) error {
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

func GetModerationStats(c *fiber.Ctx, store stores.Store) error {
	stats, err := store.GetStatsStore().GetModerationStats()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch moderation statistics: " + err.Error(),
		})
	}

	return c.JSON(stats)
}

func CreateModerationNotification(c *fiber.Ctx, store stores.Store) error {
	var notification lib.ModerationNotification

	if err := c.BodyParser(&notification); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = time.Now()
	}

	if notification.PubKey == "" || notification.EventID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Pubkey and EventID are required",
		})
	}

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

func GetBlockedEvent(c *fiber.Ctx, store stores.Store) error {
	eventID := c.Params("id")
	if eventID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Event ID is required",
		})
	}

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

	return c.JSON(fiber.Map{
		"event":              events[0],
		"moderation_details": notification,
	})
}

func UnblockEvent(c *fiber.Ctx, store stores.Store) error {
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

	if err := store.UnmarkEventBlocked(req.EventID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to unblock event: " + err.Error(),
		})
	}

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

func DeleteModeratedEvent(c *fiber.Ctx, store stores.Store) error {
	// Get event ID from the URL parameters
	eventID := c.Params("id")
	if eventID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Event ID is required",
		})
	}

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

	if err := store.DeleteEvent(eventID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete event: " + err.Error(),
		})
	}

	if err := store.UnmarkEventBlocked(eventID); err != nil {
		// Log but don't fail - the main deletion was successful
		logging.Infof("Error removing event %s from blocked list: %v", eventID, err)
	}

	notifications, _, err := store.GetStatsStore().GetAllModerationNotifications(1, 100)
	if err == nil {
		for _, notification := range notifications {
			if notification.EventID == eventID {
				store.GetStatsStore().DeleteModerationNotification(notification.ID)
				break
			}
		}
	}

	filter := nostr.Filter{
		Kinds: []int{19841},
		Tags: nostr.TagMap{
			"e": []string{eventID},
		},
	}

	moderationTickets, err := store.QueryEvents(filter)
	if err == nil && len(moderationTickets) > 0 {
		for _, ticket := range moderationTickets {
			if err := store.DeleteEvent(ticket.ID); err != nil {
				// Log but don't fail - the main deletion was successful
				logging.Infof("Error deleting moderation ticket %s for event %s: %v", ticket.ID, eventID, err)
			} else {
				logging.Infof("Successfully deleted moderation ticket %s for event %s", ticket.ID, eventID)
			}
		}
	}

	return c.JSON(fiber.Map{
		"success":  true,
		"message":  "Event permanently deleted",
		"event_id": eventID,
	})
}
