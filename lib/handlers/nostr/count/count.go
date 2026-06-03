package count

import (
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/sessions"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/transports/websocket"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func BuildCountsHandler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	return func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		data, err := read()
		if err != nil {
			logging.Infof("Error reading from stream:%s", err)
			write("NOTICE", "Error reading from stream.")
			return
		}

		var request nostr.CountEnvelope
		connPubkey := ""

		var wrapper struct {
			Request         *nostr.CountEnvelope `json:"request"`
			AuthPubkey      string               `json:"auth_pubkey"`
			IsAuthenticated bool                 `json:"is_authenticated"`
		}
		if err := json.Unmarshal(data, &wrapper); err == nil && wrapper.Request != nil {
			request = *wrapper.Request
			if wrapper.IsAuthenticated {
				connPubkey = wrapper.AuthPubkey
			}
		} else if err := json.Unmarshal(data, &request); err != nil {
			logging.Infof("Error unmarshaling count request:%s", err)
			write("NOTICE", "Error unmarshaling count request.")
			return
		}

		if connPubkey == "" {
			sessions.Sessions.Range(func(key, value interface{}) bool {
				pubkey, ok := key.(string)
				if !ok {
					return true
				}
				session, ok := value.(*sessions.Session)
				if !ok || !session.Authenticated {
					return true
				}
				connPubkey = pubkey
				return false
			})
		}

		// Check if the request is for counting restricted content
		if isRestrictedCountRequest(request.Filters) {
			logging.Infof("Refusing to count restricted content for subscription ID: %s\n", request.SubscriptionID)
			write("CLOSED", request.SubscriptionID, "auth-required: cannot count other people's DMs")
			return
		}

		accessControl := websocket.GetAccessControl()
		var totalCount int
		for _, filter := range request.Filters {
			events, err := store.QueryEvents(filter)
			if err != nil {
				logging.Infof("Error counting events for filter: %v", err)
				continue
			}

			if accessControl == nil {
				totalCount += len(events)
				continue
			}

			for _, event := range events {
				if err := accessControl.CanReadEvent(event, connPubkey, store); err == nil {
					totalCount++
				}
			}
		}

		logging.Infof("Total count: %d", totalCount)
		logging.Infof("Testing to see request.SubscriptionID:%s", request.SubscriptionID)
		responseJSON, _ := json.Marshal(map[string]int{"count": totalCount})
		write("COUNT", request.SubscriptionID, string(responseJSON))
	}
}

// isRestrictedCountRequest remains the same as in your original handleCounts
func isRestrictedCountRequest(filters []nostr.Filter) bool {
	for _, filter := range filters {
		for _, kind := range filter.Kinds {
			if kind == 4 { // Assuming '4' is for direct messages or similar
				return true
			}
		}
	}
	return false
}
