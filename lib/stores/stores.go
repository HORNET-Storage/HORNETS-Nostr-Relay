package stores

import (
	"log"

	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"
	"github.com/multiformats/go-multibase"
)

type Store interface {
	InitStore(args ...interface{}) error
	StoreLeaf(root string, leaf *merkle_dag.DagLeaf) error
	RetrieveLeaf(root string, hash string) (*merkle_dag.DagLeaf, error)
	StoreDag(dag *merkle_dag.Dag) error
	BuildDagFromStore(root string) (*merkle_dag.Dag, error)
}

func BuildDagFromStore(store Store, root string) (*merkle_dag.Dag, error) {
	encoding, _, err := multibase.Decode(root)
	if err != nil {
		log.Println("Failed to discover encoding:", err)
		return nil, err
	}
	encoder := multibase.MustNewEncoder(encoding)

	builder := merkle_dag.CreateDagBuilder()

	var addLeavesRecursively func(builder *merkle_dag.DagBuilder, encoder multibase.Encoder, hash string) error

	addLeavesRecursively = func(builder *merkle_dag.DagBuilder, encoder multibase.Encoder, hash string) error {
		leaf, err := store.RetrieveLeaf(root, hash)
		if err != nil {
			log.Println("Unable to find leaf in the database:", err)
			return err
		}

		builder.AddLeaf(leaf, encoder, nil)

		for _, childHash := range leaf.Links {
			if err := addLeavesRecursively(builder, encoder, childHash); err != nil {
				log.Println("Error adding child leaf:", err)
			}
		}

		return nil
	}

	if err := addLeavesRecursively(builder, encoder, root); err != nil {
		log.Println("Failed to add leaves from database:", err)
		return nil, err
	}

	return builder.BuildDag(root), nil
}

func StoreDag(store Store, dag *merkle_dag.Dag) error {

	return nil
}
