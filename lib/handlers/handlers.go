package handlers

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
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

	result, message := WaitForUploadMessage(ctx, stream)
	if !result {
		WriteErrorToStream(stream, "Failed to recieve upload message in time", nil)

		stream.Close()
		return
	}

	log.Println("Recieved upload message")

	encoding, _, err := multibase.Decode(message.Root)
	if err != nil {
		WriteErrorToStream(stream, "Failed to discover encoding from root hash: %e", err)

		stream.Close()
		return
	}

	encoder := multibase.MustNewEncoder(encoding)

	result, err = message.Leaf.VerifyRootLeaf(encoder)
	if err != nil || !result {
		WriteErrorToStream(stream, "Failed to verify root leaf", err)

		stream.Close()
		return
	}

	SplitLeafContent(ctx, &message.Leaf)

	err = AddLeafToDatabase(ctx, &message.Leaf)
	if err != nil {
		WriteErrorToStream(stream, "Failed to verify root leaf", err)

		stream.Close()
		return
	}

	log.Println("Processed root leaf")

	err = WriteResponseToStream(ctx, stream, true)
	if err != nil || !result {
		log.Printf("Failed to write response to stream: %e\n", err)

		stream.Close()
		return
	}

	for {
		log.Println("Waiting for upload message")

		result, message := WaitForUploadMessage(ctx, stream)
		if !result {
			WriteErrorToStream(stream, "Failed to recieve upload message in time", nil)

			stream.Close()
			break
		}

		log.Println("Recieved upload message")

		encoding, _, err := multibase.Decode(message.Root)
		if err != nil {
			WriteErrorToStream(stream, "Failed to discover encoding from root hash: %e", err)

			stream.Close()
			break
		}

		encoder := multibase.MustNewEncoder(encoding)

		/*
			data := encoder.Encode(message.Leaf.Data)

			if IsHash(data) {
				if err := RepairLeafContent(ctx, &message.Leaf); err != nil {
					WriteErrorToStream(stream, "Server does not contain leaf data: %e", err)

					stream.Close()
					return
				}
			}
		*/

		result, err = message.Leaf.VerifyLeaf(encoder)
		if err != nil || !result {
			WriteErrorToStream(stream, "Failed to verify leaf", err)

			stream.Close()
			break
		}

		parent, err := GetLeafFromDatabase(ctx, message.Parent)
		if err != nil || !result {
			WriteErrorToStream(stream, "Failed to find parent leaf", err)

			stream.Close()
			break
		}

		if message.Branch != nil {
			result, err = parent.VerifyBranch(message.Branch)
			if err != nil || !result {
				WriteErrorToStream(stream, "Failed to verify leaf branch", err)

				stream.Close()
				break
			}
		}

		SplitLeafContent(ctx, &message.Leaf)

		err = AddLeafToDatabase(ctx, &message.Leaf)
		if err != nil {
			WriteErrorToStream(stream, "Failed to add leaf to block database", err)

			stream.Close()
			return
		}

		log.Printf("Processed leaf: %s\n", message.Leaf.Hash)

		err = WriteResponseToStream(ctx, stream, true)
		if err != nil || !result {
			log.Printf("Failed to write response to stream: %e\n", err)

			stream.Close()
			break
		}
	}

	log.Println("Building and verifying dag")

	dag, err := BuildDagFromDatabase(ctx, message.Root)
	if err != nil {
		WriteErrorToStream(stream, "Failed to build dag from provided leaves: %e", err)

		stream.Close()
		return
	}

	result, err = dag.Verify(encoder)
	if err != nil {
		WriteErrorToStream(stream, "Failed to verify dag: %e", err)

		stream.Close()
		return
	}

	if !result {
		log.Printf("Failed to verify dag: %s\n", message.Root)
	}

	log.Println("Upload finished")

	stream.Close()
	return
}

func DownloadStreamHandler(stream network.Stream) {
	ctx := keys.GetContext()

	enc := cbor.NewEncoder(stream)

	result, message := WaitForDownloadMessage(ctx, stream)
	if !result {
		WriteErrorToStream(stream, "Failed to recieve upload message in time", nil)

		stream.Close()
		return
	}

	// Ensure the node is storing the root leaf
	rootLeaf, err := GetLeafFromDatabase(ctx, message.Root)
	if err != nil {
		WriteErrorToStream(stream, "Node does not have root leaf", nil)

		stream.Close()
		return
	}

	encoding, _, err := multibase.Decode(message.Root)
	if err != nil {
		WriteErrorToStream(stream, "Failed to discover encoding from root hash: %e", err)

		stream.Close()
		return
	}

	encoder := multibase.MustNewEncoder(encoding)

	err = RepairLeafContent(ctx, rootLeaf)
	if err != nil {
		WriteErrorToStream(stream, "Node does not have root leaf content", nil)

		stream.Close()
		return
	}

	result, err = rootLeaf.VerifyRootLeaf(encoder)
	if err != nil || !result {
		WriteErrorToStream(stream, "Failed to verify root leaf", err)

		stream.Close()
		return
	}

	if message.Hash != nil {

	} else if message.Range != nil {
		// Range of leaves by labels

	} else if message.Label != nil {
		// Specific Leaf from label

	} else if message.Hash != nil {
		// Specific Leaf from hash

	} else {
		log.Printf("Download requested for: %s\n", message.Root)

		// Entire dag
		dag, err := BuildDagFromDatabase(ctx, message.Root)
		if err != nil {
			WriteErrorToStream(stream, "Failed to build dag from root %e", err)

			stream.Close()
			return
		}

		count := len(dag.Leafs)

		rootLeaf := dag.Leafs[dag.Root]

		message := lib.UploadMessage{
			Root:  dag.Root,
			Count: count,
			Leaf:  *rootLeaf,
		}

		if err := enc.Encode(&message); err != nil {
			WriteErrorToStream(stream, "Failed to write to stream", err)

			stream.Close()
			return
		}

		log.Println("Uploaded root leaf")

		if result := WaitForResponse(ctx, stream); !result {
			WriteErrorToStream(stream, "Did not recieve a valid response", nil)

			stream.Close()
			return
		}

		log.Println("Response received")

		err = UploadLeafChildren(ctx, stream, rootLeaf, dag)
		if err != nil {
			WriteErrorToStream(stream, "Failed to upload leaf children: %e", err)

			stream.Close()
			return
		}

		log.Println("Dag has been uploaded")
	}

	stream.Close()
}

func UploadLeafChildren(ctx context.Context, stream network.Stream, leaf *merkle_dag.DagLeaf, dag *merkle_dag.Dag) error {
	streamEncoder := cbor.NewEncoder(stream)

	encoding, _, err := multibase.Decode(dag.Root)
	if err != nil {
		log.Println("Failed to discover encoding")
		return err
	}

	encoder := multibase.MustNewEncoder(encoding)

	count := len(dag.Leafs)

	for label, hash := range leaf.Links {
		child, exists := dag.Leafs[hash]
		if !exists {
			return fmt.Errorf("Leaf with has does not exist in dag")
		}

		result, err := child.VerifyLeaf(encoder)
		if err != nil {
			log.Println("Failed to verify leaf")
			return err
		}

		if !result {
			return fmt.Errorf("Failed to verify leaf")
		}

		var branch *merkle_dag.ClassicTreeBranch

		if len(leaf.Links) > 1 {
			branch, err = leaf.GetBranch(label)
			if err != nil {
				log.Println("Failed to get branch")
				return err
			}

			result, err = leaf.VerifyBranch(branch)
			if err != nil {
				log.Println("Failed to verify branch")
				return err
			}

			if !result {
				return fmt.Errorf("Failed to verify branch for leaf")
			}
		}

		message := lib.UploadMessage{
			Root:   dag.Root,
			Count:  count,
			Leaf:   *child,
			Parent: leaf.Hash,
			Branch: branch,
		}

		if err := streamEncoder.Encode(&message); err != nil {
			log.Println("Failed to encode to stream")
			return err
		}

		log.Println("Uploaded next leaf")

		if result = WaitForResponse(ctx, stream); !result {
			return fmt.Errorf("Did not recieve a valid response")
		}

		log.Println("Response recieved")
	}

	for _, hash := range leaf.Links {
		child, exists := dag.Leafs[hash]
		if !exists {
			return fmt.Errorf("Leaf with hash does not exist in dag")
		}

		if len(child.Links) > 0 {
			err = UploadLeafChildren(ctx, stream, child, dag)
			if err != nil {
				log.Println("Failed to Upload Leaf Children")
				return err
			}
		}
	}

	return nil
}

func BuildDagFromDatabase(ctx context.Context, root string) (*merkle_dag.Dag, error) {
	encoding, _, err := multibase.Decode(root)
	if err != nil {
		log.Println("Failed to discover encoding")
		return nil, err
	}

	encoder := multibase.MustNewEncoder(encoding)

	builder := merkle_dag.CreateDagBuilder()

	rootLeaf, err := GetLeafFromDatabase(ctx, root)
	if err != nil {
		log.Println("Unable to find leaf in the database")
		return nil, err
	}

	RepairLeafContent(ctx, rootLeaf)

	builder.AddLeaf(rootLeaf, encoder, nil)

	err = AddLeavesFromDatabase(ctx, builder, encoder, rootLeaf)
	if err != nil {
		log.Println("Failed to add leaves from database")
		return nil, err
	}

	return builder.BuildDag(root), nil
}

func AddLeavesFromDatabase(ctx context.Context, builder *merkle_dag.DagBuilder, encoder multibase.Encoder, leaf *merkle_dag.DagLeaf) error {
	for _, hash := range leaf.Links {
		childLeaf, err := GetLeafFromDatabase(ctx, hash)
		if err != nil {
			log.Println("Unable to find leaf in the database")
			return err
		}

		err = RepairLeafContent(ctx, childLeaf)

		builder.AddLeaf(childLeaf, encoder, leaf)

		AddLeavesFromDatabase(ctx, builder, encoder, childLeaf)
	}

	return nil
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

	log.Printf("Searching for leaf with key: %s\n", key)
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

func AddLeafToDatabase(ctx context.Context, leaf *merkle_dag.DagLeaf) error {
	blockDb := ctx.Value(keys.BlockDatabase).(*hornet_badger.BadgerDB)

	cborData, err := cbor.Marshal(leaf)
	if err != nil {
		log.Fatal(err)
	}

	key := merkle_dag.GetHash(leaf.Hash)

	log.Printf("Adding key to block database: %s\n", key)

	err = blockDb.Update(key, cborData)
	if err != nil {
		return err
	}

	return nil
}

func WriteErrorToStream(stream network.Stream, message string, err error) error {
	enc := cbor.NewEncoder(stream)

	log.Println(message)

	data := lib.ErrorMessage{
		Message: fmt.Sprintf(message, err),
	}

	if err := enc.Encode(&data); err != nil {
		return err
	}

	return nil
}

func WriteResponseToStream(ctx context.Context, stream network.Stream, response bool) error {
	streamEncoder := cbor.NewEncoder(stream)

	message := lib.ResponseMessage{
		Ok: response,
	}

	if err := streamEncoder.Encode(&message); err != nil {
		return err
	}

	return nil
}

func WaitForResponse(ctx context.Context, stream network.Stream) bool {
	streamDecoder := cbor.NewDecoder(stream)

	var response lib.ResponseMessage

	timeout := time.NewTimer(5 * time.Second)

wait:
	for {
		select {
		case <-timeout.C:
			return false
		default:
			if err := streamDecoder.Decode(&response); err == nil {
				if err == io.EOF {
					return false
				}

				break wait
			}
		}
	}

	if !response.Ok {
		return false
	}

	return true
}

func WaitForUploadMessage(ctx context.Context, stream network.Stream) (bool, *lib.UploadMessage) {
	streamDecoder := cbor.NewDecoder(stream)

	var message lib.UploadMessage

	timeout := time.NewTimer(5 * time.Second)

wait:
	for {
		select {
		case <-timeout.C:
			return false, nil
		default:
			err := streamDecoder.Decode(&message)

			if err != nil {
				log.Printf("Error reading from stream: %e", err)
			}

			if err == io.EOF {
				return false, nil
			}

			if err == nil {
				break wait
			}
		}
	}

	return true, &message
}

func WaitForDownloadMessage(ctx context.Context, stream network.Stream) (bool, *lib.DownloadMessage) {
	streamDecoder := cbor.NewDecoder(stream)

	var message lib.DownloadMessage

	timeout := time.NewTimer(5 * time.Second)

wait:
	for {
		select {
		case <-timeout.C:
			return false, nil
		default:
			err := streamDecoder.Decode(&message)

			if err != nil {
				log.Printf("Error reading from stream: %e", err)
			}

			if err == io.EOF {
				return false, nil
			}

			if err == nil {
				break wait
			}
		}
	}

	return true, &message
}
