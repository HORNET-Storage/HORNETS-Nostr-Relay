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

		dag := &merkle_dag.Dag{
			Root:  message.Root,
			Leafs: make(map[string]*merkle_dag.DagLeaf),
		}

		packet := merkle_dag.BatchedTransmissionPacketFromSerializable(&message.Packet)

		err = dag.ApplyAndVerifyBatchedTransmissionPacket(packet)
		if err != nil {
			write(utils.BuildErrorMessage(fmt.Sprintf("Failed to verify partial dag with %d leaves", len(dag.Leafs)), err))
			return
		}

		if canUploadDag != nil {
			rootLeaf := packet.GetRootLeaf()
			if !canUploadDag(rootLeaf, &message.PublicKey, &message.Signature) {
				write(utils.BuildErrorMessage("Not allowed to upload this", nil))
				return
			}
		}

		// Check to see if any data in the dag is not allowed to be stored by this relay and store them
		for _, leaf := range packet.Leaves {
			if leaf.Type == "File" {
				data, err := dag.GetContentFromLeaf(leaf)
				if err != nil {
					write(utils.BuildErrorMessage("Failed to extract content from file leaf", err))
					return
				}

				mimeType := mimetype.Detect(data)

				if !utils.IsMimeTypePermitted(mimeType.String()) {
					write(utils.BuildErrorMessage("Mime type is not allowed to be stored by this relay ("+mimeType.String()+")", err))
					return
				}
			}
		}

		serialiedPublicKey, err := signing.SerializePublicKey(publicKey)
		if err != nil {
			write(utils.BuildErrorMessage("Failed to serialize public key", err))
			return
		}

		dagData := types.DagData{
			PublicKey: *serialiedPublicKey,
			Signature: hex.EncodeToString(signature.Serialize()),
		}

		if !message.IsFinalPacket {
			err = write(lib_stream.BuildResponseMessage(true))
			if err != nil {
				write(utils.BuildErrorMessage("Failed to write response to stream", err))
				return
			}

			for {
				message, err := read()
				if err != nil {
					write(utils.BuildErrorMessage("Failed to recieve upload message in time", nil))
					return
				}
				packet := merkle_dag.BatchedTransmissionPacketFromSerializable(&message.Packet)

				err = dag.ApplyAndVerifyBatchedTransmissionPacket(packet)
				if err != nil {
					write(utils.BuildErrorMessage(fmt.Sprintf("Failed to verify partial dag with %d leaves", len(dag.Leafs)), err))
					return
				}

				// Check to see if any data in the dag is not allowed to be stored by this relay and store them
				for _, leaf := range packet.Leaves {
					if leaf.Type == "File" {
						data, err := dag.GetContentFromLeaf(leaf)
						if err != nil {
							write(utils.BuildErrorMessage("Failed to extract content from file leaf", err))
							return
						}

						mimeType := mimetype.Detect(data)

						if !utils.IsMimeTypePermitted(mimeType.String()) {
							write(utils.BuildErrorMessage("Mime type is not allowed to be stored by this relay ("+mimeType.String()+")", err))
							return
						}
					}
				}

				err = write(lib_stream.BuildResponseMessage(true))
				if err != nil {
					write(utils.BuildErrorMessage("Failed to write response to stream", err))
					break
				}
				if message.IsFinalPacket {
					break
				}
			}
		}

		err = dag.Verify()
		if err != nil {
			write(utils.BuildErrorMessage("Failed to verify dag", err))
			return
		}

		// Recalculate parent hashes to ensure relay can trust them for upward traversal
		err = dag.IterateDag(func(leaf *merkle_dag.DagLeaf, parent *merkle_dag.DagLeaf) error {
			if parent != nil {
				// Set the parent hash for this child
				leaf.ParentHash = parent.Hash
			} else {
				// Root has no parent
				leaf.ParentHash = ""
			}
			return nil
		})
		if err != nil {
			write(utils.BuildErrorMessage("Failed to recalculate parent hashes", err))
			return
		}

		dagData.Dag = *dag

		err = store.StoreDag(&dagData)
		if err != nil {
			write(utils.BuildErrorMessage("Failed to commit dag to long term store", err))
			return
		}

		// Send final success response to client
		err = write(lib_stream.BuildResponseMessage(true))
		if err != nil {
			write(utils.BuildErrorMessage("Failed to write final response to stream", err))
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
			}
		}(message.PublicKey, totalDagSize)

		if handleRecievedDag != nil {
			handleRecievedDag(&dagData.Dag, &message.PublicKey)
		}
	}

	return handler
}
