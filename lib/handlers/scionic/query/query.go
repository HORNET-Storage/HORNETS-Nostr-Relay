package query

import (
	"context"
	"fmt"

	"github.com/fxamacker/cbor/v2"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	utils "github.com/HORNET-Storage/hornet-storage/lib/handlers/scionic"
	"github.com/HORNET-Storage/hornet-storage/lib/sessions/libp2p/middleware"
	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
)

func AddQueryHandler(libp2phost host.Host, store stores.Store) {
	libp2phost.SetStreamHandler("/query", middleware.SessionMiddleware(libp2phost)(BuildQueryStreamHandler(store)))
}

func BuildQueryStreamHandler(store stores.Store) func(network.Stream) {
	queryStreamHandler := func(stream network.Stream) {
		enc := cbor.NewEncoder(stream)

		libp2pStream := &types.Libp2pStream{Stream: stream, Ctx: context.Background()}

		message, err := utils.WaitForAdvancedQueryMessage(libp2pStream)
		if err != nil {
			utils.WriteErrorToStream(libp2pStream, "Failed to recieve upload message in time", nil)

			stream.Close()
			return
		}

		hashes, err := store.QueryDag(message.Filter, false)
		if err != nil {
			utils.WriteErrorToStream(libp2pStream, "Failed to query database", nil)

			stream.Close()
			return
		}

		fmt.Printf("Query Found %d hashes\n", len(hashes))

		response := types.QueryResponse{
			Hashes: hashes,
		}

		if err := enc.Encode(&response); err != nil {
			utils.WriteErrorToStream(libp2pStream, "Failed to encode response", nil)

			stream.Close()
			return
		}

		stream.Close()
	}

	return queryStreamHandler
}
