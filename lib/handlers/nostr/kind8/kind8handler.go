package kind8

import (
	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// BuildKind8Handler constructs and returns a handler function for kind 8 (Badge Award) events.
func BuildKind8Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
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
		success := lib_nostr.ValidateEvent(write, env, 8)
		if !success {
			return
		}

		// Validate the badge award event's tags.
		if !isValidBadgeAwardEvent(env.Event) {
			write("NOTICE", "Invalid badge award event.")
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

// isValidBadgeAwardEvent checks for the presence of required tags in a Badge Award event.
func isValidBadgeAwardEvent(event nostr.Event) bool {
	hasATag := false
	hasPTag := false

	for _, tag := range event.Tags {
		switch tag[0] {
		case "a":
			if len(tag) > 1 && tag[1] != "" {
				hasATag = true
			}
		case "p":
			if len(tag) > 1 && tag[1] != "" {
				hasPTag = true
			}
		}
	}

	if !hasATag {
		logging.Info("Badge Award event missing 'a' tag.")
		return false
	}
	if !hasPTag {
		logging.Info("Badge Award event missing 'p' tag.")
		return false
	}

	return true
}
