package badgerhold

import (
	"errors"
	"fmt"
	"path/filepath"

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

		return nil
	})

	return err
}

func (store *BadgerholdStore) StoreLeaf(root string, leafData *types.DagLeafData) error {
	if leafData.Leaf.ContentHash != nil && leafData.Leaf.Content == nil {
		return fmt.Errorf("leaf has content hash but no content")
	}

	rootCID, err := cid.Decode(root)
	if err != nil {
		return fmt.Errorf("invalid root CID: %w", err)
	}

	leafCID := rootCID
	if root != leafData.Leaf.Hash {
		leafCID, err = cid.Decode(leafData.Leaf.Hash)
		if err != nil {
			return fmt.Errorf("invalid leaf CID: %w", err)
		}
	}

	return store.Database.Badger().Update(func(tx *badger.Txn) error {
		var err error
		var contentSize int64
		var mimeType string

		if leafData.Leaf.Content != nil {
			contentSize = int64(len(leafData.Leaf.Content))
			mtype := mimetype.Detect(leafData.Leaf.Content)
			mimeType = mtype.String()

			// Use raw Badger Set for []byte content (BadgerHold requires named types)
			err = tx.Set(makeKey("content", leafData.Leaf.ContentHash), leafData.Leaf.Content)
			if err != nil {
				return err
			}

			leafData.Leaf.Content = nil
		}

		// Store leaf content once (content-addressed by hash only)
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
			ParentHash:        leafData.Leaf.ParentHash,
			AdditionalData:    leafData.Leaf.AdditionalData,
		}

		// Store leaf content with hash as key (enables BadgerHold indexing)
		err = store.Database.TxInsert(tx, leafData.Leaf.Hash, leafContent)
		if err != nil {
			if err == badgerhold.ErrKeyExists {
				// Leaf already exists, update it (in case metadata changed)
				err = store.Database.TxUpdate(tx, leafData.Leaf.Hash, leafContent)
			}
			if err != nil {
				return err
			}
		}

		// Store ownership record (root + pubkey + leaf hash)
		ownership := types.DagOwnership{
			Root:      root,
			PublicKey: leafData.PublicKey,
			Signature: leafData.Signature,
			LeafHash:  leafData.Leaf.Hash,
		}

		ownershipKey := makeKey("ownership", root, leafData.PublicKey, leafData.Leaf.Hash)
		err = store.Database.TxInsert(tx, ownershipKey, ownership)
		if err != nil {
			if err == badgerhold.ErrKeyExists {
				err = store.Database.TxUpdate(tx, ownershipKey, ownership)
			}
			if err != nil {
				return err
			}
		}

		for key, value := range leafData.Leaf.AdditionalData {
			entry := types.AdditionalDataEntry{
				Hash:  leafCID.Bytes(),
				Key:   key,
				Value: value,
			}

			// Store tags with indexed keys for efficient queries
			tagKey := makeKey("tags", root, leafData.PublicKey, leafData.Leaf.Hash, key)
			err = store.Database.TxInsert(tx, tagKey, entry)
			if err != nil {
				if err == badgerhold.ErrKeyExists {
					err = store.Database.TxUpdate(tx, tagKey, entry)
				}
				if err != nil {
					return err
				}
			}
		}

		if contentSize > 0 {
			go func() {
				fileName := filepath.Base(leafData.Leaf.ItemName)

				err = store.StatsDatabase.SaveFile(
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

		return nil
	})
}

// RetrieveLeaf retrieves a single DAG leaf, optionally filtering by public key.
// If pubKey is empty and multiple signatures exist, returns the first one found.
func (store *BadgerholdStore) RetrieveLeaf(root string, hash string, includeContent bool) (*types.DagLeafData, error) {
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

	// Find ownership record for this root+hash
	// Try to find ownership records matching this root and leaf hash
	var ownerships []types.DagOwnership
	err = store.Database.Find(&ownerships, badgerhold.Where("Root").Eq(root).And("LeafHash").Eq(hash))
	if err != nil && err != badgerhold.ErrNotFound {
		return nil, fmt.Errorf("failed to query ownership: %w", err)
	}

	if len(ownerships) == 0 {
		return nil, fmt.Errorf("no ownership record found for root %s, hash %s", root, hash)
	}

	// Use the first ownership record (fallback behavior when pubKey not specified)
	ownership := ownerships[0]

	// Construct DagLeafData
	data := &types.DagLeafData{
		PublicKey: ownership.PublicKey,
		Signature: ownership.Signature,
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
			ParentHash:        leafContent.ParentHash,
			AdditionalData:    leafContent.AdditionalData,
		},
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

	// Execute ownership query
	err := store.Database.Find(&ownerships, query)
	if err != nil && err != badgerhold.ErrNotFound {
		return nil, fmt.Errorf("failed to query ownership records: %w", err)
	}

	// Get unique leaf hashes from ownership records
	leafHashes := make(map[string]struct{})
	for _, ownership := range ownerships {
		leafHashes[ownership.LeafHash] = struct{}{}
	}

	// Now filter by ItemName if needed (requires querying LeafContent)
	if len(filter.Names) > 0 {
		nameFilter := make(map[string]bool)
		for _, name := range filter.Names {
			nameFilter[name] = true
		}

		filteredHashes := make(map[string]struct{})
		for hash := range leafHashes {
			var leafContent types.LeafContent
			err := store.Database.Get(hash, &leafContent)
			if err != nil {
				continue // Skip if not found
			}

			if nameFilter[leafContent.ItemName] {
				filteredHashes[hash] = struct{}{}
			}
		}
		leafHashes = filteredHashes
	}

	// If we have tag filters, apply them
	if len(filter.Tags) > 0 {
		tagMatchMap := make(map[string]int) // hash -> count of matching tags
		requiredMatches := len(filter.Tags)

		for tagKey, tagValue := range filter.Tags {
			var tagEntries []types.AdditionalDataEntry

			// Query tags using indexed fields (Key and Value)
			err := store.Database.Find(&tagEntries, badgerhold.Where("Key").Eq(tagKey).And("Value").Eq(tagValue))
			if err != nil && err != badgerhold.ErrNotFound {
				return nil, fmt.Errorf("failed to query AdditionalDataEntry for key=%s, value=%s: %w", tagKey, tagValue, err)
			}

			// Count matching tags for each hash
			for _, entry := range tagEntries {
				hashCID, err := cid.Cast(entry.Hash)
				if err != nil {
					continue // Skip invalid hashes
				}
				hashStr := hashCID.String()

				// Only count if this hash is in our result set
				if _, inSet := leafHashes[hashStr]; inSet {
					tagMatchMap[hashStr]++
				}
			}
		}

		// Keep only hashes that matched ALL required tags
		filteredHashes := make(map[string]struct{})
		for hash, count := range tagMatchMap {
			if count == requiredMatches {
				filteredHashes[hash] = struct{}{}
			}
		}
		leafHashes = filteredHashes
	}

	// Convert hashSet to a slice of strings
	hashes := make([]string, 0, len(leafHashes))
	for hash := range leafHashes {
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
		ParentHash:        leaf.Leaf.ParentHash,
		AdditionalData:    leaf.Leaf.AdditionalData,
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
			ParentHash:        leaf.ParentHash,
			AdditionalData:    leaf.AdditionalData,
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
