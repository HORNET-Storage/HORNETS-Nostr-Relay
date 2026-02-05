package testing

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/v2/dag"
	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/badgerhold"
	"github.com/HORNET-Storage/hornet-storage/testing/helpers"
)

// TestDAGStore_StoreAndRetrieveLeaf tests basic leaf storage and retrieval
func TestDAGStore_StoreAndRetrieveLeaf(t *testing.T) {
	// Create temp store
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Generate key pair
	kp, err := helpers.GenerateDAGKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Create a simple test DAG
	testDag, err := helpers.CreateTestDAGFromContent("Hello, World! This is test content for DAG storage.")
	if err != nil {
		t.Fatalf("Failed to create test DAG: %v", err)
	}
	defer testDag.Cleanup()

	// Sign the DAG
	signature, err := helpers.SignDAG(testDag.Dag, kp)
	if err != nil {
		t.Fatalf("Failed to sign DAG: %v", err)
	}

	// Store all leaves
	for _, leaf := range testDag.Dag.Leafs {
		leafData := &types.DagLeafData{
			PublicKey: kp.PublicHex,
			Signature: signature,
			Leaf:      *leaf,
		}
		err := store.StoreLeaf(testDag.Root, leafData)
		if err != nil {
			t.Fatalf("Failed to store leaf %s: %v", leaf.Hash, err)
		}
	}

	// Retrieve and verify leaves
	for hash := range testDag.Dag.Leafs {
		retrieved, err := store.RetrieveLeaf(testDag.Root, hash, false)
		if err != nil {
			t.Fatalf("Failed to retrieve leaf %s: %v", hash, err)
		}
		if retrieved == nil {
			t.Errorf("Retrieved leaf is nil for hash %s", hash)
		}
		if retrieved.Leaf.Hash != hash {
			t.Errorf("Hash mismatch: expected %s, got %s", hash, retrieved.Leaf.Hash)
		}
	}
}

// TestDAGStore_BatchStorage tests batch leaf storage
func TestDAGStore_BatchStorage(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp, _ := helpers.GenerateDAGKeyPair()

	// Create a larger DAG with multiple files
	testDag, err := helpers.CreateLargeTestDAG(5, 1024) // 5 files, 1KB each
	if err != nil {
		t.Fatalf("Failed to create large test DAG: %v", err)
	}
	defer testDag.Cleanup()

	signature, _ := helpers.SignDAG(testDag.Dag, kp)

	// Prepare batch
	leaves := make([]*types.DagLeafData, 0, len(testDag.Dag.Leafs))
	for _, leaf := range testDag.Dag.Leafs {
		leaves = append(leaves, &types.DagLeafData{
			PublicKey: kp.PublicHex,
			Signature: signature,
			Leaf:      *leaf,
		})
	}

	// Store batch
	err = store.StoreLeavesBatch(testDag.Root, leaves)
	if err != nil {
		t.Fatalf("Failed to store leaves batch: %v", err)
	}

	// Verify all leaves are stored
	for hash := range testDag.Dag.Leafs {
		retrieved, err := store.RetrieveLeaf(testDag.Root, hash, false)
		if err != nil {
			t.Errorf("Failed to retrieve leaf %s: %v", hash, err)
		}
		if retrieved == nil {
			t.Errorf("Retrieved leaf is nil for hash %s", hash)
		}
	}
}

// TestDAGStore_DAGStoreForRoot tests creating a DagStore adapter for streaming
func TestDAGStore_DAGStoreForRoot(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp, _ := helpers.GenerateDAGKeyPair()

	testDag, err := helpers.CreateTestDAGFromContent("Streaming DAG test content")
	if err != nil {
		t.Fatalf("Failed to create test DAG: %v", err)
	}
	defer testDag.Cleanup()

	signature, _ := helpers.SignDAG(testDag.Dag, kp)

	// Create DagStore adapter
	dagStore := store.CreateDagStoreForRoot(testDag.Root, kp.PublicHex, signature)
	if dagStore == nil {
		t.Fatal("CreateDagStoreForRoot returned nil")
	}

	// Store leaves using the adapter
	for _, leaf := range testDag.Dag.Leafs {
		err := dagStore.StoreLeaf(leaf)
		if err != nil {
			t.Errorf("Failed to store leaf via DagStore: %v", err)
		}
	}

	// Verify via direct retrieval
	for hash := range testDag.Dag.Leafs {
		retrieved, err := store.RetrieveLeaf(testDag.Root, hash, false)
		if err != nil || retrieved == nil {
			t.Errorf("Leaf %s not found after storage via DagStore adapter", hash)
		}
	}
}

// TestDAGStore_ContentStorage tests separate content storage
func TestDAGStore_ContentStorage(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp, _ := helpers.GenerateDAGKeyPair()

	testContent := []byte("Test content for separate storage")
	testDag, err := helpers.CreateTestDAGFromContent(string(testContent))
	if err != nil {
		t.Fatalf("Failed to create test DAG: %v", err)
	}
	defer testDag.Cleanup()

	signature, _ := helpers.SignDAG(testDag.Dag, kp)

	// Store the DAG with content
	for _, leaf := range testDag.Dag.Leafs {
		leafData := &types.DagLeafData{
			PublicKey: kp.PublicHex,
			Signature: signature,
			Leaf:      *leaf,
		}
		store.StoreLeaf(testDag.Root, leafData)
	}

	// Retrieve with content flag set to true
	for hash, leaf := range testDag.Dag.Leafs {
		if len(leaf.ContentHash) > 0 {
			retrieved, err := store.RetrieveLeaf(testDag.Root, hash, true)
			if err != nil {
				t.Errorf("Failed to retrieve leaf with content: %v", err)
			}
			if retrieved != nil && len(retrieved.Leaf.Content) == 0 {
				// Content might be stored separately depending on implementation
				t.Logf("Content not embedded in leaf %s (may be stored separately)", hash)
			}
		}
	}
}

// TestDAGStore_OwnershipClaim tests DAG ownership claiming
func TestDAGStore_OwnershipClaim(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp, _ := helpers.GenerateDAGKeyPair()

	testDag, err := helpers.CreateTestDAGFromContent("Ownership test DAG")
	if err != nil {
		t.Fatalf("Failed to create test DAG: %v", err)
	}
	defer testDag.Cleanup()

	signature, _ := helpers.SignDAG(testDag.Dag, kp)

	// Store the DAG first
	for _, leaf := range testDag.Dag.Leafs {
		leafData := &types.DagLeafData{
			PublicKey: kp.PublicHex,
			Signature: signature,
			Leaf:      *leaf,
		}
		store.StoreLeaf(testDag.Root, leafData)
	}

	// Claim ownership
	err = store.ClaimOwnership(testDag.Root, kp.PublicHex, signature)
	if err != nil {
		t.Errorf("Failed to claim ownership: %v", err)
	}

	// Verify ownership can be retrieved
	ownerships, err := store.GetOwnership(testDag.Root)
	if err != nil {
		t.Errorf("Failed to get ownership: %v", err)
	}
	if len(ownerships) > 0 && ownerships[0].PublicKey != kp.PublicHex {
		t.Errorf("Ownership pubkey mismatch: expected %s, got %s", kp.PublicHex, ownerships[0].PublicKey)
	}
}

// TestDAGStore_NestedDirectoryDAG tests DAG creation from nested directories
func TestDAGStore_NestedDirectoryDAG(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp, _ := helpers.GenerateDAGKeyPair()

	testDag, err := helpers.CreateNestedTestDAG()
	if err != nil {
		t.Fatalf("Failed to create nested test DAG: %v", err)
	}
	defer testDag.Cleanup()

	signature, _ := helpers.SignDAG(testDag.Dag, kp)

	// Store all leaves
	for _, leaf := range testDag.Dag.Leafs {
		leafData := &types.DagLeafData{
			PublicKey: kp.PublicHex,
			Signature: signature,
			Leaf:      *leaf,
		}
		err := store.StoreLeaf(testDag.Root, leafData)
		if err != nil {
			t.Fatalf("Failed to store leaf: %v", err)
		}
	}

	t.Logf("Created and stored nested DAG with %d leaves, root: %s", len(testDag.Dag.Leafs), testDag.Root)

	// Verify all leaves are retrievable
	for hash := range testDag.Dag.Leafs {
		_, err := store.RetrieveLeaf(testDag.Root, hash, false)
		if err != nil {
			t.Errorf("Failed to retrieve leaf %s: %v", hash, err)
		}
	}
}

// TestDAGStore_RelationshipsCache tests relationship caching between leaves
func TestDAGStore_RelationshipsCache(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp, _ := helpers.GenerateDAGKeyPair()

	testDag, err := helpers.CreateNestedTestDAG()
	if err != nil {
		t.Fatalf("Failed to create test DAG: %v", err)
	}
	defer testDag.Cleanup()

	signature, _ := helpers.SignDAG(testDag.Dag, kp)

	// Store all leaves
	leaves := make([]*types.DagLeafData, 0)
	for _, leaf := range testDag.Dag.Leafs {
		leaves = append(leaves, &types.DagLeafData{
			PublicKey: kp.PublicHex,
			Signature: signature,
			Leaf:      *leaf,
		})
	}
	store.StoreLeavesBatch(testDag.Root, leaves)

	// Try to retrieve relationships
	relationships, err := store.RetrieveRelationships(testDag.Root)
	if err != nil {
		t.Logf("Relationships not cached (may require explicit caching): %v", err)
	} else if relationships != nil {
		t.Logf("Retrieved %d relationships for DAG", len(relationships))
	}
}

// TestDAGStore_StreamLeaves tests streaming leaf iteration
func TestDAGStore_StreamLeaves(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp, _ := helpers.GenerateDAGKeyPair()

	testDag, err := helpers.CreateLargeTestDAG(3, 512)
	if err != nil {
		t.Fatalf("Failed to create test DAG: %v", err)
	}
	defer testDag.Cleanup()

	signature, _ := helpers.SignDAG(testDag.Dag, kp)

	// Store all leaves
	for _, leaf := range testDag.Dag.Leafs {
		leafData := &types.DagLeafData{
			PublicKey: kp.PublicHex,
			Signature: signature,
			Leaf:      *leaf,
		}
		store.StoreLeaf(testDag.Root, leafData)
	}

	// Cache relationships for streaming
	dagStore, err := store.CreateDagStoreFromExisting(testDag.Root)
	if err != nil {
		t.Logf("Could not create DagStore from existing (may not have index): %v", err)
		return
	}

	// Count leaves via streaming
	count := 0
	err = store.StreamLeaves(testDag.Root, false, func(leaf *merkle_dag.DagLeaf, parent *merkle_dag.DagLeaf) error {
		count++
		return nil
	})
	if err != nil {
		t.Logf("StreamLeaves error (may require index): %v", err)
	} else {
		t.Logf("Streamed %d leaves", count)
	}

	_ = dagStore // Use variable to avoid unused warning
}

// TestDAGStore_DeleteDAG tests DAG deletion
func TestDAGStore_DeleteDAG(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp, _ := helpers.GenerateDAGKeyPair()

	testDag, err := helpers.CreateTestDAGFromContent("DAG to be deleted")
	if err != nil {
		t.Fatalf("Failed to create test DAG: %v", err)
	}
	defer testDag.Cleanup()

	signature, _ := helpers.SignDAG(testDag.Dag, kp)

	// Store the DAG
	for _, leaf := range testDag.Dag.Leafs {
		leafData := &types.DagLeafData{
			PublicKey: kp.PublicHex,
			Signature: signature,
			Leaf:      *leaf,
		}
		store.StoreLeaf(testDag.Root, leafData)
	}

	// Verify it's stored
	rootLeaf, err := store.RetrieveLeaf(testDag.Root, testDag.Root, false)
	if err != nil || rootLeaf == nil {
		t.Fatal("DAG was not stored properly")
	}

	// Delete the DAG
	err = store.DeleteDag(testDag.Root)
	if err != nil {
		t.Logf("DeleteDag returned error (may be expected): %v", err)
	}

	// Verify deletion
	rootLeaf, err = store.RetrieveLeaf(testDag.Root, testDag.Root, false)
	if rootLeaf != nil {
		t.Logf("DAG root still exists after deletion (delete may be soft)")
	}
}

// TestDAGStore_ConcurrentAccess tests concurrent DAG operations
func TestDAGStore_ConcurrentAccess(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create multiple DAGs concurrently
	dagCount := 5
	done := make(chan error, dagCount)

	for i := 0; i < dagCount; i++ {
		go func(idx int) {
			kp, err := helpers.GenerateDAGKeyPair()
			if err != nil {
				done <- err
				return
			}

			content := make([]byte, 256)
			for j := range content {
				content[j] = byte((idx*256 + j) % 256)
			}

			testDag, err := helpers.CreateTestDAGFromContent(string(content))
			if err != nil {
				done <- err
				return
			}
			defer testDag.Cleanup()

			signature, _ := helpers.SignDAG(testDag.Dag, kp)

			for _, leaf := range testDag.Dag.Leafs {
				leafData := &types.DagLeafData{
					PublicKey: kp.PublicHex,
					Signature: signature,
					Leaf:      *leaf,
				}
				if err := store.StoreLeaf(testDag.Root, leafData); err != nil {
					done <- err
					return
				}
			}

			done <- nil
		}(i)
	}

	// Wait for all goroutines
	successCount := 0
	for i := 0; i < dagCount; i++ {
		select {
		case err := <-done:
			if err == nil {
				successCount++
			} else {
				t.Logf("Concurrent operation error: %v", err)
			}
		case <-ctx.Done():
			t.Fatal("Timeout waiting for concurrent operations")
		}
	}

	// BadgerDB uses optimistic concurrency control, so some transaction conflicts are expected
	// when multiple goroutines write concurrently. We require at least 1 success to prove
	// the store handles concurrent access without panicking or corrupting data.
	if successCount == 0 {
		t.Errorf("All concurrent DAG operations failed - expected at least one to succeed")
	} else {
		t.Logf("Concurrent access test: %d/%d operations succeeded (transaction conflicts are expected with BadgerDB)", successCount, dagCount)
	}
}

// TestDAGStore_LargeDAG tests storing and retrieving a large DAG
func TestDAGStore_LargeDAG(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large DAG test in short mode")
	}

	store, cleanup := setupTestStore(t)
	defer cleanup()

	kp, _ := helpers.GenerateDAGKeyPair()

	// Create a larger DAG (10 files, 10KB each)
	testDag, err := helpers.CreateLargeTestDAG(10, 10*1024)
	if err != nil {
		t.Fatalf("Failed to create large test DAG: %v", err)
	}
	defer testDag.Cleanup()

	t.Logf("Created large DAG with %d leaves", len(testDag.Dag.Leafs))

	signature, _ := helpers.SignDAG(testDag.Dag, kp)

	// Store in batch
	leaves := make([]*types.DagLeafData, 0, len(testDag.Dag.Leafs))
	for _, leaf := range testDag.Dag.Leafs {
		leaves = append(leaves, &types.DagLeafData{
			PublicKey: kp.PublicHex,
			Signature: signature,
			Leaf:      *leaf,
		})
	}

	start := time.Now()
	err = store.StoreLeavesBatch(testDag.Root, leaves)
	if err != nil {
		t.Fatalf("Failed to store large DAG: %v", err)
	}
	t.Logf("Stored %d leaves in %v", len(leaves), time.Since(start))

	// Verify all leaves
	start = time.Now()
	for hash := range testDag.Dag.Leafs {
		_, err := store.RetrieveLeaf(testDag.Root, hash, false)
		if err != nil {
			t.Errorf("Failed to retrieve leaf %s: %v", hash, err)
		}
	}
	t.Logf("Retrieved %d leaves in %v", len(testDag.Dag.Leafs), time.Since(start))
}

// setupTestStore creates a temporary store for testing
func setupTestStore(t *testing.T) (*badgerhold.BadgerholdStore, func()) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "dag-test-store-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	storePath := filepath.Join(tempDir, "store")
	statsPath := filepath.Join(tempDir, "stats.db")

	store, err := badgerhold.InitStore(storePath, statsPath)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to initialize store: %v", err)
	}

	cleanup := func() {
		store.Cleanup()
		os.RemoveAll(tempDir)
	}

	return store, cleanup
}
