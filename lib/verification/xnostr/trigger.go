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
		// No X handle, nothing to verify
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

	// Queue the verification instead of processing it immediately
	err := store.AddToPendingVerification(event.PubKey, xHandle)
	if err != nil {
		log.Printf("Error queueing X-Nostr verification: %v", err)
		return
	}

	log.Printf("Queued X-Nostr verification for pubkey %s with handle %s", event.PubKey, xHandle)
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
