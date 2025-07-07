package handlers

import (
	"strconv"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
)

// getPaymentNotifications retrieves payment notifications with pagination
func GetPaymentNotifications(c *fiber.Ctx, store stores.Store) error {
	logging.Info("Payment notification request made")

	// Log detailed request information
	logging.Infof("Request details - URL: %s, Method: %s", c.OriginalURL(), c.Method())
	logging.Infof("Query parameters - page: %s, limit: %s, filter: %s, pubkey: %s",
		c.Query("page", "1"),
		c.Query("limit", "10"),
		c.Query("filter", "all"),
		c.Query("pubkey", ""))

	// Log request body if present
	if len(c.Body()) > 0 {
		logging.Infof("Request body: %s", string(c.Body()))
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

	logging.Infof("Processing request with filter: %s, pubkey: %s, page: %d, limit: %d",
		filterType, pubkey, page, limit)

	var notifications []lib.PaymentNotification
	var metadata *lib.PaginationMetadata
	var fetchErr error

	switch filterType {
	case "unread":
		logging.Infof("Fetching unread notifications with page: %d, limit: %d", page, limit)
		notifications, metadata, fetchErr = store.GetStatsStore().GetUnreadPaymentNotifications(page, limit)
		if fetchErr != nil {
			logging.Infof("Error fetching unread notifications: %v", fetchErr)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to fetch notifications: " + fetchErr.Error(),
			})
		}
		logging.Infof("Raw result from GetUnreadPaymentNotifications: %+v", notifications)

		// Return 204 No Content status with no body when there are no unread notifications
		if len(notifications) == 0 {
			logging.Info("No unread notifications found, returning 204 No Content")
			return c.Status(fiber.StatusNoContent).Send(nil)
		}
		logging.Infof("Found %d unread notifications", len(notifications))
	case "user":
		if pubkey == "" {
			logging.Info("Missing pubkey parameter for user filter, returning 400 Bad Request")
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Pubkey parameter is required for user filter",
			})
		}
		logging.Infof("Fetching notifications for user: %s with page: %d, limit: %d", pubkey, page, limit)
		notifications, metadata, fetchErr = store.GetStatsStore().GetUserPaymentNotifications(pubkey, page, limit)
		if fetchErr != nil {
			logging.Infof("Error fetching user notifications: %v", fetchErr)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to fetch notifications: " + fetchErr.Error(),
			})
		}
	default: // "all"
		logging.Infof("Fetching all notifications with page: %d, limit: %d", page, limit)
		notifications, metadata, fetchErr = store.GetStatsStore().GetAllPaymentNotifications(page, limit)
		if fetchErr != nil {
			logging.Infof("Error fetching all notifications: %v", fetchErr)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to fetch notifications: " + fetchErr.Error(),
			})
		}
		logging.Infof("Raw result from GetAllPaymentNotifications: %+v", notifications)

		// Detailed logging of database result
		logging.Infof("Result data type: %T", notifications)
		logging.Infof("Database response type: %+v", fetchErr)

		// Inspect database connection
		if store.GetStatsStore() == nil {
			logging.Infof("ERROR: Statistics store is nil")
		} else {
			logging.Infof("Statistics store type: %T", store.GetStatsStore())
		}
	}

	// Log detailed notification information
	if len(notifications) > 0 {
		logging.Infof("Retrieved %d notifications", len(notifications))
		for i, n := range notifications {
			logging.Infof("Notification %d: ID=%d, PubKey=%s, TxID=%s, Amount=%d, IsRead=%v",
				i+1, n.ID, n.PubKey, n.TxID, n.Amount, n.IsRead)
		}
	} else {
		logging.Info("No notifications found")
	}

	if metadata != nil {
		logging.Infof("Pagination: TotalItems=%d, TotalPages=%d, CurrentPage=%d, PageSize=%d",
			metadata.TotalItems, metadata.TotalPages, metadata.CurrentPage, metadata.PageSize)
	}

	responseData := fiber.Map{
		"notifications": notifications,
		"pagination":    metadata,
	}

	logging.Infof("Sending response with %d notifications", len(notifications))
	logging.Infof("Response data: %+v", responseData)

	resp := c.JSON(responseData)
	logging.Infof("Response status: %d", c.Response().StatusCode())
	return resp
}

// markPaymentNotificationAsRead marks a payment notification as read
func MarkPaymentNotificationAsRead(c *fiber.Ctx, store stores.Store) error {
	// Get notification ID from request body
	var req struct {
		ID uint `json:"id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	logging.Infof("Payment notification %v has been read", req.ID)

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
func MarkAllPaymentNotificationsAsRead(c *fiber.Ctx, store stores.Store) error {
	// Get user pubkey from request body if provided (for logging purposes)
	var req struct {
		Pubkey string `json:"pubkey"`
	}

	if err := c.BodyParser(&req); err != nil {
		logging.Infof("ERROR: Failed to parse request body: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	logging.Infof("Received mark all as read from: %s", req.Pubkey)

	// Check if store is available
	if store == nil {
		logging.Infof("ERROR: Store is nil")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Store not available",
		})
	}

	// Check if stats store is available
	statsStore := store.GetStatsStore()
	if statsStore == nil {
		logging.Infof("ERROR: Stats store is nil")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Stats store not available",
		})
	}

	logging.Infof("About to call MarkAllPaymentNotificationsAsRead")

	// Mark all notifications as read globally
	if err := statsStore.MarkAllPaymentNotificationsAsRead(); err != nil {
		logging.Infof("ERROR: Failed to mark notifications as read: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to mark notifications as read: " + err.Error(),
		})
	}

	logging.Infof("Successfully marked all notifications as read")

	return c.JSON(fiber.Map{
		"success": true,
		"message": "All notifications marked as read",
	})
}

// getPaymentStats gets statistics about payments and subscriptions
func GetPaymentStats(c *fiber.Ctx, store stores.Store) error {
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
func CreatePaymentNotification(c *fiber.Ctx, store stores.Store) error {
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
	logging.Infof("Creating payment notification: %+v", notification)

	if err := store.GetStatsStore().CreatePaymentNotification(&notification); err != nil {
		logging.Infof("ERROR creating notification: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create notification: " + err.Error(),
		})
	}

	logging.Infof("Successfully created payment notification with ID: %d", notification.ID)

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Payment notification created successfully",
		"id":      notification.ID,
	})
}
