package demodata

import (
	"fmt"
	"strconv"

	"github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics"
)

// GenerateWalletBalance creates wallet balance history data
func (g *DemoDataGenerator) GenerateWalletBalance(store statistics.StatisticsStore) error {
	logging.Infof("Generating wallet balance history...")

	// Start with initial balance (e.g., 1,000,000 satoshis)
	initialBalance := int64(1000000)
	currentBalance := initialBalance

	// Generate a balance entry for each month in our range
	monthRange := monthsBetween(g.StartMonth, g.EndMonth)

	for i := 0; i <= monthRange; i++ {
		// Create a timestamp within this month
		currentMonth := addMonth(g.StartMonth)
		for j := 0; j < i; j++ {
			currentMonth = addMonth(currentMonth)
		}
		// Use the timestamp but don't store it in an unused variable
		getRandomTimeInMonth(currentMonth, g.rng)

		// Add some randomness to balance growth (generally increasing)
		// Sometimes decreasing to mimic spending
		balanceChange := int64(float64(currentBalance) * (0.05 + g.rng.Float64()*0.15))
		if g.rng.Float64() < 0.25 { // 25% chance of decrease
			balanceChange = -balanceChange
		}

		currentBalance += balanceChange
		if currentBalance < 0 {
			currentBalance = initialBalance / 10 // Prevent negative balance
		}

		// Create balance record (using string representation as expected by the handler)
		err := store.UpdateWalletBalance("demo-wallet", fmt.Sprintf("%d", currentBalance))
		if err != nil {
			return fmt.Errorf("error generating wallet balance for month %d: %v", i, err)
		}
	}

	logging.Infof("Wallet balance history generation complete!")
	return nil
}

// GenerateWalletTransactions creates transaction history data
func (g *DemoDataGenerator) GenerateWalletTransactions(store statistics.StatisticsStore, count int) error {
	logging.Infof("Generating %d wallet transactions...\n", count)

	// Define transaction types
	types := []string{"deposit", "withdrawal", "payment"}

	// Generate transactions spread across the time range
	monthRange := monthsBetween(g.StartMonth, g.EndMonth)

	// Process in batches for better performance
	batchSize := 20
	for i := 0; i < count; i += batchSize {
		currentBatchSize := batchSize
		if i+currentBatchSize > count {
			currentBatchSize = count - i
		}

		logging.Infof("Generating transactions %d to %d...\n", i+1, i+currentBatchSize)

		for j := 0; j < currentBatchSize; j++ {
			// Select random month
			monthOffset := g.rng.Intn(monthRange + 1)
			txMonth := addMonth(g.StartMonth)
			for k := 0; k < monthOffset; k++ {
				txMonth = addMonth(txMonth)
			}
			txDate := getRandomTimeInMonth(txMonth, g.rng)

			// Determine transaction type
			txType := types[g.rng.Intn(len(types))]

			// Set amount based on type
			var amount int64

			switch txType {
			case "deposit":
				amount = 50000 + int64(g.rng.Float64()*200000) // 50K-250K sats
			case "withdrawal":
				amount = 10000 + int64(g.rng.Float64()*100000) // 10K-110K sats
			case "payment":
				amount = 5000 + int64(g.rng.Float64()*50000) // 5K-55K sats
			}

			// Create transaction record - using only fields that exist in the struct
			tx := lib.WalletTransactions{
				Address: generateRandomHex(34), // Random address
				Date:    txDate,
				Output:  fmt.Sprintf("%d:%d", g.rng.Intn(5), g.rng.Intn(10000)),
				Value:   fmt.Sprintf("%d", amount),
			}

			// Save the transaction
			if err := store.SaveWalletTransaction(tx); err != nil {
				return fmt.Errorf("error creating wallet transaction at index %d: %v", i+j, err)
			}
		}
	}

	logging.Infof("Wallet transaction generation complete!")
	return nil
}

// GenerateWalletAddresses creates wallet address data
func (g *DemoDataGenerator) GenerateWalletAddresses(store statistics.StatisticsStore, count int) error {
	logging.Infof("Generating %d wallet addresses...\n", count)

	// Generate addresses
	for i := 0; i < count; i++ {
		// Create a unique index
		indexHornets := strconv.Itoa(i)

		// Generate a realistic BTC address (starting with bc1 for SegWit)
		address := "bc1" + generateRandomHex(38)

		// Create wallet address record
		walletAddress := &lib.WalletAddress{
			IndexHornets: indexHornets,
			Address:      address,
		}

		// Save the address
		if err := store.SaveAddress(walletAddress); err != nil {
			return fmt.Errorf("error creating wallet address at index %d: %v", i, err)
		}
	}

	logging.Infof("Wallet address generation complete!")
	return nil
}
