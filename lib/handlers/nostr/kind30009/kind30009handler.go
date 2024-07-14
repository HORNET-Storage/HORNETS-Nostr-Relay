package kind30009

import (
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
		success := lib_nostr.ValidateEvent(write, env, 30009)
		if !success {
			return
		}

		// Perform validation of the Badge Definition event.
		isValid, errMsg := validateBadgeDefinitionEvent(env.Event)
		if !isValid {
			log.Println(errMsg)
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
