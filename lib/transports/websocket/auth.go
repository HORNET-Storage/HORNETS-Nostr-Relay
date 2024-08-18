package websocket

import (
	"github.com/gofiber/contrib/websocket"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/sessions"
)

func handleAuthMessage(c *websocket.Conn, env *nostr.AuthEnvelope, challenge string, state *connectionState) {
	write := func(messageType string, params ...interface{}) {
		response := lib_nostr.BuildResponse(messageType, params)
		if len(response) > 0 {
			handleIncomingMessage(c, response)
		}
	}

	if env.Event.Kind != 22242 {
		write("OK", env.Event.ID, false, "Error auth event kind must be 22242")
		return
	}

	isValid, errMsg := lib_nostr.AuthTimeCheck(env.Event.CreatedAt.Time().Unix())
	if !isValid {
		write("OK", env.Event.ID, false, errMsg)
		return
	}

	success, err := env.Event.CheckSignature()
	if err != nil {
		write("NOTICE", "Failed to check signature")
		return
	}

	if !success {
		write("OK", env.Event.ID, false, "Signature failed to verify")
		return
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
					return
				}
			}
		}
	}

	if !hasRelayTag || !hasChallengeTag {
		write("OK", env.Event.ID, false, "Error event does not have required tags")
		return
	}

	err = sessions.CreateSession(env.Event.PubKey)
	if err != nil {
		write("NOTICE", "Failed to create session")
		return
	}

	err = AuthenticateConnection(c)
	if err != nil {
		write("OK", env.Event.ID, false, "Error authorizing connection")
		return
	}

	state.authenticated = true

	if state.authenticated {
		// Authenticating user session.
		userSession := sessions.GetSession(env.Event.PubKey)
		userSession.Signature = &env.Event.Sig
		userSession.Authenticated = true
	}

	write("OK", env.Event.ID, true, "")
}
