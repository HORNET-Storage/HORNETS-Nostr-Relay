package download

import (
	"context"

	"github.com/fxamacker/cbor/v2"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/dag"
	types "github.com/HORNET-Storage/hornet-storage/lib"
	utils "github.com/HORNET-Storage/hornet-storage/lib/handlers/scionic"
	"github.com/HORNET-Storage/hornet-storage/lib/sessions/libp2p/middleware"
	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
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

		rootData, err := store.RetrieveLeaf(message.Root, message.Root, true, false)
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

		includeContent := true

		if message.Filter != nil {
			includeContent = message.Filter.IncludeContent
		}

		dagData, err := store.BuildDagFromStore(message.Root, includeContent, false)
		if err != nil {
			utils.WriteErrorToStream(libp2pStream, "Failed to build dag from root %e", err)

			stream.Close()
			return
		}

		dag := dagData.Dag

		if message.Filter != nil && message.Filter.LeafRanges != nil {
			partialDag, err := dag.GetPartial(message.Filter.LeafRanges.From, message.Filter.LeafRanges.To)
			if err != nil {
				utils.WriteErrorToStream(libp2pStream, "Failed to build partial dag %e", err)

				stream.Close()
				return
			}

			sequence := partialDag.GetLeafSequence()

			for _, packet := range sequence {
				message := types.UploadMessage{
					Root:   dag.Root,
					Packet: *packet.ToSerializable(),
				}

				if packet.Leaf.Hash == dag.Root {
					message.PublicKey = dagData.PublicKey
					message.Signature = dagData.Signature
				}

				err := enc.Encode(&message)
				if err != nil {
					utils.WriteErrorToStream(libp2pStream, "Failed to encode partial dag %e", err)

					stream.Close()
					return
				}

				resp, err := utils.WaitForResponse(libp2pStream)
				if err != nil {
					utils.WriteErrorToStream(libp2pStream, "Failed to wait for response %e", err)

					stream.Close()
					return
				}

				if !resp.Ok {
					utils.WriteErrorToStream(libp2pStream, "client responded withg false", nil)

					stream.Close()
					return
				}
			}
		} else {
			sequence := dag.GetLeafSequence()

			for _, packet := range sequence {
				message := types.UploadMessage{
					Root:   dag.Root,
					Packet: *packet.ToSerializable(),
				}

				if packet.Leaf.Hash == dag.Root {
					message.PublicKey = dagData.PublicKey
					message.Signature = dagData.Signature
				}

				err := enc.Encode(&message)
				if err != nil {
					utils.WriteErrorToStream(libp2pStream, "Failed to encode partial dag %e", err)

					stream.Close()
					return
				}

				resp, err := utils.WaitForResponse(libp2pStream)
				if err != nil {
					utils.WriteErrorToStream(libp2pStream, "Failed to wait for response %e", err)

					stream.Close()
					return
				}

				if !resp.Ok {
					utils.WriteErrorToStream(libp2pStream, "client responded with false", nil)

					stream.Close()
					return
				}
			}
		}

		stream.Close()
	}

	return downloadStreamHandler
}
