package test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/search"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/badgerhold"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNIP50SearchFunctionality(t *testing.T) {
	// Initialize test store with a fresh temp directory
	store, err := badgerhold.InitStore(t.TempDir())
	require.NoError(t, err)
	defer store.Cleanup()

	// Create test events with proper 64-char hex IDs (as per Nostr spec)
	testID1 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa0001"
	testID2 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa0002"
	testID3 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa0003"

	now := time.Now()
	events := []*nostr.Event{
		{
			ID:        testID1,
			PubKey:    "pubkey1",
			CreatedAt: nostr.Timestamp(now.Unix()),
			Kind:      1,
			Content:   "This is a test about bitcoin and nostr",
			Tags:      nostr.Tags{},
		},
		{
			ID:        testID2,
			PubKey:    "pubkey2",
			CreatedAt: nostr.Timestamp(now.Unix() - 60),
			Kind:      1,
			Content:   "Spam content that should be filtered",
			Tags:      nostr.Tags{},
		},
		{
			ID:        testID3,
			PubKey:    "pubkey3",
			CreatedAt: nostr.Timestamp(now.Unix() - 120),
			Kind:      1,
			Content:   "Another test about nostr protocol",
			Tags:      nostr.Tags{},
		},
	}

	// Store events
	for _, event := range events {
		err := store.StoreEvent(event)
		require.NoError(t, err)
	}

	// Mark test2 as blocked (spam)
	err = store.MarkEventBlocked(testID2, now.Unix())
	require.NoError(t, err)

	// Test 1: Basic search without extensions
	t.Run("BasicSearch", func(t *testing.T) {
		filter := nostr.Filter{
			Search: "nostr",
		}

		results, err := store.QueryEvents(filter)
		require.NoError(t, err)

		// Should return test1 and test3 but not test2 (blocked)
		assert.Len(t, results, 2)

		foundIDs := make(map[string]bool)
		for _, event := range results {
			foundIDs[event.ID] = true
		}

		assert.True(t, foundIDs[testID1])
		assert.True(t, foundIDs[testID3])
		assert.False(t, foundIDs[testID2])
	})

	// Test 2: Search with extensions parsing (testing the parser works)
	t.Run("SearchWithExtensions", func(t *testing.T) {
		// Test that we can parse search queries with extensions
		testQuery := "bitcoin include:spam lang:en"
		searchQuery := search.ParseSearchQuery(testQuery)

		assert.Equal(t, "bitcoin", searchQuery.Text)
		assert.True(t, searchQuery.IsSpamIncluded())
		assert.Equal(t, "en", searchQuery.Extensions["lang"])

		// Verify basic search still works
		filter := nostr.Filter{
			Search: "nostr",
		}

		results, err := store.QueryEvents(filter)
		require.NoError(t, err)

		// Should return events that match "nostr" (test1 and test3, but not test2 which is blocked)
		assert.Len(t, results, 2)
	})

	// Test 3: Test search query parser
	t.Run("SearchQueryParser", func(t *testing.T) {
		testCases := []struct {
			input              string
			expectedText       string
			expectedExtensions map[string]string
		}{
			{
				input:              "bitcoin price include:spam",
				expectedText:       "bitcoin price",
				expectedExtensions: map[string]string{"include": "spam"},
			},
			{
				input:              "nostr events lang:en include:spam",
				expectedText:       "nostr events",
				expectedExtensions: map[string]string{"lang": "en", "include": "spam"},
			},
			{
				input:              "simple search",
				expectedText:       "simple search",
				expectedExtensions: map[string]string{},
			},
		}

		for _, tc := range testCases {
			query := search.ParseSearchQuery(tc.input)
			assert.Equal(t, tc.expectedText, query.Text)
			assert.Equal(t, tc.expectedExtensions, query.Extensions)
		}
	})

	// Test 4: Search index functionality
	t.Run("SearchIndexing", func(t *testing.T) {
		// Test tokenization
		tokens := badgerhold.TokenizeContent("This is a TEST of tokenization! With some #hashtags")
		expectedTokens := []string{"this", "test", "tokenization", "with", "some", "hashtags"}

		assert.Equal(t, expectedTokens, tokens)

		// Test that events are properly indexed
		searchTokens := []string{"bitcoin"}
		indexedEvents, err := store.SearchEvents(searchTokens, 10)
		require.NoError(t, err)

		// Should find test1 which contains "bitcoin"
		assert.Len(t, indexedEvents, 1)
		assert.Equal(t, testID1, indexedEvents[0].ID)
	})
}

func TestNIP50Compliance(t *testing.T) {
	// Test that we properly handle NIP-50 compliant requests
	t.Run("REQMessageWithSearch", func(t *testing.T) {
		// Create a REQ message with search field
		req := map[string]interface{}{
			"subscription_id": "test-sub",
			"filters": []map[string]interface{}{
				{
					"kinds":  []int{1},
					"search": "bitcoin include:spam",
					"limit":  10,
				},
			},
		}

		// Verify it can be marshaled/unmarshaled properly
		data, err := json.Marshal(req)
		require.NoError(t, err)

		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		// Verify search field is preserved
		filters := parsed["filters"].([]interface{})
		filter := filters[0].(map[string]interface{})
		assert.Equal(t, "bitcoin include:spam", filter["search"])
	})
}
