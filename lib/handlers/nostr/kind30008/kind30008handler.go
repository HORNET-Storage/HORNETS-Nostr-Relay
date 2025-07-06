package kind30008

import (
	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// BuildKind30008Handler constructs and returns a handler function for kind 30008 (Profile Badges) events.
func BuildKind30008Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
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
		success := lib_nostr.ValidateEvent(write, env, 30008)
		if !success {
			return
		}

		// Perform validation specific to Profile Badges events.
		isValid, errMsg := validateProfileBadgesEvent(env.Event)
		if !isValid {
			logging.Info(errMsg)
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

	logging.Info("Profile Badges event is valid.")
	return true, ""
}
