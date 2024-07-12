package kind10001

import (
	"fmt"
	"log"

	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// BuildKind10001Handler constructs and returns a handler function for kind 10001 (Pinned Notes) events.
func BuildKind10001Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		log.Println("Handling pinned notes event.")

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

		if event.Kind != 10001 {
			write("NOTICE", fmt.Sprintf("Received non-pinned-notes event (kind %d) on pinned-notes handler, ignoring.", event.Kind))
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

		// Retrieve existing kind 10001 events for the pubkey to determine if this is an update
		filter := nostr.Filter{
			Authors: []string{event.PubKey},
			Kinds:   []int{10001},
		}
		existingEvents, err := store.QueryEvents(filter)
		if err != nil {
			log.Printf("Error querying existing kind 10001 events: %v", err)
			write("NOTICE", fmt.Sprintf("Error querying existing events: %v", err))
			return
		}

		// Perform tag validation only if there are no existing events (i.e., it's a new event)
		if len(existingEvents) == 0 {
			if err := validatePinnedNotesTags(event.Tags); err != nil {
				write("NOTICE", err.Error())
				return
			}
		}

		// Delete existing kind 10001 events if any
		for _, oldEvent := range existingEvents {
			if err := store.DeleteEvent(oldEvent.ID); err != nil {
				log.Printf("Error deleting old kind 10001 event %s: %v", oldEvent.ID, err)
			}
		}

		if err := store.StoreEvent(&event); err != nil {
			write("OK", event.ID, false, fmt.Sprintf("Error storing event: %v", err))
			return
		}

		write("OK", event.ID, true, "Pinned notes updated successfully")
	}

	return handler
}

// validatePinnedNotesTags checks if the tags array contains at least one of the expected tags for pinned notes.
func validatePinnedNotesTags(tags nostr.Tags) error {
	expectedTags := map[string]bool{"e": true}
	foundValidTag := false

	for _, tag := range tags {
		if expectedTags[tag[0]] {
			foundValidTag = true
			break // As soon as one valid tag is found, break out of the loop
		}
	}

	if !foundValidTag {
		log.Println("No expected tags found in pinned notes event")
		return fmt.Errorf("pinned notes event must contain at least one of the expected tags (e)")
	}

	return nil
}
