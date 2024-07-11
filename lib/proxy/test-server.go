package proxy

// import (
// 	"context"
// 	"fmt"
// 	"log"
// 	"strconv"
// 	"strings"

// 	jsoniter "github.com/json-iterator/go"

// 	"github.com/gofiber/contrib/websocket"
// 	"github.com/gofiber/fiber/v2"
// 	"github.com/nbd-wtf/go-nostr"
// 	"github.com/spf13/viper"

// 	"github.com/HORNET-Storage/hornet-storage/lib/blossom"
// 	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
// 	"github.com/HORNET-Storage/hornet-storage/lib/stores"
// )

// type connectionState struct {
// 	authenticated bool
// 	pendingReq    *nostr.ReqEnvelope
// }

// func StartServer(store stores.Store) error {
// 	// Generate the global challenge
// 	_, err := generateGlobalChallenge()
// 	if err != nil {
// 		log.Fatalf("Failed to generate global challenge: %v", err)
// 	}

// 	app := fiber.New()
// 	app.Use(handleRelayInfoRequests)
// 	app.Get("/", websocket.New(handleWebSocketConnections))

// 	if viper.GetBool("blossom") {
// 		server := blossom.NewServer(store)
// 		server.SetupRoutes(app)
// 	}

// 	port := viper.GetString("port")
// 	p, err := strconv.Atoi(port)
// 	if err != nil {
// 		log.Fatalf("Error parsing port %s: %v", port, err)
// 	}

// 	for {
// 		port := fmt.Sprintf(":%d", p+1)
// 		err := app.Listen(port)
// 		if err != nil {
// 			log.Printf("Error starting web-server: %v\n", err)
// 			if strings.Contains(err.Error(), "address already in use") {
// 				p += 1
// 			} else {
// 				break
// 			}
// 		} else {
// 			break
// 		}
// 	}
// 	return err
// }

// func handleRelayInfoRequests(c *fiber.Ctx) error {
// 	if c.Method() == "GET" && c.Get("Accept") == "application/nostr+json" {
// 		relayInfo := getRelayInfo()
// 		c.Set("Access-Control-Allow-Origin", "*")
// 		return c.JSON(relayInfo)
// 	}
// 	return c.Next()
// }

// func getRelayInfo() nip11RelayInfo {
// 	return nip11RelayInfo{
// 		Name:          viper.GetString("RelayName"),
// 		Description:   viper.GetString("RelayDescription"),
// 		Pubkey:        viper.GetString("RelayPubkey"),
// 		Contact:       viper.GetString("RelayContact"),
// 		SupportedNIPs: []int{1, 11, 2, 9, 18, 23, 24, 25, 51, 56, 57, 42},
// 		Software:      viper.GetString("RelaySoftware"),
// 		Version:       viper.GetString("RelayVersion"),
// 	}
// }

// func handleWebSocketConnections(c *websocket.Conn) {
// 	defer removeListener(c)

// 	challenge := getGlobalChallenge()
// 	log.Printf("Using global challenge for connection: %s", challenge)

// 	state := &connectionState{authenticated: false}

// 	for {
// 		if err := processWebSocketMessage(c, challenge, state); err != nil {
// 			log.Printf("Error processing WebSocket message: %v\n", err)
// 			break
// 		}
// 	}
// }

// func processWebSocketMessage(c *websocket.Conn, challenge string, state *connectionState) error {
// 	// var json = jsoniter.ConfigCompatibleWithStandardLibrary
// 	_, message, err := c.ReadMessage()
// 	if err != nil {
// 		return fmt.Errorf("read error: %w", err)
// 	}

// 	rawMessage := nostr.ParseMessage(message)

// 	switch env := rawMessage.(type) {
// 	case *nostr.EventEnvelope:
// 		return handleEventMessage(c, env, state)

// 	case *nostr.ReqEnvelope:
// 		if !state.authenticated {
// 			// Store the REQ for later processing
// 			state.pendingReq = env
// 			// Send AUTH challenge
// 			response := lib_nostr.BuildResponse("AUTH", challenge)
// 			return handleIncomingMessage(c, response)
// 		}
// 		// Process REQ if authenticated
// 		return handleReqMessage(c, env)

// 	case *nostr.AuthEnvelope:
// 		if err := thandleAuthMessage(c, env, challenge, state); err != nil {
// 			return err
// 		}
// 		// If authentication successful and there's a pending REQ, process it
// 		if state.authenticated && state.pendingReq != nil {
// 			err := handleReqMessage(c, state.pendingReq)
// 			state.pendingReq = nil
// 			return err
// 		}
// 		return nil

// 	case *nostr.CloseEnvelope:
// 		return handleCloseMessage(c, env)

// 	case *nostr.CountEnvelope:
// 		return handleCountMessage(c, env, challenge)

// 	default:
// 		log.Println("Unknown message type:")
// 		return nil
// 	}
// }

// func handleEventMessage(c *websocket.Conn, env *nostr.EventEnvelope, state *connectionState) error {
// 	log.Println("Received EVENT message:", env.Kind)

// 	settings, err := lib_nostr.LoadRelaySettings()
// 	if err != nil {
// 		log.Fatalf("Failed to load relay settings: %v", err)
// 		return err
// 	}

// 	if settings.Mode == "unlimited" {
// 		return handleUnlimitedModeEvent(c, env)
// 	} else if settings.Mode == "smart" {
// 		return handleSmartModeEvent(c, env)
// 	}

// 	return nil
// }

// func handleUnlimitedModeEvent(c *websocket.Conn, env *nostr.EventEnvelope) error {
// 	var json = jsoniter.ConfigCompatibleWithStandardLibrary
// 	log.Println("Unlimited Mode processing.")
// 	handler := lib_nostr.GetHandler("universal")

// 	if handler != nil {
// 		notifyListeners(&env.Event)

// 		read := func() ([]byte, error) {
// 			return json.Marshal(env)
// 		}

// 		write := func(messageType string, params ...interface{}) {
// 			response := lib_nostr.BuildResponse(messageType, params)
// 			if len(response) > 0 {
// 				handleIncomingMessage(c, response)
// 			}
// 		}

// 		if verifyNote(&env.Event) {
// 			handler(read, write)
// 		} else {
// 			write("OK", env.ID, false, "Invalid note")
// 		}
// 	}

// 	return nil
// }

// func handleSmartModeEvent(c *websocket.Conn, env *nostr.EventEnvelope) error {
// 	var json = jsoniter.ConfigCompatibleWithStandardLibrary
// 	handler := lib_nostr.GetHandler(fmt.Sprintf("kind/%d", env.Kind))

// 	if handler != nil {
// 		notifyListeners(&env.Event)

// 		read := func() ([]byte, error) {
// 			return json.Marshal(env)
// 		}

// 		write := func(messageType string, params ...interface{}) {
// 			response := lib_nostr.BuildResponse(messageType, params)
// 			if len(response) > 0 {
// 				handleIncomingMessage(c, response)
// 			}
// 		}

// 		if verifyNote(&env.Event) {
// 			handler(read, write)
// 		} else {
// 			write("OK", env.ID, false, "Invalid note")
// 		}
// 	}

// 	return nil
// }

// func handleReqMessage(c *websocket.Conn, env *nostr.ReqEnvelope) error {
// 	var json = jsoniter.ConfigCompatibleWithStandardLibrary
// 	handler := lib_nostr.GetHandler("filter")

// 	if handler != nil {
// 		_, cancelFunc := context.WithCancel(context.Background())

// 		setListener(env.SubscriptionID, c, env.Filters, cancelFunc)

// 		read := func() ([]byte, error) {
// 			return json.Marshal(env)
// 		}

// 		write := func(messageType string, params ...interface{}) {
// 			response := lib_nostr.BuildResponse(messageType, params)
// 			if len(response) > 0 {
// 				handleIncomingMessage(c, response)
// 			}
// 		}

// 		handler(read, write)
// 	}

// 	return nil
// }

// func thandleAuthMessage(c *websocket.Conn, env *nostr.AuthEnvelope, challenge string, state *connectionState) error {
// 	log.Println("Received AUTH message")

// 	write := func(messageType string, params ...interface{}) {
// 		response := lib_nostr.BuildResponse(messageType, params)
// 		if len(response) > 0 {
// 			handleIncomingMessage(c, response)
// 		}
// 	}

// 	if env.Event.Kind != 22242 {
// 		write("OK", env.Event.ID, false, "Error auth event kind must be 22242")
// 		return nil
// 	}

// 	isValid, errMsg := lib_nostr.AuthTimeCheck(env.Event.CreatedAt.Time().Unix())
// 	if !isValid {
// 		write("OK", env.Event.ID, false, errMsg)
// 		return nil
// 	}

// 	result, err := env.Event.CheckSignature()
// 	if err != nil || !result {
// 		write("OK", env.Event.ID, false, "Error checking event signature")
// 		return nil
// 	}

// 	var hasRelayTag, hasChallengeTag bool
// 	for _, tag := range env.Event.Tags {
// 		if len(tag) >= 2 {
// 			if tag[0] == "relay" {
// 				hasRelayTag = true
// 			} else if tag[0] == "challenge" {
// 				hasChallengeTag = true
// 				if tag[1] != challenge {
// 					write("OK", env.Event.ID, false, "Error checking session challenge")
// 					return nil
// 				}
// 			}
// 		}
// 	}

// 	if !hasRelayTag || !hasChallengeTag {
// 		write("CLOSE", env.Event.ID, false, "Error event does not have required tags")
// 		return nil
// 	}

// 	err = AuthenticateConnection(c)
// 	if err != nil {
// 		write("OK", env.Event.ID, false, "Error authorizing connection")
// 		return nil
// 	}

// 	log.Println("Connection successfully authenticated.")
// 	state.authenticated = true

// 	successMsg := nostr.OKEnvelope{
// 		EventID: env.Event.ID,
// 		OK:      true,
// 		Reason:  "",
// 	}

// 	if err := sendWebSocketMessage(c, successMsg); err != nil {
// 		log.Printf("Error sending 'OK' envelope over WebSocket: %v", err)
// 	}

// 	write("OK", env.Event.ID, true, "")
// 	return nil
// }

// func handleCloseMessage(c *websocket.Conn, env *nostr.CloseEnvelope) error {
// 	var json = jsoniter.ConfigCompatibleWithStandardLibrary
// 	var closeEvent []string
// 	err := json.Unmarshal([]byte(env.String()), &closeEvent)
// 	if err != nil {
// 		fmt.Println("Error:", err)
// 		errMsg := "Error unmarshalling CLOSE request: " + err.Error()
// 		if writeErr := sendWebSocketMessage(c, nostr.NoticeEnvelope(errMsg)); writeErr != nil {
// 			fmt.Println("Error sending NOTICE message:", writeErr)
// 		}
// 		return err
// 	}
// 	subscriptionID := closeEvent[1]
// 	log.Println("Received CLOSE message:", subscriptionID)

// 	removeListenerId(c, subscriptionID)

// 	responseMsg := nostr.ClosedEnvelope{SubscriptionID: subscriptionID, Reason: "Subscription closed successfully."}
// 	log.Println("Response message:", responseMsg)

// 	if err := sendWebSocketMessage(c, responseMsg); err != nil {
// 		log.Printf("Error sending 'CLOSED' envelope over WebSocket: %v", err)
// 	}

// 	return nil
// }

// func handleCountMessage(c *websocket.Conn, env *nostr.CountEnvelope, challenge string) error {
// 	var json = jsoniter.ConfigCompatibleWithStandardLibrary
// 	handler := lib_nostr.GetHandler("count")

// 	if handler != nil {
// 		_, cancelFunc := context.WithCancel(context.Background())

// 		setListener(env.SubscriptionID, c, env.Filters, cancelFunc)

// 		response := lib_nostr.BuildResponse("AUTH", challenge)
// 		if len(response) > 0 {
// 			handleIncomingMessage(c, response)
// 		}

// 		read := func() ([]byte, error) {
// 			return json.Marshal(env)
// 		}

// 		write := func(messageType string, params ...interface{}) {
// 			response := lib_nostr.BuildResponse(messageType, params)
// 			if len(response) > 0 {
// 				handleIncomingMessage(c, response)
// 			}
// 		}

// 		handler(read, write)
// 	}

// 	return nil
// }
