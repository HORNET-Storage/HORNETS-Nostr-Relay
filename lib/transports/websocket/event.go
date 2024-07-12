package websocket

import (
	"fmt"
	"log"

	"github.com/gofiber/contrib/websocket"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func handleEventMessage(c *websocket.Conn, env *nostr.EventEnvelope) {
	settings, err := lib_nostr.LoadRelaySettings()
	if err != nil {
		log.Printf("Failed to load relay settings: %v", err)
	}

	if settings.Mode == "unlimited" {
		handleUnlimitedModeEvent(c, env)
	} else if settings.Mode == "smart" {
		handleSmartModeEvent(c, env)
	}
}

func handleUnlimitedModeEvent(c *websocket.Conn, env *nostr.EventEnvelope) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	handler := lib_nostr.GetHandler("universal")

	if handler != nil {
		notifyListeners(&env.Event)

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

func handleSmartModeEvent(c *websocket.Conn, env *nostr.EventEnvelope) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	handler := lib_nostr.GetHandler(fmt.Sprintf("kind/%d", env.Kind))

	if handler != nil {
		notifyListeners(&env.Event)

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
