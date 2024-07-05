package kind7

import (
	"log"
	"regexp"

	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func BuildKind7Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		log.Println("Handling reaction event.")

		// Load and check relay settings
		settings, err := lib_nostr.LoadRelaySettings()
		if err != nil {
			log.Fatalf("Failed to load relay settings: %v", err)
			return
		}

		// Read data from the stream.
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

		event := env.Event

		blocked := lib_nostr.IsTheKindAllowed(event.Kind, settings)

		// Check if the event kind is allowed
		if !blocked {
			log.Printf("Kind %d not handled by this relay", event.Kind)
			write("NOTICE", "This kind is not handled by the relay.")
			return
		}

		// Validate the event kind is for reactions (kind 7).
		if event.Kind != 7 {
			write("NOTICE", "Unsupported event kind for reaction handler.")
			return
		}

		success, err := event.CheckSignature()
		if err != nil {
			write("OK", event.ID, false, "Failed to check signature")
			return
		}

		if !success {
			write("OK", event.ID, false, "Signature failed to verify")
			return
		}

		// Validate the content of the reaction.
		if !isValidReactionContent(event.Content) {
			write("NOTICE", "Invalid reaction content.")
			return
		}

		/// Store the new event.
		if err := store.StoreEvent(&event); err != nil {
			write("OK", event.ID, false, "Failed to store the event.")
			return
		}

		// Successfully processed reaction event.
		write("OK", event.ID, true, "Reaction event processed successfully.")
	}

	return handler
}

// isValidReactionContent checks if the reaction content is valid.
func isValidReactionContent(content string) bool {
	// Allow "+" for like/upvote, "-" for dislike/downvote, and validate custom emojis if necessary.
	switch content {
	case "+", "-":
		return true
	default:
		return isValidEmoji(content) // Implement this function to validate custom emojis.
	}
}

// isValidEmoji checks if the content is a valid emoji or custom shortcode.
func isValidEmoji(content string) bool {
	// Regular expression for standard Unicode emojis.
	unicodeEmojiRegex := regexp.MustCompile(`[\x{1F600}-\x{1F64F}\x{1F300}-\x{1F5FF}\x{1F680}-\x{1F6FF}\x{1F700}-\x{1F77F}\x{1F780}-\x{1F7FF}\x{1F800}-\x{1F8FF}\x{1F900}-\x{1F9FF}\x{1FA00}-\x{1FA6F}\x{1FA70}-\x{1FAFF}\x{2600}-\x{26FF}\x{2700}-\x{27BF}]+`)

	// Return true if content matches the Unicode emoji regex.
	if unicodeEmojiRegex.MatchString(content) {
		return true
	}

	// Regular expression for custom shortcode format ":shortcode:".
	customShortcodeRegex := regexp.MustCompile(`^:[a-zA-Z0-9_+-]+:$`)

	// Simplified return as suggested by the static check.
	return customShortcodeRegex.MatchString(content)
}
