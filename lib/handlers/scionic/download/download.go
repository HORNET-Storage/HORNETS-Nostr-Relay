package download

import (
	"context"
	"strconv"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/v2/dag"
	types "github.com/HORNET-Storage/hornet-storage/lib"
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

		defer stream.Close()

		// Receive download request
		message, err := lib_stream.WaitForDownloadMessage(libp2pStream)
		if err != nil {
			lib_stream.WriteErrorToStream(libp2pStream, "Failed to receive download message", err)
			return
		}

		// Retrieve root leaf for authorization check
		rootData, err := store.RetrieveLeaf(message.Root, message.Root, false)
		if err != nil {
			lib_stream.WriteErrorToStream(libp2pStream, "Node does not have root leaf", err)
			return
		}

		rootLeaf := rootData.Leaf

		// Check authorization
		if canDownloadDag != nil && !canDownloadDag(&rootLeaf, &message.PublicKey, &message.Signature) {
			lib_stream.WriteErrorToStream(libp2pStream, "Not allowed to download this", nil)
			return
		}

		// Determine content inclusion from filter
		includeContent := true
		if message.Filter != nil {
			includeContent = message.Filter.IncludeContent
		}

		var dagData *types.DagData

		// Handle filtered/partial DAG requests
		if message.Filter != nil && (message.Filter.LeafRanges != nil || len(message.Filter.LeafHashes) > 0) {
			var leafHashes []string

			// Handle LeafHashes (direct hash specification)
			if len(message.Filter.LeafHashes) > 0 {
				leafHashes = message.Filter.LeafHashes
			} else if message.Filter.LeafRanges != nil {
				// Handle LeafRanges (label range specification)
				// Retrieve cached labels
				labels, err := store.RetrieveLabels(message.Root)
				if err != nil {
					lib_stream.WriteErrorToStream(libp2pStream, "Failed to retrieve cached labels", err)
					return
				}

				// Convert label range to hash array
				for i := message.Filter.LeafRanges.From; i <= message.Filter.LeafRanges.To; i++ {
					label := strconv.Itoa(i)
					if hash, exists := labels[label]; exists {
						leafHashes = append(leafHashes, hash)
					} else {
						lib_stream.WriteErrorToStream(libp2pStream, "Label not found in cached labels", nil)
						return
					}
				}
			}

			// Build partial DAG directly from store (efficient!)
			dagData, err = store.BuildPartialDagFromStore(message.Root, leafHashes, includeContent, true)
			if err != nil {
				lib_stream.WriteErrorToStream(libp2pStream, "Failed to build partial dag from store", err)
				return
			}
		} else {
			// Build full DAG from store
			dagData, err = store.BuildDagFromStore(message.Root, includeContent)
			if err != nil {
				lib_stream.WriteErrorToStream(libp2pStream, "Failed to build dag from store", err)
				return
			}
		}

		// Send DAG in batched packets
		sequence := dagData.Dag.GetBatchedLeafSequence()
		total := len(sequence)

		for i, packet := range sequence {
			uploadMsg := lib_types.UploadMessage{
				Root:          dagData.Dag.Root,
				Packet:        *packet.ToSerializable(),
				IsFinalPacket: i == total-1,
			}

			// Include public key and signature in first packet (contains root)
			if packet.GetRootLeaf() != nil {
				uploadMsg.PublicKey = dagData.PublicKey
				uploadMsg.Signature = dagData.Signature
			}

			err := lib_stream.WriteMessageToStream(libp2pStream, uploadMsg)
			if err != nil {
				lib_stream.WriteErrorToStream(libp2pStream, "Failed to send packet", err)
				return
			}

			// Wait for client acknowledgment
			resp, err := lib_stream.WaitForResponse(libp2pStream)
			if err != nil {
				lib_stream.WriteErrorToStream(libp2pStream, "Failed to receive acknowledgment", err)
				return
			}

			if !resp.Ok {
				lib_stream.WriteErrorToStream(libp2pStream, "Client rejected packet", nil)
				return
			}
		}
	}

	return downloadStreamHandler
}
