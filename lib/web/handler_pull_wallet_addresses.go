package web

import (
	"log"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	gorm "github.com/HORNET-Storage/hornet-storage/lib/stores/stats_stores"
	"github.com/gofiber/fiber/v2"
)

// Refactored pullWalletAddresses function
func pullWalletAddresses(c *fiber.Ctx, store *gorm.GormStatisticsStore) error {
	log.Println("Get addresses request received")

	// Fetch wallet addresses using the statistics store
	walletAddresses, err := store.FetchWalletAddresses()
	if err != nil {
		log.Printf("Error fetching addresses: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Map the addresses to the response format
	var addresses []types.AddressResponse
	for _, wa := range walletAddresses {
		addresses = append(addresses, types.AddressResponse{
			Index:   wa.Index,
			Address: wa.Address,
		})
	}

	// Respond with the formatted addresses
	return c.JSON(addresses)
}
