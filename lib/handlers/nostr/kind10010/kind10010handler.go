package kind10010

import (
	"encoding/json"
	"log"

	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
)

// FilterPreference represents a user's content filtering preferences
type FilterPreference struct {
	Enabled      bool   `json:"enabled"`
	Instructions string `json:"instructions"`
}

// BuildKind10010Handler creates a handler for kind 10010 filter preference events
func BuildKind10010Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		// Use Jsoniter for JSON operations
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		// Read data from the stream
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
		success := lib_nostr.ValidateEvent(write, env, 10010)
		if !success {
			return
		}

		// Verify and store the event
		if err := store.StoreEvent(&env.Event); err != nil {
			log.Printf("Error storing filter preference event: %v", err)
			write("OK", env.Event.ID, false, "Failed to store filter preference")
			return
		}

		// If successful, overwrite any previous filter preference
		// by deleting old ones for this user
		filters, err := store.QueryEvents(nostr.Filter{
			Kinds:   []int{10010},
			Authors: []string{env.Event.PubKey},
			Limit:   10,
		})

		if err == nil {
			for _, oldEvent := range filters {
				if oldEvent.ID != env.Event.ID {
					if err := store.DeleteEvent(oldEvent.ID); err != nil {
						log.Printf("Warning: could not delete old filter preference: %v", err)
					}
				}
			}
		}

		log.Printf("Stored filter preference for user %s", env.Event.PubKey)
		write("OK", env.Event.ID, true, "Event stored successfully")
	}

	return handler
}

// GetUserFilterPreference retrieves a user's filter preferences
func GetUserFilterPreference(store stores.Store, pubkey string) (*FilterPreference, error) {
	// Query the latest filter preference for this user
	events, err := store.QueryEvents(nostr.Filter{
		Kinds:   []int{10010},
		Authors: []string{pubkey},
		Limit:   1,
	})

	if err != nil {
		return nil, err
	}

	if len(events) == 0 {
		// No filter preference found
		return &FilterPreference{Enabled: false}, nil
	}

	// Parse the content as JSON
	var pref FilterPreference
	if err := json.Unmarshal([]byte(events[0].Content), &pref); err != nil {
		// If we can't parse as JSON, assume it's just instructions
		return &FilterPreference{
			Enabled:      true,
			Instructions: events[0].Content,
		}, nil
	}

	return &pref, nil
}
