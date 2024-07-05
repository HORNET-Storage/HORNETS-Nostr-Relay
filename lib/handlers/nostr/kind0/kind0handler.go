package kind0

import (
	"fmt"
	"log"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	jsoniter "github.com/json-iterator/go"

	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func BuildKind0Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		// Use Jsoniter for JSON operations
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		log.Println("Working with kind zero handler.")

		// Load and check relay settings
		settings, err := lib_nostr.LoadRelaySettings()
		if err != nil {
			log.Fatalf("Failed to load relay settings: %v", err)
			return
		}

		log.Println("Settings", settings)

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

		event := env.Event

		blocked := lib_nostr.IsTheKindAllowed(event.Kind, settings)

		// Check if the event kind is allowed
		if !blocked {
			log.Printf("Kind %d not handled by this relay", event.Kind)
			write("NOTICE", "This kind is not handled by the relay.")
			return
		}

		// Check if the event is of kind 0
		if event.Kind != 0 {
			log.Printf("Received non-kind-0 event on kind-0 handler, ignoring.")
			return
		}

		log.Printf("Processing kind 0 event: %s", event)

		// Perform time check
		isValid, errMsg := lib_nostr.TimeCheck(event.CreatedAt.Time().Unix())
		if !isValid {
			// If the timestamp is invalid, respond with an error message and return early
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

		// Retrieve existing kind 0 events for the pubkey
		filter := nostr.Filter{
			Authors: []string{event.PubKey},
			Kinds:   []int{0},
		}
		existingEvents, err := store.QueryEvents(filter)
		if err != nil {
			log.Printf("Error querying existing kind 0 events: %v", err)
			write("NOTICE", fmt.Sprintf("Error querying existing events: %v", err))
			return
		}

		// Delete existing kind 0 events if any
		if len(existingEvents) > 0 {
			for _, oldEvent := range existingEvents {
				if err := store.DeleteEvent(oldEvent.ID); err != nil {
					log.Printf("Error deleting old kind 0 event %s: %v", oldEvent.ID, err)
					write("NOTICE", "Error deleting old kind 0 event %s: %v", oldEvent.ID, err)
				}
			}
		}

		// Store the event
		if err := store.StoreEvent(&event); err != nil {
			// Example: Sending an "OK" message with an error indication
			write("OK", event.ID, false, "error storing event")
		} else {
			// Example: Successfully stored event, sending a success "OK" message
			write("OK", event.ID, true, "")
		}
	}

	return handler
}
