package kind117

import (
	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func BuildKind117Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
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
		success := lib_nostr.ValidateEvent(write, env, 117)
		if !success {
			return
		}

		// Extract blossom_hash from the event tags
		var blossomHash string
		for _, tag := range env.Event.Tags {
			if len(tag) >= 2 && tag[0] == "blossom_hash" {
				blossomHash = tag[1]
				break
			}
		}

		if blossomHash == "" {
			write("NOTICE", "Kind 117 event must include a blossom_hash tag")
			return
		}

		// Check if this author already has a kind 117 event with the same blossom_hash
		filter := nostr.Filter{
			Kinds:   []int{117},
			Authors: []string{env.Event.PubKey},
			Tags:    nostr.TagMap{"blossom_hash": []string{blossomHash}},
		}

		existingEvents, err := store.QueryEvents(filter)
		if err != nil {
			logging.Infof("Kind 117 handler: Error checking for existing events: %v", err)
			write("NOTICE", "Failed to check for existing events")
			return
		}

		if len(existingEvents) > 0 {
			logging.Infof("Kind 117 handler: Rejecting duplicate - Author %s already has kind 117 event with hash %s (existing event ID: %s)",
				env.Event.PubKey, blossomHash, existingEvents[0].ID)
			write("NOTICE", "Duplicate kind 117 event rejected - author already has an event with this blossom_hash")
			return
		}

		logging.Infof("Kind 117 handler: Storing new event - Author: %s, Hash: %s, Event ID: %s",
			env.Event.PubKey, blossomHash, env.Event.ID)

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
