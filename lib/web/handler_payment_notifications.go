package web

import (
	"log"
	"strconv"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
)

// getPaymentNotifications retrieves payment notifications with pagination
func getPaymentNotifications(c *fiber.Ctx, store stores.Store) error {
	log.Println("Payment notification request made")

	// Log detailed request information
	log.Printf("Request details - URL: %s, Method: %s", c.OriginalURL(), c.Method())
	log.Printf("Query parameters - page: %s, limit: %s, filter: %s, pubkey: %s",
		c.Query("page", "1"),
		c.Query("limit", "10"),
		c.Query("filter", "all"),
		c.Query("pubkey", ""))

	// Log request body if present
	if len(c.Body()) > 0 {
		log.Printf("Request body: %s", string(c.Body()))
	}
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

	log.Printf("Processing request with filter: %s, pubkey: %s, page: %d, limit: %d",
		filterType, pubkey, page, limit)

	var notifications []lib.PaymentNotification
	var metadata *lib.PaginationMetadata
	var fetchErr error

	switch filterType {
	case "unread":
		log.Printf("Fetching unread notifications with page: %d, limit: %d", page, limit)
		notifications, metadata, fetchErr = store.GetStatsStore().GetUnreadPaymentNotifications(page, limit)
		if fetchErr != nil {
			log.Printf("Error fetching unread notifications: %v", fetchErr)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to fetch notifications: " + fetchErr.Error(),
			})
		}
		log.Printf("Raw result from GetUnreadPaymentNotifications: %+v", notifications)

		// Return 204 No Content status with no body when there are no unread notifications
		if len(notifications) == 0 {
			log.Println("No unread notifications found, returning 204 No Content")
			return c.Status(fiber.StatusNoContent).Send(nil)
		}
		log.Printf("Found %d unread notifications", len(notifications))
	case "user":
		if pubkey == "" {
			log.Println("Missing pubkey parameter for user filter, returning 400 Bad Request")
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Pubkey parameter is required for user filter",
			})
		}
		log.Printf("Fetching notifications for user: %s with page: %d, limit: %d", pubkey, page, limit)
		notifications, metadata, fetchErr = store.GetStatsStore().GetUserPaymentNotifications(pubkey, page, limit)
		if fetchErr != nil {
			log.Printf("Error fetching user notifications: %v", fetchErr)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to fetch notifications: " + fetchErr.Error(),
			})
		}
	default: // "all"
		log.Printf("Fetching all notifications with page: %d, limit: %d", page, limit)
		notifications, metadata, fetchErr = store.GetStatsStore().GetAllPaymentNotifications(page, limit)
		if fetchErr != nil {
			log.Printf("Error fetching all notifications: %v", fetchErr)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to fetch notifications: " + fetchErr.Error(),
			})
		}
		log.Printf("Raw result from GetAllPaymentNotifications: %+v", notifications)

		// Detailed logging of database result
		log.Printf("Result data type: %T", notifications)
		log.Printf("Database response type: %+v", fetchErr)

		// Inspect database connection
		if store.GetStatsStore() == nil {
			log.Printf("ERROR: Statistics store is nil")
		} else {
			log.Printf("Statistics store type: %T", store.GetStatsStore())
		}
	}

	// Log detailed notification information
	if len(notifications) > 0 {
		log.Printf("Retrieved %d notifications", len(notifications))
		for i, n := range notifications {
			log.Printf("Notification %d: ID=%d, PubKey=%s, TxID=%s, Amount=%d, IsRead=%v",
				i+1, n.ID, n.PubKey, n.TxID, n.Amount, n.IsRead)
		}
	} else {
		log.Println("No notifications found")
	}

	if metadata != nil {
		log.Printf("Pagination: TotalItems=%d, TotalPages=%d, CurrentPage=%d, PageSize=%d",
			metadata.TotalItems, metadata.TotalPages, metadata.CurrentPage, metadata.PageSize)
	}

	responseData := fiber.Map{
		"notifications": notifications,
		"pagination":    metadata,
	}

	log.Printf("Sending response with %d notifications", len(notifications))
	log.Printf("Response data: %+v", responseData)

	resp := c.JSON(responseData)
	log.Printf("Response status: %d", c.Response().StatusCode())
	return resp
}

// markPaymentNotificationAsRead marks a payment notification as read
func markPaymentNotificationAsRead(c *fiber.Ctx, store stores.Store) error {
	// Get notification ID from request body
	var req struct {
		ID uint `json:"id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	log.Printf("Payment notification %v has been read", req.ID)

	if req.ID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Notification ID is required",
		})
	}

	// Mark notification as read
	if err := store.GetStatsStore().MarkPaymentNotificationAsRead(req.ID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to mark notification as read: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Notification marked as read",
	})
}

// markAllPaymentNotificationsAsRead marks all payment notifications as read for a user
func markAllPaymentNotificationsAsRead(c *fiber.Ctx, store stores.Store) error {
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
	if err := store.GetStatsStore().MarkAllPaymentNotificationsAsRead(req.Pubkey); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to mark notifications as read: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "All notifications marked as read",
	})
}

// getPaymentStats gets statistics about payments and subscriptions
func getPaymentStats(c *fiber.Ctx, store stores.Store) error {
	// Get payment statistics
	stats, err := store.GetStatsStore().GetPaymentStats()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch payment statistics: " + err.Error(),
		})
	}

	return c.JSON(stats)
}

// createPaymentNotification creates a new payment notification (for testing only)
func createPaymentNotification(c *fiber.Ctx, store stores.Store) error {
	// This handler should only be used for testing
	var notification lib.PaymentNotification

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
	if notification.PubKey == "" || notification.TxID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Pubkey and TxID are required",
		})
	}

	// Create the notification
	log.Printf("Creating payment notification: %+v", notification)

	if err := store.GetStatsStore().CreatePaymentNotification(&notification); err != nil {
		log.Printf("ERROR creating notification: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create notification: " + err.Error(),
		})
	}

	log.Printf("Successfully created payment notification with ID: %d", notification.ID)

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Payment notification created successfully",
		"id":      notification.ID,
	})
}
