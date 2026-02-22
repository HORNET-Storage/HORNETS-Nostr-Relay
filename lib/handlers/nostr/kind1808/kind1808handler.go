package kind1808

import (
	"strings"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gabriel-vasile/mimetype"
	jsoniter "github.com/json-iterator/go"

	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// Helper function for safe string preview
func previewString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

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

		// Log incoming event details
		logging.Infof("[KIND 1808] Received event ID: %s from pubkey: %s", env.Event.ID, env.Event.PubKey)
		logging.Infof("[KIND 1808] Event content preview: %s", previewString(env.Event.Content, 100))
		logging.Infof("[KIND 1808] Event has %d tags", len(env.Event.Tags))

		// Check relay settings for allowed events whilst also verifying signatures and kind number
		success := lib_nostr.ValidateEvent(write, env, 1808)
		if !success {
			logging.Infof("[KIND 1808] Event %s failed validation", env.Event.ID)
			return
		}

		// Validate audio note structure and extract blossom information
		hasAudioURL := false
		hasDuration := false
		var audioURL string
		var duration string
		var blossomHash string
		var blossomMimeType string
		hasBlossomTag := false

		// Log all tags for debugging
		for i, tag := range env.Event.Tags {
			if len(tag) > 0 {
				logging.Infof("[KIND 1808] Tag %d: type='%s', values=%v", i, tag[0], tag[1:])
			}
		}

		for _, tag := range env.Event.Tags {
			if tag[0] == "url" && len(tag) >= 2 {
				hasAudioURL = true
				audioURL = tag[1]
				logging.Infof("[KIND 1808] Found URL tag: %s", previewString(audioURL, 100))
			}
			if tag[0] == "duration" && len(tag) >= 2 {
				hasDuration = true
				duration = tag[1]
				logging.Infof("[KIND 1808] Found duration tag: %s seconds", duration)
			}
			// Blossom tag format: ["blossom", "hash", "mime/type"]
			if tag[0] == "blossom" && len(tag) >= 3 {
				hasBlossomTag = true
				blossomHash = tag[1]
				blossomMimeType = tag[2]
				logging.Infof("[KIND 1808] Found blossom tag: hash=%s, mime=%s", blossomHash, blossomMimeType)
			}
		}

		if !hasAudioURL {
			logging.Infof("[KIND 1808] Rejected event %s: missing 'url' tag", env.Event.ID)
			write("NOTICE", "Audio note must have a 'url' tag")
			return
		}

		if !hasDuration {
			logging.Infof("[KIND 1808] Rejected event %s: missing 'duration' tag", env.Event.ID)
			write("NOTICE", "Audio note must have a 'duration' tag")
			return
		}

		if !hasBlossomTag {
			logging.Infof("[KIND 1808] Rejected event %s: missing 'blossom' tag", env.Event.ID)
			write("NOTICE", "Audio note must have a 'blossom' tag with hash and mime type")
			return
		}

		// Fetch the blob from storage to validate it exists and check its mime type
		logging.Infof("[KIND 1808] Fetching blob with hash: %s", blossomHash)
		blobData, err := store.GetBlob(blossomHash)
		if err != nil {
			logging.Infof("[KIND 1808] Failed to retrieve blob %s: %v", blossomHash, err)
			write("NOTICE", "Blossom blob not found in storage. Upload the audio file first.")
			return
		}
		logging.Infof("[KIND 1808] Successfully retrieved blob, size: %d bytes", len(blobData))

		// Detect actual mime type from blob data
		actualMimeType := mimetype.Detect(blobData)
		actualMimeTypeStr := actualMimeType.String()
		logging.Infof("[KIND 1808] Detected mime type: %s (tag claims: %s)", actualMimeTypeStr, blossomMimeType)

		// Accept MP4 format (both audio/mp4 and video/mp4 as MP4 is just a container)
		isValidMP4 := strings.HasPrefix(actualMimeTypeStr, "audio/mp4") || strings.HasPrefix(actualMimeTypeStr, "video/mp4")
		if !isValidMP4 {
			logging.Infof("[KIND 1808] Rejected event %s: detected mime type %s is not MP4", env.Event.ID, actualMimeTypeStr)
			write("NOTICE", "Invalid format. Only MP4 audio files are accepted")
			return
		}

		// Verify the blossom tag also specifies MP4 (either audio/mp4 or video/mp4)
		tagMimeBase := strings.Split(blossomMimeType, ";")[0]
		isTagMP4 := tagMimeBase == "audio/mp4" || tagMimeBase == "video/mp4"
		if !isTagMP4 {
			logging.Infof("[KIND 1808] Rejected event %s: tag mime type %s is not MP4", env.Event.ID, tagMimeBase)
			write("NOTICE", "Blossom tag must specify MP4 format (audio/mp4 or video/mp4)")
			return
		}

		logging.Infof("[KIND 1808] ✓ Validated audio blob: hash=%s, detected=%s, tag=%s, size=%d bytes",
			blossomHash, actualMimeTypeStr, blossomMimeType, len(blobData))

		// Store the new event
		if err := store.StoreEvent(&env.Event); err != nil {
			logging.Infof("[KIND 1808] Failed to store event %s: %v", env.Event.ID, err)
			write("NOTICE", "Failed to store the event")
			return
		}

		// Log audio note processing
		logging.Infof("[KIND 1808] ✅ Successfully stored event %s from pubkey %s", env.Event.ID, env.Event.PubKey)

		// Successfully processed event
		write("OK", env.Event.ID, true, "Audio note stored successfully")
	}

	return handler
}
