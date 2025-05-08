package xnostr

import (
	"encoding/json"
	"log"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/nbd-wtf/go-nostr"
)

// TriggerVerification checks if a kind 0 event contains an X handle and triggers verification if it does
func TriggerVerification(
	event *nostr.Event,
	store stores.Store,
	xnostrService *Service,
	relayPrivKey *btcec.PrivateKey,
) {
	// Only process kind 0 events
	if event.Kind != 0 {
		return
	}

	// Parse the profile content
	var content map[string]interface{}
	if err := json.Unmarshal([]byte(event.Content), &content); err != nil {
		log.Printf("Error parsing profile content: %v", err)
		return
	}

	// Check if the profile has an X handle
	xHandleRaw, ok := content["x"]
	if !ok {
		// No X handle, delete any existing kind 555 events for this pubkey
		log.Printf("No X handle found for pubkey %s, deleting any existing verifications", event.PubKey)
		deleteExistingVerifications(event.PubKey, store)
		return
	}

	// Get the X handle
	var xHandle string
	switch v := xHandleRaw.(type) {
	case string:
		xHandle = CleanXHandle(v)
	default:
		log.Printf("Invalid X handle type: %T", xHandleRaw)
		return
	}

	// If X handle is empty after cleaning, delete any existing kind 555 events and remove from pending queue
	if xHandle == "" {
		log.Printf("Empty X handle for pubkey %s, deleting any existing verifications", event.PubKey)
		deleteExistingVerifications(event.PubKey, store)

		// Also remove from pending verification queue
		err := store.RemoveFromPendingVerification(event.PubKey)
		if err != nil {
			log.Printf("Warning: Failed to remove from pending verification queue: %v", err)
		}

		return
	}

	// Queue the verification instead of processing it immediately
	err := store.AddToPendingVerification(event.PubKey, xHandle)
	if err != nil {
		log.Printf("Error queueing X-Nostr verification: %v", err)
		return
	}

	log.Printf("Queued X-Nostr verification for pubkey %s with handle %s", event.PubKey, xHandle)
}

// deleteExistingVerifications deletes any existing kind 555 events for a pubkey
func deleteExistingVerifications(pubkey string, store stores.Store) {
	// Create a filter to find kind 555 events for this pubkey
	filter := nostr.Filter{
		Kinds: []int{555},
		Tags: map[string][]string{
			"p": {pubkey},
		},
	}

	// Query the store
	events, err := store.QueryEvents(filter)
	if err != nil {
		log.Printf("Error querying existing kind 555 events: %v", err)
		return
	}

	// Delete any existing kind 555 events
	if len(events) > 0 {
		log.Printf("Deleting %d existing kind 555 events for pubkey %s", len(events), pubkey)
		for _, event := range events {
			if err := store.DeleteEvent(event.ID); err != nil {
				log.Printf("Error deleting kind 555 event %s: %v", event.ID, err)
			}
		}
	}
}

// SchedulePeriodicVerifications schedules periodic verification updates for all profiles with X handles
func SchedulePeriodicVerifications(
	store stores.Store,
	xnostrService *Service,
	relayPrivKey *btcec.PrivateKey,
	updateInterval int64,
	followerUpdateInterval int64, // New parameter for follower update interval
) {
	// Create a worker to handle the verifications
	worker := NewWorker(
		store,
		xnostrService,
		relayPrivKey,
		time.Duration(updateInterval)*time.Hour,
		5, // Default concurrency
	)

	// Start the worker
	worker.Start()

	// Schedule initial verifications
	worker.ScheduleVerifications()

	// Schedule periodic verifications
	ticker := time.NewTicker(time.Duration(followerUpdateInterval) * time.Hour)
	log.Printf("Scheduling periodic X-Nostr verifications every %d hours", followerUpdateInterval)

	go func() {
		for range ticker.C {
			log.Printf("Running scheduled X-Nostr verifications")
			worker.ScheduleVerifications()
		}
	}()
}
