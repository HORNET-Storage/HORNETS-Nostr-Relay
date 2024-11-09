package web

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"sync"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
)

// Constants for address statuses
const (
	AddressStatusAvailable = "available"
	AddressStatusAllocated = "allocated"
	AddressStatusUsed      = "used"
)

// saveWalletAddresses processes incoming Bitcoin addresses and stores them for future allocation.
func saveWalletAddresses(c *fiber.Ctx, store stores.Store) error {
	log.Println("Addresses request received")

	var addresses []types.Address
	if err := json.Unmarshal(c.Body(), &addresses); err != nil {
		log.Printf("Error unmarshaling JSON: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON"})
	}

	expectedWalletName := viper.GetString("wallet_name")
	if expectedWalletName == "" {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Wallet name not configured"})
	}

	statsStore := store.GetStatsStore()
	subscriberStore := store.GetSubscriberStore()
	if statsStore == nil || subscriberStore == nil {
		log.Println("Error: StatsStore or SubscriberStore is nil or not initialized")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Stores not available"})
	}

	const numWorkers = 5
	jobs := make(chan types.Address, len(addresses))
	results := make(chan error, len(addresses))
	var wg sync.WaitGroup

	// Worker function to process addresses
	worker := func(jobs <-chan types.Address, results chan<- error) {
		for addr := range jobs {
			err := processAddress(addr, expectedWalletName, statsStore, subscriberStore)
			results <- err
		}
		wg.Done()
	}

	// Start worker pool
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go worker(jobs, results)
	}

	// Enqueue jobs
	for _, addr := range addresses {
		jobs <- addr
	}
	close(jobs)

	// Wait for all workers to finish
	wg.Wait()
	close(results)

	// Collect results
	var processedCount, errorCount int
	for err := range results {
		if err != nil {
			log.Printf("Error processing address: %v", err)
			errorCount++
		} else {
			processedCount++
		}
	}

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": fmt.Sprintf("Processed %d addresses successfully, %d errors", processedCount, errorCount),
	})
}

// processAddress handles individual address processing, ensuring atomicity and reducing contention.
func processAddress(addr types.Address, expectedWalletName string, statsStore stores.StatisticsStore, subscriberStore stores.SubscriberStore) error {
	if addr.WalletName != expectedWalletName {
		log.Printf("Skipping address from unknown wallet: %s", addr.WalletName)
		return nil
	}

	// Check and save address to StatsStore
	existsInStatsStore, err := statsStore.AddressExists(addr.Address)
	if err != nil {
		log.Printf("Error checking address existence in StatsStore: %v", err)
		return err
	}
	if !existsInStatsStore {
		newStatsAddress := types.WalletAddress{
			Index:   addr.Index,
			Address: addr.Address,
		}
		if err := statsStore.SaveAddress(&newStatsAddress); err != nil {
			log.Printf("Error saving new address to StatsStore: %v", err)
			return err
		}
		log.Printf("Address saved to StatsStore: %v", newStatsAddress)
	}

	// Check and save address to SubscriberStore
	existsInSubscriberStore, err := subscriberStore.AddressExists(addr.Address)
	if err != nil {
		log.Printf("Error checking address existence in SubscriberStore: %v", err)
		return err
	}
	if !existsInSubscriberStore {
		subscriptionAddress := &types.SubscriberAddress{
			Index:       fmt.Sprint(addr.Index),
			Address:     addr.Address,
			WalletName:  addr.WalletName,
			Status:      AddressStatusAvailable,
			AllocatedAt: &time.Time{},
			Npub:        nil,
		}
		if err := subscriberStore.SaveSubscriberAddress(subscriptionAddress); err != nil {
			log.Printf("Error saving SubscriberAddress to SubscriberStore: %v", err)
			return err
		}
		log.Printf("SubscriberAddress saved to SubscriberStore: %v", subscriptionAddress)
	}

	return nil
}

// package web

// import (
// 	"encoding/json"
// 	"fmt"
// 	"log"
// 	"time"

// 	types "github.com/HORNET-Storage/hornet-storage/lib"
// 	"github.com/HORNET-Storage/hornet-storage/lib/stores"
// 	"github.com/gofiber/fiber/v2"
// 	"github.com/spf13/viper"
// )

// // Address status constants
// const (
// 	AddressStatusAvailable = "available"
// 	AddressStatusAllocated = "allocated"
// 	AddressStatusUsed      = "used"
// )

// // saveWalletAddresses processes incoming Bitcoin addresses and stores them for future
// // subscriber allocation. These addresses will be used when subscribers initialize their
// // subscription and need a payment address.
// func saveWalletAddresses(c *fiber.Ctx, store stores.Store) error {
// 	log.Println("Addresses request received")

// 	body := c.Body()

// 	var addresses []types.Address
// 	if err := json.Unmarshal(body, &addresses); err != nil {
// 		log.Printf("Error unmarshaling JSON directly: %v", err)
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
// 			"error": "Cannot parse JSON",
// 		})
// 	}

// 	expectedWalletName := viper.GetString("wallet_name")
// 	if expectedWalletName == "" {
// 		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
// 			"error": "Wallet name not configured",
// 		})
// 	}

// 	log.Printf("Expected wallet name: %s", expectedWalletName)

// 	statsStore := store.GetStatsStore()
// 	if statsStore == nil {
// 		log.Println("Error: StatsStore is nil or not initialized")
// 		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
// 			"error": "StatsStore not available",
// 		})
// 	}
// 	log.Println("Successfully accessed StatsStore")

// 	log.Println("Successfully accessed SubscriberStore")

// 	processedCount := 0

// 	// Process each address from the request
// 	for _, addr := range addresses {
// 		if addr.WalletName != expectedWalletName {
// 			log.Printf("Skipping address from unknown wallet: %s", addr.WalletName)
// 			continue
// 		}

// 		// Check if the address exists in StatsStore and save if it doesn't
// 		existsInStatsStore, err := statsStore.AddressExists(addr.Address)
// 		if err != nil {
// 			log.Printf("Error checking address existence in StatsStore: %v", err)
// 			continue
// 		}

// 		// Save address to StatsStore if it doesn't exist
// 		if !existsInStatsStore {
// 			newStatsAddress := types.WalletAddress{
// 				Index:   addr.Index,
// 				Address: addr.Address,
// 			}
// 			log.Printf("Attempting to save new address to StatsStore: %v", newStatsAddress)
// 			if err := statsStore.SaveAddress(&newStatsAddress); err != nil {
// 				log.Printf("Error saving new address to StatsStore: %v", err)
// 				continue
// 			}
// 			log.Printf("Address saved to StatsStore: %v", newStatsAddress)
// 		}

// 		// Check if the address exists in SubscriberStore and save if it doesn't
// 		existsInSubscriberStore, err := store.GetSubscriberStore().AddressExists(addr.Address)
// 		if err != nil {
// 			log.Printf("Error checking address existence in SubscriberStore: %v", err)
// 			continue
// 		}

// 		// Save WalletAddress to SubscriberStore if it doesn't exist
// 		if !existsInSubscriberStore {
// 			// Save Subscriber-specific data to SubscriberStore
// 			subscriptionAddress := &types.SubscriberAddress{
// 				Index:       fmt.Sprint(addr.Index),
// 				Address:     addr.Address,
// 				WalletName:  addr.WalletName,
// 				Status:      AddressStatusAvailable,
// 				AllocatedAt: &time.Time{},
// 				Npub:        nil, // Use nil for empty pointer
// 			}
// 			log.Printf("Attempting to save SubscriberAddress to SubscriberStore: %v", subscriptionAddress)
// 			if err := store.GetSubscriberStore().SaveSubscriberAddress(subscriptionAddress); err != nil {
// 				log.Printf("Error saving SubscriberAddress to SubscriberStore: %v", err)
// 				continue
// 			}
// 			log.Printf("SubscriberAddress saved to SubscriberStore: %v", subscriptionAddress)
// 		}

// 		processedCount++
// 	}

// 	// Return success response with number of addresses processed
// 	return c.JSON(fiber.Map{
// 		"status":  "success",
// 		"message": fmt.Sprintf("Processed %d addresses successfully", processedCount),
// 	})
// }
