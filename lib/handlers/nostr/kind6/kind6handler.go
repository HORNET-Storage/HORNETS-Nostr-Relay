package kind6

import (
	"fmt"
	"log"

	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func BuildKind6Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		data, err := read()
		if err != nil {
			log.Printf("Error reading from stream: %v", err)
			return
		}

		var event nostr.Event
		err = json.Unmarshal(data, &event)
		if err != nil {
			log.Printf("Error unmarshaling event: %v", err)
			return
		}

		// Validate event kind for repost (kind 6 or kind 16 for generic repost)
		if event.Kind != 6 && event.Kind != 16 {
			log.Printf("Received event of kind %d in repost handler, ignoring.", event.Kind)
			return
		}

		// Perform time check (Example: Allow only events within the last 30 days)
		isValid, errMsg := lib_nostr.TimeCheck(event.CreatedAt.Time().Unix())
		if !isValid {
			// If the timestamp is invalid, respond with an error message and return early
			log.Println(errMsg)
			write("OK", event.ID, false, errMsg)
			return
		}

		// Validate 'e' tag for reposted event ID and optionally 'p' tag for public key
		repostedEventID, repostedEventFound := getTagValue(event.Tags, "e", "p")

		if !repostedEventFound {
			log.Println("Reposted event ID not found in 'e' or 'p' tag.")
			return
		}

		// Query the database to validate the existence of the reposted event
		filter := nostr.Filter{
			IDs: []string{repostedEventID},
		}
		repostedEvents, err := store.QueryEvents(filter)
		if err != nil || len(repostedEvents) == 0 {
			errMsg := fmt.Sprintf("Reposted event %s not found", repostedEventID)
			log.Println(errMsg)
			write("OK", event.ID, false, errMsg)
			return
		}

		// Store the repost event
		if err := store.StoreEvent(&event); err != nil {
			errMsg := fmt.Sprintf("Error storing repost event %s: %v", event.ID, err)
			log.Println(errMsg)
			write("OK", event.ID, false, errMsg)
		} else {
			log.Printf("Successfully stored repost event %s.", event.ID)
			write("OK", event.ID, true, "Reposted event stored successfully.")
		}
	}

	return handler
}

// getTagValue searches for a tag by its keys ('e' or 'p') and returns the first found value and a boolean indicating if it was found.
func getTagValue(tags nostr.Tags, keys ...string) (string, bool) {
	keySet := make(map[string]bool)
	for _, key := range keys {
		keySet[key] = true
	}

	for _, tag := range tags {
		if _, ok := keySet[tag[0]]; ok && len(tag) > 1 {
			return tag[1], true
		}
	}
	return "", false
}
