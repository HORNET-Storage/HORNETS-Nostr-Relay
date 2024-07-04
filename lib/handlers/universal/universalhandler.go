package universalhandler

import (
	"fmt"
	"log"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func BuildUniversalHandler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		log.Println("Working with default event handler.")
		settings, err := lib_nostr.LoadRelaySettings()
		if err != nil {
			log.Fatalf("Failed to load relay settings: %v", err)
			return
		}

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

		event := env.Event

		blocked := lib_nostr.IsKindBlocked(event.Kind, settings)

		// Check if the event kind is allowed
		if blocked {
			log.Printf("Kind %d not handled by this relay", event.Kind)
			write("NOTICE", "This kind is not handled by the relay.")
			return
		}

		log.Printf("Default handling for event of kind %d: %s", event.Kind, event.Content)

		isValid, errMsg := lib_nostr.TimeCheck(event.CreatedAt.Time().Unix())
		if !isValid {
			// If the timestamp is invalid, respond with an error message and return early
			log.Println(errMsg)
			write("OK", event.ID, false, errMsg)
			return
		}

		success, err := event.CheckSignature()
		if err != nil {
			write("OK", event.ID, false, "Failed to check signature")
			return
		}

		if !success {
			write("OK", event.ID, false, "Signature failed to verify")
			return
		}

		// Store the event
		if err := store.StoreEvent(&event); err != nil {
			write("OK", event.ID, false, fmt.Sprintf("Error storing event: %s", err))
			return
		} else {
			// Send a success response
			write("OK", event.ID, true, "")
		}
	}

	return handler
}
