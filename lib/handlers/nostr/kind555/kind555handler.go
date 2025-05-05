package kind555

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/verification/xnostr"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/nbd-wtf/go-nostr"
)

// XNostrVerifier handles X-Nostr verification events (kind 555)
type XNostrVerifier struct {
	store         stores.Store
	xnostrService *xnostr.Service
}

// NewXNostrVerifier creates a new X-Nostr verifier
func NewXNostrVerifier(store stores.Store, xnostrService *xnostr.Service) *XNostrVerifier {
	return &XNostrVerifier{
		store:         store,
		xnostrService: xnostrService,
	}
}

// CreateVerificationEvent creates a kind 555 verification event for a user
func (v *XNostrVerifier) CreateVerificationEvent(
	pubkey string,
	xHandle string,
	isVerified bool,
	followerCount string,
	verificationSource string,
	relayPrivKey *btcec.PrivateKey,
) (*nostr.Event, error) {
	// Create the event content
	content := map[string]interface{}{
		"pubkey":              pubkey,
		"x_handle":            xHandle,
		"verified":            isVerified,
		"follower_count":      followerCount,
		"verified_at":         time.Now().Format(time.RFC3339),
		"verification_source": verificationSource,
	}

	// Convert to JSON
	contentJSON, err := json.Marshal(content)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal content: %w", err)
	}

	// Create tags for easy querying
	tags := nostr.Tags{
		{"p", pubkey},  // Reference to the user's pubkey
		{"x", xHandle}, // The X handle
		{"verified", fmt.Sprintf("%t", isVerified)}, // Verification status
		{"verification_source", verificationSource}, // Verification source (bio or tweet)
	}

	// Create the event
	event := &nostr.Event{
		PubKey:    hex.EncodeToString(relayPrivKey.PubKey().SerializeCompressed()),
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      555,
		Tags:      tags,
		Content:   string(contentJSON),
	}

	// Sign the event
	hash := sha256.Sum256(event.Serialize())
	sig, err := schnorr.Sign(relayPrivKey, hash[:])
	if err != nil {
		return nil, fmt.Errorf("error signing event: %v", err)
	}
	event.ID = hex.EncodeToString(hash[:])
	event.Sig = hex.EncodeToString(sig.Serialize())

	// Store the event
	if err := v.store.StoreEvent(event); err != nil {
		return nil, fmt.Errorf("failed to store event: %w", err)
	}

	return event, nil
}

// GetVerificationForUser gets the latest verification event for a user
func (v *XNostrVerifier) GetVerificationForUser(pubkey string) (*nostr.Event, error) {
	// Create a filter to find kind 555 events for this user
	filter := nostr.Filter{
		Kinds: []int{555},
		Tags: map[string][]string{
			"p": {pubkey},
		},
		Limit: 1,
	}

	// Query the store
	events, err := v.store.QueryEvents(filter)
	if err != nil {
		return nil, err
	}

	// Check if we found any events
	if len(events) == 0 {
		return nil, fmt.Errorf("no verification event found for pubkey %s", pubkey)
	}

	return events[0], nil
}

// UpdateVerification updates the verification status for a user
func (v *XNostrVerifier) UpdateVerification(pubkey string, relayPrivKey *btcec.PrivateKey) (*nostr.Event, error) {
	// Get the user's profile using QueryEvents
	filter := nostr.Filter{
		Authors: []string{pubkey},
		Kinds:   []int{0},
		Limit:   1,
	}

	events, err := v.store.QueryEvents(filter)
	if err != nil {
		return nil, fmt.Errorf("failed to query profile: %w", err)
	}

	if len(events) == 0 {
		return nil, fmt.Errorf("no profile found for pubkey %s", pubkey)
	}

	event := events[0]

	// Parse the profile content
	var content map[string]interface{}
	if err := json.Unmarshal([]byte(event.Content), &content); err != nil {
		return nil, fmt.Errorf("invalid profile JSON: %w", err)
	}

	// Get the X handle
	xHandleRaw, ok := content["x"]
	if !ok {
		return nil, fmt.Errorf("no X handle in profile")
	}

	var xHandle string
	switch v := xHandleRaw.(type) {
	case string:
		xHandle = cleanXHandle(v)
	default:
		return nil, fmt.Errorf("invalid X handle type")
	}

	// Verify the X profile
	verificationResult, err := v.xnostrService.VerifyProfile(pubkey, xHandle)
	if err != nil {
		log.Printf("Error verifying X profile: %v", err)
		// Continue anyway to create the event with verification failed
		verificationResult = &xnostr.VerificationResult{
			IsVerified:    false,
			FollowerCount: "0",
			Error:         err.Error(),
		}
	}

	// Create the verification event
	return v.CreateVerificationEvent(
		pubkey,
		xHandle,
		verificationResult.IsVerified,
		verificationResult.FollowerCount,
		verificationResult.VerificationSource,
		relayPrivKey,
	)
}

// IsUserVerified checks if a user has a verified X profile
func (v *XNostrVerifier) IsUserVerified(pubkey string) (bool, error) {
	event, err := v.GetVerificationForUser(pubkey)
	if err != nil {
		if err.Error() == fmt.Sprintf("no verification event found for pubkey %s", pubkey) {
			return false, nil
		}
		return false, err
	}

	// Parse the content
	var content map[string]interface{}
	if err := json.Unmarshal([]byte(event.Content), &content); err != nil {
		return false, fmt.Errorf("invalid event content: %w", err)
	}

	// Check if verified
	verified, ok := content["verified"].(bool)
	if !ok {
		return false, fmt.Errorf("invalid verified field in event content")
	}

	return verified, nil
}

// GetFollowerCount gets the follower count for a user
func (v *XNostrVerifier) GetFollowerCount(pubkey string) (string, error) {
	event, err := v.GetVerificationForUser(pubkey)
	if err != nil {
		return "", err
	}

	// Parse the content
	var content map[string]interface{}
	if err := json.Unmarshal([]byte(event.Content), &content); err != nil {
		return "", fmt.Errorf("invalid event content: %w", err)
	}

	// Get follower count
	followerCount, ok := content["follower_count"].(string)
	if !ok {
		return "", fmt.Errorf("invalid follower_count field in event content")
	}

	return followerCount, nil
}

// cleanXHandle removes the @ symbol from an X handle if present
func cleanXHandle(handle string) string {
	// Remove @ if present
	if len(handle) > 0 && handle[0] == '@' {
		return handle[1:]
	}
	return handle
}
