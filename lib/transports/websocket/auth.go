package websocket

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/gofiber/contrib/websocket"
	"github.com/joho/godotenv"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/sessions"
	"github.com/HORNET-Storage/hornet-storage/lib/signing"
	stores_graviton "github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	"github.com/deroproject/graviton"
)

var session_state bool

const envFile = ".env"

func handleAuthMessage(c *websocket.Conn, env *nostr.AuthEnvelope, challenge string, state *connectionState) {
	write := func(messageType string, params ...interface{}) {
		response := lib_nostr.BuildResponse(messageType, params)
		if len(response) > 0 {
			handleIncomingMessage(c, response)
		}
	}

	if env.Event.Kind != 22242 {
		write("OK", env.Event.ID, false, "Error auth event kind must be 22242")
		return
	}

	isValid, errMsg := lib_nostr.AuthTimeCheck(env.Event.CreatedAt.Time().Unix())
	if !isValid {
		write("OK", env.Event.ID, false, errMsg)
		return
	}

	success, err := env.Event.CheckSignature()
	if err != nil {
		write("NOTICE", "Failed to check signature")
		return
	}

	if !success {
		write("OK", env.Event.ID, false, "Signature failed to verify")
		return
	}

	var hasRelayTag, hasChallengeTag bool
	for _, tag := range env.Event.Tags {
		if len(tag) >= 2 {
			if tag[0] == "relay" {
				hasRelayTag = true
			} else if tag[0] == "challenge" {
				hasChallengeTag = true
				if tag[1] != challenge {
					write("OK", env.Event.ID, false, "Error checking session challenge")
					return
				}
			}
		}
	}

	if !hasRelayTag || !hasChallengeTag {
		write("OK", env.Event.ID, false, "Error event does not have required tags")
		return
	}

	err = sessions.CreateSession(env.Event.PubKey)
	if err != nil {
		write("NOTICE", "Failed to create session")
		return
	}

	err = AuthenticateConnection(c)
	if err != nil {
		write("OK", env.Event.ID, false, "Error authorizing connection")
		return
	}

	state.authenticated = false // This is where we would update the access for the subscribers.

	session_state = true

	if session_state {
		// Authenticating user session.
		userSession := sessions.GetSession(env.Event.PubKey)
		userSession.Signature = &env.Event.Sig
		userSession.Authenticated = true
	}

	err = godotenv.Load(envFile)
	if err != nil {
		log.Printf("error loading .env file: %s", err)
		return
	}
	// load keys from environment for signing kind 411
	privKey, _, err := loadSecp256k1Keys()
	if err != nil {
		log.Printf("error loading keys from environment. check if you have the key in the environment: %s", err)
		return
	}

	nip88Event, err := CreateNIP88Event(privKey, env.Event.PubKey, &stores_graviton.GravitonStore{})
	if err != nil {
		log.Printf("Failed to create NIP-88 event: %v", err)
		return
	}

	// Marshal the NIP-88 event to JSON
	nip88EventJSON, err := json.Marshal(nip88Event)
	if err != nil {
		log.Fatalf("Failed to marshal NIP-88 event to JSON: %v", err)
	}

	// Convert the JSON byte slice to a string
	nip88EventString := string(nip88EventJSON)

	// Use the JSON string in the write function
	write("OK", env.Event.ID, true, nip88EventString)
}

type Address struct {
	Index       uint
	Address     string
	Status      string
	AllocatedAt *time.Time
}

const (
	AddressStatusAvailable = "available"
	AddressStatusAllocated = "allocated"
	AddressStatusUsed      = "used"
)

func allocateAddress(store *stores_graviton.GravitonStore) (*Address, error) {
	ss, err := store.Database.LoadSnapshot(0)
	if err != nil {
		return nil, fmt.Errorf("failed to load snapshot: %v", err)
	}

	addressTree, err := ss.GetTree("relay_addresses")
	if err != nil {
		return nil, fmt.Errorf("failed to get address tree: %v", err)
	}

	cursor := addressTree.Cursor()
	for _, v, err := cursor.First(); err == nil; _, v, err = cursor.Next() {
		var addr Address
		if err := json.Unmarshal(v, &addr); err != nil {
			return nil, err
		}
		if addr.Status == AddressStatusAvailable {
			now := time.Now()
			addr.Status = AddressStatusAllocated
			addr.AllocatedAt = &now

			value, err := json.Marshal(addr)
			if err != nil {
				return nil, err
			}
			if err := addressTree.Put([]byte(fmt.Sprintf("%d", addr.Index)), value); err != nil {
				return nil, err
			}
			if _, err := graviton.Commit(addressTree); err != nil {
				return nil, err
			}
			return &addr, nil
		}
	}

	return nil, fmt.Errorf("no available addresses")
}

func CreateNIP88Event(relayPrivKey *btcec.PrivateKey, userPubKey string, store *stores_graviton.GravitonStore) (*nostr.Event, error) {
	subscriptionTiers := []types.SubscriptionTier{
		{DataLimit: "1 GB per month", Price: "10,000 sats"},
		{DataLimit: "5 GB per month", Price: "40,000 sats"},
		{DataLimit: "10 GB per month", Price: "70,000 sats"},
	}

	// Allocate a new address for this subscription
	addr, err := allocateAddress(store)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate address: %v", err)
	}

	tags := []nostr.Tag{
		{"subscription-duration", "1 month"},
		{"npub", userPubKey},
		{"relay-bitcoin-address", addr.Address},
		// Add Lightning invoice if applicable
		{"relay-dht-key", viper.GetString("RelayDHTkey")},
	}

	for _, tier := range subscriptionTiers {
		tags = append(tags, nostr.Tag{"subscription-tier", tier.DataLimit, tier.Price})
	}

	event := &nostr.Event{
		PubKey:    hex.EncodeToString(relayPrivKey.PubKey().SerializeCompressed()),
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      88,
		Tags:      tags,
		Content:   "",
	}

	hash := sha256.Sum256(event.Serialize())
	sig, err := schnorr.Sign(relayPrivKey, hash[:])
	if err != nil {
		return nil, fmt.Errorf("error signing event: %v", err)
	}
	event.ID = hex.EncodeToString(hash[:])
	event.Sig = hex.EncodeToString(sig.Serialize())

	return event, nil
}

func loadSecp256k1Keys() (*btcec.PrivateKey, *btcec.PublicKey, error) {

	privateKey, publicKey, err := signing.DeserializePrivateKey(os.Getenv("NOSTR_PRIVATE_KEY"))
	if err != nil {
		return nil, nil, fmt.Errorf("error getting keys: %s", err)
	}

	return privateKey, publicKey, nil
}
