package kind443

import (
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// BuildKind443Handler handles KeyPackage Events (MIP-00)
// These events contain TLS-serialized MLS KeyPackages that advertise a user's
// ability to join groups. They include credentials, signing keys, and supported extensions.
// The relay treats these as standard events without needing to understand the MLS content.
func BuildKind443Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
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
		success := lib_nostr.ValidateEvent(write, env, 443)
		if !success {
			return
		}

		// Validate required tags for KeyPackage events
		hasMlsProtocol := false
		hasMlsCiphersuite := false
		hasEncoding := false

		for _, tag := range env.Event.Tags {
			if len(tag) >= 2 {
				switch tag[0] {
				case "mls_protocol_version":
					hasMlsProtocol = true
				case "mls_ciphersuite":
					hasMlsCiphersuite = true
				case "encoding":
					hasEncoding = true
				}
			}
		}

		// Check for required tags
		if !hasMlsProtocol {
			write("NOTICE", "KeyPackage Event (kind 443) must include 'mls_protocol_version' tag")
			return
		}
		if !hasMlsCiphersuite {
			write("NOTICE", "KeyPackage Event (kind 443) must include 'mls_ciphersuite' tag")
			return
		}
		if !hasEncoding {
			write("NOTICE", "KeyPackage Event (kind 443) must include 'encoding' tag")
			return
		}

		// Store the event
		// The relay doesn't need to understand or decrypt the MLS KeyPackage content
		if err := store.StoreEvent(&env.Event); err != nil {
			write("NOTICE", "Failed to store the event")
			return
		}

		// Successfully processed event
		write("OK", env.Event.ID, true, "KeyPackage Event stored successfully")
	}

	return handler
}