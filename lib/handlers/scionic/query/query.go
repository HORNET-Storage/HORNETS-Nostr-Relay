package query

import (
	"fmt"

	"github.com/fxamacker/cbor/v2"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	utils "github.com/HORNET-Storage/hornet-storage/lib/handlers/scionic"
	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
)

func AddQueryHandler(libp2phost host.Host, store stores.Store) {
	libp2phost.SetStreamHandler("/query/1.0.0", BuildQueryStreamHandler(store))
}

func BuildQueryStreamHandler(store stores.Store) func(network.Stream) {
	queryStreamHandler := func(stream network.Stream) {
		enc := cbor.NewEncoder(stream)

		result, message := utils.WaitForQueryMessage(stream)
		if !result {
			utils.WriteErrorToStream(stream, "Failed to recieve upload message in time", nil)

			stream.Close()
			return
		}

		hashes, err := store.QueryDag(message.QueryFilter)
		if err != nil {
			utils.WriteErrorToStream(stream, "Failed to query database", nil)

			stream.Close()
			return
		}

		fmt.Printf("Query Found %d hashes\n", len(hashes))

		response := types.QueryResponse{
			Hashes: hashes,
		}

		if err := enc.Encode(&response); err != nil {
			utils.WriteErrorToStream(stream, "Failed to encode response", nil)

			stream.Close()
			return
		}

		stream.Close()
	}

	return queryStreamHandler
}
