package stores

import (
	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/dag"
	lib_types "github.com/HORNET-Storage/go-hornet-storage-lib/lib"
	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
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

	// Moderation
	AddToPendingModeration(eventID string, imageURLs []string) error
	RemoveFromPendingModeration(eventID string) error
	IsPendingModeration(eventID string) (bool, error)
	GetPendingModerationEvents() ([]types.PendingModeration, error)
	GetAndRemovePendingModeration(batchSize int) ([]types.PendingModeration, error)
	MarkEventBlocked(eventID string, timestamp int64) error
	MarkEventBlockedWithDetails(eventID string, timestamp int64, reason string, contentLevel int, mediaURL string) error
	DeleteBlockedEventsOlderThan(age int64) (int, error)
	DeleteResolutionEventsOlderThan(age int64) (int, error)
	IsEventBlocked(eventID string) (bool, error)
	UnmarkEventBlocked(eventID string) error

	// Dispute Moderation
	AddToPendingDisputeModeration(disputeID string, ticketID string, eventID string, mediaURL string, disputeReason string, userPubKey string) error
	RemoveFromPendingDisputeModeration(disputeID string) error
	IsPendingDisputeModeration(disputeID string) (bool, error)
	GetPendingDisputeModerationEvents() ([]types.PendingDisputeModeration, error)
	GetAndRemovePendingDisputeModeration(batchSize int) ([]types.PendingDisputeModeration, error)
	MarkEventDisputed(eventID string) error
	HasEventDispute(eventID string) (bool, error)
	HasUserDisputedEvent(eventID string, userPubKey string) (bool, error)

	// Pubkey Blocking
	IsBlockedPubkey(pubkey string) (bool, error)
	BlockPubkey(pubkey string, reason string) error
	UnblockPubkey(pubkey string) error
	ListBlockedPubkeys() ([]types.BlockedPubkey, error)

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
			logging.Infof("Unable to find leaf in the database:%s", err)
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
				logging.Infof("Error adding child leaf:%s", err)
			}
		}

		return nil
	}

	if err := addLeavesRecursively(builder, root); err != nil {
		logging.Infof("Failed to add leaves from database:%s", err)
		return nil, err
	}

	dag := builder.BuildDag(root)

	err := dag.Verify()
	if err != nil {
		logging.Infof("Failed to verify full dag")
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
			logging.Infof("Failed to store leaf: " + err.Error())
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
