package blossom

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/HORNET-Storage/hornet-storage/lib/signing"
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

	data := c.Body()

	mtype := mimetype.Detect(data)

	checkHash := sha256.Sum256(data)
	encodedHash := hex.EncodeToString(checkHash[:])

	// Ensure the public key is valid
	_, err := signing.DeserializePublicKey(pubkey)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid public key"})
	}

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

	var event *nostr.Event
	if len(events) <= 0 {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "no events match this file upload"})
	}

	event = events[0]

	fileHash := event.Tags.GetFirst([]string{"blossom_hash"})
	if fileHash == nil {
		// This is theoretically impossible but rather have it than not
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "matching event blossom_hash mismatch"})
	}

	// Check the submitted hash matches the data being submitted
	if encodedHash != fileHash.Value() {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "submitted hex encoded hash does not match hex encoded hash of data"})
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

	return c.SendStatus(fiber.StatusOK)
}
