package kind444

import (
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// BuildKind444Handler handles Welcome Events (MIP-02)
// These are UNSIGNED events containing encrypted MLS Welcome messages using NIP-59 gift-wrapping.
// Welcome Events enable secure group invitations by providing new members with everything
// they need to join groups. They are intentionally unsigned to prevent accidental public
// publishing if the event leaks.
func BuildKind444Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
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

		// Verify this is a kind 444 event
		if env.Event.Kind != 444 {
			write("OK", env.Event.ID, false, "Invalid event kind")
			return
		}

		// Check if kind 444 is allowed by relay settings
		blocked := lib_nostr.IsKindAllowed(444)
		if !blocked {
			write("OK", env.Event.ID, false, "This kind is not handled by the relay")
			return
		}

		// Validate time check (prevent time travel)
		timeCheck := lib_nostr.TimeCheck(env.Event.CreatedAt.Time().Unix())
		if !timeCheck {
			write("OK", env.Event.ID, false, "The event creation date must be after January 1, 2019")
			return
		}

		// IMPORTANT: We do NOT validate signatures for kind 444 events
		// These events are intentionally unsigned to prevent accidental public publishing
		// Most relays will reject unsigned events by default, providing safety

		// Validate required tags for Welcome Events
		hasETag := false
		hasRelaysTag := false
		hasEncoding := false

		for _, tag := range env.Event.Tags {
			if len(tag) >= 1 {
				switch tag[0] {
				case "e":
					if len(tag) >= 2 {
						hasETag = true
					}
				case "relays":
					if len(tag) >= 2 {
						hasRelaysTag = true
					}
				case "encoding":
					if len(tag) >= 2 {
						hasEncoding = true
					}
				}
			}
		}

		// Check for required tags
		if !hasETag {
			write("NOTICE", "Welcome Event (kind 444) must include 'e' tag referencing the KeyPackage Event")
			return
		}
		if !hasRelaysTag {
			write("NOTICE", "Welcome Event (kind 444) must include 'relays' tag with relay URLs")
			return
		}
		if !hasEncoding {
			write("NOTICE", "Welcome Event (kind 444) must include 'encoding' tag")
			return
		}

		// Store the event
		// The relay doesn't need to decrypt or understand the MLS Welcome content
		if err := store.StoreEvent(&env.Event); err != nil {
			write("NOTICE", "Failed to store the event")
			return
		}

		// Successfully processed event
		write("OK", env.Event.ID, true, "Welcome Event stored successfully")
	}

	return handler
}