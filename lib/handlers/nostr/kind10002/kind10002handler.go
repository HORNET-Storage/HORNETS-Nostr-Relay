package kind10002

import (
	"fmt"
	"log"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"
)

// BuildKind10002Handler constructs and returns a handler function for kind 10002 (Relay List Metadata) events.
func BuildKind10002Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

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

		// Check relay settings for allowed events whilst also verifying signatures and kind number
		success := lib_nostr.ValidateEvent(write, env, 10002)
		if !success {
			return
		}

		// Validate the event's tags
		if err := validateRelayListTags(env.Event.Tags); err != nil {
			write("NOTICE", err.Error())
			return
		}

		// Retrieve existing kind 10002 events for the pubkey to determine if this is an update
		filter := nostr.Filter{
			Authors: []string{env.Event.PubKey},
			Kinds:   []int{10002},
		}
		existingEvents, err := store.QueryEvents(filter)
		if err != nil {
			log.Printf("Error querying existing kind 10002 events: %v", err)
			write("NOTICE", fmt.Sprintf("Error querying existing events: %v", err))
			return
		}

		// Delete existing kind 10002 events if any
		for _, oldEvent := range existingEvents {
			if err := store.DeleteEvent(oldEvent.ID); err != nil {
				log.Printf("Error deleting old kind 10002 event %s: %v", oldEvent.ID, err)
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

// validateRelayListTags checks if the tags array contains valid 'r' tags for relay list metadata.
func validateRelayListTags(tags nostr.Tags) error {
	if len(tags) == 0 {
		return fmt.Errorf("relay list event must contain at least one 'r' tag")
	}

	for _, tag := range tags {
		if len(tag) < 2 || tag[0] != "r" {
			return fmt.Errorf("invalid tag format in relay list event")
		}

		// Validate relay URI
		if !isValidRelayURI(tag[1]) {
			return fmt.Errorf("invalid relay URI: %s", tag[1])
		}

		// Check for valid read/write marker if present
		if len(tag) > 2 {
			if tag[2] != "read" && tag[2] != "write" {
				return fmt.Errorf("invalid read/write marker: %s", tag[2])
			}
		}
	}

	return nil
}

// isValidRelayURI checks if the given URI is a valid relay URI.
func isValidRelayURI(uri string) bool {

	// For now, we'll just check if it starts with "wss://"
	return len(uri) > 6 && uri[:6] == "wss://"
}
