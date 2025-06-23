package access

import (
	"fmt"
	"log"
	"strings"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/access"
	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
)

// GetAllowedReadNpubs handles GET requests for paginated allowed read NPUBs
func GetAllowedReadNpubs(c *fiber.Ctx, store stores.Store) error {
	// Get pagination parameters
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("pageSize", 20)

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Get NPUBs from statistics store
	statsStore := store.GetStatsStore()
	if statsStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Statistics store not available",
		})
	}

	npubs, metadata, err := statsStore.GetAllowedReadNpubs(page, pageSize)
	if err != nil {
		log.Printf("Error getting allowed read NPUBs: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to retrieve allowed read NPUBs",
		})
	}

	return c.JSON(fiber.Map{
		"npubs":      npubs,
		"pagination": metadata,
	})
}

// GetAllowedWriteNpubs handles GET requests for paginated allowed write NPUBs
func GetAllowedWriteNpubs(c *fiber.Ctx, store stores.Store) error {
	// Get pagination parameters
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("pageSize", 20)

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Get NPUBs from statistics store
	statsStore := store.GetStatsStore()
	if statsStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Statistics store not available",
		})
	}

	npubs, metadata, err := statsStore.GetAllowedWriteNpubs(page, pageSize)
	if err != nil {
		log.Printf("Error getting allowed write NPUBs: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to retrieve allowed write NPUBs",
		})
	}

	return c.JSON(fiber.Map{
		"npubs":      npubs,
		"pagination": metadata,
	})
}

// AddAllowedReadNpub handles POST requests to add an NPUB to the read list
func AddAllowedReadNpub(c *fiber.Ctx, store stores.Store) error {
	var req struct {
		Npub string `json:"npub"`
		Tier string `json:"tier"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate NPUB format
	if !strings.HasPrefix(req.Npub, "npub") || len(req.Npub) < 10 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid NPUB format",
		})
	}

	// Get access control
	accessControl := getAccessControlInstance(store)
	if accessControl == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Access control not initialized",
		})
	}

	// Get current user from context
	addedBy := c.Get("userPubkey", "admin")

	// Add NPUB to read list
	if err := accessControl.AddNpubToReadList(req.Npub, req.Tier, addedBy); err != nil {
		log.Printf("Error adding NPUB to read list: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to add NPUB to read list",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "NPUB added to read list successfully",
	})
}

// AddAllowedWriteNpub handles POST requests to add an NPUB to the write list
func AddAllowedWriteNpub(c *fiber.Ctx, store stores.Store) error {
	var req struct {
		Npub string `json:"npub"`
		Tier string `json:"tier"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate NPUB format
	if !strings.HasPrefix(req.Npub, "npub") || len(req.Npub) < 10 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid NPUB format",
		})
	}

	// Get access control
	accessControl := getAccessControlInstance(store)
	if accessControl == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Access control not initialized",
		})
	}

	// Get current user from context
	addedBy := c.Get("userPubkey", "admin")

	// Add NPUB to write list
	if err := accessControl.AddNpubToWriteList(req.Npub, req.Tier, addedBy); err != nil {
		log.Printf("Error adding NPUB to write list: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to add NPUB to write list",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "NPUB added to write list successfully",
	})
}

// RemoveAllowedReadNpub handles DELETE requests to remove an NPUB from the read list
func RemoveAllowedReadNpub(c *fiber.Ctx, store stores.Store) error {
	npub := c.Params("npub")

	// Validate NPUB format
	if !strings.HasPrefix(npub, "npub") || len(npub) < 10 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid NPUB format",
		})
	}

	// Get access control
	accessControl := getAccessControlInstance(store)
	if accessControl == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Access control not initialized",
		})
	}

	// Remove NPUB from read list
	if err := accessControl.RemoveNpubFromReadList(npub); err != nil {
		log.Printf("Error removing NPUB from read list: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to remove NPUB from read list",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "NPUB removed from read list successfully",
	})
}

// RemoveAllowedWriteNpub handles DELETE requests to remove an NPUB from the write list
func RemoveAllowedWriteNpub(c *fiber.Ctx, store stores.Store) error {
	npub := c.Params("npub")

	// Validate NPUB format
	if !strings.HasPrefix(npub, "npub") || len(npub) < 10 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid NPUB format",
		})
	}

	// Get access control
	accessControl := getAccessControlInstance(store)
	if accessControl == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Access control not initialized",
		})
	}

	// Remove NPUB from write list
	if err := accessControl.RemoveNpubFromWriteList(npub); err != nil {
		log.Printf("Error removing NPUB from write list: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to remove NPUB from write list",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "NPUB removed from write list successfully",
	})
}

// BulkImportNpubs handles POST requests to bulk import NPUBs
func BulkImportNpubs(c *fiber.Ctx, store stores.Store) error {
	var req struct {
		Type  string   `json:"type"`  // "read" or "write"
		Npubs []string `json:"npubs"` // Format: ["npub1:tier", "npub2:tier", ...]
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate type
	if req.Type != "read" && req.Type != "write" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Type must be 'read' or 'write'",
		})
	}

	// Get access control
	accessControl := getAccessControlInstance(store)
	if accessControl == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Access control not initialized",
		})
	}

	// Get current user from context
	addedBy := c.Get("userPubkey", "admin")

	// Parse NPUBs and prepare for bulk import
	if req.Type == "read" {
		var npubList []types.AllowedReadNpub
		for _, entry := range req.Npubs {
			parts := strings.SplitN(entry, ":", 2)
			npub := parts[0]
			tier := ""
			if len(parts) > 1 {
				tier = parts[1]
			}

			// Validate NPUB format
			if !strings.HasPrefix(npub, "npub") || len(npub) < 10 {
				continue // Skip invalid entries
			}

			npubList = append(npubList, types.AllowedReadNpub{
				Npub:     npub,
				TierName: tier,
				AddedBy:  addedBy,
				AddedAt:  time.Now(),
			})
		}

		if err := accessControl.BulkImportReadNpubs(npubList); err != nil {
			log.Printf("Error bulk importing read NPUBs: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to bulk import read NPUBs",
			})
		}

		return c.JSON(fiber.Map{
			"success":  true,
			"message":  fmt.Sprintf("Successfully imported %d read NPUBs", len(npubList)),
			"imported": len(npubList),
		})
	} else {
		var npubList []types.AllowedWriteNpub
		for _, entry := range req.Npubs {
			parts := strings.SplitN(entry, ":", 2)
			npub := parts[0]
			tier := ""
			if len(parts) > 1 {
				tier = parts[1]
			}

			// Validate NPUB format
			if !strings.HasPrefix(npub, "npub") || len(npub) < 10 {
				continue // Skip invalid entries
			}

			npubList = append(npubList, types.AllowedWriteNpub{
				Npub:     npub,
				TierName: tier,
				AddedBy:  addedBy,
				AddedAt:  time.Now(),
			})
		}

		if err := accessControl.BulkImportWriteNpubs(npubList); err != nil {
			log.Printf("Error bulk importing write NPUBs: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to bulk import write NPUBs",
			})
		}

		return c.JSON(fiber.Map{
			"success":  true,
			"message":  fmt.Sprintf("Successfully imported %d write NPUBs", len(npubList)),
			"imported": len(npubList),
		})
	}
}

// Helper function to get access control instance
func getAccessControlInstance(store stores.Store) *access.AccessControl {
	// Load allowed users settings
	settings, err := config.GetConfig()
	if err != nil {
		log.Printf("Error getting config: %v", err)
		return nil
	}

	// Get statistics store
	statsStore := store.GetStatsStore()
	if statsStore == nil {
		log.Printf("Statistics store not available")
		return nil
	}

	// Create access control instance
	return access.NewAccessControl(statsStore, &settings.AllowedUsersSettings)
}
