package testing

import (
	"fmt"
	"testing"
	"time"

	"github.com/HORNET-Storage/hornet-storage/testing/helpers"
	"github.com/nbd-wtf/go-nostr"
)

// =============================================================================
// Batch Blocked Check
// =============================================================================

// TestBatchCheckEventsBlocked verifies the batch blocked-check method.
func TestBatchCheckEventsBlocked(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp, _ := helpers.GenerateKeyPair()

	// Store 5 events
	var ids []string
	for i := 0; i < 5; i++ {
		event := &nostr.Event{
			PubKey:    kp.PublicKey,
			CreatedAt: nostr.Timestamp(time.Now().Unix() - int64(i)),
			Kind:      1,
			Content:   fmt.Sprintf("event %d", i),
		}
		event.Sign(kp.PrivateKey)
		store.StoreEvent(event)
		ids = append(ids, event.ID)
	}

	// Block events 1 and 3
	store.MarkEventBlocked(ids[1], time.Now().Unix())
	store.MarkEventBlocked(ids[3], time.Now().Unix())

	// Batch check — returns sparse map (only blocked IDs appear as true)
	result, err := store.BatchCheckEventsBlocked(ids)
	if err != nil {
		t.Fatalf("BatchCheckEventsBlocked: %v", err)
	}

	// Blocked IDs should be true
	if !result[ids[1]] {
		t.Errorf("Event 1 should be blocked")
	}
	if !result[ids[3]] {
		t.Errorf("Event 3 should be blocked")
	}
	// Non-blocked IDs should not appear (zero value = false)
	if result[ids[0]] {
		t.Errorf("Event 0 should not be blocked")
	}
	if result[ids[2]] {
		t.Errorf("Event 2 should not be blocked")
	}
	if result[ids[4]] {
		t.Errorf("Event 4 should not be blocked")
	}
}

// TestBatchCheckEventsBlocked_Empty verifies batch check with empty input.
func TestBatchCheckEventsBlocked_Empty(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	result, err := store.BatchCheckEventsBlocked(nil)
	if err != nil {
		t.Fatalf("BatchCheckEventsBlocked: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected empty result, got %d entries", len(result))
	}
}

// =============================================================================
// Batch Pending Moderation Check
// =============================================================================

// TestBatchCheckPendingModeration verifies the batch pending-moderation check.
func TestBatchCheckPendingModeration(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp, _ := helpers.GenerateKeyPair()

	var ids []string
	for i := 0; i < 4; i++ {
		event := &nostr.Event{
			PubKey:    kp.PublicKey,
			CreatedAt: nostr.Timestamp(time.Now().Unix() - int64(i)),
			Kind:      1,
			Content:   fmt.Sprintf("event %d", i),
		}
		event.Sign(kp.PrivateKey)
		store.StoreEvent(event)
		ids = append(ids, event.ID)
	}

	// Mark events 0 and 2 as pending moderation
	store.AddToPendingModeration(ids[0], []string{"http://example.com/img.jpg"})
	store.AddToPendingModeration(ids[2], []string{"http://example.com/img2.jpg"})

	result, err := store.BatchCheckPendingModeration(ids)
	if err != nil {
		t.Fatalf("BatchCheckPendingModeration: %v", err)
	}

	// Pending IDs should be true
	if !result[ids[0]] {
		t.Errorf("Event 0 should be pending")
	}
	if !result[ids[2]] {
		t.Errorf("Event 2 should be pending")
	}
	// Non-pending IDs should not appear (zero value = false)
	if result[ids[1]] {
		t.Errorf("Event 1 should not be pending")
	}
	if result[ids[3]] {
		t.Errorf("Event 3 should not be pending")
	}
}

// =============================================================================
// Blocked Events at Store Level
// =============================================================================

// TestQueryEvents_BlockedEventsStillReturned verifies that store.QueryEvents
// still returns blocked events. Moderation filtering happens in the handler
// layer, not at the store level. This ensures the store layer is neutral.
func TestQueryEvents_BlockedEventsStillReturned(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp, _ := helpers.GenerateKeyPair()

	// Store 3 events
	var ids []string
	for i := 0; i < 3; i++ {
		e := &nostr.Event{
			PubKey:    kp.PublicKey,
			CreatedAt: nostr.Timestamp(time.Now().Unix() + int64(i)),
			Kind:      1,
			Content:   fmt.Sprintf("event %d", i),
		}
		e.Sign(kp.PrivateKey)
		store.StoreEvent(e)
		ids = append(ids, e.ID)
	}

	// Block event 1
	store.MarkEventBlocked(ids[1], time.Now().Unix())

	// QueryEvents at the store level should still return all 3
	// (moderation filtering is in the handler, not the store)
	filter := nostr.Filter{
		Authors: []string{kp.PublicKey},
		Kinds:   []int{1},
	}
	events, err := store.QueryEvents(filter)
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}

	if len(events) != 3 {
		t.Errorf("Store-level query should return all 3 events (including blocked), got %d", len(events))
	}

	// But BatchCheckEventsBlocked should correctly identify the blocked one
	blocked, _ := store.BatchCheckEventsBlocked(ids)
	if !blocked[ids[1]] {
		t.Errorf("Event 1 should be identified as blocked")
	}
	if blocked[ids[0]] || blocked[ids[2]] {
		t.Errorf("Events 0 and 2 should not be blocked")
	}
}
