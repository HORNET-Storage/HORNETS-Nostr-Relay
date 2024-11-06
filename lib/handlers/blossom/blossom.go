package blossom

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
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
	app.Get("/blossom/:hash", s.getBlob)
	app.Put("/blossom/upload", s.uploadBlob)
}

func (s *Server) getBlob(c *fiber.Ctx) error {
	hash := c.Params("hash")
	data, err := s.storage.GetBlob(hash)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Blob not found"})
	}
	return c.Send(data)
}

func (s *Server) uploadBlob(c *fiber.Ctx) error {
	pubkey := c.Query("pubkey")

	subscriber, err := s.storage.GetSubscriber(pubkey)
	if err != nil {
		return err
	}

	// Validate subscription status and storage quota
	if err := validateUploadEligibility(s.storage, subscriber, c.Body()); err != nil {
		log.Printf("Upload validation failed for subscriber %s: %v", pubkey, err)
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	data := c.Body()

	checkHash := sha256.Sum256(data)
	encodedHash := hex.EncodeToString(checkHash[:])

	filter := nostr.Filter{
		Kinds:   []int{117},
		Authors: []string{pubkey},
		Tags:    nostr.TagMap{"blossom_hash": []string{encodedHash}},
	}

	fmt.Println("Recieved a blossom blob")

	events, err := s.storage.QueryEvents(filter)
	if err != nil {
		return err
	}

	var event *nostr.Event
	if len(events) <= 0 {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "no events match this file upload"})
	}

	event = events[0]

	fileHash := event.Tags.GetFirst([]string{"blossom_hash"})

	// Check the submitted hash matches the data being submitted
	if encodedHash != fileHash.Value() {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "submitted hex encoded hash does not match hex encoded hash of data"})
	}

	// Store the blob
	err = s.storage.StoreBlob(data, checkHash[:], pubkey)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to store blob"})
	}

	fmt.Println("Finished a blossom blob")

	return c.SendStatus(fiber.StatusOK)
}

// validateUploadEligibility checks if the subscriber can upload the file
func validateUploadEligibility(store stores.Store, subscriber *types.Subscriber, data []byte) error {
	// Check subscription expiration
	if time.Now().After(subscriber.EndDate) {
		return fmt.Errorf("subscription expired on %s", subscriber.EndDate.Format(time.RFC3339))
	}

	// Try to use subscriber store features if available
	subscriberStore, ok := store.(stores.SubscriberStore)
	if !ok {
		// Fallback to basic validation if subscriber store is not available
		return nil
	}

	// Check storage quota
	fileSize := int64(len(data))
	if err := subscriberStore.CheckStorageAvailability(subscriber.Npub, fileSize); err != nil {
		// Get current usage for detailed error message
		stats, statsErr := subscriberStore.GetSubscriberStorageStats(subscriber.Npub)
		if statsErr != nil {
			return fmt.Errorf("storage quota exceeded")
		}

		return fmt.Errorf("storage quota exceeded: used %d of %d bytes (%.2f%%), attempting to upload %d bytes",
			stats.CurrentUsageBytes,
			stats.StorageLimitBytes,
			stats.UsagePercentage,
			fileSize)
	}

	return nil
}
