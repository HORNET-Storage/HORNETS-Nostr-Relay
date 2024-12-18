package web

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	"github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
)

// UserMetadata represents the content of kind 0 events
type UserMetadata struct {
	Name    string `json:"name,omitempty"`
	About   string `json:"about,omitempty"`
	Picture string `json:"picture,omitempty"`
}

// PaidSubscriberProfile represents the response structure
type PaidSubscriberProfile struct {
	Pubkey  string `json:"pubkey"`
	Picture string `json:"picture"`
}

// HandleGetPaidSubscriberProfiles gets profile pictures of paid subscribers
func HandleGetPaidSubscriberProfiles(c *fiber.Ctx, store stores.Store) error {
	log.Printf("Fetching paid subscriber profiles")

	// First, get relay settings to determine free tier status
	var relaySettings lib.RelaySettings
	if err := viper.UnmarshalKey("relay_settings", &relaySettings); err != nil {
		log.Printf("Error loading relay settings: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to load relay settings",
		})
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

		// Skip if it matches the free tier limit
		if relaySettings.FreeTierEnabled && subscriptionTier == relaySettings.FreeTierLimit {
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

	log.Printf("Found %d paid subscribers", len(paidSubscribers))

	// Step 2: Get kind 0 events for these subscribers
	profiles := make([]PaidSubscriberProfile, 0)
	timestamp := nostr.Timestamp(time.Now().Unix())
	for _, pubkey := range paidSubscribers {
		metadataEvents, err := store.QueryEvents(nostr.Filter{
			Kinds:   []int{0},
			Authors: []string{pubkey},
			Until:   &timestamp, // Fixed: Now passing a pointer to the timestamp
			Limit:   1,          // We only need the latest one
		})
		if err != nil {
			log.Printf("Error querying metadata for pubkey %s: %v", pubkey, err)
			continue
		}

		if len(metadataEvents) > 0 {
			var metadata UserMetadata
			if err := json.Unmarshal([]byte(metadataEvents[0].Content), &metadata); err != nil {
				log.Printf("Error unmarshaling metadata for pubkey %s: %v", pubkey, err)
				continue
			}

			if metadata.Picture != "" {
				profiles = append(profiles, PaidSubscriberProfile{
					Pubkey:  pubkey,
					Picture: metadata.Picture,
				})
			}
		}
	}

	log.Printf("Returning %d profiles with pictures", len(profiles))
	return c.JSON(profiles)
}
