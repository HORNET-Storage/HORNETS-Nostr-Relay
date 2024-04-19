package kind30008

import (
	"fmt"
	"log"

	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// BuildKind30008Handler constructs and returns a handler function for kind 30008 (Profile Badges) events.
func BuildKind30008Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		log.Println("Working with Profile Badges handler.")

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

		// Ensure the event is of kind 30008.
		if event.Kind != 30008 {
			log.Printf("Received non-kind-30008 event on Profile Badges handler, ignoring.")
			return
		}

		// Perform validation specific to Profile Badges events.
		isValid, errMsg := validateProfileBadgesEvent(event)
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

// validateProfileBadgesEvent checks for the correct structure in a Profile Badges event.
func validateProfileBadgesEvent(event nostr.Event) (bool, string) {
	hasDTag := false
	aePairsValid := true

	for _, tag := range event.Tags {
		if tag[0] == "d" && len(tag) > 1 && tag[1] == "profile_badges" {
			hasDTag = true
		}

		// Check the correctness of 'a' and 'e' tag pairs
		if (tag[0] == "a" || tag[0] == "e") && len(tag) <= 1 {
			aePairsValid = false
		}
	}

	if !hasDTag {
		return false, "Profile Badges event missing 'd' tag with value 'profile_badges'"
	}
	if !aePairsValid {
		return false, "Profile Badges event contains invalid 'a' and 'e' tag pairs"
	}

	log.Println("Profile Badges event is valid.")
	return true, ""
}
