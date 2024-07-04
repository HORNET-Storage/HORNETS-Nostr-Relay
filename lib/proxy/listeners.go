package proxy

import (
	"context"
	"fmt"

	"github.com/gofiber/contrib/websocket"
	"github.com/nbd-wtf/go-nostr"
	"github.com/puzpuzpuz/xsync/v3"
)

// Global map to hold all listeners indexed by WebSocket connections and subscription IDs.
var listeners = xsync.NewMapOf[*websocket.Conn, ListenerData]()

// SetListener sets a new listener with given ID, WebSocket connection, filters, and cancel function.
func setListener(id string, ws *websocket.Conn, filters nostr.Filters, challenge *string, cancel context.CancelFunc) {
	conData, _ := listeners.LoadOrCompute(ws, func() ListenerData {
		return ListenerData{
			challenge:     challenge,
			subscriptions: xsync.NewMapOf[string, *Subscription](),
		}
	})

	conData.subscriptions.Store(id, &Subscription{filters: filters, cancel: cancel})
	conData.challenge = challenge
	conData.authenticated = false
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
			fmt.Println("No more subscriptions for this connection. All listeners removed.")
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
		conData.subscriptions.Range(func(id string, listener *Subscription) bool {
			if !listener.filters.Match(event) {
				return true
			}
			if err := ws.WriteJSON(nostr.EventEnvelope{SubscriptionID: &id, Event: *event}); err != nil {
				fmt.Printf("Error notifying listener: %v\n", err)
			}
			return true
		})
		return true
	})
}

// LogCurrentSubscriptions logs current subscriptions for debugging purposes.
func logCurrentSubscriptions() {
	empty := true // Assume initially that there are no subscriptions
	listeners.Range(func(ws *websocket.Conn, conData ListenerData) bool {
		conData.subscriptions.Range(func(id string, listener *Subscription) bool {
			fmt.Printf("Subscription ID: %s, Filters: %+v\n", id, listener.filters)
			empty = false // Found at least one subscription, so not empty
			return true
		})
		return true
	})
	if empty {
		fmt.Println("No active subscriptions.")
	}
}

func GetListenerChallenge(ws *websocket.Conn) (*string, error) {
	conData, ok := listeners.Load(ws)
	if !ok {
		return nil, fmt.Errorf("no listeners found for this WebSocket connection")
	}

	return conData.challenge, nil
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
