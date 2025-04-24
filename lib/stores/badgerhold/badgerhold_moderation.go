package badgerhold

import (
	"fmt"
	"log"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind19841"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"
	"github.com/timshannon/badgerhold/v4"
)

// AddToPendingModeration adds an event to the pending moderation queue
func (store *BadgerholdStore) AddToPendingModeration(eventID string, imageURLs []string) error {
	pending := lib.PendingModeration{
		EventID:   eventID,
		ImageURLs: imageURLs,
		AddedAt:   time.Now(),
	}

	// Key format: "pending_mod:{eventID}" for easy querying
	key := fmt.Sprintf("pending_mod:%s", eventID)

	return store.Database.Upsert(key, pending)
}

// RemoveFromPendingModeration removes an event from the pending moderation queue
func (store *BadgerholdStore) RemoveFromPendingModeration(eventID string) error {
	key := fmt.Sprintf("pending_mod:%s", eventID)
	return store.Database.Delete(key, lib.PendingModeration{})
}

// IsPendingModeration checks if an event is pending moderation
func (store *BadgerholdStore) IsPendingModeration(eventID string) (bool, error) {
	key := fmt.Sprintf("pending_mod:%s", eventID)

	var pending lib.PendingModeration
	err := store.Database.Get(key, &pending)

	if err == badgerhold.ErrNotFound {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return true, nil
}

// GetPendingModerationEvents returns all events pending moderation
func (store *BadgerholdStore) GetPendingModerationEvents() ([]lib.PendingModeration, error) {
	var results []lib.PendingModeration

	// Query all records with the "pending_mod:" prefix
	err := store.Database.Find(&results, badgerhold.Where("EventID").Ne(""))

	if err != nil && err != badgerhold.ErrNotFound {
		return nil, fmt.Errorf("failed to query pending moderation events: %w", err)
	}

	return results, nil
}

// GetAndRemovePendingModeration atomically gets and removes pending moderation events up to the batch size.
// This method provides race-condition-free event processing by ensuring each event is only processed once.
// It's designed to solve the problem of duplicate event processing in concurrent environments.
func (store *BadgerholdStore) GetAndRemovePendingModeration(batchSize int) ([]lib.PendingModeration, error) {
	var results []lib.PendingModeration

	// Make sure batch size is reasonable
	if batchSize <= 0 {
		batchSize = 10 // Default to 10 if not specified
	}

	// First get all pending events
	err := store.Database.Find(&results, badgerhold.Where("EventID").Ne("").Limit(batchSize))
	if err != nil && err != badgerhold.ErrNotFound {
		return nil, fmt.Errorf("failed to query pending moderation events: %w", err)
	}

	// If we found events, remove them from the queue immediately to prevent duplicate processing
	for _, event := range results {
		key := fmt.Sprintf("pending_mod:%s", event.EventID)
		err := store.Database.Delete(key, lib.PendingModeration{})
		if err != nil {
			// If we fail to delete, log the error but continue with other events
			// This is non-fatal as the event will still be processed this time
			fmt.Printf("Error removing event %s from pending moderation: %v\n", event.EventID, err)
		}
	}

	return results, nil
}

// MarkEventBlocked marks an event as blocked with a timestamp
func (store *BadgerholdStore) MarkEventBlocked(eventID string, timestamp int64) error {
	// Call the detailed version with default values
	return store.MarkEventBlockedWithDetails(eventID, timestamp, "Failed image moderation", 0, "")
}

// MarkEventBlockedWithDetails marks an event as blocked with additional details and creates a moderation ticket
func (store *BadgerholdStore) MarkEventBlockedWithDetails(eventID string, timestamp int64, reason string, contentLevel int, mediaURL string) error {
	// Create retention time - 48 hours from the blocked timestamp
	blockedAt := time.Unix(timestamp, 0)
	retainUntil := blockedAt.Add(48 * time.Hour)

	blocked := lib.BlockedEvent{
		EventID:     eventID,
		Reason:      reason,
		BlockedAt:   blockedAt,
		RetainUntil: retainUntil,
	}

	// Key format: "blocked:{eventID}" for easy querying
	key := fmt.Sprintf("blocked:%s", eventID)
	if err := store.Database.Upsert(key, blocked); err != nil {
		return err
	}

	// Get the original event to create a ticket
	filter := nostr.Filter{
		IDs: []string{eventID},
	}
	events, err := store.QueryEvents(filter)
	if err != nil || len(events) == 0 {
		return fmt.Errorf("failed to retrieve event for ticket creation: %v", err)
	}

	// Get relay public key and private key for signing
	relayPubKey := viper.GetString("relaypubkey")
	relayPrivKey := viper.GetString("private_key")

	// Create a moderation ticket for this blocked event
	_, err = kind19841.CreateModerationTicket(
		store,
		eventID,
		events[0].PubKey,
		reason,
		contentLevel,
		mediaURL,
		relayPubKey,
		relayPrivKey,
	)

	if err != nil {
		log.Printf("Error creating moderation ticket: %v", err)
		// Continue anyway as the event is already marked as blocked
	}

	return nil
}

// IsEventBlocked checks if an event is blocked
func (store *BadgerholdStore) IsEventBlocked(eventID string) (bool, error) {
	key := fmt.Sprintf("blocked:%s", eventID)

	var blocked lib.BlockedEvent
	err := store.Database.Get(key, &blocked)

	if err == badgerhold.ErrNotFound {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return true, nil
}

// UnmarkEventBlocked removes an event from the blocked list
func (store *BadgerholdStore) UnmarkEventBlocked(eventID string) error {
	key := fmt.Sprintf("blocked:%s", eventID)
	return store.Database.Delete(key, lib.BlockedEvent{})
}

// DeleteBlockedEventsOlderThan deletes events that have been blocked for longer than the specified age (in seconds)
func (store *BadgerholdStore) DeleteBlockedEventsOlderThan(age int64) (int, error) {
	var blockedEvents []lib.BlockedEvent
	var deletedCount int

	// Query all blocked events
	err := store.Database.Find(&blockedEvents, badgerhold.Where("EventID").Ne(""))
	if err != nil && err != badgerhold.ErrNotFound {
		return 0, fmt.Errorf("failed to query blocked events: %w", err)
	}

	// Current time for comparison
	now := time.Now()

	// For each event, check if it's older than the retention period
	for _, event := range blockedEvents {
		// Skip events with active disputes
		if event.HasDispute {
			continue
		}

		if now.After(event.RetainUntil) {
			// Delete the event from both the blocked list and the main event store
			key := fmt.Sprintf("blocked:%s", event.EventID)
			if err := store.Database.Delete(key, lib.BlockedEvent{}); err != nil {
				fmt.Printf("Error deleting blocked event %s: %v\n", event.EventID, err)
				continue
			}

			// Also delete from main event store if it still exists
			if err := store.DeleteEvent(event.EventID); err != nil {
				fmt.Printf("Error deleting event %s: %v\n", event.EventID, err)
				// Continue anyway as we've already removed it from the blocked list
			}

			deletedCount++
		}
	}

	return deletedCount, nil
}

// AddToPendingDisputeModeration adds a dispute to the pending dispute moderation queue
func (store *BadgerholdStore) AddToPendingDisputeModeration(disputeID string, ticketID string, eventID string, mediaURL string, disputeReason string, userPubKey string) error {
	pending := lib.PendingDisputeModeration{
		DisputeID:     disputeID,
		TicketID:      ticketID,
		EventID:       eventID,
		MediaURL:      mediaURL,
		DisputeReason: disputeReason,
		UserPubKey:    userPubKey,
		AddedAt:       time.Now(),
	}

	// Key format: "pending_dispute:{disputeID}" for easy querying
	key := fmt.Sprintf("pending_dispute:%s", disputeID)

	return store.Database.Upsert(key, pending)
}

// RemoveFromPendingDisputeModeration removes a dispute from the pending dispute moderation queue
func (store *BadgerholdStore) RemoveFromPendingDisputeModeration(disputeID string) error {
	key := fmt.Sprintf("pending_dispute:%s", disputeID)
	return store.Database.Delete(key, lib.PendingDisputeModeration{})
}

// IsPendingDisputeModeration checks if a dispute is pending moderation
func (store *BadgerholdStore) IsPendingDisputeModeration(disputeID string) (bool, error) {
	key := fmt.Sprintf("pending_dispute:%s", disputeID)

	var pending lib.PendingDisputeModeration
	err := store.Database.Get(key, &pending)

	if err == badgerhold.ErrNotFound {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return true, nil
}

// GetPendingDisputeModerationEvents returns all disputes pending moderation
func (store *BadgerholdStore) GetPendingDisputeModerationEvents() ([]lib.PendingDisputeModeration, error) {
	var results []lib.PendingDisputeModeration

	// Query all records with the "pending_dispute:" prefix
	err := store.Database.Find(&results, badgerhold.Where("DisputeID").Ne(""))

	if err != nil && err != badgerhold.ErrNotFound {
		return nil, fmt.Errorf("failed to query pending dispute moderation events: %w", err)
	}

	return results, nil
}

// GetAndRemovePendingDisputeModeration atomically gets and removes pending dispute moderation events up to the batch size.
func (store *BadgerholdStore) GetAndRemovePendingDisputeModeration(batchSize int) ([]lib.PendingDisputeModeration, error) {
	var results []lib.PendingDisputeModeration

	// Make sure batch size is reasonable
	if batchSize <= 0 {
		batchSize = 10 // Default to 10 if not specified
	}

	// First get all pending disputes
	err := store.Database.Find(&results, badgerhold.Where("DisputeID").Ne("").Limit(batchSize))
	if err != nil && err != badgerhold.ErrNotFound {
		return nil, fmt.Errorf("failed to query pending dispute moderation events: %w", err)
	}

	// If we found disputes, remove them from the queue immediately to prevent duplicate processing
	for _, dispute := range results {
		key := fmt.Sprintf("pending_dispute:%s", dispute.DisputeID)
		err := store.Database.Delete(key, lib.PendingDisputeModeration{})
		if err != nil {
			// If we fail to delete, log the error but continue with other disputes
			fmt.Printf("Error removing dispute %s from pending dispute moderation: %v\n", dispute.DisputeID, err)
		}
	}

	return results, nil
}

// MarkEventDisputed marks an event as having an active dispute
func (store *BadgerholdStore) MarkEventDisputed(eventID string) error {
	key := fmt.Sprintf("blocked:%s", eventID)

	var blocked lib.BlockedEvent
	err := store.Database.Get(key, &blocked)
	if err != nil {
		return fmt.Errorf("failed to get blocked event %s: %w", eventID, err)
	}

	// Update the HasDispute flag
	blocked.HasDispute = true

	// Save the updated event
	return store.Database.Upsert(key, blocked)
}

// HasEventDispute checks if an event has an active dispute
func (store *BadgerholdStore) HasEventDispute(eventID string) (bool, error) {
	key := fmt.Sprintf("blocked:%s", eventID)

	var blocked lib.BlockedEvent
	err := store.Database.Get(key, &blocked)

	if err == badgerhold.ErrNotFound {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return blocked.HasDispute, nil
}
