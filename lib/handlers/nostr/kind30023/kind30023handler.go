package kind30023

import (
	"log"
	"regexp"

	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func BuildKind30023Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		// Read data from the stream.
		data, err := read()
		if err != nil {
			write("NOTICE", "Error reading from stream.")
			return
		}

		// Unmarshal the received data into a Nostr event.
		var event nostr.Event
		if err := json.Unmarshal(data, &event); err != nil {
			write("NOTICE", "Error unmarshaling event.")
			return
		}

		log.Println("Working with nip-23 handler.", event.Kind)

		// Validate the event kind.
		if event.Kind != 30023 && event.Kind != 30024 {
			write("NOTICE", "Unsupported event kind for this handler.")
			return
		}

		// Validate Markdown content.
		if !validateMarkdownContent(event.Content) {
			write("NOTICE", "Invalid Markdown content.")
			return
		}

		// Extract 'd' tag value for identifying the article or draft.
		dTagValue := ""
		for _, tag := range event.Tags {
			if tag[0] == "d" {
				dTagValue = tag[1]
				break
			}
		}

		if dTagValue == "" {
			write("NOTICE", "Missing 'd' tag in event.")
			return
		}

		// Create a filter to check for existing events with the same 'd' tag and author.
		filter := nostr.Filter{
			Authors: []string{event.PubKey},
			Tags:    map[string][]string{"d": {dTagValue}},
		}
		existingEvents, err := store.QueryEvents(filter)
		if err != nil {
			write("NOTICE", "Failed to query existing events.")
			return
		}

		// Delete existing events if any.
		for _, existingEvent := range existingEvents {
			if err := store.DeleteEvent(existingEvent.ID); err != nil {
				write("NOTICE", "Failed to delete an existing event.")
				return
			}
		}

		// Store the new event.
		if err := store.StoreEvent(&event); err != nil {
			write("OK", event.ID, false, "Failed to store the event.")
			return
		}

		// Successfully processed event.
		write("OK", event.ID, true, "Event processed successfully.")
	}

	return handler
}

func validateMarkdownContent(content string) bool {
	// Regular expression to detect HTML tags.
	// This is a simplistic approach and might not catch all HTML usages,
	// especially malformed tags or embedded scripts.
	htmlTagRegex := regexp.MustCompile(`<("[^"]*"|'[^']*'|[^'">])*>`)

	// Regular expression to detect hard line-breaks.
	// Markdown uses two spaces at the end of a line to indicate a break.
	hardLineBreakRegex := regexp.MustCompile(`[ ]{2,}\n`)

	// Check for HTML tags
	if htmlTagRegex.MatchString(content) {
		log.Println("Found HTML tags.")
		return false // Found HTML tags, return false
	}

	// Check for hard line-breaks
	if hardLineBreakRegex.MatchString(content) {
		log.Println("Found hard line-breaks.")
		return false // Found hard line-breaks, return false
	}

	// If none of the checks failed, return true
	return true
}
