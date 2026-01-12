package testing

import (
	"context"
	"testing"
	"time"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/v2/dag"
	lib_types "github.com/HORNET-Storage/go-hornet-storage-lib/lib"
	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/connmgr"
	"github.com/HORNET-Storage/hornet-storage/testing/helpers"
)

// TestDAGUpload_SingleSmallFile tests uploading a single small file DAG
func TestDAGUpload_SingleSmallFile(t *testing.T) {
	helpers.RunTestWithFixture(t, "single_small_file", func(t *testing.T, fd *helpers.FixtureDAG) {
		testDAGUpload(t, fd)
	})
}

// TestDAGUpload_SingleLargeFile tests uploading a chunked file DAG
func TestDAGUpload_SingleLargeFile(t *testing.T) {
	helpers.RunTestWithFixture(t, "single_large_file", func(t *testing.T, fd *helpers.FixtureDAG) {
		testDAGUpload(t, fd)
	})
}

// TestDAGUpload_FlatDirectory tests uploading a flat directory DAG
func TestDAGUpload_FlatDirectory(t *testing.T) {
	helpers.RunTestWithFixture(t, "flat_directory", func(t *testing.T, fd *helpers.FixtureDAG) {
		testDAGUpload(t, fd)
	})
}

// TestDAGUpload_NestedDirectory tests uploading a nested directory DAG
func TestDAGUpload_NestedDirectory(t *testing.T) {
	helpers.RunTestWithFixture(t, "nested_directory", func(t *testing.T, fd *helpers.FixtureDAG) {
		testDAGUpload(t, fd)
	})
}

// TestDAGUpload_DeepHierarchy tests uploading a deeply nested DAG
func TestDAGUpload_DeepHierarchy(t *testing.T) {
	helpers.RunTestWithFixture(t, "deep_hierarchy", func(t *testing.T, fd *helpers.FixtureDAG) {
		testDAGUpload(t, fd)
	})
}

// TestDAGUpload_MixedSizes tests uploading a DAG with mixed file sizes
func TestDAGUpload_MixedSizes(t *testing.T) {
	helpers.RunTestWithFixture(t, "mixed_sizes", func(t *testing.T, fd *helpers.FixtureDAG) {
		testDAGUpload(t, fd)
	})
}

// TestDAGUpload_ManySmallFiles tests uploading a DAG with many small files
func TestDAGUpload_ManySmallFiles(t *testing.T) {
	helpers.RunTestWithFixture(t, "many_small_files", func(t *testing.T, fd *helpers.FixtureDAG) {
		testDAGUpload(t, fd)
	})
}

// testDAGUpload is a helper that tests uploading a DAG from a fixture
func testDAGUpload(t *testing.T, fd *helpers.FixtureDAG) {
	// Create test relay
	relay, err := helpers.NewTestLibp2pRelay(helpers.DefaultTestLibp2pConfig())
	if err != nil {
		t.Fatalf("Failed to create test relay: %v", err)
	}
	defer relay.Cleanup()

	if err := relay.WaitForReady(5 * time.Second); err != nil {
		t.Fatalf("Relay not ready: %v", err)
	}

	// Generate key pair for signing
	kp, err := helpers.GenerateDAGKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cm, err := relay.GetConnectionManager(ctx)
	if err != nil {
		t.Fatalf("Failed to get connection manager: %v", err)
	}
	defer cm.Disconnect("test-relay")

	// Upload the DAG
	err = connmgr.UploadDag(ctx, cm, fd.Dag, kp.PrivateKey, nil)
	if err != nil {
		t.Fatalf("Failed to upload DAG: %v", err)
	}

	// Verify DAG exists in store
	exists, err := relay.Store.HasLeafGlobal(fd.Dag.Root)
	if err != nil {
		t.Fatalf("Failed to check if DAG exists: %v", err)
	}
	if !exists {
		t.Error("DAG root not found in store after upload")
	}

	t.Logf("Successfully uploaded %s DAG with root: %s (%d leaves)",
		fd.Fixture.Name, fd.Dag.Root, len(fd.Dag.Leafs))
}

// TestDAGDownload_AllFixtures tests downloading DAGs for all fixtures
func TestDAGDownload_AllFixtures(t *testing.T) {
	helpers.RunTestWithAllFixtures(t, func(t *testing.T, fd *helpers.FixtureDAG) {
		testDAGDownload(t, fd)
	})
}

// testDAGDownload is a helper that tests uploading and downloading a DAG
func testDAGDownload(t *testing.T, fd *helpers.FixtureDAG) {
	relay, err := helpers.NewTestLibp2pRelay(helpers.DefaultTestLibp2pConfig())
	if err != nil {
		t.Fatalf("Failed to create test relay: %v", err)
	}
	defer relay.Cleanup()

	if err := relay.WaitForReady(5 * time.Second); err != nil {
		t.Fatalf("Relay not ready: %v", err)
	}

	kp, err := helpers.GenerateDAGKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cm, err := relay.GetConnectionManager(ctx)
	if err != nil {
		t.Fatalf("Failed to get connection manager: %v", err)
	}
	defer cm.Disconnect("test-relay")

	// Upload the DAG
	err = connmgr.UploadDag(ctx, cm, fd.Dag, kp.PrivateKey, nil)
	if err != nil {
		t.Fatalf("Failed to upload DAG: %v", err)
	}

	// Download the DAG
	_, dagData, err := connmgr.DownloadDag(ctx, cm, "test-relay", fd.Dag.Root, nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to download DAG: %v", err)
	}

	// Verify the downloaded DAG
	if dagData.Dag.Root != fd.Dag.Root {
		t.Errorf("Downloaded DAG root mismatch: got %s, want %s", dagData.Dag.Root, fd.Dag.Root)
	}

	if len(dagData.Dag.Leafs) != len(fd.Dag.Leafs) {
		t.Errorf("Downloaded DAG leaf count mismatch: got %d, want %d",
			len(dagData.Dag.Leafs), len(fd.Dag.Leafs))
	}

	// Verify the downloaded DAG is valid
	if err := dagData.Dag.Verify(); err != nil {
		t.Errorf("Downloaded DAG verification failed: %v", err)
	}

	t.Logf("Successfully downloaded %s DAG (%d leaves)", fd.Fixture.Name, len(dagData.Dag.Leafs))
}

// TestDAGPartialDownload_FlatDirectory tests partial download with flat directory
func TestDAGPartialDownload_FlatDirectory(t *testing.T) {
	helpers.RunTestWithFixture(t, "flat_directory", func(t *testing.T, fd *helpers.FixtureDAG) {
		testDAGPartialDownload(t, fd)
	})
}

// TestDAGPartialDownload_NestedDirectory tests partial download with nested directory
func TestDAGPartialDownload_NestedDirectory(t *testing.T) {
	helpers.RunTestWithFixture(t, "nested_directory", func(t *testing.T, fd *helpers.FixtureDAG) {
		testDAGPartialDownload(t, fd)
	})
}

// TestDAGPartialDownload_DeepHierarchy tests partial download with deep hierarchy
func TestDAGPartialDownload_DeepHierarchy(t *testing.T) {
	helpers.RunTestWithFixture(t, "deep_hierarchy", func(t *testing.T, fd *helpers.FixtureDAG) {
		testDAGPartialDownload(t, fd)
	})
}

// testDAGPartialDownload tests partial download functionality
func testDAGPartialDownload(t *testing.T, fd *helpers.FixtureDAG) {
	relay, err := helpers.NewTestLibp2pRelay(helpers.DefaultTestLibp2pConfig())
	if err != nil {
		t.Fatalf("Failed to create test relay: %v", err)
	}
	defer relay.Cleanup()

	if err := relay.WaitForReady(5 * time.Second); err != nil {
		t.Fatalf("Relay not ready: %v", err)
	}

	kp, err := helpers.GenerateDAGKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cm, err := relay.GetConnectionManager(ctx)
	if err != nil {
		t.Fatalf("Failed to get connection manager: %v", err)
	}
	defer cm.Disconnect("test-relay")

	// Upload the DAG first
	err = connmgr.UploadDag(ctx, cm, fd.Dag, kp.PrivateKey, nil)
	if err != nil {
		t.Fatalf("Failed to upload DAG: %v", err)
	}

	// Get file leaf hashes in deterministic order
	fileHashes := helpers.GetFileLeafHashes(fd.Dag)
	if len(fileHashes) < 2 {
		typeCounts := helpers.GetLeafTypeCount(fd.Dag)
		t.Skipf("DAG has insufficient file leaves for partial download test (need >=2, got %d, types: %v, total leaves: %d)",
			len(fileHashes), typeCounts, len(fd.Dag.Leafs))
	}

	// Request first 2 file leaves (deterministic selection)
	requestedHashes := fileHashes[:2]

	filter := &lib_types.DownloadFilter{
		LeafHashes:     requestedHashes,
		IncludeContent: true,
	}

	_, dagData, err := connmgr.DownloadDag(ctx, cm, "test-relay", fd.Dag.Root, nil, filter, nil)
	if err != nil {
		t.Fatalf("Failed to partial download DAG: %v", err)
	}

	// Verify we got leaves back
	if len(dagData.Dag.Leafs) == 0 {
		t.Error("Partial download returned no leaves")
	}

	// Verify the partial DAG
	if err := dagData.Dag.Verify(); err != nil {
		t.Errorf("Partial DAG verification failed: %v", err)
	}

	t.Logf("Successfully partial downloaded %s DAG: requested %d files, got %d leaves",
		fd.Fixture.Name, len(requestedHashes), len(dagData.Dag.Leafs))
}

// TestDAGDownloadWithoutContent tests downloading DAG structure only
func TestDAGDownloadWithoutContent(t *testing.T) {
	helpers.RunTestWithFixture(t, "flat_directory", func(t *testing.T, fd *helpers.FixtureDAG) {
		relay, err := helpers.NewTestLibp2pRelay(helpers.DefaultTestLibp2pConfig())
		if err != nil {
			t.Fatalf("Failed to create test relay: %v", err)
		}
		defer relay.Cleanup()

		if err := relay.WaitForReady(5 * time.Second); err != nil {
			t.Fatalf("Relay not ready: %v", err)
		}

		kp, err := helpers.GenerateDAGKeyPair()
		if err != nil {
			t.Fatalf("Failed to generate key pair: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		cm, err := relay.GetConnectionManager(ctx)
		if err != nil {
			t.Fatalf("Failed to get connection manager: %v", err)
		}
		defer cm.Disconnect("test-relay")

		// Upload the DAG first
		err = connmgr.UploadDag(ctx, cm, fd.Dag, kp.PrivateKey, nil)
		if err != nil {
			t.Fatalf("Failed to upload DAG: %v", err)
		}

		// Download without content
		filter := &lib_types.DownloadFilter{
			IncludeContent: false,
		}

		_, dagData, err := connmgr.DownloadDag(ctx, cm, "test-relay", fd.Dag.Root, nil, filter, nil)
		if err != nil {
			t.Fatalf("Failed to download DAG without content: %v", err)
		}

		if dagData.Dag.Root != fd.Dag.Root {
			t.Errorf("Downloaded DAG root mismatch: got %s, want %s", dagData.Dag.Root, fd.Dag.Root)
		}

		t.Logf("Successfully downloaded DAG structure without content")
	})
}

// TestDAGQuery tests querying for DAGs by public key
func TestDAGQuery(t *testing.T) {
	relay, err := helpers.NewTestLibp2pRelay(helpers.DefaultTestLibp2pConfig())
	if err != nil {
		t.Fatalf("Failed to create test relay: %v", err)
	}
	defer relay.Cleanup()

	if err := relay.WaitForReady(5 * time.Second); err != nil {
		t.Fatalf("Relay not ready: %v", err)
	}

	kp, err := helpers.GenerateDAGKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Upload multiple DAGs from different fixtures
	fixtures := []string{"single_small_file", "flat_directory", "nested_directory"}
	var uploadedRoots []string

	for _, fixtureName := range fixtures {
		fd, err := helpers.CreateDAGFromFixtureName(fixtureName)
		if err != nil {
			t.Fatalf("Failed to create DAG from fixture %s: %v", fixtureName, err)
		}
		defer fd.Cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		cm, err := relay.GetConnectionManager(ctx)
		if err != nil {
			t.Fatalf("Failed to get connection manager: %v", err)
		}

		err = connmgr.UploadDag(ctx, cm, fd.Dag, kp.PrivateKey, nil)
		cm.Disconnect("test-relay")
		if err != nil {
			t.Fatalf("Failed to upload DAG %s: %v", fixtureName, err)
		}

		uploadedRoots = append(uploadedRoots, fd.Dag.Root)
	}

	// Query for DAGs by public key
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cm, err := relay.GetConnectionManager(ctx)
	if err != nil {
		t.Fatalf("Failed to get connection manager: %v", err)
	}
	defer cm.Disconnect("test-relay")

	queryFilter := lib_types.QueryFilter{
		PubKeys: []string{kp.PublicHex},
	}

	hashes, err := connmgr.QueryDag(ctx, cm, "test-relay", queryFilter)
	if err != nil {
		t.Fatalf("Failed to query DAGs: %v", err)
	}

	if len(hashes) != len(uploadedRoots) {
		t.Errorf("Query returned wrong number of DAGs: got %d, want %d", len(hashes), len(uploadedRoots))
	}

	// Verify all uploaded roots are in the query results
	hashSet := make(map[string]bool)
	for _, h := range hashes {
		hashSet[h] = true
	}
	for _, root := range uploadedRoots {
		if !hashSet[root] {
			t.Errorf("Uploaded root %s not found in query results", root)
		}
	}

	t.Logf("Successfully queried %d DAGs", len(hashes))
}

// TestDAGUploadAndReupload tests uploading the same DAG twice
func TestDAGUploadAndReupload(t *testing.T) {
	helpers.RunTestWithFixture(t, "flat_directory", func(t *testing.T, fd *helpers.FixtureDAG) {
		relay, err := helpers.NewTestLibp2pRelay(helpers.DefaultTestLibp2pConfig())
		if err != nil {
			t.Fatalf("Failed to create test relay: %v", err)
		}
		defer relay.Cleanup()

		if err := relay.WaitForReady(5 * time.Second); err != nil {
			t.Fatalf("Relay not ready: %v", err)
		}

		kp, err := helpers.GenerateDAGKeyPair()
		if err != nil {
			t.Fatalf("Failed to generate key pair: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		cm, err := relay.GetConnectionManager(ctx)
		if err != nil {
			t.Fatalf("Failed to get connection manager: %v", err)
		}
		defer cm.Disconnect("test-relay")

		// Upload first time
		err = connmgr.UploadDag(ctx, cm, fd.Dag, kp.PrivateKey, nil)
		if err != nil {
			t.Fatalf("Failed first upload: %v", err)
		}

		// Upload same DAG again
		err = connmgr.UploadDag(ctx, cm, fd.Dag, kp.PrivateKey, nil)
		if err != nil {
			t.Fatalf("Failed second upload (reupload): %v", err)
		}

		// Verify DAG still exists correctly
		exists, err := relay.Store.HasLeafGlobal(fd.Dag.Root)
		if err != nil {
			t.Fatalf("Failed to check if DAG exists: %v", err)
		}
		if !exists {
			t.Error("DAG root not found after reupload")
		}

		t.Logf("Successfully handled DAG reupload")
	})
}

// TestDAGRoundTrip tests a complete upload and download cycle with verification
func TestDAGRoundTrip_AllFixtures(t *testing.T) {
	helpers.RunTestWithAllFixtures(t, func(t *testing.T, fd *helpers.FixtureDAG) {
		relay, err := helpers.NewTestLibp2pRelay(helpers.DefaultTestLibp2pConfig())
		if err != nil {
			t.Fatalf("Failed to create test relay: %v", err)
		}
		defer relay.Cleanup()

		if err := relay.WaitForReady(5 * time.Second); err != nil {
			t.Fatalf("Relay not ready: %v", err)
		}

		kp, err := helpers.GenerateDAGKeyPair()
		if err != nil {
			t.Fatalf("Failed to generate key pair: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		cm, err := relay.GetConnectionManager(ctx)
		if err != nil {
			t.Fatalf("Failed to get connection manager: %v", err)
		}
		defer cm.Disconnect("test-relay")

		// Verify original DAG first
		if err := fd.Dag.Verify(); err != nil {
			t.Fatalf("Original DAG verification failed: %v", err)
		}

		// Upload the DAG
		err = connmgr.UploadDag(ctx, cm, fd.Dag, kp.PrivateKey, nil)
		if err != nil {
			t.Fatalf("Failed to upload DAG: %v", err)
		}

		// Download the DAG
		_, dagData, err := connmgr.DownloadDag(ctx, cm, "test-relay", fd.Dag.Root, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to download DAG: %v", err)
		}

		// Compare original and downloaded DAGs
		if dagData.Dag.Root != fd.Dag.Root {
			t.Errorf("Root mismatch: got %s, want %s", dagData.Dag.Root, fd.Dag.Root)
		}

		if len(dagData.Dag.Leafs) != len(fd.Dag.Leafs) {
			t.Errorf("Leaf count mismatch: got %d, want %d",
				len(dagData.Dag.Leafs), len(fd.Dag.Leafs))
		}

		// Verify the downloaded DAG
		if err := dagData.Dag.Verify(); err != nil {
			t.Errorf("Downloaded DAG verification failed: %v", err)
		}

		// Verify all original leaf hashes are present
		for hash := range fd.Dag.Leafs {
			if _, exists := dagData.Dag.Leafs[hash]; !exists {
				t.Errorf("Leaf %s from original not found in downloaded DAG", hash)
			}
		}

		t.Logf("Round trip successful for %s: %d leaves", fd.Fixture.Name, len(dagData.Dag.Leafs))
	})
}

// TestDAGConcurrentUploads tests multiple concurrent DAG uploads
func TestDAGConcurrentUploads(t *testing.T) {
	relay, err := helpers.NewTestLibp2pRelay(helpers.DefaultTestLibp2pConfig())
	if err != nil {
		t.Fatalf("Failed to create test relay: %v", err)
	}
	defer relay.Cleanup()

	if err := relay.WaitForReady(5 * time.Second); err != nil {
		t.Fatalf("Relay not ready: %v", err)
	}

	kp, err := helpers.GenerateDAGKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Use different fixtures for concurrent uploads
	fixtures := helpers.GetMultiFileFixtures()
	numUploads := len(fixtures)
	errChan := make(chan error, numUploads)
	rootChan := make(chan string, numUploads)

	// Launch concurrent uploads
	for i, fixture := range fixtures {
		go func(idx int, fix helpers.TestFixture) {
			fd, err := helpers.CreateDAGFromFixture(fix)
			if err != nil {
				errChan <- err
				return
			}
			defer fd.Cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			cm, err := relay.GetConnectionManager(ctx)
			if err != nil {
				errChan <- err
				return
			}
			defer cm.Disconnect("test-relay")

			err = connmgr.UploadDag(ctx, cm, fd.Dag, kp.PrivateKey, nil)
			if err != nil {
				errChan <- err
				return
			}

			rootChan <- fd.Dag.Root
			errChan <- nil
		}(i, fixture)
	}

	// Wait for all uploads to complete
	var uploadedRoots []string
	for i := 0; i < numUploads; i++ {
		err := <-errChan
		if err != nil {
			t.Errorf("Concurrent upload failed: %v", err)
		}
	}

	close(rootChan)
	for root := range rootChan {
		uploadedRoots = append(uploadedRoots, root)
	}

	// Verify all DAGs exist
	for _, root := range uploadedRoots {
		exists, err := relay.Store.HasLeafGlobal(root)
		if err != nil {
			t.Errorf("Error checking DAG %s: %v", root, err)
		}
		if !exists {
			t.Errorf("DAG %s not found after concurrent upload", root)
		}
	}

	t.Logf("Successfully completed %d concurrent uploads", len(uploadedRoots))
}

// TestDAGVerification tests that uploaded DAGs are properly verified
func TestDAGVerification(t *testing.T) {
	helpers.RunTestWithAllFixtures(t, func(t *testing.T, fd *helpers.FixtureDAG) {
		relay, err := helpers.NewTestLibp2pRelay(helpers.DefaultTestLibp2pConfig())
		if err != nil {
			t.Fatalf("Failed to create test relay: %v", err)
		}
		defer relay.Cleanup()

		if err := relay.WaitForReady(5 * time.Second); err != nil {
			t.Fatalf("Relay not ready: %v", err)
		}

		kp, err := helpers.GenerateDAGKeyPair()
		if err != nil {
			t.Fatalf("Failed to generate key pair: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		cm, err := relay.GetConnectionManager(ctx)
		if err != nil {
			t.Fatalf("Failed to get connection manager: %v", err)
		}
		defer cm.Disconnect("test-relay")

		// Upload the DAG
		err = connmgr.UploadDag(ctx, cm, fd.Dag, kp.PrivateKey, nil)
		if err != nil {
			t.Fatalf("Failed to upload DAG: %v", err)
		}

		// Verify the DAG can be built from store and is valid
		dagData, err := relay.Store.BuildDagFromStore(fd.Dag.Root, true)
		if err != nil {
			t.Fatalf("Failed to build DAG from store: %v", err)
		}

		// Verify the DAG
		if err := dagData.Dag.Verify(); err != nil {
			t.Errorf("Stored DAG failed verification: %v", err)
		}

		t.Logf("DAG verification successful for %s", fd.Fixture.Name)
	})
}

// TestDAGTransmissionPackets tests that DAGs can be transmitted as packets
func TestDAGTransmissionPackets(t *testing.T) {
	helpers.RunTestWithAllFixtures(t, func(t *testing.T, fd *helpers.FixtureDAG) {
		// Verify original DAG
		if err := fd.Dag.Verify(); err != nil {
			t.Fatalf("Original DAG verification failed: %v", err)
		}

		// Get batched transmission sequence
		sequence := fd.Dag.GetBatchedLeafSequence()
		if len(sequence) == 0 {
			t.Fatal("No transmission packets generated")
		}

		// Simulate receiving the DAG
		receiverDag := &merkle_dag.Dag{
			Root:  fd.Dag.Root,
			Leafs: make(map[string]*merkle_dag.DagLeaf),
		}

		for i, packet := range sequence {
			err := receiverDag.ApplyAndVerifyBatchedTransmissionPacket(packet)
			if err != nil {
				t.Fatalf("Failed to apply packet %d: %v", i, err)
			}
		}

		// Verify the received DAG
		if len(receiverDag.Leafs) != len(fd.Dag.Leafs) {
			t.Errorf("Receiver DAG has %d leaves, expected %d",
				len(receiverDag.Leafs), len(fd.Dag.Leafs))
		}

		if err := receiverDag.Verify(); err != nil {
			t.Errorf("Receiver DAG verification failed: %v", err)
		}

		t.Logf("Successfully transmitted %s DAG in %d packets (%d leaves)",
			fd.Fixture.Name, len(sequence), len(fd.Dag.Leafs))
	})
}

// TestDAGPartialTransmission tests partial DAG transmission
func TestDAGPartialTransmission(t *testing.T) {
	helpers.RunTestWithMultiFileFixtures(t, func(t *testing.T, fd *helpers.FixtureDAG) {
		// Verify original DAG
		if err := fd.Dag.Verify(); err != nil {
			t.Fatalf("Original DAG verification failed: %v", err)
		}

		// Get file leaf hashes in deterministic order
		fileHashes := helpers.GetFileLeafHashes(fd.Dag)
		if len(fileHashes) < 2 {
			t.Skip("DAG has insufficient file leaves for partial test")
		}

		// Get partial DAG with first 2 files
		partialDag, err := fd.Dag.GetPartial(fileHashes[:2], true)
		if err != nil {
			t.Fatalf("Failed to get partial DAG: %v", err)
		}

		// Verify partial DAG
		if err := partialDag.Verify(); err != nil {
			t.Fatalf("Partial DAG verification failed: %v", err)
		}

		if !partialDag.IsPartial() {
			t.Error("Partial DAG should be marked as partial")
		}

		// Get transmission sequence from partial
		sequence := partialDag.GetLeafSequence()
		if len(sequence) == 0 {
			t.Fatal("No transmission packets from partial DAG")
		}

		// Simulate receiving the partial DAG
		receiverDag := &merkle_dag.Dag{
			Root:  partialDag.Root,
			Leafs: make(map[string]*merkle_dag.DagLeaf),
		}

		for i, packet := range sequence {
			receiverDag.ApplyTransmissionPacket(packet)
			if err := receiverDag.Verify(); err != nil {
				t.Fatalf("Verification failed after packet %d: %v", i, err)
			}
		}

		if len(receiverDag.Leafs) != len(partialDag.Leafs) {
			t.Errorf("Receiver has %d leaves, expected %d",
				len(receiverDag.Leafs), len(partialDag.Leafs))
		}

		t.Logf("Successfully transmitted partial %s DAG: %d leaves",
			fd.Fixture.Name, len(partialDag.Leafs))
	})
}
