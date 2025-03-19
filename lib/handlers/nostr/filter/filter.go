package filter

import (
	"log"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/subscription"
	"github.com/HORNET-Storage/hornet-storage/lib/signing"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib"
)

func BuildFilterHandler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		data, err := read()
		if err != nil {
			log.Println("Error reading from stream:", err)
			write("NOTICE", "Error reading from stream.")
			return
		}

		var request nostr.ReqEnvelope
		if err := json.Unmarshal(data, &request); err != nil {
			log.Println("Error unmarshaling request:", err)
			write("NOTICE", "Error unmarshaling request.")
			return
		}

		// Initialize subscription manager if needed for kind 888 events
		var subManager *subscription.SubscriptionManager
		// Check if any filter is requesting kind 888 events
		needsSubscriptionManager := false
		for _, filter := range request.Filters {
			for _, kind := range filter.Kinds {
				if kind == 888 {
					needsSubscriptionManager = true
					break
				}
			}
			if needsSubscriptionManager {
				break
			}
		}

		// Only initialize subscription manager if necessary
		if needsSubscriptionManager {
			// Get relay private key for signing
			serializedPrivateKey := viper.GetString("private_key")
			
			// Use existing DeserializePrivateKey function from signing package
			relayPrivKey, _, err := signing.DeserializePrivateKey(serializedPrivateKey)
			if err != nil {
				log.Printf("Error loading private key: %v", err)
			} else {
				// Load relay settings
				var settings lib.RelaySettings
				if err := viper.UnmarshalKey("relay_settings", &settings); err != nil {
					log.Printf("Error loading relay settings: %v", err)
				}

				// Get relay DHT key
				relayDHTKey := viper.GetString("RelayDHTkey")

				// Initialize subscription manager
				subManager = subscription.NewSubscriptionManager(
					store,
					relayPrivKey,
					relayDHTKey,
					settings.SubscriptionTiers,
				)
			}
		}

		// Ensure that we respond to the client after processing all filters
		// defer responder(stream, "EOSE", request.SubscriptionID, "End of stored events")
		var combinedEvents []*nostr.Event
		for _, filter := range request.Filters {
			events, err := store.QueryEvents(filter)
			if err != nil {
				log.Printf("Error querying events for filter: %v", err)
				continue
			}
			combinedEvents = append(combinedEvents, events...)
		}

		// Deduplicate events
		uniqueEvents := deduplicateEvents(combinedEvents)

		// Send each unique event to the client
		for _, event := range uniqueEvents {
			// Check and update kind 888 events if necessary
			if event.Kind == 888 && subManager != nil {
				updatedEvent, err := subManager.CheckAndUpdateSubscriptionEvent(event)
				if err != nil {
					log.Printf("Error updating kind 888 event: %v", err)
				} else {
					event = updatedEvent
				}
			}

			eventJSON, err := json.Marshal(event)
			if err != nil {
				log.Printf("Error marshaling event: %v", err)
				continue
			}
			write("EVENT", request.SubscriptionID, string(eventJSON))
		}

		write("EOSE", request.SubscriptionID, "End of stored events")
	}

	return handler
}

func deduplicateEvents(events []*nostr.Event) []*nostr.Event {
	seen := make(map[string]struct{})
	var uniqueEvents []*nostr.Event

	for _, event := range events {
		if _, exists := seen[event.ID]; !exists {
			seen[event.ID] = struct{}{}
			uniqueEvents = append(uniqueEvents, event)
		}
	}

	return uniqueEvents
}