// events.go - Kind 888 event management

package subscription

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/nbd-wtf/go-nostr"

	"github.com/HORNET-Storage/hornet-storage/lib/types"
)

// createEvent is a helper function to create a NIP-88 event with common logic
func (m *SubscriptionManager) createEvent(
	subscriber *types.Subscriber,
	activeTier string,
	expirationDate time.Time,
	storageInfo *StorageInfo,
) (*nostr.Event, error) {
	// Determine subscription status based on tier and expiration
	status := m.getSubscriptionStatus(activeTier)
	if activeTier != "" && !expirationDate.IsZero() && expirationDate.After(time.Now()) {
		status = "active"
	}

	// Get current credit for the subscriber
	creditSats, err := m.store.GetStatsStore().GetSubscriberCredit(subscriber.Npub)
	if err != nil {
		log.Printf("Warning: could not get credit for subscriber: %v", err)
		creditSats = 0
	}

	// Create storage tag value
	totalBytesStr := func() string {
		if storageInfo.IsUnlimited {
			return "unlimited"
		}
		return fmt.Sprintf("%d", storageInfo.TotalBytes)
	}()

	log.Printf("[DEBUG] Creating kind 888 event for %s: usedBytes=%d, totalBytes=%s, isUnlimited=%t",
		subscriber.Npub, storageInfo.UsedBytes, totalBytesStr, storageInfo.IsUnlimited)

	// Get relay mode from config
	relayMode := m.getRelayMode()

	// Prepare tags with free tier consideration
	tags := []nostr.Tag{
		{"subscription_duration", "1 month"},
		{"p", subscriber.Npub},
		{"subscription_status", status},
		{"relay_bitcoin_address", subscriber.Address},
		{"relay_dht_key", m.relayDHTKey},
		{"storage", fmt.Sprintf("%d", storageInfo.UsedBytes), totalBytesStr, fmt.Sprintf("%d", storageInfo.UpdatedAt.Unix())},
		{"relay_mode", relayMode},
	}

	// Add credit information if there is any
	if creditSats > 0 {
		tags = append(tags, nostr.Tag{
			"credit", fmt.Sprintf("%d", creditSats),
		})
	}

	// Add tier information if tier is assigned
	if activeTier != "" {
		tags = append(tags, nostr.Tag{
			"active_subscription",
			activeTier,
			fmt.Sprintf("%d", expirationDate.Unix()),
		})
	}

	// Create the event
	event := &nostr.Event{
		PubKey:    hex.EncodeToString(m.relayPrivateKey.PubKey().SerializeCompressed()),
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      888,
		Tags:      tags,
		Content:   "",
	}

	// Sign the event
	serializedEvent := event.Serialize()
	hash := sha256.Sum256(serializedEvent)
	event.ID = hex.EncodeToString(hash[:])
	sig, err := schnorr.Sign(m.relayPrivateKey, hash[:])
	if err != nil {
		return nil, fmt.Errorf("error signing event: %v", err)
	}
	event.Sig = hex.EncodeToString(sig.Serialize())

	return event, nil
}

// createOrUpdateNIP88Event creates or updates a subscriber's NIP-88 event
func (m *SubscriptionManager) createOrUpdateNIP88Event(
	subscriber *types.Subscriber,
	activeTier interface{}, // Can be string or *types.SubscriptionTier
	expirationDate time.Time,
	storageInfo *StorageInfo,
) error {
	var tierName string
	switch t := activeTier.(type) {
	case string:
		tierName = t
	case *types.SubscriptionTier:
		tierName = t.Name
	default:
		tierName = ""
	}
	log.Printf("Creating/updating NIP-88 event for %s with tier %s",
		subscriber.Npub, tierName)

	// Delete ALL existing NIP-88 events for this user (check both npub and hex formats)
	hex, npub, err := normalizePubkey(subscriber.Npub)
	if err != nil {
		return fmt.Errorf("failed to normalize pubkey: %v", err)
	}

	existingEvents, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{888},
		Tags: nostr.TagMap{
			"p": []string{npub, hex}, // Check both formats
		},
		// Remove limit to get all events
	})
	if err == nil && len(existingEvents) > 0 {
		log.Printf("Deleting %d existing NIP-88 events for %s", len(existingEvents), subscriber.Npub)
		for _, event := range existingEvents {
			if err := m.store.DeleteEvent(event.ID); err != nil {
				log.Printf("Warning: failed to delete existing NIP-88 event %s: %v", event.ID, err)
			}
		}
	}

	// Create new event
	event, err := m.createEvent(subscriber, tierName, expirationDate, storageInfo)
	if err != nil {
		return err
	}

	return m.store.StoreEvent(event)
}

// createNIP88EventIfNotExists creates a new NIP-88 event for a subscriber if none exists
func (m *SubscriptionManager) createNIP88EventIfNotExists(
	subscriber *types.Subscriber,
	activeTier string,
	expirationDate time.Time,
	storageInfo *StorageInfo,
) error {
	log.Printf("Checking for existing NIP-88 event for subscriber %s", subscriber.Npub)

	// Check for existing event
	existingEvents, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{888},
		Tags: nostr.TagMap{
			"p": []string{subscriber.Npub},
		},
		Limit: 1,
	})
	if err != nil {
		log.Printf("Error querying events: %v", err)
		return fmt.Errorf("error querying existing NIP-88 events: %v", err)
	}

	if len(existingEvents) > 0 {
		log.Printf("NIP-88 event already exists for subscriber %s, skipping creation", subscriber.Npub)
		return nil
	}

	log.Printf("Creating new NIP-88 event for subscriber %s", subscriber.Npub)
	log.Printf("Subscriber Address: %s", subscriber.Address)

	// Create new event
	event, err := m.createEvent(subscriber, activeTier, expirationDate, storageInfo)
	if err != nil {
		return err
	}

	log.Println("Subscription Event before storing: ", event.String())

	// Store and verify
	if err := m.store.StoreEvent(event); err != nil {
		return fmt.Errorf("error storing event: %v", err)
	}

	// Verification
	storedEvents, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{888},
		Tags: nostr.TagMap{
			"p": []string{subscriber.Npub},
		},
		Limit: 1,
	})
	if err != nil {
		log.Printf("Error verifying stored event: %v", err)
	} else {
		log.Printf("Verified stored event. Found %d events", len(storedEvents))
		if len(storedEvents) > 0 {
			log.Printf("Event details: %+v", storedEvents[0])
		}
	}

	return nil
}

// extractStorageInfo gets storage information from NIP-88 event
func (m *SubscriptionManager) extractStorageInfo(event *nostr.Event) (StorageInfo, error) {
	var info StorageInfo

	for _, tag := range event.Tags {
		if tag[0] == "storage" && len(tag) >= 4 {
			used, err := strconv.ParseInt(tag[1], 10, 64)
			if err != nil {
				return info, fmt.Errorf("invalid used storage value: %v", err)
			}

			total, err := strconv.ParseInt(tag[2], 10, 64)
			if err != nil {
				return info, fmt.Errorf("invalid total storage value: %v", err)
			}

			updated, err := strconv.ParseInt(tag[3], 10, 64)
			if err != nil {
				return info, fmt.Errorf("invalid update timestamp: %v", err)
			}

			info.UsedBytes = used
			info.TotalBytes = total
			info.UpdatedAt = time.Unix(updated, 0)
			return info, nil
		}
	}

	// Return zero values if no storage tag found
	return StorageInfo{
		UsedBytes:  0,
		TotalBytes: 0,
		UpdatedAt:  time.Now(),
	}, nil
}
