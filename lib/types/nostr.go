// Nostr event and related types
package types

import (
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// NostrEvent represents a Nostr event with additional fields
type NostrEvent struct {
	ID        string `badgerhold:"index"`
	PubKey    string `badgerhold:"index"`
	CreatedAt nostr.Timestamp
	Kind      string `badgerhold:"index"`
	Tags      nostr.Tags
	Content   string
	Sig       string
	// Extra fields for serialization - this field is used indirectly during JSON serialization
	// and shouldn't be removed even though it appears unused in static analysis
	Extra map[string]any `json:"-"` // Fields will be added to the parent object during serialization
}

// TagEntry represents a tag entry for indexing
type TagEntry struct {
	EventID  string `badgerhold:"index"`
	TagName  string `badgerhold:"index"`
	TagValue string `badgerhold:"index"`
}

// Kind represents event kind metadata
type Kind struct {
	ID               uint `gorm:"primaryKey"`
	KindNumber       int
	EventID          string
	TimestampHornets time.Time `gorm:"autoCreateTime"`
	Size             float64
}

// QueryFilter represents filtering options for queries
type QueryFilter struct {
	Names   []string
	PubKeys []string
	Tags    map[string]string
}

// QueryMessage represents a simple query message
type QueryMessage struct {
	QueryFilter map[string]string
}

// AdvancedQueryMessage represents an advanced query with structured filter
type AdvancedQueryMessage struct {
	Filter QueryFilter
}

// QueryResponse represents the response to a query
type QueryResponse struct {
	Hashes []string
}

// ResponseMessage represents a generic response
type ResponseMessage struct {
	Ok bool
}

// ErrorMessage represents an error response
type ErrorMessage struct {
	Message string
}
