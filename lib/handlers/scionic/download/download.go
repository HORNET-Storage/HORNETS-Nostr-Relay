package download

import (
	"context"
	"fmt"
	"log"

	"github.com/fxamacker/cbor/v2"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	utils "github.com/HORNET-Storage/hornet-storage/lib/handlers/scionic"
	"github.com/HORNET-Storage/hornet-storage/lib/sessions/libp2p/middleware"
	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"
)

func AddDownloadHandler(libp2phost host.Host, store stores.Store, canDownloadDag func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool) {
	libp2phost.SetStreamHandler("/download", middleware.SessionMiddleware(libp2phost)(BuildDownloadStreamHandler(store, canDownloadDag)))
}

func BuildDownloadStreamHandler(store stores.Store, canDownloadDag func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool) func(network.Stream) {
	downloadStreamHandler := func(stream network.Stream) {
		enc := cbor.NewEncoder(stream)

		libp2pStream := &types.Libp2pStream{Stream: stream, Ctx: context.Background()}

		message, err := utils.WaitForDownloadMessage(libp2pStream)
		if err != nil {
			utils.WriteErrorToStream(libp2pStream, "Failed to recieve upload message in time", nil)

			stream.Close()
			return
		}

		// Ensure the node is storing the root leaf
		rootData, err := store.RetrieveLeaf(message.Root, message.Root, true)
		if err != nil {
			utils.WriteErrorToStream(libp2pStream, "Node does not have root leaf", nil)

			stream.Close()
			return
		}

		rootLeaf := rootData.Leaf

		err = rootLeaf.VerifyRootLeaf()
		if err != nil {
			utils.WriteErrorToStream(libp2pStream, "Failed to verify root leaf", err)

			stream.Close()
			return
		}

		if !canDownloadDag(&rootLeaf, &message.PublicKey, &message.Signature) {
			utils.WriteErrorToStream(libp2pStream, "Not allowed to download this", nil)

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
			utils.WriteErrorToStream(libp2pStream, "Failed to build dag from root %e", err)

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

					resp, err := utils.WaitForResponse(libp2pStream)
					if err != nil {
						return err
					}

					if !resp.Ok {
						return fmt.Errorf("client responded withg false")
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

						resp, err := utils.WaitForResponse(libp2pStream)
						if err != nil {
							return err
						}

						if !resp.Ok {
							return fmt.Errorf("cient responded withg false")
						}
					}
				}

				return nil
			})

			if err != nil {
				utils.WriteErrorToStream(libp2pStream, "Failed to download dag %e", err)

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

					resp, err := utils.WaitForResponse(libp2pStream)
					if err != nil {
						return err
					}

					if !resp.Ok {
						return fmt.Errorf("client responded withg false")
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

					resp, err := utils.WaitForResponse(libp2pStream)
					if err != nil {
						return err
					}

					if !resp.Ok {
						return fmt.Errorf("client responded withg false")
					}
				}

				return nil
			})

			if err != nil {
				utils.WriteErrorToStream(libp2pStream, "Failed to download dag", err)

				stream.Close()
				return
			}
		}

		log.Println("Dag has been downloaded")

		stream.Close()
	}

	return downloadStreamHandler
}
