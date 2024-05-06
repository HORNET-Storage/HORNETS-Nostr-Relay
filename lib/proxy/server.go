package proxy

import (
	"context"
	"fmt"
	"log"

	jsoniter "github.com/json-iterator/go"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/puzpuzpuz/xsync/v3"
	"github.com/spf13/viper"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// func initConfig() {
// 	configPath := "."            // Define the path where the config file should be
// 	configName := "relay_config" // The name of the config file
// 	configType := "json"         // The file type of the config

// 	viper.SetConfigType(configType)
// 	viper.SetConfigName(configName)
// 	viper.AddConfigPath(configPath)

// 	if err := viper.ReadInConfig(); err != nil {
// 		// If the file does not exist, create a new one with some default settings
// 		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
// 			log.Println("No config file found. Creating a new one...")
// 			os.Chdir(configPath) // Change directory if necessary
// 			file, err := os.Create(configName + "." + configType)
// 			if err != nil {
// 				log.Fatalf("Failed to create config file, %s", err)
// 			}
// 			file.Close()

// 			// Set some default settings if necessary
// 			viper.Set("relay_settings", RelaySettings{})

// 			// Write the new configuration file
// 			if err := viper.WriteConfigAs(configName + "." + configType); err != nil {
// 				log.Fatalf("Error writing initial config file, %s", err)
// 			}
// 		} else {
// 			log.Fatalf("Error reading config file, %s", err)
// 		}
// 	}
// }

func StartServer() error {
	app := fiber.New()

	// initConfig()

	// Middleware for handling relay information requests
	app.Use(handleRelayInfoRequests)
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	// Setup WebSocket route at the root
	app.Get("/", websocket.New(func(c *websocket.Conn) {
		handleWebSocketConnections(c) // Pass the host to the connection handler
	}))

	// Wrap handlePanel with the store
	app.All("/panel", handlePanel)

	return app.Listen(":9900")
}

func handlePanel(c *fiber.Ctx) error {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	switch c.Method() {
	case "GET":
		return c.SendString("GET request on /panel")

	case "POST":
		var data map[string]interface{} // Use interface{} for initial unmarshaling
		if err := c.BodyParser(&data); err != nil {
			return c.Status(400).SendString(err.Error())
		}

		log.Println("Received JSON data:", data)

		if _, ok := data["relaycount"]; ok {
			handler := lib_nostr.GetHandler("relaycount")
			if handler != nil {
				read := func() ([]byte, error) {
					bytes, err := json.Marshal(data) // Directly marshal the relaycount data
					if err != nil {
						return nil, err
					}
					return bytes, nil
				}

				write := func(messageType string, params ...interface{}) {
					response := lib_nostr.BuildResponse(messageType, params)
					if len(response) > 0 {
						log.Println("Sending response:", string(response))

						var responseArray []string
						err := json.Unmarshal([]byte(response), &responseArray)
						if err != nil {
							log.Println("Error unmarshaling response into array:", err)
							return
						}

						if len(responseArray) == 0 {
							c.Status(fiber.StatusInternalServerError).SendString("Empty response array")
							return
						}

						var responseData map[string]int
						err = json.Unmarshal([]byte(responseArray[0]), &responseData)
						if err != nil {
							log.Println("Error unmarshaling response JSON:", err)
							c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
							return
						}

						c.JSON(responseData)
					}
				}

				handler(read, write)
			} else {
				log.Println("Handler for 'relaycount' not found")
				return c.Status(400).SendString("Handler not available for 'relaycount'")
			}
		} else if relaySettingsData, ok := data["relay_settings"]; ok {
			var relaySettings RelaySettings
			relaySettingsJSON, err := json.Marshal(relaySettingsData) // Marshal the interface{} data back to JSON
			if err != nil {
				log.Println("Error marshaling relay settings:", err)
				return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
			}

			log.Println("Received relay settings JSON:", string(relaySettingsJSON))

			if err := json.Unmarshal(relaySettingsJSON, &relaySettings); err != nil {
				log.Println("Error unmarshaling relay settings:", err)
				return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
			}

			// Store in Viper
			viper.Set("relay_settings", relaySettings)

			// Save the changes to the configuration file
			if err := viper.WriteConfig(); err != nil {
				log.Printf("Error writing config: %s", err)
				return c.Status(fiber.StatusInternalServerError).SendString("Failed to update settings")
			}

			log.Println("Stored relay settings:", relaySettings)

			return c.SendStatus(fiber.StatusOK)
		} else {
			log.Println("Invalid data key received. Expected 'relaycount' or 'relay_settings'")
			return c.Status(400).SendString("Invalid data key: 'relaycount' or 'relay_settings' expected")
		}

	default:
		return c.SendStatus(405) // Method Not Allowed
	}

	return nil
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
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
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
				bytes, err := json.Marshal(env.Event)
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
