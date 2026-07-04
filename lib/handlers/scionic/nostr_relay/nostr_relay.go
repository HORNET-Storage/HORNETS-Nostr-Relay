package nostr_relay

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	nostr_auth "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/auth"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	ws "github.com/HORNET-Storage/hornet-storage/lib/transports/websocket"

	lib_types "github.com/HORNET-Storage/go-hornet-storage-lib/lib"
	hsListener "github.com/HORNET-Storage/go-hornet-storage-lib/lib/connmgr/hyperswarm"
)

type dhtAuthState struct {
	pubkey        string
	authenticated bool
}

// AddNostrRelayHandler registers the /nostr protocol on the hyperswarm listener.
// This creates a single bidirectional stream per client that speaks full Nostr
// protocol (EVENT, REQ, CLOSE, AUTH, COUNT) — exactly like a WebSocket connection
// but over the DHT.
func AddNostrRelayHandler(listener *hsListener.HyperswarmListener, store stores.Store) {
	listener.SetStreamHandler("/nostr", buildNostrStreamHandler(store))
}

// buildNostrStreamHandler returns a stream handler that processes Nostr protocol
// messages in a loop, dispatching to the same handlers the WebSocket server uses.
func buildNostrStreamHandler(store stores.Store) hsListener.StreamHandler {
	return func(stream lib_types.Stream) {
		defer stream.Close()

		logging.Info("/nostr: new DHT relay connection")

		// Send AUTH challenge immediately, same as the WebSocket transport does.
		// go-nostr's relay.Auth() waits for this before sending its signed response.
		challenge, err := generateChallenge()
		if err != nil {
			logging.Errorf("/nostr: failed to generate AUTH challenge: %v", err)
			return
		}
		authMsg := lib_nostr.BuildResponse("AUTH", challenge)
		logging.Infof("/nostr: sending AUTH challenge (%d bytes): %s", len(authMsg), string(authMsg[:min(len(authMsg), 200)]))
		if _, err := stream.Write(authMsg); err != nil {
			logging.Errorf("/nostr: failed to send AUTH challenge: %v", err)
			return
		}

		var json = jsoniter.ConfigCompatibleWithStandardLibrary
		authState := &dhtAuthState{}
		scanner := bufio.NewScanner(stream)
		// Allow messages up to 2MB (matches go-nostr's 33MB but reasonable for DHT)
		scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

		// writeFn sends a Nostr protocol response back on the stream.
		//
		// The internal handler convention passes EVENT data as a Go
		// string (JSON text).  BuildResponse would double-encode it
		// (string→quoted-string), which go-nostr can't parse.
		// The WebSocket transport works around this in handleIncomingMessage
		// by re-deserializing and re-serializing through go-nostr types.
		// We do the same here so the bytes on the wire are valid NIP-01.
		writeFn := func(messageType string, params ...interface{}) {
			var json = jsoniter.ConfigCompatibleWithStandardLibrary
			// Flatten variadic params (the filter handler wraps them
			// in an extra []interface{}).
			flat := lib_nostr.ExtractInterfaceValues(params)

			var out []byte
			switch messageType {
			case "EVENT":
				// flat = [subID, eventJSONString]
				if len(flat) >= 2 {
					subID, _ := flat[0].(string)
					eventStr, _ := flat[1].(string)
					var evt nostr.Event
					if err := json.Unmarshal([]byte(eventStr), &evt); err == nil {
						env := nostr.EventEnvelope{SubscriptionID: &subID, Event: evt}
						if b, err := env.MarshalJSON(); err == nil {
							out = append(b, '\n')
						}
					}
				}
			case "EOSE":
				if len(flat) >= 1 {
					subID, _ := flat[0].(string)
					env := nostr.EOSEEnvelope(subID)
					if b, err := env.MarshalJSON(); err == nil {
						out = append(b, '\n')
					}
				}
			case "OK":
				if len(flat) >= 2 {
					eventID, _ := flat[0].(string)
					ok, _ := flat[1].(bool)
					reason := ""
					if len(flat) >= 3 {
						reason, _ = flat[2].(string)
					}
					env := nostr.OKEnvelope{EventID: eventID, OK: ok, Reason: reason}
					if b, err := env.MarshalJSON(); err == nil {
						out = append(b, '\n')
					}
				}
			case "NOTICE":
				if len(flat) >= 1 {
					msg, _ := flat[0].(string)
					env := nostr.NoticeEnvelope(msg)
					if b, err := env.MarshalJSON(); err == nil {
						out = append(b, '\n')
					}
				}
			case "AUTH":
				if len(flat) >= 1 {
					challenge, _ := flat[0].(string)
					env := nostr.AuthEnvelope{Challenge: &challenge}
					if b, err := env.MarshalJSON(); err == nil {
						out = append(b, '\n')
					}
				}
			case "CLOSED":
				if len(flat) >= 1 {
					reason, _ := flat[0].(string)
					env := nostr.ClosedEnvelope{SubscriptionID: "", Reason: reason}
					if len(flat) >= 2 {
						env.SubscriptionID, _ = flat[0].(string)
						env.Reason, _ = flat[1].(string)
					}
					if b, err := env.MarshalJSON(); err == nil {
						out = append(b, '\n')
					}
				}
			default:
				// Fallback to BuildResponse for anything else
				out = lib_nostr.BuildResponse(messageType, params)
			}

			if len(out) > 0 {
				logging.Infof("/nostr: SENDING %s (%d bytes): %s", messageType, len(out), string(out[:min(len(out), 200)]))
				if _, err := stream.Write(out); err != nil {
					logging.Errorf("/nostr: write error sending %s: %v", messageType, err)
				}
			}
		}

		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			envelope := nostr.ParseMessage(line)
			if envelope == nil {
				logging.Infof("/nostr: unparseable message (%d bytes), skipping", len(line))
				continue
			}

			switch env := envelope.(type) {
			case *nostr.EventEnvelope:
				logging.Infof("/nostr: EVENT kind=%d id=%s", env.Kind, env.Event.ID[:16])
				handleEvent(env, writeFn, store, json)

			case *nostr.ReqEnvelope:
				logging.Infof("/nostr: REQ sub=%s filters=%d", env.SubscriptionID, len(env.Filters))
				handleReq(env, writeFn, json, authState)

			case *nostr.CountEnvelope:
				handleCount(env, writeFn, json, authState)

			case *nostr.CloseEnvelope:
				logging.Infof("/nostr: CLOSE %s", string(*env))

			case *nostr.AuthEnvelope:
				logging.Infof("/nostr: AUTH received, event_id=%s", env.Event.ID[:16])
				result, message, ok := nostr_auth.AuthenticateEvent(&env.Event, challenge, store, ws.GetAccessControl())
				if ok {
					authState.pubkey = result.PubKey
					authState.authenticated = true
				}
				writeFn("OK", env.Event.ID, ok, message)

			default:
				logging.Infof("/nostr: unhandled envelope type %T", envelope)
			}
		}

		if err := scanner.Err(); err != nil && err != io.EOF {
			logging.Debugf("/nostr: stream read error: %v", err)
		}

		logging.Debug("/nostr: DHT relay connection closed")
	}
}

// handleEvent dispatches an EVENT message to the appropriate kind handler.
func handleEvent(env *nostr.EventEnvelope, writeFn lib_nostr.KindWriter, store stores.Store, json jsoniter.API) {
	// Check blocked pubkeys
	if store != nil {
		isBlocked, err := store.IsBlockedPubkey(env.Event.PubKey)
		if err != nil {
			logging.Debugf("/nostr: error checking blocked pubkey: %v", err)
		} else if isBlocked {
			logging.Infof("/nostr: rejected event from blocked pubkey: %s", env.Event.PubKey)
			writeFn("OK", env.Event.ID, false, "Event rejected: Pubkey is blocked")
			return
		}
	}

	if accessControl := ws.GetAccessControl(); accessControl != nil {
		if err := accessControl.CanWriteEvent(&env.Event, store); err != nil {
			logging.Infof("/nostr: write access denied for pubkey: %s", env.Event.PubKey)
			writeFn("OK", env.Event.ID, false, "Event rejected: Write access denied")
			return
		}
	}

	// Find handler for this kind
	handler := lib_nostr.GetHandler(fmt.Sprintf("kind/%d", env.Kind))

	if handler != nil {
		if !lib_nostr.IsKindAllowed(env.Kind) {
			logging.Infof("/nostr: rejected kind %d (not in whitelist)", env.Kind)
			writeFn("OK", env.Event.ID, false, fmt.Sprintf("Kind %d not allowed", env.Kind))
			return
		}

		read := func() ([]byte, error) {
			return json.Marshal(env)
		}
		handler(read, writeFn)
	} else if lib_nostr.IsKindAllowed(env.Kind) {
		universalHandler := lib_nostr.GetHandler("universal")
		if universalHandler != nil {
			logging.Infof("/nostr: handling allowed kind %d with universal handler", env.Kind)
			read := func() ([]byte, error) {
				return json.Marshal(env)
			}
			universalHandler(read, writeFn)
		} else {
			writeFn("OK", env.Event.ID, false, "Universal handler not available")
		}
	} else {
		logging.Infof("/nostr: rejected kind %d (not allowed by event filtering config)", env.Kind)
		writeFn("OK", env.Event.ID, false, fmt.Sprintf("Unregistered kind %d not allowed", env.Kind))
	}
}

// handleReq dispatches a REQ message to the filter handler.
func handleReq(env *nostr.ReqEnvelope, writeFn lib_nostr.KindWriter, json jsoniter.API, authState *dhtAuthState) {
	handler := lib_nostr.GetHandler("filter")
	if handler == nil {
		writeFn("NOTICE", "Filter handler not available")
		return
	}

	read := func() ([]byte, error) {
		wrapper := struct {
			Request         *nostr.ReqEnvelope `json:"request"`
			AuthPubkey      string             `json:"auth_pubkey"`
			IsAuthenticated bool               `json:"is_authenticated"`
		}{
			Request:         env,
			AuthPubkey:      authState.pubkey,
			IsAuthenticated: authState.authenticated,
		}
		return json.Marshal(wrapper)
	}

	handler(read, writeFn)
}

// handleCount dispatches a COUNT message to the count handler.
func handleCount(env *nostr.CountEnvelope, writeFn lib_nostr.KindWriter, json jsoniter.API, authState *dhtAuthState) {
	handler := lib_nostr.GetHandler("count")
	if handler == nil {
		writeFn("NOTICE", "Count handler not available")
		return
	}

	read := func() ([]byte, error) {
		wrapper := struct {
			Request         *nostr.CountEnvelope `json:"request"`
			AuthPubkey      string               `json:"auth_pubkey"`
			IsAuthenticated bool                 `json:"is_authenticated"`
		}{
			Request:         env,
			AuthPubkey:      authState.pubkey,
			IsAuthenticated: authState.authenticated,
		}
		return json.Marshal(wrapper)
	}

	handler(read, writeFn)
}

// generateChallenge creates a random hex string for NIP-42 AUTH.
func generateChallenge() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
