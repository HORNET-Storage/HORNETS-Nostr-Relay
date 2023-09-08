package handlers

import (
	"context"
	"crypto/sha256"
	"log"

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
	//enc := cbor.NewEncoder(stream)

	contentDb := ctx.Value(keys.ContentDatabase).(*hornet_badger.BadgerDB)
	blockDb := ctx.Value(keys.BlockDatabase).(*hornet_badger.BadgerDB)

	leaves := map[string]lib.DagLeafMessage{}

	for {
		var message lib.DagLeafMessage

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

			if result {
				log.Println("Leaf verified correctly")
			} else {
				log.Println("Failed to verify leaf")
			}
		} else {
			result, err := message.Leaf.VerifyLeaf(encoder)
			if err != nil {
				log.Fatal(err)
			}

			if result {
				log.Println("Leaf verified correctly")
			} else {
				log.Println("Failed to verify leaf")
			}
		}

		h := sha256.New()
		h.Write(message.Leaf.Data)
		hashed := h.Sum(nil)

		contentDb.UpdateFromByteKey(hashed, message.Leaf.Data)

		message.Leaf.Data = hashed

		cborData, err := cbor.Marshal(message.Leaf)
		if err != nil {
			log.Fatal(err)
		}

		blockDb.Update(message.Leaf.Hash, cborData)

		leaves[message.Leaf.Hash] = message

		if len(leaves) == message.Count {
			log.Println("All leaves recieved")

			log.Println(message.Root)

			encoding, _, err := multibase.Decode(message.Root)
			encoder := multibase.MustNewEncoder(encoding)

			builder := merkle_dag.CreateDagBuilder()

			rootLeafBytes, err := blockDb.Get(message.Root)
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

			AddLeaves(ctx, builder, encoder, rootLeaf)

			dag := builder.BuildDag(message.Root)

			result, err := dag.Verify(encoder)
			if err != nil {
				log.Fatal(err)
			}

			if result {
				log.Println("Verified correctly")
			} else {
				log.Println("Failed to verify")
			}

			return
		}
	}
}

func DownloadStreamHandler(stream network.Stream) {

}

func BranchStreamhandler(stream network.Stream) {

}

func AddLeaves(ctx context.Context, builder *merkle_dag.DagBuilder, encoder multibase.Encoder, leaf *merkle_dag.DagLeaf) {
	blockDb := ctx.Value(keys.BlockDatabase).(*hornet_badger.BadgerDB)
	contentDb := ctx.Value(keys.ContentDatabase).(*hornet_badger.BadgerDB)

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

		childContentLeafBytes, err := contentDb.GetFromByteKey(childLeaf.Data)
		if err != nil {
			log.Fatal(err)
		}

		childLeaf.Data = childContentLeafBytes

		builder.AddLeaf(childLeaf, encoder, leaf)
		log.Println("Leaf added: " + childLeaf.Hash)

		AddLeaves(ctx, builder, encoder, childLeaf)
	}
}
