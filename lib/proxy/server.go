package proxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"

	jsoniter "github.com/json-iterator/go"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	"github.com/HORNET-Storage/hornet-storage/lib/blossom"
	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
)

const challengeLength = 32

func StartServer(store stores.Store) error {
	app := fiber.New()

	// initConfig()

	// Middleware for handling relay information requests
	app.Use(handleRelayInfoRequests)

	// Setup WebSocket route at the root
	app.Get("/", websocket.New(func(c *websocket.Conn) {
		handleWebSocketConnections(c) // Pass the host to the connection handler
	}))

	if viper.GetBool("blossom") {
		server := blossom.NewServer(store)
		server.SetupRoutes(app)
	}

	port := viper.GetString("port")
	p, err := strconv.Atoi(port)
	if err != nil {
		log.Fatal("Error parsing port #{port}: #{err}")
	}

	for {
		port := fmt.Sprintf(":%d", p+1)
		err := app.Listen(port)
		if err != nil {
			log.Printf("Error starting web-server: %v\n", err)
			if strings.Contains(err.Error(), "address already in use") {
				p += 1
			} else {
				break
			}
		} else {
			break
		}
	}
	return err
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

	rawMessage := nostr.ParseMessage(message)

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

			challenge, err := generateChallenge()
			if err != nil {
				log.Printf("Failed to generate challenge: %w", err)
			}

			setListener(env.SubscriptionID, c, env.Filters, challenge, cancelFunc)

			if challenge != nil {
				response := lib_nostr.BuildResponse("AUTH", challenge)

				if len(response) > 0 {
					handleIncomingMessage(c, response)
				}
			}

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
		removeListenerId(c, subscriptionID)

		log.Println("Response message:", responseMsg)
		// Send the prepared CLOSED or error message
		if err := sendWebSocketMessage(c, responseMsg); err != nil {
			log.Printf("Error sending 'CLOSED' envelope over WebSocket: %v", err)
		}

	case *nostr.CountEnvelope:
		handler := lib_nostr.GetHandler("count")

		if handler != nil {
			_, cancelFunc := context.WithCancel(context.Background())

			challenge, err := generateChallenge()
			if err != nil {
				log.Printf("Failed to generate challenge: %w", err)
			}

			setListener(env.SubscriptionID, c, env.Filters, challenge, cancelFunc)

			if challenge != nil {
				response := lib_nostr.BuildResponse("AUTH", challenge)

				if len(response) > 0 {
					handleIncomingMessage(c, response)
				}
			}

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
		/*
			case *nostr.AuthEnvelope:
				handler := lib_nostr.GetHandler("auth")

				if handler != nil {
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
		*/
	case *nostr.AuthEnvelope:
		write := func(messageType string, params ...interface{}) {
			response := lib_nostr.BuildResponse(messageType, params)

			if len(response) > 0 {
				handleIncomingMessage(c, response)
			}
		}

		if env.Event.Kind != 22242 {
			write("OK", env.Event.ID, false, "Error auth event kind must be 22242")
			return nil
		}

		isValid, errMsg := lib_nostr.TimeCheck(env.Event.CreatedAt.Time().Unix())
		if !isValid {
			write("OK", env.Event.ID, false, errMsg)
			return nil
		}

		result, err := env.Event.CheckSignature()
		if err != nil {
			write("OK", env.Event.ID, false, "Error checking event signature")
			return nil
		}

		if !result {
			write("OK", env.Event.ID, false, "Error signature verification failed")
			return nil
		}

		var hasRelayTag, hasChallengeTag bool
		for _, tag := range env.Event.Tags {
			if len(tag) >= 2 {
				if tag[0] == "relay" {
					hasRelayTag = true
				} else if tag[0] == "challenge" {
					hasChallengeTag = true

					challenge, err := GetListenerChallenge(c)
					if err != nil {
						write("OK", env.Event.ID, false, "Error checking session")
						return nil
					}

					tag := env.Event.Tags.GetFirst([]string{"challenge"})
					eventChallenge := tag.Value()

					if challenge == nil || *challenge != eventChallenge {
						write("OK", env.Event.ID, false, "Error checking session")
						return nil
					}
				}
			}
		}

		if !hasRelayTag || !hasChallengeTag {
			write("CLOSE", env.Event.ID, false, "Error event does not have required tags")
			return nil
		}

		err = AuthenticateConnection(c)
		if err != nil {
			write("OK", env.Event.ID, false, "Error checking session")
			return nil
		}

		write("OK", env.Event.ID, true, "")
	default:
		log.Println("Unknown message type:")
	}

	return nil
}

func generateChallenge() (*string, error) {
	bytes := make([]byte, challengeLength)

	_, err := rand.Read(bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %v", err)
	}

	challenge := hex.EncodeToString(bytes)

	return &challenge, nil
}
