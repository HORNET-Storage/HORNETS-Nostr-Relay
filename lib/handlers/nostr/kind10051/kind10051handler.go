package kind10051

import (
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// BuildKind10051Handler handles KeyPackage Relays List Events (MIP-00)
// These events advertise which relays contain a user's KeyPackages,
// helping others know where to look when they want to invite the user to groups.
// This is a simple list event that requires at least one relay tag.
func BuildKind10051Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		// Read data from the stream
		data, err := read()
		if err != nil {
			write("NOTICE", "Error reading data from stream")
			return
		}

		// Unmarshal the nostr envelope
		var env nostr.EventEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			write("NOTICE", "Failed to deserialize the event envelope")
			return
		}

		// Check relay settings for allowed events whilst also verifying signatures and kind number
		success := lib_nostr.ValidateEvent(write, env, 10051)
		if !success {
			return
		}

		// Validate that the event has at least one "relay" tag
		hasRelayTag := false
		for _, tag := range env.Event.Tags {
			if len(tag) >= 2 && tag[0] == "relay" {
				hasRelayTag = true
				break
			}
		}

		if !hasRelayTag {
			write("NOTICE", "KeyPackage Relays List (kind 10051) must include at least one 'relay' tag")
			return
		}

		// Store the event
		if err := store.StoreEvent(&env.Event); err != nil {
			write("NOTICE", "Failed to store the event")
			return
		}

		// Successfully processed event
		write("OK", env.Event.ID, true, "KeyPackage Relays List stored successfully")
	}

	return handler
}