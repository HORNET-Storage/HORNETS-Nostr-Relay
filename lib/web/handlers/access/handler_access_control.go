package access

import (
	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
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
