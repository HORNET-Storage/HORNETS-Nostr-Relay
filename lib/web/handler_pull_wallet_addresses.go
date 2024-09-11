package web

import (
	"log"

	"github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	"github.com/gofiber/fiber/v2"
)

// AddressResponse represents the format of the address data to be returned
type AddressResponse struct {
	Index   string `json:"index"`
	Address string `json:"address"`
}

func pullWalletAddresses(c *fiber.Ctx) error {
	log.Println("Get addresses request received")

	// Initialize the Gorm database
	db, err := graviton.InitGorm()
	if err != nil {
		log.Printf("Failed to connect to the database: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	var walletAddresses []lib.WalletAddress
	if err := db.Find(&walletAddresses).Error; err != nil {
		log.Printf("Error fetching addresses from database: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Map the addresses to the response format
	var addresses []AddressResponse
	for _, wa := range walletAddresses {
		addresses = append(addresses, AddressResponse{
			Index:   wa.Index,
			Address: wa.Address,
		})
	}

	// Respond with the formatted addresses
	return c.JSON(addresses)
}
