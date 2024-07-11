package proxy

import (
	"log"

	"github.com/gofiber/contrib/websocket"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func handleAuthMessage(c *websocket.Conn, env *nostr.AuthEnvelope, challenge string, state *connectionState) {
	log.Println("Received AUTH message")

	write := func(messageType string, params ...interface{}) {
		response := lib_nostr.BuildResponse(messageType, params)
		if len(response) > 0 {
			handleIncomingMessage(c, response)
		}
	}

	if env.Event.Kind != 22242 {
		write("OK", env.Event.ID, false, "Error auth event kind must be 22242")
	}

	isValid, errMsg := lib_nostr.AuthTimeCheck(env.Event.CreatedAt.Time().Unix())
	if !isValid {
		write("OK", env.Event.ID, false, errMsg)
	}

	result, err := env.Event.CheckSignature()
	if err != nil || !result {
		write("OK", env.Event.ID, false, "Error checking event signature")
	}

	var hasRelayTag, hasChallengeTag bool
	for _, tag := range env.Event.Tags {
		if len(tag) >= 2 {
			if tag[0] == "relay" {
				hasRelayTag = true
			} else if tag[0] == "challenge" {
				hasChallengeTag = true
				if tag[1] != challenge {
					write("OK", env.Event.ID, false, "Error checking session challenge")
				}
			}
		}
	}

	if !hasRelayTag || !hasChallengeTag {
		write("CLOSE", env.Event.ID, false, "Error event does not have required tags")
	}

	err = AuthenticateConnection(c)
	if err != nil {
		write("OK", env.Event.ID, false, "Error authorizing connection")
	}

	log.Println("Connection successfully authenticated.")
	state.authenticated = true

	successMsg := nostr.OKEnvelope{
		EventID: env.Event.ID,
		OK:      true,
		Reason:  "",
	}

	if err := sendWebSocketMessage(c, successMsg); err != nil {
		log.Printf("Error sending 'OK' envelope over WebSocket: %v", err)
	}

	write("OK", env.Event.ID, true, "")
}
