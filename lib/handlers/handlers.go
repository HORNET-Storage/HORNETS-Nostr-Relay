package handlers

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"regexp"
	"strconv"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib"

	"github.com/fxamacker/cbor/v2"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/multiformats/go-multibase"

	keys "github.com/HORNET-Storage/hornet-storage/lib/context"
	hornet_badger "github.com/HORNET-Storage/hornet-storage/lib/database/badger"
	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"
)

func IsHash(s string) bool {
	// A SHA-256 hash should be exactly 64 characters long
	if len(s) != 64 {
		return false
	}

	// A SHA-256 hash consists only of hexadecimal characters
	matched, _ := regexp.MatchString("^[a-fA-F0-9]+$", s)

	return matched
}

func UploadStreamHandler(stream network.Stream) {
	ctx := keys.GetContext()

	blockDb := ctx.Value(keys.BlockDatabase).(*hornet_badger.BadgerDB)

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

	result, err = message.Leaf.VerifyRootLeaf(encoder)
	if err != nil || !result {
		WriteErrorToStream(stream, "Failed to verify root leaf", err)

		stream.Close()
		return
	}

	SplitLeafContent(ctx, &message.Leaf)

	cborData, err := cbor.Marshal(message.Leaf)
	if err != nil {
		log.Fatal(err)
	}

	key := merkle_dag.GetHash(message.Leaf.Hash)

	log.Printf("Adding key to block database: %s\n", key)
	err = blockDb.Update(key, cborData)
	if err != nil {
		WriteErrorToStream(stream, "Failed to add leaf to block database", err)

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

		_, err = GetLeafFromDatabase(ctx, message.Parent)
		if err != nil || !result {
			WriteErrorToStream(stream, "Failed to find parent leaf", err)

			stream.Close()
			break
		}

		//result, err = parent.VerifyBranch(message.Branch)
		//if err != nil || !result {
		//	WriteErrorToStream(stream, "Failed to verify leaf branch", err)

		//	stream.Close()
		//	break
		//}

		SplitLeafContent(ctx, &message.Leaf)

		cborData, err := cbor.Marshal(message.Leaf)
		if err != nil {
			log.Fatal(err)
		}

		key := merkle_dag.GetHash(message.Leaf.Hash)

		log.Printf("Adding key to block database: %s\n", key)
		err = blockDb.Update(key, cborData)
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

	time.Sleep(5 * time.Second)

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

	timeout.Stop()

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

		rootLeaf := dag.Leafs[dag.Root]

		parsed, err := strconv.ParseInt(rootLeaf.LatestLabel, 10, 64)
		if err != nil {
			fmt.Println("Failed to parse label")
		}

		latestLabel := int(parsed)

		for i := 1; i < latestLabel; i++ {
			var foundLeaf *merkle_dag.DagLeaf

			for hash, leaf := range dags.Leafs {
				label := merkle_dag.GetLabel(hash)

				if label != "" {
					if label == strconv.FormatInt(int64(i), 64) {
						foundLeaf = leaf
					}
				}
			}

			if foundLeaf == nil {
				continue
			}

			message := lib.UploadMessage{
				Root:  dag.Root,
				Count: count,
				Leaf:  *foundLeaf,
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
