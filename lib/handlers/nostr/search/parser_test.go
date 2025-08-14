package search

import (
	"testing"
)

func TestParseSearchQuery(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		expectedText       string
		expectedExtensions map[string]string
	}{
		{
			name:               "Simple search without extensions",
			input:              "best nostr apps",
			expectedText:       "best nostr apps",
			expectedExtensions: map[string]string{},
		},
		{
			name:               "Search with include:spam extension",
			input:              "best nostr apps include:spam",
			expectedText:       "best nostr apps",
			expectedExtensions: map[string]string{"include": "spam"},
		},
		{
			name:         "Multiple extensions",
			input:        "bitcoin price:high include:spam lang:en",
			expectedText: "bitcoin",
			expectedExtensions: map[string]string{
				"price":   "high",
				"include": "spam",
				"lang":    "en",
			},
		},
		{
			name:               "Extension in the middle of text",
			input:              "search for include:spam nostr events",
			expectedText:       "search for nostr events",
			expectedExtensions: map[string]string{"include": "spam"},
		},
		{
			name:               "Empty search string",
			input:              "",
			expectedText:       "",
			expectedExtensions: map[string]string{},
		},
		{
			name:         "Only extensions",
			input:        "include:spam lang:en",
			expectedText: "",
			expectedExtensions: map[string]string{
				"include": "spam",
				"lang":    "en",
			},
		},
		{
			name:         "Case insensitive extensions",
			input:        "test INCLUDE:SPAM Lang:EN",
			expectedText: "test",
			expectedExtensions: map[string]string{
				"include": "spam",
				"lang":    "en",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseSearchQuery(tt.input)

			if result.Text != tt.expectedText {
				t.Errorf("Expected text '%s', got '%s'", tt.expectedText, result.Text)
			}

			if len(result.Extensions) != len(tt.expectedExtensions) {
				t.Errorf("Expected %d extensions, got %d", len(tt.expectedExtensions), len(result.Extensions))
			}

			for key, expectedValue := range tt.expectedExtensions {
				if value, exists := result.Extensions[key]; !exists || value != expectedValue {
					t.Errorf("Expected extension %s:%s, got %s:%s", key, expectedValue, key, value)
				}
			}
		})
	}
}

func TestSearchQueryMethods(t *testing.T) {
	query := ParseSearchQuery("nostr events include:spam lang:en")

	// Test HasExtension
	if !query.HasExtension("include") {
		t.Error("Expected HasExtension('include') to return true")
	}

	if !query.HasExtension("INCLUDE") {
		t.Error("Expected HasExtension('INCLUDE') to return true (case insensitive)")
	}

	if query.HasExtension("nonexistent") {
		t.Error("Expected HasExtension('nonexistent') to return false")
	}

	// Test GetExtension
	value, exists := query.GetExtension("lang")
	if !exists || value != "en" {
		t.Errorf("Expected GetExtension('lang') to return 'en', true; got '%s', %v", value, exists)
	}

	// Test IsSpamIncluded
	if !query.IsSpamIncluded() {
		t.Error("Expected IsSpamIncluded() to return true")
	}

	// Test without spam
	query2 := ParseSearchQuery("nostr events")
	if query2.IsSpamIncluded() {
		t.Error("Expected IsSpamIncluded() to return false when no include:spam")
	}
}
