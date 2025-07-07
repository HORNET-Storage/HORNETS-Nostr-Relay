package kind30078

import (
	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/sync"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"
)

// BuildKind30078Handler creates a handler for DHT Relay List events (kind 30078)
// These events contain relay lists that should be stored in the DHT
func BuildKind30078Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	return func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		// Use Jsoniter for JSON operations
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		// Read data from the stream
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
		success := lib_nostr.ValidateEvent(write, env, 30078)
		if !success {
			return
		}

		// Extract relay URLs and DHT key from event tags
		var relayURLs []string
		var dhtKey string

		for _, tag := range env.Event.Tags {
			if len(tag) >= 2 {
				switch tag[0] {
				case "r":
					relayURLs = append(relayURLs, tag[1])
				case "dht":
					dhtKey = tag[1]
				}
			}
		}

		// Validate that we have both relay URLs and a DHT key
		if len(relayURLs) == 0 {
			write("NOTICE", "No relay URLs found in event tags.")
			return
		}

		if dhtKey == "" {
			write("NOTICE", "No DHT key found in event tags.")
			return
		}

		// Get the relay store
		relayStore := sync.GetRelayStore()
		if relayStore == nil {
			write("NOTICE", "Relay store not initialized.")
			return
		}

		// Store the relay list in the DHT
		err = relayStore.StoreRelayList(dhtKey, relayURLs, env.Event.PubKey, env.Event.Sig)
		if err != nil {
			logging.Infof("Error storing relay list in DHT: %v", err)
			write("NOTICE", "Failed to store relay list in DHT.")
			return
		}

		// Store the event
		if err := store.StoreEvent(&env.Event); err != nil {
			write("NOTICE", "Failed to store the event.")
			return
		}

		// Successfully processed event
		write("OK", env.Event.ID, true, "Relay list stored in DHT")
	}
}
