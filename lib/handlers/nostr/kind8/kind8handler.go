package kind8

import (
	"log"

	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// BuildKind8Handler constructs and returns a handler function for kind 8 (Badge Award) events.
func BuildKind8Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		log.Println("Handling badge award event.")

		// Load and check relay settings
		settings, err := lib_nostr.LoadRelaySettings()
		if err != nil {
			log.Fatalf("Failed to load relay settings: %v", err)
			return
		}

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

		event := env.Event

		blocked := lib_nostr.IsTheKindAllowed(event.Kind, settings)

		// Check if the event kind is allowed
		if !blocked {
			log.Printf("Kind %d not handled by this relay", event.Kind)
			write("NOTICE", "This kind is not handled by the relay.")
			return
		}

		// Validate that the event kind is for badge awards (kind 8).
		if event.Kind != 8 {
			write("NOTICE", "Unsupported event kind for badge award handler.")
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

		// Validate the badge award event's tags.
		if !isValidBadgeAwardEvent(event) {
			write("NOTICE", "Invalid badge award event.")
			return
		}

		// Store the new event.
		if err := store.StoreEvent(&event); err != nil {
			write("OK", event.ID, false, "Failed to store the event.")
			return
		}

		// Successfully processed badge award event.
		write("OK", event.ID, true, "Badge award event processed successfully.")
	}

	return handler
}

// isValidBadgeAwardEvent checks for the presence of required tags in a Badge Award event.
func isValidBadgeAwardEvent(event nostr.Event) bool {
	hasATag := false
	hasPTag := false

	for _, tag := range event.Tags {
		switch tag[0] {
		case "a":
			if len(tag) > 1 && tag[1] != "" {
				hasATag = true
			}
		case "p":
			if len(tag) > 1 && tag[1] != "" {
				hasPTag = true
			}
		}
	}

	if !hasATag {
		log.Println("Badge Award event missing 'a' tag.")
		return false
	}
	if !hasPTag {
		log.Println("Badge Award event missing 'p' tag.")
		return false
	}

	return true
}
