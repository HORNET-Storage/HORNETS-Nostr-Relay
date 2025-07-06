package kind11011

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/sync"
	"github.com/anacrolix/torrent/bencode"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"
)

// BuildKind11011Handler constructs and returns a handler function for kind 10000 (Mute List) events.
func BuildKind11011Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		logging.Infof("Handling kind 11011")
		var j = jsoniter.ConfigCompatibleWithStandardLibrary

		// Read data from the stream.
		data, err := read()
		if err != nil {
			write("NOTICE", "Error reading from stream.")
			return
		}

		// Unmarshal the received data into a Nostr event
		var env nostr.EventEnvelope
		if err := j.Unmarshal(data, &env); err != nil {
			write("NOTICE", "Error unmarshaling event.")
			return
		}

		// Check relay settings for allowed events whilst also verifying signatures and kind number
		success := lib_nostr.ValidateEvent(write, env, 11011)
		if !success {
			logging.Infof("Could not validate event")
			return
		}

		err = HandleRelayList(env.Event)
		if err != nil {
			write("NOTICE", "Failed to handle relay list:", err)
			// TODO: abort processing?
			//return
		}

		// Store the event
		if err := store.StoreEvent(&env.Event); err != nil {
			logging.Infof("failed to store event: %v", err)
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

	logging.Infof("received relay list from client -- payload: %s, pubkey: %s, signature: %s", payload, pubKey, signature)
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
		logging.Infof("Error decoding:%s", err)
		return nil, err
	}

	if decoded["salt"] != nil {
		logging.Infof("salt: %x\n", decoded["salt"])
	} else {
		logging.Infof("salt not provided by client")
	}
	logging.Infof("seq: %d\n", decoded["seq"])
	logging.Infof("v: %s\n", decoded["v"])

	// Decode the hex string into a byte slice
	pubKeyBytes, err := hex.DecodeString(pubKey)
	if err != nil {
		return nil, err
	}

	// Create an Ed25519 public key from the byte slice
	key := ed25519.PublicKey(pubKeyBytes)

	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return nil, err
	}

	// Verify the signature
	if ed25519.Verify(key, payloadBytes, sigBytes) {
		logging.Info("Signature is valid!")
	} else {
		logging.Info("Signature is invalid!")
		return nil, errors.New("signature is invalid")
	}

	return sync.ParseURLs([]byte(decoded["v"].(string))), nil
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

func HandleRelayList(event nostr.Event) error {
	payload, pubkey, sig, success := getDHTPayloadPubkeySig(&event)
	if !success {
		logging.Info("NOTICE Error parsing tags for event.")
		return errors.New("error parsing tags for event")
	}

	relayURLs, err := getURLs(payload, sig, pubkey)
	if err != nil {
		logging.Infof("Error parsing relay URLs: %v", err)
		return errors.New("error parsing relay URLs")
	}

	relayStore := sync.GetRelayStore()
	if relayStore == nil {
		logging.Info("relay store has not been initialized")
		return errors.New("relay store has not been initialized")
	}

	for _, relayURL := range relayURLs {
		relay := sync.PerformNIP11Request(relayURL)
		if relay != nil {
			relayStore.AddRelay(relay)
			relayStore.AddAuthor(event.PubKey)
		}
	}

	err = relayStore.AddUploadable(payload, pubkey, sig, true)
	if err != nil {
		logging.Infof("Error adding uploadable to sync store: %v", err)
		return errors.New("error adding upload to sync store")
	}

	return nil
}
