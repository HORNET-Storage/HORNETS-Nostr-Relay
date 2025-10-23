package test

import (
	"testing"
	"time"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/v2/dag"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics/gorm/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveTagsFixesPanicIssue(t *testing.T) {
	// Initialize the statistics store
	store, err := sqlite.InitStore()
	require.NoError(t, err)
	defer func() {
		// Clean up - close database connection properly
		if db, err := store.DB.DB(); err == nil {
			db.Close()
		}
	}()

	// Create a test leaf with additional data
	leaf := &merkle_dag.DagLeaf{
		Hash: "test-hash-" + time.Now().Format("20060102150405"),
		AdditionalData: map[string]string{
			"name":    "test-file.txt",
			"version": "1.0",
			"author":  "test-author",
		},
	}

	// Test that SaveTags doesn't panic
	err = store.SaveTags("test-root", leaf)
	assert.NoError(t, err, "SaveTags should not return an error")

	// Test calling SaveTags again with the same data (should handle duplicates)
	err = store.SaveTags("test-root", leaf)
	assert.NoError(t, err, "SaveTags should handle duplicates without error")

	// Test with empty additional data
	emptyLeaf := &merkle_dag.DagLeaf{
		Hash:           "empty-hash",
		AdditionalData: map[string]string{},
	}
	err = store.SaveTags("test-root", emptyLeaf)
	assert.NoError(t, err, "SaveTags should handle empty additional data")
}

func TestSaveFileHandlesUniqueConstraint(t *testing.T) {
	// Initialize the statistics store
	store, err := sqlite.InitStore()
	require.NoError(t, err)
	defer func() {
		// Clean up - close database connection properly
		if db, err := store.DB.DB(); err == nil {
			db.Close()
		}
	}()

	// Test data
	root := "test-root"
	hash := "unique-hash-" + time.Now().Format("20060102150405")
	fileName := "test-file.jpg"
	mimeType := "image/jpeg"
	leafCount := 1
	size := int64(1024)

	// First save should succeed
	err = store.SaveFile(root, hash, fileName, mimeType, leafCount, size)
	assert.NoError(t, err, "First SaveFile should succeed")

	// Second save with same hash should not fail (FirstOrCreate handles it)
	err = store.SaveFile(root, hash, fileName, mimeType, leafCount, size)
	assert.NoError(t, err, "Second SaveFile with same hash should not fail")

	// Query to verify only one record exists
	files, err := store.QueryFiles(map[string]interface{}{"hash": hash})
	assert.NoError(t, err)
	assert.Len(t, files, 1, "Should only have one file with the hash")
}
