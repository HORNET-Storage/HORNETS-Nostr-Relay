package web

import (
	"log"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

// Address status constants
const (
	AddressStatusAvailable = "available"
	AddressStatusAllocated = "allocated"
	AddressStatusUsed      = "used"
)

func handleAddresses(c *fiber.Ctx) error {
	log.Println("Addresses request received")
	var addresses []types.Address

	// Parse the JSON request body
	if err := c.BodyParser(&addresses); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	// Initialize the Gorm database
	db, err := graviton.InitGorm()
	if err != nil {
		log.Printf("Failed to connect to the database: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Initialize the Graviton store
	gravitonStore := graviton.GravitonStore{} // Assuming this is initialized appropriately elsewhere

	// Load the snapshot and tree from the Graviton store
	err = gravitonStore.InitStore()
	if err != nil {
		log.Printf("Failed to load store: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

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

// package web

// import (
// 	"log"

// 	types "github.com/HORNET-Storage/hornet-storage/lib"
// 	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
// 	"github.com/gofiber/fiber/v2"
// 	"github.com/spf13/viper"
// 	"gorm.io/gorm"
// )

// // Address represents the structure of the address data
// type Address struct {
// 	Index      string `json:"index"`
// 	Address    string `json:"address"`
// 	WalletName string `json:"wallet_name"` // Add wallet name to the Address struct
// }

// func handleAddresses(c *fiber.Ctx) error {
// 	log.Println("Addresses request received")
// 	var addresses []Address

// 	// Parse the JSON request body
// 	if err := c.BodyParser(&addresses); err != nil {
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
// 			"error": "Cannot parse JSON",
// 		})
// 	}

// 	// Initialize the Gorm database
// 	db, err := graviton.InitGorm()
// 	if err != nil {
// 		log.Printf("Failed to connect to the database: %v", err)
// 		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
// 	}

// 	// Get the expected wallet name from the configuration
// 	expectedWalletName := viper.GetString("wallet_name")

// 	if expectedWalletName == "" {
// 		log.Println("No expected wallet name set in configuration.")
// 		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
// 	}

// 	// Process each address
// 	for _, addr := range addresses {
// 		// Check if the wallet name matches the expected one
// 		if addr.WalletName != expectedWalletName {
// 			log.Printf("Address from unknown wallet: %s, skipping.", addr.WalletName)
// 			continue
// 		}

// 		var existingAddress types.WalletAddress
// 		result := db.Where("address = ?", addr.Address).First(&existingAddress)

// 		if result.Error == nil {
// 			// Address already exists, skip it
// 			log.Printf("Duplicate address found, skipping: %s", addr.Address)
// 			continue
// 		}

// 		if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
// 			log.Printf("Error querying address: %v", result.Error)
// 			continue
// 		}

// 		// Create a new address
// 		newAddress := types.WalletAddress{
// 			Index:   addr.Index,
// 			Address: addr.Address,
// 		}
// 		if err := db.Create(&newAddress).Error; err != nil {
// 			log.Printf("Error saving new address: %v", err)
// 			continue
// 		}
// 	}

// 	// Respond with a success message
// 	return c.JSON(fiber.Map{
// 		"status":  "success",
// 		"message": "Addresses received and processed successfully",
// 	})
// }
