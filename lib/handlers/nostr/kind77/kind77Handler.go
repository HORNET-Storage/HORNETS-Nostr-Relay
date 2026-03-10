package kind77

import (
	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func BuildKind77Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		// Read data from the stream
		data, err := read()
		if err != nil {
			write("NOTICE", "Error reading data from stream")
			return
		}

		// Unmarshal the nostr envelope
		var env nostr.EventEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			write("NOTICE", "Failed to deserialize the event envelope")
			return
		}

		// Check relay settings for allowed events whilst also verifying signatures and kind number
		success := lib_nostr.ValidateEvent(write, env, -1)
		if !success {
			return
		}

		// Validate tags
		message := validateTags(env.Event.Tags)
		if len(message) > 0 {
			write("NOTICE", message)
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

// validateTags checks if the tags array contains the expected structure for a Kind 77 repo announcement event.
func validateTags(tags nostr.Tags) string {
	hasRepoTag := false
	hasTypeTag := false

	validTypes := map[string]bool{
		"repo-created":   true,
		"repo-forked":    true,
		"issue-created":  true,
		"issue-closed":   true,
		"issue-reopened": true,
		"pr-created":     true,
		"pr-closed":      true,
		"pr-reopened":    true,
		"pr-merged":      true,
		"star":           true,
		"comment":        true,
		"review":         true,
		"label-added":    true,
		"label-removed":  true,
		"assigned":       true,
	}

	for _, tag := range tags {
		if len(tag) < 2 {
			continue
		}
		if tag[0] == "r" {
			hasRepoTag = true
		}
		if tag[0] == "type" {
			if !validTypes[tag[1]] {
				return "Invalid 'type' tag value: " + tag[1]
			}
			hasTypeTag = true
		}
	}

	if !hasRepoTag {
		return "Missing 'r' tag (repository identifier)."
	}
	if !hasTypeTag {
		return "Missing 'type' tag (announcement type)."
	}

	return ""
}
