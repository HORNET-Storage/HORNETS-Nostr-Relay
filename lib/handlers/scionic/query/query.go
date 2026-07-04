package query

import (
	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"

	lib_types "github.com/HORNET-Storage/hdk-nostr-go/lib"
	lib_stream "github.com/HORNET-Storage/hdk-nostr-go/lib/connmgr"
	hsListener "github.com/HORNET-Storage/hdk-nostr-go/lib/connmgr/hyperswarm"
)

func AddQueryHandler(listener *hsListener.HyperswarmListener, store stores.Store) {
	listener.SetStreamHandler("/query", BuildQueryStreamHandler(store))
}

func BuildQueryStreamHandler(store stores.Store) hsListener.StreamHandler {
	queryStreamHandler := func(stream lib_types.Stream) {
		defer stream.Close()

		message, err := lib_stream.WaitForAdvancedQueryMessage(stream)
		if err != nil {
			lib_stream.WriteErrorToStream(stream, "Failed to recieve upload message in time", err)
			return
		}

		hashes, err := store.QueryDag(message.Filter)
		if err != nil {
			lib_stream.WriteErrorToStream(stream, "Failed to query database", err)
			return
		}

		logging.Infof("Query Found %d hashes\n", len(hashes))

		response := types.QueryResponse{
			Hashes: hashes,
		}

		if err := lib_stream.WriteMessageToStream(stream, response); err != nil {
			lib_stream.WriteErrorToStream(stream, "Failed to encode response", err)
			return
		}
	}

	return queryStreamHandler
}
