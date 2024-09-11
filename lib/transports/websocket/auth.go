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

// var session_state bool

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

	// Initialize the Graviton store
	store := &stores_graviton.GravitonStore{}
	err = store.InitStore()
	if err != nil {
		log.Printf("Failed to initialize the Graviton store: %v", err)
		write("NOTICE", "Failed to initialize the Graviton store: %v", err)
		return
	}

	// Retrieve the subscriber using their npub
	subscriber, err := store.GetSubscriber(env.Event.PubKey)
	if err != nil {
		log.Printf("Subscriber not found or error retrieving subscriber: %v", err)
		state.authenticated = false
		// Proceed to create and send the NIP-88 event even if the subscriber is not found
	}

	// Check if the subscription is still active
	if subscriber != nil && time.Now().After(subscriber.EndDate) {
		log.Printf("Subscriber %s subscription expired on %s", subscriber.Npub, subscriber.EndDate)
		state.authenticated = false
		// Proceed to create and send the NIP-88 event even if the subscription has expired
	} else {
		// If the subscription is valid, set authenticated to true
		state.authenticated = true
		return
	}

	// Create session regardless of subscription status
	err = sessions.CreateSession(env.Event.PubKey)
	if err != nil {
		write("NOTICE", "Failed to create session")
		return
	}

	userSession := sessions.GetSession(env.Event.PubKey)
	userSession.Signature = &env.Event.Sig
	userSession.Authenticated = true

	err = godotenv.Load(envFile)
	if err != nil {
		log.Printf("error loading .env file: %s", err)
		write("NOTICE", "error loading .env file: %s", err)
		return
	}

	// Load keys from environment for signing kind 411
	privKey, _, err := loadSecp256k1Keys()
	if err != nil {
		log.Printf("error loading keys from environment. check if you have the key in the environment: %s", err)
		write("NOTICE", "error loading keys from environment. check if you have the key in the environment: %s", err)
		return
	}

	// Allocate the address to the user (npub)
	err = CreateNIP88Event(privKey, env.Event.PubKey, store)
	if err != nil {
		log.Printf("Failed to create NIP-88 event: %v", err)
		write("NOTICE", "Failed to create NIP-88 event: %v", err)
		return
	}

	// Use the JSON string in the write function
	write("OK", env.Event.ID, true, "Kind 88 event successfully")

	if !state.authenticated {
		log.Printf("Session established but subscription expired or subscriber not found for %s", env.Event.PubKey)
		write("NOTICE", "Session established but subscription expired or subscriber not found. Renew to continue access.")
	}
}

type Address struct {
	Index       uint
	Address     string
	Status      string
	AllocatedAt *time.Time
	Npub        string // Associate the address with an Npub
}

const (
	AddressStatusAvailable = "available"
	AddressStatusAllocated = "allocated"
	AddressStatusUsed      = "used"
)

// Allocate the address to a specific npub (subscriber)
func generateUniqueBitcoinAddress(store *stores_graviton.GravitonStore, npub string) (*Address, error) {
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
			addr.Npub = npub

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

func CreateNIP88Event(relayPrivKey *btcec.PrivateKey, userPubKey string, store *stores_graviton.GravitonStore) error {
	// Check if a NIP-88 event already exists for this user
	existingEvent, err := getExistingNIP88Event(store, userPubKey)
	if err != nil {
		return fmt.Errorf("error checking existing NIP-88 event: %v", err)
	}
	if existingEvent != nil {
		return nil // Event already exists, no need to create a new one
	}

	subscriptionTiers := []types.SubscriptionTier{
		{DataLimit: "1 GB per month", Price: "10000"},
		{DataLimit: "5 GB per month", Price: "40000"},
		{DataLimit: "10 GB per month", Price: "70000"},
	}

	uniqueAddress, err := generateUniqueBitcoinAddress(store, userPubKey)
	if err != nil {
		return fmt.Errorf("failed to generate unique Bitcoin address: %v", err)
	}

	tags := []nostr.Tag{
		{"subscription_duration", "1 month"},
		{"p", userPubKey},
		{"subscription_status", "inactive"},
		{"relay_bitcoin_address", uniqueAddress.Address},
		{"relay_dht_key", viper.GetString("RelayDHTkey")},
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

	// Generate the event ID
	serializedEvent := event.Serialize()
	hash := sha256.Sum256(serializedEvent)
	event.ID = hex.EncodeToString(hash[:])

	// Sign the event
	sig, err := schnorr.Sign(relayPrivKey, hash[:])
	if err != nil {
		return fmt.Errorf("error signing event: %v", err)
	}
	event.Sig = hex.EncodeToString(sig.Serialize())

	// Store the event
	err = store.StoreEvent(event)
	if err != nil {
		return fmt.Errorf("failed to store NIP-88 event: %v", err)
	}

	return nil
}

func getExistingNIP88Event(store *stores_graviton.GravitonStore, userPubKey string) (*nostr.Event, error) {
	filter := nostr.Filter{
		Kinds: []int{88},
		Tags: nostr.TagMap{
			"p": []string{userPubKey},
		},
		Limit: 1,
	}

	events, err := store.QueryEvents(filter)
	if err != nil {
		return nil, err
	}

	if len(events) > 0 {
		return events[0], nil
	}

	return nil, nil
}

func loadSecp256k1Keys() (*btcec.PrivateKey, *btcec.PublicKey, error) {

	privateKey, publicKey, err := signing.DeserializePrivateKey(os.Getenv("NOSTR_PRIVATE_KEY"))
	if err != nil {
		return nil, nil, fmt.Errorf("error getting keys: %s", err)
	}

	return privateKey, publicKey, nil
}
