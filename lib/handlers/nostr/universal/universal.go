package universal

import (
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func isReplaceable(kind int) bool {
	return (kind >= 10000 && kind < 20000) || kind == 0 || kind == 3
}

func isEphemeral(kind int) bool {
	return kind >= 20000 && kind < 30000
}

func isAddressable(kind int) bool {
	return kind >= 30000 && kind < 40000
}

func getTagValue(tags nostr.Tags, key string) string {
	for _, tag := range tags {
		if len(tag) >= 2 && tag[0] == key {
			return tag[1]
		}
	}
	return ""
}

func BuildUniversalHandler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
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
		success := lib_nostr.ValidateEvent(write, env, -1)
		if !success {
			return
		}

		kind := env.Event.Kind

		// Ephemeral events: don't store, just acknowledge
		if isEphemeral(kind) {
			write("OK", env.Event.ID, true, "Ephemeral event acknowledged")
			return
		}

		// Replaceable events: delete older events with same pubkey+kind
		if isReplaceable(kind) {
			existingEvents, err := store.QueryEvents(nostr.Filter{
				Kinds:   []int{kind},
				Authors: []string{env.Event.PubKey},
			})
			if err == nil {
				for _, oldEvent := range existingEvents {
					// Only delete if older than new event
					if oldEvent.CreatedAt < env.Event.CreatedAt {
						store.DeleteEvent(oldEvent.ID)
					} else if oldEvent.CreatedAt > env.Event.CreatedAt {
						// New event is older, reject it
						write("OK", env.Event.ID, false, "Replaced by newer event")
						return
					}
					// If same timestamp, keep both (let storage handle dedup by ID)
				}
			}
		}

		// Addressable events: delete older events with same pubkey+kind+d-tag
		if isAddressable(kind) {
			dTag := getTagValue(env.Event.Tags, "d")
			existingEvents, err := store.QueryEvents(nostr.Filter{
				Kinds:   []int{kind},
				Authors: []string{env.Event.PubKey},
				Tags:    nostr.TagMap{"d": []string{dTag}},
			})
			if err == nil {
				for _, oldEvent := range existingEvents {
					if oldEvent.CreatedAt < env.Event.CreatedAt {
						store.DeleteEvent(oldEvent.ID)
					} else if oldEvent.CreatedAt > env.Event.CreatedAt {
						write("OK", env.Event.ID, false, "Replaced by newer event")
						return
					}
				}
			}
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
