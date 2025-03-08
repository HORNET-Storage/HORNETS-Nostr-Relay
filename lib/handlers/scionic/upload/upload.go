package upload

import (
	"context"
	"fmt"
	"log"

	"github.com/gabriel-vasile/mimetype"
	"github.com/gofiber/contrib/websocket"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/dag"
	types "github.com/HORNET-Storage/hornet-storage/lib"
	utils "github.com/HORNET-Storage/hornet-storage/lib/handlers/scionic"
	"github.com/HORNET-Storage/hornet-storage/lib/sessions/libp2p/middleware"
	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
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

		dag := &merkle_dag.Dag{
			Root:  message.Root,
			Leafs: make(map[string]*merkle_dag.DagLeaf),
		}

		packet := merkle_dag.TransmissionPacketFromSerializable(&message.Packet)

		err = packet.Leaf.VerifyRootLeaf()
		if err != nil {
			write(utils.BuildErrorMessage("Failed to verify root leaf", err))
			return
		}

		dag.ApplyTransmissionPacket(packet)

		err = dag.Verify()
		if err != nil {
			write(utils.BuildErrorMessage(fmt.Sprintf("Failed to verify partial dag with %d leaves", len(dag.Leafs)), err))
			return
		}

		if !canUploadDag(packet.Leaf, &message.PublicKey, &message.Signature) {
			write(utils.BuildErrorMessage("Not allowed to upload this", nil))
			return
		}

		err = write(utils.BuildResponseMessage(true))
		if err != nil {
			write(utils.BuildErrorMessage("Failed to write response to stream", err))
			return
		}

		dagData := types.DagData{
			PublicKey: message.PublicKey,
			Signature: message.Signature,
		}

		for {
			message, err := read()
			if err != nil {
				write(utils.BuildErrorMessage("Failed to recieve upload message in time", nil))
				return
			}

			packet := merkle_dag.TransmissionPacketFromSerializable(&message.Packet)

			err = packet.Leaf.VerifyLeaf()
			if err != nil {
				write(utils.BuildErrorMessage("Failed to verify leaf", err))
				return
			}

			dag.ApplyTransmissionPacket(packet)

			err = dag.Verify()
			if err != nil {
				write(utils.BuildErrorMessage(fmt.Sprintf("Failed to verify partial dag with %d leaves", len(dag.Leafs)), err))
				return
			}

			err = write(utils.BuildResponseMessage(true))
			if err != nil {
				write(utils.BuildErrorMessage("Failed to write response to stream", err))
				break
			}

			if len(dag.Leafs) >= (dag.Leafs[dag.Root].LeafCount + 1) {
				fmt.Println("All leaves receieved")
				break
			}
		}

		// Verify the dag
		err = dag.Verify()
		if err != nil {
			write(utils.BuildErrorMessage("Failed to verify dag", err))
			fmt.Println("Failed to verify dag???")
			return
		}

		fmt.Println("Dag verified")

		// Check to see if any data in the dag is not allows to be stored by this relay
		for _, leaf := range dag.Leafs {
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

				store.GetStatsStore().SaveFile(dag.Root, leaf.Hash, leaf.ItemName, mimeType.String(), len(leaf.Links), int64(len(data)))
			}
		}

		fmt.Println("Files saved to stats")

		dagData.Dag = *dag

		err = store.StoreDag(&dagData, false)
		if err != nil {
			write(utils.BuildErrorMessage("Failed to commit dag to long term store", err))
			fmt.Println("Fuck sake")
			return
		}

		handleRecievedDag(&dagData.Dag, &message.PublicKey)
	}

	return handler
}
