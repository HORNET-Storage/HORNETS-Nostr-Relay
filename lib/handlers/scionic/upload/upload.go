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
	if err := processPacketStreaming(dagStore, packet, &totalDagSize, &leafCount, write); err != nil {
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

			if err := processPacketStreaming(dagStore, pkt, &totalDagSize, &leafCount, write); err != nil {
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

	logging.Infof("Streaming upload complete: %d leaves, %d bytes", leafCount, totalDagSize)
}

func processPacketStreaming(dagStore *merkle_dag.DagStore, packet *merkle_dag.BatchedTransmissionPacket, totalSize *int64, leafCount *int, write utils.DagWriter) error {
	if err := dagStore.AddBatchedTransmissionPacket(packet); err != nil {
		write(utils.BuildErrorMessage(fmt.Sprintf("Failed to apply packet with %d leaves", len(packet.Leaves)), err))
		return err
	}

	for _, leaf := range packet.Leaves {
		*leafCount++
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
