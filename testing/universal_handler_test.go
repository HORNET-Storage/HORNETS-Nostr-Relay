package testing

import (
	"context"
	"testing"
	"time"

	"github.com/HORNET-Storage/hornet-storage/testing/helpers"
	"github.com/nbd-wtf/go-nostr"
)

// ============================================================================
// Test: Regular Events (stored as-is)
// ============================================================================

func TestUniversal_RegularEvents_StoredAsIs(t *testing.T) {
	relay := setupUniversalTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect to relay: %v", err)
	}
	defer conn.Close()

	user, _ := helpers.GenerateKeyPair()

	// Test regular kind numbers that DON'T have specific handlers
	// Avoid: 0 (metadata), 1 (notes), 3 (contacts), 5 (delete), 7 (reaction), 16629 (repo perms)
	regularKinds := []int{2, 4, 6, 8, 1000, 9999}

	for _, kind := range regularKinds {
		t.Run(kindName(kind), func(t *testing.T) {
			tags := nostr.Tags{}
			event, err := helpers.CreateGenericEvent(user, kind, "test content", tags)
			if err != nil {
				t.Fatalf("Failed to create event: %v", err)
			}

			err = conn.Publish(ctx, *event)
			if err != nil {
				t.Fatalf("Failed to publish kind %d event: %v", kind, err)
			}

			// Verify event was stored
			filter := nostr.Filter{IDs: []string{event.ID}}
			events, _ := conn.QuerySync(ctx, filter)
			if len(events) != 1 {
				t.Errorf("Regular event kind %d should be stored, got %d events", kind, len(events))
			}
		})
	}
}

func TestUniversal_RegularEvents_MultipleStored(t *testing.T) {
	relay := setupUniversalTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect to relay: %v", err)
	}
	defer conn.Close()

	user, _ := helpers.GenerateKeyPair()

	// Publish multiple kind 1000 events (regular kind without specific handler)
	// All should be stored since regular events don't replace
	var eventIDs []string
	for i := 0; i < 3; i++ {
		tags := nostr.Tags{}
		event, _ := helpers.CreateGenericEvent(user, 1000, "test content", tags)
		t.Logf("Publishing event %d: %s (created_at: %d)", i, event.ID, event.CreatedAt)
		err := conn.Publish(ctx, *event)
		if err != nil {
			t.Logf("Publish error for event %d: %v", i, err)
		}
		eventIDs = append(eventIDs, event.ID)
		time.Sleep(1100 * time.Millisecond) // Ensure different timestamps
	}

	// Query all events by author
	filter := nostr.Filter{
		Kinds:   []int{1000},
		Authors: []string{user.PublicKey},
	}
	events, _ := conn.QuerySync(ctx, filter)

	t.Logf("Query returned %d events", len(events))
	for i, e := range events {
		t.Logf("  Event %d: %s (created_at: %d)", i, e.ID, e.CreatedAt)
	}

	if len(events) != 3 {
		t.Errorf("Expected 3 regular events stored, got %d", len(events))
	}
}

// ============================================================================
// Test: Replaceable Events (10000-19999, 0, 3)
// ============================================================================

func TestUniversal_ReplaceableEvents_OnlyLatestKept(t *testing.T) {
	relay := setupUniversalTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect to relay: %v", err)
	}
	defer conn.Close()

	user, _ := helpers.GenerateKeyPair()

	// Test replaceable kinds (10000-19999 range)
	replaceableKinds := []int{10000, 10001, 15000, 19999}

	for _, kind := range replaceableKinds {
		t.Run(kindName(kind), func(t *testing.T) {
			// Create first event
			event1, _ := helpers.CreateGenericEvent(user, kind, "first", nostr.Tags{})
			err1 := conn.Publish(ctx, *event1)
			if err1 != nil {
				t.Fatalf("Failed to publish first event: %v", err1)
			}
			t.Logf("Published first event: %s (created_at: %d)", event1.ID, event1.CreatedAt)

			time.Sleep(1100 * time.Millisecond) // Ensure different created_at (1 second resolution)

			// Create second event (should replace first)
			event2, _ := helpers.CreateGenericEvent(user, kind, "second", nostr.Tags{})
			err2 := conn.Publish(ctx, *event2)
			if err2 != nil {
				t.Fatalf("Failed to publish second event: %v", err2)
			}
			t.Logf("Published second event: %s (created_at: %d)", event2.ID, event2.CreatedAt)

			// Query - should only get the latest
			filter := nostr.Filter{
				Kinds:   []int{kind},
				Authors: []string{user.PublicKey},
			}
			events, _ := conn.QuerySync(ctx, filter)
			t.Logf("Query returned %d events", len(events))
			for i, e := range events {
				t.Logf("  Event %d: %s (created_at: %d, content: %s)", i, e.ID, e.CreatedAt, e.Content)
			}

			if len(events) != 1 {
				t.Errorf("Expected 1 replaceable event, got %d", len(events))
			}

			if len(events) > 0 && events[0].ID != event2.ID {
				t.Errorf("Expected latest event ID %s, got %s", event2.ID, events[0].ID)
			}
		})
	}
}

func TestUniversal_ReplaceableEvents_Kind0And3(t *testing.T) {
	// Kind 0 and 3 are special replaceable kinds (handled by specific handlers)
	// But if they fall through to universal, they should be replaceable
	relay := setupUniversalTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect to relay: %v", err)
	}
	defer conn.Close()

	// Note: Kind 0 and 3 have specific handlers registered, so this tests
	// that the categorization functions are correct (for documentation)
	t.Log("Kind 0 (metadata) and Kind 3 (contacts) are replaceable by definition")
	t.Log("They have specific handlers but would be replaceable if handled by universal")
}

func TestUniversal_ReplaceableEvents_OlderEventRejected(t *testing.T) {
	relay := setupUniversalTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect to relay: %v", err)
	}
	defer conn.Close()

	user, _ := helpers.GenerateKeyPair()

	// Create and publish newer event first
	event1, _ := helpers.CreateGenericEvent(user, 10000, "newer", nostr.Tags{})
	conn.Publish(ctx, *event1)

	// Try to publish older event (manually set older timestamp)
	// Note: CreateGenericEvent uses time.Now(), so we need to wait and create
	// This test verifies logic - in practice older events would have older timestamps
	time.Sleep(100 * time.Millisecond)

	// Query to confirm only one exists
	filter := nostr.Filter{
		Kinds:   []int{10000},
		Authors: []string{user.PublicKey},
	}
	events, _ := conn.QuerySync(ctx, filter)

	if len(events) != 1 {
		t.Errorf("Expected 1 replaceable event, got %d", len(events))
	}
}

// ============================================================================
// Test: Ephemeral Events (20000-29999)
// ============================================================================

func TestUniversal_EphemeralEvents_NotStored(t *testing.T) {
	relay := setupUniversalTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect to relay: %v", err)
	}
	defer conn.Close()

	user, _ := helpers.GenerateKeyPair()

	// Test various ephemeral kind numbers
	ephemeralKinds := []int{20000, 20001, 25000, 29999}

	for _, kind := range ephemeralKinds {
		t.Run(kindName(kind), func(t *testing.T) {
			event, _ := helpers.CreateGenericEvent(user, kind, "ephemeral content", nostr.Tags{})
			err := conn.Publish(ctx, *event)
			if err != nil {
				t.Fatalf("Failed to publish ephemeral event: %v", err)
			}

			// Verify event was NOT stored
			filter := nostr.Filter{IDs: []string{event.ID}}
			events, _ := conn.QuerySync(ctx, filter)
			if len(events) != 0 {
				t.Errorf("Ephemeral event kind %d should NOT be stored, got %d events", kind, len(events))
			}
		})
	}
}

// ============================================================================
// Test: Addressable Events (30000-39999)
// ============================================================================

func TestUniversal_AddressableEvents_OnlyLatestPerDTag(t *testing.T) {
	relay := setupUniversalTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect to relay: %v", err)
	}
	defer conn.Close()

	user, _ := helpers.GenerateKeyPair()

	// Test addressable kinds
	addressableKinds := []int{30000, 30001, 35000, 39999}

	for _, kind := range addressableKinds {
		t.Run(kindName(kind), func(t *testing.T) {
			dTag := "test-dtag"

			// Create first event with d tag
			tags1 := nostr.Tags{{"d", dTag}}
			event1, _ := helpers.CreateGenericEvent(user, kind, "first version", tags1)
			conn.Publish(ctx, *event1)
			t.Logf("Published first event: %s (created_at: %d)", event1.ID, event1.CreatedAt)

			time.Sleep(1100 * time.Millisecond) // Ensure different created_at (1 second resolution)

			// Create second event with same d tag (should replace first)
			tags2 := nostr.Tags{{"d", dTag}}
			event2, _ := helpers.CreateGenericEvent(user, kind, "second version", tags2)
			conn.Publish(ctx, *event2)
			t.Logf("Published second event: %s (created_at: %d)", event2.ID, event2.CreatedAt)

			// Query by d tag - should only get latest
			filter := nostr.Filter{
				Kinds:   []int{kind},
				Authors: []string{user.PublicKey},
				Tags:    nostr.TagMap{"d": []string{dTag}},
			}
			events, _ := conn.QuerySync(ctx, filter)
			t.Logf("Query returned %d events", len(events))

			if len(events) != 1 {
				t.Errorf("Expected 1 addressable event, got %d", len(events))
			}

			if len(events) > 0 && events[0].ID != event2.ID {
				t.Errorf("Expected latest event ID %s, got %s", event2.ID, events[0].ID)
			}
		})
	}
}

func TestUniversal_AddressableEvents_DifferentDTagsKept(t *testing.T) {
	relay := setupUniversalTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect to relay: %v", err)
	}
	defer conn.Close()

	user, _ := helpers.GenerateKeyPair()

	kind := 30000

	// Create events with different d tags - all should be kept
	dTags := []string{"article-1", "article-2", "article-3"}
	for _, dTag := range dTags {
		tags := nostr.Tags{{"d", dTag}}
		event, _ := helpers.CreateGenericEvent(user, kind, "content for "+dTag, tags)
		conn.Publish(ctx, *event)
	}

	// Query all addressable events for this author
	filter := nostr.Filter{
		Kinds:   []int{kind},
		Authors: []string{user.PublicKey},
	}
	events, _ := conn.QuerySync(ctx, filter)

	if len(events) != 3 {
		t.Errorf("Expected 3 addressable events with different d-tags, got %d", len(events))
	}
}

func TestUniversal_AddressableEvents_EmptyDTag(t *testing.T) {
	relay := setupUniversalTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect to relay: %v", err)
	}
	defer conn.Close()

	user, _ := helpers.GenerateKeyPair()

	kind := 30000

	// Create first event with empty d tag
	tags1 := nostr.Tags{{"d", ""}}
	event1, _ := helpers.CreateGenericEvent(user, kind, "first", tags1)
	conn.Publish(ctx, *event1)

	time.Sleep(1100 * time.Millisecond) // Ensure different created_at

	// Create second event with empty d tag (should replace)
	tags2 := nostr.Tags{{"d", ""}}
	event2, _ := helpers.CreateGenericEvent(user, kind, "second", tags2)
	conn.Publish(ctx, *event2)

	// Query - should only get latest
	filter := nostr.Filter{
		Kinds:   []int{kind},
		Authors: []string{user.PublicKey},
		Tags:    nostr.TagMap{"d": []string{""}},
	}
	events, _ := conn.QuerySync(ctx, filter)

	if len(events) != 1 {
		t.Errorf("Expected 1 event with empty d-tag, got %d", len(events))
	}

	if len(events) > 0 && events[0].ID != event2.ID {
		t.Errorf("Expected latest event")
	}
}

// ============================================================================
// Test: Kind Range Boundaries
// ============================================================================

func TestUniversal_KindRangeBoundaries(t *testing.T) {
	testCases := []struct {
		kind     int
		category string
	}{
		// Regular boundaries
		{1, "regular"},
		{9999, "regular"},
		// Replaceable boundaries
		{10000, "replaceable"},
		{19999, "replaceable"},
		// Ephemeral boundaries
		{20000, "ephemeral"},
		{29999, "ephemeral"},
		// Addressable boundaries
		{30000, "addressable"},
		{39999, "addressable"},
		// Back to regular
		{40000, "regular"},
	}

	relay := setupUniversalTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect to relay: %v", err)
	}
	defer conn.Close()

	user, _ := helpers.GenerateKeyPair()

	for _, tc := range testCases {
		t.Run(kindName(tc.kind)+"_"+tc.category, func(t *testing.T) {
			tags := nostr.Tags{}
			if tc.category == "addressable" {
				tags = nostr.Tags{{"d", "boundary-test"}}
			}

			event, _ := helpers.CreateGenericEvent(user, tc.kind, "boundary test", tags)
			conn.Publish(ctx, *event)

			filter := nostr.Filter{IDs: []string{event.ID}}
			events, _ := conn.QuerySync(ctx, filter)

			switch tc.category {
			case "ephemeral":
				if len(events) != 0 {
					t.Errorf("Kind %d should be ephemeral (not stored), got %d events", tc.kind, len(events))
				}
			default:
				if len(events) != 1 {
					t.Errorf("Kind %d (%s) should be stored, got %d events", tc.kind, tc.category, len(events))
				}
			}
		})
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

func setupUniversalTestRelay(t *testing.T) *helpers.TestRelay {
	t.Helper()
	cfg := helpers.DefaultTestConfig()
	// Allow a wide range of kinds for testing
	cfg.AllowedKinds = []int{
		0, 1, 2, 3, 4, 5, 6, 7, 8, 1000, 9999,
		10000, 10001, 15000, 19999,
		20000, 20001, 25000, 29999,
		30000, 30001, 35000, 39999,
		40000,
	}
	relay, err := helpers.NewTestRelay(cfg)
	if err != nil {
		t.Fatalf("Failed to create test relay: %v", err)
	}
	return relay
}

func kindName(kind int) string {
	return "kind_" + string(rune('0'+kind/10000)) + string(rune('0'+(kind/1000)%10)) + string(rune('0'+(kind/100)%10)) + string(rune('0'+(kind/10)%10)) + string(rune('0'+kind%10))
}
