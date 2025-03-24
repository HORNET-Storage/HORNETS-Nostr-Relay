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
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		data, err := read()
		if err != nil {
			log.Println("Error reading from stream:", err)
			write("NOTICE", "Error reading from stream.")
			return
		}

		var event nostr.Event
		if err := json.Unmarshal(data, &event); err != nil {
			log.Println("Error unmarshaling event:", err)
			write("NOTICE", "Error unmarshaling event.")
			return
		}

		// Verify the event is kind 10010
		if event.Kind != 10010 {
			log.Println("Received non-10010 event in kind10010 handler")
			write("NOTICE", "Wrong event type.")
			return
		}

		// Verify and store the event
		if err := store.StoreEvent(&event); err != nil {
			log.Printf("Error storing filter preference event: %v", err)
			write("OK", event.ID, false, "Failed to store filter preference")
			return
		}

		// If successful, overwrite any previous filter preference
		// by deleting old ones for this user
		filters, err := store.QueryEvents(nostr.Filter{
			Kinds:   []int{10010},
			Authors: []string{event.PubKey},
			Limit:   10,
		})

		if err == nil {
			for _, oldEvent := range filters {
				if oldEvent.ID != event.ID {
					if err := store.DeleteEvent(oldEvent.ID); err != nil {
						log.Printf("Warning: could not delete old filter preference: %v", err)
					}
				}
			}
		}

		log.Printf("Stored filter preference for user %s", event.PubKey)
		write("OK", event.ID, true, "")
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
