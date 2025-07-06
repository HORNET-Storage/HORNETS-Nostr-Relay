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

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/dag"
	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	types "github.com/HORNET-Storage/hornet-storage/lib"
	utils "github.com/HORNET-Storage/hornet-storage/lib/handlers/scionic"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/sessions/libp2p/middleware"
	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
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
			write(lib_stream.BuildErrorMessage("Failed to recieve upload message", err))
			return
		}

		publicKey, err := signing.DeserializePublicKey(message.PublicKey)
		if err != nil {
			write(lib_stream.BuildErrorMessage("Failed to deserialize public key", err))
			return
		}

		signatureBytes, err := hex.DecodeString(message.Signature)
		if err != nil {
			write(lib_stream.BuildErrorMessage("Failed to deserialize signature", err))
			return
		}

		signature, err := schnorr.ParseSignature(signatureBytes)
		if err != nil {
			write(lib_stream.BuildErrorMessage("Failed to deserialize signature", err))
			return
		}

		err = signing.VerifySerializedCIDSignature(signature, message.Root, publicKey)
		if err != nil {
			write(lib_stream.BuildErrorMessage("Signature failed to verify", err))
			return
		}

		logging.Infof("Dag uploading: " + message.Root)

		dag := &merkle_dag.Dag{
			Root:  message.Root,
			Leafs: make(map[string]*merkle_dag.DagLeaf),
		}

		packet := merkle_dag.TransmissionPacketFromSerializable(&message.Packet)

		err = packet.Leaf.VerifyRootLeaf()
		if err != nil {
			write(lib_stream.BuildErrorMessage("Failed to verify root leaf", err))
			return
		}

		dag.ApplyTransmissionPacket(packet)

		err = dag.Verify()
		if err != nil {
			write(lib_stream.BuildErrorMessage(fmt.Sprintf("Failed to verify partial dag with %d leaves", len(dag.Leafs)), err))
			return
		}

		if !canUploadDag(packet.Leaf, &message.PublicKey, &message.Signature) {
			write(lib_stream.BuildErrorMessage("Not allowed to upload this", nil))
			return
		}

		err = write(lib_stream.BuildResponseMessage(true))
		if err != nil {
			write(lib_stream.BuildErrorMessage("Failed to write response to stream", err))
			return
		}

		dagData := types.DagData{
			PublicKey: message.PublicKey,
			Signature: message.Signature,
		}

		for {
			message, err := read()
			if err != nil {
				write(lib_stream.BuildErrorMessage("Failed to recieve upload message in time", nil))
				return
			}

			packet := merkle_dag.TransmissionPacketFromSerializable(&message.Packet)

			err = packet.Leaf.VerifyLeaf()
			if err != nil {
				write(lib_stream.BuildErrorMessage("Failed to verify leaf", err))
				return
			}

			dag.ApplyTransmissionPacket(packet)

			err = dag.Verify()
			if err != nil {
				write(lib_stream.BuildErrorMessage(fmt.Sprintf("Failed to verify partial dag with %d leaves", len(dag.Leafs)), err))
				return
			}

			err = write(lib_stream.BuildResponseMessage(true))
			if err != nil {
				write(lib_stream.BuildErrorMessage("Failed to write response to stream", err))
				break
			}

			if len(dag.Leafs) >= (dag.Leafs[dag.Root].LeafCount + 1) {
				logging.Infof("All leaves receieved")
				break
			}
		}

		// Verify the dag
		err = dag.Verify()
		if err != nil {
			write(lib_stream.BuildErrorMessage("Failed to verify dag", err))
			logging.Infof("Failed to verify dag???")
			return
		}

		logging.Infof("Dag verified")

		// Check to see if any data in the dag is not allowed to be stored by this relay
		for _, leaf := range dag.Leafs {
			if leaf.Type == "File" {
				data, err := dag.GetContentFromLeaf(leaf)
				if err != nil {
					write(lib_stream.BuildErrorMessage("Failed to extract content from file leaf", err))
					return
				}

				mimeType := mimetype.Detect(data)

				if !utils.IsMimeTypePermitted(mimeType.String()) {
					write(lib_stream.BuildErrorMessage("Mime type is not allowed to be stored by this relay ("+mimeType.String()+")", err))
					return
				}
			}
		}

		dagData.Dag = *dag

		err = store.StoreDag(&dagData, false)
		if err != nil {
			write(lib_stream.BuildErrorMessage("Failed to commit dag to long term store", err))
			return
		}

		// Calculate total DAG size for subscription tracking
		var totalDagSize int64
		for _, leaf := range dag.Leafs {
			if leaf.Content != nil {
				totalDagSize += int64(len(leaf.Content))
			}
		}

		// Update subscription storage usage for the DAG upload asynchronously
		go func(pubKey string, size int64) {
			subManager := subscription.GetGlobalManager()
			if subManager != nil {
				if err := subManager.UpdateStorageUsage(pubKey, size); err != nil {
					logging.Infof("Warning: Failed to update storage usage for pubkey %s: %v\n", pubKey, err)
				}
			} else {
				logging.Infof("Warning: Global subscription manager not available, storage not tracked for pubkey %s\n", pubKey)
			}
		}(message.PublicKey, totalDagSize)

		logging.Infof("Dag Uploaded: " + message.Root)

		handleRecievedDag(&dagData.Dag, &message.PublicKey)
	}

	return handler
}
