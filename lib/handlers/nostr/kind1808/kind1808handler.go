package kind1808

import (
	"strings"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/sync"
	"github.com/gabriel-vasile/mimetype"
	jsoniter "github.com/json-iterator/go"

	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// BuildKind1808Handler creates a handler for kind 1808 (audio notes) events
// Audio notes contain transcriptions in the content field and audio metadata in tags
func BuildKind1808Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
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
		success := lib_nostr.ValidateEvent(write, env, 1808)
		if !success {
			return
		}

		// Validate audio note structure and extract blossom information
		hasAudioURL := false
		hasDuration := false
		var blossomHash string
		var blossomMimeType string
		hasBlossomTag := false

		for _, tag := range env.Event.Tags {
			if tag[0] == "url" && len(tag) >= 2 {
				hasAudioURL = true
			}
			if tag[0] == "duration" && len(tag) >= 2 {
				hasDuration = true
			}
			// Blossom tag format: ["blossom", "hash", "mime/type"]
			if tag[0] == "blossom" && len(tag) >= 3 {
				hasBlossomTag = true
				blossomHash = tag[1]
				blossomMimeType = tag[2]
			}
		}

		if !hasAudioURL {
			write("NOTICE", "Audio note must have a 'url' tag")
			return
		}

		if !hasDuration {
			write("NOTICE", "Audio note must have a 'duration' tag")
			return
		}

		if !hasBlossomTag {
			write("NOTICE", "Audio note must have a 'blossom' tag with hash and mime type")
			return
		}

		// Fetch the blob from storage to validate it exists and check its mime type
		blobData, err := store.GetBlob(blossomHash)
		if err != nil {
			write("NOTICE", "Blossom blob not found in storage. Upload the audio file first.")
			logging.Infof("Failed to retrieve blob %s: %v", blossomHash, err)
			return
		}

		// Detect actual mime type from blob data
		actualMimeType := mimetype.Detect(blobData)
		actualMimeTypeStr := actualMimeType.String()

		// Only accept audio/mp4 format
		if !strings.HasPrefix(actualMimeTypeStr, "audio/mp4") {
			write("NOTICE", "Invalid audio format. Only audio/mp4 (M4A/AAC) is currently accepted")
			logging.Infof("Rejected audio note with invalid mime type: %s (expected audio/mp4)", actualMimeTypeStr)
			return
		}

		// Verify the mime type in the tag matches the actual detected mime type
		tagMimeBase := strings.Split(blossomMimeType, ";")[0]
		if !strings.HasPrefix(actualMimeTypeStr, tagMimeBase) {
			write("NOTICE", "Mime type mismatch between tag and actual blob content")
			logging.Infof("Rejected audio note: tag claims '%s' but blob is '%s'", blossomMimeType, actualMimeTypeStr)
			return
		}

		logging.Infof("Validated audio blob: hash=%s, mime=%s, size=%d bytes", blossomHash, actualMimeTypeStr, len(blobData))

		// Store the new event
		if err := store.StoreEvent(&env.Event); err != nil {
			write("NOTICE", "Failed to store the event")
			return
		}

		// Handle reply sync logic (same as kind 1)
		var replyingToMissingID *string = nil
		var dhtKey *string = nil
		var parentAuthor *string = nil
		for _, tag := range env.Event.Tags {
			if tag[0] == "e" && len(tag) == 3 && tag[2] == "reply" {
				missing := missingEvent(tag[1])
				if missing {
					replyingToMissingID = &tag[1]
				}
			}
			if tag[0] == "dht_key" && len(tag) == 2 {
				dhtKey = &tag[1]
			}
			if tag[0] == "p" && len(tag) == 2 {
				parentAuthor = &tag[1]
			}
		}

		if replyingToMissingID != nil && dhtKey != nil && parentAuthor != nil {
			relayStore := sync.GetRelayStore()
			if relayStore != nil {
				relays, err := relayStore.GetRelayListFromDHT(dhtKey)
				if err != nil {
					logging.Infof("Failed to get relay list: %v", err)
					write("NOTICE", "Failed to get relay list.")
				} else {
					filter := nostr.Filter{Authors: []string{*parentAuthor}}
					for _, relay := range relays {
						relayStore.SyncWithRelay(relay, filter)
						relayStore.AddRelay(relay)
					}
					relayStore.AddAuthor(*parentAuthor)
				}
			} else {
				logging.Info("relay store has not been initialized")
				write("NOTICE", "Relay store has not be initialized")
			}
		} else {
			logging.Info("event tags incomplete, cannot sync")
			logging.Infof("replyToMissingID: %v dhtKey: %v parentAuthor: %v", replyingToMissingID, dhtKey, parentAuthor)
		}

		// Log audio note processing
		logging.Infof("Stored audio note event %s from %s", env.Event.ID, env.Event.PubKey)

		// Successfully processed event
		write("OK", env.Event.ID, true, "Audio note stored successfully")
	}

	return handler
}

func missingEvent(_ string) bool {
	// TODO: Implement proper event existence check
	// For now, assume all referenced events are missing to trigger sync
	return true
}
