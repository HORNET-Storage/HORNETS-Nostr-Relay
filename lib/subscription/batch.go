// batch.go - Batch operations and scheduling

package subscription

import (
	"fmt"
	"log"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	"github.com/HORNET-Storage/hornet-storage/lib/types"
)

// ScheduleBatchUpdateAfter schedules a batch update of all kind 888 events after
// the specified delay. If called multiple times, it cancels any previous scheduled update
// and restarts the timer with a new delay (sliding window approach).
func ScheduleBatchUpdateAfter(delay time.Duration) {
	scheduledUpdateMutex.Lock()
	defer scheduledUpdateMutex.Unlock()

	// Cancel any existing scheduled update
	if scheduledUpdateTimer != nil {
		scheduledUpdateTimer.Stop()
		log.Printf("Rescheduling batch update of kind 888 events (settings changed again)")
	} else {
		log.Printf("Scheduling batch update of kind 888 events in %v", delay)
	}

	// Schedule new update
	scheduledUpdateTimer = time.AfterFunc(delay, func() {
		scheduledUpdateMutex.Lock()
		scheduledUpdateTimer = nil
		scheduledUpdateMutex.Unlock()

		log.Printf("Starting batch update of kind 888 events after %v cooldown", delay)
		manager := GetGlobalManager()
		if manager != nil {
			if err := manager.BatchUpdateAllSubscriptionEvents(); err != nil {
				log.Printf("Error in batch update: %v", err)
			} else {
				log.Printf("Successfully completed batch update of kind 888 events")
			}
		}
	})
}

// BatchUpdateAllSubscriptionEvents processes all kind 888 events in batches
// to update storage allocations after allowed users settings have changed
func (m *SubscriptionManager) BatchUpdateAllSubscriptionEvents() error {
	log.Printf("Starting batch update of all kind 888 subscription events")

	// Get the current allowed users settings
	var allowedUsersSettings types.AllowedUsersSettings
	if err := viper.UnmarshalKey("allowed_users", &allowedUsersSettings); err != nil {
		return fmt.Errorf("failed to load allowed users settings: %v", err)
	}

	// Process events in batches of 50
	batchSize := 50
	processed := 0

	for {
		// Query the next batch of events
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

		// Process each event in the batch
		for _, event := range events {
			if err := m.processSingleSubscriptionEvent(event); err != nil {
				log.Printf("Error processing event %s: %v", event.ID, err)
				// Continue with next event even if this one fails
			}

			processed++
		}

		log.Printf("Processed %d kind 888 events so far", processed)

		// If we received fewer events than requested, we've reached the end of available events
		if len(events) < batchSize {
			break
		}
	}

	log.Printf("Completed batch update, processed %d kind 888 events", processed)
	return nil
}

// processSingleSubscriptionEvent handles updating relay_mode tag for existing kind 888 events
// This function only updates the relay_mode tag and preserves all other subscription details
func (m *SubscriptionManager) processSingleSubscriptionEvent(event *nostr.Event) error {
	// Extract pubkey
	pubkey := getTagValue(event.Tags, "p")
	if pubkey == "" {
		return fmt.Errorf("no pubkey found in event")
	}

	// Check if relay_mode tag already exists and is current
	currentRelayMode := getTagValue(event.Tags, "relay_mode")
	expectedRelayMode := m.getRelayMode()

	// If relay_mode is already correct, no update needed
	if currentRelayMode == expectedRelayMode {
		log.Printf("Event for %s already has correct relay_mode: %s", pubkey, currentRelayMode)
		return nil
	}

	log.Printf("Updating relay_mode for %s from '%s' to '%s'", pubkey, currentRelayMode, expectedRelayMode)

	// Get all existing event details to preserve them
	storageInfo, err := m.extractStorageInfo(event)
	if err != nil {
		return fmt.Errorf("failed to extract storage info: %v", err)
	}

	// Get subscription details (preserve existing values)
	activeTier := getTagValue(event.Tags, "active_subscription")
	expirationUnix := getTagUnixValue(event.Tags, "active_subscription")
	expirationDate := time.Unix(expirationUnix, 0)
	address := getTagValue(event.Tags, "relay_bitcoin_address")

	// Update the event with only the relay_mode tag changed
	return m.createOrUpdateNIP88Event(&types.Subscriber{
		Npub:    pubkey,
		Address: address,
	}, activeTier, expirationDate, &storageInfo)
}
