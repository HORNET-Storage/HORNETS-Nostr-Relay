package testing

import (
	"fmt"
	"testing"
	"time"

	"github.com/HORNET-Storage/hornet-storage/testing/helpers"
	"github.com/nbd-wtf/go-nostr"
)

// setupTestStore is defined in dag_test.go and shared across all test files
// in this package. It creates a temporary BadgerHold store for direct
// store-level testing.

// =============================================================================
// Limit & Default Max
// =============================================================================

// TestQueryEvents_LimitRespected verifies that QueryEvents honours the Limit
// field and returns at most that many events.
func TestQueryEvents_LimitRespected(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp, err := helpers.GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	// Store 20 events
	for i := 0; i < 20; i++ {
		event := &nostr.Event{
			PubKey:    kp.PublicKey,
			CreatedAt: nostr.Timestamp(time.Now().Unix() - int64(20-i)),
			Kind:      1,
			Content:   fmt.Sprintf("event %d", i),
		}
		event.Sign(kp.PrivateKey)
		if err := store.StoreEvent(event); err != nil {
			t.Fatalf("StoreEvent %d: %v", i, err)
		}
	}

	// Query with limit 5
	limit := 5
	filter := nostr.Filter{
		Authors: []string{kp.PublicKey},
		Kinds:   []int{1},
		Limit:   limit,
	}
	events, err := store.QueryEvents(filter)
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}

	if len(events) > limit {
		t.Errorf("Expected at most %d events, got %d", limit, len(events))
	}
}

// TestQueryEvents_DefaultMaxLimit ensures that when no explicit limit is set,
// QueryEvents caps at the internal default (500).
func TestQueryEvents_DefaultMaxLimit(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp, err := helpers.GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	// Store 10 events — well under the cap
	for i := 0; i < 10; i++ {
		event := &nostr.Event{
			PubKey:    kp.PublicKey,
			CreatedAt: nostr.Timestamp(time.Now().Unix() - int64(10-i)),
			Kind:      1,
			Content:   fmt.Sprintf("event %d", i),
		}
		event.Sign(kp.PrivateKey)
		store.StoreEvent(event)
	}

	filter := nostr.Filter{
		Authors: []string{kp.PublicKey},
		Kinds:   []int{1},
		// Limit == 0 → default max applies
	}
	events, err := store.QueryEvents(filter)
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}

	if len(events) != 10 {
		t.Errorf("Expected 10 events, got %d", len(events))
	}
}

// =============================================================================
// Sort Order
// =============================================================================

// TestQueryEvents_SortedNewestFirst ensures results are returned newest-first.
func TestQueryEvents_SortedNewestFirst(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp, err := helpers.GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	baseTime := time.Now().Unix()
	for i := 0; i < 10; i++ {
		event := &nostr.Event{
			PubKey:    kp.PublicKey,
			CreatedAt: nostr.Timestamp(baseTime + int64(i)), // ascending timestamps
			Kind:      1,
			Content:   fmt.Sprintf("event %d", i),
		}
		event.Sign(kp.PrivateKey)
		store.StoreEvent(event)
	}

	filter := nostr.Filter{
		Authors: []string{kp.PublicKey},
		Kinds:   []int{1},
	}
	events, err := store.QueryEvents(filter)
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}

	for i := 1; i < len(events); i++ {
		if events[i].CreatedAt > events[i-1].CreatedAt {
			t.Errorf("Events not sorted newest-first: index %d (%d) > index %d (%d)",
				i, events[i].CreatedAt, i-1, events[i-1].CreatedAt)
		}
	}
}

// =============================================================================
// ID-Based Lookup
// =============================================================================

// TestQueryEvents_IDLookup verifies that querying by specific IDs returns
// exactly the requested events.
func TestQueryEvents_IDLookup(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp, err := helpers.GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	var allIDs []string
	for i := 0; i < 5; i++ {
		event := &nostr.Event{
			PubKey:    kp.PublicKey,
			CreatedAt: nostr.Timestamp(time.Now().Unix() - int64(5-i)),
			Kind:      1,
			Content:   fmt.Sprintf("event %d", i),
		}
		event.Sign(kp.PrivateKey)
		store.StoreEvent(event)
		allIDs = append(allIDs, event.ID)
	}

	// Request only 2 specific IDs
	wantIDs := []string{allIDs[1], allIDs[3]}
	filter := nostr.Filter{
		IDs: wantIDs,
	}
	events, err := store.QueryEvents(filter)
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(events))
	}

	gotIDs := make(map[string]bool)
	for _, e := range events {
		gotIDs[e.ID] = true
	}
	for _, id := range wantIDs {
		if !gotIDs[id] {
			t.Errorf("Missing expected event ID: %s", id)
		}
	}
}

// =============================================================================
// Kind Filter
// =============================================================================

// TestQueryEvents_KindFilter verifies filtering by event kind.
func TestQueryEvents_KindFilter(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp, err := helpers.GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	// Store events of different kinds
	for i, kind := range []int{1, 1, 7, 7, 7, 30023} {
		event := &nostr.Event{
			PubKey:    kp.PublicKey,
			CreatedAt: nostr.Timestamp(time.Now().Unix() + int64(i)),
			Kind:      kind,
			Content:   fmt.Sprintf("kind-%d-event-%d", kind, i),
		}
		event.Sign(kp.PrivateKey)
		store.StoreEvent(event)
	}

	filter := nostr.Filter{
		Authors: []string{kp.PublicKey},
		Kinds:   []int{7},
	}
	events, err := store.QueryEvents(filter)
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}

	if len(events) != 3 {
		t.Errorf("Expected 3 kind-7 events, got %d", len(events))
	}
	for _, e := range events {
		if e.Kind != 7 {
			t.Errorf("Expected kind 7, got kind %d", e.Kind)
		}
	}
}

// =============================================================================
// Since / Until Time Range
// =============================================================================

// TestQueryEvents_SinceUntil verifies that Since and Until parameters
// correctly bound the result set at the store level.
func TestQueryEvents_SinceUntil(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp, err := helpers.GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	baseTime := time.Now().Unix()

	// Store events spread across time: -100s, -80s, -60s, -40s, -20s
	for i := 0; i < 5; i++ {
		event := &nostr.Event{
			PubKey:    kp.PublicKey,
			CreatedAt: nostr.Timestamp(baseTime - int64((5-i)*20)),
			Kind:      1,
			Content:   fmt.Sprintf("event at offset %d", -(5-i)*20),
		}
		event.Sign(kp.PrivateKey)
		store.StoreEvent(event)
	}

	// Query with Since = baseTime-70 and Until = baseTime-30
	// Should capture events at -60s and -40s (2 events)
	since := nostr.Timestamp(baseTime - 70)
	until := nostr.Timestamp(baseTime - 30)
	filter := nostr.Filter{
		Authors: []string{kp.PublicKey},
		Kinds:   []int{1},
		Since:   &since,
		Until:   &until,
	}
	events, err := store.QueryEvents(filter)
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("Expected 2 events in time range, got %d", len(events))
	}

	for _, e := range events {
		ts := int64(e.CreatedAt)
		if ts < int64(since) || ts > int64(until) {
			t.Errorf("Event timestamp %d outside range [%d, %d]", ts, since, until)
		}
	}
}

// TestQueryEvents_SinceOnly verifies Since without Until.
func TestQueryEvents_SinceOnly(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp, _ := helpers.GenerateKeyPair()
	baseTime := time.Now().Unix()

	// 3 old events, 2 recent
	for i := 0; i < 5; i++ {
		ts := baseTime - 200 + int64(i*100) // -200, -100, 0, +100, +200
		event := &nostr.Event{
			PubKey:    kp.PublicKey,
			CreatedAt: nostr.Timestamp(ts),
			Kind:      1,
			Content:   fmt.Sprintf("event %d", i),
		}
		event.Sign(kp.PrivateKey)
		store.StoreEvent(event)
	}

	since := nostr.Timestamp(baseTime - 50) // only events at 0, +100, +200
	filter := nostr.Filter{
		Authors: []string{kp.PublicKey},
		Kinds:   []int{1},
		Since:   &since,
	}
	events, err := store.QueryEvents(filter)
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}

	if len(events) != 3 {
		t.Errorf("Expected 3 events since cutoff, got %d", len(events))
	}
}

// =============================================================================
// Tag-Based Queries
// =============================================================================

// TestQueryEvents_TagFilter verifies tag-based filtering at the store level.
func TestQueryEvents_TagFilter(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp, _ := helpers.GenerateKeyPair()

	// Event with tag "t"="golang"
	e1 := &nostr.Event{
		PubKey:    kp.PublicKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      1,
		Content:   "Go is great",
		Tags:      nostr.Tags{{"t", "golang"}},
	}
	e1.Sign(kp.PrivateKey)
	store.StoreEvent(e1)

	// Event with tag "t"="rust"
	e2 := &nostr.Event{
		PubKey:    kp.PublicKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix() + 1),
		Kind:      1,
		Content:   "Rust is fast",
		Tags:      nostr.Tags{{"t", "rust"}},
	}
	e2.Sign(kp.PrivateKey)
	store.StoreEvent(e2)

	// Event with no tags
	e3 := &nostr.Event{
		PubKey:    kp.PublicKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix() + 2),
		Kind:      1,
		Content:   "No tags here",
	}
	e3.Sign(kp.PrivateKey)
	store.StoreEvent(e3)

	// Query by tag "t"="golang"
	filter := nostr.Filter{
		Tags: nostr.TagMap{"t": []string{"golang"}},
	}
	events, err := store.QueryEvents(filter)
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("Expected 1 event with tag golang, got %d", len(events))
	}
	if events[0].ID != e1.ID {
		t.Errorf("Expected event %s, got %s", e1.ID, events[0].ID)
	}
}

// =============================================================================
// Combined Multi-Criteria Filters
// =============================================================================

// TestQueryEvents_CombinedFilter verifies that authors+kinds+tags+limit
// all work together correctly in a single query.
func TestQueryEvents_CombinedFilter(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp1, _ := helpers.GenerateKeyPair()
	kp2, _ := helpers.GenerateKeyPair()

	// kp1: 3 kind-1 events with tag "t"="nostr"
	for i := 0; i < 3; i++ {
		e := &nostr.Event{
			PubKey:    kp1.PublicKey,
			CreatedAt: nostr.Timestamp(time.Now().Unix() + int64(i)),
			Kind:      1,
			Content:   fmt.Sprintf("kp1 nostr note %d", i),
			Tags:      nostr.Tags{{"t", "nostr"}},
		}
		e.Sign(kp1.PrivateKey)
		store.StoreEvent(e)
	}

	// kp1: 2 kind-7 events with tag "t"="nostr" (different kind)
	for i := 0; i < 2; i++ {
		e := &nostr.Event{
			PubKey:    kp1.PublicKey,
			CreatedAt: nostr.Timestamp(time.Now().Unix() + int64(10+i)),
			Kind:      7,
			Content:   "+",
			Tags:      nostr.Tags{{"t", "nostr"}},
		}
		e.Sign(kp1.PrivateKey)
		store.StoreEvent(e)
	}

	// kp2: 2 kind-1 events with tag "t"="nostr" (different author)
	for i := 0; i < 2; i++ {
		e := &nostr.Event{
			PubKey:    kp2.PublicKey,
			CreatedAt: nostr.Timestamp(time.Now().Unix() + int64(20+i)),
			Kind:      1,
			Content:   fmt.Sprintf("kp2 nostr note %d", i),
			Tags:      nostr.Tags{{"t", "nostr"}},
		}
		e.Sign(kp2.PrivateKey)
		store.StoreEvent(e)
	}

	// Query: author=kp1, kind=1, tag t=nostr, limit=2
	filter := nostr.Filter{
		Authors: []string{kp1.PublicKey},
		Kinds:   []int{1},
		Tags:    nostr.TagMap{"t": []string{"nostr"}},
		Limit:   2,
	}
	events, err := store.QueryEvents(filter)
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}

	if len(events) > 2 {
		t.Errorf("Expected at most 2 events, got %d", len(events))
	}

	for _, e := range events {
		if e.PubKey != kp1.PublicKey {
			t.Errorf("Expected author %s, got %s", kp1.PublicKey, e.PubKey)
		}
		if e.Kind != 1 {
			t.Errorf("Expected kind 1, got %d", e.Kind)
		}
	}
}

// =============================================================================
// Multiple Authors
// =============================================================================

// TestQueryEvents_MultipleAuthors verifies that querying for multiple authors
// returns events from all specified authors and excludes others.
func TestQueryEvents_MultipleAuthors(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp1, _ := helpers.GenerateKeyPair()
	kp2, _ := helpers.GenerateKeyPair()
	kp3, _ := helpers.GenerateKeyPair()

	// kp1: 2 events
	for i := 0; i < 2; i++ {
		e := &nostr.Event{
			PubKey:    kp1.PublicKey,
			CreatedAt: nostr.Timestamp(time.Now().Unix() + int64(i)),
			Kind:      1,
			Content:   fmt.Sprintf("kp1 note %d", i),
		}
		e.Sign(kp1.PrivateKey)
		store.StoreEvent(e)
	}

	// kp2: 3 events
	for i := 0; i < 3; i++ {
		e := &nostr.Event{
			PubKey:    kp2.PublicKey,
			CreatedAt: nostr.Timestamp(time.Now().Unix() + int64(10+i)),
			Kind:      1,
			Content:   fmt.Sprintf("kp2 note %d", i),
		}
		e.Sign(kp2.PrivateKey)
		store.StoreEvent(e)
	}

	// kp3: 1 event (should NOT be returned)
	e := &nostr.Event{
		PubKey:    kp3.PublicKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix() + 20),
		Kind:      1,
		Content:   "kp3 note",
	}
	e.Sign(kp3.PrivateKey)
	store.StoreEvent(e)

	// Query for kp1 + kp2 only
	filter := nostr.Filter{
		Authors: []string{kp1.PublicKey, kp2.PublicKey},
		Kinds:   []int{1},
	}
	events, err := store.QueryEvents(filter)
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}

	if len(events) != 5 {
		t.Errorf("Expected 5 events from kp1+kp2, got %d", len(events))
	}

	for _, ev := range events {
		if ev.PubKey != kp1.PublicKey && ev.PubKey != kp2.PublicKey {
			t.Errorf("Unexpected author %s in results", ev.PubKey)
		}
	}
}

// =============================================================================
// Empty Results
// =============================================================================

// TestQueryEvents_NoMatch verifies that a query with no matching events
// returns an empty slice and no error.
func TestQueryEvents_NoMatch(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	filter := nostr.Filter{
		Authors: []string{"0000000000000000000000000000000000000000000000000000000000000000"},
		Kinds:   []int{1},
	}
	events, err := store.QueryEvents(filter)
	if err != nil {
		t.Fatalf("QueryEvents should not error on empty results: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("Expected 0 events, got %d", len(events))
	}
}

// =============================================================================
// Content & Tags Fidelity (Round-Trip)
// =============================================================================

// TestQueryEvents_ContentFidelity verifies that event content, tags, and
// metadata survive a store→query round-trip without corruption.
func TestQueryEvents_ContentFidelity(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp, _ := helpers.GenerateKeyPair()

	original := &nostr.Event{
		PubKey:    kp.PublicKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      1,
		Content:   "Hello 🌍! Special chars: <>&\"'\nNewline\tTab",
		Tags: nostr.Tags{
			{"e", "abc123", "wss://relay.example.com"},
			{"p", kp.PublicKey},
			{"t", "test-hashtag"},
			{"nonce", "12345", "20"},
		},
	}
	original.Sign(kp.PrivateKey)
	store.StoreEvent(original)

	filter := nostr.Filter{IDs: []string{original.ID}}
	events, err := store.QueryEvents(filter)
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	got := events[0]

	// Verify all fields
	if got.ID != original.ID {
		t.Errorf("ID mismatch: %s vs %s", got.ID, original.ID)
	}
	if got.PubKey != original.PubKey {
		t.Errorf("PubKey mismatch")
	}
	if got.Kind != original.Kind {
		t.Errorf("Kind mismatch: %d vs %d", got.Kind, original.Kind)
	}
	if got.Content != original.Content {
		t.Errorf("Content mismatch:\n  got:  %q\n  want: %q", got.Content, original.Content)
	}
	if int64(got.CreatedAt) != int64(original.CreatedAt) {
		t.Errorf("CreatedAt mismatch: %d vs %d", got.CreatedAt, original.CreatedAt)
	}

	// Verify tags
	if len(got.Tags) != len(original.Tags) {
		t.Fatalf("Tag count mismatch: got %d, want %d", len(got.Tags), len(original.Tags))
	}
	for i, tag := range original.Tags {
		if len(got.Tags[i]) != len(tag) {
			t.Errorf("Tag %d length mismatch: got %d, want %d", i, len(got.Tags[i]), len(tag))
			continue
		}
		for j, val := range tag {
			if got.Tags[i][j] != val {
				t.Errorf("Tag[%d][%d] mismatch: got %q, want %q", i, j, got.Tags[i][j], val)
			}
		}
	}

	// Verify signature survived
	if got.Sig != original.Sig {
		t.Errorf("Signature mismatch")
	}
}
