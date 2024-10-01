package web

import (
	"log"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Address status constants
const (
	AddressStatusAvailable = "available"
	AddressStatusAllocated = "allocated"
	AddressStatusUsed      = "used"
)

func saveWalletAddresses(c *fiber.Ctx) error {
	log.Println("Addresses request received")
	var addresses []types.Address

	// Parse the JSON request body
	if err := c.BodyParser(&addresses); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	// Initialize the Gorm database
	dbPath := viper.GetString("relay_stats_db")
	if dbPath == "" {
		log.Fatal("Database path not found in config")
	}

	// Initialize the Gorm database
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Printf("Failed to connect to the database: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
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

		// Check if the address already exists in the SQL database
		var existingAddress types.WalletAddress
		result := db.Where("address = ?", addr.Address).First(&existingAddress)

		if result.Error == nil {
			// Address already exists, skip it
			log.Printf("Duplicate address found, skipping: %s", addr.Address)
			continue
		}

		if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
			log.Printf("Error querying address: %v", result.Error)
			continue
		}

		// Create a new address in the SQL database
		newAddress := types.WalletAddress{
			Index:   addr.Index,
			Address: addr.Address,
		}
		if err := db.Create(&newAddress).Error; err != nil {
			log.Printf("Error saving new address: %v", err)
			continue
		}

		// Add the address to the Graviton store
		gravitonAddress := &types.Address{
			Index:       addr.Index, // Assuming addr.Index is a string that needs parsing
			Address:     addr.Address,
			WalletName:  addr.WalletName,
			Status:      AddressStatusAvailable, // Set the status to available
			AllocatedAt: nil,
		}

		gravitonStore.SaveAddress(gravitonAddress)
	}

	// Respond with a success message
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Addresses received and processed successfully",
	})
}