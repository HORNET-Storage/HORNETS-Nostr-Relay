package badgerhold

import (
	"errors"
	"fmt"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/v2/dag"
	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/dgraph-io/badger/v4"
	"github.com/ipfs/go-cid"
	"github.com/timshannon/badgerhold/v4"
)

// BadgerholdLeafStore adapts BadgerholdStore to implement dag.LeafStore interface.
type BadgerholdLeafStore struct {
	store     *BadgerholdStore
	root      string
	publicKey string
	signature string
}

func NewBadgerholdLeafStore(store *BadgerholdStore, root string, publicKey string, signature string) *BadgerholdLeafStore {
	return &BadgerholdLeafStore{
		store:     store,
		root:      root,
		publicKey: publicKey,
		signature: signature,
	}
}

func (s *BadgerholdLeafStore) StoreLeaf(leaf *merkle_dag.DagLeaf) error {
	leafData := &types.DagLeafData{
		PublicKey: s.publicKey,
		Signature: s.signature,
		Leaf:      *leaf,
	}
	return s.store.StoreLeaf(s.root, leafData)
}

func (s *BadgerholdLeafStore) StoreLeaves(leaves []*merkle_dag.DagLeaf) error {
	leafDataList := make([]*types.DagLeafData, len(leaves))
	for i, leaf := range leaves {
		leafDataList[i] = &types.DagLeafData{
			PublicKey: s.publicKey,
			Signature: s.signature,
			Leaf:      *leaf,
		}
	}
	return s.store.StoreLeavesBatch(s.root, leafDataList)
}

func (s *BadgerholdLeafStore) RetrieveLeaves(hashes []string) (map[string]*merkle_dag.DagLeaf, error) {
	result := make(map[string]*merkle_dag.DagLeaf)
	for _, hash := range hashes {
		leaf, err := s.RetrieveLeaf(hash)
		if err != nil {
			return nil, err
		}
		if leaf != nil {
			result[hash] = leaf
		}
	}
	return result, nil
}

func (s *BadgerholdLeafStore) RetrieveLeaf(hash string) (*merkle_dag.DagLeaf, error) {
	// Use fast retrieval that skips ownership lookup for better performance
	return s.store.RetrieveLeafFast(hash)
}

func (s *BadgerholdLeafStore) HasLeaf(hash string) (bool, error) {
	var leafContent types.LeafContent
	err := s.store.Database.Get(hash, &leafContent)
	if err != nil {
		if errors.Is(err, badgerhold.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *BadgerholdLeafStore) DeleteLeaf(hash string) error {
	// Delete the leaf content
	err := s.store.Database.Delete(hash, types.LeafContent{})
	if err != nil && !errors.Is(err, badgerhold.ErrNotFound) {
		return err
	}
	return nil
}

func (s *BadgerholdLeafStore) Count() int {
	var ownerships []types.DagOwnership
	err := s.store.Database.Find(&ownerships, badgerhold.Where("Root").Eq(s.root))
	if err != nil {
		return 0
	}
	return len(ownerships)
}

func (s *BadgerholdLeafStore) GetAllLeafHashes() []string {
	var ownerships []types.DagOwnership
	err := s.store.Database.Find(&ownerships, badgerhold.Where("Root").Eq(s.root))
	if err != nil {
		return nil
	}

	hashes := make([]string, 0, len(ownerships))
	seen := make(map[string]bool)
	for _, ownership := range ownerships {
		if !seen[ownership.LeafHash] {
			hashes = append(hashes, ownership.LeafHash)
			seen[ownership.LeafHash] = true
		}
	}
	return hashes
}

func (s *BadgerholdLeafStore) GetCachedRelationships() map[string]string {
	relationships, err := s.store.RetrieveRelationships(s.root)
	if err != nil {
		return nil
	}
	return relationships
}

// BadgerholdContentStore adapts BadgerholdStore to implement dag.ContentStore interface.
type BadgerholdContentStore struct {
	store *BadgerholdStore
	root  string
}

func NewBadgerholdContentStore(store *BadgerholdStore, root string) *BadgerholdContentStore {
	return &BadgerholdContentStore{
		store: store,
		root:  root,
	}
}

func (s *BadgerholdContentStore) StoreContent(hash string, content []byte) error {
	key := makeKey("content", []byte(hash))

	// Check if content exists using a read-only transaction (no conflicts)
	contentExists := false
	err := s.store.Database.Badger().View(func(tx *badger.Txn) error {
		_, err := tx.Get(key)
		if err == badger.ErrKeyNotFound {
			return nil // Content doesn't exist
		}
		if err != nil {
			return err
		}
		contentExists = true
		return nil
	})
	if err != nil {
		return err
	}

	// Only write content if it doesn't exist
	if !contentExists {
		return s.store.Database.Badger().Update(func(tx *badger.Txn) error {
			// Double-check inside transaction in case of race
			_, err := tx.Get(key)
			if err == badger.ErrKeyNotFound {
				return tx.Set(key, content)
			}
			if err != nil {
				return err
			}
			// Content was written by another goroutine, that's fine
			return nil
		})
	}

	return nil
}

func (s *BadgerholdContentStore) RetrieveContent(hash string) ([]byte, error) {
	rootCID, err := cid.Decode(s.root)
	if err != nil {
		return nil, err
	}
	return s.store.RetrieveContent(rootCID, []byte(hash))
}

func (s *BadgerholdContentStore) HasContent(hash string) (bool, error) {
	var content []byte
	err := s.store.Database.Badger().View(func(tx *badger.Txn) error {
		key := makeKey("content", []byte(hash))
		item, err := tx.Get(key)
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			content = val
			return nil
		})
	})
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return false, nil
		}
		return false, err
	}
	return len(content) > 0, nil
}

func (s *BadgerholdContentStore) DeleteContent(hash string) error {
	return s.store.Database.Badger().Update(func(tx *badger.Txn) error {
		key := makeKey("content", []byte(hash))
		return tx.Delete(key)
	})
}

func (store *BadgerholdStore) CreateDagStoreForRoot(root string, publicKey string, signature string) *merkle_dag.DagStore {
	leafStore := NewBadgerholdLeafStore(store, root, publicKey, signature)
	contentStore := NewBadgerholdContentStore(store, root)

	return merkle_dag.NewEmptyDagStoreWithOptions(
		merkle_dag.WithLeafStore(leafStore),
		merkle_dag.WithContentStore(contentStore),
		merkle_dag.WithSeparateContent(true),
	)
}

func (store *BadgerholdStore) CreateDagStoreFromExisting(root string) (*merkle_dag.DagStore, error) {
	// Get root leaf to retrieve public key and signature
	rootData, err := store.RetrieveLeaf(root, root, false)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve root leaf: %w", err)
	}

	leafStore := NewBadgerholdLeafStore(store, root, rootData.PublicKey, rootData.Signature)
	contentStore := NewBadgerholdContentStore(store, root)

	dagStore := merkle_dag.NewEmptyDagStoreWithOptions(
		merkle_dag.WithLeafStore(leafStore),
		merkle_dag.WithContentStore(contentStore),
		merkle_dag.WithSeparateContent(true),
	)

	// Set the root
	if err := dagStore.SetRoot(root); err != nil {
		return nil, fmt.Errorf("failed to set root: %w", err)
	}

	return dagStore, nil
}

func (store *BadgerholdStore) VerifyDagStreaming(root string) error {
	dagStore, err := store.CreateDagStoreFromExisting(root)
	if err != nil {
		return err
	}
	return dagStore.VerifyStreaming()
}

func (store *BadgerholdStore) StreamLeaves(root string, includeContent bool, callback func(leaf *merkle_dag.DagLeaf, parent *merkle_dag.DagLeaf) error) error {
	dagStore, err := store.CreateDagStoreFromExisting(root)
	if err != nil {
		return err
	}

	return dagStore.IterateDagStreaming(func(leaf *merkle_dag.DagLeaf, parent *merkle_dag.DagLeaf) error {
		// Optionally load content
		if includeContent && len(leaf.ContentHash) > 0 {
			rootCID, err := cid.Decode(root)
			if err != nil {
				return err
			}
			content, err := store.RetrieveContent(rootCID, leaf.ContentHash)
			if err != nil {
				return err
			}
			leaf.Content = content
		}
		return callback(leaf, parent)
	})
}

func (store *BadgerholdStore) GetLeafSequenceStreaming(root string, includeContent bool, callback func(packet *merkle_dag.TransmissionPacket, index int, total int) error) error {
	dagStore, err := store.CreateDagStoreFromExisting(root)
	if err != nil {
		return err
	}

	// First count total leaves
	count, err := dagStore.CountLeavesStreaming()
	if err != nil {
		return err
	}

	index := 0

	return dagStore.IterateDagStreaming(func(leaf *merkle_dag.DagLeaf, parent *merkle_dag.DagLeaf) error {
		// Optionally load content
		if includeContent && len(leaf.ContentHash) > 0 {
			rootCID, err := cid.Decode(root)
			if err != nil {
				return err
			}
			content, err := store.RetrieveContent(rootCID, leaf.ContentHash)
			if err != nil {
				return err
			}
			leaf.Content = content
		}

		// Build transmission packet
		var parentHash string
		proofs := make(map[string]*merkle_dag.ClassicTreeBranch)

		if parent != nil {
			parentHash = parent.Hash

			// Generate proof if parent has multiple children
			if parent.CurrentLinkCount > 1 {
				branch, err := parent.GetBranch(leaf.Hash)
				if err == nil && branch != nil {
					proofs[leaf.Hash] = branch
				}
			}
		}

		packet := &merkle_dag.TransmissionPacket{
			Leaf:       leaf,
			ParentHash: parentHash,
			Proofs:     proofs,
		}

		err := callback(packet, index, count)
		index++
		return err
	})
}
