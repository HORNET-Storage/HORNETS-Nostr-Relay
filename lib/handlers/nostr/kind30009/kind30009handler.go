package kind30009

import (
	"fmt"
	"log"

	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// BuildKind30009Handler constructs and returns a handler function for kind 30009 (Badge Definition) events.
func BuildKind30009Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		log.Println("Working with Badge Definition handler.")

		settings, err := lib_nostr.LoadRelaySettings()
		if err != nil {
			log.Fatalf("Failed to load relay settings: %v", err)
			return
		}

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

		blocked := lib_nostr.IsTheKindAllowed(event.Kind, settings)

		// Check if the event kind is allowed
		if !blocked {
			log.Printf("Kind %d not handled by this relay", event.Kind)
			write("NOTICE", "This kind is not handled by the relay.")
			return
		}

		// Check if the event is of kind 30009.
		if event.Kind != 30009 {
			log.Printf("Received non-kind-30009 event on Badge Definition handler, ignoring.")
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

		log.Printf("Processing Badge Definition event: %s", event.Content)

		// Perform validation of the Badge Definition event.
		isValid, errMsg := validateBadgeDefinitionEvent(event)
		if !isValid {
			log.Println(errMsg)
			write("NOTICE", event.ID, false, errMsg)
			return
		}

		// Store the event in the provided storage system.
		if err := store.StoreEvent(&event); err != nil {
			write("NOTICE", event.ID, false, fmt.Sprintf("Error storing event: %v", err))
			return
		}

		write("OK", event.ID, true, "")
	}

	return handler
}

// validateBadgeDefinitionEvent performs specific validations for Badge Definition events.
func validateBadgeDefinitionEvent(event nostr.Event) (bool, string) {
	log.Println("Validating Badge Definition event.")
	// Example validation: ensure the 'd' tag is present and correctly formatted.
	for _, tag := range event.Tags {
		if tag[0] == "d" && len(tag) == 2 {
			log.Println("Badge Definition event is valid.")
			return true, ""
		}
	}
	return false, "Missing or invalid 'd' tag in Badge Definition event"
}
