package access_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/HORNET-Storage/hornet-storage/lib/access"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/badgerhold"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
	"github.com/nbd-wtf/go-nostr"
)

func TestCanWriteEventAllowsRepoCollaboratorInInviteOnlyMode(t *testing.T) {
	store := newAccessTestStore(t)
	defer store.Cleanup()

	owner := newAccessTestPubkey(t)
	maintainer := newAccessTestPubkey(t)
	triage := newAccessTestPubkey(t)
	stranger := newAccessTestPubkey(t)
	repoID := "11111111-1111-1111-1111-111111111111"

	permissionEvent := &nostr.Event{
		ID:        accessTestEventID(1),
		PubKey:    owner,
		CreatedAt: nostr.Now(),
		Kind:      16629,
		Tags: nostr.Tags{
			{"r", repoID},
			{"p", maintainer, "maintainer"},
			{"p", triage, "triage"},
		},
	}
	if err := store.StoreEvent(permissionEvent); err != nil {
		t.Fatalf("StoreEvent: %v", err)
	}

	accessControl := access.NewAccessControl(store.GetStatsStore(), &types.AllowedUsersSettings{
		Mode:                    "invite-only",
		Read:                    "all_users",
		Write:                   "allowed_users",
		RepoAccessOverrideKinds: []int{73},
	})

	if err := accessControl.CanWriteEvent(&nostr.Event{PubKey: maintainer, Kind: 73, Tags: nostr.Tags{{"r", repoID}}}, store); err != nil {
		t.Fatalf("expected maintainer to be allowed for repo event: %v", err)
	}

	if err := accessControl.CanWriteEvent(&nostr.Event{PubKey: triage, Kind: 73, Tags: nostr.Tags{{"r", repoID}}}, store); err != nil {
		t.Fatalf("expected triage collaborator to be allowed for repo event: %v", err)
	}

	if err := accessControl.CanWriteEvent(&nostr.Event{PubKey: maintainer, Kind: 1, Tags: nostr.Tags{{"r", repoID}}}, store); err == nil {
		t.Fatal("expected unconfigured kind to be denied")
	}

	if err := accessControl.CanWriteEvent(&nostr.Event{PubKey: maintainer, Kind: 73}, store); err == nil {
		t.Fatal("expected repo event without r tag to be denied")
	}

	if err := accessControl.CanWriteEvent(&nostr.Event{PubKey: stranger, Kind: 73, Tags: nostr.Tags{{"r", repoID}}}, store); err == nil {
		t.Fatal("expected pubkey without repo permission to be denied")
	}
}

func newAccessTestStore(t *testing.T) *badgerhold.BadgerholdStore {
	t.Helper()

	tempDir := t.TempDir()
	store, err := badgerhold.InitStore(filepath.Join(tempDir, "store"), filepath.Join(tempDir, "stats.db"))
	if err != nil {
		t.Fatalf("InitStore: %v", err)
	}

	return store
}

func newAccessTestPubkey(t *testing.T) string {
	t.Helper()

	publicKey, err := nostr.GetPublicKey(nostr.GeneratePrivateKey())
	if err != nil {
		t.Fatalf("GetPublicKey: %v", err)
	}
	return publicKey
}

func accessTestEventID(sequence int) string {
	return fmt.Sprintf("%064x", sequence)
}
