// free_tier.go - Free tier management

package subscription

import (
	"fmt"
	"log"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	"github.com/HORNET-Storage/hornet-storage/lib/types"
)

// InitDailyFreeSubscriptionRenewal sets up a daily job to refresh expired free tier subscriptions
// This should be called once when the application starts
func InitDailyFreeSubscriptionRenewal() {
	go func() {
		for {
			now := time.Now()
			nextRun := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 1, 0, 0, now.Location())
			delay := nextRun.Sub(now)

			log.Printf("Scheduled free tier renewal in %v (at %s)",
				delay, nextRun.Format("2006-01-02 15:04:05"))

			time.Sleep(delay)

			// Run the renewal
			manager := GetGlobalManager()
			if manager != nil {
				log.Printf("Starting daily free tier renewal process")
				if err := manager.RefreshExpiredFreeTierSubscriptions(); err != nil {
					log.Printf("Error in free tier renewal: %v", err)
				} else {
					log.Printf("Successfully completed daily free tier renewal")
				}
			}
		}
	}()
}

// RefreshExpiredFreeTierSubscriptions finds and refreshes all expired free tier subscriptions
func (m *SubscriptionManager) RefreshExpiredFreeTierSubscriptions() error {
	// Load allowed users settings to check for free mode
	var allowedUsersSettings types.AllowedUsersSettings
	if err := viper.UnmarshalKey("allowed_users", &allowedUsersSettings); err != nil {
		log.Printf("Error loading allowed users settings: %v", err)
		return fmt.Errorf("failed to load allowed users settings: %v", err)
	}

	// Only process free tier renewals in free mode
	if allowedUsersSettings.Mode != "free" {
		log.Printf("Skipping free tier renewal - not in free mode (current mode: %s)", allowedUsersSettings.Mode)
		return nil
	}

	log.Printf("Checking for expired free tier subscriptions to refresh")

	now := time.Now()
	batchSize := 50
	processed := 0
	refreshed := 0

	for {
		// Query all kind 888 events in batches
		filter := nostr.Filter{
			Kinds: []int{888},
			Limit: batchSize,
		}

		events, err := m.store.QueryEvents(filter)
		if err != nil {
			return fmt.Errorf("error querying events: %v", err)
		}

		// Exit if no more events
		if len(events) == 0 {
			break
		}

		for _, event := range events {
			processed++

			// Get user pubkey
			pubkey := getTagValue(event.Tags, "p")
			if pubkey == "" {
				continue
			}

			// Check if it's a free tier subscription (PriceSats = "0")
			activeTier := getTagValue(event.Tags, "active_subscription")
			isFreeTier := false
			for _, tier := range allowedUsersSettings.Tiers {
				if tier.Name == activeTier && tier.PriceSats <= 0 {
					isFreeTier = true
					break
				}
			}
			if !isFreeTier {
				continue
			}

			// Get Bitcoin address
			address := getTagValue(event.Tags, "relay_bitcoin_address")

			// Check expiration date
			expirationUnix := getTagUnixValue(event.Tags, "active_subscription")
			expirationDate := time.Unix(expirationUnix, 0)

			// Skip if not expired
			if !now.After(expirationDate) {
				continue
			}

			// Get current storage info
			storageInfo, err := m.extractStorageInfo(event)
			if err != nil {
				log.Printf("Warning: could not extract storage info for %s: %v", pubkey, err)
				continue
			}

			// Reset used storage to zero
			storageInfo.UsedBytes = 0

			// Set new expiration date
			newExpiration := now.AddDate(0, 1, 0)

			// Look for pending storage adjustments
			pendingTierLimit := ""
			for _, tag := range event.Tags {
				if tag[0] == "storage_adjustment_pending" && len(tag) > 1 {
					pendingTierLimit = tag[1]
					break
				}
			}

			// Apply pending adjustment if found
			if pendingTierLimit != "" {
				// Find the tier by name
				var pendingTier *types.SubscriptionTier
				for i := range allowedUsersSettings.Tiers {
					if allowedUsersSettings.Tiers[i].Name == pendingTierLimit {
						pendingTier = &allowedUsersSettings.Tiers[i]
						break
					}
				}
				if pendingTier != nil {
					storageInfo.TotalBytes = pendingTier.MonthlyLimitBytes
					if pendingTier.Unlimited {
						storageInfo.TotalBytes = types.MaxMonthlyLimitBytes
					}
					log.Printf("Applying pending adjustment for %s: new limit %s (%d bytes)",
						pubkey, pendingTierLimit, storageInfo.TotalBytes)
				}
			}

			// Determine which tier to use (keep current free tier or use pending adjustment)
			tierToUse := activeTier
			if pendingTierLimit != "" {
				tierToUse = pendingTierLimit
			}

			// Update the NIP-88 event
			err = m.createOrUpdateNIP88Event(&types.Subscriber{
				Npub:    pubkey,
				Address: address,
			}, tierToUse, newExpiration, &storageInfo)

			if err != nil {
				log.Printf("Error refreshing free tier: %v", err)
			} else {
				refreshed++
				log.Printf("Refreshed free tier for %s until %s",
					pubkey, newExpiration.Format("2006-01-02"))
			}
		}

		// If we got fewer events than requested, we've reached the end
		if len(events) < batchSize {
			break
		}
	}

	log.Printf("Free tier refresh complete: processed %d events, refreshed %d subscriptions",
		processed, refreshed)
	return nil
}
