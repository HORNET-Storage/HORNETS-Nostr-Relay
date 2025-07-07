package handlers

import (
	"fmt"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
)

// BlockedPubkeyRequest represents a request to block a pubkey
type BlockedPubkeyRequest struct {
	Pubkey string `json:"pubkey"`
	Reason string `json:"reason"`
}

// BlockedPubkeyResponse represents a blocked pubkey in API responses
type BlockedPubkeyResponse struct {
	Pubkey    string    `json:"pubkey"`
	Reason    string    `json:"reason"`
	BlockedAt time.Time `json:"blocked_at"`
}

// getBlockedPubkeys returns a list of all blocked pubkeys
func GetBlockedPubkeys(c *fiber.Ctx, store stores.Store) error {
	blockedPubkeys, err := store.ListBlockedPubkeys()
	if err != nil {
		logging.Infof("Error getting blocked pubkeys: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to retrieve blocked pubkeys: %v", err),
		})
	}

	// Convert to response format
	response := make([]BlockedPubkeyResponse, len(blockedPubkeys))
	for i, pubkey := range blockedPubkeys {
		response[i] = BlockedPubkeyResponse{
			Pubkey:    pubkey.Pubkey,
			Reason:    pubkey.Reason,
			BlockedAt: pubkey.BlockedAt,
		}
	}

	return c.JSON(fiber.Map{
		"blocked_pubkeys": response,
		"count":           len(response),
	})
}

// blockPubkey handles a request to block a pubkey
func BlockPubkey(c *fiber.Ctx, store stores.Store) error {
	var request BlockedPubkeyRequest
	if err := c.BodyParser(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request format",
		})
	}

	if request.Pubkey == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Pubkey is required",
		})
	}

	// Check if already blocked
	isBlocked, _ := store.IsBlockedPubkey(request.Pubkey)
	if isBlocked {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": "Pubkey is already blocked",
		})
	}

	// Default reason if not provided
	if request.Reason == "" {
		request.Reason = "Blocked by admin"
	}

	// Block the pubkey
	err := store.BlockPubkey(request.Pubkey, request.Reason)
	if err != nil {
		logging.Infof("Error blocking pubkey %s: %v", request.Pubkey, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to block pubkey: %v", err),
		})
	}

	logging.Infof("Pubkey %s blocked: %s", request.Pubkey, request.Reason)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"message": fmt.Sprintf("Pubkey %s has been blocked", request.Pubkey),
	})
}

// unblockPubkey handles a request to unblock a pubkey
// This endpoint expects ONLY the pubkey hex string, without any prefix
// Example URL: DELETE /api/blocked-pubkeys/2f844e5ff4913de5d018a3eed1e422e4e8146d10beddcf2ade819ac1e72839f9
func UnblockPubkey(c *fiber.Ctx, store stores.Store) error {
	pubkey := c.Params("pubkey")
	if pubkey == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Pubkey parameter is required",
		})
	}

	// Check if actually blocked
	isBlocked, _ := store.IsBlockedPubkey(pubkey)
	if !isBlocked {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Pubkey is not blocked",
		})
	}

	// Unblock the pubkey
	err := store.UnblockPubkey(pubkey)
	if err != nil {
		logging.Infof("Error unblocking pubkey %s: %v", pubkey, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to unblock pubkey: %v", err),
		})
	}

	logging.Infof("Pubkey %s unblocked", pubkey)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"message": fmt.Sprintf("Pubkey %s has been unblocked", pubkey),
	})
}
