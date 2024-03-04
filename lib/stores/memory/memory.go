package graviton

import (
	"crypto/sha256"
	"log"

	"github.com/deroproject/graviton"
	"github.com/fxamacker/cbor/v2"

	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"
)

type GravitonStore struct {
	Database *graviton.Store
}

func (store *GravitonStore) InitStore(args ...interface{}) error {
	//db, err := graviton.NewDiskStore("gravitondb")
	db, err := graviton.NewMemStore()
	if err != nil {
		return err
	}

	store.Database = db

	snapshot, err := db.LoadSnapshot(0)
	if err != nil {
		return err
	}

	tree, err := snapshot.GetTree("content")
	if err != nil {
		return err
	}

	_, err = graviton.Commit(tree)
	if err != nil {
		return err
	}

	return nil
}

func (store *GravitonStore) StoreLeaf(root string, leaf *merkle_dag.DagLeaf) error {
	snapshot, err := store.Database.LoadSnapshot(0)
	if err != nil {
		return err
	}

	var contentTree *graviton.Tree = nil

	if leaf.Data != nil {
		h := sha256.New()
		h.Write(leaf.Data)
		hashed := h.Sum(nil)

		contentTree, err = snapshot.GetTree("content")
		if err != nil {
			return err
		}

		err = contentTree.Put(hashed, leaf.Data)
		if err != nil {
			return err
		}

		leaf.Data = hashed
	}

	cborData, err := cbor.Marshal(leaf)
	if err != nil {
		return err
	}

	key := leaf.Hash // merkle_dag.GetHash(leaf.Hash)

	log.Printf("Adding key to block database: %s\n", key)

	tree, err := snapshot.GetTree(root)
	if err != nil {
		return err
	}

	err = tree.Put([]byte(key), cborData)
	if err != nil {
		return err
	}

	if contentTree != nil {
		_, err = graviton.Commit(tree, contentTree)
		if err != nil {
			return err
		}
	} else {
		_, err = graviton.Commit(tree)
		if err != nil {
			return err
		}
	}

	return nil
}

func (store *GravitonStore) RetrieveLeaf(root string, hash string) (*merkle_dag.DagLeaf, error) {
	key := []byte(hash) // merkle_dag.GetHash(hash)

	snapshot, err := store.Database.LoadSnapshot(0)
	if err != nil {
		return nil, err
	}

	tree, err := snapshot.GetTree(root)
	if err != nil {
		return nil, err
	}

	log.Printf("Searching for leaf with key: %s\n", key)
	bytes, err := tree.Get(key)
	if err != nil {
		return nil, err
	}

	var leaf *merkle_dag.DagLeaf = &merkle_dag.DagLeaf{}

	err = cbor.Unmarshal(bytes, leaf)
	if err != nil {
		return nil, err
	}

	if leaf.Data != nil {
		contentTree, err := snapshot.GetTree("content")
		if err != nil {
			return nil, err
		}

		bytes, err := contentTree.Get(leaf.Data)
		if err != nil {
			return nil, err
		}

		if len(bytes) > 0 {
			leaf.Data = bytes
		} else {
			leaf.Data = nil
		}
	}

	return leaf, nil
}

func (store *GravitonStore) BuildDagFromStore(root string) (*merkle_dag.Dag, error) {
	return stores.BuildDagFromStore(store, root)
}

func (store *GravitonStore) StoreDag(dag *merkle_dag.Dag) error {
	return stores.StoreDag(store, dag)
}
