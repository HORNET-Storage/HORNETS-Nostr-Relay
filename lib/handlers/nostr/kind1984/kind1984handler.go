package kind1984

import (
	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// BuildKind1984Handler constructs and returns a handler function for kind 1984 (Report) events.
func BuildKind1984Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
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
		success := lib_nostr.ValidateEvent(write, env, 1984)
		if !success {
			return
		}

		// Validate the report event's tags.
		if errMsg := validateReportEventTags(env.Event.Tags); errMsg != "" {
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

// validateReportEventTags checks if the tags array contains the expected structure for a report event.
func validateReportEventTags(tags nostr.Tags) string {
	hasValidReportTag := false
	for _, tag := range tags {
		if (tag[0] == "p" || tag[0] == "e") && len(tag) == 3 {
			hasValidReportTag = true
		}
	}
	if !hasValidReportTag {
		return "Report event missing valid 'p' or 'e' report tag."
	}
	return ""
}
