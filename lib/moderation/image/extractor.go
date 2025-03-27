package image

import (
	"regexp"
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

// Common image extensions
var imageExtensions = []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg", ".avif"}

// URL extraction regex (basic version)
var urlRegex = regexp.MustCompile(`https?://[^\s<>"']+`)

// ExtractImageURLs extracts image URLs from a Nostr event
func ExtractImageURLs(event *nostr.Event) []string {
	var urls []string
	seen := make(map[string]bool) // Track seen URLs to avoid duplicates

	// Extract from content text
	contentURLs := urlRegex.FindAllString(event.Content, -1)
	for _, url := range contentURLs {
		if !seen[url] && hasImageExtension(url) {
			urls = append(urls, url)
			seen[url] = true
		}
	}

	// Extract from r tags (common in Nostr Build)
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "r" {
			url := tag[1]
			if !seen[url] && hasImageExtension(url) {
				urls = append(urls, url)
				seen[url] = true
			}
		}
	}

	// Extract from imeta tags
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "imeta" {
			for _, value := range tag[1:] {
				if strings.HasPrefix(value, "url ") {
					imageURL := strings.TrimPrefix(value, "url ")
					if !seen[imageURL] {
						urls = append(urls, imageURL)
						seen[imageURL] = true
					}
				}
			}
		}
	}

	return urls
}

// hasImageExtension checks if a URL points to an image
func hasImageExtension(url string) bool {
	url = strings.Split(url, "?")[0] // Remove query parameters
	urlLower := strings.ToLower(url)

	// Check for file extensions
	for _, ext := range imageExtensions {
		if strings.HasSuffix(urlLower, ext) {
			return true
		}
	}

	// Check for common image hosting services that might not have extensions
	imageHostingPatterns := []string{
		"imgur.com",
		"nostr.build/i/",
		"nostr.build/p/",
		"image.nostr.build",
		"i.nostr.build",
		"void.cat",
		"primal.net/",
		"pbs.twimg.com",
		"cloudflare-ipfs.com",
		"i.ibb.co",
	}

	for _, pattern := range imageHostingPatterns {
		if strings.Contains(urlLower, pattern) {
			return true
		}
	}

	return false
}

// HasImages returns true if the event contains any images
func HasImages(event *nostr.Event) bool {
	return len(ExtractImageURLs(event)) > 0
}
