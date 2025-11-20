package blossom

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/HORNET-Storage/hornet-storage/lib/handlers/scionic"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/subscription"
	"github.com/HORNET-Storage/hornet-storage/lib/web/middleware"
	"github.com/gabriel-vasile/mimetype"
	"github.com/gofiber/fiber/v2"
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

	// Check if the MIME type is allowed by relay configuration
	if !scionic.IsMimeTypePermitted(mtype.String()) {
		logging.Infof("Blossom upload rejected: MIME type not permitted - Author: %s, Type: %s", pubkey, mtype.String())
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"message": "MIME type is not allowed to be stored by this relay (" + mtype.String() + ")",
		})
	}

	checkHash := sha256.Sum256(data)
	encodedHash := hex.EncodeToString(checkHash[:])

	// Use hash as filename for content-addressed storage
	name := encodedHash

	logging.Infof("Blossom upload: Storing blob - Author: %s, Hash: %s", pubkey, encodedHash)

	// Store the blob
	err := s.storage.StoreBlob(data, checkHash[:], pubkey)
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
