package kind1063

import (
	"log"
	"regexp"
	"strconv"
	"strings"

	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func BuildKind1063Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
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
		success := lib_nostr.ValidateEvent(write, env, 1063)
		if !success {
			return
		}

		// Validate Kind 1063 specific requirements
		if !validateAudioMessage(&env.Event, write) {
			return
		}

		// Handle replaceable event logic
		blossomHash := getTagValue(env.Event.Tags, "blossom_hash")
		if blossomHash == "" {
			write("NOTICE", "Missing required blossom_hash tag")
			return
		}

		// Check for existing events to replace (same author + same blossom_hash)
		existingEvents, err := findExistingAudioEvents(store, env.Event.PubKey, blossomHash)
		if err != nil {
			log.Printf("Kind 1063 handler: Error checking for existing events: %v", err)
			write("NOTICE", "Failed to check for existing events")
			return
		}

		// Handle replacement logic
		var eventsToDelete []string
		for _, existingEvent := range existingEvents {
			if env.Event.CreatedAt > existingEvent.CreatedAt {
				// New event is newer, mark old event for deletion
				eventsToDelete = append(eventsToDelete, existingEvent.ID)
				log.Printf("Kind 1063 handler: Replacing older event %s with newer event %s",
					existingEvent.ID, env.Event.ID)
			} else {
				// Existing event is newer or same age, reject new event
				log.Printf("Kind 1063 handler: Rejecting event %s - older than existing event %s",
					env.Event.ID, existingEvent.ID)
				write("NOTICE", "Event rejected - older than existing event for same audio file")
				return
			}
		}

		// Delete replaced events
		for _, eventID := range eventsToDelete {
			if err := store.DeleteEvent(eventID); err != nil {
				log.Printf("Kind 1063 handler: Warning - failed to delete replaced event %s: %v", eventID, err)
				// Continue anyway - the new event will still be stored
			}
		}

		// Store the new event
		if err := store.StoreEvent(&env.Event); err != nil {
			write("NOTICE", "Failed to store the event")
			return
		}

		log.Printf("Kind 1063 handler: Stored audio message - Author: %s, Hash: %s, Event ID: %s, Replaced: %d events",
			env.Event.PubKey, blossomHash, env.Event.ID, len(eventsToDelete))

		// Successfully processed event
		write("OK", env.Event.ID, true, "Event stored successfully")
	}

	return handler
}

// validateAudioMessage validates Kind 1063 specific requirements
func validateAudioMessage(event *nostr.Event, write lib_nostr.KindWriter) bool {
	// Required tags
	requiredTags := []string{"blossom_hash", "url", "m", "duration", "transcription_status", "transcription_provider", "language"}

	for _, tagName := range requiredTags {
		if getTagValue(event.Tags, tagName) == "" {
			write("NOTICE", "Missing required tag: "+tagName)
			return false
		}
	}

	// Validate blossom_hash format (64-character hex string)
	blossomHash := getTagValue(event.Tags, "blossom_hash")
	if !isValidSHA256(blossomHash) {
		write("NOTICE", "Invalid blossom_hash format - must be 64-character hex string")
		return false
	}

	// Validate URL format
	url := getTagValue(event.Tags, "url")
	if !isValidBlossomURL(url) {
		write("NOTICE", "Invalid Blossom URL format")
		return false
	}

	// Validate duration
	durationStr := getTagValue(event.Tags, "duration")
	duration, err := strconv.Atoi(durationStr)
	if err != nil || duration < 1 || duration > 3600 {
		write("NOTICE", "Invalid duration - must be integer between 1 and 3600 seconds")
		return false
	}

	// Validate MIME type
	mimeType := getTagValue(event.Tags, "m")
	allowedMimeTypes := []string{"audio/aac", "audio/mpeg", "audio/wav", "audio/mp4", "audio/webm"}
	if !contains(allowedMimeTypes, mimeType) {
		write("NOTICE", "Invalid MIME type - must be supported audio format")
		return false
	}

	// Validate transcription status
	status := getTagValue(event.Tags, "transcription_status")
	allowedStatuses := []string{"local", "processing", "completed", "failed", "manual"}
	if !contains(allowedStatuses, status) {
		write("NOTICE", "Invalid transcription_status - must be: local, processing, completed, failed, or manual")
		return false
	}

	// Validate language code (basic validation)
	language := getTagValue(event.Tags, "language")
	if !isValidLanguageCode(language) {
		write("NOTICE", "Invalid language code")
		return false
	}

	// Validate optional confidence score
	confidence := getTagValue(event.Tags, "confidence")
	if confidence != "" {
		confScore, err := strconv.ParseFloat(confidence, 64)
		if err != nil || confScore < 0.0 || confScore > 1.0 {
			write("NOTICE", "Invalid confidence score - must be float between 0.0 and 1.0")
			return false
		}
	}

	return true
}

// findExistingAudioEvents finds existing Kind 1063 events from same author with same blossom_hash
func findExistingAudioEvents(store stores.Store, pubkey, blossomHash string) ([]*nostr.Event, error) {
	// Use broad search approach (like we did for Blossom upload) due to tag indexing issues
	filter := nostr.Filter{
		Kinds:   []int{1063},
		Authors: []string{pubkey},
	}

	allEvents, err := store.QueryEvents(filter)
	if err != nil {
		return nil, err
	}

	// Manually filter for events with matching blossom_hash
	var matchingEvents []*nostr.Event
	for _, event := range allEvents {
		eventHash := getTagValue(event.Tags, "blossom_hash")
		if eventHash == blossomHash {
			matchingEvents = append(matchingEvents, event)
		}
	}

	return matchingEvents, nil
}

// Helper functions

func getTagValue(tags nostr.Tags, tagName string) string {
	for _, tag := range tags {
		if len(tag) >= 2 && tag[0] == tagName {
			return tag[1]
		}
	}
	return ""
}

func isValidSHA256(hash string) bool {
	if len(hash) != 64 {
		return false
	}
	matched, _ := regexp.MatchString("^[a-fA-F0-9]{64}$", hash)
	return matched
}

func isValidBlossomURL(url string) bool {
	// Must be HTTPS or HTTP (allow both for flexibility)
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return false
	}

	// More flexible path checking - look for common Blossom patterns
	// Could be /blossom/, /blob/, /file/, or just the hash directly
	blossomPatterns := []string{"/blossom/", "/blob/", "/file/", "/media/", "/upload/"}

	var hash string
	foundPattern := false

	for _, pattern := range blossomPatterns {
		if strings.Contains(url, pattern) {
			parts := strings.Split(url, pattern)
			if len(parts) >= 2 {
				// Take the last part after the pattern
				hash = strings.Split(parts[len(parts)-1], "/")[0] // Remove any trailing path
				hash = strings.Split(hash, "?")[0]                // Remove query parameters
				foundPattern = true
				break
			}
		}
	}

	// If no pattern found, try to extract hash from the end of the URL
	if !foundPattern {
		urlParts := strings.Split(strings.TrimSuffix(url, "/"), "/")
		if len(urlParts) > 0 {
			lastPart := urlParts[len(urlParts)-1]
			lastPart = strings.Split(lastPart, "?")[0] // Remove query parameters
			if len(lastPart) == 64 {                   // SHA-256 length
				hash = lastPart
				foundPattern = true
			}
		}
	}

	// Validate the extracted hash
	if !foundPattern || hash == "" {
		return false
	}

	return isValidSHA256(hash)
}

func isValidLanguageCode(code string) bool {
	// Basic validation for common language codes
	if code == "auto" {
		return true
	}

	// ISO 639-1 codes are 2 characters
	if len(code) == 2 {
		matched, _ := regexp.MatchString("^[a-z]{2}$", code)
		return matched
	}

	// ISO 639-3 codes are 3 characters (also allow these)
	if len(code) == 3 {
		matched, _ := regexp.MatchString("^[a-z]{3}$", code)
		return matched
	}

	return false
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
