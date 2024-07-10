package proxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"

	"sync/atomic"

	"github.com/gofiber/contrib/websocket"
	"github.com/nbd-wtf/go-nostr"
	"github.com/puzpuzpuz/xsync/v3"
)

// Global map to hold all listeners indexed by WebSocket connections and subscription IDs.
var listeners = xsync.NewMapOf[*websocket.Conn, ListenerData]()

// Global challenge variable
var globalChallenge atomic.Value

const challengeLength = 32

// SetListener sets a new listener with given ID, WebSocket connection, filters, and cancel function.
func setListener(id string, ws *websocket.Conn, filters nostr.Filters, cancel context.CancelFunc) {
	conData, _ := listeners.LoadOrCompute(ws, func() ListenerData {
		return ListenerData{
			challenge:     "",
			subscriptions: xsync.NewMapOf[string, *Subscription](),
		}
	})

	conData.subscriptions.Store(id, &Subscription{filters: filters, cancel: cancel})
	conData.authenticated = false
	listeners.Store(ws, conData)
}

// RemoveListenerId removes a listener by its ID and cancels its context.
// Returns true if a listener was successfully found and removed, false otherwise.
func removeListenerId(ws *websocket.Conn, id string) bool {
	removed := false
	if conData, ok := listeners.Load(ws); ok {
		if listener, ok := conData.subscriptions.LoadAndDelete(id); ok {
			listener.cancel()
			removed = true // Indicate a listener was removed
		}
		if conData.subscriptions.Size() == 0 {
			listeners.Delete(ws)
		}
	}
	return removed
}

// RemoveListener removes all listeners associated with a WebSocket connection.
func removeListener(ws *websocket.Conn) {
	listeners.Delete(ws)
}

// NotifyListeners notifies all listeners with an event if it matches their filters.
func notifyListeners(event *nostr.Event) {
	listeners.Range(func(ws *websocket.Conn, conData ListenerData) bool {
		if !conData.authenticated {
			return true // Skip notification if not authenticated
		}
		conData.subscriptions.Range(func(id string, listener *Subscription) bool {
			if !listener.filters.Match(event) {
				return true
			}
			if err := ws.WriteJSON(nostr.EventEnvelope{SubscriptionID: &id, Event: *event}); err != nil {
				log.Printf("Error notifying listener: %v\n", err)
			}
			return true
		})
		return true
	})
}

func GetListenerChallenge(ws *websocket.Conn) (*string, error) {
	conData, ok := listeners.Load(ws)
	if !ok {
		return nil, fmt.Errorf("no listeners found for this WebSocket connection")
	}

	return &conData.challenge, nil
}

func AuthenticateConnection(ws *websocket.Conn) error {
	conData, ok := listeners.Load(ws)
	if !ok {
		return fmt.Errorf("no listeners found for this WebSocket connection")
	}

	conData.authenticated = true
	listeners.Store(ws, conData)

	return nil
}

// Generate the global challenge
func generateGlobalChallenge() (string, error) {
	bytes := make([]byte, challengeLength)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %v", err)
	}
	challenge := hex.EncodeToString(bytes)
	globalChallenge.Store(challenge)
	return challenge, nil
}

// Get the global challenge
func getGlobalChallenge() string {
	return globalChallenge.Load().(string)
}
