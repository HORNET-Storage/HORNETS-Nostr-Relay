package query

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/sessions/libp2p/middleware"
	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"

	lib_stream "github.com/HORNET-Storage/go-hornet-storage-lib/lib/connmgr"
	libp2p_stream "github.com/HORNET-Storage/go-hornet-storage-lib/lib/connmgr/libp2p"
)

func AddQueryHandler(libp2phost host.Host, store stores.Store) {
	libp2phost.SetStreamHandler("/query", middleware.SessionMiddleware(libp2phost)(BuildQueryStreamHandler(store)))
}

func BuildQueryStreamHandler(store stores.Store) func(network.Stream) {
	queryStreamHandler := func(stream network.Stream) {
		ctx := context.Background()

		libp2pStream := libp2p_stream.New(stream, ctx)

		message, err := lib_stream.WaitForAdvancedQueryMessage(libp2pStream)
		if err != nil {
			lib_stream.WriteErrorToStream(libp2pStream, "Failed to recieve upload message in time", nil)

			stream.Close()
			return
		}

		hashes, err := store.QueryDag(message.Filter, false)
		if err != nil {
			lib_stream.WriteErrorToStream(libp2pStream, "Failed to query database", nil)

			stream.Close()
			return
		}

		fmt.Printf("Query Found %d hashes\n", len(hashes))

		response := types.QueryResponse{
			Hashes: hashes,
		}

		if err := lib_stream.WriteMessageToStream(libp2pStream, response); err != nil {
			lib_stream.WriteErrorToStream(libp2pStream, "Failed to encode response", nil)

			stream.Close()
			return
		}

		stream.Close()
	}

	return queryStreamHandler
}
