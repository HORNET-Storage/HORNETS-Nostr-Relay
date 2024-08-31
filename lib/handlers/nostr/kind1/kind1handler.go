package kind1

import (
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/sync"
	jsoniter "github.com/json-iterator/go"
	"log"

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
		}

		if replyingToMissingID != nil && dhtKey != nil {
			relayStore := sync.GetRelayStore()
			if relayStore != nil {
				relays, err := relayStore.GetRelayListDHT(dhtKey)
				if err != nil {
					log.Printf("Failed to get relay list: %v", err)
					write("NOTICE", "Failed to get relay list.")
				} else {
					filter := nostr.Filter{IDs: []string{*replyingToMissingID}}
					// TODO: maybe this should be handled on a different thread
					// TODO: maybe we should sync more than just the one event we are missing
					for _, relay := range relays {
						relayStore.SyncWithRelay(relay, filter)
						//go relayStore.SyncWithRelay(relay, filter)
					}
				}
			} else {
				log.Println("relay store has not been initialized")
				write("NOTICE", "Relay store has not be initialized")
			}
		}

		// Successfully processed event
		write("OK", env.Event.ID, true, "Event stored successfully")
	}

	return handler
}

func missingEvent(eventId string) bool {
	return true
}
