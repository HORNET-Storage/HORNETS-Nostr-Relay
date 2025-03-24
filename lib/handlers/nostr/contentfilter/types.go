package contentfilter

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// FilterResult represents the result of filtering an event
type FilterResult struct {
	Pass   bool   `json:"pass"`
	Reason string `json:"reason"`
}

// FilterRequest represents the structured request format for Nest Feeder
type FilterRequest struct {
	CustomInstruction string      `json:"custom_instruction"`
	EventData         interface{} `json:"event_data"`
}

// CacheItem represents a cached filter result with expiration
type CacheItem struct {
	Result    FilterResult
	Timestamp time.Time
}

// DefaultFilterInstructions provides a sensible default for users
const DefaultFilterInstructions = `I want to filter my Nostr feed based on these preferences:
Rule 1: Include content about technology, programming, and cryptocurrency.
Rule 2: Filter out offensive content and spam.
Rule 3: Include thoughtful discussions, even if controversial, as long as they're respectful.`

// GenerateInstructionsHash creates a hash from filter instructions for cache keys
func GenerateInstructionsHash(instructions string) string {
	hash := sha256.Sum256([]byte(instructions))
	return hex.EncodeToString(hash[:])
}
