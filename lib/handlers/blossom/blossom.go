package blossom

import (
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
)

type Server struct {
	storage stores.Store
}

func NewServer(store stores.Store) *Server {
	return &Server{storage: store}
}

func (s *Server) SetupRoutes(app *fiber.App) {
	app.Get("/blossom/:sha256", s.getBlob)
	app.Head("/blossom/:sha256", s.hasBlob)
	app.Put("/blossom/upload", s.uploadBlob)
	app.Get("/blossom/list/:pubkey", s.listBlobs)
	app.Delete("/blossom/:sha256", s.deleteBlob)
}

func (s *Server) getBlob(c *fiber.Ctx) error {
	sha256 := c.Params("sha256")
	data, contentType, err := s.storage.GetBlob(sha256)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Blob not found"})
	}
	c.Set("Content-Type", *contentType)
	return c.Send(data)
}

func (s *Server) hasBlob(c *fiber.Ctx) error {
	sha256 := c.Params("sha256")
	_, _, err := s.storage.GetBlob(sha256)
	if err != nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	return c.SendStatus(fiber.StatusOK)
}

func (s *Server) uploadBlob(c *fiber.Ctx) error {
	// TODO: Implement authorization check

	// Replace this with the public key from the nostr auth event when nip-42 is implemented
	pubkey := c.Params("pubkey")

	data := c.Body()
	contentType := c.Get("Content-Type")

	descriptor, err := s.storage.StoreBlob(data, contentType, pubkey)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Failed to store blob"})
	}

	return c.JSON(descriptor)
}

func (s *Server) listBlobs(c *fiber.Ctx) error {
	// TODO: Implement authorization check

	pubkey := c.Params("pubkey")
	since := c.QueryInt("since", 0)
	until := c.QueryInt("until", int(time.Now().Unix()))

	blobs, err := s.storage.ListBlobs(pubkey, int64(since), int64(until))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Failed to list blobs"})
	}

	return c.JSON(blobs)
}

func (s *Server) deleteBlob(c *fiber.Ctx) error {
	// TODO: Implement authorization check

	sha256 := c.Params("sha256")
	err := s.storage.DeleteBlob(sha256)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Failed to delete blob"})
	}

	return c.SendStatus(fiber.StatusOK)
}
