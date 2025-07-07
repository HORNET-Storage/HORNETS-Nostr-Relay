package kind1

import (
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/sync"
	jsoniter "github.com/json-iterator/go"

	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func BuildKind1Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		// Use Jsoniter for JSON operations
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

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

		// Check relay settings for allowed events whilst also verifying signatures and kind number
		success := lib_nostr.ValidateEvent(write, env, 1)
		if !success {
			return
		}

		// Store the new event
		if err := store.StoreEvent(&env.Event); err != nil {
			write("NOTICE", "Failed to store the event")
			return
		}

		var replyingToMissingID *string = nil
		var dhtKey *string = nil
		var parentAuthor *string = nil
		for _, tag := range env.Event.Tags {
			if tag[0] == "e" && len(tag) == 3 && tag[2] == "reply" {
				missing := missingEvent(tag[1])
				if missing {
					replyingToMissingID = &tag[1]
				}
			}
			if tag[0] == "dht_key" && len(tag) == 2 {
				dhtKey = &tag[1]
			}
			if tag[0] == "p" && len(tag) == 2 {
				parentAuthor = &tag[1]
			}
		}

		if replyingToMissingID != nil && dhtKey != nil && parentAuthor != nil {
			relayStore := sync.GetRelayStore()
			if relayStore != nil {
				relays, err := relayStore.GetRelayListFromDHT(dhtKey)
				if err != nil {
					logging.Infof("Failed to get relay list: %v", err)
					write("NOTICE", "Failed to get relay list.")
				} else {
					filter := nostr.Filter{Authors: []string{*parentAuthor}}
					for _, relay := range relays {
						// TODO: maybe this should be handled on a different thread
						//go relayStore.SyncWithRelay(relay, filter)
						relayStore.SyncWithRelay(relay, filter)
						relayStore.AddRelay(relay)
					}
					relayStore.AddAuthor(*parentAuthor)
				}
			} else {
				logging.Info("relay store has not been initialized")
				write("NOTICE", "Relay store has not be initialized")
			}
		} else {
			logging.Info("event tags incomplete, cannot sync")
			logging.Infof("replyToMissingID: %v dhtKey: %v parentAuthor: %v", replyingToMissingID, dhtKey, parentAuthor)
		}

		// Successfully processed event
		write("OK", env.Event.ID, true, "Event stored successfully")
	}

	return handler
}

func missingEvent(_ string) bool {
	// TODO: Implement proper event existence check
	// For now, assume all referenced events are missing to trigger sync
	return true
}
