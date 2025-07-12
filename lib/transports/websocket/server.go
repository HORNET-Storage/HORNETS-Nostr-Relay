package websocket

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/spf13/viper"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/blossom"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/upnp"
)

type connectionState struct {
	authenticated bool
	pubkey        string    // Store the pubkey to know who owns this connection
	blockedCheck  time.Time // When we last checked if this pubkey is blocked
}

// isConnectionBlocked checks if a connection's pubkey is blocked
func isConnectionBlocked(state *connectionState, store stores.Store) (bool, error) {
	// Skip if no pubkey (not authenticated yet)
	if state.pubkey == "" {
		return false, nil
	}

	// Only check once per minute to avoid excessive database lookups
	if time.Since(state.blockedCheck) < time.Minute {
		return false, nil
	}

	// Check if pubkey is blocked
	state.blockedCheck = time.Now()
	return store.IsBlockedPubkey(state.pubkey)
}

// terminateIfBlocked checks if a connection is blocked and terminates it if so
func terminateIfBlocked(c *websocket.Conn, state *connectionState, store stores.Store) bool {
	isBlocked, err := isConnectionBlocked(state, store)
	if err != nil {
		logging.Infof("Error checking if pubkey is blocked: %v", err)
		return false
	}

	if isBlocked {
		logging.Infof("Terminating connection from blocked pubkey: %s", state.pubkey)
		c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(4403, "Blocked pubkey"))
		c.Close()
		return true
	}
	return false
}

func BuildServer(store stores.Store) *fiber.App {
	app := fiber.New()

	// Middleware for handling relay information requests
	app.Use(handleRelayInfoRequests)

	app.Get("/", websocket.New(func(c *websocket.Conn) {
		defer removeListener(c)

		challenge := getGlobalChallenge()

		// Initialize state with empty pubkey and current time for blocked check
		state := &connectionState{
			authenticated: false,
			pubkey:        "",
			blockedCheck:  time.Now(),
		}

		// Send the AUTH challenge immediately upon connection
		authChallenge := []interface{}{"AUTH", challenge}
		jsonAuth, err := json.Marshal(authChallenge)
		if err != nil {
			logging.Infof("Error marshalling auth interface: %v", err)
		}

		handleIncomingMessage(c, jsonAuth)

		// Start a background goroutine to periodically check if the pubkey becomes blocked
		connClosed := make(chan struct{})
		defer close(connClosed)

		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					if state.pubkey != "" {
						isBlocked, err := store.IsBlockedPubkey(state.pubkey)
						if err != nil {
							logging.Infof("Error checking if pubkey is blocked: %v", err)
							continue
						}

						if isBlocked {
							logging.Infof("Terminating connection from newly blocked pubkey: %s", state.pubkey)
							c.WriteMessage(websocket.CloseMessage,
								websocket.FormatCloseMessage(4403, "Blocked pubkey"))
							c.Close()
							return
						}
					}
				case <-connClosed:
					return
				}
			}
		}()

		for {
			if err := processWebSocketMessage(c, challenge, state, store); err != nil {
				break
			}
		}
	}))

	// Enable blossom routes for unchunked file storage
	server := blossom.NewServer(store)
	server.SetupRoutes(app)

	return app
}

func StartServer(app *fiber.App) error {
	// Generate the global challenge
	_, err := generateGlobalChallenge()
	if err != nil {
		logging.Fatalf("Failed to generate global challenge: %v", err)
	}

	address := viper.GetString("server.address")
	port := config.GetPort("nostr")

	err = app.Listen(fmt.Sprintf("%s:%d", address, port))
	if err != nil {
		logging.Fatalf("error starting nostr server: %v\n", err)
	}

	if viper.GetBool("server.upnp") {
		upnp := upnp.Get()

		err := upnp.ForwardPort(uint16(port), "Hornet Storage Nostr Relay")
		if err != nil {
			logging.Error("Failed to forward port using UPnP", map[string]interface{}{
				"port": port,
			})
		}
	}

	return err
}

func handleRelayInfoRequests(c *fiber.Ctx) error {
	if c.Method() == "GET" && c.Get("Accept") == "application/nostr+json" {
		relayInfo := GetRelayInfo()
		c.Set("Access-Control-Allow-Origin", "*")
		return c.JSON(relayInfo)
	}
	return c.Next()
}

func GetRelayInfo() NIP11RelayInfo {
	// Format contact as "email | npub"
	var contact string
	email := viper.GetString("relay.contact")
	publicKeyHex := viper.GetString("relay.public_key")

	if email != "" && publicKeyHex != "" {
		// Convert hex public key to npub format
		if npub, err := nip19.EncodePublicKey(publicKeyHex); err == nil {
			contact = fmt.Sprintf("%s | %s", email, npub)
		} else {
			contact = email // fallback to just email if conversion fails
		}
	} else if email != "" {
		contact = email
	} else if publicKeyHex != "" {
		// Convert hex public key to npub format
		if npub, err := nip19.EncodePublicKey(publicKeyHex); err == nil {
			contact = npub
		}
	}

	// These values are set in main.go init() for backward compatibility
	relayInfo := NIP11RelayInfo{
		Name:          viper.GetString("relay.name"),
		Description:   viper.GetString("relay.description"),
		Pubkey:        publicKeyHex, // Keep for internal use, excluded from JSON
		Contact:       contact,
		Icon:          viper.GetString("relay.icon"),
		SupportedNIPs: viper.GetIntSlice("relay.supported_nips"),
		Software:      viper.GetString("relay.software"),
		Version:       viper.GetString("relay.version"),
	}

	privKey, _, err := signing.DeserializePrivateKey(viper.GetString("relay.private_key"))
	libp2pId := viper.GetString("LibP2PID")
	libp2pAddrs := viper.GetStringSlice("LibP2PAddrs")
	if libp2pId != "" && len(libp2pAddrs) > 0 && err == nil {
		relayInfo.HornetExtension = &HornetExtension{
			LibP2PID:    libp2pId,
			LibP2PAddrs: libp2pAddrs,
		}
		err = SignRelay(&relayInfo, privKey)
		if err != nil {
			logging.Infof("Error signing relay info: %v", err)
		}
	} else {
		logging.Infof("Not advertising hornet extension because libp2pID == %s and libp2paddrs == %s", libp2pId, libp2pAddrs)
	}

	return relayInfo
}

func SignRelay(relay *NIP11RelayInfo, privKey *btcec.PrivateKey) error {
	relayBytes := PackRelayForSig(relay)
	hash := sha256.Sum256(relayBytes)

	signature, err := schnorr.Sign(privKey, hash[:])
	if err != nil {
		return err
	}

	if relay.HornetExtension == nil {
		relay.HornetExtension = &HornetExtension{}
	}

	relay.HornetExtension.Signature = hex.EncodeToString(signature.Serialize())
	relay.HornetExtension.LastUpdated = time.Now()
	return nil
}

func PackRelayForSig(nr *NIP11RelayInfo) []byte {
	var packed []byte

	// Pack Name
	packed = append(packed, []byte(nr.Name)...)
	packed = append(packed, 0) // null terminator

	// Pack Description
	packed = append(packed, []byte(nr.Description)...)
	packed = append(packed, 0)

	// Pack PublicKey
	pubkeyBytes, err := hex.DecodeString(nr.Pubkey)
	if err != nil {
		logging.Infof("Skipping packing invalid pubkey %s", nr.Pubkey)
	} else {
		packed = append(packed, pubkeyBytes...)
	}

	// Pack Contact
	packed = append(packed, []byte(nr.Contact)...)
	packed = append(packed, 0)

	// Pack SupportedNIPs (sorted)
	sort.Ints(nr.SupportedNIPs)
	for _, nip := range nr.SupportedNIPs {
		nipBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(nipBytes, uint32(nip))
		packed = append(packed, nipBytes...)
	}

	// Pack Software
	packed = append(packed, []byte(nr.Software)...)
	packed = append(packed, 0)

	// Pack Version
	packed = append(packed, []byte(nr.Version)...)
	packed = append(packed, 0)

	if nr.HornetExtension != nil {
		// Pack ID
		packed = append(packed, []byte(nr.HornetExtension.LibP2PID)...)
		packed = append(packed, 0) // null terminator

		// Pack Addrs
		for _, addr := range nr.HornetExtension.LibP2PAddrs {
			packed = append(packed, []byte(addr)...)
			packed = append(packed, 0) // null terminator
		}
		packed = append(packed, 0) // double null terminator to indicate end of Addrs

		// Pack LastUpdated
		unixTime := nr.HornetExtension.LastUpdated.Unix()
		timeBytes := make([]byte, 8) // Use 8 bytes for int64
		binary.BigEndian.PutUint64(timeBytes, uint64(unixTime))
		packed = append(packed, timeBytes...)
	}

	return packed
}

func processWebSocketMessage(c *websocket.Conn, challenge string, state *connectionState, store stores.Store) error {
	_, message, err := c.ReadMessage()
	if err != nil {
		return fmt.Errorf("read error: %w", err)
	}

	// Special handling for AUTH messages
	var rawArray []interface{}
	if err := json.Unmarshal(message, &rawArray); err == nil {
		if len(rawArray) >= 2 {
			if msgType, ok := rawArray[0].(string); ok && msgType == "AUTH" {
				logging.Infof("Detected AUTH message")

				// If second element is a string, it's the initial challenge
				if challenge, ok := rawArray[1].(string); ok {
					logging.Infof("Initial AUTH challenge received: %s", challenge)
					return nil
				}

				// If second element is a map, it's the auth event
				if eventMap, ok := rawArray[1].(map[string]interface{}); ok {
					logging.Infof("Received AUTH event")
					eventBytes, err := json.Marshal(eventMap)
					if err != nil {
						logging.Infof("Failed to marshal event map: %v", err)
						return nil
					}

					var event nostr.Event
					if err := json.Unmarshal(eventBytes, &event); err != nil {
						logging.Infof("Failed to unmarshal event: %v", err)
						return nil
					}

					authEnv := &nostr.AuthEnvelope{Event: event}
					logging.Infof("Handling AUTH event")
					handleAuthMessage(c, authEnv, challenge, state, store)
					return nil
				}
			}
		}
	}

	// For all non-AUTH messages from authenticated users, check if blocked
	if state.authenticated && state.pubkey != "" && terminateIfBlocked(c, state, store) {
		return fmt.Errorf("connection terminated: blocked pubkey")
	}

	// Parse the message
	rawMessage := nostr.ParseMessage(message)

	// Handle different message types
	switch env := rawMessage.(type) {
	case *nostr.EventEnvelope:
		handleEventMessage(c, env, state, store)
	case *nostr.ReqEnvelope:
		handleReqMessage(c, env, state, store)
	case *nostr.AuthEnvelope:
		logging.Infof("Handling AUTH message")
		handleAuthMessage(c, env, challenge, state, store)
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
		logging.Infof("Unknown message type: %s", string(label))
	}

	return nil
}
