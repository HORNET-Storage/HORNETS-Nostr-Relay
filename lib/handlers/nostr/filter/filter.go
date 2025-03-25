package filter

import (
	"log"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/sessions"
	"github.com/HORNET-Storage/hornet-storage/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/subscription"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	"github.com/HORNET-Storage/hornet-storage/lib"
	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/contentfilter"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind10010"
)

// getAuthenticatedPubkey attempts to extract the authenticated pubkey from session state
// This function looks for session information in the connection data
func getAuthenticatedPubkey() string {
	// In a real implementation, we would access the authenticated pubkey from the
	// connection state that's stored in the websocket handler.

	// For this implementation, we need to use a more direct approach since
	// we don't have access to the connection state.
	// Note: The read parameter is not used in this implementation but would be used
	// in a more complete implementation to access connection-specific data

	// Find currently authenticated pubkeys by scanning the session store
	var authenticatedPubkeys []string
	sessions.Sessions.Range(func(key, value interface{}) bool {
		pubkey, ok := key.(string)
		if !ok {
			return true // continue
		}

		session, ok := value.(*sessions.Session)
		if !ok {
			return true // continue
		}

		if session.Authenticated {
			authenticatedPubkeys = append(authenticatedPubkeys, pubkey)
			log.Printf("Found authenticated pubkey in sessions: %s", pubkey)
		}

		return true // continue
	})

	// Log the authenticated pubkeys we found for debugging
	if len(authenticatedPubkeys) > 0 {
		log.Printf("Found %d authenticated pubkeys in session store", len(authenticatedPubkeys))

		// Return the first authenticated pubkey we found
		// In a real implementation, we would match this to the specific connection
		return authenticatedPubkeys[0]
	} else {
		log.Printf("No authenticated pubkeys found in session store")
	}

	// If we can't determine the authenticated pubkey, return empty string
	return ""
}

// addLogging adds detailed logging for debugging
func addLogging(reqEnvelope *nostr.ReqEnvelope, connPubkey string) {
	log.Printf("Authenticated pubkey for filter request: %s", connPubkey)

	// Log the kinds being requested
	for i, filter := range reqEnvelope.Filters {
		log.Printf("Filter #%d requests kinds: %v", i+1, filter.Kinds)

		// Log any 'p' tags that might be filtering by pubkey
		for tagName, tagValues := range filter.Tags {
			if tagName == "p" {
				log.Printf("Filter #%d requests events for pubkeys: %v", i+1, tagValues)
			}
		}
	}
}

func BuildFilterHandler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	// Initialize content filter service with direct Ollama integration
	filterConfig := contentfilter.ServiceConfig{
		APIURL:     viper.GetString("ollama_url"),
		Model:      viper.GetString("ollama_model"),
		Timeout:    time.Duration(viper.GetInt("ollama_timeout")) * time.Millisecond,
		CacheSize:  viper.GetInt("content_filter_cache_size"),
		CacheTTL:   time.Duration(viper.GetInt("content_filter_cache_ttl")) * time.Minute,
		FilterKind: []int{1}, // Default to filtering only kind 1 events (text notes)
		Enabled:    viper.GetBool("content_filter_enabled"),
	}

	// Create the filter service
	filterService := contentfilter.NewService(filterConfig)

	// Start a background goroutine to periodically clean up the cache
	filterService.RunPeriodicCacheCleanup(15 * time.Minute)

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

		// Get the authenticated pubkey for the current connection
		connPubkey := getAuthenticatedPubkey()

		// Add detailed logging
		addLogging(&request, connPubkey)

		// Apply content filtering if the user is authenticated
		if connPubkey != "" && filterService.ShouldFilterKind(1) {
			// Get user's filter preferences
			pref, err := kind10010.GetUserFilterPreference(store, connPubkey)
			if err == nil && pref.Enabled && pref.Instructions != "" {
				log.Printf("Applying content filter for user %s", connPubkey)

				// Separate filterable (kind 1) and non-filterable events
				var filterableEvents []*nostr.Event
				var nonFilterableEvents []*nostr.Event

				for _, e := range uniqueEvents {
					if filterService.ShouldFilterKind(e.Kind) {
						filterableEvents = append(filterableEvents, e)
					} else {
						nonFilterableEvents = append(nonFilterableEvents, e)
						log.Printf("EXEMPT from filtering: ID=%s, Kind=%d, PubKey=%s (non-filterable event kind)",
							e.ID, e.Kind, e.PubKey)
					}
				}

				// Store the original count for proper logging
				originalCount := len(filterableEvents)

				// Log event details before filtering for diagnostics
				log.Printf("Before filtering: Processing %d filterable events for user %s", originalCount, connPubkey)
				for _, e := range filterableEvents {
					log.Printf("Event to filter: ID=%s, Kind=%d, PubKey=%s, Content (first 50 chars): %s",
						e.ID, e.Kind, e.PubKey, truncateString(e.Content, 50))
				}

				// Only filter the filterable events
				if len(filterableEvents) > 0 {
					filteredEvents, err := filterService.FilterEvents(filterableEvents, pref.Instructions)
					if err != nil {
						log.Printf("Error filtering events: %v", err)
						// On error, use the original filterable events
						// Combine with non-filterable events
						uniqueEvents = append(nonFilterableEvents, filterableEvents...)
					} else {
						// Log which events passed filtering
						log.Printf("Events that passed filtering:")
						for _, e := range filteredEvents {
							log.Printf("PASSED: ID=%s, Kind=%d, PubKey=%s", e.ID, e.Kind, e.PubKey)
						}

						// Combine filtered events with non-filterable events
						uniqueEvents = append(nonFilterableEvents, filteredEvents...)
						log.Printf("Filtered events: %d/%d filterable events passed filter, %d exempt events",
							len(filteredEvents), originalCount, len(nonFilterableEvents))
					}
				} else {
					// No filterable events, just use non-filterable ones
					uniqueEvents = nonFilterableEvents
					log.Printf("No filterable events to process, %d exempt events passed through", len(nonFilterableEvents))
				}
			} else {
				log.Printf("Content filtering not enabled for user %s", connPubkey)
			}
		}

		// Send each unique event to the client
		for _, event := range uniqueEvents {
			// Special handling for kind 10010 events - only visible to the author
			if event.Kind == 10010 {
				// Extract the pubkey the event is about (the author)
				eventPubkey := event.PubKey

				// If the authenticated user doesn't match the author
				if connPubkey != "" && connPubkey != eventPubkey {
					// Skip this event - user is not authorized to see filter preferences for other users
					log.Printf("DENIED: Skipping kind 10010 event for pubkey %s - requested by different pubkey %s",
						eventPubkey, connPubkey)
					continue
				}
			}

			// Special handling for kind 888 events
			if event.Kind == 888 {
				log.Printf("Processing kind 888 event with ID: %s", event.ID)

				// Extract the pubkey the event is about (from the p tag)
				eventPubkey := ""
				for _, tag := range event.Tags {
					if tag[0] == "p" && len(tag) > 1 {
						eventPubkey = tag[1]
						log.Printf("Kind 888 event is about pubkey: %s", eventPubkey)
						break
					}
				}

				// Log the auth and authorization check
				log.Printf("Auth check for kind 888: event pubkey=%s, connection pubkey=%s",
					eventPubkey, connPubkey)

				// If the pubkey in the event is not empty and doesn't match the authenticated user
				if eventPubkey != "" && connPubkey != "" && eventPubkey != connPubkey {
					// Skip this event - user is not authorized to see subscription details for other users
					log.Printf("DENIED: Skipping kind 888 event for pubkey %s - requested by different pubkey %s",
						eventPubkey, connPubkey)
					continue
				} else {
					log.Printf("ALLOWED: Showing kind 888 event for pubkey %s to connection with pubkey %s",
						eventPubkey, connPubkey)
				}

				// Only update if we have a subscription manager
				if subManager != nil {
					log.Printf("Checking if kind 888 event needs update...")
					updatedEvent, err := subManager.CheckAndUpdateSubscriptionEvent(event)
					if err != nil {
						log.Printf("Error updating kind 888 event: %v", err)
					} else if updatedEvent != event {
						log.Printf("Event was updated with new information")
						event = updatedEvent
					} else {
						log.Printf("Event did not need updating")
					}
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

// truncateString limits the length of a string for logging purposes
func truncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength] + "..."
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
