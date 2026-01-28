package download

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gofiber/contrib/websocket"
	"github.com/ipfs/go-cid"
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

func AddDownloadHandlerForWebsockets(store stores.Store, canDownloadDag func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool) func(*websocket.Conn) {
	ctx := context.Background()

	return func(conn *websocket.Conn) {
		wsStream := &types.WebSocketStream{Conn: conn, Ctx: ctx}

		message, err := lib_stream.WaitForDownloadMessage(wsStream)
		if err != nil {
			lib_stream.WriteErrorToStream(wsStream, "Failed to receive download message", err)
			return
		}

		handleDownload(store, wsStream, message, canDownloadDag)
	}
}

func BuildDownloadStreamHandler(store stores.Store, canDownloadDag func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool) func(network.Stream) {
	downloadStreamHandler := func(stream network.Stream) {
		ctx := context.Background()
		libp2pStream := libp2p_stream.New(stream, ctx)

		defer stream.Close()

		message, err := lib_stream.WaitForDownloadMessage(libp2pStream)
		if err != nil {
			lib_stream.WriteErrorToStream(libp2pStream, "Failed to receive download message", err)
			return
		}

		handleDownload(store, libp2pStream, message, canDownloadDag)
	}

	return downloadStreamHandler
}

func handleDownload(store stores.Store, stream lib_types.Stream, message *lib_types.DownloadMessage, canDownloadDag func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool) {
	rootData, err := store.RetrieveLeaf(message.Root, message.Root, false)
	if err != nil {
		// Check if this hash exists as a non-root leaf and provide a helpful error
		if parentRoot, findErr := store.FindRootForLeaf(message.Root); findErr == nil && parentRoot != "" {
			lib_stream.WriteErrorToStream(stream, fmt.Sprintf("Hash '%s' is a leaf hash, not a root hash. It belongs to DAG with root: %s", message.Root, parentRoot), nil)
			return
		}
		lib_stream.WriteErrorToStream(stream, "Node does not have root leaf", err)
		return
	}

	// Validate that ownership record exists - this is required for serving DAGs
	if rootData.PublicKey == "" || rootData.Signature == "" {
		lib_stream.WriteErrorToStream(stream, fmt.Sprintf("No ownership record found for root hash '%s'. The DAG may have been stored without proper signing or has been orphaned.", message.Root), nil)
		return
	}

	rootLeaf := rootData.Leaf

	if canDownloadDag != nil && !canDownloadDag(&rootLeaf, &message.PublicKey, &message.Signature) {
		lib_stream.WriteErrorToStream(stream, "Not allowed to download this", nil)
		return
	}

	includeContent := true
	if message.Filter != nil {
		includeContent = message.Filter.IncludeContent
	}

	if message.Filter != nil && (message.Filter.LeafRanges != nil || len(message.Filter.LeafHashes) > 0) {
		handlePartialDownload(store, stream, message, includeContent, rootData)
		return
	}

	handleStreamingDownload(store, stream, message, includeContent, rootData)
}

func handleStreamingDownload(store stores.Store, stream lib_types.Stream, message *lib_types.DownloadMessage, includeContent bool, rootData *types.DagLeafData) {
	dagStore, err := store.CreateDagStoreFromExisting(message.Root)
	if err != nil {
		lib_stream.WriteErrorToStream(stream, "Failed to create dag store", err)
		return
	}

	if !dagStore.HasIndex() {
		if err := dagStore.BuildIndex(); err != nil {
			lib_stream.WriteErrorToStream(stream, "Failed to build index", err)
			return
		}
	}

	totalLeaves, err := dagStore.CountLeavesStreaming()
	if err != nil {
		lib_stream.WriteErrorToStream(stream, "Failed to count leaves", err)
		return
	}

	const batchSize = 10
	var batch []*merkle_dag.TransmissionPacket
	packetIndex := 0
	leafIndex := 0

	parentCache := make(map[string]*merkle_dag.DagLeaf)

	err = dagStore.IterateDagWithIndex(func(leafHash string, parentHash string) error {
		leafIndex++

		leaf, err := dagStore.RetrieveLeafWithoutContent(leafHash)
		if err != nil {
			return err
		}
		if leaf == nil {
			return nil
		}

		if includeContent && len(leaf.ContentHash) > 0 {
			rootCID, err := cid.Decode(message.Root)
			if err != nil {
				return err
			}
			content, err := store.RetrieveContent(rootCID, leaf.ContentHash)
			if err != nil {
				fmt.Printf("Warning: Could not retrieve content for leaf %s: %v\n", leaf.Hash, err)
			} else {
				leaf.Content = content
			}
		}

		proofs := make(map[string]*merkle_dag.ClassicTreeBranch)
		if parentHash != "" {
			parent, cached := parentCache[parentHash]
			if !cached {
				parent, err = dagStore.RetrieveLeafWithoutContent(parentHash)
				if err == nil && parent != nil {
					parentCache[parentHash] = parent
				}
			}
			if parent != nil && parent.CurrentLinkCount > 1 {
				branch, err := parent.GetBranch(leaf.Hash)
				if err == nil && branch != nil {
					proofs[leaf.Hash] = branch
				}
			}
		}

		packet := &merkle_dag.TransmissionPacket{
			Leaf:       leaf,
			ParentHash: parentHash,
			Proofs:     proofs,
		}

		batch = append(batch, packet)

		if len(batch) >= batchSize || leafIndex == totalLeaves {
			if err := sendBatch(stream, batch, message.Root, rootData.PublicKey, rootData.Signature, packetIndex, leafIndex == totalLeaves); err != nil {
				return err
			}
			batch = batch[:0]
			parentCache = make(map[string]*merkle_dag.DagLeaf)
			packetIndex++
		}

		return nil
	})

	if err != nil {
		lib_stream.WriteErrorToStream(stream, "Failed to stream DAG leaves", err)
	}
}

func sendBatch(stream lib_types.Stream, batch []*merkle_dag.TransmissionPacket, root string, publicKey string, signature string, packetIndex int, isFinal bool) error {
	batchedPacket := &merkle_dag.BatchedTransmissionPacket{
		Leaves:        make([]*merkle_dag.DagLeaf, len(batch)),
		Relationships: make(map[string]string),
	}

	for i, p := range batch {
		batchedPacket.Leaves[i] = p.Leaf
		// Always add to relationships - root has empty parent hash
		batchedPacket.Relationships[p.Leaf.Hash] = p.ParentHash
	}

	uploadMsg := lib_types.UploadMessage{
		Root:          root,
		Packet:        *batchedPacket.ToSerializable(),
		IsFinalPacket: isFinal,
	}

	if packetIndex == 0 {
		uploadMsg.PublicKey = publicKey
		uploadMsg.Signature = signature
	}

	if err := lib_stream.WriteMessageToStream(stream, uploadMsg); err != nil {
		return err
	}

	resp, err := lib_stream.WaitForResponse(stream)
	if err != nil {
		// On final packet, client may disconnect before sending ack - this is fine
		if isFinal {
			return nil
		}
		return err
	}

	if !resp.Ok {
		return lib_stream.WriteErrorToStream(stream, "Client rejected packet", nil)
	}

	return nil
}

func handlePartialDownload(store stores.Store, stream lib_types.Stream, message *lib_types.DownloadMessage, includeContent bool, rootData *types.DagLeafData) {
	var leafHashes []string

	if len(message.Filter.LeafHashes) > 0 {
		leafHashes = message.Filter.LeafHashes
	} else if message.Filter.LeafRanges != nil {
		labels, err := store.RetrieveLabels(message.Root)
		if err != nil {
			lib_stream.WriteErrorToStream(stream, "Failed to retrieve cached labels", err)
			return
		}

		for i := message.Filter.LeafRanges.From; i <= message.Filter.LeafRanges.To; i++ {
			label := strconv.Itoa(i)
			if hash, exists := labels[label]; exists {
				leafHashes = append(leafHashes, hash)
			} else {
				lib_stream.WriteErrorToStream(stream, "Label not found in cached labels", nil)
				return
			}
		}
	}

	dagData, err := store.BuildPartialDagFromStore(message.Root, leafHashes, includeContent, true)
	if err != nil {
		lib_stream.WriteErrorToStream(stream, "Failed to build partial dag from store", err)
		return
	}

	sendDagPackets(stream, dagData)
}

func sendDagPackets(stream lib_types.Stream, dagData *types.DagData) {
	sequence := dagData.Dag.GetBatchedLeafSequence()
	total := len(sequence)

	for i, packet := range sequence {
		isFinalPacket := i == total-1

		uploadMsg := lib_types.UploadMessage{
			Root:          dagData.Dag.Root,
			Packet:        *packet.ToSerializable(),
			IsFinalPacket: isFinalPacket,
		}

		if packet.GetRootLeaf() != nil {
			uploadMsg.PublicKey = dagData.PublicKey
			uploadMsg.Signature = dagData.Signature
		}

		err := lib_stream.WriteMessageToStream(stream, uploadMsg)
		if err != nil {
			lib_stream.WriteErrorToStream(stream, "Failed to send packet", err)
			return
		}

		resp, err := lib_stream.WaitForResponse(stream)
		if err != nil {
			// On final packet, client may disconnect before sending ack - this is fine
			if isFinalPacket {
				return
			}
			lib_stream.WriteErrorToStream(stream, "Failed to receive acknowledgment", err)
			return
		}

		if !resp.Ok {
			lib_stream.WriteErrorToStream(stream, "Client rejected packet", nil)
			return
		}
	}
}
