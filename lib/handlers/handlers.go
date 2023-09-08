package handlers

import (
	"context"
	"crypto/sha256"
	"log"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib"

	"github.com/fxamacker/cbor/v2"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/multiformats/go-multibase"

	keys "github.com/HORNET-Storage/hornet-storage/lib/context"
	hornet_badger "github.com/HORNET-Storage/hornet-storage/lib/database/badger"
	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"
)

func UploadStreamHandler(stream network.Stream) {
	ctx := keys.GetContext()

	dec := cbor.NewDecoder(stream)

	blockDb := ctx.Value(keys.BlockDatabase).(*hornet_badger.BadgerDB)

	leaves := map[string]lib.UploadMessage{}

	for {
		var message lib.UploadMessage

		if err := dec.Decode(&message); err != nil {
			continue
		}

		encoding, _, err := multibase.Decode(message.Root)
		encoder := multibase.MustNewEncoder(encoding)

		if message.Leaf.Hash == message.Root {
			result, err := message.Leaf.VerifyRootLeaf(encoder)
			if err != nil {
				log.Fatal(err)
			}

			if !result {
				log.Printf("Failed to verify root leaf: %s\n", message.Leaf.Hash)

				stream.Close()
				return
			}
		} else {
			result, err := message.Leaf.VerifyLeaf(encoder)
			if err != nil {
				log.Fatal(err)
			}

			if !result {
				log.Printf("Failed to verify leaf: %s\n", message.Leaf.Hash)

				stream.Close()
				return
			}
		}

		SplitLeafContent(ctx, &message.Leaf)

		cborData, err := cbor.Marshal(message.Leaf)
		if err != nil {
			log.Fatal(err)
		}

		blockDb.Update(message.Leaf.Hash, cborData)

		leaves[message.Leaf.Hash] = message

		if len(leaves) == message.Count {
			dag := BuildDagFromDatabase(ctx, message.Root)

			result, err := dag.Verify(encoder)
			if err != nil {
				log.Fatal(err)
			}

			if !result {
				log.Printf("Failed to verify dag: %s\n", message.Root)
			}

			stream.Close()
			return
		}
	}
}

func DownloadStreamHandler(stream network.Stream) {
	//ctx := keys.GetContext()

	dec := cbor.NewDecoder(stream)
	//enc := cbor.NewEncoder(stream)

	//contentDb := ctx.Value(keys.ContentDatabase).(*hornet_badger.BadgerDB)
	//blockDb := ctx.Value(keys.BlockDatabase).(*hornet_badger.BadgerDB)

	var message lib.DownloadMessage

	timeout := time.NewTimer(5 * time.Second)

first:
	for {
		select {
		case <-timeout.C:
			stream.Close()
			return
		default:
			if err := dec.Decode(&message); err == nil {
				break first
			}
		}
	}

	if message.Hash != nil {

	} else if message.Range != nil {
		// Range of leaves by labels

	} else if message.Label != nil {
		// Specific Leaf from label

	} else if message.Hash != nil {
		// Specific Leaf from hash

	} else {
		// Entire dag

	}
}

func BuildDagFromDatabase(ctx context.Context, root string) *merkle_dag.Dag {
	blockDb := ctx.Value(keys.BlockDatabase).(*hornet_badger.BadgerDB)
	contentDb := ctx.Value(keys.ContentDatabase).(*hornet_badger.BadgerDB)

	encoding, _, err := multibase.Decode(root)
	encoder := multibase.MustNewEncoder(encoding)

	builder := merkle_dag.CreateDagBuilder()

	rootLeafBytes, err := blockDb.Get(root)
	if err != nil {
		log.Fatal(err)
	}

	log.Println(len(rootLeafBytes))

	var rootLeaf *merkle_dag.DagLeaf = &merkle_dag.DagLeaf{}

	err = cbor.Unmarshal(rootLeafBytes, rootLeaf)
	if err != nil {
		log.Fatal(err)
	}

	contentLeafBytes, err := contentDb.GetFromByteKey(rootLeaf.Data)
	if err != nil {
		log.Fatal(err)
	}

	rootLeaf.Data = contentLeafBytes

	builder.AddLeaf(rootLeaf, encoder, nil)

	AddLeavesFromDatabase(ctx, builder, encoder, rootLeaf)

	return builder.BuildDag(root)
}

func AddLeavesFromDatabase(ctx context.Context, builder *merkle_dag.DagBuilder, encoder multibase.Encoder, leaf *merkle_dag.DagLeaf) {
	blockDb := ctx.Value(keys.BlockDatabase).(*hornet_badger.BadgerDB)

	for _, hash := range leaf.Links {
		childLeafBytes, err := blockDb.Get(hash)
		if err != nil {
			log.Fatal(err)
		}

		var childLeaf *merkle_dag.DagLeaf = &merkle_dag.DagLeaf{}

		err = cbor.Unmarshal(childLeafBytes, childLeaf)
		if err != nil {
			log.Fatal(err)
		}

		err = RepairLeafContent(ctx, childLeaf)

		builder.AddLeaf(childLeaf, encoder, leaf)

		AddLeavesFromDatabase(ctx, builder, encoder, childLeaf)
	}
}

func SplitLeafContent(ctx context.Context, leaf *merkle_dag.DagLeaf) error {
	contentDb := ctx.Value(keys.ContentDatabase).(*hornet_badger.BadgerDB)

	h := sha256.New()
	h.Write(leaf.Data)
	hashed := h.Sum(nil)

	contentDb.UpdateFromByteKey(hashed, leaf.Data)

	leaf.Data = hashed

	return nil
}

func RepairLeafContent(ctx context.Context, leaf *merkle_dag.DagLeaf) error {
	contentDb := ctx.Value(keys.ContentDatabase).(*hornet_badger.BadgerDB)

	bytes, err := contentDb.GetFromByteKey(leaf.Data)
	if err != nil {
		return err
	}

	leaf.Data = bytes

	return nil
}
