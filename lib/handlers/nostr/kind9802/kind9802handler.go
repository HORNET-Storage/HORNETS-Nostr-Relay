package kind9802

import (
	"fmt"
	"log"

	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func BuildKind9802Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		// Load and check relay settings
		settings, err := lib_nostr.LoadRelaySettings()
		if err != nil {
			log.Fatalf("Failed to load relay settings: %v", err)
			return
		}

		data, err := read()
		if err != nil {
			log.Printf("Error reading from stream: %v", err)
			return
		}

		// Unmarshal the received data into a Nostr event
		var env nostr.EventEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			write("NOTICE", "Error unmarshaling event.")
			return
		}

		event := env.Event

		blocked := lib_nostr.IsTheKindAllowed(event.Kind, settings)

		// Check if the event kind is allowed
		if !blocked {
			log.Printf("Kind %d not handled by this relay", event.Kind)
			write("NOTICE", "This kind is not handled by the relay.")
			return
		}

		if event.Kind != 9802 {
			log.Printf("Received event of kind %d in highlight handler, ignoring.", event.Kind)
			return
		}

		// Check time validity
		isValid, errMsg := lib_nostr.TimeCheck(event.CreatedAt.Time().Unix())
		if !isValid {
			log.Println(errMsg)
			write("OK", event.ID, false, errMsg)
			return
		}

		success, err := event.CheckSignature()
		if err != nil {
			write("OK", event.ID, false, "Failed to check signature")
			return
		}

		if !success {
			write("OK", event.ID, false, "Signature failed to verify")
			return
		}

		// Check if at least one of the expected tags ('a', 'e', 'r', 'p', 'context') is present
		if !hasExpectedTag(event.Tags, "a", "e", "r", "p", "context") {
			log.Println("No expected tags found in the event.")
			return
		}

		// Store the highlight event
		if err := store.StoreEvent(&event); err != nil {
			errMsg := fmt.Sprintf("Error storing highlight event %s: %v", event.ID, err)
			log.Println(errMsg)
			write("OK", event.ID, false, errMsg)
		} else {
			log.Printf("Successfully stored highlight event %s.", event.ID)
			write("OK", event.ID, true, "Highlight event stored successfully.")
		}
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
