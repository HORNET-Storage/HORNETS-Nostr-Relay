package blossom

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
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
	// Routes will be set up in web/server.go with appropriate middleware
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

	filter := nostr.Filter{
		Kinds:   []int{117},
		Authors: []string{pubkey},
		Tags:    nostr.TagMap{"blossom_hash": []string{encodedHash}},
	}

	// Ensure a kind 117 file information event was uploaded first
	events, err := s.storage.QueryEvents(filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to query events"})
	}

	if len(events) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "no matching kind 117 event found",
			"detail":  fmt.Sprintf("Please create a kind 117 event with blossom_hash tag '%s' before uploading", encodedHash),
		})
	}

	event := events[0]

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

	// Store the blob
	err = s.storage.StoreBlob(data, checkHash[:], pubkey)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to store blob"})
	}

	// Extract the file name from the kind 117 file information event
	var name string
	nameTag := event.Tags.GetFirst([]string{"name"})
	if nameTag == nil {
		name = "Unknown"
	} else {
		name = nameTag.Value()
	}

	// Store the file in the statistics database
	s.storage.GetStatsStore().SaveFile("blossom", encodedHash, name, mtype.String(), 0, int64(len(data)))

	// Return success with the hash for confirmation
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "File uploaded successfully",
		"hash":    encodedHash,
		"size":    len(data),
		"type":    mtype.String(),
	})
}
