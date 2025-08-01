package websocket

import (
	"fmt"

	"github.com/gofiber/contrib/websocket"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
)

func handleEventMessage(c *websocket.Conn, env *nostr.EventEnvelope, _ *connectionState, store stores.Store) {
	// Always check if the event is from a blocked pubkey regardless of authentication
	// Note: We use the pubkey from the event itself, not the connection state,
	// as events could be relayed from other pubkeys or unauthenticated users
	write := func(messageType string, params ...interface{}) {
		response := lib_nostr.BuildResponse(messageType, params)
		if len(response) > 0 {
			handleIncomingMessage(c, response)
		}
	}

	if store != nil {
		isBlocked, err := store.IsBlockedPubkey(env.Event.PubKey)
		if err != nil {
			logging.Infof("Error checking if pubkey is blocked: %v", err)
		} else if isBlocked {
			logging.Infof("Rejected event from blocked pubkey: %s", env.Event.PubKey)

			// Notify the client that their event was rejected
			write("OK", env.Event.ID, false, "Event rejected: Pubkey is blocked")
			return
		}
	}

	// Check write access permissions using H.O.R.N.E.T Allowed Users system
	if accessControl := GetAccessControl(); accessControl != nil {
		err := accessControl.CanWrite(env.Event.PubKey)
		if err != nil {
			logging.Infof("Write access denied for pubkey: %s", env.Event.PubKey)
			write("OK", env.Event.ID, false, "Event rejected: Write access denied")
			return
		}
	}

	if viper.GetString("event_filtering.mode") == "blacklist" {
		handleBlacklistModeEvent(c, env)
	} else {
		handleWhitelistModeEvent(c, env)
	}
}

func handleBlacklistModeEvent(c *websocket.Conn, env *nostr.EventEnvelope) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	handler := lib_nostr.GetHandler("universal")

	logging.Info("handled by blacklist mode.")

	read := func() ([]byte, error) {
		return json.Marshal(env)
	}

	write := func(messageType string, params ...interface{}) {
		response := lib_nostr.BuildResponse(messageType, params)
		if len(response) > 0 {
			handleIncomingMessage(c, response)
		}
	}

	if handler != nil {
		notifyListeners(&env.Event)

		handler(read, write)
	} else {
		write("OK", env.Event.ID, false, "Universal handler not supported")
	}
}

func handleWhitelistModeEvent(c *websocket.Conn, env *nostr.EventEnvelope) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	handler := lib_nostr.GetHandler(fmt.Sprintf("kind/%d", env.Kind))
	logging.Info("handled by whitelist mode.")

	read := func() ([]byte, error) {
		return json.Marshal(env)
	}

	write := func(messageType string, params ...interface{}) {
		response := lib_nostr.BuildResponse(messageType, params)
		if len(response) > 0 {
			handleIncomingMessage(c, response)
		}
	}

	if handler != nil {
		notifyListeners(&env.Event)

		handler(read, write)
	} else {
		write("OK", env.Event.ID, false, "Kind not supported")
	}
}
