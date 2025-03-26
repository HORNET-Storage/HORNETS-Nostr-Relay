package contentmoderation

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// MediaURLPattern defines a pattern and extraction rule for a media URL
type MediaURLPattern struct {
	// Name is a human-readable name for this pattern (e.g., "scionic", "primal")
	Name string

	// Pattern is the regex pattern to match this URL type
	Pattern *regexp.Regexp

	// MediaIDGroup is the regex capture group number for the media ID/hash
	MediaIDGroup int
}

// Common URL patterns for media in Nostr events
// These can be extended as needed for additional hosting services
var mediaURLPatterns = []MediaURLPattern{
	{
		Name:         "scionic",
		Pattern:      regexp.MustCompile(`https?://[^/]+/download/([a-f0-9]+)`),
		MediaIDGroup: 1,
	},
	{
		Name:         "primal",
		Pattern:      regexp.MustCompile(`https?://m\.primal\.net/([A-Za-z0-9]+\.(jpg|jpeg|png|gif|webp|mp4))`),
		MediaIDGroup: 1,
	},
	{
		Name:         "nostr_build",
		Pattern:      regexp.MustCompile(`https?://i\.nostr\.build/([A-Za-z0-9]+\.(jpg|jpeg|png|gif|webp|mp4))`),
		MediaIDGroup: 1,
	},
	{
		Name:         "standard_url",
		Pattern:      regexp.MustCompile(`https?://[^/\s]+/\S+\.(jpg|jpeg|png|gif|webp|mp4|webm|mov)\b`),
		MediaIDGroup: 0, // Use full URL as ID for general case
	},
}

// ExtractMediaReferences analyzes a Nostr event to find all media references
// It returns a slice of MediaReferenceInfo with details about each reference
func ExtractMediaReferences(event *nostr.Event) []MediaReferenceInfo {
	if event == nil {
		return nil
	}

	var references []MediaReferenceInfo

	// 1. Check event content for media URLs
	references = append(references, extractMediaURLsFromText(event.Content)...)

	// 2. Check for image/media tags
	references = append(references, extractMediaFromTags(event.Tags)...)

	// Debug logging
	if len(references) > 0 {
		log.Printf("Found %d media references in event %s", len(references), event.ID)
	}

	return references
}

// extractMediaURLsFromText finds media URLs in text content
func extractMediaURLsFromText(content string) []MediaReferenceInfo {
	var references []MediaReferenceInfo

	// Apply each URL pattern to find media references
	for _, urlPattern := range mediaURLPatterns {
		matches := urlPattern.Pattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) > urlPattern.MediaIDGroup {
				mediaURL := match[0] // Full URL

				// For standard URLs, use a hash of the URL as the ID
				mediaID := match[urlPattern.MediaIDGroup]
				if urlPattern.Name == "standard_url" {
					// Get the last part of the URL as the ID
					parts := strings.Split(mediaURL, "/")
					if len(parts) > 0 {
						mediaID = parts[len(parts)-1]
					} else {
						// If we can't extract a good ID, hash the URL
						mediaID = hashString(mediaURL)
					}
				}

				references = append(references, MediaReferenceInfo{
					MediaURL:   mediaURL,
					MediaID:    mediaID,
					Type:       "content_url",
					SourceType: urlPattern.Name,
					Metadata:   make(map[string]string),
				})
			}
		}
	}

	return references
}

// extractMediaFromTags extracts media references from event tags
func extractMediaFromTags(tags nostr.Tags) []MediaReferenceInfo {
	var references []MediaReferenceInfo

	for _, tag := range tags {
		if len(tag) < 2 {
			continue
		}

		tagName := tag[0]

		// Process "imeta" tags (common for Nostr clients with metadata)
		if tagName == "imeta" && len(tag) >= 2 {
			// Extract URL and metadata from imeta tag
			metadata := make(map[string]string)
			var mediaURL string

			for _, field := range tag[1:] {
				if strings.HasPrefix(field, "url ") {
					mediaURL = strings.TrimPrefix(field, "url ")
				} else if strings.HasPrefix(field, "blurhash ") {
					metadata["blurhash"] = strings.TrimPrefix(field, "blurhash ")
				} else if strings.HasPrefix(field, "dim ") {
					metadata["dimensions"] = strings.TrimPrefix(field, "dim ")
				}
			}

			if mediaURL != "" {
				mediaID := extractMediaIDFromURL(mediaURL)
				references = append(references, MediaReferenceInfo{
					MediaURL:   mediaURL,
					MediaID:    mediaID,
					Type:       "imeta_tag",
					SourceType: detectSourceType(mediaURL),
					Metadata:   metadata,
				})
			}
		}

		// Process "r" tags that often point to media
		if tagName == "r" && isLikelyMediaURL(tag[1]) {
			mediaURL := tag[1]
			mediaID := extractMediaIDFromURL(mediaURL)
			references = append(references, MediaReferenceInfo{
				MediaURL:   mediaURL,
				MediaID:    mediaID,
				Type:       "r_tag",
				SourceType: detectSourceType(mediaURL),
				Metadata:   make(map[string]string),
			})
		}

		// Process direct "image" or "media" tags
		if (tagName == "image" || tagName == "media") && len(tag) >= 2 {
			mediaURL := tag[1]
			mediaID := extractMediaIDFromURL(mediaURL)
			references = append(references, MediaReferenceInfo{
				MediaURL:   mediaURL,
				MediaID:    mediaID,
				Type:       tagName + "_tag",
				SourceType: detectSourceType(mediaURL),
				Metadata:   make(map[string]string),
			})
		}
	}

	return references
}

// extractMediaIDFromURL extracts a media ID from a URL
func extractMediaIDFromURL(url string) string {
	// Try each pattern to extract the media ID
	for _, pattern := range mediaURLPatterns {
		match := pattern.Pattern.FindStringSubmatch(url)
		if match != nil && len(match) > pattern.MediaIDGroup {
			if pattern.Name == "standard_url" {
				// For standard URLs, use a hash of the URL as the ID
				parts := strings.Split(url, "/")
				if len(parts) > 0 {
					return parts[len(parts)-1]
				}
				return hashString(url)
			}
			return match[pattern.MediaIDGroup]
		}
	}

	// If no pattern matches, extract the filename from the URL
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	// If all else fails, hash the URL
	return hashString(url)
}

// detectSourceType determines the source of a media URL
func detectSourceType(url string) string {
	for _, pattern := range mediaURLPatterns {
		if pattern.Pattern.MatchString(url) {
			return pattern.Name
		}
	}
	return "unknown"
}

// isLikelyMediaURL checks if a URL is likely a media URL
func isLikelyMediaURL(url string) bool {
	// Check if URL matches any of our media patterns
	for _, pattern := range mediaURLPatterns {
		if pattern.Pattern.MatchString(url) {
			return true
		}
	}

	// Check for common image/video extensions
	lowerURL := strings.ToLower(url)
	mediaExtensions := []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".mp4", ".webm", ".mov"}
	for _, ext := range mediaExtensions {
		if strings.HasSuffix(lowerURL, ext) {
			return true
		}
	}

	// Check for common media hosting domains that might not follow usual patterns
	mediaHosts := []string{"primal.net", "nostr.build", "imgur.com", "cloudinary.com", "giphy.com"}
	for _, host := range mediaHosts {
		if strings.Contains(lowerURL, host) {
			return true
		}
	}

	return false
}

// hashString creates a SHA-256 hash of a string
func hashString(s string) string {
	hash := sha256.Sum256([]byte(s))
	return hex.EncodeToString(hash[:])
}

// ProcessEventMediaReferences analyzes an event for media references and checks their moderation status
// Returns true if the event is safe to deliver, false if it should be filtered
// If awaitingModeration is true, the event contains media that is awaiting moderation
func ProcessEventMediaReferences(service *Service, event *nostr.Event) (safe bool, awaitingModeration bool, err error) {
	if event == nil {
		return true, false, nil
	}

	// Extract media references from the event
	references := ExtractMediaReferences(event)
	if len(references) == 0 {
		return true, false, nil // No media references, event is safe
	}

	// Check the status of each reference
	safe = true
	for _, ref := range references {
		// For scionic media, check internal moderation status
		if ref.SourceType == "scionic" {
			record, err := service.store.GetMediaRecord(ref.MediaID)
			if err != nil {
				// If we can't find the record, continue but log the error
				log.Printf("Error checking media status for %s: %v", ref.MediaID, err)
				continue
			}

			// Check the status
			switch record.Status {
			case StatusRejected, StatusDeleted:
				// Media is rejected, event is not safe
				log.Printf("Event %s references rejected media %s", event.ID, ref.MediaID)
				safe = false
				return false, false, nil // Return immediately for rejected media
			case StatusAwaiting, StatusProcessing:
				// Media is awaiting moderation
				awaitingModeration = true
			case StatusApproved:
				// Media is approved, continue checking other references
			}

			// Save the reference for future lookups
			reference := &EventMediaReference{
				EventID:     event.ID,
				MediaURL:    ref.MediaURL,
				MediaID:     ref.MediaID,
				SourceType:  ref.SourceType,
				ReferenceAt: time.Unix(int64(event.CreatedAt), 0),
			}
			if err := service.store.SaveEventMediaReference(reference); err != nil {
				log.Printf("Error saving media reference: %v", err)
			}
		} else if service.config.CheckExternalMedia {
			// For external media, check if we already have cached results
			cache, err := service.store.GetExternalMediaCache(ref.MediaURL)
			if err == nil {
				// We have cached results
				if ContentStatus(cache.Status) == StatusRejected || ContentStatus(cache.Status) == StatusDeleted {
					// Media is rejected, event is not safe
					log.Printf("Event %s references rejected external media %s", event.ID, ref.MediaURL)
					safe = false
					return false, false, nil // Return immediately for rejected media
				}
			} else {
				// No cached results, mark as awaiting moderation
				// Note: We don't actually fetch and analyze external media here
				// That would be done by a separate background process
				awaitingModeration = true
			}

			// Save the reference for future processing
			reference := &EventMediaReference{
				EventID:     event.ID,
				MediaURL:    ref.MediaURL,
				MediaID:     ref.MediaID,
				SourceType:  ref.SourceType,
				ReferenceAt: time.Unix(int64(event.CreatedAt), 0),
			}
			if err := service.store.SaveEventMediaReference(reference); err != nil {
				log.Printf("Error saving media reference: %v", err)
			}
		}
	}

	// Update event status based on findings
	if safe && !awaitingModeration {
		// All media is approved
		service.store.MarkEventStatus(event.ID, EventStatusSafe)
	} else if !safe {
		// Some media is rejected
		service.store.MarkEventStatus(event.ID, EventStatusUnsafe)
	} else if awaitingModeration {
		// Some media is awaiting moderation
		service.store.MarkEventStatus(event.ID, EventStatusAwaiting)
	}

	return safe, awaitingModeration, nil
}
