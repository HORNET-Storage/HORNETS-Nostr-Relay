package kind411creator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
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
	Name              string                   `json:"name"`
	Description       string                   `json:"description,omitempty"`
	Pubkey            string                   `json:"pubkey"`
	Contact           string                   `json:"contact"`
	SupportedNIPs     []int                    `json:"supported_nips"`
	Software          string                   `json:"software"`
	Version           string                   `json:"version"`
	DHTkey            string                   `json:"dhtkey,omitempty"`
	SubscriptionTiers []map[string]interface{} `json:"subscription_tiers,omitempty"` // New field
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

	// Retrieve subscription tiers from Viper
	var subscriptionTiers []map[string]interface{}
	rawTiers := viper.Get("subscription_tiers")
	if rawTiers != nil {
		if tiers, ok := rawTiers.([]interface{}); ok {
			for _, tier := range tiers {
				if tierMap, ok := tier.(map[string]interface{}); ok {
					subscriptionTiers = append(subscriptionTiers, tierMap)
				} else {
					log.Printf("error asserting tier to map[string]interface{}: %v", tier)
				}
			}
		} else {
			log.Printf("error asserting subscription_tiers to []interface{}: %v", rawTiers)
		}
	}

	// Get relay info
	relayInfo := RelayInfo{
		Name:              viper.GetString("RelayName"),
		Description:       viper.GetString("RelayDescription"),
		Pubkey:            viper.GetString("RelayPubkey"),
		Contact:           viper.GetString("RelayContact"),
		SupportedNIPs:     []int{1, 11, 2, 9, 18, 23, 24, 25, 51, 56, 57, 42, 45, 50, 65, 116},
		Software:          viper.GetString("RelaySoftware"),
		Version:           viper.GetString("RelayVersion"),
		DHTkey:            viper.GetString("RelayDHTkey"),
		SubscriptionTiers: subscriptionTiers,
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
