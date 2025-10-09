package blossom

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/subscription"
	"github.com/HORNET-Storage/hornet-storage/lib/web/middleware"
	"github.com/gabriel-vasile/mimetype"
	"github.com/gofiber/fiber/v2"
	"github.com/nbd-wtf/go-nostr"
)

type Server struct {
	storage stores.Store
}

func NewServer(store stores.Store) *Server {
	return &Server{storage: store}
}

func (s *Server) SetupRoutes(app *fiber.App) {
	// Public endpoint - no auth required for downloads
	app.Get("/blossom/:hash", s.GetBlobHandler())

	// Protected endpoint - requires NIP-98 auth for uploads
	app.Put("/blossom/upload", middleware.NIP98Middleware(), s.UploadBlobHandler())

	// logging.Info("Blossom file storage routes initialized with NIP-98 authentication")
}

// GetBlobHandler returns the handler for getting blobs
func (s *Server) GetBlobHandler() fiber.Handler {
	return s.getBlob
}

// UploadBlobHandler returns the handler for uploading blobs
func (s *Server) UploadBlobHandler() fiber.Handler {
	return s.uploadBlob
}

func (s *Server) getBlob(c *fiber.Ctx) error {
	hash := c.Params("hash")
	data, err := s.storage.GetBlob(hash)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Blob not found"})
	}

	// Set appropriate content type if possible
	mtype := mimetype.Detect(data)
	c.Set("Content-Type", mtype.String())

	return c.Send(data)
}

func (s *Server) uploadBlob(c *fiber.Ctx) error {
	// Get authenticated pubkey from NIP-98 middleware context
	pubkey, ok := c.Locals("nip98_pubkey").(string)
	if !ok || pubkey == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "authentication required"})
	}

	data := c.Body()
	if len(data) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "empty request body"})
	}

	mtype := mimetype.Detect(data)

	checkHash := sha256.Sum256(data)
	encodedHash := hex.EncodeToString(checkHash[:])

	// WORKAROUND: Use broad search instead of tag filtering due to relay tag indexing issues
	// The relay's tag indexing system doesn't work for blossom_hash tags, so we search broadly
	// and manually filter the results
	broadFilter := nostr.Filter{
		Kinds:   []int{117},
		Authors: []string{pubkey},
	}

	logging.Infof("Blossom upload: Searching for kind 117 events (broad search) - Author: %s, Hash: %s", pubkey, encodedHash)

	// Get all kind 117 events from this author
	allEvents, err := s.storage.QueryEvents(broadFilter)
	if err != nil {
		logging.Infof("Blossom upload: Error querying events: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to query events"})
	}

	logging.Infof("Blossom upload: Found %d total kind 117 events from author, filtering for hash %s", len(allEvents), encodedHash)

	// Manually filter for events with matching blossom_hash
	var matchingEvents []*nostr.Event
	for _, event := range allEvents {
		for _, tag := range event.Tags {
			if len(tag) >= 2 && tag[0] == "blossom_hash" && tag[1] == encodedHash {
				matchingEvents = append(matchingEvents, event)
				logging.Infof("Blossom upload: Found matching event - ID: %s, Hash: %s", event.ID, tag[1])
				break
			}
		}
	}

	// Determine file name based on whether Kind 117 event exists
	var name string

	if len(matchingEvents) == 0 {
		// No Kind 117 event found - proceed with upload anyway
		logging.Infof("Blossom upload: No kind 117 event found for hash %s from author %s, proceeding without metadata", encodedHash, pubkey)
		name = encodedHash // Use hash as filename when no Kind 117 event exists
	} else {
		// Kind 117 event found - validate and extract metadata
		event := matchingEvents[0]

		fileHash := event.Tags.GetFirst([]string{"blossom_hash"})
		if fileHash == nil {
			// This is theoretically impossible but rather have it than not
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "matching event blossom_hash mismatch"})
		}

		// Check the submitted hash matches the data being submitted
		if encodedHash != fileHash.Value() {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "hash mismatch",
				"detail":  fmt.Sprintf("calculated hash %s does not match event hash %s", encodedHash, fileHash.Value()),
			})
		}

		// Extract the file name from the kind 117 file information event
		nameTag := event.Tags.GetFirst([]string{"name"})
		if nameTag == nil {
			name = encodedHash // Fallback to hash if no name tag
		} else {
			name = nameTag.Value()
		}

		logging.Infof("Blossom upload: Using metadata from kind 117 event - Name: %s, Hash: %s", name, encodedHash)
	}

	// Store the blob
	err = s.storage.StoreBlob(data, checkHash[:], pubkey)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to store blob"})
	}

	// Store the file in the statistics database
	s.storage.GetStatsStore().SaveFile("blossom", encodedHash, name, mtype.String(), 0, int64(len(data)))

	// Update subscription storage usage for the file upload asynchronously
	go func(pk string, size int64) {
		subManager := subscription.GetGlobalManager()
		if subManager != nil {
			if err := subManager.UpdateStorageUsage(pk, size); err != nil {
				logging.Infof("Warning: Failed to update storage usage for pubkey %s: %v", pk, err)
			}
		} else {
			logging.Infof("Warning: Global subscription manager not available, storage not tracked for pubkey %s", pk)
		}
	}(pubkey, int64(len(data)))

	// Return success with the hash for confirmation
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "File uploaded successfully",
		"hash":    encodedHash,
		"size":    len(data),
		"type":    mtype.String(),
	})
}
