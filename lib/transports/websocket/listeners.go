package websocket

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"

	"sync/atomic"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/gofiber/contrib/websocket"
	"github.com/nbd-wtf/go-nostr"
	"github.com/puzpuzpuz/xsync/v3"
)

// Global map to hold all listeners indexed by WebSocket connections and subscription IDs.
var listeners = xsync.NewMapOf[*websocket.Conn, ListenerData]()

// Per-connection write mutexes to prevent concurrent websocket writes
// between the notification processor goroutine and connection handler goroutines.
var connWriteMu = xsync.NewMapOf[*websocket.Conn, *sync.Mutex]()

// Buffered channel for async event notifications.
// Events are queued here by notifyListeners and processed by a dedicated goroutine.
var notificationChan = make(chan nostr.Event, 1000)

// Global challenge variable
var globalChallenge atomic.Value

const challengeLength = 32

// notificationProcessorOnce ensures the notification processor starts exactly once.
var notificationProcessorOnce sync.Once

// getConnWriteMutex returns (or creates) the write mutex for a given connection.
func getConnWriteMutex(ws *websocket.Conn) *sync.Mutex {
	mu, _ := connWriteMu.LoadOrCompute(ws, func() *sync.Mutex {
		return &sync.Mutex{}
	})
	return mu
}

// StartNotificationProcessor starts the background goroutine that processes
// event notifications asynchronously. Safe to call multiple times — only starts once.
func StartNotificationProcessor() {
	notificationProcessorOnce.Do(func() {
		go func() {
			for {
				select {
				case event := <-notificationChan:
					processNotification(&event)
				case <-shutdownChan:
					// Drain any remaining notifications before exiting
					for {
						select {
						case event := <-notificationChan:
							processNotification(&event)
						default:
							return
						}
					}
				}
			}
		}()
		logging.Info("Async notification processor started")
	})
}

// processNotification handles the actual fan-out to all matching listeners.
// Runs on the dedicated notification goroutine — never on the event handler path.
func processNotification(event *nostr.Event) {
	listeners.Range(func(ws *websocket.Conn, conData ListenerData) bool {
		if !conData.authenticated {
			return true // Skip unauthenticated connections
		}
		conData.subscriptions.Range(func(id string, listener *Subscription) bool {
			if !listener.filters.Match(event) {
				return true
			}
			mu := getConnWriteMutex(ws)
			mu.Lock()
			err := ws.WriteJSON(nostr.EventEnvelope{SubscriptionID: &id, Event: *event})
			mu.Unlock()
			if err != nil {
				if !isConnectionClosedError(err) {
					logging.Infof("Error notifying listener: %v", err)
				}
			}
			return true
		})
		return true
	})
}

// notifyListeners queues an event for async notification to all matching listeners.
// This is non-blocking — the event is pushed to a channel and processed by a
// dedicated background goroutine, so the calling event handler is never held up.
func notifyListeners(event *nostr.Event) {
	select {
	case notificationChan <- *event:
		// Event queued successfully
	default:
		// Channel is full — drop the notification to prevent blocking.
		// This should be rare with a 1000-event buffer; if it happens
		// frequently, increase the buffer size.
		logging.Infof("Warning: notification channel full, dropping notification for event %s", event.ID)
	}
}

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
			removed = true
		}
		if conData.subscriptions.Size() == 0 {
			listeners.Delete(ws)
			connWriteMu.Delete(ws)
		}
	}
	return removed
}

// RemoveListener removes all listeners associated with a WebSocket connection.
func removeListener(ws *websocket.Conn) {
	listeners.Delete(ws)
	connWriteMu.Delete(ws)
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
	val := globalChallenge.Load()
	if val == nil {
		// Generate challenge if not set
		challenge, err := generateGlobalChallenge()
		if err != nil {
			return ""
		}
		return challenge
	}
	return val.(string)
}

// InitGlobalChallenge initializes the global challenge for testing or manual setup
func InitGlobalChallenge() error {
	_, err := generateGlobalChallenge()
	return err
}
