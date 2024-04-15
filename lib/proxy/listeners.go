package proxy

import (
	"context"
	"fmt"

	"github.com/gofiber/contrib/websocket"
	"github.com/nbd-wtf/go-nostr"
	"github.com/puzpuzpuz/xsync/v3"
)

// Global map to hold all listeners indexed by WebSocket connections and subscription IDs.
var listeners = xsync.NewMapOf[*websocket.Conn, *xsync.MapOf[string, *Listener]]()

// SetListener sets a new listener with given ID, WebSocket connection, filters, and cancel function.
func setListener(id string, ws *websocket.Conn, filters nostr.Filters, cancel context.CancelFunc) {
	subs, _ := listeners.LoadOrCompute(ws, func() *xsync.MapOf[string, *Listener] {
		return xsync.NewMapOf[string, *Listener]()
	})
	subs.Store(id, &Listener{filters: filters, cancel: cancel})
}

// RemoveListenerId removes a listener by its ID and cancels its context.
// Returns true if a listener was successfully found and removed, false otherwise.
func removeListenerId(ws *websocket.Conn, id string) bool {
	removed := false
	if subs, ok := listeners.Load(ws); ok {
		if listener, ok := subs.LoadAndDelete(id); ok {
			listener.cancel()
			removed = true // Indicate a listener was removed
		}
		if subs.Size() == 0 {
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
	listeners.Range(func(ws *websocket.Conn, subs *xsync.MapOf[string, *Listener]) bool {
		subs.Range(func(id string, listener *Listener) bool {
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
