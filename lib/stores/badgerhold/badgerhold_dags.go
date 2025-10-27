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
	db := store.Database.Badger()

	prefixes := []string{"leaf_", "tags_"}

	return db.Update(func(tx *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false

		// Delete leaves and tags by prefix
		for _, p := range prefixes {
			prefix := []byte(fmt.Sprintf("%s%s_", p, root))
			it := tx.NewIterator(opts)

			defer it.Close()

			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				item := it.Item()
				key := item.KeyCopy(nil)

				if err := tx.Delete(key); err != nil {
					return err
				}
			}

			it.Close()
		}

		// Delete cached labels
		labelsKey := makeKey("labels", root)
		if err := tx.Delete(labelsKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		return nil
	})
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

		wrappedLeaf := WrapLeaf(leafData)

		err = store.Database.TxUpsert(tx, makeKey("leaf", rootCID.Bytes(), leafCID.Bytes()), wrappedLeaf)
		if err != nil {
			return err
		}

		for key, value := range leafData.Leaf.AdditionalData {
			entry := types.AdditionalDataEntry{
				Hash:  leafCID.Bytes(),
				Key:   key,
				Value: value,
			}

			err = store.Database.TxUpsert(tx, makeKey("tags", rootCID.Bytes(), leafCID.Bytes(), key), entry)
			if err != nil {
				return err
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

// RetrieveLeaf retrieves a single DAG leaf from Badgerhold using the binary-safe key format.
func (store *BadgerholdStore) RetrieveLeaf(root string, hash string, includeContent bool) (*types.DagLeafData, error) {
	rootCID, err := cid.Decode(root)
	if err != nil {
		return nil, fmt.Errorf("invalid root CID: %w", err)
	}

	leafCID := rootCID
	if root != hash {
		leafCID, err = cid.Decode(hash)
		if err != nil {
			return nil, fmt.Errorf("invalid leaf CID: %w", err)
		}
	}

	key := makeKey("leaf", rootCID.Bytes(), leafCID.Bytes())

	var wrappedLeaf types.WrappedLeaf

	err = store.Database.Get(key, &wrappedLeaf)
	if err != nil {
		if errors.Is(err, badgerhold.ErrNotFound) {
			return nil, fmt.Errorf("leaf not found for root %s, hash %s", root, hash)
		}
		return nil, fmt.Errorf("failed to retrieve leaf: %w", err)
	}

	data := UnwrapLeaf(&wrappedLeaf)

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
	var results []types.WrappedLeaf

	// Start query with a dummy condition
	query := badgerhold.Where("Hash").Ne("") // Ensures chaining works
	first := true

	// Add filtering by PublicKey
	if len(filter.PubKeys) > 0 {
		pubKeysAsInterface := make([]interface{}, len(filter.PubKeys))
		for i, pubKey := range filter.PubKeys {
			pubKeysAsInterface[i] = pubKey
		}

		if first {
			query = badgerhold.Where("PublicKey").In(pubKeysAsInterface...)
			first = false
		} else {
			query = query.And("PublicKey").In(pubKeysAsInterface...)
		}
	}

	// Add filtering by ItemName
	if len(filter.Names) > 0 {
		namesAsInterface := make([]interface{}, len(filter.Names))
		for i, name := range filter.Names {
			namesAsInterface[i] = name
		}

		if first {
			query = badgerhold.Where("ItemName").In(namesAsInterface...)
			first = false
		} else {
			query = query.And("ItemName").In(namesAsInterface...)
		}
	}

	// Execute the primary query
	err := store.Database.Find(&results, query)
	if err != nil && err != badgerhold.ErrNotFound {
		return nil, fmt.Errorf("failed to query WrappedLeaf: %w", err)
	}

	// Extract hashes from primary results
	hashSet := make(map[string]struct{})
	for _, leaf := range results {
		hashSet[leaf.Hash] = struct{}{}
	}

	var entries []types.AdditionalDataEntry
	err = store.Database.Find(&entries, badgerhold.Where("Key").Ne(""))
	if err != nil {
		return nil, err
	}

	// If we have tag filters, run a secondary query to filter based on tags
	if len(filter.Tags) > 0 {
		for tagKey, tagValue := range filter.Tags {
			var tagEntries []types.AdditionalDataEntry

			err := store.Database.Find(&tagEntries, badgerhold.Where("Key").Eq(tagKey).And("Value").Eq(tagValue))
			if err != nil && err != badgerhold.ErrNotFound {
				return nil, fmt.Errorf("failed to query AdditionalDataEntry for key=%s, value=%s: %w", tagKey, tagValue, err)
			}

			// Keep only hashes that match the tag query
			tempHashSet := make(map[string]struct{})
			for _, entry := range tagEntries {
				// Convert entry.Hash ([]byte) to string for map lookup
				hashStr := string(entry.Hash)
				if _, exists := hashSet[hashStr]; exists { // Keep only those already in our result set
					tempHashSet[hashStr] = struct{}{}
				}
			}
			hashSet = tempHashSet // Update result set to only include tag-matching hashes
		}
	}

	// Convert hashSet to a slice of strings
	hashes := make([]string, 0, len(hashSet))
	for hash := range hashSet {
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
