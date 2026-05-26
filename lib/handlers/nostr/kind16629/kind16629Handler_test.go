package kind16629

import (
	"fmt"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/badgerhold"
)

type testKeyPair struct {
	privateKey string
	publicKey  string
}

func TestKind16629AutoAddsRepoCollaboratorsToRelayWriteAccess(t *testing.T) {
	store := newKind16629TestStore(t)
	defer store.Cleanup()

	owner := newTestKeyPair(t)
	writeCollaborator := newTestKeyPair(t)
	maintainer := newTestKeyPair(t)
	triageCollaborator := newTestKeyPair(t)
	otherOwner := newTestKeyPair(t)
	otherCollaborator := newTestKeyPair(t)

	statsStore := store.GetStatsStore()
	if err := statsStore.SetRelayOwner(owner.publicKey, "test"); err != nil {
		t.Fatalf("SetRelayOwner: %v", err)
	}

	configureKind16629AutoAddTest(t, true)
	settings, err := config.GetAllowedUsersSettings()
	if err != nil {
		t.Fatalf("GetAllowedUsersSettings: %v", err)
	}
	if !settings.AutoAddRepoCollaborators || settings.Mode != "invite-only" || settings.Write != "allowed_users" {
		t.Fatalf("unexpected allowed_users settings: %+v", settings)
	}

	runKind16629Handler(t, store, newKind16629TestEvent(t, owner, owner.publicKey+":autoadd", "autoadd",
		nostr.Tag{"p", writeCollaborator.publicKey, "write"},
		nostr.Tag{"p", maintainer.publicKey, "maintainer"},
		nostr.Tag{"p", triageCollaborator.publicKey, "triage"},
	))

	waitForAllowedUser(t, store, writeCollaborator.publicKey)
	waitForAllowedUser(t, store, maintainer.publicKey)
	assertAllowedUserAbsentFor(t, store, triageCollaborator.publicKey, 250*time.Millisecond)

	runKind16629Handler(t, store, newKind16629TestEvent(t, owner, owner.publicKey+":autoadd", "autoadd",
		nostr.Tag{"p", writeCollaborator.publicKey, "triage"},
	))

	waitForAllowedUser(t, store, writeCollaborator.publicKey)

	runKind16629Handler(t, store, newKind16629TestEvent(t, otherOwner, otherOwner.publicKey+":other", "other",
		nostr.Tag{"p", otherCollaborator.publicKey, "write"},
	))

	assertAllowedUserAbsentFor(t, store, otherCollaborator.publicKey, 250*time.Millisecond)
}

func newKind16629TestStore(t *testing.T) *badgerhold.BadgerholdStore {
	t.Helper()

	tempDir := t.TempDir()
	store, err := badgerhold.InitStore(filepath.Join(tempDir, "store"), filepath.Join(tempDir, "stats.db"))
	if err != nil {
		t.Fatalf("InitStore: %v", err)
	}

	return store
}

func configureKind16629AutoAddTest(t *testing.T, enabled bool) {
	t.Helper()

	viper.Reset()
	viper.Set("event_filtering.registered_kinds", []int{16629})
	viper.Set("event_filtering.kind_whitelist", []string{"kind16629"})
	viper.Set("event_filtering.allow_unregistered_kinds", false)
	viper.Set("allowed_users.mode", "invite-only")
	viper.Set("allowed_users.read", "all_users")
	viper.Set("allowed_users.write", "allowed_users")
	viper.Set("allowed_users.auto_add_repo_collaborators", enabled)
	viper.Set("allowed_users.last_updated", 0)
	config.InitConfigForTesting()
}

func newTestKeyPair(t *testing.T) testKeyPair {
	t.Helper()

	privateKey := nostr.GeneratePrivateKey()
	publicKey, err := nostr.GetPublicKey(privateKey)
	if err != nil {
		t.Fatalf("GetPublicKey: %v", err)
	}

	return testKeyPair{privateKey: privateKey, publicKey: publicKey}
}

func newKind16629TestEvent(t *testing.T, keyPair testKeyPair, rTag string, repoName string, pTags ...nostr.Tag) *nostr.Event {
	t.Helper()

	cloneURL := fmt.Sprintf("nestr://localhost?id=%s&repo_author=%s&repo_name=%s",
		url.QueryEscape(rTag), url.QueryEscape(keyPair.publicKey), url.QueryEscape(repoName))
	tags := nostr.Tags{
		{"r", rTag},
		{"n", repoName},
		{"clone", cloneURL},
		{"relay", "ws://localhost:9000"},
	}
	tags = append(tags, pTags...)

	event := &nostr.Event{
		PubKey:    keyPair.publicKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      16629,
		Tags:      tags,
		Content:   "",
	}
	if err := event.Sign(keyPair.privateKey); err != nil {
		t.Fatalf("Sign: %v", err)
	}

	return event
}

func runKind16629Handler(t *testing.T, store stores.Store, event *nostr.Event) {
	t.Helper()

	json := jsoniter.ConfigCompatibleWithStandardLibrary
	data, err := json.Marshal(nostr.EventEnvelope{Event: *event})
	if err != nil {
		t.Fatalf("Marshal event envelope: %v", err)
	}

	var responses []struct {
		messageType string
		params      []interface{}
	}
	handler := BuildKind16629Handler(store)
	handler(
		func() ([]byte, error) { return data, nil },
		func(messageType string, params ...interface{}) {
			responses = append(responses, struct {
				messageType string
				params      []interface{}
			}{messageType: messageType, params: params})
		},
	)

	if len(responses) == 0 {
		t.Fatalf("handler produced no response")
	}

	last := responses[len(responses)-1]
	if last.messageType != "OK" || len(last.params) < 3 || last.params[0] != event.ID || last.params[1] != true {
		t.Fatalf("expected OK response for %s, got %#v", event.ID, last)
	}
}

func waitForAllowedUser(t *testing.T, store stores.Store, pubkey string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if user, err := store.GetStatsStore().GetAllowedUser(pubkey); err == nil && user != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("expected %s to be auto-added to allowed users", pubkey)
}

func assertAllowedUserAbsentFor(t *testing.T, store stores.Store, pubkey string, duration time.Duration) {
	t.Helper()

	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		if user, err := store.GetStatsStore().GetAllowedUser(pubkey); err == nil && user != nil {
			t.Fatalf("expected %s not to be auto-added to allowed users", pubkey)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
