package kind9735

import (
	"fmt"
	"log"
	"time"

	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// BuildKind9735Handler constructs and returns a handler function for kind 9735 (Zap Receipt) events.
func BuildKind9735Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		log.Println("Working with zap receipt event handler.")

		// Read data from the stream.
		data, err := read()
		if err != nil {
			write("NOTICE", "Error reading from stream.")
			return
		}

		// Unmarshal the received data into a Nostr event.
		var event nostr.Event
		if err := json.Unmarshal(data, &event); err != nil {
			write("NOTICE", "Error unmarshaling event.")
			return
		}

		// Check if the event is of kind 9735.
		if event.Kind != 9735 {
			log.Printf("Received non-kind-9735 event on kind-9735 handler, ignoring.")
			return
		}

		log.Printf("Processing zap receipt event: %s", event.ID)

		// Perform standard time check.
		isValid, errMsg := timeCheck(event.CreatedAt.Time().Unix())
		if !isValid {
			log.Println(errMsg)
			write("OK", event.ID, false, errMsg)
			return
		}

		// Store the event in the provided storage system.
		if err := store.StoreEvent(&event); err != nil {
			write("OK", event.ID, false, fmt.Sprintf("Error storing event: %v", err))
			return
		}

		write("OK", event.ID, true, "")
	}

	return handler
}

// timeCheck performs a basic check on the timestamp to ensure it's within an acceptable range.
func timeCheck(timestamp int64) (bool, string) {
	currentTime := time.Now().Unix()
	if currentTime-timestamp > 3600 { // More than an hour old
		return false, "Event timestamp is too old"
	}
	return true, ""
}
