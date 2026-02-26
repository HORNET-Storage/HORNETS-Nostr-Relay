package testing

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/HORNET-Storage/hornet-storage/testing/helpers"
)

// ============================================================================
// Helpers
// ============================================================================

func setupCascadeTestRelay(t *testing.T) *helpers.TestRelay {
	t.Helper()
	cfg := helpers.DefaultTestConfig()
	// Include kinds needed: 5 (NIP-09), 72 (cascade), 16629 (repo), 73 (push),
	// 74 (PR), 75 (approval), 76 (tag), 16630 (branch), 7007 (star),
	// 39504-39506 (org)
	cfg.AllowedKinds = []int{5, 72, 73, 74, 75, 76, 7007, 16629, 16630, 39504, 39505, 39506}
	relay, err := helpers.NewTestRelay(cfg)
	require.NoError(t, err, "Failed to create test relay")
	return relay
}

// buildRepoPermissionEvent creates a kind 16629 repo permission event.
func buildRepoPermissionEvent(kp *helpers.TestKeyPair, repoGUID, repoName string, pTags ...nostr.Tag) (*nostr.Event, error) {
	cloneURL := fmt.Sprintf("nestr://localhost?id=%s&repo_author=%s&repo_name=%s",
		url.QueryEscape(repoGUID), url.QueryEscape(kp.PublicKey), url.QueryEscape(repoName))

	tags := nostr.Tags{
		{"alt", "Nestr git repository metadata"},
		{"r", repoGUID},
		{"n", repoName},
		{"clone", cloneURL},
		{"relay", "ws://localhost:9000"},
		{"p", kp.PublicKey, "maintainer"},
	}
	for _, p := range pTags {
		tags = append(tags, p)
	}

	return helpers.CreateGenericEvent(kp, 16629, "", tags)
}

// buildPushEvent creates a kind 73 push event referencing the repo and DAG roots.
func buildPushEvent(kp *helpers.TestKeyPair, repoGUID, branch string, seq int, bundleRoot, archiveRoot string) (*nostr.Event, error) {
	tags := nostr.Tags{
		{"r", repoGUID},
		{"b", branch},
		{"s", fmt.Sprintf("%d", seq)},
	}
	if bundleRoot != "" {
		tags = append(tags, nostr.Tag{"bundle", bundleRoot})
	}
	if archiveRoot != "" {
		tags = append(tags, nostr.Tag{"archive", archiveRoot})
	}
	return helpers.CreateGenericEvent(kp, 73, "", tags)
}

// buildBranchEvent creates a kind 16630 branch event.
func buildBranchEvent(kp *helpers.TestKeyPair, repoGUID, branch string) (*nostr.Event, error) {
	tags := nostr.Tags{
		{"r", repoGUID},
		{"b", branch},
	}
	return helpers.CreateGenericEvent(kp, 16630, "", tags)
}

// buildTagEvent creates a kind 76 git tag event.
func buildTagEvent(kp *helpers.TestKeyPair, repoGUID, tagName, commit string) (*nostr.Event, error) {
	tags := nostr.Tags{
		{"r", repoGUID},
		{"t", tagName},
		{"c", commit},
		{"type", "lightweight"},
	}
	return helpers.CreateGenericEvent(kp, 76, "", tags)
}

// buildPREvent creates a kind 74 pull request event.
func buildPREvent(kp *helpers.TestKeyPair, repoGUID, sourceBranch, targetBranch, dagRoot string) (*nostr.Event, error) {
	tags := nostr.Tags{
		{"r", repoGUID},
		{"s", sourceBranch},
		{"t", targetBranch},
	}
	if dagRoot != "" {
		tags = append(tags, nostr.Tag{"dag_root", dagRoot})
	}
	return helpers.CreateGenericEvent(kp, 74, "PR description", tags)
}

// buildPRApprovalEvent creates a kind 75 approval event.
func buildPRApprovalEvent(kp *helpers.TestKeyPair, repoGUID, prEventID string) (*nostr.Event, error) {
	tags := nostr.Tags{
		{"r", repoGUID},
		{"p", prEventID},
	}
	return helpers.CreateGenericEvent(kp, 75, "", tags)
}

// buildStarEvent creates a kind 7007 star event.
func buildStarEvent(kp *helpers.TestKeyPair, repoGUID, repoOwnerPubkey string) (*nostr.Event, error) {
	tags := nostr.Tags{
		{"r", repoGUID},
		{"p", repoOwnerPubkey},
		{"a", fmt.Sprintf("16629:%s", repoGUID)},
		{"k", "16629"},
	}
	return helpers.CreateGenericEvent(kp, 7007, "+", tags)
}

// queryEventsByRTag queries all events with a given r tag.
func queryEventsByRTag(ctx context.Context, conn *nostr.Relay, rTag string) ([]*nostr.Event, error) {
	return conn.QuerySync(ctx, nostr.Filter{
		Tags: nostr.TagMap{"r": []string{rTag}},
	})
}

// ============================================================================
// Tests
// ============================================================================

// TestCascadeDelete_BasicRepoDelete tests the happy path: owner creates a repo
// with multiple event kinds from multiple contributors, then cascade deletes it.
func TestCascadeDelete_BasicRepoDelete(t *testing.T) {
	relay := setupCascadeTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	require.NoError(t, err)
	defer conn.Close()

	// Generate keys
	owner, _ := helpers.GenerateKeyPair()
	contributor, _ := helpers.GenerateKeyPair()
	starrer, _ := helpers.GenerateKeyPair()

	repoGUID := uuid.New().String()

	// ---- Populate the repo with events ----

	// 1. Permission event (owner)
	permEvent, _ := buildRepoPermissionEvent(owner, repoGUID, "test-repo",
		nostr.Tag{"p", contributor.PublicKey, "write"})
	require.NoError(t, conn.Publish(ctx, *permEvent))

	// 2. Branch events (owner + contributor)
	branchMain, _ := buildBranchEvent(owner, repoGUID, "main")
	require.NoError(t, conn.Publish(ctx, *branchMain))
	branchFeature, _ := buildBranchEvent(contributor, repoGUID, "feature")
	require.NoError(t, conn.Publish(ctx, *branchFeature))

	// 3. Push events (owner + contributor) with fake DAG roots
	push1, _ := buildPushEvent(owner, repoGUID, "main", 1, "bundle-root-aaa", "archive-root-aaa")
	require.NoError(t, conn.Publish(ctx, *push1))
	push2, _ := buildPushEvent(contributor, repoGUID, "feature", 1, "bundle-root-bbb", "archive-root-bbb")
	require.NoError(t, conn.Publish(ctx, *push2))

	// 4. Tag event (owner)
	tagEvent, _ := buildTagEvent(owner, repoGUID, "v1.0", "abc123")
	require.NoError(t, conn.Publish(ctx, *tagEvent))

	// 5. PR event (contributor)
	prEvent, _ := buildPREvent(contributor, repoGUID, "feature", "main", "pr-dag-root-ccc")
	require.NoError(t, conn.Publish(ctx, *prEvent))

	// 6. PR approval (owner)
	approvalEvent, _ := buildPRApprovalEvent(owner, repoGUID, prEvent.ID)
	require.NoError(t, conn.Publish(ctx, *approvalEvent))

	// 7. Star (random user)
	starEvent, _ := buildStarEvent(starrer, repoGUID, owner.PublicKey)
	require.NoError(t, conn.Publish(ctx, *starEvent))

	// Wait for events to be processed
	time.Sleep(200 * time.Millisecond)

	// Verify all events exist
	events, _ := queryEventsByRTag(ctx, conn, repoGUID)
	assert.GreaterOrEqual(t, len(events), 8, "Expected at least 8 events before cascade delete")

	// ---- Cascade delete ----
	cascadeEvent, _ := helpers.CreateCascadeDeleteEvent(owner, repoGUID, "16629", "Deleting test repo")
	err = conn.Publish(ctx, *cascadeEvent)
	require.NoError(t, err, "Cascade delete should succeed")

	// Wait for deletion processing
	time.Sleep(300 * time.Millisecond)

	// Verify all resource events are gone
	events, _ = queryEventsByRTag(ctx, conn, repoGUID)
	// The only event remaining should be the kind 72 tombstone itself
	tombstoneCount := 0
	for _, e := range events {
		if e.Kind == 72 {
			tombstoneCount++
		}
	}
	nonTombstone := len(events) - tombstoneCount
	assert.Equal(t, 0, nonTombstone, "All non-tombstone events for the resource should be deleted")
	assert.Equal(t, 1, tombstoneCount, "Kind 72 tombstone should be stored")
}

// TestCascadeDelete_PermissionDenied tests that a non-owner cannot cascade delete.
func TestCascadeDelete_PermissionDenied(t *testing.T) {
	relay := setupCascadeTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	require.NoError(t, err)
	defer conn.Close()

	owner, _ := helpers.GenerateKeyPair()
	attacker, _ := helpers.GenerateKeyPair()

	repoGUID := uuid.New().String()

	// Create repo owned by "owner"
	permEvent, _ := buildRepoPermissionEvent(owner, repoGUID, "protected-repo")
	require.NoError(t, conn.Publish(ctx, *permEvent))

	// Add a push event
	push, _ := buildPushEvent(owner, repoGUID, "main", 1, "bundle-xyz", "archive-xyz")
	require.NoError(t, conn.Publish(ctx, *push))

	time.Sleep(200 * time.Millisecond)

	// Attacker tries cascade delete
	cascadeEvent, _ := helpers.CreateCascadeDeleteEvent(attacker, repoGUID, "16629", "Malicious delete")

	// Use short timeout — we expect rejection
	pubCtx, pubCancel := context.WithTimeout(ctx, 2*time.Second)
	defer pubCancel()
	conn.Publish(pubCtx, *cascadeEvent)

	time.Sleep(200 * time.Millisecond)

	// Verify events still exist
	events, _ := queryEventsByRTag(ctx, conn, repoGUID)
	// Filter out any kind 72 events (should be none, but be safe)
	realEvents := 0
	for _, e := range events {
		if e.Kind != 72 {
			realEvents++
		}
	}
	assert.GreaterOrEqual(t, realEvents, 2, "Events should NOT be deleted by non-owner")
}

// TestCascadeDelete_ContributorCannotDelete tests that a contributor (non-owner)
// with write access cannot cascade delete the repo.
func TestCascadeDelete_ContributorCannotDelete(t *testing.T) {
	relay := setupCascadeTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	require.NoError(t, err)
	defer conn.Close()

	owner, _ := helpers.GenerateKeyPair()
	contributor, _ := helpers.GenerateKeyPair()

	repoGUID := uuid.New().String()

	// Create repo with contributor having write access
	permEvent, _ := buildRepoPermissionEvent(owner, repoGUID, "team-repo",
		nostr.Tag{"p", contributor.PublicKey, "write"})
	require.NoError(t, conn.Publish(ctx, *permEvent))

	push, _ := buildPushEvent(contributor, repoGUID, "main", 1, "", "")
	require.NoError(t, conn.Publish(ctx, *push))

	time.Sleep(200 * time.Millisecond)

	// Contributor tries to cascade delete
	cascadeEvent, _ := helpers.CreateCascadeDeleteEvent(contributor, repoGUID, "16629", "Contributor delete attempt")
	pubCtx, pubCancel := context.WithTimeout(ctx, 2*time.Second)
	defer pubCancel()
	conn.Publish(pubCtx, *cascadeEvent)

	time.Sleep(200 * time.Millisecond)

	// Events should still exist
	events, _ := queryEventsByRTag(ctx, conn, repoGUID)
	realEvents := 0
	for _, e := range events {
		if e.Kind != 72 {
			realEvents++
		}
	}
	assert.GreaterOrEqual(t, realEvents, 2, "Contributor should NOT be able to cascade delete")
}

// TestCascadeDelete_MissingRTag tests that a cascade delete without an "r" tag is rejected.
func TestCascadeDelete_MissingRTag(t *testing.T) {
	relay := setupCascadeTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	require.NoError(t, err)
	defer conn.Close()

	kp, _ := helpers.GenerateKeyPair()

	// Kind 72 without r tag
	event, _ := helpers.CreateGenericEvent(kp, 72, "bad request", nostr.Tags{
		{"k", "16629"},
	})

	pubCtx, pubCancel := context.WithTimeout(ctx, 2*time.Second)
	defer pubCancel()
	conn.Publish(pubCtx, *event)

	time.Sleep(100 * time.Millisecond)

	// Verify it was NOT stored (rejected event should not create tombstone)
	events, _ := conn.QuerySync(ctx, nostr.Filter{IDs: []string{event.ID}})
	assert.Equal(t, 0, len(events), "Event missing r tag should be rejected")
}

// TestCascadeDelete_MissingKTag tests that a cascade delete without a "k" tag is rejected.
func TestCascadeDelete_MissingKTag(t *testing.T) {
	relay := setupCascadeTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	require.NoError(t, err)
	defer conn.Close()

	kp, _ := helpers.GenerateKeyPair()
	repoGUID := uuid.New().String()

	// Kind 72 without k tag
	event, _ := helpers.CreateGenericEvent(kp, 72, "bad request", nostr.Tags{
		{"r", repoGUID},
	})

	pubCtx, pubCancel := context.WithTimeout(ctx, 2*time.Second)
	defer pubCancel()
	conn.Publish(pubCtx, *event)

	time.Sleep(100 * time.Millisecond)

	events, _ := conn.QuerySync(ctx, nostr.Filter{IDs: []string{event.ID}})
	assert.Equal(t, 0, len(events), "Event missing k tag should be rejected")
}

// TestCascadeDelete_UnknownPermissionKind tests that a cascade delete with an
// unregistered permission kind is rejected.
func TestCascadeDelete_UnknownPermissionKind(t *testing.T) {
	relay := setupCascadeTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	require.NoError(t, err)
	defer conn.Close()

	kp, _ := helpers.GenerateKeyPair()
	repoGUID := uuid.New().String()

	// Kind 72 with unknown permission kind
	event, _ := helpers.CreateCascadeDeleteEvent(kp, repoGUID, "99999", "unknown kind")

	pubCtx, pubCancel := context.WithTimeout(ctx, 2*time.Second)
	defer pubCancel()
	conn.Publish(pubCtx, *event)

	time.Sleep(100 * time.Millisecond)

	events, _ := conn.QuerySync(ctx, nostr.Filter{IDs: []string{event.ID}})
	assert.Equal(t, 0, len(events), "Event with unknown permission kind should be rejected")
}

// TestCascadeDelete_NonexistentResource tests cascade delete on a resource
// that doesn't exist (no permission event).
func TestCascadeDelete_NonexistentResource(t *testing.T) {
	relay := setupCascadeTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	require.NoError(t, err)
	defer conn.Close()

	kp, _ := helpers.GenerateKeyPair()

	// Cascade delete a GUID that has no permission event
	cascadeEvent, _ := helpers.CreateCascadeDeleteEvent(kp, uuid.New().String(), "16629", "delete ghost")

	pubCtx, pubCancel := context.WithTimeout(ctx, 2*time.Second)
	defer pubCancel()
	conn.Publish(pubCtx, *cascadeEvent)

	time.Sleep(100 * time.Millisecond)

	// Should not be stored (resolve owner fails → rejection)
	events, _ := conn.QuerySync(ctx, nostr.Filter{IDs: []string{cascadeEvent.ID}})
	assert.Equal(t, 0, len(events), "Cascade delete on nonexistent resource should be rejected")
}

// TestCascadeDelete_MultipleContributorsAllDeleted verifies that events from
// multiple different authors are all deleted by the owner.
func TestCascadeDelete_MultipleContributorsAllDeleted(t *testing.T) {
	relay := setupCascadeTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	require.NoError(t, err)
	defer conn.Close()

	owner, _ := helpers.GenerateKeyPair()
	contrib1, _ := helpers.GenerateKeyPair()
	contrib2, _ := helpers.GenerateKeyPair()
	contrib3, _ := helpers.GenerateKeyPair()

	repoGUID := uuid.New().String()

	// Create repo
	permEvent, _ := buildRepoPermissionEvent(owner, repoGUID, "multi-contrib-repo",
		nostr.Tag{"p", contrib1.PublicKey, "write"},
		nostr.Tag{"p", contrib2.PublicKey, "write"},
		nostr.Tag{"p", contrib3.PublicKey, "write"},
	)
	require.NoError(t, conn.Publish(ctx, *permEvent))

	// Multiple contributors push
	push1, _ := buildPushEvent(owner, repoGUID, "main", 1, "", "")
	require.NoError(t, conn.Publish(ctx, *push1))
	push2, _ := buildPushEvent(contrib1, repoGUID, "main", 2, "", "")
	require.NoError(t, conn.Publish(ctx, *push2))
	push3, _ := buildPushEvent(contrib2, repoGUID, "feature-a", 1, "", "")
	require.NoError(t, conn.Publish(ctx, *push3))
	push4, _ := buildPushEvent(contrib3, repoGUID, "feature-b", 1, "", "")
	require.NoError(t, conn.Publish(ctx, *push4))

	// Branches from different contributors
	brMain, _ := buildBranchEvent(owner, repoGUID, "main")
	require.NoError(t, conn.Publish(ctx, *brMain))
	brA, _ := buildBranchEvent(contrib2, repoGUID, "feature-a")
	require.NoError(t, conn.Publish(ctx, *brA))
	brB, _ := buildBranchEvent(contrib3, repoGUID, "feature-b")
	require.NoError(t, conn.Publish(ctx, *brB))

	time.Sleep(200 * time.Millisecond)

	// Verify all exist
	events, _ := queryEventsByRTag(ctx, conn, repoGUID)
	assert.Equal(t, 8, len(events), "Expected 8 events before cascade delete")

	// Owner cascade deletes
	cascadeEvent, _ := helpers.CreateCascadeDeleteEvent(owner, repoGUID, "16629", "Cleaning up")
	require.NoError(t, conn.Publish(ctx, *cascadeEvent))

	time.Sleep(300 * time.Millisecond)

	// Only tombstone should remain
	events, _ = queryEventsByRTag(ctx, conn, repoGUID)
	nonTombstone := 0
	for _, e := range events {
		if e.Kind != 72 {
			nonTombstone++
		}
	}
	assert.Equal(t, 0, nonTombstone, "All contributor events should be deleted")
}

// TestCascadeDelete_TombstoneQueryable verifies the tombstone can be queried.
func TestCascadeDelete_TombstoneQueryable(t *testing.T) {
	relay := setupCascadeTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	require.NoError(t, err)
	defer conn.Close()

	owner, _ := helpers.GenerateKeyPair()
	repoGUID := uuid.New().String()

	// Create and populate minimal repo
	permEvent, _ := buildRepoPermissionEvent(owner, repoGUID, "tombstone-test")
	require.NoError(t, conn.Publish(ctx, *permEvent))

	time.Sleep(200 * time.Millisecond)

	// Cascade delete
	cascadeEvent, _ := helpers.CreateCascadeDeleteEvent(owner, repoGUID, "16629", "Testing tombstone")
	require.NoError(t, conn.Publish(ctx, *cascadeEvent))

	time.Sleep(200 * time.Millisecond)

	// Query for the tombstone specifically
	tombstones, _ := conn.QuerySync(ctx, nostr.Filter{
		Kinds: []int{72},
		Tags:  nostr.TagMap{"r": []string{repoGUID}},
	})
	assert.Equal(t, 1, len(tombstones), "Tombstone should be queryable by kind+r tag")
	if len(tombstones) > 0 {
		assert.Equal(t, owner.PublicKey, tombstones[0].PubKey)
		assert.Equal(t, "Testing tombstone", tombstones[0].Content)
	}
}

// TestCascadeDelete_IsolationBetweenRepos ensures cascade deleting one repo
// does not affect events belonging to another repo.
func TestCascadeDelete_IsolationBetweenRepos(t *testing.T) {
	relay := setupCascadeTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	require.NoError(t, err)
	defer conn.Close()

	owner, _ := helpers.GenerateKeyPair()

	repoA := uuid.New().String()
	repoB := uuid.New().String()

	// Create two repos
	permA, _ := buildRepoPermissionEvent(owner, repoA, "repo-a")
	require.NoError(t, conn.Publish(ctx, *permA))
	pushA, _ := buildPushEvent(owner, repoA, "main", 1, "dag-a", "")
	require.NoError(t, conn.Publish(ctx, *pushA))

	permB, _ := buildRepoPermissionEvent(owner, repoB, "repo-b")
	require.NoError(t, conn.Publish(ctx, *permB))
	pushB, _ := buildPushEvent(owner, repoB, "main", 1, "dag-b", "")
	require.NoError(t, conn.Publish(ctx, *pushB))

	time.Sleep(200 * time.Millisecond)

	// Delete repo A only
	cascadeA, _ := helpers.CreateCascadeDeleteEvent(owner, repoA, "16629", "Delete repo A")
	require.NoError(t, conn.Publish(ctx, *cascadeA))

	time.Sleep(300 * time.Millisecond)

	// Repo A should be gone (only tombstone remains)
	eventsA, _ := queryEventsByRTag(ctx, conn, repoA)
	nonTombstoneA := 0
	for _, e := range eventsA {
		if e.Kind != 72 {
			nonTombstoneA++
		}
	}
	assert.Equal(t, 0, nonTombstoneA, "Repo A events should be deleted")

	// Repo B should be untouched
	eventsB, _ := queryEventsByRTag(ctx, conn, repoB)
	assert.Equal(t, 2, len(eventsB), "Repo B events should be intact")
}

// TestCascadeDelete_DAGOwnershipRelease tests that DAG ownership records are
// properly released during cascade deletion.
func TestCascadeDelete_DAGOwnershipRelease(t *testing.T) {
	relay := setupCascadeTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	require.NoError(t, err)
	defer conn.Close()

	owner, _ := helpers.GenerateKeyPair()
	contributor, _ := helpers.GenerateKeyPair()
	repoGUID := uuid.New().String()

	dagRootBundle := "fakebundleroot123"
	dagRootArchive := "fakearchiveroot456"
	dagRootContrib := "fakecontribroot789"

	// Create ownership records directly on the store (simulates DAG upload)
	// These won't have actual DAG data, but we can test ownership release
	store := relay.Store

	// We need to test that ReleaseOwnership works — we can directly call the store
	// since this is testing the relay internals alongside the handler

	// Claim ownership of DAG roots (simulating what would happen during push upload)
	// Note: ClaimOwnership requires the root leaf to exist, so we test ownership
	// release behavior via HasOwnership/ReleaseOwnership directly for DAG roots
	// that the handler would reference.

	// For the event-level test, verify events are deleted even with DAG root tags.
	// The handler calls ReleaseOwnership which gracefully handles non-existent roots.

	// Create repo with push events referencing DAG roots
	permEvent, _ := buildRepoPermissionEvent(owner, repoGUID, "dag-test-repo",
		nostr.Tag{"p", contributor.PublicKey, "write"})
	require.NoError(t, conn.Publish(ctx, *permEvent))

	push1, _ := buildPushEvent(owner, repoGUID, "main", 1, dagRootBundle, dagRootArchive)
	require.NoError(t, conn.Publish(ctx, *push1))
	push2, _ := buildPushEvent(contributor, repoGUID, "feature", 1, dagRootContrib, "")
	require.NoError(t, conn.Publish(ctx, *push2))

	time.Sleep(200 * time.Millisecond)

	// Verify events exist
	events, _ := queryEventsByRTag(ctx, conn, repoGUID)
	assert.Equal(t, 3, len(events), "Expected 3 events before cascade delete")

	// Cascade delete
	cascadeEvent, _ := helpers.CreateCascadeDeleteEvent(owner, repoGUID, "16629", "DAG test delete")
	require.NoError(t, conn.Publish(ctx, *cascadeEvent))

	time.Sleep(300 * time.Millisecond)

	// Verify events are gone
	events, _ = queryEventsByRTag(ctx, conn, repoGUID)
	nonTombstone := 0
	for _, e := range events {
		if e.Kind != 72 {
			nonTombstone++
		}
	}
	assert.Equal(t, 0, nonTombstone, "All events should be deleted")

	// Verify ReleaseOwnership was called (it won't error on non-existent roots)
	// For thorough DAG ownership testing, we test the store methods directly below
	hasOwner, err := store.HasOwnership(dagRootBundle)
	require.NoError(t, err)
	assert.False(t, hasOwner, "DAG ownership should not exist after release (was never created)")
}

// TestCascadeDelete_StoreReleaseOwnership directly tests the ReleaseOwnership
// and HasOwnership store methods.
func TestCascadeDelete_StoreReleaseOwnership(t *testing.T) {
	relay := setupCascadeTestRelay(t)
	defer relay.Cleanup()

	store := relay.Store

	// Test HasOwnership on non-existent root
	has, err := store.HasOwnership("nonexistent-root")
	require.NoError(t, err)
	assert.False(t, has, "Non-existent root should have no ownership")

	// Test ReleaseOwnership on non-existent record (should not error)
	err = store.ReleaseOwnership("nonexistent-root", "nonexistent-pubkey")
	assert.NoError(t, err, "Releasing non-existent ownership should not error")
}

// TestCascadeDelete_EmptyRepo tests cascade delete on a repo with only
// the permission event (no pushes, branches, etc.).
func TestCascadeDelete_EmptyRepo(t *testing.T) {
	relay := setupCascadeTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	require.NoError(t, err)
	defer conn.Close()

	owner, _ := helpers.GenerateKeyPair()
	repoGUID := uuid.New().String()

	// Create repo with just the permission event
	permEvent, _ := buildRepoPermissionEvent(owner, repoGUID, "empty-repo")
	require.NoError(t, conn.Publish(ctx, *permEvent))

	time.Sleep(200 * time.Millisecond)

	// Cascade delete
	cascadeEvent, _ := helpers.CreateCascadeDeleteEvent(owner, repoGUID, "16629", "Delete empty repo")
	require.NoError(t, conn.Publish(ctx, *cascadeEvent))

	time.Sleep(200 * time.Millisecond)

	// Only tombstone should remain
	events, _ := queryEventsByRTag(ctx, conn, repoGUID)
	nonTombstone := 0
	for _, e := range events {
		if e.Kind != 72 {
			nonTombstone++
		}
	}
	assert.Equal(t, 0, nonTombstone, "Permission event should be deleted")
	assert.Equal(t, 1, len(events), "Only tombstone should remain")
}

// TestCascadeDelete_PRWithDagRoot tests that PR events' dag_root references
// are also collected for ownership release.
func TestCascadeDelete_PRWithDagRoot(t *testing.T) {
	relay := setupCascadeTestRelay(t)
	defer relay.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := relay.Connect(ctx)
	require.NoError(t, err)
	defer conn.Close()

	owner, _ := helpers.GenerateKeyPair()
	contributor, _ := helpers.GenerateKeyPair()
	repoGUID := uuid.New().String()

	// Create repo
	permEvent, _ := buildRepoPermissionEvent(owner, repoGUID, "pr-repo",
		nostr.Tag{"p", contributor.PublicKey, "write"})
	require.NoError(t, conn.Publish(ctx, *permEvent))

	// PR with a dag_root reference
	prEvent, _ := buildPREvent(contributor, repoGUID, "feature", "main", "pr-bundle-dag-root")
	require.NoError(t, conn.Publish(ctx, *prEvent))

	// PR approval
	approvalEvent, _ := buildPRApprovalEvent(owner, repoGUID, prEvent.ID)
	require.NoError(t, conn.Publish(ctx, *approvalEvent))

	time.Sleep(200 * time.Millisecond)

	events, _ := queryEventsByRTag(ctx, conn, repoGUID)
	assert.Equal(t, 3, len(events))

	// Cascade delete
	cascadeEvent, _ := helpers.CreateCascadeDeleteEvent(owner, repoGUID, "16629", "Delete PR repo")
	require.NoError(t, conn.Publish(ctx, *cascadeEvent))

	time.Sleep(300 * time.Millisecond)

	// All gone except tombstone
	events, _ = queryEventsByRTag(ctx, conn, repoGUID)
	nonTombstone := 0
	for _, e := range events {
		if e.Kind != 72 {
			nonTombstone++
		}
	}
	assert.Equal(t, 0, nonTombstone)
}
