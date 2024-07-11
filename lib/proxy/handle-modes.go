package proxy

import (
	"fmt"
	"log"

	jsoniter "github.com/json-iterator/go"

	"github.com/gofiber/contrib/websocket"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func handleUnlimitedModeEvent(c *websocket.Conn, env *nostr.EventEnvelope) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	log.Println("Unlimited Mode processing.")
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

		if verifyNote(&env.Event) {
			handler(read, write)
		} else {
			write("OK", env.ID, false, "Invalid note")
		}
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

		if verifyNote(&env.Event) {
			handler(read, write)
		} else {
			write("OK", env.ID, false, "Invalid note")
		}
	}

}
