package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// generateChallenge creates a random challenge and its corresponding hash using SHA-256
func generateChallenge() (string, string, error) {
	// Get the current timestamp in RFC3339Nano format
	timestamp := time.Now().Format(time.RFC3339Nano)

	// Generate 16 random bytes
	randomBytes := make([]byte, 16)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", "", err
	}

	// Create the challenge by concatenating the random bytes (as hex string) and the timestamp
	challenge := fmt.Sprintf("%s-%s", hex.EncodeToString(randomBytes), timestamp)

	// Use SHA-256 to hash the challenge
	hash := sha256.Sum256([]byte(challenge))

	// Return the challenge and its SHA-256 hash (as a hex string)
	return challenge, hex.EncodeToString(hash[:]), nil
}
