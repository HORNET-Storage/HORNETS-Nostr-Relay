package badgerhold

import (
	"fmt"
	"log"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/timshannon/badgerhold/v4"
)

// PendingVerificationKey is the key used to store pending verifications
type PendingVerificationKey struct {
	PubKey string `badgerhold:"key"`
}

// AddToPendingVerification adds a pubkey to the pending verification queue
func (store *BadgerholdStore) AddToPendingVerification(pubkey, xHandle string) error {
	// First, check if there's an existing pending verification for this pubkey
	var existingVerification types.PendingVerification
	err := store.Database.Get(PendingVerificationKey{PubKey: pubkey}, &existingVerification)

	// If there's an existing verification, remove it first
	if err == nil {
		// Only log if the handle has changed
		if existingVerification.XHandle != xHandle {
			log.Printf("X handle changed for pubkey %s: %s -> %s, triggering new verification",
				pubkey, existingVerification.XHandle, xHandle)
		}

		// Remove the existing verification
		err = store.RemoveFromPendingVerification(pubkey)
		if err != nil {
			log.Printf("Warning: Failed to remove existing pending verification: %v", err)
			// Continue anyway to try inserting the new one
		}
	}

	// Create a new pending verification
	pendingVerification := types.PendingVerification{
		PubKey:        pubkey,
		XHandle:       xHandle,
		CreatedAt:     time.Now(),
		LastAttemptAt: time.Time{}, // Zero time value
		Attempts:      0,
	}

	// Store the pending verification
	err = store.Database.Insert(PendingVerificationKey{PubKey: pubkey}, pendingVerification)
	if err != nil {
		return fmt.Errorf("failed to add to pending verification: %w", err)
	}

	log.Printf("Added to pending verification queue: pubkey=%s, handle=%s", pubkey, xHandle)
	return nil
}

// RemoveFromPendingVerification removes a pubkey from the pending verification queue
func (store *BadgerholdStore) RemoveFromPendingVerification(pubkey string) error {
	// Delete the pending verification
	err := store.Database.Delete(PendingVerificationKey{PubKey: pubkey}, &types.PendingVerification{})
	if err != nil {
		return fmt.Errorf("failed to remove from pending verification: %w", err)
	}

	return nil
}

// IsPendingVerification checks if a pubkey is in the pending verification queue
func (store *BadgerholdStore) IsPendingVerification(pubkey string) (bool, error) {
	// Check if the pending verification exists
	var pendingVerification types.PendingVerification
	err := store.Database.Get(PendingVerificationKey{PubKey: pubkey}, &pendingVerification)
	if err != nil {
		if err == badgerhold.ErrNotFound {
			return false, nil
		}
		return false, fmt.Errorf("failed to check pending verification: %w", err)
	}

	return true, nil
}

// GetPendingVerifications returns all pending verifications
func (store *BadgerholdStore) GetPendingVerifications() ([]types.PendingVerification, error) {
	// Get all pending verifications
	var pendingVerifications []types.PendingVerification
	err := store.Database.Find(&pendingVerifications, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending verifications: %w", err)
	}

	return pendingVerifications, nil
}

// GetAndRemovePendingVerifications gets and removes a batch of pending verifications
// Only returns verifications that haven't been attempted in the last 24 hours
func (store *BadgerholdStore) GetAndRemovePendingVerifications(batchSize int) ([]types.PendingVerification, error) {
	// Get all pending verifications
	var allPendingVerifications []types.PendingVerification
	err := store.Database.Find(&allPendingVerifications, badgerhold.Where("PubKey").Ne(""))
	if err != nil {
		return nil, fmt.Errorf("failed to get pending verifications: %w", err)
	}

	// Filter out verifications that were attempted less than 24 hours ago or have empty handles
	var eligibleVerifications []types.PendingVerification
	twentyFourHoursAgo := time.Now().Add(-24 * time.Hour)

	for _, verification := range allPendingVerifications {
		// Skip verifications with empty handles
		if verification.XHandle == "" {
			// Remove these from the queue since they shouldn't be there
			err := store.RemoveFromPendingVerification(verification.PubKey)
			if err != nil {
				fmt.Printf("Failed to remove pending verification with empty handle for pubkey %s: %v\n",
					verification.PubKey, err)
			}
			continue
		}

		// If LastAttemptAt is zero or more than 24 hours ago, include it
		if verification.LastAttemptAt.IsZero() || verification.LastAttemptAt.Before(twentyFourHoursAgo) {
			eligibleVerifications = append(eligibleVerifications, verification)
			if len(eligibleVerifications) >= batchSize {
				break
			}
		}
	}

	// Remove the eligible pending verifications from the database
	for _, pendingVerification := range eligibleVerifications {
		err := store.RemoveFromPendingVerification(pendingVerification.PubKey)
		if err != nil {
			// Log the error but continue
			fmt.Printf("Failed to remove pending verification for pubkey %s: %v\n", pendingVerification.PubKey, err)
		}
	}

	return eligibleVerifications, nil
}

// RequeueFailedVerification re-queues a failed verification with updated attempt count and timestamp
func (store *BadgerholdStore) RequeueFailedVerification(pubkey, xHandle string, attempts int) error {
	// First, remove any existing verification for this pubkey
	err := store.RemoveFromPendingVerification(pubkey)
	if err != nil {
		log.Printf("Warning: Failed to remove existing verification when requeuing: %v", err)
		// Continue anyway to try inserting the new one
	}

	// Create a new pending verification with updated attempt count and timestamp
	pendingVerification := types.PendingVerification{
		PubKey:        pubkey,
		XHandle:       xHandle,
		CreatedAt:     time.Now(),
		LastAttemptAt: time.Now(),
		Attempts:      attempts,
	}

	// Store the pending verification
	err = store.Database.Insert(PendingVerificationKey{PubKey: pubkey}, pendingVerification)
	if err != nil {
		return fmt.Errorf("failed to requeue failed verification: %w", err)
	}

	log.Printf("Requeued failed verification for pubkey=%s, handle=%s, attempts=%d", pubkey, xHandle, attempts)
	return nil
}
