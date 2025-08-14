package search

import (
	"regexp"
	"strings"
)

// SearchQuery represents a parsed search query with text and extensions
type SearchQuery struct {
	Text       string            // The main search text (without extensions)
	Extensions map[string]string // Key-value pairs for extensions like include:spam
}

// extensionRegex matches key:value pairs in search strings
var extensionRegex = regexp.MustCompile(`\b(\w+):(\S+)\b`)

// ParseSearchQuery parses a search string into text and extensions
// Example: "best nostr apps include:spam" -> {Text: "best nostr apps", Extensions: {"include": "spam"}}
func ParseSearchQuery(search string) SearchQuery {
	if search == "" {
		return SearchQuery{
			Text:       "",
			Extensions: make(map[string]string),
		}
	}

	query := SearchQuery{
		Extensions: make(map[string]string),
	}

	// Find all extensions in the search string
	matches := extensionRegex.FindAllStringSubmatch(search, -1)

	// Store the original search for text extraction
	remainingText := search

	// Extract extensions and remove them from the search text
	for _, match := range matches {
		if len(match) >= 3 {
			key := strings.ToLower(match[1])
			value := strings.ToLower(match[2])
			query.Extensions[key] = value

			// Remove the extension from the remaining text
			remainingText = strings.Replace(remainingText, match[0], "", 1)
		}
	}

	// Clean up the remaining text
	query.Text = strings.TrimSpace(remainingText)
	// Remove any double spaces that might have been created
	query.Text = regexp.MustCompile(`\s+`).ReplaceAllString(query.Text, " ")

	return query
}

// HasExtension checks if a specific extension exists in the query
func (q *SearchQuery) HasExtension(key string) bool {
	_, exists := q.Extensions[strings.ToLower(key)]
	return exists
}

// GetExtension returns the value of a specific extension
func (q *SearchQuery) GetExtension(key string) (string, bool) {
	value, exists := q.Extensions[strings.ToLower(key)]
	return value, exists
}

// IsSpamIncluded returns true if the search should include spam results
func (q *SearchQuery) IsSpamIncluded() bool {
	value, exists := q.GetExtension("include")
	return exists && value == "spam"
}
