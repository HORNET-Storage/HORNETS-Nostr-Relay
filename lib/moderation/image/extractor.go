package image

import (
	"regexp"
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

// Common media file extensions
var (
	// Image extensions
	imageExtensions = []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg", ".avif"}

	// Video extensions
	videoExtensions = []string{".mp4", ".webm", ".mov", ".avi", ".mkv", ".m4v", ".ogv", ".mpg", ".mpeg"}

	// Combined for efficiency
	mediaExtensions = append(append([]string{}, imageExtensions...), videoExtensions...)
)

// URL extraction regex (basic version)
var urlRegex = regexp.MustCompile(`https?://[^\s<>"']+`)

// ExtractMediaURLs extracts media (image and video) URLs from a Nostr event
func ExtractMediaURLs(event *nostr.Event) []string {
	var urls []string
	seen := make(map[string]bool) // Track seen URLs to avoid duplicates

	// Extract from content text
	contentURLs := urlRegex.FindAllString(event.Content, -1)
	for _, url := range contentURLs {
		if !seen[url] && hasMediaExtension(url) {
			urls = append(urls, url)
			seen[url] = true
		}
	}

	// Extract from r tags (common in Nostr Build)
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "r" {
			url := tag[1]
			if !seen[url] && hasMediaExtension(url) {
				urls = append(urls, url)
				seen[url] = true
			}
		}
	}

	// Extract from imeta tags
	for _, tag := range event.Tags {
		if len(tag) >= 2 && (tag[0] == "imeta" || tag[0] == "vmeta") {
			for _, value := range tag[1:] {
				if strings.HasPrefix(value, "url ") {
					mediaURL := strings.TrimPrefix(value, "url ")
					if !seen[mediaURL] {
						urls = append(urls, mediaURL)
						seen[mediaURL] = true
					}
				}
			}
		}
	}

	return urls
}

// hasMediaExtension checks if a URL points to a media file (image or video)
func hasMediaExtension(url string) bool {
	url = strings.Split(url, "?")[0] // Remove query parameters
	urlLower := strings.ToLower(url)

	// Check for file extensions
	for _, ext := range mediaExtensions {
		if strings.HasSuffix(urlLower, ext) {
			return true
		}
	}

	// Check for common media hosting services that might not have extensions
	mediaHostingPatterns := []string{
		// Image hosting services
		"imgur.com",
		"nostr.build/i/",
		"nostr.build/p/",
		"image.nostr.build",
		"i.nostr.build",

		// Video hosting services
		"nostr.build/v/",
		"v.nostr.build",
		"video.nostr.build",
		"youtube.com/watch",
		"youtu.be/",
		"vimeo.com/",

		// Generic hosting
		"void.cat",
		"primal.net/",
		"pbs.twimg.com",
		"cloudflare-ipfs.com",
		"i.ibb.co",
	}

	for _, pattern := range mediaHostingPatterns {
		if strings.Contains(urlLower, pattern) {
			return true
		}
	}

	return false
}

// For backward compatibility
func ExtractImageURLs(event *nostr.Event) []string {
	return ExtractMediaURLs(event)
}

// HasMedia returns true if the event contains any media (images or videos)
func HasMedia(event *nostr.Event) bool {
	return len(ExtractMediaURLs(event)) > 0
}

// For backward compatibility
func HasImages(event *nostr.Event) bool {
	return HasMedia(event)
}

// IsVideo determines if a URL likely points to a video
func IsVideo(url string) bool {
	url = strings.Split(url, "?")[0] // Remove query parameters
	urlLower := strings.ToLower(url)

	// Check for video file extensions
	for _, ext := range videoExtensions {
		if strings.HasSuffix(urlLower, ext) {
			return true
		}
	}

	// Check for common video hosting patterns
	videoHostingPatterns := []string{
		"nostr.build/v/",
		"v.nostr.build",
		"video.nostr.build",
		"youtube.com/watch",
		"youtu.be/",
		"vimeo.com/",
	}

	for _, pattern := range videoHostingPatterns {
		if strings.Contains(urlLower, pattern) {
			return true
		}
	}

	return false
}

// IsImage determines if a URL likely points to an image
func IsImage(url string) bool {
	url = strings.Split(url, "?")[0] // Remove query parameters
	urlLower := strings.ToLower(url)

	// Check for image file extensions
	for _, ext := range imageExtensions {
		if strings.HasSuffix(urlLower, ext) {
			return true
		}
	}

	// Check for common image hosting patterns
	imageHostingPatterns := []string{
		"imgur.com",
		"nostr.build/i/",
		"nostr.build/p/",
		"image.nostr.build",
		"i.nostr.build",
	}

	for _, pattern := range imageHostingPatterns {
		if strings.Contains(urlLower, pattern) {
			return true
		}
	}

	return false
}
