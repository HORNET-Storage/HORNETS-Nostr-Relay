package kind30000

import (
	"fmt"
	"log"

	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// BuildKind30000Handler constructs and returns a handler function for kind 30000 (Follow Sets) events.
func BuildKind30000Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary
		log.Println("Handling follow sets (kind 30000) events.")

		// Load and check relay settings
		settings, err := lib_nostr.LoadRelaySettings()
		if err != nil {
			log.Fatalf("Failed to load relay settings: %v", err)
			return
		}

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

		event := env.Event

		blocked := lib_nostr.IsTheKindAllowed(event.Kind, settings)

		// Check if the event kind is allowed
		if !blocked {
			log.Printf("Kind %d not handled by this relay", event.Kind)
			write("NOTICE", "This kind is not handled by the relay.")
			return
		}

		if event.Kind != 30000 {
			write("NOTICE", fmt.Sprintf("Received non-follow-sets event (kind %d) on follow-sets handler, ignoring.", event.Kind))
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

		if errMsg := validateFollowSetTags(event.Tags); errMsg != "" {
			write("NOTICE", errMsg)
			return
		}

		// Query and delete existing kind 30000 events for the pubkey
		filter := nostr.Filter{
			Authors: []string{event.PubKey},
			Kinds:   []int{30000},
		}
		existingEvents, err := store.QueryEvents(filter)
		if err != nil {
			log.Printf("Error querying existing kind 30000 events: %v", err)
			write("NOTICE", fmt.Sprintf("Error querying existing events: %v", err))
			return
		}

		for _, oldEvent := range existingEvents {
			if err := store.DeleteEvent(oldEvent.ID); err != nil {
				log.Printf("Error deleting old kind 30000 event %s: %v", oldEvent.ID, err)
				// Optionally respond or handle delete failures
			}
		}

		log.Printf("Storing new follow sets event: %s", event.ID)
		if err := store.StoreEvent(&event); err != nil {
			write("OK", event.ID, false, fmt.Sprintf("Error storing event: %v", err))
			return
		}

		write("OK", event.ID, true, "")
	}

	return handler
}

// validateFollowSetTags checks if the tags array contains the expected tags for a follow set, including a "d" tag.
func validateFollowSetTags(tags nostr.Tags) string {
	hasDTag := false
	for _, tag := range tags {
		if tag[0] == "d" {
			hasDTag = true
			break
		}
	}
	if !hasDTag {
		return "Follow sets event missing 'd' identifier tag."
	}
	return ""
}
