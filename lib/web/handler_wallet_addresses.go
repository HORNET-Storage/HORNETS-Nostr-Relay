package web

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
)

// Address status constants
const (
	AddressStatusAvailable = "available"
	AddressStatusAllocated = "allocated"
	AddressStatusUsed      = "used"
)

// saveWalletAddresses processes incoming Bitcoin addresses and stores them for future
// subscriber allocation. These addresses will be used when subscribers initialize their
// subscription and need a payment address.
func saveWalletAddresses(c *fiber.Ctx, store stores.Store) error {
	log.Println("Addresses request received")

	body := c.Body()

	var addresses []types.Address
	if err := json.Unmarshal(body, &addresses); err != nil {
		log.Printf("Error unmarshaling JSON directly: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	expectedWalletName := viper.GetString("wallet_name")
	if expectedWalletName == "" {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Wallet name not configured",
		})
	}

	log.Printf("Expected wallet name: %s", expectedWalletName)

	statsStore := store.GetStatsStore()
	if statsStore == nil {
		log.Println("Error: StatsStore is nil or not initialized")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "StatsStore not available",
		})
	}
	log.Println("Successfully accessed StatsStore")

	log.Println("Successfully accessed SubscriberStore")

	processedCount := 0

	// Process each address from the request
	for _, addr := range addresses {
		if addr.WalletName != expectedWalletName {
			log.Printf("Skipping address from unknown wallet: %s", addr.WalletName)
			continue
		}

		// Check if the address exists in StatsStore and save if it doesn't
		existsInStatsStore, err := statsStore.AddressExists(addr.Address)
		if err != nil {
			log.Printf("Error checking address existence in StatsStore: %v", err)
			continue
		}

		// Save address to StatsStore if it doesn't exist
		if !existsInStatsStore {
			newStatsAddress := types.WalletAddress{
				Index:   addr.Index,
				Address: addr.Address,
			}
			log.Printf("Attempting to save new address to StatsStore: %v", newStatsAddress)
			if err := statsStore.SaveAddress(&newStatsAddress); err != nil {
				log.Printf("Error saving new address to StatsStore: %v", err)
				continue
			}
			log.Printf("Address saved to StatsStore: %v", newStatsAddress)
		}

		// Check if the address exists in SubscriberStore and save if it doesn't
		existsInSubscriberStore, err := store.GetSubscriberStore().AddressExists(addr.Address)
		if err != nil {
			log.Printf("Error checking address existence in SubscriberStore: %v", err)
			continue
		}

		// Save WalletAddress to SubscriberStore if it doesn't exist
		if !existsInSubscriberStore {
			newSubscriberAddress := types.WalletAddress{
				Index:   addr.Index,
				Address: addr.Address,
			}
			log.Printf("Attempting to save new WalletAddress to SubscriberStore: %v", newSubscriberAddress)
			if err := store.GetSubscriberStore().SaveSubscriberAddresses(&newSubscriberAddress); err != nil {
				log.Printf("Error saving WalletAddress to SubscriberStore: %v", err)
				continue
			}
			log.Printf("WalletAddress saved to SubscriberStore: %v", newSubscriberAddress)

			// Save Subscriber-specific data to SubscriberStore
			subscriptionAddress := &types.SubscriberAddress{
				Index:       fmt.Sprint(addr.Index),
				Address:     addr.Address,
				WalletName:  addr.WalletName,
				Status:      AddressStatusAvailable,
				AllocatedAt: &time.Time{},
				Npub:        "", // Use nil for empty pointer
			}
			log.Printf("Attempting to save SubscriberAddress to SubscriberStore: %v", subscriptionAddress)
			if err := store.GetSubscriberStore().SaveSubscriberAddress(subscriptionAddress); err != nil {
				log.Printf("Error saving SubscriberAddress to SubscriberStore: %v", err)
				continue
			}
			log.Printf("SubscriberAddress saved to SubscriberStore: %v", subscriptionAddress)
		}

		processedCount++
	}

	// Return success response with number of addresses processed
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": fmt.Sprintf("Processed %d addresses successfully", processedCount),
	})
}
