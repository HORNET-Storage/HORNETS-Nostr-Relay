package upload

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/gabriel-vasile/mimetype"
	"github.com/gofiber/contrib/websocket"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/v2/dag"
	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	types "github.com/HORNET-Storage/hornet-storage/lib"
	utils "github.com/HORNET-Storage/hornet-storage/lib/handlers/scionic"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/sessions/libp2p/middleware"
	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
	badgerhold_store "github.com/HORNET-Storage/hornet-storage/lib/stores/badgerhold"
	"github.com/HORNET-Storage/hornet-storage/lib/subscription"

	lib_types "github.com/HORNET-Storage/go-hornet-storage-lib/lib"
	lib_stream "github.com/HORNET-Storage/go-hornet-storage-lib/lib/connmgr"
	libp2p_stream "github.com/HORNET-Storage/go-hornet-storage-lib/lib/connmgr/libp2p"
)

type CanUploadDagFunc func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool
type HandleUploadedDagFunc func(dag *merkle_dag.Dag, pubKey *string)

func AddUploadHandlerForLibp2p(ctx context.Context, libp2phost host.Host, store stores.Store, canUploadDag CanUploadDagFunc, handleRecievedDag HandleUploadedDagFunc) {
	handler := BuildUploadStreamHandler(store, canUploadDag, handleRecievedDag)

	wrapper := func(stream network.Stream) {
		read := func() (*lib_types.UploadMessage, error) {
			libp2pStream := libp2p_stream.New(stream, ctx)

			return lib_stream.WaitForUploadMessage(libp2pStream)
		}

		write := func(message interface{}) error {
			libp2pStream := libp2p_stream.New(stream, ctx)

			return lib_stream.WriteMessageToStream(libp2pStream, message)
		}

		handler(read, write)

		stream.Close()
	}

	libp2phost.SetStreamHandler("/upload", middleware.SessionMiddleware(libp2phost)(wrapper))
}

func AddUploadHandlerForWebsockets(store stores.Store, canUploadDag CanUploadDagFunc, handleRecievedDag HandleUploadedDagFunc) func(*websocket.Conn) {
	ctx := context.Background()

	handler := BuildUploadStreamHandler(store, canUploadDag, handleRecievedDag)

	wrapper := func(conn *websocket.Conn) {
		read := func() (*lib_types.UploadMessage, error) {
			wsStream := &types.WebSocketStream{Conn: conn, Ctx: ctx}

			return lib_stream.WaitForUploadMessage(wsStream)
		}

		write := func(message interface{}) error {
			wsStream := &types.WebSocketStream{Conn: conn, Ctx: ctx}

			return lib_stream.WriteMessageToStream(wsStream, message)
		}

		handler(read, write)
	}

	return wrapper
}

func BuildUploadStreamHandler(store stores.Store, canUploadDag func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool, handleRecievedDag func(dag *merkle_dag.Dag, pubKey *string)) utils.UploadDagHandler {
	handler := func(read utils.UploadDagReader, write utils.DagWriter) {
		message, err := read()
		if err != nil {
			write(utils.BuildErrorMessage("Failed to recieve upload message", err))
			return
		}

		publicKey, err := signing.DeserializePublicKey(message.PublicKey)
		if err != nil {
			write(utils.BuildErrorMessage("Failed to deserialize public key", err))
			return
		}

		signatureBytes, err := hex.DecodeString(message.Signature)
		if err != nil {
			write(utils.BuildErrorMessage("Failed to deserialize signature", err))
			return
		}

		signature, err := schnorr.ParseSignature(signatureBytes)
		if err != nil {
			write(utils.BuildErrorMessage("Failed to deserialize signature", err))
			return
		}

		err = signing.VerifySerializedCIDSignature(signature, message.Root, publicKey)
		if err != nil {
			write(utils.BuildErrorMessage("Signature failed to verify", err))
			return
		}

		serializedPublicKey, err := signing.SerializePublicKey(publicKey)
		if err != nil {
			write(utils.BuildErrorMessage("Failed to serialize public key", err))
			return
		}

		handleStreamingUpload(store, read, write, message, *serializedPublicKey, hex.EncodeToString(signature.Serialize()), canUploadDag, handleRecievedDag)
	}

	return handler
}

func handleStreamingUpload(
	store stores.Store,
	read utils.UploadDagReader,
	write utils.DagWriter,
	message *lib_types.UploadMessage,
	publicKey string,
	signature string,
	canUploadDag func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool,
	handleRecievedDag func(dag *merkle_dag.Dag, pubKey *string),
) {
	dagStore := store.CreateDagStoreForRoot(message.Root, publicKey, signature)

	var totalDagSize int64
	leafCount := 0
	packetCount := 0

	// Track uploaded leaf hashes and referenced (linked) leaf hashes for partial DAG support
	uploadedHashes := make(map[string]bool)
	referencedHashes := make(map[string]bool)

	packet := merkle_dag.BatchedTransmissionPacketFromSerializable(&message.Packet)

	if canUploadDag != nil {
		rootLeaf := packet.GetRootLeaf()
		if rootLeaf == nil {
			write(utils.BuildErrorMessage("First packet must contain root leaf", nil))
			return
		}
		if !canUploadDag(rootLeaf, &message.PublicKey, &message.Signature) {
			write(utils.BuildErrorMessage("Not allowed to upload this", nil))
			return
		}
	}

	packetCount++
	if err := processPacketStreamingWithTracking(dagStore, packet, &totalDagSize, &leafCount, uploadedHashes, referencedHashes, write); err != nil {
		return
	}

	if !message.IsFinalPacket {
		if err := write(lib_stream.BuildResponseMessage(true)); err != nil {
			write(utils.BuildErrorMessage("Failed to write response to stream", err))
			return
		}

		for {
			msg, err := read()
			if err != nil {
				store.DeleteDag(message.Root)
				write(utils.BuildErrorMessage("Failed to recieve upload message in time", nil))
				return
			}

			pkt := merkle_dag.BatchedTransmissionPacketFromSerializable(&msg.Packet)
			packetCount++

			if err := processPacketStreamingWithTracking(dagStore, pkt, &totalDagSize, &leafCount, uploadedHashes, referencedHashes, write); err != nil {
				store.DeleteDag(message.Root)
				return
			}

			if err := write(lib_stream.BuildResponseMessage(true)); err != nil {
				write(utils.BuildErrorMessage("Failed to write response to stream", err))
				break
			}

			if msg.IsFinalPacket {
				break
			}
		}
	}

	// Handle partial DAG: check for referenced leaves that weren't uploaded
	missingHashes := findMissingLeafHashes(uploadedHashes, referencedHashes)
	if len(missingHashes) > 0 {
		logging.Infof("Partial DAG detected for root %s: %d referenced leaves not in upload, checking global store", message.Root, len(missingHashes))

		// Verify all missing leaves exist in global store
		if err := handlePartialDagLeaves(store, missingHashes, write); err != nil {
			store.DeleteDag(message.Root)
			return
		}
	}

	if err := dagStore.VerifyStreaming(); err != nil {
		store.DeleteDag(message.Root)
		write(utils.BuildErrorMessage("Failed to verify dag", err))
		return
	}

	if err := store.CacheRelationshipsStreaming(dagStore); err != nil {
		logging.Infof("Warning: Failed to cache relationships: %v", err)
	}

	if err := store.CacheLabelsStreaming(dagStore); err != nil {
		logging.Infof("Warning: Failed to cache labels: %v", err)
	}

	badgerhold_store.GetAndResetSkippedLeafCount()

	if err := write(lib_stream.BuildResponseMessage(true)); err != nil {
		write(utils.BuildErrorMessage("Failed to write final response to stream", err))
		return
	}

	go func(pubKey string, size int64) {
		subManager := subscription.GetGlobalManager()
		if subManager != nil {
			if err := subManager.UpdateStorageUsage(pubKey, size); err != nil {
				logging.Infof("Warning: Failed to update storage usage for pubkey %s: %v\n", pubKey, err)
			}
		}
	}(message.PublicKey, totalDagSize)

	if handleRecievedDag != nil {
		dagData, err := store.BuildDagFromStore(message.Root, false)
		if err == nil {
			handleRecievedDag(&dagData.Dag, &message.PublicKey)
		}
	}

	if len(missingHashes) > 0 {
		logging.Infof("Streaming upload complete (partial DAG): %d uploaded leaves + %d existing leaves, %d bytes", leafCount, len(missingHashes), totalDagSize)
	} else {
		logging.Infof("Streaming upload complete: %d leaves, %d bytes", leafCount, totalDagSize)
	}
}

func processPacketStreamingWithTracking(dagStore *merkle_dag.DagStore, packet *merkle_dag.BatchedTransmissionPacket, totalSize *int64, leafCount *int, uploadedHashes map[string]bool, referencedHashes map[string]bool, write utils.DagWriter) error {
	if err := dagStore.AddBatchedTransmissionPacket(packet); err != nil {
		write(utils.BuildErrorMessage(fmt.Sprintf("Failed to apply packet with %d leaves", len(packet.Leaves)), err))
		return err
	}

	for _, leaf := range packet.Leaves {
		*leafCount++

		// Track this uploaded leaf
		uploadedHashes[leaf.Hash] = true

		// Track all referenced children (Links)
		for _, childHash := range leaf.Links {
			referencedHashes[childHash] = true
		}

		if leaf.Content != nil {
			*totalSize += int64(len(leaf.Content))
		}
		if leaf.Type == "File" && leaf.Content != nil {
			mimeType := mimetype.Detect(leaf.Content)
			if !utils.IsMimeTypePermitted(mimeType.String()) {
				write(utils.BuildErrorMessage("Mime type is not allowed to be stored by this relay ("+mimeType.String()+")", nil))
				return fmt.Errorf("mime type not permitted: %s", mimeType.String())
			}
		}
	}
	return nil
}

// findMissingLeafHashes returns hashes that are referenced but not uploaded
func findMissingLeafHashes(uploadedHashes, referencedHashes map[string]bool) []string {
	var missing []string
	for hash := range referencedHashes {
		if !uploadedHashes[hash] {
			missing = append(missing, hash)
		}
	}
	return missing
}

// handlePartialDagLeaves verifies that missing leaves exist globally.
// It recursively checks children of existing leaves to ensure the full DAG can be reconstructed.
func handlePartialDagLeaves(store stores.Store, missingHashes []string, write utils.DagWriter) error {
	// Use a set to track all leaves we need to verify and avoid duplicates
	toVerify := make(map[string]bool)
	for _, hash := range missingHashes {
		toVerify[hash] = true
	}

	verified := make(map[string]bool)
	var notFoundHashes []string
	verifiedCount := 0

	// Verify leaves iteratively (BFS-style) to handle transitive dependencies
	for len(toVerify) > 0 {
		// Get next hash to verify
		var currentHash string
		for h := range toVerify {
			currentHash = h
			break
		}
		delete(toVerify, currentHash)

		if verified[currentHash] {
			continue
		}
		verified[currentHash] = true

		// Check if leaf exists globally
		exists, err := store.HasLeafGlobal(currentHash)
		if err != nil {
			logging.Infof("Error checking leaf existence for %s: %v", currentHash, err)
			write(utils.BuildErrorMessage("Failed to check existing leaf", err))
			return err
		}

		if !exists {
			notFoundHashes = append(notFoundHashes, currentHash)
			continue
		}
		verifiedCount++

		// Get the leaf's children and add them to verification queue if not already verified
		links, err := store.GetLeafLinksGlobal(currentHash)
		if err != nil {
			logging.Infof("Error getting leaf links for %s: %v", currentHash, err)
			write(utils.BuildErrorMessage("Failed to get leaf children", err))
			return err
		}

		for _, childHash := range links {
			if !verified[childHash] {
				toVerify[childHash] = true
			}
		}
	}

	if len(notFoundHashes) > 0 {
		logging.Infof("Partial DAG rejected: %d referenced leaves not found in store", len(notFoundHashes))
		write(utils.BuildErrorMessage("Partial upload rejected: some referenced leaves don't exist on this relay", nil))
		return fmt.Errorf("missing leaves: %d not found", len(notFoundHashes))
	}

	logging.Infof("Partial DAG verified: %d existing leaves linked", verifiedCount)
	return nil
}
