package filter

import (
	"log"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func BuildFilterHandler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary
		log.Println("Working with filter request.")

		data, err := read()
		if err != nil {
			log.Println("Error reading from stream:", err)
			write("NOTICE", "Error reading from stream.")
			return
		}

		var request nostr.ReqEnvelope
		if err := json.Unmarshal(data, &request); err != nil {
			log.Println("Error unmarshaling request:", err)
			write("NOTICE", "Error unmarshaling request.")
			return
		}

		// Ensure that we respond to the client after processing all filters
		// defer responder(stream, "EOSE", request.SubscriptionID, "End of stored events")
		var combinedEvents []*nostr.Event
		for _, filter := range request.Filters {
			events, err := store.QueryEvents(filter)
			if err != nil {
				log.Printf("Error querying events for filter: %v", err)
				continue
			}
			combinedEvents = append(combinedEvents, events...)
		}

		// Deduplicate events
		uniqueEvents := deduplicateEvents(combinedEvents)

		// Send each unique event to the client
		for _, event := range uniqueEvents {
			eventJSON, err := json.Marshal(event)
			if err != nil {
				log.Printf("Error marshaling event: %v", err)
				continue
			}
			write("EVENT", request.SubscriptionID, string(eventJSON))
		}

		write("EOSE", request.SubscriptionID, "End of stored events")
	}

	return handler
}

func deduplicateEvents(events []*nostr.Event) []*nostr.Event {
	seen := make(map[string]struct{})
	var uniqueEvents []*nostr.Event

	for _, event := range events {
		if _, exists := seen[event.ID]; !exists {
			seen[event.ID] = struct{}{}
			uniqueEvents = append(uniqueEvents, event)
		}
	}

	return uniqueEvents
}
