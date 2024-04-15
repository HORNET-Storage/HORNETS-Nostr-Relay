package stores

import (
	"log"

	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"
	"github.com/nbd-wtf/go-nostr"
)

type Store interface {
	InitStore(args ...interface{}) error
	StoreLeaf(root string, leaf *merkle_dag.DagLeaf) error
	RetrieveLeaf(root string, hash string, includeContent bool) (*merkle_dag.DagLeaf, error)
	StoreDag(dag *merkle_dag.Dag) error
	BuildDagFromStore(root string, includeContent bool) (*merkle_dag.Dag, error)
	RetrieveLeafContent(contentHash []byte) ([]byte, error)
	QueryEvents(filter nostr.Filter) ([]*nostr.Event, error)
	StoreEvent(event *nostr.Event) error
	DeleteEvent(eventID string) error
}

func BuildDagFromStore(store Store, root string, includeContent bool) (*merkle_dag.Dag, error) {
	builder := merkle_dag.CreateDagBuilder()

	var addLeavesRecursively func(builder *merkle_dag.DagBuilder, hash string) error

	addLeavesRecursively = func(builder *merkle_dag.DagBuilder, hash string) error {
		leaf, err := store.RetrieveLeaf(root, hash, includeContent)
		if err != nil {
			log.Println("Unable to find leaf in the database:", err)
			return err
		}

		builder.AddLeaf(leaf, nil)

		for _, childHash := range leaf.Links {
			if err := addLeavesRecursively(builder, childHash); err != nil {
				log.Println("Error adding child leaf:", err)
			}
		}

		return nil
	}

	if err := addLeavesRecursively(builder, root); err != nil {
		log.Println("Failed to add leaves from database:", err)
		return nil, err
	}

	return builder.BuildDag(root), nil
}

func StoreDag(store Store, dag *merkle_dag.Dag) error {

	return nil
}
