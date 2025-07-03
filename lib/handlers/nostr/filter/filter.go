package filter

import (
	"log"
	"strings"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/sessions"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/subscription"
	"github.com/HORNET-Storage/hornet-storage/lib/sync"
	"github.com/HORNET-Storage/hornet-storage/lib/transports/websocket"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
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
		log.Printf("Wrapper unmarshaled successfully - AuthPubkey: '%s', IsAuthenticated: %v", wrapper.AuthPubkey, wrapper.IsAuthenticated)
		if wrapper.IsAuthenticated && wrapper.AuthPubkey != "" {
			log.Printf("Using authenticated pubkey from request wrapper: %s", wrapper.AuthPubkey)
			return wrapper.AuthPubkey
		} else {
			log.Printf("Wrapper found but not authenticated or empty pubkey")
		}
	} else {
		log.Printf("Failed to unmarshal wrapper structure: %v", err)
	}

	// If we couldn't extract from wrapper, fall back to the old method
	// Find currently authenticated pubkeys by scanning the session store
	log.Printf("Falling back to session store check...")
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
	// No debug flags - removed all debug and benchmarking code

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

		// Initialize subscription manager if needed for kind 11888 events
		var subManager *subscription.SubscriptionManager
		// Check if any filter is requesting kind 11888 events
		needsSubscriptionManager := false
		for _, filter := range request.Filters {
			for _, kind := range filter.Kinds {
				if kind == 11888 {
					needsSubscriptionManager = true
					break
				}
			}
			if needsSubscriptionManager {
				break
			}
		}

		// Only get subscription manager if necessary
		if needsSubscriptionManager {
			// Use the global subscription manager instead of creating a new one
			subManager = subscription.GetGlobalManager()
			if subManager == nil {
				log.Printf("Warning: Global subscription manager not available for kind 11888 event processing")
			}
		}

		// Ensure that we respond to the client after processing all filters
		// defer responder(stream, "EOSE", request.SubscriptionID, "End of stored events")
		var combinedEvents []*nostr.Event
		var missingEventIDs []string

		// Get global relay store for missing note retrieval
		relayStore := sync.GetRelayStore()

		for _, filter := range request.Filters {
			events, err := store.QueryEvents(filter)
			if err != nil {
				log.Printf("Error querying events for filter: %v", err)
				continue
			}

			// Check for missing specific event IDs
			if len(filter.IDs) > 0 {
				foundIDs := make(map[string]bool)
				for _, event := range events {
					foundIDs[event.ID] = true
				}

				// Find missing event IDs
				for _, requestedID := range filter.IDs {
					if !foundIDs[requestedID] {
						missingEventIDs = append(missingEventIDs, requestedID)
						log.Printf(ColorYellow+"[MISSING NOTE] Event ID %s not found locally, will attempt DHT retrieval"+ColorReset, requestedID)
					}
				}
			}

			// No debug logging for database results

			combinedEvents = append(combinedEvents, events...)
		}

		// Attempt to retrieve missing events via DHT (with timeout protection)
		if len(missingEventIDs) > 0 && relayStore != nil {
			log.Printf(ColorCyan+"[MISSING NOTE] Attempting to retrieve %d missing events via DHT"+ColorReset, len(missingEventIDs))

			// Limit the number of missing events we try to retrieve to prevent excessive delays
			maxMissingEvents := 5
			if len(missingEventIDs) > maxMissingEvents {
				log.Printf(ColorYellow+"[MISSING NOTE] Limiting DHT retrieval to %d events (requested %d)"+ColorReset, maxMissingEvents, len(missingEventIDs))
				missingEventIDs = missingEventIDs[:maxMissingEvents]
			}

			for _, eventID := range missingEventIDs {
				log.Printf(ColorCyan+"[MISSING NOTE] Searching for event %s via DHT"+ColorReset, eventID)

				// Strategy 1: If we have author filters in any filter, use those for DHT lookup
				var potentialAuthors []string
				for _, filter := range request.Filters {
					if len(filter.Authors) > 0 {
						potentialAuthors = append(potentialAuthors, filter.Authors...)
					}
				}

				// Try to retrieve from each potential author's relay list (limited attempts)
				var foundEvent *nostr.Event
				maxAuthors := 3 // Limit to prevent excessive DHT lookups
				for i, authorPubkey := range potentialAuthors {
					if i >= maxAuthors {
						break
					}

					response, err := sync.RetrieveMissingNote(eventID, authorPubkey, relayStore, store)
					if err != nil {
						log.Printf(ColorRed+"[MISSING NOTE] Error retrieving event %s for author %s: %v"+ColorReset, eventID, authorPubkey, err)
						continue
					}

					if response.Found && response.Event != nil {
						log.Printf(ColorGreen+"[MISSING NOTE] Successfully retrieved event %s from %s"+ColorReset, eventID, response.RelayURL)
						foundEvent = response.Event
						break
					}
				}

				// Strategy 2: If no authors specified or no event found, try a broader DHT search
				if foundEvent == nil {
					log.Printf(ColorYellow+"[MISSING NOTE] Event %s not found via author-specific search, trying broader DHT search"+ColorReset, eventID)

					// Try to find the event by querying multiple known relay lists
					// This could be enhanced to iterate through all DHT entries
					// For now, we'll just log that we tried
					log.Printf(ColorYellow+"[MISSING NOTE] Broader DHT search for event %s not yet implemented"+ColorReset, eventID)
				}

				// If we found the event, add it to combined events
				if foundEvent != nil {
					combinedEvents = append(combinedEvents, foundEvent)
					log.Printf(ColorGreen+"[MISSING NOTE] Added retrieved event %s to response"+ColorReset, eventID)
				}
			}
		}

		// Deduplicate events
		uniqueEvents := deduplicateEvents(combinedEvents)

		// No verbose debug logging

		// Get the authenticated pubkey for the current connection
		connPubkey := getAuthenticatedPubkey(data)

		// Get moderation mode from relay_settings (default to strict if not specified)
		var isStrict bool = true // Default to strict mode

		// First try to get the moderation mode directly from viper
		moderationMode := viper.GetString("content_filtering.image_moderation.mode")
		if moderationMode == "passive" {
			isStrict = false
			logging.Info("[MODERATION] Using passive moderation mode")
		}

		// Filter out blocked events and handle pending moderation events based on mode
		var filteredEvents []*nostr.Event
		var blockedCount int
		var pendingCount int
		var pendingAllowedCount int

		// Always apply moderation filtering
		for _, event := range uniqueEvents {
			// First check if event is blocked (blocked events are always filtered out)
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

			// Then check if event is pending moderation
			isPending, err := store.IsPendingModeration(event.ID)
			if err != nil {
				log.Printf("Error checking if event %s is pending moderation: %v", event.ID, err)
				// If there's an error, assume not pending
				filteredEvents = append(filteredEvents, event)
				continue
			}

			if isPending {
				pendingCount++

				// Handle based on moderation mode
				if !isStrict {
					// Passive mode: include all pending events
					log.Printf(ColorYellowBold+"[MODERATION] PASSIVE MODE: Including pending event %s for all users"+ColorReset, event.ID)
					filteredEvents = append(filteredEvents, event)
					pendingAllowedCount++
				} else if connPubkey != "" && connPubkey == event.PubKey {
					// Strict mode: only include if requester is author
					log.Printf(ColorYellowBold+"[MODERATION] STRICT MODE: Including pending event %s for author %s"+ColorReset,
						event.ID, connPubkey)
					filteredEvents = append(filteredEvents, event)
					pendingAllowedCount++
				} else {
					// Strict mode: exclude if requester is not author
					log.Printf(ColorYellowBold+"[MODERATION] STRICT MODE: Excluding pending event %s (not author)"+ColorReset, event.ID)
					// Skip this event in strict mode if requester is not the author
					continue
				}
			} else {
				// Not blocked or pending, add to filtered events
				filteredEvents = append(filteredEvents, event)
			}
		}

		// Replace uniqueEvents with the filtered set
		if blockedCount > 0 || pendingCount > 0 {
			log.Printf(ColorRedBold+"[MODERATION] Filtered out %d blocked events, %d/%d pending events allowed"+ColorReset,
				blockedCount, pendingAllowedCount, pendingCount)
			uniqueEvents = filteredEvents
		}

		// No verbose debug logging

		// Get the authenticated pubkey for the current connection
		connPubkey = getAuthenticatedPubkey(data)

		// Add detailed logging
		addLogging(&request, connPubkey)

		// Check read access permissions using the global access control system
		accessControl := websocket.GetAccessControl()
		if accessControl != nil {
			// Only check access if we have a valid pubkey
			if connPubkey == "" {
				log.Printf(ColorRed + "[ACCESS CONTROL] Read access denied: No authenticated user" + ColorReset)
				write("NOTICE", "Read access denied: Authentication required")
				return
			}

			// Check if user has read access
			err := accessControl.CanRead(connPubkey)
			if err != nil {
				log.Printf(ColorRed+"[ACCESS CONTROL] Read access denied for pubkey: %s"+ColorReset, connPubkey)
				write("NOTICE", "Read access denied: User not in allowed list")
				return
			}

			log.Printf(ColorGreen+"[ACCESS CONTROL] Read access granted for user: %s"+ColorReset, connPubkey)
		}

		// Apply mute word filtering if the user is authenticated
		if connPubkey != "" {
			// Get user's filter preferences
			pref, err := kind10010.GetUserFilterPreference(store, connPubkey)

			// Check if filtering is enabled
			if err == nil && pref.Enabled {
				// Separate filterable (kind 1) and non-filterable events
				var filterableEvents []*nostr.Event
				var nonFilterableEvents []*nostr.Event

				for _, e := range uniqueEvents {
					if e.PubKey == connPubkey {
						nonFilterableEvents = append(nonFilterableEvents, e)
						log.Printf(ColorGreenBold+"[CONTENT FILTER] EXEMPT EVENT: ID=%s, Kind=%d, PubKey=%s (authored by requester)"+ColorReset,
							e.ID, e.Kind, e.PubKey)
					} else if e.Kind == 1 { // Only filter kind 1 (text notes)
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

				// Apply mute word filtering if mute words are present
				if len(pref.MuteWords) > 0 {
					log.Printf(ColorCyanBold+"[CONTENT FILTER] APPLYING MUTE WORD FILTER FOR USER: %s"+ColorReset, connPubkey)

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

				// Combine filtered events with non-filterable events
				uniqueEvents = append(nonFilterableEvents, filterableEvents...)
				log.Printf(ColorYellowBold+"[CONTENT FILTER] MUTE-ONLY FILTER: %d events passed mute filtering, %d exempt events"+ColorReset,
					len(filterableEvents), len(nonFilterableEvents))
			} else {
				log.Printf(ColorCyan+"Content filtering not enabled for user %s"+ColorReset, connPubkey)
			}
		}

		// No verbose debug logging

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

			// Special handling for kind 11888 events
			if event.Kind == 11888 {
				log.Printf("Processing kind 11888 event with ID: %s", event.ID)

				// Extract the pubkey the event is about (from the p tag)
				eventPubkey := ""
				for _, tag := range event.Tags {
					if tag[0] == "p" && len(tag) > 1 {
						eventPubkey = tag[1]
						log.Printf("Kind 11888 event is about pubkey: %s", eventPubkey)
						break
					}
				}

				// Log the auth and authorization check
				log.Printf("Auth check for kind 11888: event pubkey=%s, connection pubkey=%s",
					eventPubkey, connPubkey)

				// If the pubkey in the event is not empty and doesn't match the authenticated user
				if eventPubkey != "" && connPubkey != "" && eventPubkey != connPubkey {
					// Skip this event - user is not authorized to see subscription details for other users
					log.Printf("DENIED: Skipping kind 11888 event for pubkey %s - requested by different pubkey %s",
						eventPubkey, connPubkey)
					continue
				} else {
					log.Printf("ALLOWED: Showing kind 11888 event for pubkey %s to connection with pubkey %s",
						eventPubkey, connPubkey)
				}

				// Only update if we have a subscription manager
				// Run update asynchronously to avoid blocking event processing
				if subManager != nil {
					go func(eventCopy *nostr.Event, manager *subscription.SubscriptionManager) {
						log.Printf("Checking if kind 11888 event needs update...")
						updatedEvent, err := manager.CheckAndUpdateSubscriptionEvent(eventCopy)
						if err != nil {
							log.Printf("Error updating kind 11888 event: %v", err)
						} else if updatedEvent != eventCopy {
							log.Printf("Event was updated with new information")
						} else {
							log.Printf("Event did not need updating")
						}
					}(event, subManager)
				}
			}

			// Special handling for moderation-related events (19841, 19842, 19843)
			if event.Kind == 19841 || event.Kind == 19843 {
				// These are relay-created events (tickets and resolutions)
				// Extract the user pubkey from the p tag
				userPubkey := ""
				for _, tag := range event.Tags {
					if tag[0] == "p" && len(tag) > 1 {
						userPubkey = tag[1]
						break
					}
				}

				// Only allow the referenced user to see these events
				if userPubkey != "" && connPubkey != "" && connPubkey != userPubkey {
					log.Printf(ColorRedBold+"[MODERATION] DENIED: Skipping kind %d event - requested by %s but references user %s"+ColorReset,
						event.Kind, connPubkey, userPubkey)
					continue
				} else {
					log.Printf(ColorGreenBold+"[MODERATION] ALLOWED: Showing kind %d event for user %s"+ColorReset,
						event.Kind, userPubkey)
				}
			} else if event.Kind == 19842 {
				// Disputes are created by users
				// Only allow the author to see their own disputes
				if connPubkey != "" && connPubkey != event.PubKey {
					log.Printf(ColorRedBold+"[MODERATION] DENIED: Skipping kind 19842 dispute event - requested by %s but created by %s"+ColorReset,
						connPubkey, event.PubKey)
					continue
				} else {
					log.Printf(ColorGreenBold+"[MODERATION] ALLOWED: Showing kind 19842 dispute event created by %s"+ColorReset,
						event.PubKey)
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
