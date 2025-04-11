package filter

import (
	"log"
	"strings"
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

// getAuthenticatedPubkey attempts to extract the authenticated pubkey from request data
// This function first checks if the data contains an auth wrapper, then falls back to sessions
func getAuthenticatedPubkey(data []byte) string {
	// First, try to extract pubkey from the wrapper structure
	var wrapper struct {
		Request         *nostr.ReqEnvelope `json:"request"`
		AuthPubkey      string             `json:"auth_pubkey"`
		IsAuthenticated bool               `json:"is_authenticated"`
	}

	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(data, &wrapper); err == nil {
		if wrapper.IsAuthenticated && wrapper.AuthPubkey != "" {
			log.Printf("Using authenticated pubkey from request wrapper: %s", wrapper.AuthPubkey)
			return wrapper.AuthPubkey
		}
	}

	// If we couldn't extract from wrapper, fall back to the old method
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

// ANSI color codes for colorized logging
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"

	// Bold variants
	ColorRedBold    = "\033[1;31m"
	ColorGreenBold  = "\033[1;32m"
	ColorYellowBold = "\033[1;33m"
	ColorBlueBold   = "\033[1;34m"
	ColorPurpleBold = "\033[1;35m"
	ColorCyanBold   = "\033[1;36m"
	ColorWhiteBold  = "\033[1;37m"
)

// addLogging adds detailed logging for debugging
func addLogging(reqEnvelope *nostr.ReqEnvelope, connPubkey string) {
	log.Printf(ColorBlue+"Authenticated pubkey for filter request: %s"+ColorReset, connPubkey)

	// Log the kinds being requested
	for i, filter := range reqEnvelope.Filters {
		log.Printf(ColorCyan+"Filter #%d requests kinds: %v"+ColorReset, i+1, filter.Kinds)

		// Log any 'p' tags that might be filtering by pubkey
		for tagName, tagValues := range filter.Tags {
			if tagName == "p" {
				log.Printf(ColorCyan+"Filter #%d requests events for pubkeys: %v"+ColorReset, i+1, tagValues)
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

		// First try to extract from wrapper structure
		var wrapper struct {
			Request *nostr.ReqEnvelope `json:"request"`
		}
		if err := json.Unmarshal(data, &wrapper); err == nil && wrapper.Request != nil {
			log.Println("Successfully extracted request from wrapper structure")
			request = *wrapper.Request
		} else {
			// Fall back to direct unmarshal (for backward compatibility)
			if err := json.Unmarshal(data, &request); err != nil {
				log.Println("Error unmarshaling request:", err)
				write("NOTICE", "Error unmarshaling request.")
				return
			}
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

		// Filter out blocked events
		var filteredEvents []*nostr.Event
		var blockedCount int

		for _, event := range uniqueEvents {
			isBlocked, err := store.IsEventBlocked(event.ID)
			if err != nil {
				log.Printf("Error checking if event %s is blocked: %v", event.ID, err)
				// If there's an error, assume not blocked
				filteredEvents = append(filteredEvents, event)
				continue
			}

			if isBlocked {
				log.Printf(ColorRedBold+"[MODERATION] BLOCKED EVENT: ID=%s, Kind=%d, PubKey=%s (failed image moderation)"+ColorReset,
					event.ID, event.Kind, event.PubKey)
				blockedCount++
				// Skip this event - it's blocked for moderation reasons
				continue
			}

			// Not blocked, add to filtered events
			filteredEvents = append(filteredEvents, event)
		}

		// Replace uniqueEvents with the filtered set
		if blockedCount > 0 {
			log.Printf(ColorRedBold+"[MODERATION] Filtered out %d blocked events"+ColorReset, blockedCount)
			uniqueEvents = filteredEvents
		}

		// Get the authenticated pubkey for the current connection
		connPubkey := getAuthenticatedPubkey(data)

		// Add detailed logging
		addLogging(&request, connPubkey)

		// Apply content filtering if the user is authenticated
		if connPubkey != "" && filterService.ShouldFilterKind(1) {
			// Get user's filter preferences
			pref, err := kind10010.GetUserFilterPreference(store, connPubkey)

			// Check if filtering is enabled and either instructions or mute words are present
			if err == nil && pref.Enabled && (pref.Instructions != "" || len(pref.MuteWords) > 0) {
				log.Printf(ColorCyanBold+"[CONTENT FILTER] APPLYING FILTER FOR USER: %s"+ColorReset, connPubkey)

				// Separate filterable (kind 1) and non-filterable events
				var filterableEvents []*nostr.Event
				var nonFilterableEvents []*nostr.Event

				for _, e := range uniqueEvents {
					if e.PubKey == connPubkey {
						nonFilterableEvents = append(nonFilterableEvents, e)
						log.Printf(ColorGreenBold+"[CONTENT FILTER] EXEMPT EVENT: ID=%s, Kind=%d, PubKey=%s (authored by requester)"+ColorReset,
							e.ID, e.Kind, e.PubKey)
					} else if filterService.ShouldFilterKind(e.Kind) {
						filterableEvents = append(filterableEvents, e)
					} else {
						nonFilterableEvents = append(nonFilterableEvents, e)
						log.Printf(ColorGreenBold+"[CONTENT FILTER] EXEMPT EVENT: ID=%s, Kind=%d, PubKey=%s (non-filterable event kind)"+ColorReset,
							e.ID, e.Kind, e.PubKey)
					}
				}

				// Store the original count for proper logging
				originalCount := len(filterableEvents)

				// Log event details before filtering for diagnostics
				log.Printf(ColorCyanBold+"[CONTENT FILTER] PROCESSING: %d filterable events for user %s"+ColorReset, originalCount, connPubkey)
				for _, e := range filterableEvents {
					log.Printf(ColorCyan+"[CONTENT FILTER] EVENT TO FILTER: ID=%s, Kind=%d, PubKey=%s, Content: %s"+ColorReset,
						e.ID, e.Kind, e.PubKey, truncateString(e.Content, 50))
				}

				// Step 1: Apply mute word filtering if mute words are present
				if len(pref.MuteWords) > 0 {
					// Create a filtered list that excludes events with muted words
					var muteFilteredEvents []*nostr.Event

					for _, e := range filterableEvents {
						containsMutedWord := false

						// Check if event content contains any muted word
						for _, muteWord := range pref.MuteWords {
							if muteWord != "" && strings.Contains(strings.ToLower(e.Content), strings.ToLower(muteWord)) {
								log.Printf(ColorRedBold+"[CONTENT FILTER] MUTED WORD: '%s' found in event ID=%s"+ColorReset,
									muteWord, e.ID)
								containsMutedWord = true
								break
							}
						}

						// Only keep events that don't contain muted words
						if !containsMutedWord {
							muteFilteredEvents = append(muteFilteredEvents, e)
						}
					}

					// Replace the filterable events with the mute-filtered list
					filterableEvents = muteFilteredEvents

					log.Printf(ColorYellowBold+"[CONTENT FILTER] MUTE FILTER: %d/%d events passed mute word filtering"+ColorReset,
						len(filterableEvents), originalCount)
				}

				// Step 2: Apply instruction-based filtering if instructions exist
				if pref.Instructions != "" {
					// Only filter the filterable events if there are any left after mute filtering
					if len(filterableEvents) > 0 {
						filteredEvents, err := filterService.FilterEvents(filterableEvents, pref.Instructions)
						if err != nil {
							log.Printf("Error filtering events: %v", err)
							// On error, use the events that passed mute filtering
							// Combine with non-filterable events
							uniqueEvents = append(nonFilterableEvents, filterableEvents...)
						} else {
							// Log which events passed filtering
							log.Printf(ColorGreenBold + "[CONTENT FILTER] EVENTS THAT PASSED FILTERING:" + ColorReset)
							for _, e := range filteredEvents {
								log.Printf(ColorGreen+"[CONTENT FILTER] PASSED: ID=%s, Kind=%d, PubKey=%s"+ColorReset, e.ID, e.Kind, e.PubKey)
							}

							// Combine filtered events with non-filterable events
							uniqueEvents = append(nonFilterableEvents, filteredEvents...)
							log.Printf(ColorYellowBold+"[CONTENT FILTER] RESULTS: %d/%d filterable events passed filter, %d exempt events"+ColorReset,
								len(filteredEvents), originalCount, len(nonFilterableEvents))
						}
					} else {
						// No filterable events left after mute filtering, just use non-filterable ones
						uniqueEvents = nonFilterableEvents
						log.Printf(ColorYellowBold+"[CONTENT FILTER] NO FILTERABLE EVENTS: %d exempt events passed through"+ColorReset, len(nonFilterableEvents))
					}
				} else {
					// No instructions but we've filtered by mute words, use the mute-filtered events
					uniqueEvents = append(nonFilterableEvents, filterableEvents...)
					log.Printf(ColorYellowBold+"[CONTENT FILTER] MUTE-ONLY FILTER: %d events passed mute filtering, %d exempt events"+ColorReset,
						len(filterableEvents), len(nonFilterableEvents))
				}
			} else {
				log.Printf(ColorCyan+"Content filtering not enabled for user %s"+ColorReset, connPubkey)
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
