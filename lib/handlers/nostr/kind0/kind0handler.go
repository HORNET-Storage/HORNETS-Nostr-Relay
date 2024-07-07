package kind0

import (
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
		success := lib_nostr.ValidateEvent(write, env, 0)
		if !success {
			return
		}

		// Retrieve existing kind 0 events for the pubkey
		filter := nostr.Filter{
			Authors: []string{env.Event.PubKey},
			Kinds:   []int{0},
		}
		existingEvents, err := store.QueryEvents(filter)
		if err != nil {
			log.Printf("Error querying existing kind 0 events: %v", err)
			write("NOTICE", "Error querying existing events")
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

		// Store the new event
		if err := store.StoreEvent(&env.Event); err != nil {
			write("NOTICE", "Failed to store the event")
			return
		}

		// Successfully processed event
		write("OK", env.Event.ID, true, "Event stored successfully")
	}

	return handler
}
