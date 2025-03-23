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

	// First, try to get paid subscribers from the statistics store
	paidSubscribers, err := store.GetStatsStore().GetPaidSubscribers()
	if err != nil {
		log.Printf("Error fetching paid subscribers from database, falling back to events: %v", err)

		// Fall back to the original method
		return getSubscribersFromEvents(c, store)
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
		profiles, err := getProfilesForPubkeys(store, pubkeys)
		if err != nil {
			log.Printf("Error getting profiles: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to fetch profiles",
			})
		}

		log.Printf("Returning %d profiles with pictures", len(profiles))
		return c.JSON(profiles)
	}

	// If no results from database, fall back to the original method
	log.Printf("No paid subscribers found in database, falling back to events")
	return getSubscribersFromEvents(c, store)
}

// getProfilesForPubkeys gets profile pictures for a list of public keys
func getProfilesForPubkeys(store stores.Store, pubkeys []string) ([]PaidSubscriberProfile, error) {
	profiles := make([]PaidSubscriberProfile, 0)
	timestamp := nostr.Timestamp(time.Now().Unix())

	for _, pubkey := range pubkeys {
		metadataEvents, err := store.QueryEvents(nostr.Filter{
			Kinds:   []int{0},
			Authors: []string{pubkey},
			Until:   &timestamp,
			Limit:   1, // We only need the latest one
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

	return profiles, nil
}

// getSubscribersFromEvents gets paid subscribers using the original event-based method
func getSubscribersFromEvents(c *fiber.Ctx, store stores.Store) error {
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

	log.Printf("Found %d paid subscribers from events", len(paidSubscribers))

	// Get profiles for the subscribers
	profiles, err := getProfilesForPubkeys(store, paidSubscribers)
	if err != nil {
		log.Printf("Error getting profiles: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch profiles",
		})
	}

	log.Printf("Returning %d profiles with pictures", len(profiles))
	return c.JSON(profiles)
}
