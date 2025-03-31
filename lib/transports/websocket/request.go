package websocket

import (
	"context"

	jsoniter "github.com/json-iterator/go"

	"github.com/gofiber/contrib/websocket"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
)

func handleReqMessage(c *websocket.Conn, env *nostr.ReqEnvelope, state *connectionState, store stores.Store) {
	// If the user is authenticated, check if they're blocked
	if state.authenticated && terminateIfBlocked(c, state, store) {
		return
	}

	// Anonymous read-only access is allowed (no authentication required)
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	handler := lib_nostr.GetHandler("filter")

	if handler != nil {
		_, cancelFunc := context.WithCancel(context.Background())

		setListener(env.SubscriptionID, c, env.Filters, cancelFunc)

		read := func() ([]byte, error) {
			return json.Marshal(env)
		}

		write := func(messageType string, params ...interface{}) {
			response := lib_nostr.BuildResponse(messageType, params)
			if len(response) > 0 {
				handleIncomingMessage(c, response)
			}
		}

		handler(read, write)
	}
}
