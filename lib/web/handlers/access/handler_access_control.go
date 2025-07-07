package access

import (
	"strings"

	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/subscription"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
)

func GetAllowedUsersPaginated(c *fiber.Ctx, store stores.Store) error {
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("pageSize", 20)

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	statsStore := store.GetStatsStore()
	if statsStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Statistics store not available",
		})
	}

	allowedUsers, metadata, err := statsStore.GetUsersPaginated(page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to retrieve allowed users",
		})
	}

	return c.JSON(fiber.Map{
		"allowed_users": allowedUsers,
		"pagination":    metadata,
	})
}

func AddAllowedUser(c *fiber.Ctx, store stores.Store) error {
	var req struct {
		Npub string `json:"npub"`
		Tier string `json:"tier"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	pubKey, err := signing.DeserializePublicKey(req.Npub)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid public key format",
		})
	}

	createdBy := c.Get("userPubkey", "admin")

	serializedPubKey, err := signing.SerializePublicKey(pubKey)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Failed to sertialize public key",
		})
	}

	statsStore := store.GetStatsStore()
	if statsStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Statistics store not available",
		})
	}

	if err := statsStore.AddAllowedUser(*serializedPubKey, req.Tier, createdBy); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to add NPUB to read list",
		})
	}

	// Update the user's kind 11888 event if we're in invite-only mode
	// This ensures their storage allocation reflects their new tier immediately
	go func() {
		var allowedUsersSettings struct {
			Mode string `json:"mode"`
		}
		if err := viper.UnmarshalKey("allowed_users", &allowedUsersSettings); err != nil {
			logging.Infof("Warning: Could not load allowed_users settings to check mode: %v", err)
			return
		}

		currentMode := strings.ToLower(allowedUsersSettings.Mode)
		logging.Infof("User %s added with tier %s in mode %s", req.Npub, req.Tier, currentMode)

		// Only update subscription events in invite-only mode
		// In other modes, the batch update or initialization handles it
		if currentMode == "invite-only" && req.Tier != "" {
			manager := subscription.GetGlobalManager()
			if manager != nil {
				logging.Infof("Updating kind 11888 event for newly added user %s (will lookup tier from database)", req.Npub)
				// Use the new function that follows the correct flow: DB lookup -> config lookup -> update event
				if err := manager.UpdateUserSubscriptionFromDatabase(req.Npub); err != nil {
					logging.Infof("Error updating subscription event for %s: %v", req.Npub, err)
				} else {
					logging.Infof("Successfully updated subscription event for %s", req.Npub)
				}
			} else {
				logging.Infof("Warning: Global subscription manager not available")
			}
		}
	}()

	return c.JSON(fiber.Map{
		"success": true,
		"message": "NPUB added to read list successfully",
	})
}

func RemoveAllowedUser(c *fiber.Ctx, store stores.Store) error {
	var req struct {
		Npub string `json:"npub"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	pubKey, err := signing.DeserializePublicKey(req.Npub)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid public key format",
		})
	}

	serializedPubKey, err := signing.SerializePublicKey(pubKey)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Failed to sertialize public key",
		})
	}

	statsStore := store.GetStatsStore()
	if statsStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Statistics store not available",
		})
	}

	if err := statsStore.RemoveAllowedUser(*serializedPubKey); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to remove user",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "User added to allowed users successfully",
	})
}

// RelayOwner management handlers

func GetRelayOwner(c *fiber.Ctx, store stores.Store) error {
	statsStore := store.GetStatsStore()
	if statsStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Statistics store not available",
		})
	}

	owner, err := statsStore.GetRelayOwner()
	if err != nil {
		// Return null if no owner is set (not an error)
		return c.JSON(fiber.Map{
			"relay_owner": nil,
		})
	}

	return c.JSON(fiber.Map{
		"relay_owner": owner,
	})
}

func SetRelayOwner(c *fiber.Ctx, store stores.Store) error {
	var req struct {
		Npub string `json:"npub"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	pubKey, err := signing.DeserializePublicKey(req.Npub)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid public key format",
		})
	}

	createdBy := c.Get("userPubkey", "admin")

	serializedPubKey, err := signing.SerializePublicKey(pubKey)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Failed to serialize public key",
		})
	}

	statsStore := store.GetStatsStore()
	if statsStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Statistics store not available",
		})
	}

	if err := statsStore.SetRelayOwner(*serializedPubKey, createdBy); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to set relay owner",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Relay owner set successfully",
	})
}

func RemoveRelayOwner(c *fiber.Ctx, store stores.Store) error {
	statsStore := store.GetStatsStore()
	if statsStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Statistics store not available",
		})
	}

	if err := statsStore.RemoveRelayOwner(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to remove relay owner",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Relay owner removed successfully",
	})
}
