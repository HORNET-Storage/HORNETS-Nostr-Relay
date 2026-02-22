// events.go - Kind 11888 event management

package subscription

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/nbd-wtf/go-nostr"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
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
		logging.Infof("Warning: could not get credit for subscriber: %v", err)
		creditSats = 0
	}

	// Create storage tag value
	totalBytesStr := func() string {
		if storageInfo.IsUnlimited {
			return "unlimited"
		}
		return fmt.Sprintf("%d", storageInfo.TotalBytes)
	}()

	logging.Infof("[DEBUG] Creating kind 11888 event for %s: usedBytes=%d, totalBytes=%s, isUnlimited=%t",
		subscriber.Npub, storageInfo.UsedBytes, totalBytesStr, storageInfo.IsUnlimited)

	// Get relay mode from config
	relayMode := m.getRelayMode()

	// Prepare tags with free tier consideration
	tags := []nostr.Tag{
		{"subscription_duration", "1 month"},
		{"p", subscriber.Npub},
		{"subscription_status", status},
		{"relay_bitcoin_address", subscriber.Address}, // This can now be empty string for non-subscription modes
		{"relay_dht_key", m.relayDHTKey},
		{"storage", fmt.Sprintf("%d", storageInfo.UsedBytes), totalBytesStr, fmt.Sprintf("%d", storageInfo.UpdatedAt.Unix())},
		{"relay_mode", relayMode},
	}

	// Log address handling for debugging
	if subscriber.Address == "" {
		logging.Infof("Creating kind 11888 event without Bitcoin address for user %s", subscriber.Npub)
	} else {
		logging.Infof("Creating kind 11888 event with Bitcoin address %s for user %s", subscriber.Address, subscriber.Npub)
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
		Kind:      11888,
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
	logging.Infof("Creating/updating NIP-88 event for %s with tier %s",
		subscriber.Npub, tierName)

	// Delete ALL existing NIP-88 events for this user (check both npub and hex formats)
	hex, npub, err := normalizePubkey(subscriber.Npub)
	if err != nil {
		return fmt.Errorf("failed to normalize pubkey: %v", err)
	}

	existingEvents, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{11888},
		Tags: nostr.TagMap{
			"p": []string{npub, hex}, // Check both formats
		},
		// Remove limit to get all events
	})
	if err == nil && len(existingEvents) > 0 {
		logging.Infof("Deleting %d existing NIP-88 events for %s", len(existingEvents), subscriber.Npub)
		for _, event := range existingEvents {
			if err := m.store.DeleteEvent(event.ID); err != nil {
				logging.Infof("Warning: failed to delete existing NIP-88 event %s: %v", event.ID, err)
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

	// Check for existing event
	existingEvents, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{11888},
		Tags: nostr.TagMap{
			"p": []string{subscriber.Npub},
		},
		Limit: 1,
	})
	if err != nil {
		logging.Infof("Error querying events: %v", err)
		return fmt.Errorf("error querying existing NIP-88 events: %v", err)
	}

	if len(existingEvents) > 0 {

		return nil
	}

	logging.Infof("Creating new NIP-88 event for subscriber %s", subscriber.Npub)
	logging.Infof("Subscriber Address: %s", subscriber.Address)

	// Create new event
	event, err := m.createEvent(subscriber, activeTier, expirationDate, storageInfo)
	if err != nil {
		return err
	}

	logging.Infof("Subscription Event before storing: %s", event.String())

	// Store and verify
	if err := m.store.StoreEvent(event); err != nil {
		return fmt.Errorf("error storing event: %v", err)
	}

	// Verification
	storedEvents, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{11888},
		Tags: nostr.TagMap{
			"p": []string{subscriber.Npub},
		},
		Limit: 1,
	})
	if err != nil {
		logging.Infof("Error verifying stored event: %v", err)
	} else {
		logging.Infof("Verified stored event. Found %d events", len(storedEvents))
		if len(storedEvents) > 0 {
			logging.Infof("Event details: %+v", storedEvents[0])
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

			// Handle "unlimited" storage case
			if tag[2] == "unlimited" {
				info.IsUnlimited = true
				info.TotalBytes = 0 // 0 with IsUnlimited=true means unlimited
			} else {
				total, err := strconv.ParseInt(tag[2], 10, 64)
				if err != nil {
					return info, fmt.Errorf("invalid total storage value: %v", err)
				}
				info.TotalBytes = total
				info.IsUnlimited = false
			}

			updated, err := strconv.ParseInt(tag[3], 10, 64)
			if err != nil {
				return info, fmt.Errorf("invalid update timestamp: %v", err)
			}

			info.UsedBytes = used
			info.UpdatedAt = time.Unix(updated, 0)
			return info, nil
		}
	}

	// Return zero values if no storage tag found
	return StorageInfo{
		UsedBytes:   0,
		TotalBytes:  0,
		IsUnlimited: false,
		UpdatedAt:   time.Now(),
	}, nil
}
