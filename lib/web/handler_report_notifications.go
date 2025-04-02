package web

import (
	"log"
	"strconv"

	"github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
	"github.com/nbd-wtf/go-nostr"
)

// GetReportNotifications retrieves all report notifications with pagination
func getReportNotifications(c *fiber.Ctx, store stores.Store) error {
	log.Println("Report notification request made.")
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
	filterType := c.Query("filter", "all") // all, unread

	var notifications []lib.ReportNotification
	var metadata *lib.PaginationMetadata
	var fetchErr error

	switch filterType {
	case "unread":
		notifications, metadata, fetchErr = store.GetStatsStore().GetUnreadReportNotifications(page, limit)
		if fetchErr != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to fetch report notifications: " + fetchErr.Error(),
			})
		}

		// Return 204 No Content status with no body when there are no unread notifications
		if len(notifications) == 0 {
			return c.Status(fiber.StatusNoContent).Send(nil)
		}
	default: // "all"
		notifications, metadata, fetchErr = store.GetStatsStore().GetAllReportNotifications(page, limit)
		if fetchErr != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to fetch report notifications: " + fetchErr.Error(),
			})
		}
	}

	log.Println("Report Notifications: ", notifications)

	return c.JSON(fiber.Map{
		"notifications": notifications,
		"pagination":    metadata,
	})
}

// MarkReportNotificationAsRead marks a report notification as read
func markReportNotificationAsRead(c *fiber.Ctx, store stores.Store) error {
	// Get notification ID from request body
	var req struct {
		ID uint `json:"id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	log.Printf("Report notification %v has been read.", req.ID)

	if req.ID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Notification ID is required",
		})
	}

	// Mark notification as read
	if err := store.GetStatsStore().MarkReportNotificationAsRead(req.ID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to mark notification as read: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Notification marked as read",
	})
}

// MarkAllReportNotificationsAsRead marks all report notifications as read
func markAllReportNotificationsAsRead(c *fiber.Ctx, store stores.Store) error {
	// Mark all notifications as read
	if err := store.GetStatsStore().MarkAllReportNotificationsAsRead(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to mark all notifications as read: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "All report notifications marked as read",
	})
}

// GetReportStats gets statistics about reported content
func getReportStats(c *fiber.Ctx, store stores.Store) error {
	// Get report statistics
	stats, err := store.GetStatsStore().GetReportStats()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch report statistics: " + err.Error(),
		})
	}

	return c.JSON(stats)
}

// GetReportedEvent retrieves the original event that was reported
func getReportedEvent(c *fiber.Ctx, store stores.Store) error {
	// Get event ID from the URL parameters
	eventID := c.Params("id")
	if eventID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Event ID is required",
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

	// Return the event
	return c.JSON(fiber.Map{
		"event": events[0],
	})
}
