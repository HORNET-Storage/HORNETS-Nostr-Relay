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

// OllamaRequest represents the request structure for Ollama API
type OllamaRequest struct {
	Model     string `json:"model"`
	Prompt    string `json:"prompt"`
	Stream    bool   `json:"stream"`
	MaxTokens int    `json:"max_tokens,omitempty"`
}

// OllamaResponse represents the response structure from Ollama API
type OllamaResponse struct {
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	Response  string `json:"response"`
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

// BuildPrompt creates a prompt that combines the custom instruction with clear guidance
func BuildPrompt(content string, customInstruction string) string {
	// If custom instruction is empty, use a default
	if customInstruction == "" {
		customInstruction = DefaultFilterInstructions
	}

	// Create the full prompt with clear instructions on how to respond
	return `
` + customInstruction + `

Content to evaluate: "` + content + `"

IMPORTANT RESPONSE INSTRUCTIONS:
- Your response must be ONLY "true" or "false" without any additional text, explanation, or quotation marks.
- Respond with "true" if the content should appear in the user's feed according to the instructions.
- Respond with "false" if it should be filtered out.
- Do not include any reasoning, explanations, or other text in your response.
- The reason for your decision will be extracted separately, focus only on the true/false response.`
}
