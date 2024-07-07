package kind9802

import (
	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func BuildKind9802Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		// Read data from stream
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
		success := lib_nostr.ValidateEvent(write, env, 9802)
		if !success {
			return
		}

		// Check if at least one of the expected tags ('a', 'e', 'r', 'p', 'context') is present
		if !hasExpectedTag(env.Event.Tags, "a", "e", "r", "p", "context") {
			write("OK", env.Event.ID, false, "No expected tags found in the event.")
			return
		}

		/// Store the new event
		if err := store.StoreEvent(&env.Event); err != nil {
			write("NOTICE", "Failed to store the event")
			return
		}

		// Successfully processed event
		write("OK", env.Event.ID, true, "Event stored successfully")
	}

	return handler
}

// hasExpectedTag checks if at least one of the specified tags is present in the tags list.
func hasExpectedTag(tags nostr.Tags, keys ...string) bool {
	keySet := make(map[string]bool)
	for _, key := range keys {
		keySet[key] = true
	}

	for _, tag := range tags {
		if _, ok := keySet[tag[0]]; ok && len(tag) > 1 {
			return true
		}
	}
	return false
}
