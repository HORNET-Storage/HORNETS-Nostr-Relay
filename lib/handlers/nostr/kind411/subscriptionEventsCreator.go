package kind411creator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	stores_graviton "github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/deroproject/graviton"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"
)

// Address status constants
const (
	AddressStatusAvailable = "available"
	AddressStatusAllocated = "allocated"
	AddressStatusUsed      = "used"
	envFile                = ".env"
	nostrPrivateKeyVar     = "NOSTR_PRIVATE_KEY"
)

type RelayInfo struct {
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Pubkey        string `json:"pubkey"`
	Contact       string `json:"contact"`
	SupportedNIPs []int  `json:"supported_nips"`
	Software      string `json:"software"`
	Version       string `json:"version"`
	DHTkey        string `json:"dhtkey,omitempty"`
}

func CreateKind411Event(privateKey *secp256k1.PrivateKey, publicKey *secp256k1.PublicKey, store stores.Store) error {
	// Retrieve existing kind 411 events
	filter := nostr.Filter{
		Kinds: []int{411},
	}
	existingEvents, err := store.QueryEvents(filter)
	if err != nil {
		return fmt.Errorf("error querying existing kind 411 events: %v", err)
	}

	// Delete existing kind 411 events if any
	if len(existingEvents) > 0 {
		for _, oldEvent := range existingEvents {
			if err := store.DeleteEvent(oldEvent.ID); err != nil {
				return fmt.Errorf("error deleting old kind 411 event %s: %v", oldEvent.ID, err)
			}
			log.Printf("Deleted existing kind 411 event with ID: %s", oldEvent.ID)
		}
	}

	// Get relay info
	relayInfo := RelayInfo{
		Name:          viper.GetString("RelayName"),
		Description:   viper.GetString("RelayDescription"),
		Pubkey:        viper.GetString("RelayPubkey"),
		Contact:       viper.GetString("RelayContact"),
		SupportedNIPs: []int{1, 11, 2, 9, 18, 23, 24, 25, 51, 56, 57, 42, 45, 50, 65, 116},
		Software:      viper.GetString("RelaySoftware"),
		Version:       viper.GetString("RelayVersion"),
		DHTkey:        viper.GetString("RelayDHTkey"),
	}

	// Convert relay info to JSON
	content, err := json.Marshal(relayInfo)
	if err != nil {
		return fmt.Errorf("error marshaling relay info: %v", err)
	}

	// Create the event
	event, err := createAnyEvent(privateKey, publicKey, 411, string(content), nil)
	if err != nil {
		return fmt.Errorf("error creating kind 411 event: %v", err)
	}

	// Store the new event
	if err := store.StoreEvent(event); err != nil {
		return fmt.Errorf("error storing kind 411 event: %v", err)
	}

	// Print the event for verification
	eventJSON, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		log.Printf("Error marshaling event for printing: %v", err)
	} else {
		log.Printf("Created and stored kind 411 event:\n%s", string(eventJSON))
	}

	log.Println("Kind 411 event created and stored successfully")
	return nil
}

func createAnyEvent(privateKey *secp256k1.PrivateKey, publicKey *secp256k1.PublicKey, kind int, content string, tags []nostr.Tag) (*nostr.Event, error) {
	stringKey := hex.EncodeToString(schnorr.SerializePubKey(publicKey))
	log.Println("Public Key: ", stringKey)

	event := &nostr.Event{
		PubKey:    stringKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      kind,
		Tags:      tags,
		Content:   content,
	}

	serializedEvent := serializeEventForID(event)
	hash := sha256.Sum256([]byte(serializedEvent))
	eventID := hex.EncodeToString(hash[:])
	event.ID = eventID

	signature, err := schnorr.Sign(privateKey, hash[:])
	if err != nil {
		return nil, err
	}

	sigHex := hex.EncodeToString(signature.Serialize())
	event.Sig = sigHex

	err = signing.VerifySignature(signature, hash[:], publicKey)
	if err != nil {
		log.Printf("error verifying signature, %s", err)
		return nil, fmt.Errorf("error verifying signature, %s", err)
	} else {
		log.Println("Signature is valid.")
	}

	return event, nil
}

func serializeEventForID(event *nostr.Event) string {
	// Assuming the Tags and other fields are already correctly filled except ID and Sig
	serialized, err := json.Marshal([]interface{}{
		0,
		event.PubKey,
		event.CreatedAt,
		event.Kind,
		event.Tags,
		event.Content,
	})
	if err != nil {
		panic(err) // Handle error properly in real code
	}

	// Convert the JSON array to a string
	compactSerialized := string(serialized)

	return compactSerialized
}

func allocateAddress(store *stores_graviton.GravitonStore) (*types.Address, error) {
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
		var addr types.Address
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
			if err := addressTree.Put([]byte(addr.Index), value); err != nil {
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
		{"p", userPubKey},
		{"relay-bitcoin-address", addr.Address},
		// Add Lightning invoice if applicable
		{"relay-dht-key", viper.GetString("RelayDHTkey")},
		{"subscription_status", "inactive"},
	}

	for _, tier := range subscriptionTiers {
		tags = append(tags, nostr.Tag{"subscription-tier", tier.DataLimit, tier.Price})
	}

	event := &nostr.Event{
		PubKey:    hex.EncodeToString(relayPrivKey.PubKey().SerializeCompressed()),
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      764,
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
