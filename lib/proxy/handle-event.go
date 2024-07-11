package proxy

import (
	"log"

	"github.com/gofiber/contrib/websocket"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func handleEventMessage(c *websocket.Conn, env *nostr.EventEnvelope) {
	log.Println("Received EVENT message:", env.Kind)

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
