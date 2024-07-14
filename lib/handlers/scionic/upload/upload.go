package upload

import (
	"log"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	utils "github.com/HORNET-Storage/hornet-storage/lib/handlers/scionic"
	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"
)

func AddUploadHandler(libp2phost host.Host, store stores.Store, canUploadDag func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool, handleRecievedDag func(dag *merkle_dag.Dag, pubKey *string)) {
	libp2phost.SetStreamHandler("/upload/1.0.0", BuildUploadStreamHandler(store, canUploadDag, handleRecievedDag))
}

func BuildUploadStreamHandler(store stores.Store, canUploadDag func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool, handleRecievedDag func(dag *merkle_dag.Dag, pubKey *string)) func(stream network.Stream) {
	uploadStreamHandler := func(stream network.Stream) {
		result, message := utils.WaitForUploadMessage(stream)
		if !result {
			utils.WriteErrorToStream(stream, "Failed to recieve upload message in time", nil)

			stream.Close()
			return
		}

		err := message.Leaf.VerifyRootLeaf()
		if err != nil {
			utils.WriteErrorToStream(stream, "Failed to verify root leaf", err)

			stream.Close()
			return
		}

		if !canUploadDag(&message.Leaf, &message.PublicKey, &message.Signature) {
			utils.WriteErrorToStream(stream, "Not allowed to upload this", nil)

			stream.Close()
			return
		}

		rootData := &types.DagLeafData{
			PublicKey: message.PublicKey,
			Signature: message.Signature,
			Leaf:      message.Leaf,
		}

		err = store.StoreLeaf(message.Root, rootData)
		if err != nil {
			utils.WriteErrorToStream(stream, "Failed to verify root leaf", err)

			stream.Close()
			return
		}

		err = utils.WriteResponseToStream(stream, true)
		if err != nil || !result {
			log.Printf("Failed to write response to stream: %e\n", err)

			stream.Close()
			return
		}

		leafCount := 1

		for {
			result, message := utils.WaitForUploadMessage(stream)
			if !result {
				utils.WriteErrorToStream(stream, "Failed to recieve upload message in time", nil)

				stream.Close()
				break
			}

			err = message.Leaf.VerifyLeaf()
			if err != nil {
				utils.WriteErrorToStream(stream, "Failed to verify leaf", err)

				stream.Close()
				break
			}

			parentData, err := store.RetrieveLeaf(message.Root, message.Parent, false)
			if err != nil || !result {
				utils.WriteErrorToStream(stream, "Failed to find parent leaf", err)

				stream.Close()
				break
			}

			parent := parentData.Leaf

			if message.Branch != nil {
				err = parent.VerifyBranch(message.Branch)
				if err != nil || !result {
					utils.WriteErrorToStream(stream, "Failed to verify leaf branch", err)

					stream.Close()
					break
				}
			}

			data := &types.DagLeafData{
				Leaf: message.Leaf,
			}

			err = store.StoreLeaf(message.Root, data)
			if err != nil {
				utils.WriteErrorToStream(stream, "Failed to add leaf to block database", err)

				stream.Close()
				return
			}

			leafCount++

			err = utils.WriteResponseToStream(stream, true)
			if err != nil || !result {
				log.Printf("Failed to write response to stream: %e\n", err)

				stream.Close()
				break
			}
		}

		dagData, err := store.BuildDagFromStore(message.Root, true)
		if err != nil {
			utils.WriteErrorToStream(stream, "Failed to build dag from provided leaves: %e", err)

			stream.Close()
			return
		}

		err = dagData.Dag.Verify()
		if err != nil {
			utils.WriteErrorToStream(stream, "Failed to verify dag: %e", err)

			stream.Close()
			return
		}

		handleRecievedDag(&dagData.Dag, &message.PublicKey)

		stream.Close()
	}

	return uploadStreamHandler
}
