package download

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/fxamacker/cbor/v2"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	utils "github.com/HORNET-Storage/hornet-storage/lib/handlers/scionic"
	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"
)

func AddDownloadHandler(libp2phost host.Host, store stores.Store, canDownloadDag func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool) {
	libp2phost.SetStreamHandler("/download/1.0.0", BuildDownloadStreamHandler(store, canDownloadDag))
}

func BuildDownloadStreamHandler(store stores.Store, canDownloadDag func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool) func(network.Stream) {
	downloadStreamHandler := func(stream network.Stream) {
		enc := cbor.NewEncoder(stream)

		result, message := utils.WaitForDownloadMessage(stream)
		if !result {
			utils.WriteErrorToStream(stream, "Failed to recieve upload message in time", nil)

			stream.Close()
			return
		}

		// Ensure the node is storing the root leaf
		rootData, err := store.RetrieveLeaf(message.Root, message.Root, true)
		if err != nil {
			utils.WriteErrorToStream(stream, "Node does not have root leaf", nil)

			stream.Close()
			return
		}

		rootLeaf := rootData.Leaf

		err = rootLeaf.VerifyRootLeaf()
		if err != nil {
			utils.WriteErrorToStream(stream, "Error occured when trying to verify root leaf", err)

			stream.Close()
			return
		}

		if !result {
			jsonData, _ := json.Marshal(rootLeaf)
			os.WriteFile("before_download.json", jsonData, 0644)

			utils.WriteErrorToStream(stream, "Failed to verify root leaf", nil)

			stream.Close()
			return
		}

		if !canDownloadDag(&rootLeaf, &message.PublicKey, &message.Signature) {
			utils.WriteErrorToStream(stream, "Not allowed to download this", nil)

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
			utils.WriteErrorToStream(stream, "Failed to build dag from root %e", err)

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

					if result := utils.WaitForResponse(stream); !result {
						return err
					}
				} else {
					if !message.Filter.IncludeContent {
						if leaf.Type == merkle_dag.ChunkLeafType {
							return nil
						} else if leaf.Type == merkle_dag.FileLeafType {
							leaf.Links = make(map[string]string)
						}
					}

					valid, err := utils.CheckFilter(leaf, message.Filter)

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

						if result = utils.WaitForResponse(stream); !result {
							return err
						}
					}
				}

				return nil
			})

			if err != nil {
				utils.WriteErrorToStream(stream, "Failed to download dag %e", err)

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

					if result := utils.WaitForResponse(stream); !result {
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
					if result = utils.WaitForResponse(stream); !result {
						return fmt.Errorf("did not recieve a valid response")
					}
				}

				return nil
			})

			if err != nil {
				utils.WriteErrorToStream(stream, "Failed to download dag", err)

				stream.Close()
				return
			}
		}

		log.Println("Dag has been downloaded")

		stream.Close()
	}

	return downloadStreamHandler
}
