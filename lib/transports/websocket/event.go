package websocket

import (
	"fmt"
	"log"

	"github.com/gofiber/contrib/websocket"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
)

func handleEventMessage(c *websocket.Conn, env *nostr.EventEnvelope, _ *connectionState, store stores.Store) {
	write := func(messageType string, params ...interface{}) {
		response := lib_nostr.BuildResponse(messageType, params)
		if len(response) > 0 {
			handleIncomingMessage(c, response)
		}
	}

	// Always check if the event is from a blocked pubkey regardless of authentication
	// Note: We use the pubkey from the event itself, not the connection state,
	// as events could be relayed from other pubkeys or unauthenticated users
	if store != nil {
		isBlocked, err := store.IsBlockedPubkey(env.Event.PubKey)
		if err != nil {
			log.Printf("Error checking if pubkey is blocked: %v", err)
		} else if isBlocked {
			log.Printf("Rejected event from blocked pubkey: %s", env.Event.PubKey)
			// Notify the client that their event was rejected
			write("OK", env.Event.ID, false, "Event rejected: Pubkey is blocked")
			return
		}
	}

	// Check write access permissions using H.O.R.N.E.T Allowed Users system
	if accessControl := GetAccessControl(); accessControl != nil {
		canWrite, err := accessControl.CanWrite(env.Event.PubKey)
		if err != nil {
			log.Printf("Error checking write access for %s: %v", env.Event.PubKey, err)
			// Continue on error to avoid blocking due to misconfiguration
		} else if !canWrite {
			log.Printf("Write access denied for pubkey: %s", env.Event.PubKey)
			write("OK", env.Event.ID, false, "Event rejected: Write access denied")
			return
		}
	}
	settings, err := lib_nostr.LoadRelaySettings()
	if err != nil {
		log.Printf("Failed to load relay settings: %v", err)
	}

	if settings.Mode == "blacklist" {
		handleBlacklistModeEvent(c, env)
	} else if settings.Mode == "whitelist" {
		handleWhitelistModeEvent(c, env)
	}
}

func handleBlacklistModeEvent(c *websocket.Conn, env *nostr.EventEnvelope) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	handler := lib_nostr.GetHandler("universal")

	log.Println("handled by blacklist mode.")

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
	log.Println("handled by whitelist mode.")

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
