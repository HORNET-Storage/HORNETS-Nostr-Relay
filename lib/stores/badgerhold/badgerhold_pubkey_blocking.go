package badgerhold

import (
	"fmt"
	"log"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/sessions"
	"github.com/timshannon/badgerhold/v4"
)

// IsBlockedPubkey checks if a pubkey is blocked from connecting to the relay
func (store *BadgerholdStore) IsBlockedPubkey(pubkey string) (bool, error) {
	key := fmt.Sprintf("blocked_pubkey:%s", pubkey)

	var blocked lib.BlockedPubkey
	err := store.Database.Get(key, &blocked)

	if err == badgerhold.ErrNotFound {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return true, nil
}

// BlockPubkey adds a pubkey to the blocklist
func (store *BadgerholdStore) BlockPubkey(pubkey string, reason string) error {
	blocked := lib.BlockedPubkey{
		Pubkey:    pubkey,
		Reason:    reason,
		BlockedAt: time.Now(),
	}

	// Key format: "blocked_pubkey:{pubkey}" for easy querying
	key := fmt.Sprintf("blocked_pubkey:%s", pubkey)
	err := store.Database.Upsert(key, blocked)
	if err != nil {
		return err
	}

	// Terminate any active sessions for this pubkey
	log.Printf("Terminating session for blocked pubkey: %s", pubkey)
	sessions.DeleteSession(pubkey)

	return nil
}

// UnblockPubkey removes a pubkey from the blocklist
func (store *BadgerholdStore) UnblockPubkey(pubkey string) error {
	key := fmt.Sprintf("blocked_pubkey:%s", pubkey)
	return store.Database.Delete(key, lib.BlockedPubkey{})
}

// ListBlockedPubkeys returns all blocked pubkeys
func (store *BadgerholdStore) ListBlockedPubkeys() ([]lib.BlockedPubkey, error) {
	var results []lib.BlockedPubkey

	// Find all blocked pubkeys (using key prefix matching would be better, but we'll use field matching for now)
	err := store.Database.Find(&results, badgerhold.Where("Pubkey").Ne(""))

	if err != nil && err != badgerhold.ErrNotFound {
		return nil, fmt.Errorf("failed to query blocked pubkeys: %w", err)
	}

	// Return empty slice instead of nil if no blocked pubkeys found
	if err == badgerhold.ErrNotFound {
		return []lib.BlockedPubkey{}, nil
	}

	return results, nil
}
