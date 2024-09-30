package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/joho/godotenv"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"
)

const envFile = ".env"
const nostrPrivateKeyVar = "NOSTR_PRIVATE_KEY"

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

func createKind411Event(privateKey *secp256k1.PrivateKey, publicKey *secp256k1.PublicKey, store stores.Store) error {
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

func loadSecp256k1Keys() (*btcec.PrivateKey, *btcec.PublicKey, error) {

	privateKey, publicKey, err := signing.DeserializePrivateKey(os.Getenv("NOSTR_PRIVATE_KEY"))
	if err != nil {
		return nil, nil, fmt.Errorf("error getting keys: %s", err)
	}

	return privateKey, publicKey, nil
}

func generateEd25519Keypair(privateKeyHex string) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	// Convert hex string to byte slice
	privateKeyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode hex string: %v", err)
	}

	// Ensure the private key is the correct length
	if len(privateKeyBytes) != ed25519.SeedSize {
		return nil, nil, fmt.Errorf("invalid private key length: expected %d, got %d", ed25519.SeedSize, len(privateKeyBytes))
	}

	// Clamp the private key for Ed25519 usage
	privateKeyBytes[0] &= 248  // Clear the lowest 3 bits
	privateKeyBytes[31] &= 127 // Clear the highest bit
	privateKeyBytes[31] |= 64  // Set the second highest bit

	// Generate the keypair
	privateKey := ed25519.NewKeyFromSeed(privateKeyBytes)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	viper.Set("RelayDHTkey", publicKey)
	log.Println("dht private key", hex.EncodeToString(privateKey))
	log.Println("dht public key", hex.EncodeToString(publicKey))
	viper.Set("RelayDHTkey", hex.EncodeToString(publicKey))

	return privateKey, publicKey, nil
}

func generateAndSaveNostrPrivateKey() error {
	// Check if .env file exists and if NOSTR_PRIVATE_KEY is already set
	if _, err := os.Stat(envFile); err == nil {
		err := godotenv.Load(envFile)
		if err != nil {
			return fmt.Errorf("error loading .env file: %w", err)
		}

		if os.Getenv(nostrPrivateKeyVar) != "" {
			fmt.Println("NOSTR_PRIVATE_KEY is already set in .env file")
			return nil
		}
	}

	// Generate new private key
	privateKey, err := btcec.NewPrivateKey()
	if err != nil {
		return fmt.Errorf("error generating private key: %w", err)
	}

	// Serialize and encode the private key
	serializedPrivKey := hex.EncodeToString(privateKey.Serialize())

	// Open .env file in append mode, create if not exists
	f, err := os.OpenFile(envFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("error opening .env file: %w", err)
	}
	defer f.Close()

	// Write the new key to the file
	if _, err := f.WriteString(fmt.Sprintf("\n%s=%s\n", nostrPrivateKeyVar, serializedPrivKey)); err != nil {
		return fmt.Errorf("error writing to .env file: %w", err)
	}

	fmt.Println("NOSTR_PRIVATE_KEY has been generated and saved to .env file")
	return nil
}
