package blossom

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"

	utils "github.com/HORNET-Storage/hornet-storage/lib/handlers/scionic"
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

	// Validate subscription status and storage quota using NIP-88
	if err := utils.ValidateUploadEligibility(s.storage, pubkey, c.Body()); err != nil {
		log.Printf("Upload validation failed for subscriber %s: %v", pubkey, err)
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	data := c.Body()

	// Compute the hash of the data
	checkHash := sha256.Sum256(data)
	encodedHash := hex.EncodeToString(checkHash[:])

	// Filter to find matching events with the computed hash
	filter := nostr.Filter{
		Kinds:   []int{117}, // Assuming 117 is the correct kind for blossom events
		Authors: []string{pubkey},
		Tags:    nostr.TagMap{"blossom_hash": []string{encodedHash}},
	}

	fmt.Println("Received a blossom blob")

	// Query for events matching the filter
	events, err := s.storage.QueryEvents(filter)
	if err != nil {
		return err
	}

	// Handle case where no matching events are found
	if len(events) <= 0 {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "no events match this file upload"})
	}

	event := events[0]

	// Extract the "blossom_hash" tag value from the event
	fileHash := event.Tags.GetFirst([]string{"blossom_hash"})

	// Check if the submitted hash matches the expected value from the event
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
