package kind30079

import (
	"fmt"

	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// BuildKind30079Handler constructs and returns a handler function for kind 30079 (Event Paths) events.
func BuildKind30079Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		logging.Info("Handling event path event.")

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

		success := lib_nostr.ValidateEvent(write, env, 30079)
		if !success {
			return
		}

		event := env.Event

		// Validate the presence of 'd' and 'f' tags
		if err := validateEventPathTags(event.Tags); err != nil {
			write("NOTICE", err.Error())
			return
		}

		// Query for existing events of the same kind and pubkey
		filter := nostr.Filter{
			Authors: []string{event.PubKey},
			Kinds:   []int{30079},
		}
		existingEvents, err := store.QueryEvents(filter)
		if err != nil {
			logging.Infof("Error querying existing kind 30079 events: %v", err)
			write("NOTICE", fmt.Sprintf("Error querying existing events: %v", err))
			return
		}

		// Delete existing events of the same kind
		for _, oldEvent := range existingEvents {
			if err := store.DeleteEvent(oldEvent.ID); err != nil {
				logging.Infof("Error deleting old kind 30079 event %s: %v", oldEvent.ID, err)
			}
		}

		// Store the new event
		if err := store.StoreEvent(&event); err != nil {
			write("OK", event.ID, false, fmt.Sprintf("Error storing event: %v", err))
			return
		}

		write("OK", event.ID, true, "Event path updated successfully")
	}

	return handler
}

// validateEventPathTags checks if the tags array contains the expected 'd' and 'f' tags for an event path.
func validateEventPathTags(tags nostr.Tags) error {
	var hasD, hasF bool
	for _, tag := range tags {
		switch tag[0] {
		case "d":
			hasD = true
		case "f":
			hasF = true
		}
	}
	if !hasD || !hasF {
		return fmt.Errorf("event must contain 'd' (event path) and 'f' (directory path) tags")
	}
	return nil
}
