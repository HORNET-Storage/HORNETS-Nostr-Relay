package badgerhold

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/nbd-wtf/go-nostr"
)

func TestQueryEventsFallsBackForLegacyMultiCharacterTagQueries(t *testing.T) {
	tempDir := t.TempDir()
	store, err := InitStore(filepath.Join(tempDir, "store"), filepath.Join(tempDir, "stats.db"))
	if err != nil {
		t.Fatalf("InitStore: %v", err)
	}
	defer func() {
		if cleanupErr := store.Cleanup(); cleanupErr != nil {
			t.Fatalf("Cleanup: %v", cleanupErr)
		}
	}()

	privateKey := nostr.GeneratePrivateKey()
	publicKey, err := nostr.GetPublicKey(privateKey)
	if err != nil {
		t.Fatalf("GetPublicKey: %v", err)
	}

	bundleRoot := "bafireieolfptoisxqb4ghgqimimt6v75igt7pqdnl3plibmz3n2fwbpge4"
	event := &nostr.Event{
		PubKey:    publicKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      73,
		Tags: nostr.Tags{
			{"r", "44444444-4444-4444-4444-444444444444"},
			{"bundle", bundleRoot},
		},
		Content: "push event",
	}
	if err := event.Sign(privateKey); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := store.StoreEvent(event); err != nil {
		t.Fatalf("StoreEvent: %v", err)
	}

	legacyTagIndexKey := []byte(fmt.Sprintf("tag:bundle:%s\x00%016x:%s", bundleRoot, uint64(event.CreatedAt), event.ID))
	if err := store.Database.Badger().Update(func(tx *badger.Txn) error {
		return tx.Delete(legacyTagIndexKey)
	}); err != nil {
		t.Fatalf("Delete legacy bundle tag index: %v", err)
	}

	events, err := store.QueryEvents(nostr.Filter{
		Kinds: []int{73},
		Tags: nostr.TagMap{
			"bundle": []string{bundleRoot},
		},
		Limit: 1,
	})
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event from legacy multi-character tag query, got %d", len(events))
	}
	if events[0].ID != event.ID {
		t.Fatalf("expected event %s, got %s", event.ID, events[0].ID)
	}
}
