package websocket

import (
	"bytes"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	"github.com/HORNET-Storage/hornet-storage/lib/handlers/blossom"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
)

type connectionState struct {
	authenticated bool
}

func StartServer(store stores.Store) error {
	// Generate the global challenge
	_, err := generateGlobalChallenge()
	if err != nil {
		log.Fatalf("Failed to generate global challenge: %v", err)
	}

	app := fiber.New()

	// Middleware for handling relay information requests
	app.Use(handleRelayInfoRequests)
	app.Get("/", websocket.New(handleWebSocketConnections))

	// Enable blossom routes for unchunked file storage
	server := blossom.NewServer(store)
	server.SetupRoutes(app)

	port := viper.GetString("port")
	p, err := strconv.Atoi(port)
	if err != nil {
		log.Fatalf("Error parsing port %s: %v", port, err)
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
		SupportedNIPs: []int{1, 11, 2, 9, 18, 23, 24, 25, 51, 56, 57, 42, 45, 50, 65, 116},
		Software:      viper.GetString("RelaySoftware"),
		Version:       viper.GetString("RelayVersion"),
	}
}

func handleWebSocketConnections(c *websocket.Conn) {
	defer removeListener(c)

	challenge := getGlobalChallenge()
	log.Printf("Using global challenge for connection: %s", challenge)

	state := &connectionState{authenticated: false}

	// Send the AUTH challenge immediately upon connection
	authChallenge := []interface{}{"AUTH", challenge}
	if err := sendWebSocketMessage(c, authChallenge); err != nil {
		log.Printf("Error sending AUTH challenge: %v", err)
		return
	}

	for {
		if err := processWebSocketMessage(c, challenge, state); err != nil {
			break
		}
	}
}

func processWebSocketMessage(c *websocket.Conn, challenge string, state *connectionState) error {
	_, message, err := c.ReadMessage()
	if err != nil {
		return fmt.Errorf("read error: %w", err)
	}

	rawMessage := nostr.ParseMessage(message)

	switch env := rawMessage.(type) {
	case *nostr.EventEnvelope:
		handleEventMessage(c, env)

	case *nostr.ReqEnvelope:
		handleReqMessage(c, env)

	case *nostr.AuthEnvelope:
		handleAuthMessage(c, env, challenge, state)

	case *nostr.CloseEnvelope:
		handleCloseMessage(c, env)

	case *nostr.CountEnvelope:
		handleCountMessage(c, env, challenge)

	default:
		firstComma := bytes.Index(message, []byte{','})
		if firstComma == -1 {
			return nil
		}
		label := message[0:firstComma]

		log.Println("Unknown message type: " + string(label))
	}

	return nil
}
