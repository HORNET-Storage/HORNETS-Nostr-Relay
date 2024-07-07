package kind36810

import (
	"fmt"

	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// BuildKind36810Handler constructs and returns a handler function for kind 36810 events.
func BuildKind36810Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
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
		success := lib_nostr.ValidateEvent(write, env, 36810)
		if !success {
			return
		}

		// Validate the kind 36810 event's tags.
		if errMsg := validateKind36810EventTags(env.Event.Tags); errMsg != "" {
			write("OK", env.Event.ID, false, errMsg)
			return
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

// validateKind36810EventTags checks if the tags array contains the required 'd', 't', and 'r' tags for a kind 36810 event.
func validateKind36810EventTags(tags nostr.Tags) string {
	requiredTags := map[string]bool{"d": false, "t": false, "r": false}
	for _, tag := range tags {
		if _, ok := requiredTags[tag[0]]; ok && len(tag) == 2 {
			requiredTags[tag[0]] = true
		}
	}
	for tag, found := range requiredTags {
		if !found {
			return fmt.Sprintf("Kind 36810 event missing required '%s' tag.", tag)
		}
	}
	return ""
}
