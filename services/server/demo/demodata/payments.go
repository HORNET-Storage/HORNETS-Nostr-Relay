package demodata

import (
	"fmt"

	"github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics"
)

// GeneratePaymentNotifications creates payment notifications
func (g *DemoDataGenerator) GeneratePaymentNotifications(store statistics.StatisticsStore, count int) error {
	logging.Infof("Generating %d payment notifications...\n", count)

	// Define subscription tiers
	tiers := []string{"1GB", "5GB", "10GB", "50GB", "100GB"}
	tierPrices := map[string]int64{
		"1GB":   10000,  // 10K sats
		"5GB":   40000,  // 40K sats
		"10GB":  70000,  // 70K sats
		"50GB":  300000, // 300K sats
		"100GB": 500000, // 500K sats
	}

	// Generate notifications spread across the time range
	monthRange := monthsBetween(g.StartMonth, g.EndMonth)

	// Process in batches for better performance
	batchSize := 20
	for i := 0; i < count; i += batchSize {
		// Calculate actual batch size
		currentBatchSize := batchSize
		if i+currentBatchSize > count {
			currentBatchSize = count - i
		}

		logging.Infof("Generating notifications %d to %d...\n", i+1, i+currentBatchSize)

		// Generate a batch of notifications
		for j := 0; j < currentBatchSize; j++ {
			// Select a random month within the range
			monthOffset := g.rng.Intn(monthRange + 1)
			notifMonth := addMonths(g.StartMonth, monthOffset)

			// Select a random tier
			tier := tiers[g.rng.Intn(len(tiers))]
			amount := tierPrices[tier]

			// Add some variation to the amount (±10%)
			variation := float64(amount) * (0.9 + g.rng.Float64()*0.2)
			amount = int64(variation)

			// Create the notification
			notification := &lib.PaymentNotification{
				PubKey:           g.generatePubKey(),
				TxID:             g.generateTxID(),
				Amount:           amount,
				SubscriptionTier: tier,
				IsNewSubscriber:  g.rng.Float64() < 0.3,                             // 30% are new subscribers
				ExpirationDate:   addMonth(getRandomTimeInMonth(notifMonth, g.rng)), // 1 month subscription
				CreatedAt:        getRandomTimeInMonth(notifMonth, g.rng),
				IsRead:           g.rng.Float64() < 0.7, // 70% are read
			}

			// Save the notification
			if err := store.CreatePaymentNotification(notification); err != nil {
				return fmt.Errorf("error creating payment notification at index %d: %v", i+j, err)
			}
		}
	}

	logging.Infof("Payment notification generation complete!")
	return nil
}
