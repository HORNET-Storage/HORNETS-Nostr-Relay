package kind9373

import (
	"fmt"

	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func BuildKind9373Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

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
		success := lib_nostr.ValidateEvent(write, env, 9373)
		if !success {
			return
		}

		// Validate 'e' tag for reposted event ID and optionally 'p' tag for public key
		repostedEventID, repostedEventFound := getTagValue(env.Event.Tags, "e", "p", "q")

		if !repostedEventFound {
			write("OK", env.Event.ID, false, "Reposted event ID not found in 'e' or 'p' or 'q' tag.")
			return
		}

		// Query the database to validate the existence of the reposted event
		filter := nostr.Filter{
			IDs: []string{repostedEventID},
		}
		repostedEvents, err := store.QueryEvents(filter)
		if err != nil || len(repostedEvents) == 0 {
			errMsg := fmt.Sprintf("Reposted event %s not found", repostedEventID)
			logging.Info(errMsg)
			write("OK", env.Event.ID, false, errMsg)
			return
		}

		// Store the new event
		if err := store.StoreEvent(&env.Event); err != nil {
			write("NOTICE", "Failed to store the event")
			return
		}

		// Successfully processed event
		write("OK", env.Event.ID, true, "Event stored successfully")
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
