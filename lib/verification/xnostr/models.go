package xnostr

import (
	"time"
)

// ProfileData represents the extracted Twitter profile data
type ProfileData struct {
	Npub           string `json:"npub,omitempty"`
	FollowerCount  string `json:"follower_count,omitempty"`
	Username       string `json:"username,omitempty"`
	FullName       string `json:"full_name,omitempty"`
	Bio            string `json:"bio,omitempty"`
	Location       string `json:"location,omitempty"`
	Website        string `json:"website,omitempty"`
	JoinDate       string `json:"join_date,omitempty"`
	TweetCount     string `json:"tweet_count,omitempty"`
	FollowingCount string `json:"following_count,omitempty"`
	LikesCount     string `json:"likes_count,omitempty"`
}

// VerificationResult represents the result of a verification attempt
type VerificationResult struct {
	IsVerified         bool      `json:"is_verified"`
	FollowerCount      string    `json:"follower_count"`
	Error              string    `json:"error,omitempty"`
	VerifiedAt         time.Time `json:"verified_at"`
	VerificationSource string    `json:"verification_source,omitempty"` // "bio" or "tweet"
}

// OllamaRequest represents the request structure for Ollama API
type OllamaRequest struct {
	Model  string   `json:"model"`
	Prompt string   `json:"prompt"`
	Stream bool     `json:"stream"`
	Images []string `json:"images,omitempty"`
}

// PendingVerification represents a profile waiting for X-Nostr verification
type PendingVerification struct {
	PubKey    string    `json:"pubkey"`
	XHandle   string    `json:"x_handle"`
	CreatedAt time.Time `json:"created_at"`
	Attempts  int       `json:"attempts"`
}

// CleanXHandle removes the @ symbol from an X handle if present
func CleanXHandle(handle string) string {
	// Remove @ if present
	if len(handle) > 0 && handle[0] == '@' {
		return handle[1:]
	}
	return handle
}
