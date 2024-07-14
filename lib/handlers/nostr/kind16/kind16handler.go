package kind16

import (
	"fmt"
	"log"

	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func BuildKind16Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
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

		// Validate the event
		success := lib_nostr.ValidateEvent(write, env, 16)
		if !success {
			return
		}

		event := env.Event

		// Validate 'e' tag for reposted event ID and relay URL, and 'k' tag for kind number
		repostedEventID, repostedEventFound := getTagValue(event.Tags, "e")
		_, relayFound := getTagRelay(event.Tags, "e")
		repostedEventKind, kindFound := getTagValue(event.Tags, "k")

		if !repostedEventFound || !relayFound || !kindFound {
			write("NOTICE", "Reposted event ID, relay URL, or kind tag not found in 'e' or 'k' tags.")
			return
		}

		// Ensure the reposted kind is not 1
		if repostedEventKind == "1" {
			write("NOTICE", "Kind 16 reposts cannot contain kind 1 events.")
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

		// Store the new event
		if err := store.StoreEvent(&event); err != nil {
			write("NOTICE", "Failed to store the event")
			return
		}

		// Successfully processed event
		write("OK", event.ID, true, "Event stored successfully")
	}

	return handler
}

// getTagValue searches for a tag by its key and returns the first found value and a boolean indicating if it was found.
func getTagValue(tags nostr.Tags, key string) (string, bool) {
	for _, tag := range tags {
		if tag[0] == key && len(tag) > 1 {
			return tag[1], true
		}
	}
	return "", false
}

// getTagRelay searches for a relay URL in the third position of a tag with the specified key.
func getTagRelay(tags nostr.Tags, key string) (string, bool) {
	for _, tag := range tags {
		if tag[0] == key && len(tag) > 2 {
			return tag[2], true
		}
	}
	return "", false
}
