package access_test

import (
	"encoding/hex"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/HORNET-Storage/hdk-nostr-go/lib/signing"
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
		Kind:      31415,
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
		RepoAccessOverrideKinds: []int{73, 16630},
	})

	if err := accessControl.CanWriteEvent(&nostr.Event{PubKey: maintainer, Kind: 73, Tags: nostr.Tags{{"r", repoID}}}, store); err != nil {
		t.Fatalf("expected maintainer to be allowed for repo event: %v", err)
	}

	if err := accessControl.CanWriteEvent(&nostr.Event{PubKey: triage, Kind: 16630, Tags: nostr.Tags{{"r", repoID}}}, store); err != nil {
		t.Fatalf("expected triage collaborator to be allowed for repo metadata event: %v", err)
	}

	if err := accessControl.CanWriteEvent(&nostr.Event{PubKey: triage, Kind: 73, Tags: nostr.Tags{{"r", repoID}}}, store); err == nil {
		t.Fatal("expected triage collaborator to be denied for DAG-writing repo event")
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

func TestCanWriteRequiresWriteCapableAllowedUser(t *testing.T) {
	store := newAccessTestStore(t)
	defer store.Cleanup()

	readOnlyUser := newAccessTestPubkey(t)
	writer := newAccessTestPubkey(t)

	if err := store.GetStatsStore().AddAllowedUser(readOnlyUser, false, "", "test"); err != nil {
		t.Fatalf("AddAllowedUser(read-only): %v", err)
	}
	if err := store.GetStatsStore().AddAllowedUser(writer, true, "", "test"); err != nil {
		t.Fatalf("AddAllowedUser(writer): %v", err)
	}

	accessControl := access.NewAccessControl(store.GetStatsStore(), &types.AllowedUsersSettings{
		Mode:                    "invite-only",
		Read:                    "allowed_users",
		Write:                   "allowed_users",
		RepoAccessOverrideKinds: []int{73, 74, 31415, 30078},
	})

	if err := accessControl.CanRead(readOnlyUser); err != nil {
		t.Fatalf("expected read-only allowed user to keep read access: %v", err)
	}
	if err := accessControl.CanWrite(readOnlyUser); err == nil {
		t.Fatal("expected read-only allowed user to be denied write access")
	}
	if err := accessControl.CanWrite(writer); err != nil {
		t.Fatalf("expected write-capable allowed user to keep write access: %v", err)
	}
}

func TestCanReadEventUsesLatestRepositoryPermissionEvent(t *testing.T) {
	store := newAccessTestStore(t)
	defer store.Cleanup()

	owner := newAccessTestPubkey(t)
	maintainer := newAccessTestPubkey(t)
	repoID := "22222222-2222-2222-2222-222222222222"

	olderPermissionEvent := &nostr.Event{
		ID:        accessTestEventID(10),
		PubKey:    owner,
		CreatedAt: nostr.Timestamp(100),
		Kind:      31415,
		Tags: nostr.Tags{
			{"r", repoID},
			{"visibility", "private"},
		},
	}
	if err := store.StoreEvent(olderPermissionEvent); err != nil {
		t.Fatalf("StoreEvent(olderPermissionEvent): %v", err)
	}

	newerPermissionEvent := &nostr.Event{
		ID:        accessTestEventID(11),
		PubKey:    owner,
		CreatedAt: nostr.Timestamp(200),
		Kind:      31415,
		Tags: nostr.Tags{
			{"r", repoID},
			{"visibility", "private"},
			{"p", maintainer, "maintainer"},
		},
	}
	if err := store.StoreEvent(newerPermissionEvent); err != nil {
		t.Fatalf("StoreEvent(newerPermissionEvent): %v", err)
	}

	accessControl := access.NewAccessControl(store.GetStatsStore(), &types.AllowedUsersSettings{
		Mode:                    "invite-only",
		Read:                    "allowed_users",
		Write:                   "allowed_users",
		RepoAccessOverrideKinds: []int{73, 74, 31415, 30078},
	})

	repoIssueEvent := &nostr.Event{
		ID:        accessTestEventID(12),
		PubKey:    owner,
		CreatedAt: nostr.Timestamp(300),
		Kind:      30078,
		Tags: nostr.Tags{
			{"r", repoID},
			{"d", "/apps/git/repos/22222222-2222-2222-2222-222222222222/issues/33333333-3333-3333-3333-333333333333/title"},
		},
	}

	if err := accessControl.CanReadEvent(repoIssueEvent, maintainer, store); err != nil {
		t.Fatalf("expected maintainer to be allowed to read repo event using latest permission event: %v", err)
	}
}

func TestCanReadDagAllowsMaintainerWhenBundleTagResolvesRepo(t *testing.T) {
	store := newAccessTestStore(t)
	defer store.Cleanup()

	ownerPriv := nostr.GeneratePrivateKey()
	ownerPub, err := nostr.GetPublicKey(ownerPriv)
	if err != nil {
		t.Fatalf("GetPublicKey(owner): %v", err)
	}

	maintainerPriv := nostr.GeneratePrivateKey()
	maintainerPub, err := nostr.GetPublicKey(maintainerPriv)
	if err != nil {
		t.Fatalf("GetPublicKey(maintainer): %v", err)
	}

	repoID := "33333333-3333-3333-3333-333333333333"
	bundleRoot := "bafireieolfptoisxqb4ghgqimimt6v75igt7pqdnl3plibmz3n2fwbpge4"

	permissionEvent := &nostr.Event{
		ID:        accessTestEventID(20),
		PubKey:    ownerPub,
		CreatedAt: nostr.Timestamp(100),
		Kind:      31415,
		Tags: nostr.Tags{
			{"r", repoID},
			{"visibility", "private"},
			{"p", maintainerPub, "maintainer"},
		},
	}
	if err := store.StoreEvent(permissionEvent); err != nil {
		t.Fatalf("StoreEvent(permissionEvent): %v", err)
	}

	pushEvent := &nostr.Event{
		ID:        accessTestEventID(21),
		PubKey:    ownerPub,
		CreatedAt: nostr.Timestamp(101),
		Kind:      73,
		Tags: nostr.Tags{
			{"r", repoID},
			{"bundle", bundleRoot},
		},
	}
	if err := store.StoreEvent(pushEvent); err != nil {
		t.Fatalf("StoreEvent(pushEvent): %v", err)
	}

	privateKey, _, err := signing.DeserializePrivateKey(maintainerPriv)
	if err != nil {
		t.Fatalf("DeserializePrivateKey: %v", err)
	}
	serializedPubkey, err := signing.SerializePublicKey(privateKey.PubKey())
	if err != nil {
		t.Fatalf("SerializePublicKey: %v", err)
	}
	if *serializedPubkey != maintainerPub {
		t.Fatalf("expected serialized pubkey %s to match nostr pubkey %s", *serializedPubkey, maintainerPub)
	}

	signature, err := signing.SignSerializedCid(bundleRoot, privateKey)
	if err != nil {
		t.Fatalf("SignSerializedCid: %v", err)
	}
	if err := signing.VerifySerializedCIDSignature(signature, bundleRoot, privateKey.PubKey()); err != nil {
		t.Fatalf("VerifySerializedCIDSignature: %v", err)
	}

	accessControl := access.NewAccessControl(store.GetStatsStore(), &types.AllowedUsersSettings{
		Mode:                    "invite-only",
		Read:                    "allowed_users",
		Write:                   "allowed_users",
		RepoAccessOverrideKinds: []int{73, 74, 31415, 30078},
	})

	bundleEvents, err := store.QueryEvents(nostr.Filter{
		Kinds: []int{73},
		Tags: nostr.TagMap{
			"bundle": []string{bundleRoot},
		},
		Limit: 1,
	})
	if err != nil {
		t.Fatalf("QueryEvents(bundle): %v", err)
	}
	if len(bundleEvents) != 1 {
		t.Fatalf("expected 1 bundle event, got %d", len(bundleEvents))
	}

	if err := accessControl.CanReadEvent(&nostr.Event{Kind: 73, Tags: nostr.Tags{{"r", repoID}}}, maintainerPub, store); err != nil {
		t.Fatalf("expected maintainer to be allowed to read repo event directly: %v", err)
	}

	if err := accessControl.CanReadDag(bundleRoot, maintainerPub, hex.EncodeToString(signature.Serialize()), store); err != nil {
		t.Fatalf("expected maintainer to be allowed to read bundle DAG: %v", err)
	}
}

func TestRepositoryReadOverrideDisabledInOnlyMeMode(t *testing.T) {
	store := newAccessTestStore(t)
	defer store.Cleanup()

	owner := newAccessTestPubkey(t)
	repoID := "44444444-4444-4444-4444-444444444444"
	bundleRoot := "bafireihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku"
	readerPriv := nostr.GeneratePrivateKey()
	readerPub, err := nostr.GetPublicKey(readerPriv)
	if err != nil {
		t.Fatalf("GetPublicKey(reader): %v", err)
	}

	permissionEvent := &nostr.Event{
		ID:        accessTestEventID(30),
		PubKey:    owner,
		CreatedAt: nostr.Timestamp(100),
		Kind:      31415,
		Tags: nostr.Tags{
			{"r", repoID},
			{"visibility", "public"},
			{"p", readerPub, "read"},
		},
	}
	if err := store.StoreEvent(permissionEvent); err != nil {
		t.Fatalf("StoreEvent(permissionEvent): %v", err)
	}

	pushEvent := &nostr.Event{
		ID:        accessTestEventID(31),
		PubKey:    owner,
		CreatedAt: nostr.Timestamp(101),
		Kind:      73,
		Tags: nostr.Tags{
			{"r", repoID},
			{"bundle", bundleRoot},
		},
	}
	if err := store.StoreEvent(pushEvent); err != nil {
		t.Fatalf("StoreEvent(pushEvent): %v", err)
	}

	privateKey, _, err := signing.DeserializePrivateKey(readerPriv)
	if err != nil {
		t.Fatalf("DeserializePrivateKey(reader): %v", err)
	}
	signature, err := signing.SignSerializedCid(bundleRoot, privateKey)
	if err != nil {
		t.Fatalf("SignSerializedCid: %v", err)
	}

	accessControl := access.NewAccessControl(store.GetStatsStore(), &types.AllowedUsersSettings{
		Mode:                    "only-me",
		Read:                    "only-me",
		Write:                   "only-me",
		RepoAccessOverrideKinds: []int{73, 74, 31415, 30078},
	})

	if err := accessControl.CanReadEvent(&nostr.Event{Kind: 73, Tags: nostr.Tags{{"r", repoID}}}, readerPub, store); err == nil {
		t.Fatal("expected repo read override to be disabled in only-me mode")
	}

	if err := accessControl.CanReadDag(bundleRoot, readerPub, hex.EncodeToString(signature.Serialize()), store); err == nil {
		t.Fatal("expected DAG read override to be disabled in only-me mode")
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
