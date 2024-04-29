package kind1984

import (
	"fmt"
	"log"

	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// BuildKind1984Handler constructs and returns a handler function for kind 1984 (Report) events.
func BuildKind1984Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		log.Println("Handling report (kind 1984) events.")

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

		// Validate that the event kind is specifically for report events (kind 1984).
		if event.Kind != 1984 {
			write("NOTICE", fmt.Sprintf("Received non-report event (kind %d) on report handler, ignoring.", event.Kind))
			return
		}

		// Validate the report event's tags.
		if errMsg := validateReportEventTags(event.Tags); errMsg != "" {
			write("OK", event.ID, false, errMsg)
			return
		}

		log.Printf("Storing report event: %s", event.ID)
		// Store the new report event
		if err := store.StoreEvent(&event); err != nil {
			write("OK", event.ID, false, fmt.Sprintf("Error storing event: %v", err))
			return
		}

		write("OK", event.ID, true, "")
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
