package count

import (
	"log"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func BuildCountsHandler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	return func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary
		log.Println("Working with count request.")

		data, err := read()
		if err != nil {
			log.Println("Error reading from stream:", err)
			write("NOTICE", "Error reading from stream.")
			return
		}

		var request nostr.CountEnvelope
		if err := json.Unmarshal(data, &request); err != nil {
			log.Println("Error unmarshaling count request:", err)
			write("NOTICE", "Error unmarshaling count request.")
			return
		}

		// Check if the request is for counting restricted content
		if isRestrictedCountRequest(request.Filters) {
			log.Printf("Refusing to count restricted content for subscription ID: %s\n", request.SubscriptionID)
			write("CLOSED", request.SubscriptionID, "auth-required: cannot count other people's DMs")
			return
		}

		var totalCount int
		for _, filter := range request.Filters {
			count, err := store.QueryEvents(filter) //CountEvents(filter) // Assume QueryEvents now returns both events and counts or adjust accordingly
			if err != nil {
				log.Printf("Error counting events for filter: %v", err)
				continue
			}
			totalCount += len(count)
		}

		log.Printf("Total count: %d", totalCount)
		log.Println("Testing to see request.SubscriptionID:", request.SubscriptionID)
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
