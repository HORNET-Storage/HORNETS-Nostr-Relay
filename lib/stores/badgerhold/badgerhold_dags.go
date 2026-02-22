package badgerhold

import (
	"errors"
	"fmt"
	"math/rand"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/fxamacker/cbor/v2"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/v2/dag"
	lib_types "github.com/HORNET-Storage/go-hornet-storage-lib/lib"
	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/dgraph-io/badger/v4"
	"github.com/gabriel-vasile/mimetype"
	"github.com/ipfs/go-cid"
	"github.com/timshannon/badgerhold/v4"
)

// skippedLeafCount tracks how many leaves were skipped due to already existing
var skippedLeafCount atomic.Int64

// storedLeafCount tracks how many leaves were actually stored (not skipped)
var storedLeafCount atomic.Int64

// GetAndResetSkippedLeafCount returns the count of skipped leaves and resets it
func GetAndResetSkippedLeafCount() int64 {
	return skippedLeafCount.Swap(0)
}

// GetAndResetStoredLeafCount returns the count of stored leaves and resets it
func GetAndResetStoredLeafCount() int64 {
	return storedLeafCount.Swap(0)
}

// maxRetries is the number of times to retry on transaction conflict
const maxRetries = 10

// retryWithBackoff executes fn with exponential backoff on transaction conflicts
func retryWithBackoff(fn func() error) error {
	var err error

	// Add initial random delay to stagger concurrent operations (0-50ms)
	initialJitter := time.Duration(rand.Int63n(50)) * time.Millisecond
	time.Sleep(initialJitter)

	for attempt := 0; attempt < maxRetries; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}

		// Check if it's a transaction conflict
		if errors.Is(err, badger.ErrConflict) {
			// Exponential backoff with jitter: 20-60ms, 40-120ms, 80-240ms, etc.
			baseDelay := time.Duration(20<<attempt) * time.Millisecond
			jitter := time.Duration(rand.Int63n(int64(baseDelay)))
			delay := baseDelay + jitter

			// Cap max delay at 2 seconds
			if delay > 2*time.Second {
				delay = 2*time.Second + time.Duration(rand.Int63n(500))*time.Millisecond
			}

			time.Sleep(delay)
			continue
		}

		// Non-conflict error, return immediately
		return err
	}

	return fmt.Errorf("transaction conflict after %d retries: %w", maxRetries, err)
}

func (store *BadgerholdStore) RetrieveContent(root cid.Cid, contentHash []byte) ([]byte, error) {
	key := makeKey("content", contentHash)

	var content []byte
	err := store.Database.Badger().View(func(tx *badger.Txn) error {
		item, err := tx.Get(key)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			content = append([]byte{}, val...)
			return nil
		})
	})

	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to retrieve content: %w", err)
	}

	return content, nil
}

func (store *BadgerholdStore) CacheLabels(dag *merkle_dag.Dag) error {
	err := dag.CalculateLabels()
	if err != nil {
		return err
	}

	// Encode labels map to CBOR
	labelsCBOR, err := cbor.Marshal(dag.Labels)
	if err != nil {
		return fmt.Errorf("failed to marshal labels: %w", err)
	}

	// Store using raw Badger
	return store.Database.Badger().Update(func(tx *badger.Txn) error {
		key := makeKey("labels", dag.Root)
		return tx.Set(key, labelsCBOR)
	})
}

func (store *BadgerholdStore) CacheLabelsStreaming(dagStore *merkle_dag.DagStore) error {
	if dagStore.Root == "" {
		return fmt.Errorf("no root set")
	}

	if !dagStore.HasIndex() {
		if err := dagStore.BuildIndex(); err != nil {
			return fmt.Errorf("failed to build index: %w", err)
		}
	}

	labels := make(map[string]string)
	labelCounter := 1

	err := dagStore.IterateDagWithIndex(func(leafHash string, parentHash string) error {
		// Skip the root (it's implicitly label "0")
		if leafHash == dagStore.Root {
			return nil
		}

		// Assign label to this leaf
		label := strconv.Itoa(labelCounter)
		labels[label] = leafHash
		labelCounter++

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to iterate DAG: %w", err)
	}

	// Encode labels map to CBOR
	labelsCBOR, err := cbor.Marshal(labels)
	if err != nil {
		return fmt.Errorf("failed to marshal labels: %w", err)
	}

	// Store using raw Badger
	return store.Database.Badger().Update(func(tx *badger.Txn) error {
		key := makeKey("labels", dagStore.Root)
		return tx.Set(key, labelsCBOR)
	})
}

func (store *BadgerholdStore) RetrieveLabels(root string) (map[string]string, error) {
	var labelsCBOR []byte

	err := store.Database.Badger().View(func(tx *badger.Txn) error {
		key := makeKey("labels", root)
		item, err := tx.Get(key)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			labelsCBOR = append([]byte{}, val...)
			return nil
		})
	})

	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, fmt.Errorf("labels not found for root %s", root)
		}
		return nil, fmt.Errorf("failed to retrieve labels: %w", err)
	}

	var labels map[string]string
	err = cbor.Unmarshal(labelsCBOR, &labels)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal labels: %w", err)
	}

	return labels, nil
}

func (store *BadgerholdStore) CacheRelationships(root string, relationships map[string]string) error {
	if root == "" {
		return fmt.Errorf("no root provided")
	}

	relationshipsCBOR, err := cbor.Marshal(relationships)
	if err != nil {
		return fmt.Errorf("failed to marshal relationships: %w", err)
	}

	// Store using raw Badger
	err = store.Database.Badger().Update(func(tx *badger.Txn) error {
		key := makeKey("relationships", root)
		return tx.Set(key, relationshipsCBOR)
	})
	if err != nil {
		return err
	}
	return nil
}

func (store *BadgerholdStore) CacheRelationshipsStreaming(dagStore *merkle_dag.DagStore) error {
	if dagStore.Root == "" {
		return fmt.Errorf("no root set")
	}

	if !dagStore.HasIndex() {
		if err := dagStore.BuildIndex(); err != nil {
			return fmt.Errorf("failed to build index: %w", err)
		}
	}

	relationships := make(map[string]string)
	err := dagStore.IterateDagWithIndex(func(leafHash string, parentHash string) error {
		relationships[leafHash] = parentHash
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to iterate DAG for relationships: %w", err)
	}

	return store.CacheRelationships(dagStore.Root, relationships)
}

func (store *BadgerholdStore) RetrieveRelationships(root string) (map[string]string, error) {
	var relationshipsCBOR []byte

	err := store.Database.Badger().View(func(tx *badger.Txn) error {
		key := makeKey("relationships", root)
		item, err := tx.Get(key)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			relationshipsCBOR = append([]byte{}, val...)
			return nil
		})
	})

	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, nil // Not cached yet, return nil (not an error)
		}
		return nil, fmt.Errorf("failed to retrieve relationships: %w", err)
	}

	var relationships map[string]string
	err = cbor.Unmarshal(relationshipsCBOR, &relationships)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal relationships: %w", err)
	}

	return relationships, nil
}

func (store *BadgerholdStore) DeleteDag(root string) error {
	// Delete all ownership records with matching root using BadgerHold's indexed query
	err := store.Database.DeleteMatching(&types.DagOwnership{}, badgerhold.Where("Root").Eq(root))
	if err != nil && err != badgerhold.ErrNotFound {
		return fmt.Errorf("failed to delete ownership records: %w", err)
	}

	// Delete all tags for this root using prefix scan
	db := store.Database.Badger()
	err = db.Update(func(tx *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false

		// Delete tags with this root prefix
		prefix := makeKey("tags", root)
		it := tx.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.KeyCopy(nil)

			if err := tx.Delete(key); err != nil {
				return err
			}
		}

		// Delete cached labels
		labelsKey := makeKey("labels", root)
		if err := tx.Delete(labelsKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		// Delete cached relationships
		relationshipsKey := makeKey("relationships", root)
		if err := tx.Delete(relationshipsKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		return nil
	})

	return err
}

func (store *BadgerholdStore) StoreLeavesBatch(root string, leaves []*types.DagLeafData) error {
	if len(leaves) == 0 {
		return nil
	}

	wb := store.Database.Badger().NewWriteBatch()
	defer wb.Cancel()

	var contentWrites int64

	type leafContentInsert struct {
		key   string
		value types.LeafContent
	}
	type ownershipInsert struct {
		key   string
		value types.DagOwnership
	}
	type parentCacheInsert struct {
		key   string
		value types.LeafParentCache
	}

	var leafContentsToInsert []leafContentInsert
	var ownershipsToInsert []ownershipInsert
	var parentCachesToInsert []parentCacheInsert

	for _, leafData := range leaves {
		isRootLeaf := root == leafData.Leaf.Hash

		// Check if leaf content already exists
		leafContentExists := false
		var existingLeaf types.LeafContent
		err := store.Database.Get(leafData.Leaf.Hash, &existingLeaf)
		if err == nil {
			leafContentExists = true
		}

		// Check ownership for root leaf
		// New format: root:pubkey
		// Old format (fallback): root:pubkey:root
		ownershipKey := root + ":" + leafData.PublicKey
		ownershipExists := false
		if isRootLeaf {
			var existingOwnership types.DagOwnership
			err = store.Database.Get(ownershipKey, &existingOwnership)
			if err == nil {
				ownershipExists = true
			} else {
				// Fallback: check old key format (root:pubkey:root) for backward compatibility
				oldOwnershipKey := root + ":" + leafData.PublicKey + ":" + root
				err = store.Database.Get(oldOwnershipKey, &existingOwnership)
				if err == nil {
					ownershipExists = true
				}
			}
		}

		// Skip if already fully stored
		if leafContentExists && (!isRootLeaf || ownershipExists) {
			skippedLeafCount.Add(1)
			continue
		}

		// Store content using WriteBatch (raw bytes, no BadgerHold)
		if leafData.Leaf.Content != nil {
			contentKey := makeKey("content", leafData.Leaf.ContentHash)

			// Check existence
			var contentExists bool
			store.Database.Badger().View(func(tx *badger.Txn) error {
				_, err := tx.Get(contentKey)
				contentExists = (err == nil)
				return nil
			})

			if !contentExists {
				if err := wb.Set(contentKey, leafData.Leaf.Content); err != nil {
					return fmt.Errorf("error batching content: %w", err)
				}
				contentWrites++
			}

			// Clear content to avoid storing in metadata
			leafData.Leaf.Content = nil
		}

		// Collect LeafContent if needed
		if !leafContentExists {
			leafContentsToInsert = append(leafContentsToInsert, leafContentInsert{
				key: leafData.Leaf.Hash,
				value: types.LeafContent{
					Hash:              leafData.Leaf.Hash,
					ItemName:          leafData.Leaf.ItemName,
					Type:              leafData.Leaf.Type,
					ContentHash:       leafData.Leaf.ContentHash,
					ClassicMerkleRoot: leafData.Leaf.ClassicMerkleRoot,
					CurrentLinkCount:  leafData.Leaf.CurrentLinkCount,
					LeafCount:         leafData.Leaf.LeafCount,
					ContentSize:       leafData.Leaf.ContentSize,
					DagSize:           leafData.Leaf.DagSize,
					Links:             leafData.Leaf.Links,
					AdditionalData:    leafData.Leaf.AdditionalData,
				},
			})
		}

		// Collect Ownership for root leaf
		if isRootLeaf && !ownershipExists {
			ownershipsToInsert = append(ownershipsToInsert, ownershipInsert{
				key: ownershipKey,
				value: types.DagOwnership{
					Root:      root,
					PublicKey: leafData.PublicKey,
					Signature: leafData.Signature,
				},
			})
		}

		// Collect ParentCache if needed
		if leafData.Leaf.ParentHash != "" {
			parentCacheKey := root + ":" + leafData.Leaf.Hash
			var existingCache types.LeafParentCache
			err = store.Database.Get(parentCacheKey, &existingCache)
			if errors.Is(err, badgerhold.ErrNotFound) {
				parentCachesToInsert = append(parentCachesToInsert, parentCacheInsert{
					key: parentCacheKey,
					value: types.LeafParentCache{
						RootHash:   root,
						LeafHash:   leafData.Leaf.Hash,
						ParentHash: leafData.Leaf.ParentHash,
					},
				})
			}
		}

		storedLeafCount.Add(1)
	}

	// Flush content writes
	if contentWrites > 0 {
		if err := wb.Flush(); err != nil {
			return fmt.Errorf("error flushing content batch: %w", err)
		}
	}

	// Batch all metadata writes into a single transaction
	if len(leafContentsToInsert) > 0 || len(ownershipsToInsert) > 0 || len(parentCachesToInsert) > 0 {
		err := store.Database.Badger().Update(func(tx *badger.Txn) error {
			// Insert all leaf contents
			for _, item := range leafContentsToInsert {
				if err := store.Database.TxInsert(tx, item.key, item.value); err != nil {
					if !errors.Is(err, badgerhold.ErrKeyExists) {
						return fmt.Errorf("error inserting leaf content: %w", err)
					}
				}
			}

			// Insert all ownerships
			for _, item := range ownershipsToInsert {
				if err := store.Database.TxInsert(tx, item.key, item.value); err != nil {
					if !errors.Is(err, badgerhold.ErrKeyExists) {
						return fmt.Errorf("error inserting ownership: %w", err)
					}
				}
			}

			// Insert all parent caches
			for _, item := range parentCachesToInsert {
				if err := store.Database.TxInsert(tx, item.key, item.value); err != nil {
					if !errors.Is(err, badgerhold.ErrKeyExists) {
						return fmt.Errorf("error inserting parent cache: %w", err)
					}
				}
			}

			return nil
		})
		if err != nil {
			return err
		}
	}

	// Signal the adaptive GC goroutine that a bulk write just completed
	store.SignalGC()

	return nil
}

func (store *BadgerholdStore) storeLeafMetadata(root string, leafData *types.DagLeafData) error {
	isRootLeaf := root == leafData.Leaf.Hash

	// Check if leaf content already exists
	leafContentExists := false
	var existingLeaf types.LeafContent
	err := store.Database.Get(leafData.Leaf.Hash, &existingLeaf)
	if err == nil {
		leafContentExists = true
	}

	// Only check/store ownership for root leaf (one per DAG per owner)
	ownershipExists := false
	var ownershipKey string
	if isRootLeaf {
		ownershipKey = root + ":" + leafData.PublicKey
		var existingOwnership types.DagOwnership
		err = store.Database.Get(ownershipKey, &existingOwnership)
		if err == nil {
			ownershipExists = true
		}
	}

	// Skip if both content and ownership (for root) already exist
	if leafContentExists && (!isRootLeaf || ownershipExists) {
		skippedLeafCount.Add(1)
		return nil
	}

	// Store LeafContent if needed
	if !leafContentExists {
		leafContent := types.LeafContent{
			Hash:              leafData.Leaf.Hash,
			ItemName:          leafData.Leaf.ItemName,
			Type:              leafData.Leaf.Type,
			ContentHash:       leafData.Leaf.ContentHash,
			ClassicMerkleRoot: leafData.Leaf.ClassicMerkleRoot,
			CurrentLinkCount:  leafData.Leaf.CurrentLinkCount,
			LeafCount:         leafData.Leaf.LeafCount,
			ContentSize:       leafData.Leaf.ContentSize,
			DagSize:           leafData.Leaf.DagSize,
			Links:             leafData.Leaf.Links,
			AdditionalData:    leafData.Leaf.AdditionalData,
		}
		err = store.Database.Insert(leafData.Leaf.Hash, leafContent)
		if err != nil && !errors.Is(err, badgerhold.ErrKeyExists) {
			return fmt.Errorf("error inserting leaf content: %w", err)
		}
	}

	// Store Ownership only for root leaf (one record per DAG per owner)
	if isRootLeaf && !ownershipExists {
		ownership := types.DagOwnership{
			Root:      root,
			PublicKey: leafData.PublicKey,
			Signature: leafData.Signature,
		}
		err = store.Database.Insert(ownershipKey, ownership)
		if err != nil && !errors.Is(err, badgerhold.ErrKeyExists) {
			return fmt.Errorf("error inserting ownership: %w", err)
		}
	}

	// Store ParentCache if needed
	if leafData.Leaf.ParentHash != "" {
		parentCacheKey := root + ":" + leafData.Leaf.Hash
		var existingCache types.LeafParentCache
		err = store.Database.Get(parentCacheKey, &existingCache)
		if errors.Is(err, badgerhold.ErrNotFound) {
			parentCache := types.LeafParentCache{
				RootHash:   root,
				LeafHash:   leafData.Leaf.Hash,
				ParentHash: leafData.Leaf.ParentHash,
			}
			err = store.Database.Insert(parentCacheKey, parentCache)
			if err != nil && !errors.Is(err, badgerhold.ErrKeyExists) {
				return fmt.Errorf("error inserting parent cache: %w", err)
			}
		}
	}

	storedLeafCount.Add(1)
	count := storedLeafCount.Load()
	if count%1000 == 0 {
	}

	return nil
}

func (store *BadgerholdStore) StoreLeaf(root string, leafData *types.DagLeafData) error {
	isRootLeaf := root == leafData.Leaf.Hash
	if isRootLeaf {
	}

	rootCID, err := cid.Decode(root)
	if err != nil {
		return fmt.Errorf("invalid root CID: %w", err)
	}

	leafCID := rootCID
	if !isRootLeaf {
		leafCID, err = cid.Decode(leafData.Leaf.Hash)
		if err != nil {
			return fmt.Errorf("invalid leaf CID: %w", err)
		}
	}

	var contentSize int64
	var mimeType string

	// ===== CHECK IF LEAF ALREADY EXISTS =====
	leafContentExists := false
	var existingLeaf types.LeafContent
	err = store.Database.Get(leafData.Leaf.Hash, &existingLeaf)
	if err == nil {
		leafContentExists = true
	} else if !errors.Is(err, badgerhold.ErrNotFound) {
		return fmt.Errorf("error checking leaf existence: %w", err)
	}

	// Only check/store ownership for root leaf (one per DAG per owner)
	ownershipExists := false
	var ownershipKey string
	if isRootLeaf {
		ownershipKey = root + ":" + leafData.PublicKey
		var existingOwnership types.DagOwnership
		err = store.Database.Get(ownershipKey, &existingOwnership)
		if err == nil {
			ownershipExists = true
		} else if !errors.Is(err, badgerhold.ErrNotFound) {
			return fmt.Errorf("error checking ownership existence: %w", err)
		}
	}

	// Skip if both content and ownership (for root) already exist
	if leafContentExists && (!isRootLeaf || ownershipExists) {
		if isRootLeaf {
		}
		skippedLeafCount.Add(1)
		return nil
	}

	// ===== STORE CONTENT (raw bytes) =====
	if leafData.Leaf.Content != nil {
		contentSize = int64(len(leafData.Leaf.Content))
		mtype := mimetype.Detect(leafData.Leaf.Content)
		mimeType = mtype.String()

		contentKey := makeKey("content", leafData.Leaf.ContentHash)

		// Check if content exists using a read-only transaction (no conflicts)
		contentExists := false
		err = store.Database.Badger().View(func(tx *badger.Txn) error {
			_, err := tx.Get(contentKey)
			if err == badger.ErrKeyNotFound {
				return nil
			}
			if err != nil {
				return err
			}
			contentExists = true
			return nil
		})
		if err != nil {
			return fmt.Errorf("error checking content existence: %w", err)
		}

		// Only write content if it doesn't exist
		if !contentExists {
			err = store.Database.Badger().Update(func(tx *badger.Txn) error {
				// Double-check inside transaction in case of race
				_, err := tx.Get(contentKey)
				if err == badger.ErrKeyNotFound {
					return tx.Set(contentKey, leafData.Leaf.Content)
				}
				if err != nil {
					return err
				}
				return nil
			})
			if err != nil {
				return fmt.Errorf("error storing content: %w", err)
			}
		}

		// Clear content from leaf to avoid storing it again in metadata
		leafData.Leaf.Content = nil
	}

	// ===== STORE LEAF CONTENT (metadata) =====
	if !leafContentExists {
		leafContent := types.LeafContent{
			Hash:              leafData.Leaf.Hash,
			ItemName:          leafData.Leaf.ItemName,
			Type:              leafData.Leaf.Type,
			ContentHash:       leafData.Leaf.ContentHash,
			ClassicMerkleRoot: leafData.Leaf.ClassicMerkleRoot,
			CurrentLinkCount:  leafData.Leaf.CurrentLinkCount,
			LeafCount:         leafData.Leaf.LeafCount,
			ContentSize:       leafData.Leaf.ContentSize,
			DagSize:           leafData.Leaf.DagSize,
			Links:             leafData.Leaf.Links,
			AdditionalData:    leafData.Leaf.AdditionalData,
		}

		err = store.Database.Insert(leafData.Leaf.Hash, leafContent)
		if err != nil && !errors.Is(err, badgerhold.ErrKeyExists) {
			if isRootLeaf {
			}
			return fmt.Errorf("error inserting leaf content: %w", err)
		}

		if isRootLeaf {
		}
	}

	// ===== STORE OWNERSHIP (only for root leaf) =====
	if isRootLeaf && !ownershipExists {
		ownership := types.DagOwnership{
			Root:      root,
			PublicKey: leafData.PublicKey,
			Signature: leafData.Signature,
		}

		// Key is root:publicKey (one ownership record per DAG per owner)
		// Use Insert and ignore if already exists (race condition is fine)
		err = store.Database.Insert(ownershipKey, ownership)
		if err != nil && !errors.Is(err, badgerhold.ErrKeyExists) {
			return fmt.Errorf("error inserting ownership: %w", err)
		}
	}

	// ===== STORE PARENT CACHE =====
	if leafData.Leaf.ParentHash != "" {
		// Use direct key lookup (O(1)) instead of Find query (O(n))
		parentCacheKey := root + ":" + leafData.Leaf.Hash
		var existingCache types.LeafParentCache
		err = store.Database.Get(parentCacheKey, &existingCache)

		if errors.Is(err, badgerhold.ErrNotFound) {
			parentCache := types.LeafParentCache{
				RootHash:   root,
				LeafHash:   leafData.Leaf.Hash,
				ParentHash: leafData.Leaf.ParentHash,
			}
			err = store.Database.Insert(parentCacheKey, parentCache)
			if err != nil && !errors.Is(err, badgerhold.ErrKeyExists) {
				return fmt.Errorf("error inserting parent cache: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("error checking parent cache: %w", err)
		}
	}

	if isRootLeaf {
	}

	// ===== STORE TAGS =====
	for key, value := range leafData.Leaf.AdditionalData {
		// Use direct key lookup (O(1)) instead of Find query (O(n))
		tagCompositeKey := root + ":" + leafData.PublicKey + ":" + leafData.Leaf.Hash + ":" + key

		var existingTag types.AdditionalDataEntry
		err = store.Database.Get(tagCompositeKey, &existingTag)

		// Only insert if tag doesn't exist
		if errors.Is(err, badgerhold.ErrNotFound) {
			entry := types.AdditionalDataEntry{
				Hash:  leafCID.Bytes(),
				Key:   key,
				Value: value,
			}
			err = store.Database.Insert(tagCompositeKey, entry)
			if err != nil && !errors.Is(err, badgerhold.ErrKeyExists) {
				return fmt.Errorf("error inserting tag: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("error checking tag existence: %w", err)
		}
	}

	// Handle stats recording asynchronously (not in critical path)
	if contentSize > 0 {
		go func() {
			fileName := filepath.Base(leafData.Leaf.ItemName)

			err := store.StatsDatabase.SaveFile(
				root,
				leafData.Leaf.Hash,
				fileName,
				mimeType,
				leafData.Leaf.LeafCount,
				contentSize,
			)
			if err != nil {
				logging.Infof("failed to record leaf file statistics: %v\n", err)
			}

			if len(leafData.Leaf.AdditionalData) > 0 {
				err = store.StatsDatabase.SaveTags(root, &leafData.Leaf)
				if err != nil {
					logging.Infof("failed to record leaf tags: %s\n", err.Error())
				}
			}
		}()
	}

	// Track stored leaf count
	count := storedLeafCount.Add(1)
	// Log progress every 1000 leaves
	if count%1000 == 0 {
	}

	return nil
}

// RetrieveLeafFast retrieves just the leaf content without ownership lookup.
// This is much faster for streaming/iteration where ownership info isn't needed.
func (store *BadgerholdStore) RetrieveLeafFast(hash string) (*merkle_dag.DagLeaf, error) {
	var leafContent types.LeafContent
	err := store.Database.Get(hash, &leafContent)
	if err != nil {
		if errors.Is(err, badgerhold.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to retrieve leaf content: %w", err)
	}

	return &merkle_dag.DagLeaf{
		Hash:              leafContent.Hash,
		ItemName:          leafContent.ItemName,
		Type:              leafContent.Type,
		ContentHash:       leafContent.ContentHash,
		ClassicMerkleRoot: leafContent.ClassicMerkleRoot,
		CurrentLinkCount:  leafContent.CurrentLinkCount,
		LeafCount:         leafContent.LeafCount,
		ContentSize:       leafContent.ContentSize,
		DagSize:           leafContent.DagSize,
		Links:             leafContent.Links,
		AdditionalData:    leafContent.AdditionalData,
	}, nil
}

// HasLeafGlobal checks if a leaf exists in the store by its content hash (globally, not scoped to a DAG).
// This is used for partial DAG uploads to check if referenced leaves already exist.
func (store *BadgerholdStore) HasLeafGlobal(hash string) (bool, error) {
	var leafContent types.LeafContent
	err := store.Database.Get(hash, &leafContent)
	if err != nil {
		if errors.Is(err, badgerhold.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check leaf existence: %w", err)
	}
	return true, nil
}

// GetLeafLinksGlobal returns the Links (child hashes) for a leaf by its content hash.
// This is used for partial DAG uploads to traverse and verify transitive dependencies exist.
func (store *BadgerholdStore) GetLeafLinksGlobal(hash string) ([]string, error) {
	var leafContent types.LeafContent
	err := store.Database.Get(hash, &leafContent)
	if err != nil {
		if errors.Is(err, badgerhold.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to retrieve leaf content: %w", err)
	}
	return leafContent.Links, nil
}

// ClaimOwnership allows a client to claim ownership over an existing DAG.
// The root must already exist in the store. Creates a new ownership record for the root.
// Multiple owners can claim ownership over the same DAG (each gets their own record).
func (store *BadgerholdStore) ClaimOwnership(root string, publicKey string, signature string) error {
	// Verify the root leaf exists
	exists, err := store.HasLeafGlobal(root)
	if err != nil {
		return fmt.Errorf("failed to check root existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("cannot claim ownership: root %s does not exist", root)
	}

	// Check if this owner already has ownership of this root
	// New format: root:pubkey
	// Old format (fallback): root:pubkey:root
	ownershipKey := root + ":" + publicKey
	var existingOwnership types.DagOwnership
	err = store.Database.Get(ownershipKey, &existingOwnership)
	if err != nil && errors.Is(err, badgerhold.ErrNotFound) {
		// Fallback: check old key format for backward compatibility
		oldOwnershipKey := root + ":" + publicKey + ":" + root
		err = store.Database.Get(oldOwnershipKey, &existingOwnership)
	}
	if err == nil {
		// Already owns it, update signature if different
		if existingOwnership.Signature != signature {
			existingOwnership.Signature = signature
			return store.Database.Update(ownershipKey, existingOwnership)
		}
		return nil // Already exists with same signature
	}
	if !errors.Is(err, badgerhold.ErrNotFound) {
		return fmt.Errorf("failed to check existing ownership: %w", err)
	}

	// Create new ownership record
	ownership := types.DagOwnership{
		Root:      root,
		PublicKey: publicKey,
		Signature: signature,
	}
	err = store.Database.Insert(ownershipKey, ownership)
	if err != nil && !errors.Is(err, badgerhold.ErrKeyExists) {
		return fmt.Errorf("failed to create ownership record: %w", err)
	}

	return nil
}

// GetOwnership returns all ownership records for a given DAG root.
func (store *BadgerholdStore) GetOwnership(root string) ([]types.DagOwnership, error) {
	var ownerships []types.DagOwnership
	err := store.Database.Find(&ownerships, badgerhold.Where("Root").Eq(root))
	if err != nil {
		if errors.Is(err, badgerhold.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query ownership records: %w", err)
	}
	return ownerships, nil
}

// FindRootForLeaf attempts to find the root hash that contains the given leaf hash.
// This is useful when a client provides a leaf hash instead of a root hash.
// Returns the root hash if found, empty string if not found.
func (store *BadgerholdStore) FindRootForLeaf(leafHash string) (string, error) {
	// First, check if this hash is itself a root (has ownership)
	var ownerships []types.DagOwnership
	err := store.Database.Find(&ownerships, badgerhold.Where("Root").Eq(leafHash))
	if err == nil && len(ownerships) > 0 {
		// This hash is a root itself
		return leafHash, nil
	}

	// Check if this leaf exists at all
	var leafContent types.LeafContent
	err = store.Database.Get(leafHash, &leafContent)
	if err != nil {
		if errors.Is(err, badgerhold.ErrNotFound) {
			return "", nil // Leaf not found at all
		}
		return "", fmt.Errorf("failed to query leaf content: %w", err)
	}

	// The leaf exists. Now we need to find which root it belongs to.
	// Query LeafParentCache entries that reference this leaf hash.
	// LeafParentCache entries have RootHash field we can use.
	var parentCaches []types.LeafParentCache
	err = store.Database.Find(&parentCaches, badgerhold.Where("LeafHash").Eq(leafHash).Limit(1))
	if err == nil && len(parentCaches) > 0 {
		return parentCaches[0].RootHash, nil
	}

	// If we can't find it in parent cache, this leaf might be a root itself without ownership
	// (orphaned DAG) or it's only accessible by scanning all DAGs
	return "", nil
}

// RetrieveLeaf retrieves a single DAG leaf, optionally filtering by public key.
// Ownership info is only included for root leaves (ownership is per-root, not per-leaf).
func (store *BadgerholdStore) RetrieveLeaf(root string, hash string, includeContent bool) (*types.DagLeafData, error) {
	isRootLeaf := root == hash

	rootCID, err := cid.Decode(root)
	if err != nil {
		return nil, fmt.Errorf("invalid root CID: %w", err)
	}

	// Retrieve the leaf content (content-addressed)
	var leafContent types.LeafContent
	err = store.Database.Get(hash, &leafContent)
	if err != nil {
		if errors.Is(err, badgerhold.ErrNotFound) {
			return nil, fmt.Errorf("leaf not found for hash %s", hash)
		}
		return nil, fmt.Errorf("failed to retrieve leaf content: %w", err)
	}

	// Construct DagLeafData - ownership fields are only populated for root leaf
	data := &types.DagLeafData{
		Leaf: merkle_dag.DagLeaf{
			Hash:              leafContent.Hash,
			ItemName:          leafContent.ItemName,
			Type:              leafContent.Type,
			ContentHash:       leafContent.ContentHash,
			ClassicMerkleRoot: leafContent.ClassicMerkleRoot,
			CurrentLinkCount:  leafContent.CurrentLinkCount,
			LeafCount:         leafContent.LeafCount,
			ContentSize:       leafContent.ContentSize,
			DagSize:           leafContent.DagSize,
			Links:             leafContent.Links,
			AdditionalData:    leafContent.AdditionalData,
		},
	}

	// Only fetch and include ownership data for the root leaf
	if isRootLeaf {
		var ownerships []types.DagOwnership
		err = store.Database.Find(&ownerships, badgerhold.Where("Root").Eq(root))
		if err != nil && err != badgerhold.ErrNotFound {
			return nil, fmt.Errorf("failed to query ownership: %w", err)
		}

		if len(ownerships) == 0 {
			// No ownership records means the DAG has been marked for deletion
			return nil, fmt.Errorf("DAG not available: no ownership records for root %s (marked for deletion)", root)
		}

		// Use the first ownership record
		// Clients should verify the signature matches their expected owner
		ownership := ownerships[0]
		data.PublicKey = ownership.PublicKey
		data.Signature = ownership.Signature
	}

	if includeContent && data.Leaf.ContentHash != nil {
		content, err := store.RetrieveContent(rootCID, data.Leaf.ContentHash)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve content: %w", err)
		}
		data.Leaf.Content = content
	}

	return data, nil
}

// QueryDag returns DAG root hashes that match the given filter.
// With the new ownership model, ownership is per-root only (not per-leaf).
// The function queries ownership records by pubkey, then optionally filters by leaf names/tags.
func (store *BadgerholdStore) QueryDag(filter lib_types.QueryFilter) ([]string, error) {
	var ownerships []types.DagOwnership
	var query *badgerhold.Query
	hasFilter := false

	// Add filtering by PublicKey
	if len(filter.PubKeys) > 0 {
		pubKeysAsInterface := make([]interface{}, len(filter.PubKeys))
		for i, pubKey := range filter.PubKeys {
			pubKeysAsInterface[i] = pubKey
		}
		query = badgerhold.Where("PublicKey").In(pubKeysAsInterface...)
		hasFilter = true
	}

	// If no pubkey filter, get all ownership records
	if !hasFilter {
		query = badgerhold.Where("Root").Ne("")
	}

	// Execute ownership query - this gives us all DAG roots matching the pubkey filter
	err := store.Database.Find(&ownerships, query)
	if err != nil && err != badgerhold.ErrNotFound {
		return nil, fmt.Errorf("failed to query ownership records: %w", err)
	}

	// Get unique root hashes from ownership records
	rootHashes := make(map[string]struct{})
	for _, ownership := range ownerships {
		rootHashes[ownership.Root] = struct{}{}
	}

	// If no name or tag filters, return root hashes directly
	if len(filter.Names) == 0 && len(filter.Tags) == 0 {
		hashes := make([]string, 0, len(rootHashes))
		for hash := range rootHashes {
			hashes = append(hashes, hash)
		}
		return hashes, nil
	}

	// For name/tag filters, we need to check if any leaf in each DAG matches
	// This requires traversing leaves for each root
	filteredRoots := make(map[string]struct{})

	for rootHash := range rootHashes {
		// Get all leaf hashes for this DAG from relationships cache
		relationships, err := store.RetrieveRelationships(rootHash)
		if err != nil {
			continue // Skip DAGs without relationships cache
		}

		// Collect all leaf hashes (including root)
		leafHashes := []string{rootHash}
		for leafHash := range relationships {
			leafHashes = append(leafHashes, leafHash)
		}

		// Check if any leaf matches the name filter
		if len(filter.Names) > 0 {
			nameFilter := make(map[string]bool)
			for _, name := range filter.Names {
				nameFilter[name] = true
			}

			nameMatched := false
			for _, hash := range leafHashes {
				var leafContent types.LeafContent
				err := store.Database.Get(hash, &leafContent)
				if err != nil {
					continue
				}
				if nameFilter[leafContent.ItemName] {
					nameMatched = true
					break
				}
			}
			if !nameMatched {
				continue // Skip this root, no name match
			}
		}

		// Check if any leaf matches all tag filters
		if len(filter.Tags) > 0 {
			tagMatched := false
			requiredMatches := len(filter.Tags)

			for _, hash := range leafHashes {
				hashCID, err := cid.Decode(hash)
				if err != nil {
					continue
				}

				matchCount := 0
				for tagKey, tagValue := range filter.Tags {
					// Check if this leaf has this tag with this value
					var tagEntries []types.AdditionalDataEntry
					err := store.Database.Find(&tagEntries,
						badgerhold.Where("Key").Eq(tagKey).And("Value").Eq(tagValue))
					if err != nil {
						continue
					}

					for _, entry := range tagEntries {
						entryCID, err := cid.Cast(entry.Hash)
						if err != nil {
							continue
						}
						if entryCID.String() == hashCID.String() {
							matchCount++
							break
						}
					}
				}

				if matchCount == requiredMatches {
					tagMatched = true
					break
				}
			}

			if !tagMatched {
				continue // Skip this root, no tag match
			}
		}

		// Root passed all filters
		filteredRoots[rootHash] = struct{}{}
	}

	// Convert to slice
	hashes := make([]string, 0, len(filteredRoots))
	for hash := range filteredRoots {
		hashes = append(hashes, hash)
	}

	return hashes, nil
}

func (store *BadgerholdStore) BuildDagFromStore(root string, includeContent bool) (*types.DagData, error) {
	return stores.BuildDagFromStore(store, root, includeContent)
}

func (store *BadgerholdStore) BuildPartialDagFromStore(root string, leafHashes []string, includeContent bool, pruneLinks bool) (*types.DagData, error) {
	return stores.BuildPartialDagFromStore(store, root, leafHashes, includeContent, pruneLinks)
}

func (store *BadgerholdStore) StoreDag(dag *types.DagData) error {
	return stores.StoreDag(store, dag)
}

func WrapLeaf(leaf *types.DagLeafData) *types.WrappedLeaf {
	if len(leaf.Leaf.ClassicMerkleRoot) <= 0 {
		leaf.Leaf.ClassicMerkleRoot = nil
	}

	return &types.WrappedLeaf{
		PublicKey:         leaf.PublicKey,
		Signature:         leaf.Signature,
		Hash:              leaf.Leaf.Hash,
		ItemName:          leaf.Leaf.ItemName,
		Type:              leaf.Leaf.Type,
		ContentHash:       leaf.Leaf.ContentHash,
		ClassicMerkleRoot: leaf.Leaf.ClassicMerkleRoot,
		CurrentLinkCount:  leaf.Leaf.CurrentLinkCount,
		LeafCount:         leaf.Leaf.LeafCount,
		ContentSize:       leaf.Leaf.ContentSize,
		DagSize:           leaf.Leaf.DagSize,
		Links:             leaf.Leaf.Links,
		// ParentHash is NOT stored - it's per-DAG, not per-leaf.
		// Use CacheRelationshipsStreaming() to store per-DAG relationships.
		AdditionalData: leaf.Leaf.AdditionalData,
	}
}

func UnwrapLeaf(leaf *types.WrappedLeaf) *types.DagLeafData {
	if len(leaf.ClassicMerkleRoot) <= 0 {
		leaf.ClassicMerkleRoot = nil
	}

	return &types.DagLeafData{
		PublicKey: leaf.PublicKey,
		Signature: leaf.Signature,
		Leaf: merkle_dag.DagLeaf{
			Hash:              leaf.Hash,
			ItemName:          leaf.ItemName,
			Type:              leaf.Type,
			ContentHash:       leaf.ContentHash,
			ClassicMerkleRoot: leaf.ClassicMerkleRoot,
			CurrentLinkCount:  leaf.CurrentLinkCount,
			LeafCount:         leaf.LeafCount,
			ContentSize:       leaf.ContentSize,
			DagSize:           leaf.DagSize,
			Links:             leaf.Links,
			// ParentHash is not persisted - retrieve from cached relationships if needed
			AdditionalData: leaf.AdditionalData,
		},
	}
}

func makeKey(parts ...interface{}) []byte {
	var key []byte

	for i, part := range parts {
		if i > 0 {
			key = append(key, 0x00)
		}

		switch v := part.(type) {
		case string:
			key = append(key, v...)
		case []byte:
			key = append(key, v...)
		default:
			panic(fmt.Sprintf("makeKey: unsupported type %T", v))
		}
	}

	return key
}
