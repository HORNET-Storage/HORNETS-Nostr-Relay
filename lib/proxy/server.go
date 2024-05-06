package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/nbd-wtf/go-nostr"
	"github.com/puzpuzpuz/xsync/v3"
	"github.com/spf13/viper"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func StartServer() error {
	app := fiber.New()

	// Middleware for handling relay information requests
	app.Use(handleRelayInfoRequests)

	// Setup WebSocket route at the root
	app.Get("/", websocket.New(func(c *websocket.Conn) {
		handleWebSocketConnections(c) // Pass the host to the connection handler
	}))

	port := fmt.Sprintf(":%s", viper.GetString("web_port"))
	return app.Listen(port)
}

// Middleware function to respond with relay information on GET requests
func handleRelayInfoRequests(c *fiber.Ctx) error {
	if c.Method() == "GET" && c.Get("Accept") == "application/nostr+json" {
		relayInfo := getRelayInfo()
		c.Set("Access-Control-Allow-Origin", "*")
		return c.JSON(relayInfo)
	}
	return c.Next()
}

func getRelayInfo() nip11RelayInfo {
	return nip11RelayInfo{
		Name:          viper.GetString("RelayName"),
		Description:   viper.GetString("RelayDescription"),
		Pubkey:        viper.GetString("RelayPubkey"),
		Contact:       viper.GetString("RelayContact"),
		SupportedNIPs: []int{1, 11, 2, 9, 18, 23, 24, 25, 51, 56, 57},
		Software:      viper.GetString("RelaySoftware"),
		Version:       viper.GetString("RelayVersion"),
	}
}

// Handles WebSocket connections and their lifecycles
func handleWebSocketConnections(c *websocket.Conn) { // Replace HostType with the actual type of your host
	defer removeListener(c) // Clean up when connection closes

	for {
		if err := processWebSocketMessage(c); err != nil { // Pass the host to the message processor
			log.Printf("Error processing WebSocket message: %v\n", err)
			break
		}
	}
}

func processWebSocketMessage(c *websocket.Conn) error {
	_, message, err := c.ReadMessage()
	if err != nil {
		return fmt.Errorf("read error: %w", err)
	}

	log.Println("Logging subscriptions at entry point...")
	logCurrentSubscriptions()

	rawMessage := nostr.ParseMessage(message)
	log.Println("Received type:", rawMessage.Label())
	log.Println("Received message:", string(rawMessage.String()))

	// Your switch case for handling different types of messages
	// Ensure you handle context creation and cancellation correctly
	switch env := rawMessage.(type) {
	case *nostr.EventEnvelope:
		log.Println("Received EVENT message:", env.Kind)

		handler := lib_nostr.GetHandler(fmt.Sprintf("kind/%d", env.Kind))

		if handler != nil {
			notifyListeners(&env.Event)

			read := func() ([]byte, error) {
				bytes, err := json.Marshal(env)
				if err != nil {
					return nil, err
				}

				return bytes, nil
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
	case *nostr.ReqEnvelope:
		handler := lib_nostr.GetHandler("filter")

		if handler != nil {
			_, cancelFunc := context.WithCancel(context.Background())

			setListener(env.SubscriptionID, c, env.Filters, cancelFunc)
			logCurrentSubscriptions()

			read := func() ([]byte, error) {
				bytes, err := json.Marshal(env)
				if err != nil {
					return nil, err
				}

				return bytes, nil
			}

			write := func(messageType string, params ...interface{}) {
				response := lib_nostr.BuildResponse(messageType, params)

				if len(response) > 0 {
					handleIncomingMessage(c, response)
				}
			}

			handler(read, write)
		}
	case *nostr.CloseEnvelope:
		var closeEvent []string
		err := json.Unmarshal([]byte(env.String()), &closeEvent)
		if err != nil {
			fmt.Println("Error:", err)
			// Send a NOTICE message in case of unmarshalling error
			errMsg := "Error unmarshalling CLOSE request: " + err.Error()
			if writeErr := sendWebSocketMessage(c, nostr.NoticeEnvelope(errMsg)); writeErr != nil {
				fmt.Println("Error sending NOTICE message:", writeErr)
			}
			return err
		}
		subscriptionID := closeEvent[1]
		log.Println("Received CLOSE message:", subscriptionID)

		// Assume removeListenerId will be called
		responseMsg := nostr.ClosedEnvelope{SubscriptionID: subscriptionID, Reason: "Subscription closed successfully."}
		// Attempt to remove the listener for the given subscription ID
		if removeListenerId(c, subscriptionID) {
			// Log current subscriptions for debugging
			logCurrentSubscriptions()
		}

		log.Println("Response message:", responseMsg)
		// Send the prepared CLOSED or error message
		if err := sendWebSocketMessage(c, responseMsg); err != nil {
			log.Printf("Error sending 'CLOSED' envelope over WebSocket: %v", err)
		}

	case *nostr.CountEnvelope:
		handler := lib_nostr.GetHandler("count")

		if handler != nil {
			_, cancelFunc := context.WithCancel(context.Background())

			setListener(env.SubscriptionID, c, env.Filters, cancelFunc)
			logCurrentSubscriptions()

			read := func() ([]byte, error) {
				bytes, err := json.Marshal(env)
				if err != nil {
					return nil, err
				}

				return bytes, nil
			}

			write := func(messageType string, params ...interface{}) {
				response := lib_nostr.BuildResponse(messageType, params)

				if len(response) > 0 {
					handleIncomingMessage(c, response)
				}
			}

			handler(read, write)
		}

	default:
		log.Println("Unknown message type:")
	}

	return nil
}

// LogCurrentSubscriptions logs current subscriptions for debugging purposes.
func logCurrentSubscriptions() {
	empty := true // Assume initially that there are no subscriptions
	listeners.Range(func(ws *websocket.Conn, subs *xsync.MapOf[string, *Listener]) bool {
		subs.Range(func(id string, listener *Listener) bool {
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
