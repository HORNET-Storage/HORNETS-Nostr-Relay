package upload

import (
	"context"
	"log"

	"github.com/gabriel-vasile/mimetype"
	"github.com/gofiber/contrib/websocket"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	utils "github.com/HORNET-Storage/hornet-storage/lib/handlers/scionic"
	"github.com/HORNET-Storage/hornet-storage/lib/sessions/libp2p/middleware"
	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"
)

type CanUploadDagFunc func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool
type HandleUploadedDagFunc func(dag *merkle_dag.Dag, pubKey *string)

func AddUploadHandlerForLibp2p(ctx context.Context, libp2phost host.Host, store stores.Store, canUploadDag CanUploadDagFunc, handleRecievedDag HandleUploadedDagFunc) {
	handler := BuildUploadStreamHandler(store, canUploadDag, handleRecievedDag)

	wrapper := func(stream network.Stream) {
		read := func() (*types.UploadMessage, error) {
			libp2pStream := &types.Libp2pStream{Stream: stream, Ctx: ctx}

			log.Println("[libp2p] Waiting for message")

			return utils.WaitForUploadMessage(libp2pStream)
		}

		write := func(message interface{}) error {
			libp2pStream := &types.Libp2pStream{Stream: stream, Ctx: ctx}

			log.Println("[libp2p] Writing message")

			return utils.WriteMessageToStream(libp2pStream, message)
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
		read := func() (*types.UploadMessage, error) {
			wsStream := &types.WebSocketStream{Conn: conn, Ctx: ctx}

			log.Println("[websocket] Waiting for message")

			return utils.WaitForUploadMessage(wsStream)
		}

		write := func(message interface{}) error {
			wsStream := &types.WebSocketStream{Conn: conn, Ctx: ctx}

			log.Println("[websocket] Writing message")

			return utils.WriteMessageToStream(wsStream, message)
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

		err = message.Leaf.VerifyRootLeaf()
		if err != nil {
			write(utils.BuildErrorMessage("Failed to verify root leaf", err))
			return
		}

		if !canUploadDag(&message.Leaf, &message.PublicKey, &message.Signature) {
			write(utils.BuildErrorMessage("Not allowed to upload this", nil))
			return
		}

		rootData := &types.DagLeafData{
			PublicKey: message.PublicKey,
			Signature: message.Signature,
			Leaf:      message.Leaf,
		}

		err = store.StoreLeaf(message.Root, rootData, true)
		if err != nil {
			write(utils.BuildErrorMessage("Failed to verify root leaf", err))
			return
		}

		err = write(utils.BuildResponseMessage(true))
		if err != nil {
			write(utils.BuildErrorMessage("Failed to write response to stream", err))
			return
		}

		leafCount := 1

		for {
			message, err := read()
			if err != nil {
				write(utils.BuildErrorMessage("Failed to recieve upload message in time", nil))
				break
			}

			err = message.Leaf.VerifyLeaf()
			if err != nil {
				write(utils.BuildErrorMessage("Failed to verify leaf", err))
				break
			}

			parentData, err := store.RetrieveLeaf(message.Root, message.Parent, false, true)
			if err != nil {
				write(utils.BuildErrorMessage("Failed to find parent leaf", err))
				break
			}

			parent := parentData.Leaf

			if message.Branch != nil {
				err = parent.VerifyBranch(message.Branch)
				if err != nil {
					write(utils.BuildErrorMessage("Failed to verify leaf branch", err))
					break
				}
			}

			data := &types.DagLeafData{
				Leaf: message.Leaf,
			}

			err = store.StoreLeaf(message.Root, data, true)
			if err != nil {
				write(utils.BuildErrorMessage("Failed to add leaf to block database", err))
				return
			}

			leafCount++

			err = write(utils.BuildResponseMessage(true))
			if err != nil {
				write(utils.BuildErrorMessage("Failed to write response to stream", err))
				break
			}
		}

		// Rebuild the dag from the temporary database
		dagData, err := store.BuildDagFromStore(message.Root, true, true)
		if err != nil {
			write(utils.BuildErrorMessage("Failed to build dag from provided leaves", err))
			return
		}

		// Verify the dag
		err = dagData.Dag.Verify()
		if err != nil {
			write(utils.BuildErrorMessage("Failed to verify dag", err))
			return
		}

		// Check to see if any data in the dag is not allows to be stored by this relay
		for _, leaf := range dagData.Dag.Leafs {
			if leaf.Type == "File" {
				data, err := dagData.Dag.GetContentFromLeaf(leaf)
				if err != nil {
					write(utils.BuildErrorMessage("Failed to extract content from file leaf", err))
					return
				}

				mimeType := mimetype.Detect(data)

				if !utils.IsMimeTypePermitted(mimeType.String()) {
					write(utils.BuildErrorMessage("Mime type is not allowed to be stored by this relay ("+mimeType.String()+")", err))
					return
				}

				err = store.GetStatsStore().SaveFile(dagData.Dag.Root, leaf.Hash, leaf.ItemName, mimeType.String(), len(leaf.Links), int64(len(data)))
				if err != nil {

				}
			}
		}

		// Store dag in the long term database
		err = store.StoreDag(dagData, false)
		if err != nil {
			write(utils.BuildErrorMessage("Failed to commit dag to long term store", err))
		}

		handleRecievedDag(&dagData.Dag, &message.PublicKey)
	}

	return handler
}
