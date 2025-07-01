package handlers

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/nbd-wtf/go-nostr"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
)

// UserMetadata represents the content of kind 0 events
type UserMetadata struct {
	Name        string `json:"name,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	About       string `json:"about,omitempty"`
	Picture     string `json:"picture,omitempty"`

	// New fields from NIP-24
	Website  string `json:"website,omitempty"`
	Banner   string `json:"banner,omitempty"`
	Bot      bool   `json:"bot,omitempty"`
	Birthday struct {
		Year  int `json:"year,omitempty"`
		Month int `json:"month,omitempty"`
		Day   int `json:"day,omitempty"`
	} `json:"birthday,omitempty"`

	// Deprecated fields (for backward compatibility)
	DeprecatedDisplayName string `json:"displayName,omitempty"`
	DeprecatedUsername    string `json:"username,omitempty"`
}

// PaidSubscriberProfile represents the response structure
type PaidSubscriberProfile struct {
	Pubkey  string `json:"pubkey"`
	Picture string `json:"picture"`
	Name    string `json:"name,omitempty"`
	About   string `json:"about,omitempty"`

	// Additional NIP-24 fields
	Website  string `json:"website,omitempty"`
	Banner   string `json:"banner,omitempty"`
	Bot      bool   `json:"bot,omitempty"`
	Birthday struct {
		Year  int `json:"year,omitempty"`
		Month int `json:"month,omitempty"`
		Day   int `json:"day,omitempty"`
	} `json:"birthday,omitempty"`
}

// TODO: Update this URL once Blossom blob images are implemented, as we will have migrated away from using links
// Placeholder avatar URL to use when a subscriber doesn't have a picture
const placeholderAvatarURL = "http://localhost:3000/placeholder-avatar.png"

// HandleGetPaidSubscriberProfiles gets profile pictures of paid subscribers
func HandleGetPaidSubscriberProfiles(c *fiber.Ctx, store stores.Store) error {
	log.Printf("Fetching paid subscriber profiles")

	// First, try to get paid subscribers from the statistics store
	paidSubscribers, err := store.GetStatsStore().GetPaidSubscribers()
	if err != nil {
		log.Printf("Error fetching paid subscribers from database, falling back to events: %v", err)

		// Fall back to the original method
		return GetSubscribersFromEvents(c, store)
	}

	// If we have subscribers in the database, use them
	if len(paidSubscribers) > 0 {
		log.Printf("Found %d paid subscribers in database", len(paidSubscribers))

		// Collect the pubkeys of all paid subscribers
		pubkeys := make([]string, 0, len(paidSubscribers))
		for _, sub := range paidSubscribers {
			pubkeys = append(pubkeys, sub.Npub)
		}

		// Get profile pictures for each subscriber
		profiles, err := GetProfilesForPubkeys(store, pubkeys)
		if err != nil {
			log.Printf("Error getting profiles: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to fetch profiles",
			})
		}

		log.Printf("Returning %d profiles with pictures", len(profiles))
		// Log the response structure to help with panel-side implementation
		if len(profiles) > 0 {
			exampleJSON, _ := json.MarshalIndent(profiles[0], "", "  ")
			log.Printf("Example profile response structure: %s", exampleJSON)
		}
		return c.JSON(profiles)
	}

	// If no results from database, fall back to the original method
	log.Printf("No paid subscribers found in database, falling back to events")
	return GetSubscribersFromEvents(c, store)
}

// getProfilesForPubkeys gets profile pictures for a list of public keys
func GetProfilesForPubkeys(store stores.Store, pubkeys []string) ([]PaidSubscriberProfile, error) {
	profiles := make([]PaidSubscriberProfile, 0)

	for _, pubkey := range pubkeys {
		// Create a profile entry for this subscriber with default values
		profile := PaidSubscriberProfile{
			Pubkey:  pubkey,
			Picture: placeholderAvatarURL, // Default to placeholder
			Name:    "",                   // Empty by default
			About:   "",                   // Empty by default
		}

		metadataEvents, err := store.QueryEvents(nostr.Filter{
			Kinds:   []int{0},
			Authors: []string{pubkey},
			Limit:   1, // We only need the latest one
		})
		if err != nil {
			log.Printf("Error querying metadata for pubkey %s: %v", pubkey, err)
			// Still include the profile with the placeholder avatar
			profiles = append(profiles, profile)
			continue
		}

		if len(metadataEvents) > 0 {
			var metadata UserMetadata
			if err := json.Unmarshal([]byte(metadataEvents[0].Content), &metadata); err != nil {
				log.Printf("Error unmarshaling metadata for pubkey %s: %v", pubkey, err)
				// Still include the profile with the placeholder avatar
				profiles = append(profiles, profile)
				continue
			}

			// Populate fields from metadata
			if metadata.Picture != "" {
				profile.Picture = metadata.Picture
			}

			// Check for display_name first, then fall back to name if not present
			if metadata.DisplayName != "" {
				profile.Name = metadata.DisplayName
			} else if metadata.Name != "" {
				profile.Name = metadata.Name
			} else if metadata.DeprecatedDisplayName != "" {
				// Fall back to deprecated displayName if no other name is available
				profile.Name = metadata.DeprecatedDisplayName
			} else if metadata.DeprecatedUsername != "" {
				// Fall back to deprecated username as last resort
				profile.Name = metadata.DeprecatedUsername
			}

			if metadata.About != "" {
				profile.About = metadata.About
			}

			// Populate additional NIP-24 fields in the profile response
			profile.Website = metadata.Website
			profile.Banner = metadata.Banner
			profile.Bot = metadata.Bot
			profile.Birthday = metadata.Birthday

			// Log NIP-24 fields for debugging
			if metadata.Website != "" || metadata.Banner != "" || metadata.Bot ||
				(metadata.Birthday.Year != 0 || metadata.Birthday.Month != 0 || metadata.Birthday.Day != 0) {
				log.Printf("NIP-24 fields for %s: Website=%s, Banner=%s, Bot=%v, Birthday=%v-%v-%v",
					pubkey, metadata.Website, metadata.Banner, metadata.Bot,
					metadata.Birthday.Year, metadata.Birthday.Month, metadata.Birthday.Day)
			}
		}

		// Always add the profile
		profiles = append(profiles, profile)
	}

	return profiles, nil
}

// getSubscribersFromEvents gets paid subscribers using the original event-based method
func GetSubscribersFromEvents(c *fiber.Ctx, store stores.Store) error {
	// First, get relay settings to determine free tier status
	settings, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("error getting config: %v", err)
	}

	// Step 1: Get all kind 888 events (subscription events)
	subscriptionEvents, err := store.QueryEvents(nostr.Filter{
		Kinds: []int{888},
		Tags: nostr.TagMap{
			"subscription_status": []string{"active"},
		},
	})
	if err != nil {
		log.Printf("Error querying subscription events: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch subscribers",
		})
	}

	// Filter out free tier subscribers
	paidSubscribers := make([]string, 0)
	for _, event := range subscriptionEvents {
		// Get the subscription tier from the active_subscription tag
		var subscriptionTier string
		for _, tag := range event.Tags {
			if tag[0] == "active_subscription" && len(tag) > 1 {
				subscriptionTier = tag[1]
				break
			}
		}

		// Skip if no subscription tier found
		if subscriptionTier == "" {
			continue
		}

		// Skip if it matches a free tier (price = "0")
		isFreeTier := false
		for _, tier := range settings.AllowedUsersSettings.Tiers {
			if tier.Name == subscriptionTier && tier.PriceSats <= 0 {
				isFreeTier = true
				break
			}
		}
		if isFreeTier {
			continue
		}

		// Get the pubkey from the p tag
		for _, tag := range event.Tags {
			if tag[0] == "p" && len(tag) > 1 {
				paidSubscribers = append(paidSubscribers, tag[1])
				break
			}
		}
	}

	log.Printf("Found %d paid subscribers from events", len(paidSubscribers))

	// Get profiles for the subscribers
	profiles, err := GetProfilesForPubkeys(store, paidSubscribers)
	if err != nil {
		log.Printf("Error getting profiles: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch profiles",
		})
	}

	log.Printf("Returning %d profiles with pictures", len(profiles))
	// Log the response structure to help with panel-side implementation
	if len(profiles) > 0 {
		exampleJSON, _ := json.MarshalIndent(profiles[0], "", "  ")
		log.Printf("Profile response structure: %s", exampleJSON)
	}
	return c.JSON(profiles)
}
