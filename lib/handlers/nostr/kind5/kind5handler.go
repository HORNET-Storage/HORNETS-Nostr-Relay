package kind5

import (
	"fmt"
	"log"

	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func BuildKind5Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
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
		success := lib_nostr.ValidateEvent(write, env, 5)
		if !success {
			return
		}

		// Inside handleKindFiveEvents, within the for loop that processes each deletion request
		for _, tag := range env.Event.Tags {
			if tag[0] == "e" && len(tag) > 1 {
				eventID := tag[1]
				// Retrieve the public key of the event to be deleted
				pubKey, err := extractPubKeyFromEventID(store, eventID)
				if err != nil {
					log.Printf("Failed to extract public key for event %s: %v", eventID, err)
					// Decide how to handle this error; continue to next tag, respond with an error, etc.
					write("NOTICE", fmt.Sprintf("Failed to extract public key for event %s: %v, the event doesn't exist", eventID, err))
					continue
				}

				log.Println("Found Public key:", pubKey)

				// Validate that the deletion request and the event have the same public key
				if pubKey == env.Event.PubKey {
					if err := store.DeleteEvent(eventID); err != nil {
						log.Printf("Error deleting event %s: %v", eventID, err)
						// Optionally, handle individual delete failures
					} else {
						write("OK", env.Event.ID, true, "Deletion processed")
					}
				} else {
					log.Printf("Public key mismatch for event %s, deletion request ignored", eventID)
					write("NOTICE", fmt.Sprintf("Public key mismatch for event %s, deletion request ignored", eventID))
				}
			}
		}

		// Respond to indicate successful processing of the deletion request
		// responder(stream, "OK", event.ID, true, "Deletion processed")
	}

	return handler
}

func extractPubKeyFromEventID(store stores.Store, eventID string) (string, error) {
	events, err := store.QueryEvents(nostr.Filter{
		IDs: []string{eventID},
	})

	if err != nil {
		return "", err
	}

	if len(events) == 0 {
		return "", fmt.Errorf("no events found for ID: %s", eventID)
	}

	event := events[0]
	return event.PubKey, nil
}
