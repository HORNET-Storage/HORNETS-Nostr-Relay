package xnostr

import (
	"encoding/json"
	"log"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/nbd-wtf/go-nostr"
)

// Worker handles the background processing of X-Nostr verifications
type Worker struct {
	Store         stores.Store
	XNostrService *Service
	RelayPrivKey  *btcec.PrivateKey
	CheckInterval time.Duration
	Concurrency   int
	StopChan      chan struct{}
	Running       bool
}

// NewWorker creates a new worker for processing pending X-Nostr verifications
func NewWorker(store stores.Store, service *Service, relayPrivKey *btcec.PrivateKey, interval time.Duration, concurrency int) *Worker {
	if concurrency <= 0 {
		concurrency = 3 // Default concurrency
	}

	return &Worker{
		Store:         store,
		XNostrService: service,
		RelayPrivKey:  relayPrivKey,
		CheckInterval: interval,
		Concurrency:   concurrency,
		StopChan:      make(chan struct{}),
	}
}

// Start begins the worker process
func (w *Worker) Start() {
	if w.Running {
		return // Don't start multiple times
	}

	w.Running = true

	// Ticker for checking pending verifications
	verificationTicker := time.NewTicker(w.CheckInterval)

	// Create a worker pool using semaphore pattern
	semaphore := make(chan struct{}, w.Concurrency)

	go func() {
		log.Printf("Starting X-Nostr verification worker with check interval %s", w.CheckInterval)
		defer verificationTicker.Stop()

		for {
			select {
			case <-verificationTicker.C:
				// Get and remove pending verifications atomically
				pendingVerifications, err := w.Store.GetAndRemovePendingVerifications(10) // Process up to 10 verifications at a time
				if err != nil {
					log.Printf("Error getting pending X-Nostr verifications: %v", err)
					continue
				}

				if len(pendingVerifications) > 0 {
					log.Printf("Processing %d pending X-Nostr verifications", len(pendingVerifications))
				}

				// Process each pending verification
				for _, verification := range pendingVerifications {
					// Use the semaphore to limit concurrency
					semaphore <- struct{}{}

					go func(pubKey, xHandle string, attempts int) {
						defer func() { <-semaphore }() // Release the semaphore when done

						w.processVerification(pubKey, xHandle, attempts)
					}(verification.PubKey, verification.XHandle, verification.Attempts)
				}

			case <-w.StopChan:
				log.Println("Stopping X-Nostr verification worker")
				return
			}
		}
	}()
}

// Stop ends the worker process
func (w *Worker) Stop() {
	if !w.Running {
		return
	}

	w.Running = false
	w.StopChan <- struct{}{}
}

// Maximum number of verification attempts before giving up
const MaxVerificationAttempts = 5

// processVerification processes a single X-Nostr verification
func (w *Worker) processVerification(pubKey, xHandle string, attempts int) {
	// Skip verification if handle is empty
	if xHandle == "" {
		log.Printf("Skipping X-Nostr verification for pubkey %s: empty handle", pubKey)
		return
	}

	log.Printf("Processing X-Nostr verification for pubkey %s with handle %s (attempt %d/%d)",
		pubKey, xHandle, attempts+1, MaxVerificationAttempts)

	// Create a verifier
	verifier := NewVerifier(w.Store, w.XNostrService)

	// Update verification
	verificationEvent, err := verifier.UpdateVerification(pubKey, w.RelayPrivKey)
	if err != nil {
		log.Printf("Error updating X-Nostr verification: %v", err)

		// Check if we should retry
		if attempts < MaxVerificationAttempts-1 {
			// Requeue with increased attempt count
			log.Printf("Requeuing verification for pubkey %s for retry in 24 hours (attempt %d/%d)",
				pubKey, attempts+1, MaxVerificationAttempts)

			requeueErr := w.Store.RequeueFailedVerification(pubKey, xHandle, attempts+1)
			if requeueErr != nil {
				log.Printf("Error requeuing verification: %v", requeueErr)
			}
		} else {
			log.Printf("Maximum verification attempts reached for pubkey %s, giving up", pubKey)
		}
		return
	}

	if verificationEvent != nil {
		// Parse the event content to check if verification was actually successful
		var content map[string]interface{}
		if err := json.Unmarshal([]byte(verificationEvent.Content), &content); err != nil {
			log.Printf("Error parsing verification event content: %v", err)
		} else {
			// Check the verified field in the content
			verified, ok := content["verified"].(bool)
			if ok && verified {
				log.Printf("X-Nostr verification process completed successfully for pubkey %s", pubKey)
			} else {
				log.Printf("X-Nostr verification process completed but verification failed for pubkey %s", pubKey)
			}
		}
	} else {
		log.Printf("X-Nostr verification process failed for pubkey %s", pubKey)

		// Check if we should retry
		if attempts < MaxVerificationAttempts-1 {
			// Requeue with increased attempt count
			log.Printf("Requeuing verification for pubkey %s for retry in 24 hours (attempt %d/%d)",
				pubKey, attempts+1, MaxVerificationAttempts)

			requeueErr := w.Store.RequeueFailedVerification(pubKey, xHandle, attempts+1)
			if requeueErr != nil {
				log.Printf("Error requeuing verification: %v", requeueErr)
			}
		} else {
			log.Printf("Maximum verification attempts reached for pubkey %s, giving up", pubKey)
		}
	}
}

// ScheduleVerifications schedules verification for all profiles with X handles
func (w *Worker) ScheduleVerifications() {
	log.Println("Scheduling verification for all profiles with X handles")

	// Find all kind 0 events with an x field
	filter := nostr.Filter{
		Kinds: []int{0},
	}

	events, err := w.Store.QueryEvents(filter)
	if err != nil {
		log.Printf("Error querying kind 0 events: %v", err)
		return
	}

	// Process each event
	for _, event := range events {
		// Parse the profile content
		var content map[string]interface{}
		if err := json.Unmarshal([]byte(event.Content), &content); err != nil {
			log.Printf("Error parsing profile content: %v", err)
			continue
		}

		// Check if the profile has an X handle
		xHandleRaw, ok := content["x"]
		if !ok {
			// No X handle, nothing to verify
			continue
		}

		// Get the X handle
		var xHandle string
		switch v := xHandleRaw.(type) {
		case string:
			xHandle = CleanXHandle(v)
		default:
			log.Printf("Invalid X handle type: %T", xHandleRaw)
			continue
		}

		// Queue the verification
		err := w.Store.AddToPendingVerification(event.PubKey, xHandle)
		if err != nil {
			log.Printf("Error queueing X-Nostr verification for pubkey %s: %v", event.PubKey, err)
			continue
		}

		log.Printf("Scheduled X-Nostr verification for pubkey %s with handle %s", event.PubKey, xHandle)
	}
}
