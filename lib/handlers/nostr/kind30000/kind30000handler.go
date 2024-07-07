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
		success := lib_nostr.ValidateEvent(write, env, 30000)
		if !success {
			return
		}

		if errMsg := validateFollowSetTags(env.Event.Tags); errMsg != "" {
			write("NOTICE", errMsg)
			return
		}

		// Query and delete existing kind 30000 events for the pubkey
		filter := nostr.Filter{
			Authors: []string{env.Event.PubKey},
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
