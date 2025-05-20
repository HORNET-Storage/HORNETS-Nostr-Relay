package kind16630

import (
	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func BuildKind16630Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
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

// validatePermissionEventTags checks if the tags array contains the expected structure for a Kind 16629 event.
func validateTags(tags nostr.Tags) string {
	hasRepoTag := false
	hasBranchTag := false

	for _, tag := range tags {
		// Ensure the repository tag is present and correctly formatted
		if tag[0] == "r" && len(tag) == 2 {
			hasRepoTag = true
		}

		// Ensure the branch name tag is present
		if tag[0] == "b" && len(tag) == 2 {
			hasBranchTag = true
		}

		// TODO: Verify a kind 16629 event exists as well
	}

	// Validate required tags
	if !hasRepoTag {
		return "Missing 'r' tag (repository identifier)."
	}
	if !hasBranchTag {
		return "Missing 'b' tag (branch name)."
	}

	return ""
}
