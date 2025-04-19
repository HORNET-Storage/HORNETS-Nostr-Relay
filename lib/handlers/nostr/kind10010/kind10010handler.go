package kind10010

import (
	"log"
	"strings"

	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
)

// FilterPreference represents a user's content filtering preferences
// In the new structure:
// - Instructions are stored directly in the event content
// - Enabled status is stored in a tag ["enabled", "true/false"]
// - Mute words are stored in a tag ["mute", "word1,word2,word3"]
type FilterPreference struct {
	Instructions string   `json:"instructions"` // Filtering instructions
	Enabled      bool     `json:"enabled"`      // Whether filtering is enabled
	MuteWords    []string `json:"mute_words"`   // List of words to mute
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

		// Parse tags to check for enabled status and mute words
		// The content is now directly the instructions

		// Log the content for debugging
		log.Printf("Filter instructions: %s", env.Event.Content)

		// Check for enabled tag and mute words tag
		for _, tag := range env.Event.Tags {
			if len(tag) >= 2 {
				// Check for enabled tag
				if tag[0] == "enabled" {
					enabled := tag[1] == "true"
					log.Printf("Found enabled tag: %v", enabled)
				} else if tag[0] == "mute" && len(tag) >= 2 { // Check for mute words tag
					// Parse comma-separated mute words
					muteWords := strings.Split(tag[1], ",")
					log.Printf("Found mute words: %v", muteWords)
				}
			}
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

	// Create a new preference with default values
	pref := &FilterPreference{
		Enabled:      false,             // Default to disabled
		Instructions: events[0].Content, // Content is now directly the instructions
		MuteWords:    []string{},        // Default to empty mute words list
	}

	// Parse tags for enabled status and mute words
	for _, tag := range events[0].Tags {
		if len(tag) >= 2 {
			// Check for enabled tag
			if tag[0] == "enabled" {
				pref.Enabled = tag[1] == "true"
			} else if tag[0] == "mute" && len(tag) >= 2 { // Check for mute words tag
				// Parse comma-separated mute words
				pref.MuteWords = strings.Split(tag[1], ",")
			}
		}
	}

	return pref, nil
}
