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

		log.Println("Working with kind zero handler.")

		// Read data from the stream
		data, err := read()
		if err != nil {
			write("NOTICE", "Error reading from stream.")
			return
		}

		// Unmarshal the received data into a Nostr event
		var event nostr.Event
		if err := json.Unmarshal(data, &event); err != nil {
			write("NOTICE", "Error unmarshaling event.")
			return
		}

		// Check if the event is of kind 0
		if event.Kind != 0 {
			log.Printf("Received non-kind-0 event on kind-0 handler, ignoring.")
			return
		}

		log.Printf("Processing kind 0 event: %s", event)

		// Perform time check
		isValid, errMsg := lib_nostr.TimeCheck(event.CreatedAt.Time().Unix())
		if !isValid {
			// If the timestamp is invalid, respond with an error message and return early
			log.Println(errMsg)
			write("OK", event.ID, false, errMsg)
			return
		}

		// Store the event
		if err := store.StoreEvent(&event); err != nil {
			// Example: Sending an "OK" message with an error indication
			write("OK", event.ID, false, "error storing event")
		} else {
			// Example: Successfully stored event, sending a success "OK" message
			write("OK", event.ID, true, "")
		}
	}

	return handler
}
