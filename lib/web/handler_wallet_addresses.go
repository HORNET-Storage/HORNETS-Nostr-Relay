package web

import (
	"log"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	gorm "github.com/HORNET-Storage/hornet-storage/lib/stores/stats_stores"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
)

// Address status constants
const (
	AddressStatusAvailable = "available"
	AddressStatusAllocated = "allocated"
	AddressStatusUsed      = "used"
)

func saveWalletAddresses(c *fiber.Ctx, store *gorm.GormStatisticsStore) error {
	log.Println("Addresses request received")
	var addresses []types.Address

	// Parse the JSON request body
	if err := c.BodyParser(&addresses); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	// Initialize the Graviton store
	gravitonStore := graviton.GravitonStore{} // Assuming this is initialized appropriately elsewhere

	// Get the expected wallet name from the configuration
	expectedWalletName := viper.GetString("wallet_name")
	if expectedWalletName == "" {
		log.Println("No expected wallet name set in configuration.")
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Process each address
	for _, addr := range addresses {
		// Check if the wallet name matches the expected one
		if addr.WalletName != expectedWalletName {
			log.Printf("Address from unknown wallet: %s, skipping.", addr.WalletName)
			continue
		}

		// Check if the address already exists in the SQL database using the store method
		addressExists, err := store.AddressExists(addr.Address)
		if err != nil {
			log.Printf("Error checking if address exists: %v", err)
			continue
		}

		if addressExists {
			log.Printf("Duplicate address found, skipping: %s", addr.Address)
			continue
		}

		// Create a new address in the SQL database using the store method
		newAddress := types.WalletAddress{
			Index:   addr.Index,
			Address: addr.Address,
		}

		if err := store.SaveAddress(&newAddress); err != nil {
			log.Printf("Error saving new address: %v", err)
			continue
		}

		// Add the address to the Graviton store
		gravitonAddress := &types.Address{
			Index:       addr.Index,
			Address:     addr.Address,
			WalletName:  addr.WalletName,
			Status:      AddressStatusAvailable,
			AllocatedAt: nil,
		}

		if err := gravitonStore.SaveAddress(gravitonAddress); err != nil {
			log.Printf("Error saving address to Graviton store: %v", err)
		}
	}

	// Respond with a success message
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Addresses received and processed successfully",
	})
}
