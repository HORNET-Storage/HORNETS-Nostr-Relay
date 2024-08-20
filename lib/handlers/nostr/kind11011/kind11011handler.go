package kind11011

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/sync"
	ws "github.com/HORNET-Storage/hornet-storage/lib/transports/websocket"
	"github.com/anacrolix/log"
	"github.com/anacrolix/torrent/bencode"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"
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

		relayURLs, err := getURLs(payload, sig, pubkey)
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
			relay := performNIP11Request(relayURL)
			if relay != nil {
				relayStore.AddRelay(relay)
				if relay.HornetExtension != nil {
					relayStore.SyncWithRelay(relay)
				}
			}
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

func performNIP11Request(url string) *ws.NIP11RelayInfo {
	httpURL := strings.Replace(url, "wss://", "https://", 1)

	// Create a new request
	req, err := http.NewRequest("GET", httpURL, nil)
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return nil
	}

	// Set the required headers for NIP-11
	req.Header.Set("Accept", "application/nostr+json")

	// Create a client with a timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Perform the request
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error performing request: %v", err)
		return nil
	}
	defer resp.Body.Close()

	// Check if the status code is 200 OK
	if resp.StatusCode != http.StatusOK {
		log.Printf("Error performing request, status: %d", resp.StatusCode)
		return nil
	}

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		return nil
	}

	// Unmarshal the JSON into NIP11RelayInfo struct
	var relayInfo ws.NIP11RelayInfo
	err = json.Unmarshal(body, &relayInfo)
	if err != nil {
		log.Printf("Error unmarshaling relay info: %v", err)
		return nil
	}

	return &relayInfo
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

	var urlStrings []string
	err := json.Unmarshal([]byte(input), &urlStrings)
	if err != nil {
		log.Println("Error parsing JSON:", err)
		return []string{}
	}

	// Create a slice to store valid URLs
	var urls []string

	// Validate each URL
	for _, urlString := range urlStrings {
		// Trim any whitespace
		urlString = strings.TrimSpace(urlString)
		urls = append(urls, urlString)
		//// Parse the URL
		//multi, err := UrlStringToMultiaddr(urlString)
		//if err == nil {
		//	multiAddrs = append(multiAddrs, multi)
		//} else {
		//	log.Printf("Warning: Invalid URL skipped: %s\n", urlString)
		//}
	}

	return urls
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
