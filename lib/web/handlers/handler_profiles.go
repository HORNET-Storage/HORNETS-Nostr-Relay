package handlers

import (
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
)

// ProfilesRequest represents the request structure for getting profiles
type ProfilesRequest struct {
	Pubkeys []string `json:"pubkeys"`
}

// ProfileResponse represents the profile data in the response
type ProfileResponse struct {
	Pubkey   string            `json:"pubkey"`
	Picture  string            `json:"picture"`
	Name     string            `json:"name"`
	About    string            `json:"about"`
	Metadata ProfileMetadata   `json:"metadata"`
}

// ProfileMetadata represents subscription metadata for a profile
type ProfileMetadata struct {
	SubscriptionTier string `json:"subscriptionTier"`
	SubscribedSince  string `json:"subscribedSince"`
}

// ProfilesResponse represents the complete response structure
type ProfilesResponse struct {
	Profiles []ProfileResponse `json:"profiles"`
	NotFound []string          `json:"not_found"`
}

// HandleGetProfiles handles POST /api/profiles requests
func HandleGetProfiles(c *fiber.Ctx, store stores.Store) error {
	var request ProfilesRequest
	if err := c.BodyParser(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate request
	if len(request.Pubkeys) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "At least one pubkey is required",
		})
	}

	if len(request.Pubkeys) > 100 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Maximum 100 pubkeys allowed",
		})
	}

	logging.Info("Processing profiles request", map[string]interface{}{
		"pubkey_count": len(request.Pubkeys),
	})

	// Convert all pubkeys to hex format for consistency
	hexPubkeys := make([]string, 0, len(request.Pubkeys))
	invalidPubkeys := make([]string, 0)

	for _, pubkey := range request.Pubkeys {
		hexPubkey, err := convertToHex(pubkey)
		if err != nil {
			logging.Warn("Invalid pubkey format", map[string]interface{}{
				"pubkey": pubkey,
				"error":  err.Error(),
			})
			invalidPubkeys = append(invalidPubkeys, pubkey)
			continue
		}
		hexPubkeys = append(hexPubkeys, hexPubkey)
	}

	// Get profiles for valid pubkeys
	profiles := make([]ProfileResponse, 0)
	notFound := make([]string, 0)

	for i, hexPubkey := range hexPubkeys {
		originalPubkey := request.Pubkeys[i]
		
		// Get profile metadata from kind 0 events
		profile, found := getProfileForPubkey(store, hexPubkey)
		if !found {
			notFound = append(notFound, originalPubkey)
			continue
		}

		// Get subscription metadata
		metadata := getSubscriptionMetadata(store, hexPubkey)
		profile.Metadata = metadata

		// Use the original pubkey format from the request
		profile.Pubkey = originalPubkey
		profiles = append(profiles, profile)
	}

	// Add invalid pubkeys to not_found
	notFound = append(notFound, invalidPubkeys...)

	response := ProfilesResponse{
		Profiles: profiles,
		NotFound: notFound,
	}

	logging.Info("Profiles request completed", map[string]interface{}{
		"found_count":     len(profiles),
		"not_found_count": len(notFound),
	})

	return c.JSON(response)
}

// convertToHex converts npub or hex pubkey to hex format
func convertToHex(pubkey string) (string, error) {
	// If it looks like an npub, decode it
	if strings.HasPrefix(pubkey, "npub1") {
		_, decoded, err := nip19.Decode(pubkey)
		if err != nil {
			return "", err
		}
		return decoded.(string), nil
	}

	// If it's already hex (64 characters), validate and return
	if len(pubkey) == 64 {
		// Basic validation - check if it's valid hex
		for _, char := range pubkey {
			if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
				return "", fiber.NewError(fiber.StatusBadRequest, "Invalid hex pubkey format")
			}
		}
		return strings.ToLower(pubkey), nil
	}

	return "", fiber.NewError(fiber.StatusBadRequest, "Invalid pubkey format")
}

// getProfileForPubkey gets profile data for a single pubkey
func getProfileForPubkey(store stores.Store, hexPubkey string) (ProfileResponse, bool) {
	profile := ProfileResponse{
		Pubkey:  hexPubkey,
		Picture: "",
		Name:    "",
		About:   "",
	}

	// Query for kind 0 (profile metadata) events
	metadataEvents, err := store.QueryEvents(nostr.Filter{
		Kinds:   []int{0},
		Authors: []string{hexPubkey},
		Limit:   1, // We only need the latest one
	})
	if err != nil {
		logging.Warn("Error querying metadata events", map[string]interface{}{
			"pubkey": hexPubkey,
			"error":  err.Error(),
		})
		return profile, false
	}

	if len(metadataEvents) == 0 {
		return profile, false
	}

	// Parse the metadata from the event content
	var metadata UserMetadata
	if err := json.Unmarshal([]byte(metadataEvents[0].Content), &metadata); err != nil {
		logging.Warn("Error unmarshaling metadata", map[string]interface{}{
			"pubkey": hexPubkey,
			"error":  err.Error(),
		})
		return profile, false
	}

	// Populate profile fields
	if metadata.Picture != "" {
		profile.Picture = metadata.Picture
	}

	// Prefer display_name over name
	if metadata.DisplayName != "" {
		profile.Name = metadata.DisplayName
	} else if metadata.Name != "" {
		profile.Name = metadata.Name
	} else if metadata.DeprecatedDisplayName != "" {
		profile.Name = metadata.DeprecatedDisplayName
	} else if metadata.DeprecatedUsername != "" {
		profile.Name = metadata.DeprecatedUsername
	}

	if metadata.About != "" {
		profile.About = metadata.About
	}

	return profile, true
}

// getSubscriptionMetadata gets subscription information for a pubkey
func getSubscriptionMetadata(store stores.Store, hexPubkey string) ProfileMetadata {
	metadata := ProfileMetadata{
		SubscriptionTier: "",
		SubscribedSince:  "",
	}

	// First try to get from the paid subscribers database
	paidSubscribers, err := store.GetStatsStore().GetPaidSubscribers()
	if err == nil {
		for _, subscriber := range paidSubscribers {
			// Convert subscriber npub to hex for comparison
			subscriberHex, err := convertToHex(subscriber.Npub)
			if err != nil {
				continue
			}
			
			if strings.EqualFold(subscriberHex, hexPubkey) {
				metadata.SubscriptionTier = subscriber.Tier
				metadata.SubscribedSince = subscriber.TimestampHornets.Format("2006-01-02T15:04:05Z")
				return metadata
			}
		}
	}

	// Fallback: Query subscription events (kind 11888)
	subscriptionEvents, err := store.QueryEvents(nostr.Filter{
		Kinds: []int{11888},
		Tags: nostr.TagMap{
			"p":                 []string{hexPubkey},
			"subscription_status": []string{"active"},
		},
		Limit: 1,
	})
	if err != nil {
		logging.Warn("Error querying subscription events", map[string]interface{}{
			"pubkey": hexPubkey,
			"error":  err.Error(),
		})
		return metadata
	}

	if len(subscriptionEvents) > 0 {
		event := subscriptionEvents[0]
		
		// Extract subscription tier from active_subscription tag
		for _, tag := range event.Tags {
			if tag[0] == "active_subscription" && len(tag) > 1 {
				metadata.SubscriptionTier = tag[1]
				break
			}
		}
		
		// Use event creation time as subscribed since
		metadata.SubscribedSince = event.CreatedAt.Time().Format("2006-01-02T15:04:05Z")
	}

	return metadata
}