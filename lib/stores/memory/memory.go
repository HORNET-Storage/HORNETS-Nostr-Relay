package memory

import (
	"fmt"
	"log"

	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"
)

type MemoryStore struct {
	Leaves  map[string]*merkle_dag.DagLeaf
	Content map[string][]byte
}

func (store *MemoryStore) InitStore(args ...interface{}) error {
	store.Leaves = map[string]*merkle_dag.DagLeaf{}

	return nil
}

func (store *MemoryStore) StoreLeaf(leaf *merkle_dag.DagLeaf) error {
	key := leaf.Hash // merkle_dag.GetHash(leaf.Hash)

	log.Printf("Adding key to block database: %s\n", key)

	store.Leaves[key] = leaf

	return nil
}

func (store *MemoryStore) RetrieveLeaf(hash string) (*merkle_dag.DagLeaf, error) {
	key := hash // merkle_dag.GetHash(hash)

	log.Printf("Searching for leaf with key: %s\n", key)
	leaf, ok := store.Leaves[key]
	if !ok {
		return nil, fmt.Errorf("leaf is missing from the memory store")
	}

	return leaf, nil
}

func (store *MemoryStore) BuildDagFromStore(root string) (*merkle_dag.Dag, error) {
	return stores.BuildDagFromStore(store, root)
}

func (store *MemoryStore) StoreDag(dag *merkle_dag.Dag) error {
	return stores.StoreDag(store, dag)
}
