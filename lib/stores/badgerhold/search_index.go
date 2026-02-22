package badgerhold

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/dgraph-io/badger/v4"
	"github.com/nbd-wtf/go-nostr"
	"github.com/timshannon/badgerhold/v4"
)

// SearchIndexEntry represents a searchable event in the index
type SearchIndexEntry struct {
	EventID   string    `badgerhold:"key"`
	Content   string    // Original content
	Tokens    []string  // Tokenized content for faster searching
	Kind      int       // Event kind
	PubKey    string    // Event author
	CreatedAt time.Time // Event creation time
	UpdatedAt time.Time // Index update time
}

// TokenizeContent breaks content into searchable tokens
func TokenizeContent(content string) []string {
	// Convert to lowercase for case-insensitive search
	content = strings.ToLower(content)

	// Split by whitespace and punctuation
	var tokens []string
	var currentToken strings.Builder

	for _, r := range content {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			currentToken.WriteRune(r)
		} else {
			if currentToken.Len() > 0 {
				token := currentToken.String()
				// Only include tokens with length > 2 to avoid noise
				if len(token) > 2 {
					tokens = append(tokens, token)
				}
				currentToken.Reset()
			}
		}
	}

	// Don't forget the last token
	if currentToken.Len() > 2 {
		tokens = append(tokens, currentToken.String())
	}

	// Remove duplicates
	tokenMap := make(map[string]bool)
	uniqueTokens := []string{}
	for _, token := range tokens {
		if !tokenMap[token] {
			tokenMap[token] = true
			uniqueTokens = append(uniqueTokens, token)
		}
	}

	return uniqueTokens
}

// UpdateSearchIndex adds or updates an event in the search index
func (store *BadgerholdStore) UpdateSearchIndex(event *nostr.Event) error {
	// Only index text events (kind 1) initially
	// Can be expanded to other kinds later
	if event.Kind != 1 {
		return nil
	}

	entry := SearchIndexEntry{
		EventID:   event.ID,
		Content:   event.Content,
		Tokens:    TokenizeContent(event.Content),
		Kind:      event.Kind,
		PubKey:    event.PubKey,
		CreatedAt: event.CreatedAt.Time(),
		UpdatedAt: time.Now(),
	}

	return store.Database.Upsert(event.ID, entry)
}

// RemoveFromSearchIndex removes an event from the search index
func (store *BadgerholdStore) RemoveFromSearchIndex(eventID string) error {
	return store.Database.Delete(eventID, SearchIndexEntry{})
}

// SearchEvents performs an indexed search for events
func (store *BadgerholdStore) SearchEvents(searchTokens []string, limit int) ([]*nostr.Event, error) {
	// First, find matching entries in the search index
	var indexEntries []SearchIndexEntry

	// Build a query that matches any of the search tokens
	var queries []*badgerhold.Query
	for _, token := range searchTokens {
		// Use Contains for partial matching
		queries = append(queries, badgerhold.Where("Tokens").Contains(strings.ToLower(token)))
	}

	// Combine queries with OR logic
	var query *badgerhold.Query
	if len(queries) > 0 {
		query = queries[0]
		for i := 1; i < len(queries); i++ {
			query = query.Or(queries[i])
		}
	}

	// Execute the search
	err := store.Database.Find(&indexEntries, query)
	if err != nil && err != badgerhold.ErrNotFound {
		return nil, fmt.Errorf("search index query failed: %w", err)
	}

	// Now fetch the actual events using raw BadgerDB lookups
	var events []*nostr.Event
	err = store.Database.Badger().View(func(tx *badger.Txn) error {
		for _, entry := range indexEntries {
			ev, err := getEvent(tx, entry.EventID)
			if err == nil {
				events = append(events, ev)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch events: %w", err)
	}

	// Sort by relevance (simple scoring based on token matches)
	// This can be enhanced with more sophisticated ranking
	scoreMap := make(map[string]int)
	for _, event := range events {
		score := 0
		lowerContent := strings.ToLower(event.Content)
		for _, token := range searchTokens {
			score += strings.Count(lowerContent, strings.ToLower(token))
		}
		scoreMap[event.ID] = score
	}

	// Sort events by score (descending)
	sort.Slice(events, func(i, j int) bool {
		return scoreMap[events[i].ID] > scoreMap[events[j].ID]
	})

	// Apply limit
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}

	return events, nil
}

// RebuildSearchIndex rebuilds the entire search index
// This can be run as a maintenance task
func (store *BadgerholdStore) RebuildSearchIndex() error {
	// First, clear the existing index
	err := store.Database.DeleteMatching(SearchIndexEntry{}, &badgerhold.Query{})
	if err != nil {
		return fmt.Errorf("failed to clear search index: %w", err)
	}

	// Iterate all kind-1 events via the eti:1: prefix (raw BadgerDB)
	prefix := []byte(prefixKindTime + "1:")
	count := 0

	err = store.Database.Badger().View(func(tx *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := tx.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			eventID := extractEventIDFromKey(it.Item().KeyCopy(nil))
			ev, err := getEvent(tx, eventID)
			if err != nil {
				continue
			}
			if err := store.UpdateSearchIndex(ev); err != nil {
				fmt.Printf("Failed to index event %s: %v\n", eventID, err)
			}
			count++
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to rebuild search index: %w", err)
	}

	fmt.Printf("Rebuilt search index with %d kind-1 events\n", count)
	return nil
}
