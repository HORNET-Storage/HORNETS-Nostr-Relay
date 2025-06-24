package wallet

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"sync"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics"
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
func SaveWalletAddresses(c *fiber.Ctx, store stores.Store) error {
	log.Println("Addresses request received")

	// First try to unmarshal as array of maps (as sent by the wallet)
	var rawAddresses []map[string]interface{}
	if err := json.Unmarshal(c.Body(), &rawAddresses); err != nil {
		log.Printf("Error unmarshaling JSON: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON"})
	}

	// Convert the raw addresses to our Address struct
	var addresses []types.Address
	for _, raw := range rawAddresses {
		addr := types.Address{
			IndexHornets: raw["index"].(string), // Keep as string as per original struct
			Address:      raw["address"].(string),
			WalletName:   raw["wallet_name"].(string),
			Status:       AddressStatusAvailable, // Set default status
		}
		addresses = append(addresses, addr)
	}

	expectedWalletName := viper.GetString("wallet.name")
	if expectedWalletName == "" {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Wallet name not configured"})
	}

	statsStore := store.GetStatsStore()
	if statsStore == nil {
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
			err := ProcessWalletAddress(addr, expectedWalletName, statsStore)
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
func ProcessWalletAddress(addr types.Address, expectedWalletName string, statsStore statistics.StatisticsStore) error {
	if addr.WalletName != expectedWalletName {
		log.Printf("Skipping address from unknown wallet: %s", addr.WalletName)
		return nil
	}

	// Check and save address to StatsStore
	existsInWallet, err := statsStore.WalletAddressExists(addr.Address)
	if err != nil {
		return fmt.Errorf("error checking wallet address existence: %v", err)
	}

	if !existsInWallet {
		newStatsAddress := types.WalletAddress{
			IndexHornets: addr.IndexHornets,
			Address:      addr.Address,
		}
		if err := statsStore.SaveAddress(&newStatsAddress); err != nil {
			log.Printf("Error saving new address to StatsStore: %v", err)
			return err
		}
		log.Printf("Address saved to StatsStore: %v", newStatsAddress)
	}

	// Check and save address to SubscriberStore
	existsInSubscriber, err := statsStore.SubscriberAddressExists(addr.Address)
	if err != nil {
		return fmt.Errorf("error checking subscriber address existence: %v", err)
	}
	if !existsInSubscriber {
		subscriptionAddress := &types.SubscriberAddress{
			IndexHornets: fmt.Sprint(addr.IndexHornets),
			Address:      addr.Address,
			WalletName:   addr.WalletName,
			Status:       AddressStatusAvailable,
			AllocatedAt:  &time.Time{},
			Npub:         nil,
		}
		if err := statsStore.SaveSubscriberAddress(subscriptionAddress); err != nil {
			log.Printf("Error saving SubscriberAddress to SubscriberStore: %v", err)
			return err
		}
		log.Printf("SubscriberAddress saved to SubscriberStore: %v", subscriptionAddress)
	}

	return nil
}

// Refactored pullWalletAddresses function
func PullWalletAddresses(c *fiber.Ctx, store stores.Store) error {
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
