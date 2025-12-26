package kind445

import (
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// BuildKind445Handler handles Group Events (MIP-03)
// These are encrypted group communication messages that include:
// - Control messages (Proposals and Commits)
// - Application messages (regular chat content)
// The relay treats these as opaque encrypted blobs and doesn't need to decrypt or process the content
func BuildKind445Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
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
		// This validates the event signature, checks if kind 445 is allowed, etc.
		success := lib_nostr.ValidateEvent(write, env, 445)
		if !success {
			return
		}

		// Validate that the event has an "h" tag for group ID as required by MIP-03
		hasGroupTag := false
		for _, tag := range env.Event.Tags {
			if len(tag) >= 2 && tag[0] == "h" {
				hasGroupTag = true
				break
			}
		}

		if !hasGroupTag {
			write("NOTICE", "Group Event (kind 445) must include an 'h' tag with the group ID")
			return
		}

		// Store the event
		// The relay doesn't need to decrypt or understand the content - it just stores the encrypted blob
		if err := store.StoreEvent(&env.Event); err != nil {
			write("NOTICE", "Failed to store the event")
			return
		}

		// Successfully processed event
		write("OK", env.Event.ID, true, "Group Event stored successfully")
	}

	return handler
}