// subscription.go

package subscription

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
)

// InitializeSubscriber creates a new subscriber or retrieves an existing one and creates their initial NIP-88 event.
func (m *SubscriptionManager) InitializeSubscriber(npub string, mode string) error {
	log.Printf("Initializing subscriber for npub: %s in mode: %s", npub, mode)

	// Step 1: Conditionally allocate a Bitcoin address based on mode
	var addressStr string
	if mode == "subscription" {
		// Run address pool check in background for subscription mode
		go func() {
			if err := m.checkAddressPoolStatus(); err != nil {
				log.Printf("Warning: error checking address pool status: %v", err)
			}
		}()

		log.Println("Address Pool checked Going to allocate address")

		// Allocate a Bitcoin address for subscription mode
		address, err := m.store.GetStatsStore().AllocateBitcoinAddress(npub)
		if err != nil {
			log.Printf("Error allocating bitcoin address: %v", err)
			return fmt.Errorf("failed to allocate Bitcoin address: %v", err)
		}
		addressStr = address.Address
		log.Printf("Successfully allocated address: %s", address.Address)
	} else {
		// For non-subscription modes, use empty string
		addressStr = ""
		log.Printf("Skipping Bitcoin address allocation for mode: %s", mode)
	}

	// Step 2: Load allowed users settings to determine appropriate tier
	settings, err := config.GetConfig()
	if err != nil {
		log.Printf("Error getting config: %v", err)
		return fmt.Errorf("failed to get config: %v", err)
	}

	// Step 3: Determine appropriate tier for new user
	log.Printf("DEBUG: AllowedUsersSettings mode: %s", settings.AllowedUsersSettings.Mode)
	log.Printf("DEBUG: Available tiers in settings: %d", len(settings.AllowedUsersSettings.Tiers))
	for i, tier := range settings.AllowedUsersSettings.Tiers {
		log.Printf("DEBUG: Settings tier %d: Name='%s', MonthlyLimitBytes=%d, PriceSats=%d",
			i, tier.Name, tier.MonthlyLimitBytes, tier.PriceSats)
	}

	tierLimit := m.findAppropriateTierForUser(npub, nil, &settings.AllowedUsersSettings)

	// Create initial NIP-88 event with storage usage based on assigned tier
	storageInfo := StorageInfo{
		UsedBytes: 0,
		UpdatedAt: time.Now(),
	}

	if tierLimit != nil {
		storageInfo.TotalBytes = tierLimit.MonthlyLimitBytes
		log.Printf("Setting initial storage limit to %d bytes for tier: %s", storageInfo.TotalBytes, tierLimit.Name)
	} else {
		storageInfo.TotalBytes = 0
		log.Printf("No tier assigned. Setting initial storage limit to 0 bytes")
	}

	// Set expiration date (1 month for all initial subscriptions)
	expirationDate := time.Now().AddDate(0, 1, 0)
	if tierLimit != nil {
		log.Printf("Setting tier limit: %s with expiration: %v", tierLimit.Name, expirationDate)
	} else {
		log.Printf("Setting no tier limit with expiration: %v", expirationDate)
	}

	// Step 4: Create the NIP-88 event
	var tierLimitStr string
	if tierLimit != nil {
		tierLimitStr = tierLimit.Name
	}
	err = m.createNIP88EventIfNotExists(&types.Subscriber{
		Npub:    npub,
		Address: addressStr,
	}, tierLimitStr, expirationDate, &storageInfo)

	if err != nil {
		log.Printf("Error creating NIP-88 event: %v", err)
		return err
	}

	if tierLimit != nil {
		log.Printf("Successfully initialized subscriber %s with tier: %s", npub, tierLimit.Name)
	} else {
		log.Printf("Successfully initialized subscriber %s with no tier", npub)
	}
	return nil
}

// UpdateStorageUsage updates the storage usage for a subscriber by modifying the relevant NIP-88 event
// This function logs errors but does not fail operations to maintain development flow
func (m *SubscriptionManager) UpdateStorageUsage(npub string, newBytes int64) error {
	// Fetch current NIP-88 event data
	events, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{888},
		Tags: nostr.TagMap{
			"p": []string{npub},
		},
		Limit: 1,
	})
	if err != nil || len(events) == 0 {
		log.Printf("Warning: No NIP-88 event found for user %s, storage not tracked (newBytes: %d)", npub, newBytes)
		return nil // Don't fail the operation
	}
	currentEvent := events[0]

	// Extract and update storage information
	storageInfo, err := m.extractStorageInfo(currentEvent)
	if err != nil {
		log.Printf("Warning: Failed to extract storage info for user %s: %v", npub, err)
		return nil // Don't fail the operation
	}

	newUsedBytes := storageInfo.UsedBytes + newBytes

	// Check storage limits but only log warnings for unlimited storage
	if !storageInfo.IsUnlimited && newUsedBytes > storageInfo.TotalBytes {
		log.Printf("Warning: Storage limit exceeded for user %s: would use %d of %d bytes", npub, newUsedBytes, storageInfo.TotalBytes)
		// In development, we'll allow this but log the warning
		// In production, you might want to enforce this limit
	}

	storageInfo.UsedBytes = newUsedBytes
	storageInfo.UpdatedAt = time.Now()

	// Replacing `GetValue` and `GetUnixValue` calls with utility functions
	activeSubscription := getTagValue(currentEvent.Tags, "active_subscription")
	expirationTime := time.Unix(getTagUnixValue(currentEvent.Tags, "active_subscription"), 0)
	address := getTagValue(currentEvent.Tags, "relay_bitcoin_address")

	// Update NIP-88 event
	if err := m.createOrUpdateNIP88Event(&types.Subscriber{
		Npub:    npub,
		Address: address,
	}, activeSubscription, expirationTime, &storageInfo); err != nil {
		log.Printf("Warning: Failed to update NIP-88 event for user %s: %v", npub, err)
		return nil // Don't fail the operation
	}

	log.Printf("Successfully updated storage usage for user %s: +%d bytes (total: %d)", npub, newBytes, newUsedBytes)
	return nil
}

// CheckStorageAvailability checks if a subscriber has enough available storage for a given number of bytes.
// It retrieves storage data from the user's NIP-88 event and validates against their current usage and limits.
func (m *SubscriptionManager) CheckStorageAvailability(npub string, requestedBytes int64) error {
	// Step 1: Fetch the user's NIP-88 event
	events, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{888},
		Tags: nostr.TagMap{
			"p": []string{npub},
		},
		Limit: 1,
	})
	if err != nil || len(events) == 0 {
		return fmt.Errorf("no NIP-88 event found for user: %s", npub)
	}
	currentEvent := events[0]

	// Step 2: Extract storage information from the event
	storageInfo, err := m.extractStorageInfo(currentEvent)
	if err != nil {
		return fmt.Errorf("failed to extract storage info: %v", err)
	}

	// Step 3: Check if the user has enough available storage
	newUsage := storageInfo.UsedBytes + requestedBytes
	if newUsage > storageInfo.TotalBytes {
		return fmt.Errorf("storage limit exceeded: would use %d of %d bytes", newUsage, storageInfo.TotalBytes)
	}

	// Step 4: Optionally, check if the subscription is still active
	for _, tag := range currentEvent.Tags {
		if tag[0] == "active_subscription" && len(tag) >= 3 {
			expirationTimestamp, err := strconv.ParseInt(tag[2], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid subscription expiration timestamp: %v", err)
			}
			expirationDate := time.Unix(expirationTimestamp, 0)
			if time.Now().After(expirationDate) {
				return fmt.Errorf("subscription has expired")
			}
			break
		}
	}

	return nil
}

// requestNewAddresses sends a request to the wallet to generate new addresses
func (m *SubscriptionManager) requestNewAddresses(count int) error {
	// Get API key from config
	apiKey := viper.GetString("external_services.wallet.key")

	// Generate JWT token using API key
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"api_key": apiKey,
		"exp":     time.Now().Add(time.Hour * 24).Unix(),
		"iat":     time.Now().Unix(),
	})

	// Sign token with API key
	tokenString, err := token.SignedString([]byte(apiKey))
	if err != nil {
		return fmt.Errorf("failed to generate token: %v", err)
	}

	reqBody := map[string]interface{}{
		"count": count,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %v", err)
	}

	// Prepare HMAC signature
	timestamp := time.Now().UTC().Format(time.RFC3339)
	message := apiKey + timestamp + string(jsonData)
	h := hmac.New(sha256.New, []byte(apiKey))
	h.Write([]byte(message))
	signature := hex.EncodeToString(h.Sum(nil))

	walletAddress := config.GetExternalURL("wallet")

	// Create request
	req, err := http.NewRequest("POST",
		walletAddress+"/generate-addresses",
		bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	// Add all required headers including the new JWT
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tokenString))
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Signature", signature)

	// Send request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wallet service returned status: %v", resp.Status)
	}

	// Just decode the response to verify it's valid JSON but we don't need to process it
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %v", err)
	}

	log.Printf("Successfully requested generation of %d addresses", count)
	return nil
}

// CheckAndUpdateSubscriptionEvent checks if a kind 888 event needs to be updated
// based on current allowed users settings and updates it if necessary
func (m *SubscriptionManager) CheckAndUpdateSubscriptionEvent(event *nostr.Event) (*nostr.Event, error) {
	// Only process kind 888 events
	if event.Kind != 888 {
		return event, nil
	}

	log.Printf("Checking kind 888 event for updates based on free tier status")

	// Get the pubkey from the p tag
	var pubkey string
	for _, tag := range event.Tags {
		if tag[0] == "p" && len(tag) > 1 {
			pubkey = tag[1]
			break
		}
	}

	if pubkey == "" {
		return event, fmt.Errorf("no pubkey found in kind 888 event")
	}

	// Get subscription status
	var status string
	for _, tag := range event.Tags {
		if tag[0] == "subscription_status" && len(tag) > 1 {
			status = tag[1]
			break
		}
	}

	// Extract current storage info
	storageInfo, err := m.extractStorageInfo(event)
	if err != nil {
		return event, fmt.Errorf("failed to extract storage info: %v", err)
	}

	// Extract active subscription tier
	var activeTier string
	var expirationDate time.Time
	for _, tag := range event.Tags {
		if tag[0] == "active_subscription" && len(tag) > 1 {
			activeTier = tag[1]
			if len(tag) > 2 {
				expirationTimeUnix, err := strconv.ParseInt(tag[2], 10, 64)
				if err == nil {
					expirationDate = time.Unix(expirationTimeUnix, 0)
				}
			}
			break
		}
	}

	// Get bitcoin address from event
	var address string
	for _, tag := range event.Tags {
		if tag[0] == "relay_bitcoin_address" && len(tag) > 1 {
			address = tag[1]
			break
		}
	}

	// Load allowed users settings to check last update timestamp
	var allowedUsersSettings types.AllowedUsersSettings
	if err := viper.UnmarshalKey("allowed_users", &allowedUsersSettings); err != nil {
		log.Printf("Error loading allowed users settings: %v", err)
		return event, nil // Return original event if we can't get settings
	}

	// Check if event was created/updated after the last settings change
	eventCreatedAt := time.Unix(int64(event.CreatedAt), 0)
	settingsUpdatedAt := time.Unix(allowedUsersSettings.LastUpdated, 0)

	// If event is newer than the last settings change, it's already up to date
	if allowedUsersSettings.LastUpdated > 0 && eventCreatedAt.After(settingsUpdatedAt) {
		log.Printf("Event was created/updated after the last settings change, no update needed")
		return event, nil
	}

	needsUpdate := false

	// Check if tier needs updating based on current mode
	// Convert activeTier string to tier object for comparison
	var currentTierObj *types.SubscriptionTier
	if activeTier != "" {
		for i := range allowedUsersSettings.Tiers {
			if allowedUsersSettings.Tiers[i].Name == activeTier {
				currentTierObj = &allowedUsersSettings.Tiers[i]
				break
			}
		}
	}

	expectedTier := m.findAppropriateTierForUser(pubkey, currentTierObj, &allowedUsersSettings)

	if expectedTier == nil {
		// User should not have access in current mode
		if allowedUsersSettings.Mode == "exclusive" {
			log.Printf("User %s no longer in allowed lists for exclusive mode, but keeping existing allocation", pubkey)
			// Don't remove existing allocations, just don't give new ones
		}
	} else if expectedTier.Name != activeTier {
		// Tier has changed
		log.Printf("Expected tier changed for %s: %s -> %s", pubkey, activeTier, expectedTier.Name)
		needsUpdate = true
	} else if status == "inactive" || storageInfo.TotalBytes == 0 {
		// User should have active allocation but doesn't
		log.Printf("User %s should have active %s tier but status is %s", pubkey, expectedTier.Name, status)
		needsUpdate = true
	}

	if !needsUpdate {
		return event, nil
	}

	// Update the event based on expected tier
	if expectedTier != nil {
		// Set expiration if not set (monthly renewal)
		if expirationDate.IsZero() || expirationDate.Before(time.Now()) {
			expirationDate = time.Now().AddDate(0, 1, 0) // 1 month from now
		}

		// Calculate storage based on expected tier
		expectedTierBytes := expectedTier.MonthlyLimitBytes
		if expectedTier.Unlimited {
			expectedTierBytes = types.MaxMonthlyLimitBytes
		}

		// Don't reduce storage if they already have more (graceful transition)
		if storageInfo.TotalBytes < expectedTierBytes {
			storageInfo.TotalBytes = expectedTierBytes
		}

		// Update active tier
		activeTier = expectedTier.Name
	}

	// Create a new updated event
	updatedEvent, err := m.createEvent(&types.Subscriber{
		Npub:    pubkey,
		Address: address,
	}, activeTier, expirationDate, &storageInfo)

	if err != nil {
		return event, fmt.Errorf("failed to create updated event: %v", err)
	}

	// Store the updated event
	if err := m.store.StoreEvent(updatedEvent); err != nil {
		return event, fmt.Errorf("failed to store updated event: %v", err)
	}

	log.Printf("Successfully updated kind 888 event for pubkey %s", pubkey)
	return updatedEvent, nil
}

// UpdateNpubSubscriptionEvent updates the kind 888 event for a specific npub with new tier information
// This is called when access control lists are updated and we need to sync the kind 888 events
func (m *SubscriptionManager) UpdateNpubSubscriptionEvent(npub, tierName string) error {
	log.Printf("Updating kind 888 event for npub %s with tier %s", npub, tierName)

	// Load allowed users settings to get tier configuration
	var allowedUsersSettings types.AllowedUsersSettings
	if err := viper.UnmarshalKey("allowed_users", &allowedUsersSettings); err != nil {
		return fmt.Errorf("failed to load allowed users settings: %v", err)
	}

	// Find the tier configuration
	var activeTier *types.SubscriptionTier
	for i := range allowedUsersSettings.Tiers {
		if allowedUsersSettings.Tiers[i].Name == tierName {
			activeTier = &allowedUsersSettings.Tiers[i]
			break
		}
	}

	if activeTier == nil {
		return fmt.Errorf("tier %s not found in configuration", tierName)
	}

	// Create subscriber info
	subscriber := &types.Subscriber{
		Npub: npub,
	}

	// Set expiration date based on tier type
	expirationDate := time.Now().AddDate(0, 1, 0) // Default to 1 month
	if activeTier.PriceSats <= 0 {
		// For free tiers, set longer expiration
		expirationDate = time.Now().AddDate(1, 0, 0) // 1 year for free
	}

	// Create storage info from tier - check for unlimited storage
	isUnlimited := activeTier.MonthlyLimitBytes == 0 || allowedUsersSettings.Mode == "personal"

	log.Printf("[DEBUG] Creating storage info for npub %s: tier=%s, monthlyLimitBytes=%d, mode=%s, isUnlimited=%t",
		npub, tierName, activeTier.MonthlyLimitBytes, allowedUsersSettings.Mode, isUnlimited)

	storageInfo := &StorageInfo{
		TotalBytes:  activeTier.MonthlyLimitBytes,
		UsedBytes:   0, // Start with 0 used bytes
		IsUnlimited: isUnlimited,
		UpdatedAt:   time.Now(),
	}

	// Create or update the NIP-88 event
	return m.createOrUpdateNIP88Event(subscriber, activeTier, expirationDate, storageInfo)
}

// updatePaidSubscriberRecord is a helper method to update the PaidSubscriber table
// This should be called after successfully updating a NIP-88 event
func (m *SubscriptionManager) updatePaidSubscriberRecord(
	npub string,
	tier interface{}, // Can be string or *types.SubscriptionTier
	expirationDate time.Time,
	storageInfo *StorageInfo,
) {
	// Load allowed users settings to check if this is a free tier
	var allowedUsersSettings types.AllowedUsersSettings
	if err := viper.UnmarshalKey("allowed_users", &allowedUsersSettings); err != nil {
		log.Printf("Error loading allowed users settings: %v", err)
		return
	}

	// Determine tier name and check if it's free
	var tierName string
	var isFree bool

	switch t := tier.(type) {
	case string:
		tierName = t
		// Check if this tier name is free
		for _, allowedTier := range allowedUsersSettings.Tiers {
			if allowedTier.Name == tierName && allowedTier.PriceSats <= 0 {
				isFree = true
				break
			}
		}
	case *types.SubscriptionTier:
		tierName = t.Name
		isFree = t.PriceSats <= 0
	}

	if isFree {
		log.Printf("Skipping paid subscriber record update for free tier: %s", tierName)
		return
	}

	// Try to get the existing subscriber
	existingSubscriber, err := m.store.GetStatsStore().GetPaidSubscriberByNpub(npub)
	if err != nil {
		log.Printf("Warning: error checking for existing paid subscriber record: %v", err)
		return
	}

	// Create or update the paid subscriber record
	paidSubscriber := &types.PaidSubscriber{
		Npub:           npub,
		Tier:           tierName,
		ExpirationDate: expirationDate,
		StorageBytes:   storageInfo.TotalBytes,
		UsedBytes:      storageInfo.UsedBytes,
	}

	// If subscriber already exists, keep the ID
	var updateErr error
	if existingSubscriber != nil {
		paidSubscriber.ID = existingSubscriber.ID
		updateErr = m.store.GetStatsStore().UpdatePaidSubscriber(paidSubscriber)
	} else {
		updateErr = m.store.GetStatsStore().SavePaidSubscriber(paidSubscriber)
	}

	if updateErr != nil {
		log.Printf("Warning: failed to update paid subscriber record: %v", updateErr)
	} else {
		log.Printf("Successfully updated paid subscriber record for %s", npub)
	}
}

// isUserInAllowedLists checks if a user is in the allowed read or write lists for exclusive mode
func (m *SubscriptionManager) isUserInAllowedLists(pubkey string) bool {
	user, err := m.store.GetStatsStore().GetAllowedUser(pubkey)
	if err != nil || user == nil {
		return false
	}

	return true
}
