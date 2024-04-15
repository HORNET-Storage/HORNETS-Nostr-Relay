package graviton

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/deroproject/graviton"
	"github.com/fxamacker/cbor/v2"
	"github.com/nbd-wtf/go-nostr"

	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"

	jsoniter "github.com/json-iterator/go"
)

type GravitonStore struct {
	Database *graviton.Store
}

func (store *GravitonStore) InitStore(args ...interface{}) error {
	db, err := graviton.NewDiskStore("gravitondb")
	//db, err := graviton.NewMemStore()
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
	if leaf.ContentHash != nil && leaf.Content == nil {
		return fmt.Errorf("Leaf has content hash but no content")
	}

	snapshot, err := store.Database.LoadSnapshot(0)
	if err != nil {
		return err
	}

	var contentTree *graviton.Tree = nil

	if leaf.Content != nil {
		contentTree, err = snapshot.GetTree("content")
		if err != nil {
			return err
		}

		err = contentTree.Put(leaf.ContentHash, leaf.Content)
		if err != nil {
			return err
		}

		leaf.Content = nil
	}

	var rootLeaf *merkle_dag.DagLeaf

	if leaf.Hash == root {
		rootLeaf = leaf
	} else {
		_rootLeaf, err := store.RetrieveLeaf(root, root, false)
		if err != nil {
			return err
		}

		rootLeaf = _rootLeaf
	}

	bucket := GetBucket(rootLeaf)

	fmt.Printf("Adding to bucket: %s\n", bucket)

	cborData, err := cbor.Marshal(leaf)
	if err != nil {
		return err
	}

	key := leaf.Hash // merkle_dag.GetHash(leaf.Hash)

	log.Printf("Adding key to block database: %s\n", key)

	tree, err := snapshot.GetTree(bucket)
	if err != nil {
		return err
	}

	err = tree.Put([]byte(key), cborData)
	if err != nil {
		return err
	}

	trees := []*graviton.Tree{}

	trees = append(trees, tree)

	if rootLeaf.Hash == leaf.Hash {
		indexTree, err := snapshot.GetTree("root_index")
		if err != nil {
			return err
		}

		indexTree.Put([]byte(root), []byte(bucket))

		trees = append(trees, indexTree)
	}

	if contentTree != nil {
		trees = append(trees, contentTree)
	}

	_, err = graviton.Commit(trees...)
	if err != nil {
		return err
	}

	return nil
}

func (store *GravitonStore) RetrieveLeafContent(contentHash []byte) ([]byte, error) {
	snapshot, err := store.Database.LoadSnapshot(0)
	if err != nil {
		return nil, err
	}

	contentTree, err := snapshot.GetTree("content")
	if err != nil {
		return nil, err
	}

	bytes, err := contentTree.Get(contentHash)
	if err != nil {
		return nil, err
	}

	if len(bytes) > 0 {
		return bytes, nil
	} else {
		return nil, fmt.Errorf("content not found")
	}
}

func (store *GravitonStore) retrieveBucket(root string) (string, error) {
	snapshot, err := store.Database.LoadSnapshot(0)
	if err != nil {
		return "", err
	}

	tree, err := snapshot.GetTree("root_index")
	if err != nil {
		return "", err
	}

	bytes, err := tree.Get([]byte(root))
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func (store *GravitonStore) RetrieveLeaf(root string, hash string, includeContent bool) (*merkle_dag.DagLeaf, error) {
	key := []byte(hash) // merkle_dag.GetHash(hash)

	snapshot, err := store.Database.LoadSnapshot(0)
	if err != nil {
		return nil, err
	}

	bucket, err := store.retrieveBucket(root)
	if err != nil {
		return nil, err
	}

	tree, err := snapshot.GetTree(bucket)
	if err != nil {
		return nil, err
	}

	log.Printf("Searching for leaf with key: %s\nFrom bucket: %s", key, bucket)
	bytes, err := tree.Get(key)
	if err != nil {
		return nil, err
	}

	var leaf *merkle_dag.DagLeaf = &merkle_dag.DagLeaf{}

	err = cbor.Unmarshal(bytes, leaf)
	if err != nil {
		return nil, err
	}

	if includeContent && leaf.ContentHash != nil {
		fmt.Println("Fetching  leaf content")

		content, err := store.RetrieveLeafContent(leaf.ContentHash)
		if err != nil {
			return nil, err
		}

		leaf.Content = content
	}

	fmt.Println("Leaf found")

	return leaf, nil
}

func (store *GravitonStore) BuildDagFromStore(root string, includeContent bool) (*merkle_dag.Dag, error) {
	return stores.BuildDagFromStore(store, root, includeContent)
}

func (store *GravitonStore) StoreDag(dag *merkle_dag.Dag) error {
	return stores.StoreDag(store, dag)
}

func (store *GravitonStore) QueryEvents(filter nostr.Filter) ([]*nostr.Event, error) {
	log.Println("Processing filter:", filter)

	var events []*nostr.Event

	ss, _ := store.Database.LoadSnapshot(0)

	for _, kind := range filter.Kinds {
		tree, _ := ss.GetTree(fmt.Sprintf("kind:%d", kind))

		c := tree.Cursor()

		for _, v, err := c.First(); err == nil; _, v, err = c.Next() {
			var event nostr.Event
			if err := jsoniter.Unmarshal(v, &event); err != nil {
				continue
			}

			if filter.Matches(&event) {
				events = append(events, &event)
			}
		}
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].CreatedAt > events[j].CreatedAt
	})

	if filter.Limit > 0 && len(events) > filter.Limit {
		events = events[:filter.Limit]
	}
	log.Println("Found", len(events), "matching events")

	return events, nil
}

func (store *GravitonStore) StoreEvent(event *nostr.Event) error {
	eventData, err := jsoniter.Marshal(event)
	if err != nil {
		return err
	}

	ss, _ := store.Database.LoadSnapshot(0)
	tree, _ := ss.GetTree(fmt.Sprintf("kind:%d", event.Kind))

	tree.Put([]byte(event.ID), eventData)

	graviton.Commit(tree)

	return nil
}

func (store *GravitonStore) DeleteEvent(eventID string) error {
	ss, _ := store.Database.LoadSnapshot(0)
	tree, _ := ss.GetTree("events")

	err := tree.Delete([]byte(eventID))
	if err != nil {
		return err
	} else {
		log.Println("Deleted event", eventID)
	}

	graviton.Commit(tree)

	return nil
}

func GetBucket(leaf *merkle_dag.DagLeaf) string {
	hkind, ok := leaf.AdditionalData["hkind"]
	if ok {
		if hkind != "1" {
			return fmt.Sprintf("hkind:%s", hkind)
		}
	}

	split := strings.Split(leaf.ItemName, ".")

	if len(split) > 1 {
		return split[1]
	} else {
		if leaf.Type == merkle_dag.DirectoryLeafType {
			return "directory"
		} else {
			return "file"
		}
	}
}
