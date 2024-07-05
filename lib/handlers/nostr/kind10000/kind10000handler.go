package kind10000

import (
	"fmt"
	"log"

	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// BuildKind10000Handler constructs and returns a handler function for kind 10000 (Mute List) events.
func BuildKind10000Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		log.Println("Handling mute list event.")

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

		if event.Kind != 10000 {
			write("NOTICE", fmt.Sprintf("Received non-mute-list event (kind %d) on mute-list handler, ignoring.", event.Kind))
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

		// Retrieve existing kind 10000 events for the pubkey to determine if this is an update
		filter := nostr.Filter{
			Authors: []string{event.PubKey},
			Kinds:   []int{10000},
		}
		existingEvents, err := store.QueryEvents(filter)
		if err != nil {
			log.Printf("Error querying existing kind 10000 events: %v", err)
			write("NOTICE", fmt.Sprintf("Error querying existing events: %v", err))
			return
		}

		// Perform tag validation only if there are no existing events (i.e., it's a new event)
		if len(existingEvents) == 0 {
			if err := validateMuteListTags(event.Tags); err != nil {
				write("NOTICE", err.Error())
				return
			}
		}

		// Delete existing kind 10000 events if any
		for _, oldEvent := range existingEvents {
			if err := store.DeleteEvent(oldEvent.ID); err != nil {
				log.Printf("Error deleting old kind 10000 event %s: %v", oldEvent.ID, err)
			}
		}

		log.Printf("Storing new mute list event: %s", event.ID)
		if err := store.StoreEvent(&event); err != nil {
			write("OK", event.ID, false, fmt.Sprintf("Error storing event: %v", err))
			return
		}

		write("OK", event.ID, true, "Mute list updated successfully")
	}

	return handler
}

// validateMuteListTags checks if the tags array contains at least one of the expected tags for a mute list.
func validateMuteListTags(tags nostr.Tags) error {
	expectedTags := map[string]bool{"p": true, "t": true, "word": true, "e": true}
	foundValidTag := false

	for _, tag := range tags {
		if expectedTags[tag[0]] {
			foundValidTag = true
			break // As soon as one valid tag is found, break out of the loop
		}
	}

	if !foundValidTag {
		log.Println("No expected tags found in mute list event")
		return fmt.Errorf("mute list event must contain at least one of the expected tags (p, t, word, e)")
	}

	return nil
}
