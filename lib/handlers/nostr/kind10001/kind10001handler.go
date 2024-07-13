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
		success := lib_nostr.ValidateEvent(write, env, 10001)
		if !success {
			return
		}

		// Retrieve existing kind 10001 events for the pubkey to determine if this is an update
		filter := nostr.Filter{
			Authors: []string{env.Event.PubKey},
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
			if err := validatePinnedNotesTags(env.Event.Tags); err != nil {
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
