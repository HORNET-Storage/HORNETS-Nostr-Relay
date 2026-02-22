package filter

import (
	"strings"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/sessions"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/subscription"
	"github.com/HORNET-Storage/hornet-storage/lib/transports/websocket"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind10010"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/search"
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
		logging.Debugf("Wrapper unmarshaled successfully - AuthPubkey: '%s', IsAuthenticated: %v", wrapper.AuthPubkey, wrapper.IsAuthenticated)
		if wrapper.IsAuthenticated && wrapper.AuthPubkey != "" {
			logging.Debugf("Using authenticated pubkey from request wrapper: %s", wrapper.AuthPubkey)
			return wrapper.AuthPubkey
		} else {
			logging.Debugf("Wrapper found but not authenticated or empty pubkey")
		}
	} else {
		logging.Debugf("Failed to unmarshal wrapper structure: %v", err)
	}

	// If we couldn't extract from wrapper, fall back to the old method
	// Find currently authenticated pubkeys by scanning the session store
	logging.Debugf("Falling back to session store check...")
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
			logging.Debugf("Found authenticated pubkey in sessions: %s", pubkey)
		}

		return true // continue
	})

	// Log the authenticated pubkeys we found for debugging
	if len(authenticatedPubkeys) > 0 {
		logging.Debugf("Found %d authenticated pubkeys in session store", len(authenticatedPubkeys))

		// Return the first authenticated pubkey we found
		// In a real implementation, we would match this to the specific connection
		return authenticatedPubkeys[0]
	} else {
		logging.Debugf("No authenticated pubkeys found in session store")
	}

	// If we can't determine the authenticated pubkey, return empty string
	return ""
}

// addLogging adds detailed logging for debugging
func addLogging(reqEnvelope *nostr.ReqEnvelope, connPubkey string) {
	logging.Debugf("Authenticated pubkey for filter request: %s", connPubkey)

	// Log the kinds being requested
	for i, filter := range reqEnvelope.Filters {
		logging.Debugf("Filter #%d requests kinds: %v", i+1, filter.Kinds)

		// Log any 'p' tags that might be filtering by pubkey
		for tagName, tagValues := range filter.Tags {
			if tagName == "p" {
				logging.Debugf("Filter #%d requests events for pubkeys: %v", i+1, tagValues)
			}
		}
	}
}

func BuildFilterHandler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		data, err := read()
		if err != nil {
			logging.Infof("Error reading from stream:%s", err)
			write("NOTICE", "Error reading from stream.")
			return
		}

		var request nostr.ReqEnvelope

		// First try to extract from wrapper structure
		var wrapper struct {
			Request *nostr.ReqEnvelope `json:"request"`
		}
		if err := json.Unmarshal(data, &wrapper); err == nil && wrapper.Request != nil {
			logging.Debug("Successfully extracted request from wrapper structure")
			request = *wrapper.Request
		} else {
			// Fall back to direct unmarshal (for backward compatibility)
			if err := json.Unmarshal(data, &request); err != nil {
				logging.Infof("Error unmarshaling request:%s", err)
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
				logging.Infof("Warning: Global subscription manager not available for kind 11888 event processing")
			}
		}

		// Ensure that we respond to the client after processing all filters
		var combinedEvents []*nostr.Event

		// FIRST: Check access control BEFORE doing any searches
		connPubkey := getAuthenticatedPubkey(data)

		// Check read access permissions using the global access control system
		accessControl := websocket.GetAccessControl()
		if accessControl != nil {

			// Check if user has read access
			err := accessControl.CanRead(connPubkey)
			if err != nil {
				logging.Infof("[ACCESS CONTROL] Read access denied for pubkey: %s", connPubkey)
				write("NOTICE", "Read access denied: User not in allowed list")
				return
			}

			logging.Debugf("[ACCESS CONTROL] Read access granted for user: %s", connPubkey)
		}

		// Parse search extensions if any filter has a search
		var searchQueries []search.SearchQuery
		for _, filter := range request.Filters {
			if filter.Search != "" {
				searchQuery := search.ParseSearchQuery(filter.Search)
				searchQueries = append(searchQueries, searchQuery)
				logging.Debugf("[SEARCH] Parsed query: text='%s', extensions=%v", searchQuery.Text, searchQuery.Extensions)
			}

			events, err := store.QueryEvents(filter)
			if err != nil {
				logging.Infof("Error querying events for filter: %v", err)
				continue
			}

			combinedEvents = append(combinedEvents, events...)
		}

		// Check if any search query has include:spam extension
		includeSpam := false
		for _, sq := range searchQueries {
			if sq.IsSpamIncluded() {
				includeSpam = true
				logging.Debugf("[SEARCH] include:spam extension detected - will include blocked/pending events")
				break
			}
		}

		// Deduplicate events
		uniqueEvents := deduplicateEvents(combinedEvents)

		// Get the authenticated pubkey for the current connection
		connPubkey = getAuthenticatedPubkey(data)

		// Only run moderation filtering when image moderation is enabled.
		// When disabled, no events are ever blocked or pending, so the batch
		// DB lookups would just waste I/O on every single query.
		moderationEnabled := viper.GetBool("content_filtering.image_moderation.enabled")

		if moderationEnabled {
			// Get moderation mode from config (default to strict if not specified)
			var isStrict bool = true // Default to strict mode

			moderationMode := viper.GetString("event_filtering.moderation_mode")
			if moderationMode == "passive" {
				isStrict = false
				logging.Debug("[MODERATION] Using passive moderation mode")
			}

			// Filter out blocked events and handle pending moderation events based on mode
			var filteredEvents []*nostr.Event
			var blockedCount int
			var pendingCount int
			var pendingAllowedCount int

			// Batch-lookup blocked and pending status for all events at once
			// instead of doing N individual DB queries per event.
			eventIDs := make([]string, len(uniqueEvents))
			for i, e := range uniqueEvents {
				eventIDs[i] = e.ID
			}

			blockedMap, err := store.BatchCheckEventsBlocked(eventIDs)
			if err != nil {
				logging.Infof("Error batch-checking blocked events: %v", err)
				blockedMap = make(map[string]bool)
			}

			pendingMap, err := store.BatchCheckPendingModeration(eventIDs)
			if err != nil {
				logging.Infof("Error batch-checking pending moderation: %v", err)
				pendingMap = make(map[string]bool)
			}

			// Apply moderation filtering (unless include:spam is present)
			for _, event := range uniqueEvents {
				isBlocked := blockedMap[event.ID]

				if isBlocked {
					logging.Debugf("[MODERATION] BLOCKED EVENT: ID=%s, Kind=%d, PubKey=%s", event.ID, event.Kind, event.PubKey)
					blockedCount++

					// If include:spam is present, include even blocked events
					if includeSpam {
						filteredEvents = append(filteredEvents, event)
					} else {
						// Skip this event - it's blocked for moderation reasons
						continue
					}
				}

				isPending := pendingMap[event.ID]

				if isPending {
					pendingCount++

					// If include:spam is present, always include pending events
					if includeSpam {
						if !isBlocked {
							filteredEvents = append(filteredEvents, event)
						}
						pendingAllowedCount++
					} else {
						// Handle based on moderation mode
						if !isStrict {
							// Passive mode: include all pending events
							filteredEvents = append(filteredEvents, event)
							pendingAllowedCount++
						} else if connPubkey != "" && connPubkey == event.PubKey {
							// Strict mode: only include if requester is author
							filteredEvents = append(filteredEvents, event)
							pendingAllowedCount++
						} else {
							// Strict mode: exclude if requester is not author
							continue
						}
					}
				} else {
					// Not blocked or pending, add to filtered events
					if !isBlocked {
						filteredEvents = append(filteredEvents, event)
					}
				}
			}

			// Replace uniqueEvents with the filtered set
			if blockedCount > 0 || pendingCount > 0 {
				logging.Debugf("[MODERATION] Filtered out %d blocked events, %d/%d pending events allowed",
					blockedCount, pendingAllowedCount, pendingCount)
				uniqueEvents = filteredEvents
			}
		}

		// Add detailed logging
		addLogging(&request, connPubkey)

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
						logging.Debugf("[CONTENT FILTER] EXEMPT EVENT: ID=%s, Kind=%d, PubKey=%s (authored by requester)",
							e.ID, e.Kind, e.PubKey)
					} else if e.Kind == 1 { // Only filter kind 1 (text notes)
						filterableEvents = append(filterableEvents, e)
					} else {
						nonFilterableEvents = append(nonFilterableEvents, e)
						logging.Debugf("[CONTENT FILTER] EXEMPT EVENT: ID=%s, Kind=%d, PubKey=%s (non-filterable event kind)",
							e.ID, e.Kind, e.PubKey)
					}
				}

				// Store the original count for proper logging
				originalCount := len(filterableEvents)

				// Log event details before filtering for diagnostics
				logging.Debugf("[CONTENT FILTER] PROCESSING: %d filterable events for user %s", originalCount, connPubkey)
				for _, e := range filterableEvents {
					logging.Debugf("[CONTENT FILTER] EVENT TO FILTER: ID=%s, Kind=%d, PubKey=%s, Content: %s",
						e.ID, e.Kind, e.PubKey, truncateString(e.Content, 50))
				}

				// Apply mute word filtering if mute words are present
				if len(pref.MuteWords) > 0 {
					logging.Debugf("[CONTENT FILTER] APPLYING MUTE WORD FILTER FOR USER: %s", connPubkey)

					// Create a filtered list that excludes events with muted words
					var muteFilteredEvents []*nostr.Event

					for _, e := range filterableEvents {
						containsMutedWord := false

						// Check if event content contains any muted word
						for _, muteWord := range pref.MuteWords {
							if muteWord != "" && strings.Contains(strings.ToLower(e.Content), strings.ToLower(muteWord)) {
								logging.Debugf("[CONTENT FILTER] MUTED WORD: '%s' found in event ID=%s",
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

					logging.Debugf("[CONTENT FILTER] MUTE FILTER: %d/%d events passed mute word filtering",
						len(filterableEvents), originalCount)
				}

				// Combine filtered events with non-filterable events
				uniqueEvents = append(nonFilterableEvents, filterableEvents...)
				logging.Debugf("[CONTENT FILTER] MUTE-ONLY FILTER: %d events passed mute filtering, %d exempt events",
					len(filterableEvents), len(nonFilterableEvents))
			} else {
				logging.Debugf("Content filtering not enabled for user %s", connPubkey)
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
					logging.Debugf("DENIED: Skipping kind 10010 event for pubkey %s - requested by different pubkey %s",
						eventPubkey, connPubkey)
					continue
				}
			}

			// Special handling for kind 11888 events
			if event.Kind == 11888 {
				logging.Debugf("Processing kind 11888 event with ID: %s", event.ID)

				// Extract the pubkey the event is about (from the p tag)
				eventPubkey := ""
				for _, tag := range event.Tags {
					if tag[0] == "p" && len(tag) > 1 {
						eventPubkey = tag[1]
						logging.Debugf("Kind 11888 event is about pubkey: %s", eventPubkey)
						break
					}
				}

				// Log the auth and authorization check
				logging.Debugf("Auth check for kind 11888: event pubkey=%s, connection pubkey=%s",
					eventPubkey, connPubkey)

				// If the pubkey in the event is not empty and doesn't match the authenticated user
				if eventPubkey != "" && connPubkey != "" && eventPubkey != connPubkey {
					// Skip this event - user is not authorized to see subscription details for other users
					logging.Debugf("DENIED: Skipping kind 11888 event for pubkey %s - requested by different pubkey %s",
						eventPubkey, connPubkey)
					continue
				} else {
					logging.Debugf("ALLOWED: Showing kind 11888 event for pubkey %s to connection with pubkey %s",
						eventPubkey, connPubkey)
				}

				// Only update if we have a subscription manager
				// Run update asynchronously to avoid blocking event processing
				if subManager != nil {
					go func(eventCopy *nostr.Event, manager *subscription.SubscriptionManager) {
						logging.Debugf("Checking if kind 11888 event needs update...")
						updatedEvent, err := manager.CheckAndUpdateSubscriptionEvent(eventCopy)
						if err != nil {
							logging.Debugf("Error updating kind 11888 event: %v", err)
						} else if updatedEvent != eventCopy {
							logging.Debugf("Event was updated with new information")
						} else {
							logging.Debugf("Event did not need updating")
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
					logging.Debugf("[MODERATION] DENIED: Skipping kind %d event - requested by %s but references user %s",
						event.Kind, connPubkey, userPubkey)
					continue
				} else {
					logging.Debugf("[MODERATION] ALLOWED: Showing kind %d event for user %s",
						event.Kind, userPubkey)
				}
			} else if event.Kind == 19842 {
				// Disputes are created by users
				// Only allow the author to see their own disputes
				if connPubkey != "" && connPubkey != event.PubKey {
					logging.Debugf("[MODERATION] DENIED: Skipping kind 19842 dispute event - requested by %s but created by %s",
						connPubkey, event.PubKey)
					continue
				} else {
					logging.Debugf("[MODERATION] ALLOWED: Showing kind 19842 dispute event created by %s",
						event.PubKey)
				}
			}

			eventJSON, err := json.Marshal(event)
			if err != nil {
				logging.Infof("Error marshaling event: %v", err)
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
