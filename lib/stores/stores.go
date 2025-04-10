package stores

import (
	"fmt"
	"log"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/dag"
	lib_types "github.com/HORNET-Storage/go-hornet-storage-lib/lib"
	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics"
	"github.com/nbd-wtf/go-nostr"
)

type Store interface {
	Cleanup() error

	// Statistics Store
	GetStatsStore() statistics.StatisticsStore

	// Hornet Storage
	StoreLeaf(root string, leafData *types.DagLeafData, temp bool) error
	RetrieveLeaf(root string, hash string, includeContent bool, temp bool) (*types.DagLeafData, error)
	QueryDag(filter lib_types.QueryFilter, temp bool) ([]string, error)
	StoreDag(dag *types.DagData, temp bool) error
	BuildDagFromStore(root string, includeContent bool, temp bool) (*types.DagData, error)
	RetrieveLeafContent(contentHash []byte, temp bool) ([]byte, error)

	// Nostr
	QueryEvents(filter nostr.Filter) ([]*nostr.Event, error)
	StoreEvent(event *nostr.Event) error
	DeleteEvent(eventID string) error
	QueryBlobs(mimeType string) ([]string, error)

	// Blossom
	StoreBlob(data []byte, hash []byte, publicKey string) error
	GetBlob(hash string) ([]byte, error)
	DeleteBlob(hash string) error

	// Panel
	GetSubscriber(npub string) (*types.Subscriber, error)
	GetSubscriberByAddress(address string) (*types.Subscriber, error)
	SaveSubscriber(subscriber *types.Subscriber) error
	AllocateBitcoinAddress(npub string) (*types.Address, error)
	SaveAddress(addr *types.Address) error
	AllocateAddress() (*types.Address, error)
}

func BuildDagFromStore(store Store, root string, includeContent bool, temp bool) (*types.DagData, error) {
	builder := merkle_dag.CreateDagBuilder()

	var publicKey *string
	var signature *string
	var addLeavesRecursively func(builder *merkle_dag.DagBuilder, hash string) error

	addLeavesRecursively = func(builder *merkle_dag.DagBuilder, hash string) error {
		data, err := store.RetrieveLeaf(root, hash, includeContent, temp)
		if err != nil {
			log.Println("Unable to find leaf in the database:", err)
			return err
		}

		leaf := data.Leaf

		if leaf.Hash == root {
			publicKey = &data.PublicKey
			signature = &data.Signature

			err = leaf.VerifyRootLeaf()
			if err != nil {
				err = nil
			}
		} else {
			err = leaf.VerifyLeaf()
			if err != nil {
				err = nil
			}
		}

		if !includeContent {
			if leaf.Type == merkle_dag.FileLeafType {
				leaf.Links = make(map[string]string)

				builder.AddLeaf(&leaf, nil)

				return nil
			}
		}

		builder.AddLeaf(&leaf, nil)

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

	dag := builder.BuildDag(root)

	err := dag.Verify()
	if err != nil {
		fmt.Println("Failed to verify full dag")
		return nil, err
	}

	data := &types.DagData{
		PublicKey: *publicKey,
		Signature: *signature,
		Dag:       *dag,
	}

	return data, nil
}

func StoreDag(store Store, dag *types.DagData, temp bool) error {
	err := dag.Dag.IterateDag(func(leaf *merkle_dag.DagLeaf, parent *merkle_dag.DagLeaf) error {
		err := store.StoreLeaf(dag.Dag.Root, &types.DagLeafData{
			PublicKey: dag.PublicKey,
			Signature: dag.Signature,
			Leaf:      *leaf,
		}, temp)
		if err != nil {
			fmt.Println("Failed to store leaf: " + err.Error())
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
