package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/libp2p/go-libp2p/core/network"

	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"

	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"

	types "github.com/HORNET-Storage/hornet-storage/lib"

	"github.com/libp2p/go-libp2p/core/host"
)

func AddDownloadHandler(libp2phost host.Host, store stores.Store, canDownloadDag func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool) {
	libp2phost.SetStreamHandler("/download/1.0.0", BuildDownloadStreamHandler(store, canDownloadDag))
}

func AddUploadHandler(libp2phost host.Host, store stores.Store, canUploadDag func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool, handleRecievedDag func(dag *merkle_dag.Dag, pubKey *string)) {
	libp2phost.SetStreamHandler("/upload/1.0.0", BuildUploadStreamHandler(store, canUploadDag, handleRecievedDag))
}

func AddQueryHandler(libp2phost host.Host, store stores.Store) {
	libp2phost.SetStreamHandler("/query/1.0.0", BuildQueryStreamHandler(store))
}

func BuildUploadStreamHandler(store stores.Store, canUploadDag func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool, handleRecievedDag func(dag *merkle_dag.Dag, pubKey *string)) func(stream network.Stream) {
	uploadStreamHandler := func(stream network.Stream) {
		result, message := WaitForUploadMessage(stream)
		if !result {
			WriteErrorToStream(stream, "Failed to recieve upload message in time", nil)

			stream.Close()
			return
		}

		log.Println("Recieved upload message")

		err := message.Leaf.VerifyRootLeaf()
		if err != nil {
			WriteErrorToStream(stream, "Failed to verify root leaf", err)

			stream.Close()
			return
		}

		if !canUploadDag(&message.Leaf, &message.PublicKey, &message.Signature) {
			WriteErrorToStream(stream, "Not allowed to upload this", nil)

			stream.Close()
			return
		}

		rootData := &types.DagLeafData{
			PublicKey: message.PublicKey,
			Signature: message.Signature,
			Leaf:      message.Leaf,
		}

		err = store.StoreLeaf(message.Root, rootData)
		if err != nil {
			WriteErrorToStream(stream, "Failed to verify root leaf", err)

			stream.Close()
			return
		}

		log.Println("Processed root leaf")

		err = WriteResponseToStream(stream, true)
		if err != nil || !result {
			log.Printf("Failed to write response to stream: %e\n", err)

			stream.Close()
			return
		}

		leafCount := 1

		for {
			log.Println("Waiting for upload message")

			result, message := WaitForUploadMessage(stream)
			if !result {
				WriteErrorToStream(stream, "Failed to recieve upload message in time", nil)

				stream.Close()
				break
			}

			log.Println("Recieved upload message")

			err = message.Leaf.VerifyLeaf()
			if err != nil {
				WriteErrorToStream(stream, "Failed to verify leaf", err)

				stream.Close()
				break
			}

			parentData, err := store.RetrieveLeaf(message.Root, message.Parent, false)
			if err != nil || !result {
				WriteErrorToStream(stream, "Failed to find parent leaf", err)

				stream.Close()
				break
			}

			parent := parentData.Leaf

			if message.Branch != nil {
				err = parent.VerifyBranch(message.Branch)
				if err != nil || !result {
					WriteErrorToStream(stream, "Failed to verify leaf branch", err)

					stream.Close()
					break
				}
			}

			data := &types.DagLeafData{
				Leaf: message.Leaf,
			}

			err = store.StoreLeaf(message.Root, data)
			if err != nil {
				WriteErrorToStream(stream, "Failed to add leaf to block database", err)

				stream.Close()
				return
			}

			log.Printf("Processed leaf: %s\n", message.Leaf.Hash)

			leafCount++

			err = WriteResponseToStream(stream, true)
			if err != nil || !result {
				log.Printf("Failed to write response to stream: %e\n", err)

				stream.Close()
				break
			}
		}

		log.Printf("Building and verifying dag for %d leaves\n", leafCount)

		dagData, err := store.BuildDagFromStore(message.Root, true)
		if err != nil {
			WriteErrorToStream(stream, "Failed to build dag from provided leaves: %e", err)

			stream.Close()
			return
		}

		err = dagData.Dag.Verify()
		if err != nil {
			WriteErrorToStream(stream, "Failed to verify dag: %e", err)

			stream.Close()
			return
		}

		log.Println("Upload finished")

		handleRecievedDag(&dagData.Dag, &message.PublicKey)

		stream.Close()
	}

	return uploadStreamHandler
}

func BuildDownloadStreamHandler(store stores.Store, canDownloadDag func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool) func(network.Stream) {
	downloadStreamHandler := func(stream network.Stream) {
		enc := cbor.NewEncoder(stream)

		result, message := WaitForDownloadMessage(stream)
		if !result {
			WriteErrorToStream(stream, "Failed to recieve upload message in time", nil)

			stream.Close()
			return
		}

		// Ensure the node is storing the root leaf
		rootData, err := store.RetrieveLeaf(message.Root, message.Root, true)
		if err != nil {
			WriteErrorToStream(stream, "Node does not have root leaf", nil)

			stream.Close()
			return
		}

		rootLeaf := rootData.Leaf

		err = rootLeaf.VerifyRootLeaf()
		if err != nil {
			WriteErrorToStream(stream, "Error occured when trying to verify root leaf", err)

			stream.Close()
			return
		}

		if !result {
			jsonData, _ := json.Marshal(rootLeaf)
			os.WriteFile("before_download.json", jsonData, 0644)

			WriteErrorToStream(stream, "Failed to verify root leaf", nil)

			stream.Close()
			return
		}

		if !canDownloadDag(&rootLeaf, &message.PublicKey, &message.Signature) {
			WriteErrorToStream(stream, "Not allowed to download this", nil)

			stream.Close()
			return
		}

		log.Printf("Download requested for: %s ", message.Root)

		includeContent := true

		if message.Filter != nil {
			log.Print("with filter\n")

			includeContent = message.Filter.IncludeContent
		}

		dagData, err := store.BuildDagFromStore(message.Root, includeContent)
		if err != nil {
			WriteErrorToStream(stream, "Failed to build dag from root %e", err)

			stream.Close()
			return
		}

		dag := dagData.Dag

		count := len(dag.Leafs)

		streamEncoder := cbor.NewEncoder(stream)

		if message.Filter != nil {
			err = dag.IterateDag(func(leaf *merkle_dag.DagLeaf, parent *merkle_dag.DagLeaf) error {
				if leaf.Hash == dag.Root {
					err := leaf.VerifyRootLeaf()
					if err != nil {
						return err
					}

					if !message.Filter.IncludeContent {
						rootLeaf.Content = nil

						rootLeaf.Links = make(map[string]string)
					}

					message := types.UploadMessage{
						Root:  dag.Root,
						Count: count,
						Leaf:  rootLeaf,
					}

					if err := enc.Encode(&message); err != nil {
						return err
					}

					if result := WaitForResponse(stream); !result {
						return err
					}
				} else {
					if !message.Filter.IncludeContent {
						if leaf.Type == merkle_dag.ChunkLeafType {
							log.Println("Skipping file chunk")
							return nil
						} else if leaf.Type == merkle_dag.FileLeafType {
							log.Println("Removing links from file")
							leaf.Links = make(map[string]string)
						}
					}

					valid, err := CheckFilter(leaf, message.Filter)

					if err != nil && valid {
						if !message.Filter.IncludeContent {
							leaf.Content = nil
						}

						err := leaf.VerifyLeaf()
						if err != nil {
							return err
						}

						label := merkle_dag.GetLabel(leaf.Hash)

						var branch *merkle_dag.ClassicTreeBranch

						if len(leaf.Links) > 1 {
							branch, err = parent.GetBranch(label)
							if err != nil {
								return err
							}

							err = parent.VerifyBranch(branch)
							if err != nil {
								return err
							}

							if !result {
								return err
							}
						}

						message := types.UploadMessage{
							Root:   dag.Root,
							Count:  count,
							Leaf:   *leaf,
							Parent: parent.Hash,
							Branch: branch,
						}

						if err := streamEncoder.Encode(&message); err != nil {
							return err
						}

						if result = WaitForResponse(stream); !result {
							return err
						}
					}
				}

				return nil
			})

			if err != nil {
				WriteErrorToStream(stream, "Failed to download dag %e", err)

				stream.Close()
				return
			}
		} else {
			err = dag.IterateDag(func(leaf *merkle_dag.DagLeaf, parent *merkle_dag.DagLeaf) error {
				if leaf.Hash == dag.Root {
					err := leaf.VerifyRootLeaf()
					if err != nil {
						return err
					}

					message := types.UploadMessage{
						Root:  dag.Root,
						Count: count,
						Leaf:  rootLeaf,
					}

					if err := enc.Encode(&message); err != nil {
						return err
					}

					if result := WaitForResponse(stream); !result {
						return err
					}
				} else {
					err := leaf.VerifyLeaf()
					if err != nil {
						return err
					}

					label := merkle_dag.GetLabel(leaf.Hash)

					var branch *merkle_dag.ClassicTreeBranch

					if len(leaf.Links) > 1 {
						branch, err = parent.GetBranch(label)
						if err != nil {
							return err
						}

						err = parent.VerifyBranch(branch)
						if err != nil {
							return err
						}
					}

					message := types.UploadMessage{
						Root:   dag.Root,
						Count:  count,
						Leaf:   *leaf,
						Parent: parent.Hash,
						Branch: branch,
					}

					if err := streamEncoder.Encode(&message); err != nil {
						return err
					}
					if result = WaitForResponse(stream); !result {
						return fmt.Errorf("did not recieve a valid response")
					}
				}

				return nil
			})

			if err != nil {
				WriteErrorToStream(stream, "Failed to download dag", err)

				stream.Close()
				return
			}

		}

		log.Println("Dag has been downloaded")

		stream.Close()
	}

	return downloadStreamHandler
}

func BuildQueryStreamHandler(store stores.Store) func(network.Stream) {
	queryStreamHandler := func(stream network.Stream) {
		enc := cbor.NewEncoder(stream)

		result, message := WaitForQueryMessage(stream)
		if !result {
			WriteErrorToStream(stream, "Failed to recieve upload message in time", nil)

			stream.Close()
			return
		}

		hashes, err := store.QueryDag(message.QueryFilter)
		if err != nil {
			WriteErrorToStream(stream, "Failed to query database", nil)

			stream.Close()
			return
		}

		fmt.Printf("Query Found %d hashes\n", len(hashes))

		response := types.QueryResponse{
			Hashes: hashes,
		}

		if err := enc.Encode(&response); err != nil {
			WriteErrorToStream(stream, "Failed to encode response", nil)

			stream.Close()
			return
		}

		stream.Close()
	}

	return queryStreamHandler
}

func CheckFilter(leaf *merkle_dag.DagLeaf, filter *types.DownloadFilter) (bool, error) {
	label := merkle_dag.GetLabel(leaf.Hash)

	if len(filter.Leaves) <= 0 && len(filter.LeafRanges) <= 0 {
		return true, nil
	}

	if slices.Contains(filter.Leaves, label) {
		return true, nil
	}

	labelInt, err := strconv.Atoi(label)
	if err != nil {
		return false, err
	}

	for _, rangeItem := range filter.LeafRanges {
		fromInt, err := strconv.Atoi(rangeItem.From)
		if err != nil {
			continue // Skip invalid ranges
		}
		toInt, err := strconv.Atoi(rangeItem.To)
		if err != nil {
			continue // Skip invalid ranges
		}

		if labelInt >= fromInt && labelInt <= toInt {
			return true, nil
		}
	}

	return false, nil
}

func WriteErrorToStream(stream network.Stream, message string, err error) error {
	enc := cbor.NewEncoder(stream)

	if err != nil {
		log.Printf("%s: %v\n", message, err)
	} else {
		log.Println(message)
	}

	data := types.ErrorMessage{
		Message: fmt.Sprintf(message, err),
	}

	if err := enc.Encode(&data); err != nil {
		return err
	}

	return nil
}

func WriteResponseToStream(stream network.Stream, response bool) error {
	streamEncoder := cbor.NewEncoder(stream)

	message := types.ResponseMessage{
		Ok: response,
	}

	if err := streamEncoder.Encode(&message); err != nil {
		return err
	}

	return nil
}

func WaitForResponse(stream network.Stream) bool {
	streamDecoder := cbor.NewDecoder(stream)

	var response types.ResponseMessage

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

	return response.Ok
}

func WaitForUploadMessage(stream network.Stream) (bool, *types.UploadMessage) {
	streamDecoder := cbor.NewDecoder(stream)

	var message types.UploadMessage

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

func WaitForDownloadMessage(stream network.Stream) (bool, *types.DownloadMessage) {
	streamDecoder := cbor.NewDecoder(stream)

	var message types.DownloadMessage

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

func WaitForQueryMessage(stream network.Stream) (bool, *types.QueryMessage) {
	streamDecoder := cbor.NewDecoder(stream)

	var message types.QueryMessage

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
