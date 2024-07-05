package kind3

import (
	"fmt"
	"log"

	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func BuildKind3Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		log.Println("Handling contact list event.")

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

		// Validate event kind
		if event.Kind != 3 {
			log.Printf("Received non-kind-3 event on kind-3 handler, ignoring.")
			return
		}

		// Time validation can be similar to your deletion event handler
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

		// Retrieve existing contact list events for the user
		filter := nostr.Filter{
			Authors: []string{event.PubKey},
			Kinds:   []int{3},
		}
		existingEvents, err := store.QueryEvents(filter)
		if err != nil {
			log.Printf("Error querying existing contact list events: %v", err)
			write("NOTICE", fmt.Sprintf("Error querying existing contact list events: %v", err))
			return
		}

		// If there's an existing event, delete it
		if len(existingEvents) > 0 {
			for _, oldEvent := range existingEvents {
				if err := store.DeleteEvent(oldEvent.ID); err != nil {
					log.Printf("Error deleting old contact list event %s: %v", oldEvent.ID, err)
					// Decide how to handle delete failures
				}
			}
		}

		// Store the new event
		if err := store.StoreEvent(&event); err != nil {
			log.Printf("Error storing new contact list event: %v", err)
			write("OK", event.ID, false, fmt.Sprintf("Error storing event: %v", err))
			return
		}

		write("OK", event.ID, true, "Contact list updated successfully")
	}
	return handler
}
