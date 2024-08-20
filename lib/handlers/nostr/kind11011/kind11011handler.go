package kind11011

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/sync"
	"github.com/anacrolix/log"
	"github.com/anacrolix/torrent/bencode"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"
	"net/url"
	"strings"
)

// BuildKind11011Handler constructs and returns a handler function for kind 10000 (Mute List) events.
func BuildKind11011Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		log.Printf("Handling kind 11011")
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		// Read data from the stream.
		data, err := read()
		if err != nil {
			write("NOTICE", "Error reading from stream.")
			return
		}

		// Unmarshal the received data into a Nostr event
		var env nostr.EventEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			write("NOTICE", "Error unmarshaling event.")
			return
		}

		// Check relay settings for allowed events whilst also verifying signatures and kind number
		success := lib_nostr.ValidateEvent(write, env, 11011)
		if !success {
			log.Printf("Could not validate event")
			return
		}

		payload, pubkey, sig, success := getDHTPayloadPubkeySig(&env.Event)
		if success == false {
			write("NOTICE", "Error parsing tags for event.")
			return
		}

		relayURLs, err := getURLs(payload, pubkey, sig)
		if err != nil {
			log.Printf("Error parsing relay URLs: %v", err)
			write("NOTICE", "Error parsing URLs from tags.")
			return
		}

		relayStore := sync.GetRelayStore()
		if relayStore == nil {
			log.Println("relay store has not been initialized")
			write("NOTICE", "Relay store has not be initialized")
			return
		}

		for _, relayURL := range relayURLs {
			relay := sync.NostrRelay{Addrs: []string{relayURL}}
			relayStore.AddRelay(relay)
			// TODO: sync here? need the ID, get from NIP-11 request
			//relayStore.SyncWithRelay(&relay)
		}

		// Store the event
		if err := store.StoreEvent(&env.Event); err != nil {
			log.Printf("failed to store event: %v", err)
			write("NOTICE", "Failed to store the event")
			return
		}

		// Successfully processed event
		write("OK", env.Event.ID, true, "Event stored successfully")
	}

	return handler
}

func getDHTPayloadPubkeySig(event *nostr.Event) (string, string, string, bool) {
	var payload, pubKey, signature string
	for _, tag := range event.Tags {
		if len(tag) == 2 && tag[0] == "dht_sig" {
			signature = tag[1]
		}
		if len(tag) == 2 && tag[0] == "dht_pubkey" {
			pubKey = tag[1]
		}
	}

	payload = event.Content

	log.Printf("payload: %s, pubkey: %s, signature: %s", payload, pubKey, signature)
	return payload, pubKey, signature, true
}

// payload should be something like 4:salt6:foobar3:seqi4e1:v12:Hello world!
func getURLs(payload string, signature string, pubKey string) ([]string, error) {
	//putString := string(put)
	payloadBytes, err := hex.DecodeString(payload)
	if err != nil {
		return nil, err
	}
	decoder := bencode.NewDecoder(bytes.NewReader(payloadBytes))

	var decoded map[string]interface{}

	err = decoder.Decode(&decoded)
	if err != nil {
		log.Println("Error decoding:", err)
		return nil, err
	}

	log.Printf("salt: %s\n", decoded["salt"])
	log.Printf("seq: %d\n", decoded["seq"])
	log.Printf("v: %s\n", decoded["v"])

	// Decode the hex string into a byte slice
	pubKeyBytes, err := hex.DecodeString(pubKey)
	if err != nil {
		return nil, err
	}

	// Check if the decoded byte slice has the correct length for an Ed25519 public key
	//if len(pubKeyBytes) != ed25519.PublicKeySize {
	//	str := fmt.Sprintf("Invalid public key length. Expected %d bytes, got %d", ed25519.PublicKeySize, len(pubKeyBytes))
	//	return nil, errors.New(str)
	//}

	// Create an Ed25519 public key from the byte slice
	key := ed25519.PublicKey(pubKeyBytes)

	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return nil, err
	}

	// Verify the signature
	if ed25519.Verify(key, payloadBytes, sigBytes) {
		log.Println("Signature is valid!")
	} else {
		log.Println("Signature is invalid!")
		return nil, errors.New("signature is invalid")
	}

	return parseURLs(decoded["v"].(string)), nil
}

func parseURLs(input string) []string {
	// Split the input string by commas
	urlStrings := strings.Split(input, ",")

	// Create a slice to store valid URLs
	var multiAddrs []string

	// Validate each URL
	for _, urlString := range urlStrings {
		// Trim any whitespace
		urlString = strings.TrimSpace(urlString)

		// Parse the URL
		multi, err := UrlStringToMultiaddr(urlString)
		if err == nil {
			multiAddrs = append(multiAddrs, multi)
		} else {
			log.Printf("Warning: Invalid URL skipped: %s\n", urlString)
		}
	}

	return multiAddrs
}

func UrlStringToMultiaddr(urlStr string) (string, error) {
	// Parse the URL
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %v", err)
	}

	// Start building the multiaddr string
	var maStr strings.Builder

	// Add the appropriate protocol prefix
	switch u.Scheme {
	case "https":
		maStr.WriteString("/ip4/")
	case "wss":
		maStr.WriteString("/ip4/")
	default:
		return "", fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}

	// Add the host
	host := u.Hostname()
	maStr.WriteString(host)

	// Add the port if present
	if port := u.Port(); port != "" {
		maStr.WriteString("/tcp/")
		maStr.WriteString(port)
	} else {
		maStr.WriteString("/tcp/443")
	}

	// Add the scheme-specific suffix
	switch u.Scheme {
	case "https":
		maStr.WriteString("/https")
	case "wss":
		maStr.WriteString("/wss")
	}

	// Parse the resulting string into a multiaddr
	return maStr.String(), nil
}
