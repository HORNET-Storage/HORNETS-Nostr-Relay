// payment.go - Payment processing logic

package subscription

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	"github.com/HORNET-Storage/hornet-storage/lib/types"
)

// ProcessPayment handles a new subscription payment by updating the NIP-88 event and other relevant data
func (m *SubscriptionManager) ProcessPayment(
	npub string,
	transactionID string,
	amountSats int64,
) error {
	log.Printf("Processing payment of %d sats for %s", amountSats, npub)

	// Validate payment amount
	if amountSats <= 0 {
		return fmt.Errorf("invalid payment amount: %d", amountSats)
	}

	// Get current credit and add to payment for processing
	var totalAmount = amountSats
	creditSats, err := m.store.GetStatsStore().GetSubscriberCredit(npub)
	if err == nil && creditSats > 0 {
		totalAmount = amountSats + creditSats
		log.Printf("Adding existing credit of %d sats to payment (total: %d)",
			creditSats, totalAmount)
	}

	// Get available tiers and find the highest tier
	var highestTier *types.SubscriptionTier
	var highestTierPriceSats int64

	for _, t := range m.subscriptionTiers {
		if t.PriceSats != 0 { // Skip free tier
			PriceSats := int64(t.PriceSats)
			if PriceSats > highestTierPriceSats {
				highestTierPriceSats = PriceSats
				highestTier = &types.SubscriptionTier{
					Name:              t.Name,
					MonthlyLimitBytes: t.MonthlyLimitBytes,
					PriceSats:         t.PriceSats,
					Unlimited:         t.Unlimited,
				}
			}
		}
	}

	// Handle payment greater than the highest tier PriceSats
	if highestTier != nil && totalAmount >= highestTierPriceSats && totalAmount > highestTierPriceSats {
		// If we have credit, reset it since we're using it
		if creditSats > 0 {
			if err := m.store.GetStatsStore().UpdateSubscriberCredit(npub, 0); err != nil {
				log.Printf("Warning: failed to reset credit: %v", err)
			}
		}
		return m.processHighTierPayment(npub, transactionID, totalAmount, highestTier)
	}

	// Try to find matching tier
	tier, err := m.findMatchingTier(totalAmount)
	if err != nil {
		// No matching tier found, add to credit
		if strings.Contains(err.Error(), "no matching tier") {
			// If we already had credit, add the new payment to it
			newCredit := creditSats + amountSats

			if err := m.store.GetStatsStore().UpdateSubscriberCredit(npub, newCredit); err != nil {
				return fmt.Errorf("failed to update credit: %v", err)
			}

			log.Printf("Added %d sats to credit for %s (total credit: %d)",
				amountSats, npub, newCredit)

			// Update the NIP-88 event to reflect the new credit amount
			events, err := m.store.QueryEvents(nostr.Filter{
				Kinds: []int{11888},
				Tags:  nostr.TagMap{"p": []string{npub}},
				Limit: 1,
			})
			if err == nil && len(events) > 0 {
				currentEvent := events[0]

				// Extract current info
				storageInfo, err := m.extractStorageInfo(currentEvent)
				if err != nil {
					log.Printf("Warning: could not extract storage info: %v", err)
					return nil
				}

				// Get address and current tier information
				address := getTagValue(currentEvent.Tags, "relay_bitcoin_address")
				activeTier := getTagValue(currentEvent.Tags, "active_subscription")

				// Get expiration date if any
				var expirationDate time.Time
				if expirationUnix := getTagUnixValue(currentEvent.Tags, "active_subscription"); expirationUnix > 0 {
					expirationDate = time.Unix(expirationUnix, 0)
				} else {
					// Set default expiration for new subscription
					expirationDate = time.Now().AddDate(0, 1, 0)
				}

				// Update the NIP-88 event to reflect the new credit
				if err := m.createOrUpdateNIP88Event(&types.Subscriber{
					Npub:    npub,
					Address: address,
				}, activeTier, expirationDate, &storageInfo); err != nil {
					log.Printf("Warning: failed to update NIP-88 event with credit: %v", err)
				} else {
					log.Printf("Updated NIP-88 event for %s with credit: %d sats", npub, newCredit)
				}
			}

			return nil
		}
		return err
	}

	// We have a matching tier - reset credit if we used it
	if creditSats > 0 {
		if err := m.store.GetStatsStore().UpdateSubscriberCredit(npub, 0); err != nil {
			log.Printf("Warning: failed to reset credit after using: %v", err)
		}
	}

	log.Printf("Found matching tier: %+v", tier)

	// Fetch current NIP-88 event to get existing state
	// Normalize pubkey to search in both formats
	hexKey, npubKey, err := normalizePubkey(npub)
	if err != nil {
		return fmt.Errorf("failed to normalize pubkey: %v", err)
	}
	
	// Get all kind 11888 events and filter manually (like batch processing does)
	allEvents, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{11888},
	})
	if err != nil {
		return fmt.Errorf("failed to query events: %v", err)
	}
	
	// Find the event for this specific user
	var userEvent *nostr.Event
	for _, event := range allEvents {
		for _, tag := range event.Tags {
			if tag[0] == "p" && len(tag) > 1 {
				if tag[1] == npubKey || tag[1] == hexKey {
					userEvent = event
					break
				}
			}
		}
		if userEvent != nil {
			break
		}
	}
	
	if userEvent == nil {
		return fmt.Errorf("no NIP-88 event found for user")
	}
	currentEvent := userEvent

	// Extract current storage info
	storageInfo, err := m.extractStorageInfo(currentEvent)
	if err != nil {
		return fmt.Errorf("failed to extract storage info: %v", err)
	}

	// Validate that the paid tier has reasonable storage limits
	paidTierBytes := tier.MonthlyLimitBytes
	if paidTierBytes <= 0 && !tier.Unlimited {
		return fmt.Errorf("invalid paid tier storage limit: %d bytes", paidTierBytes)
	}
	log.Printf("Processing payment for tier: %s (%d bytes, unlimited: %t)", tier.Name, paidTierBytes, tier.Unlimited)

	// Get current expiration date from event
	expirationUnix := getTagUnixValue(currentEvent.Tags, "active_subscription")
	existingExpiration := time.Time{}
	if expirationUnix > 0 {
		existingExpiration = time.Unix(expirationUnix, 0)
	}

	// Always accumulate storage capacity for paid tiers
	prevBytes := storageInfo.TotalBytes
	newTierBytes := paidTierBytes

	// Add new tier capacity to existing capacity
	storageInfo.TotalBytes += newTierBytes
	log.Printf("Accumulating storage: adding %d bytes to existing %d bytes (new total: %d bytes)",
		newTierBytes, prevBytes, storageInfo.TotalBytes)

	// Calculate new expiration date - add one month from current expiration
	// If existing subscription is valid, extend it by 1 month
	var endDate time.Time
	if existingExpiration.After(time.Now()) {
		endDate = existingExpiration.AddDate(0, 1, 0)
		log.Printf("Extending subscription expiration from %s to %s",
			existingExpiration.Format("2006-01-02"), endDate.Format("2006-01-02"))
	} else {
		// If expired or no previous subscription, start fresh
		endDate = time.Now().AddDate(0, 1, 0)
		log.Printf("Setting new subscription expiration to %s", endDate.Format("2006-01-02"))
	}

	storageInfo.UpdatedAt = time.Now()

	// Get address from current event
	address := getTagValue(currentEvent.Tags, "relay_bitcoin_address")

	// Update the NIP-88 event
	err = m.createOrUpdateNIP88Event(&types.Subscriber{
		Npub:    npub,
		Address: address,
	}, tier, endDate, &storageInfo)

	if err != nil {
		return fmt.Errorf("failed to update NIP-88 event: %v", err)
	}

	// Also update the paid subscribers table
	m.updatePaidSubscriberRecord(npub, tier, endDate, &storageInfo)

	// Verify the update by finding the event manually
	verifyEvents, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{11888},
	})
	
	var verifyEvent *nostr.Event
	if err == nil {
		for _, event := range verifyEvents {
			for _, tag := range event.Tags {
				if tag[0] == "p" && len(tag) > 1 {
					if tag[1] == npubKey || tag[1] == hexKey {
						verifyEvent = event
						break
					}
				}
			}
			if verifyEvent != nil {
				break
			}
		}
	}
	
	if verifyEvent == nil {
		log.Printf("Warning: couldn't verify NIP-88 event update")
	} else {
		log.Printf("Updated NIP-88 event status: %s",
			getTagValue(verifyEvent.Tags, "subscription_status"))
	}

	// Check if there are any sats leftover from this payment that could be credited
	tierPriceSats := int64(tier.PriceSats)
	if totalAmount > tierPriceSats {
		leftover := totalAmount - tierPriceSats
		log.Printf("Payment has %d sats leftover after purchasing tier", leftover)

		// Update credit with leftover amount
		if err := m.store.GetStatsStore().UpdateSubscriberCredit(npub, leftover); err != nil {
			log.Printf("Warning: failed to update credit with leftover amount: %v", err)
		} else {
			log.Printf("Added %d sats to credit for %s", leftover, npub)

			// Check if the updated credit can be used to purchase additional tier capacity
			_, err := m.checkAndApplyCredit(npub, address, &storageInfo, endDate)
			if err != nil {
				log.Printf("Warning: error checking credit for additional tier purchase: %v", err)
			}

			// Fetch the final credit amount to include in the NIP-88 event
			finalCredit, _ := m.store.GetStatsStore().GetSubscriberCredit(npub)

			// Final update of the NIP-88 event to include the latest credit
			if finalCredit > 0 {
				if err := m.createOrUpdateNIP88Event(&types.Subscriber{
					Npub:    npub,
					Address: address,
				}, tier, endDate, &storageInfo); err != nil {
					log.Printf("Warning: failed to update final NIP-88 event with credit: %v", err)
				} else {
					log.Printf("Updated final NIP-88 event for %s with credit: %d sats", npub, finalCredit)
				}
			}
		}
	}

	// Add transaction processing log
	log.Printf("Successfully processed payment for %s: %d sats for tier %s",
		npub, amountSats, tier.Name)

	return nil
}

// processHighTierPayment handles payments that exceed the highest tier PriceSats by extending
// the subscription period and attempting to use any remainder for lower tiers
func (m *SubscriptionManager) processHighTierPayment(
	npub string,
	transactionID string,
	amountSats int64,
	highestTier *types.SubscriptionTier,
) error {
	log.Printf("Processing high-tier payment (tx: %s) for %s: %d sats for tier %s",
		transactionID, npub, amountSats, highestTier.Name)

	// Fetch current NIP-88 event to get existing state
	events, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{11888},
		Tags:  nostr.TagMap{"p": []string{npub}},
		Limit: 1,
	})
	if err != nil || len(events) == 0 {
		return fmt.Errorf("no NIP-88 event found for user")
	}
	currentEvent := events[0]

	// Extract current storage info and address
	storageInfo, err := m.extractStorageInfo(currentEvent)
	if err != nil {
		return fmt.Errorf("failed to extract storage info: %v", err)
	}

	address := getTagValue(currentEvent.Tags, "relay_bitcoin_address")

	// Calculate full periods and remainder for highest tier
	highestTierPriceSats := int64(highestTier.PriceSats)
	fullPeriods := amountSats / highestTierPriceSats
	remainingSats := amountSats % highestTierPriceSats

	if fullPeriods < 1 {
		fullPeriods = 1 // Ensure at least one period
	}

	// Calculate the storage for one period of highest tier
	highestTierStorageBytes := highestTier.MonthlyLimitBytes
	if highestTier.Unlimited {
		highestTierStorageBytes = types.MaxMonthlyLimitBytes // Use max limit for unlimited
	}

	// Calculate total new storage for all periods purchased
	totalNewStorage := highestTierStorageBytes * fullPeriods

	// Add the new storage to existing storage (accumulate instead of replace)
	prevBytes := storageInfo.TotalBytes
	storageInfo.TotalBytes += totalNewStorage
	storageInfo.UpdatedAt = time.Now()

	log.Printf("Upgrading storage from %d to %d bytes (adding %d bytes for %d periods of tier: %s)",
		prevBytes, storageInfo.TotalBytes, totalNewStorage, fullPeriods, highestTier.Name)

	// Calculate end date based on multiple periods
	var endDate time.Time

	// If subscription hasn't expired, extend from current end date
	if existingEndDate := getTagUnixValue(currentEvent.Tags, "active_subscription"); existingEndDate > 0 {
		endTime := time.Unix(existingEndDate, 0)
		if endTime.After(time.Now()) {
			// Extend from current end date
			endDate = endTime.AddDate(0, int(fullPeriods), 0)
			log.Printf("Extending existing subscription by %d months (from %s to %s)",
				fullPeriods, endTime.Format("2006-01-02"), endDate.Format("2006-01-02"))
		} else {
			// Expired - start fresh from now
			endDate = time.Now().AddDate(0, int(fullPeriods), 0)
			log.Printf("Existing subscription expired, starting new %d month subscription",
				fullPeriods)
		}
	} else {
		// No existing subscription, start from now
		endDate = time.Now().AddDate(0, int(fullPeriods), 0)
		log.Printf("Starting new %d month subscription", fullPeriods)
	}

	// Update the NIP-88 event with extended period
	err = m.createOrUpdateNIP88Event(&types.Subscriber{
		Npub:    npub,
		Address: address,
	}, highestTier, endDate, &storageInfo)

	if err != nil {
		return fmt.Errorf("failed to update NIP-88 event: %v", err)
	}

	// Also update the paid subscribers table
	m.updatePaidSubscriberRecord(npub, highestTier, endDate, &storageInfo)

	// Try to use remaining sats for lower tiers (cascading approach)
	// Sort tiers by PriceSats (descending)
	if remainingSats > 0 && len(m.subscriptionTiers) > 1 {
		log.Printf("Attempting to use remaining %d sats for lower tiers", remainingSats)

		// Create a sorted list of tiers by PriceSats (descending)
		type tierInfo struct {
			tier      types.SubscriptionTier
			PriceSats int64
		}

		sortedTiers := make([]tierInfo, 0)
		for _, tier := range m.subscriptionTiers {
			// Skip free tiers and the highest tier (already processed)
			if tier.PriceSats <= 0 || (tier.MonthlyLimitBytes == highestTier.MonthlyLimitBytes && tier.Unlimited == highestTier.Unlimited) {
				continue
			}

			PriceSats := int64(tier.PriceSats)
			if PriceSats > 0 {
				sortedTiers = append(sortedTiers, tierInfo{tier: tier, PriceSats: PriceSats})
			}
		}

		// Sort tiers by PriceSats (descending)
		for i := 0; i < len(sortedTiers)-1; i++ {
			for j := i + 1; j < len(sortedTiers); j++ {
				if sortedTiers[i].PriceSats < sortedTiers[j].PriceSats {
					sortedTiers[i], sortedTiers[j] = sortedTiers[j], sortedTiers[i]
				}
			}
		}

		// Get the lowest tier PriceSats for later comparison
		var lowestTierPriceSats int64 = highestTierPriceSats
		if len(sortedTiers) > 0 {
			lowestTierPriceSats = sortedTiers[len(sortedTiers)-1].PriceSats
		}

		// Try to use remaining sats for each tier
		for _, tierInfo := range sortedTiers {
			if remainingSats >= tierInfo.PriceSats {
				// We can afford this tier
				tierBytes := tierInfo.tier.MonthlyLimitBytes
				if tierInfo.tier.Unlimited {
					tierBytes = types.MaxMonthlyLimitBytes // Use max limit for unlimited
				}

				// Add storage
				storageInfo.TotalBytes += tierBytes

				log.Printf("Using %d sats for additional tier: %s (adding %d bytes)",
					tierInfo.PriceSats, tierInfo.tier.Name, tierBytes)

				// Subtract from remaining sats
				remainingSats -= tierInfo.PriceSats

				// If we run out of sats, break
				if remainingSats < lowestTierPriceSats {
					break
				}
			}
		}

		// Update the NIP-88 event with the additional storage
		if storageInfo.TotalBytes > totalNewStorage+prevBytes {
			err = m.createOrUpdateNIP88Event(&types.Subscriber{
				Npub:    npub,
				Address: address,
			}, highestTier, endDate, &storageInfo)

			if err != nil {
				return fmt.Errorf("failed to update NIP-88 event with additional storage: %v", err)
			}

			// Update the paid subscribers table
			m.updatePaidSubscriberRecord(npub, highestTier, endDate, &storageInfo)
		}
	}

	// Credit remainder if any
	if remainingSats > 0 {
		if err := m.store.GetStatsStore().UpdateSubscriberCredit(npub, remainingSats); err != nil {
			log.Printf("Warning: failed to save remainder credit of %d sats: %v", remainingSats, err)
		} else {
			log.Printf("Credited remainder of %d sats to user account", remainingSats)

			// Check if the stored credit can be used to purchase additional tier capacity
			_, err := m.checkAndApplyCredit(npub, address, &storageInfo, endDate)
			if err != nil {
				log.Printf("Warning: error checking credit for additional tier purchase: %v", err)
			}
		}
	}

	// Final update to ensure credit tag is included in NIP-88 event
	finalCredit, err := m.store.GetStatsStore().GetSubscriberCredit(npub)
	if err == nil && finalCredit > 0 {
		// One last update to ensure the credit is reflected in the NIP-88 event
		err = m.createOrUpdateNIP88Event(&types.Subscriber{
			Npub:    npub,
			Address: address,
		}, highestTier, endDate, &storageInfo)

		if err != nil {
			log.Printf("Warning: failed to update final NIP-88 event with credit: %v", err)
		} else {
			log.Printf("Updated final NIP-88 event for %s with credit: %d sats", npub, finalCredit)
		}
	}

	log.Printf("Successfully processed high-tier payment: %d sats for %d months of %s tier",
		amountSats, fullPeriods, highestTier.Name)

	return nil
}

// findMatchingTier finds the highest tier that matches the payment amount
func (m *SubscriptionManager) findMatchingTier(amountSats int64) (*types.SubscriptionTier, error) {
	if len(m.subscriptionTiers) == 0 {
		// Reload tiers from allowed_users settings
		var allowedUsersSettings types.AllowedUsersSettings
		if err := viper.UnmarshalKey("allowed_users", &allowedUsersSettings); err != nil {
			return nil, fmt.Errorf("no tiers available and failed to load allowed users settings: %v", err)
		}
		m.subscriptionTiers = allowedUsersSettings.Tiers
	}

	log.Printf("Finding tier for %d sats among %d tiers: %+v",
		amountSats, len(m.subscriptionTiers), m.subscriptionTiers)

	var bestMatch *types.SubscriptionTier
	var bestPriceSats int64

	for _, tier := range m.subscriptionTiers {
		if tier.MonthlyLimitBytes <= 0 && !tier.Unlimited {
			log.Printf("Warning: skipping invalid tier: Name='%s', MonthlyLimitBytes='%d', Unlimited='%t'",
				tier.Name, tier.MonthlyLimitBytes, tier.Unlimited)
			continue
		}
		if tier.PriceSats == 0 {
			log.Printf("Warning: skipping free tier: Name='%s'", tier.Name)
			continue
		}

		PriceSats := int64(tier.PriceSats)
		log.Printf("Checking tier: Name='%s', PriceSats='%d' (%d sats)",
			tier.Name, tier.PriceSats, PriceSats)

		// Strict matching: Payment must be >= tier PriceSats exactly
		// No tolerance - exact matches only
		exactMatch := (amountSats >= PriceSats)

		// Must match exactly AND be better than current best match
		if exactMatch && PriceSats > bestPriceSats {
			bestMatch = &types.SubscriptionTier{
				Name:              tier.Name,
				MonthlyLimitBytes: tier.MonthlyLimitBytes,
				PriceSats:         tier.PriceSats,
				Unlimited:         tier.Unlimited,
			}
			bestPriceSats = PriceSats
			log.Printf("New best match: Name='%s', PriceSats='%d' (exact match)",
				bestMatch.Name, bestMatch.PriceSats)
		}
	}

	if bestMatch == nil {
		return nil, fmt.Errorf("no matching tier for payment of %d sats", amountSats)
	}

	log.Printf("Selected tier: Name='%s', PriceSats='%d'",
		bestMatch.Name, bestMatch.PriceSats)
	return bestMatch, nil
}
