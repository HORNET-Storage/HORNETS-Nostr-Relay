package web

import (
	"log"

	types "github.com/HORNET-Storage/hornet-storage/lib"           // Adjust the import path to your actual project structure
	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton" // Adjust the import path to your actual project structure
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Address represents the structure of the address data
type Address struct {
	Index   string `json:"index"`
	Address string `json:"address"`
}

func handleAddresses(c *fiber.Ctx) error {
	log.Println("Addresses request received")
	var addresses []Address

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

	// Process each address
	for _, addr := range addresses {
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

		// Create a new address
		newAddress := types.WalletAddress{
			Index:   addr.Index,
			Address: addr.Address,
		}
		if err := db.Create(&newAddress).Error; err != nil {
			log.Printf("Error saving new address: %v", err)
			continue
		}
	}

	// Respond with a success message
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Addresses received and processed successfully",
	})
}
