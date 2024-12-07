package web

import (
	"log"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
)

// Refactored pullWalletAddresses function
func pullWalletAddresses(c *fiber.Ctx, store stores.Store) error {
	log.Println("Get addresses request received")

	// Fetch wallet addresses using the statistics store
	walletAddresses, err := store.GetStatsStore().FetchWalletAddresses()
	if err != nil {
		log.Printf("Error fetching addresses: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Map the addresses to the response format
	var addresses []types.AddressResponse
	for _, wa := range walletAddresses {
		addresses = append(addresses, types.AddressResponse{
			IndexHornets: wa.IndexHornets,
			Address:      wa.Address,
		})
	}

	// Respond with the formatted addresses
	return c.JSON(addresses)
}
