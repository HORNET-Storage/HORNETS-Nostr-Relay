package handlers

import (
	"context"
	"crypto/sha256"
	"fmt"
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

		key := merkle_dag.GetHash(message.Leaf.Hash)

		blockDb.Update(key, cborData)

		leaves[message.Leaf.Hash] = message

		if len(leaves) == message.Count {
			dag, err := BuildDagFromDatabase(ctx, message.Root)
			if err != nil {
				WriteErrorToStream(stream, "Failed to build dag from provided leaves: %e", err)

				stream.Close()
				return
			}

			result, err := dag.Verify(encoder)
			if err != nil {
				WriteErrorToStream(stream, "Failed to verify dag: %e", err)

				stream.Close()
				return
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
	ctx := keys.GetContext()

	dec := cbor.NewDecoder(stream)
	enc := cbor.NewEncoder(stream)

	blockDb := ctx.Value(keys.BlockDatabase).(*hornet_badger.BadgerDB)

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

	// Ensure the node is storing the root leaf
	blockDb.Get(message.Root)

	if message.Hash != nil {

	} else if message.Range != nil {
		// Range of leaves by labels

	} else if message.Label != nil {
		// Specific Leaf from label

	} else if message.Hash != nil {
		// Specific Leaf from hash

	} else {
		// Entire dag

		dag, err := BuildDagFromDatabase(ctx, message.Root)
		if err != nil {
			WriteErrorToStream(stream, "Failed to build dag from root %e", err)

			stream.Close()
			return
		}

		dags := *dag

		count := len(dag.Leafs)

		//label := 0

		for _, leaf := range dags.Leafs {
			message := lib.UploadMessage{
				Root:  dag.Root,
				Count: count,
				Leaf:  *leaf,
			}

			if err := enc.Encode(&message); err != nil {
				WriteErrorToStream(stream, "Failed to encode leaf to stream: %e", err)

				stream.Close()
				return
			}

			var response lib.ResponseMessage

			timeout := time.NewTimer(5 * time.Second)

		wait:
			for {
				select {
				case <-timeout.C:
					stream.Close()
					return
				default:
					if err := dec.Decode(&response); err == nil {
						break wait
					}
				}
			}

			if !response.Ok {
				stream.Close()
				return
			}
		}
	}

	stream.Close()
}

func BuildDagFromDatabase(ctx context.Context, root string) (*merkle_dag.Dag, error) {
	encoding, _, err := multibase.Decode(root)
	if err != nil {
		return nil, err
	}

	encoder := multibase.MustNewEncoder(encoding)

	builder := merkle_dag.CreateDagBuilder()

	rootLeaf, err := GetLeafFromDatabase(ctx, root)
	if err != nil {
		return nil, err
	}

	RepairLeafContent(ctx, rootLeaf)

	builder.AddLeaf(rootLeaf, encoder, nil)

	AddLeavesFromDatabase(ctx, builder, encoder, rootLeaf)

	return builder.BuildDag(root), nil
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

func GetLeafFromDatabase(ctx context.Context, hash string) (*merkle_dag.DagLeaf, error) {
	blockDb := ctx.Value(keys.BlockDatabase).(*hornet_badger.BadgerDB)

	key := merkle_dag.GetHash(hash)

	bytes, err := blockDb.Get(key)
	if err != nil {
		return nil, err
	}

	var leaf *merkle_dag.DagLeaf = &merkle_dag.DagLeaf{}

	err = cbor.Unmarshal(bytes, leaf)
	if err != nil {
		return nil, err
	}

	return leaf, nil
}

func WriteErrorToStream(stream network.Stream, message string, err error) error {
	enc := cbor.NewEncoder(stream)

	data := lib.ErrorMessage{
		Message: fmt.Sprintf(message, err),
	}

	if err := enc.Encode(&data); err != nil {
		return err
	}

	return nil
}
