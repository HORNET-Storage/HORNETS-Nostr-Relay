package download

import (
	"context"
	"strconv"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/v2/dag"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/sessions/libp2p/middleware"
	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"

	lib_types "github.com/HORNET-Storage/go-hornet-storage-lib/lib"
	lib_stream "github.com/HORNET-Storage/go-hornet-storage-lib/lib/connmgr"
	libp2p_stream "github.com/HORNET-Storage/go-hornet-storage-lib/lib/connmgr/libp2p"
)

func AddDownloadHandler(libp2phost host.Host, store stores.Store, canDownloadDag func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool) {
	libp2phost.SetStreamHandler("/download", middleware.SessionMiddleware(libp2phost)(BuildDownloadStreamHandler(store, canDownloadDag)))
}

func BuildDownloadStreamHandler(store stores.Store, canDownloadDag func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool) func(network.Stream) {
	downloadStreamHandler := func(stream network.Stream) {
		ctx := context.Background()

		libp2pStream := libp2p_stream.New(stream, ctx)

		message, err := lib_stream.WaitForDownloadMessage(libp2pStream)
		if err != nil {
			lib_stream.WriteErrorToStream(libp2pStream, "Failed to recieve upload message in time", nil)

			stream.Close()
			return
		}

		logging.Infof("Downloading Dag: " + message.Root)

		rootData, err := store.RetrieveLeaf(message.Root, message.Root, true, false)
		if err != nil {
			lib_stream.WriteErrorToStream(libp2pStream, "Node does not have root leaf", nil)

			stream.Close()
			return
		}

		rootLeaf := rootData.Leaf

		// Don't verify the root leaf here - we only have the root, not the full tree
		// The full DAG will be built from store below and verified then

		if !canDownloadDag(&rootLeaf, &message.PublicKey, &message.Signature) {
			lib_stream.WriteErrorToStream(libp2pStream, "Not allowed to download this", nil)

			stream.Close()
			return
		}

		includeContent := true

		if message.Filter != nil {
			includeContent = message.Filter.IncludeContent
		}

		dagData, err := store.BuildDagFromStore(message.Root, includeContent, false)
		if err != nil {
			lib_stream.WriteErrorToStream(libp2pStream, "Failed to build dag from root %e", err)

			stream.Close()
			return
		}

		dag := dagData.Dag

		if message.Filter != nil && message.Filter.LeafRanges != nil {
			dag.CalculateLabels()

			hashes, err := dag.GetHashesByLabelRange(strconv.Itoa(message.Filter.LeafRanges.From), strconv.Itoa(message.Filter.LeafRanges.To))
			if err != nil {
				lib_stream.WriteErrorToStream(libp2pStream, "Failed to get hash range from label range %e", err)

				stream.Close()
				return
			}

			partialDag, err := dag.GetPartial(hashes, true)
			if err != nil {
				lib_stream.WriteErrorToStream(libp2pStream, "Failed to build partial dag %e", err)

				stream.Close()
				return
			}

			sequence := partialDag.GetBatchedLeafSequence()
			total := len(sequence)

			for i, packet := range sequence {
				message := lib_types.UploadMessage{
					Root:          dag.Root,
					Packet:        *packet.ToSerializable(),
					IsFinalPacket: i == total-1, // Mark the last packet
				}

				rootLeaf := packet.GetRootLeaf()
				if rootLeaf != nil {
					message.PublicKey = dagData.PublicKey
					message.Signature = dagData.Signature
				}

				err := lib_stream.WriteMessageToStream(libp2pStream, message)
				if err != nil {
					lib_stream.WriteErrorToStream(libp2pStream, "Failed to encode partial dag %e", err)

					stream.Close()
					return
				}

				resp, err := lib_stream.WaitForResponse(libp2pStream)
				if err != nil {
					lib_stream.WriteErrorToStream(libp2pStream, "Failed to wait for response %e", err)

					stream.Close()
					return
				}

				if !resp.Ok {
					lib_stream.WriteErrorToStream(libp2pStream, "client responded withg false", nil)

					stream.Close()
					return
				}
			}
		} else {
			sequence := dag.GetBatchedLeafSequence()
			total := len(sequence)

			for i, packet := range sequence {
				message := lib_types.UploadMessage{
					Root:          dag.Root,
					Packet:        *packet.ToSerializable(),
					IsFinalPacket: i == total-1, // Mark the last packet
				}

				rootLeaf := packet.GetRootLeaf()
				if rootLeaf != nil {
					message.PublicKey = dagData.PublicKey
					message.Signature = dagData.Signature
				}

				err := lib_stream.WriteMessageToStream(libp2pStream, message)
				if err != nil {
					lib_stream.WriteErrorToStream(libp2pStream, "Failed to encode partial dag %e", err)

					stream.Close()
					return
				}

				resp, err := lib_stream.WaitForResponse(libp2pStream)
				if err != nil {
					lib_stream.WriteErrorToStream(libp2pStream, "Failed to wait for response %e", err)

					stream.Close()
					return
				}

				if !resp.Ok {
					lib_stream.WriteErrorToStream(libp2pStream, "client responded with false", nil)

					stream.Close()
					return
				}
			}
		}

		stream.Close()
	}

	return downloadStreamHandler
}
