package bbolt

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"strings"

	"github.com/HORNET-Storage/hornet-storage/lib/database/bbolt"
	"github.com/fxamacker/cbor/v2"

	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"
)

type BBoltStore struct {
	ContentDatabase *bbolt.Database
	UserDatabase    *bbolt.Database
}

func (store *BBoltStore) InitStore(args ...interface{}) error {
	prefix := "default"

	if len(args) > 0 {
		var ok bool

		prefix, ok = args[0].(string)

		if !ok {
			prefix = "default"
		}
	}

	content := strings.Join([]string{prefix, "content"}, "-")
	users := strings.Join([]string{prefix, "users"}, "-")

	contentDatabase, err := bbolt.CreateDatabase(content)
	if err != nil {
		return err
	}

	userDatabase, err := bbolt.CreateDatabase(users)
	if err != nil {
		return err
	}

	store.ContentDatabase = contentDatabase
	store.UserDatabase = userDatabase

	err = contentDatabase.CreateBucket("default")
	if err != nil {
		return err
	}

	err = userDatabase.CreateBucket("default")
	if err != nil {
		return err
	}

	return nil
}

func (store *BBoltStore) StoreLeaf(root string, leaf *merkle_dag.DagLeaf) error {
	if leaf.Data != nil {
		h := sha256.New()
		h.Write(leaf.Data)
		hashed := h.Sum(nil)

		store.ContentDatabase.UpdateValue("default", hex.EncodeToString(hashed), leaf.Data)

		leaf.Data = hashed
	}

	cborData, err := cbor.Marshal(leaf)
	if err != nil {
		return err
	}

	key := leaf.Hash // merkle_dag.GetHash(leaf.Hash)

	log.Printf("Adding key to block database: %s\n", key)

	err = store.UserDatabase.UpdateValue("default", key, cborData)
	if err != nil {
		return err
	}

	return nil
}

func (store *BBoltStore) RetrieveLeaf(root string, hash string) (*merkle_dag.DagLeaf, error) {
	key := hash // merkle_dag.GetHash(hash)

	log.Printf("Searching for leaf with key: %s\n", key)
	bytes, err := store.UserDatabase.GetValue("default", key)
	if err != nil {
		return nil, err
	}

	var leaf *merkle_dag.DagLeaf = &merkle_dag.DagLeaf{}

	err = cbor.Unmarshal(bytes, leaf)
	if err != nil {
		return nil, err
	}

	if leaf.Data != nil {
		bytes, err := store.ContentDatabase.GetValue("default", hex.EncodeToString(leaf.Data))
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

func (store *BBoltStore) BuildDagFromStore(root string) (*merkle_dag.Dag, error) {
	return stores.BuildDagFromStore(store, root)
}

func (store *BBoltStore) StoreDag(dag *merkle_dag.Dag) error {
	return stores.StoreDag(store, dag)
}
