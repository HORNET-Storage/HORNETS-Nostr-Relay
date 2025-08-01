// subscription.go

package subscription

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
)

// InitializeSubscriber creates a new subscriber or retrieves an existing one and creates their initial NIP-88 event.
func (m *SubscriptionManager) InitializeSubscriber(npub string, mode string) error {
	logging.Infof("Initializing subscriber for npub: %s in mode: %s", npub, mode)

	// Step 1: Conditionally allocate a Bitcoin address based on mode
	var addressStr string
	if mode == "subscription" {
		// Run address pool check in background for subscription mode
		go func() {
			if err := m.checkAddressPoolStatus(); err != nil {
				logging.Infof("Warning: error checking address pool status: %v", err)
			}
		}()

		logging.Info("Address Pool checked Going to allocate address")

		// Allocate a Bitcoin address for subscription mode
		address, err := m.store.GetStatsStore().AllocateBitcoinAddress(npub)
		if err != nil {
			logging.Infof("Error allocating bitcoin address: %v", err)
			return fmt.Errorf("failed to allocate Bitcoin address: %v", err)
		}
		addressStr = address.Address
		logging.Infof("Successfully allocated address: %s", address.Address)
	} else {
		// For non-subscription modes, use empty string
		addressStr = ""
		logging.Infof("Skipping Bitcoin address allocation for mode: %s", mode)
	}

	// Step 2: Load allowed users settings to determine appropriate tier
	settings, err := config.GetConfig()
	if err != nil {
		logging.Infof("Error getting config: %v", err)
		return fmt.Errorf("failed to get config: %v", err)
	}

	// Step 3: Determine appropriate tier for new user
	// logging.Infof("DEBUG: AllowedUsersSettings mode: %s", settings.AllowedUsersSettings.Mode)
	// logging.Infof("DEBUG: Available tiers in settings: %d", len(settings.AllowedUsersSettings.Tiers))
	// for i, tier := range settings.AllowedUsersSettings.Tiers {
	// 	logging.Infof("DEBUG: Settings tier %d: Name='%s', MonthlyLimitBytes=%d, PriceSats=%d",
	// 		i, tier.Name, tier.MonthlyLimitBytes, tier.PriceSats)
	// }

	tierLimit := m.findAppropriateTierForUser(npub, nil, &settings.AllowedUsersSettings)

	// Create initial NIP-88 event with storage usage based on assigned tier
	storageInfo := StorageInfo{
		UsedBytes: 0,
		UpdatedAt: time.Now(),
	}

	if tierLimit != nil {
		storageInfo.TotalBytes = tierLimit.MonthlyLimitBytes
		logging.Infof("Setting initial storage limit to %d bytes for tier: %s", storageInfo.TotalBytes, tierLimit.Name)
	} else {
		storageInfo.TotalBytes = 0
		logging.Infof("No tier assigned. Setting initial storage limit to 0 bytes")
	}

	// Set expiration date (1 month for all initial subscriptions)
	expirationDate := time.Now().AddDate(0, 1, 0)
	if tierLimit != nil {
		logging.Infof("Setting tier limit: %s with expiration: %v", tierLimit.Name, expirationDate)
	} else {
		logging.Infof("Setting no tier limit with expiration: %v", expirationDate)
	}

	// Step 4: Create the NIP-88 event asynchronously to avoid blocking event processing
	var tierLimitStr string
	if tierLimit != nil {
		tierLimitStr = tierLimit.Name
	}

	// Create NIP-88 event in background to avoid blocking the authentication flow
	go func() {
		subscriber := &types.Subscriber{
			Npub:    npub,
			Address: addressStr,
		}

		if err := m.createNIP88EventIfNotExists(subscriber, tierLimitStr, expirationDate, &storageInfo); err != nil {
			logging.Infof("Error creating NIP-88 event asynchronously for %s: %v", npub, err)
		} else {
			if tierLimit != nil {
				// logging.Infof("Successfully created NIP-88 event for subscriber %s with tier: %s", npub, tierLimit.Name)
			} else {
				// logging.Infof("Successfully created NIP-88 event for subscriber %s with no tier", npub)
			}
		}
	}()

	// Return immediately without waiting for NIP-88 event creation
	if tierLimit != nil {
		logging.Infof("Successfully initialized subscriber %s with tier: %s (NIP-88 event creating asynchronously)", npub, tierLimit.Name)
	} else {
		logging.Infof("Successfully initialized subscriber %s with no tier (NIP-88 event creating asynchronously)", npub)
	}
	return nil
}

// UpdateStorageUsage updates the storage usage for a subscriber by modifying the relevant NIP-88 event
// This function logs errors but does not fail operations to maintain development flow
func (m *SubscriptionManager) UpdateStorageUsage(npub string, newBytes int64) error {
	// Fetch current NIP-88 event data
	events, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{11888},
		Tags: nostr.TagMap{
			"p": []string{npub},
		},
		Limit: 1,
	})
	if err != nil || len(events) == 0 {
		logging.Infof("Warning: No NIP-88 event found for user %s, storage not tracked (newBytes: %d)", npub, newBytes)
		return nil // Don't fail the operation
	}
	currentEvent := events[0]

	// Extract and update storage information
	storageInfo, err := m.extractStorageInfo(currentEvent)
	if err != nil {
		logging.Infof("Warning: Failed to extract storage info for user %s: %v", npub, err)
		return nil // Don't fail the operation
	}

	newUsedBytes := storageInfo.UsedBytes + newBytes

	// Check storage limits but only log warnings for unlimited storage
	if !storageInfo.IsUnlimited && newUsedBytes > storageInfo.TotalBytes {
		logging.Infof("Warning: Storage limit exceeded for user %s: would use %d of %d bytes", npub, newUsedBytes, storageInfo.TotalBytes)
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
		logging.Infof("Warning: Failed to update NIP-88 event for user %s: %v", npub, err)
		return nil // Don't fail the operation
	}

	logging.Infof("Successfully updated storage usage for user %s: +%d bytes (total: %d)", npub, newBytes, newUsedBytes)
	return nil
}

// CheckStorageAvailability checks if a subscriber has enough available storage for a given number of bytes.
// It retrieves storage data from the user's NIP-88 event and validates against their current usage and limits.
func (m *SubscriptionManager) CheckStorageAvailability(npub string, requestedBytes int64) error {
	// Step 1: Fetch the user's NIP-88 event
	events, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{11888},
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

// RequestNewAddresses sends a request to the wallet to generate new addresses
func (m *SubscriptionManager) RequestNewAddresses(count int) error {
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

	logging.Infof("Successfully requested generation of %d addresses", count)
	return nil
}

// CheckAndUpdateSubscriptionEvent checks if a kind 11888 event needs to be updated
// based on current allowed users settings and updates it if necessary
func (m *SubscriptionManager) CheckAndUpdateSubscriptionEvent(event *nostr.Event) (*nostr.Event, error) {
	// Only process kind 11888 events
	if event.Kind != 11888 {
		return event, nil
	}

	logging.Infof("Checking kind 11888 event for updates based on free tier status")

	// Get the pubkey from the p tag
	var pubkey string
	for _, tag := range event.Tags {
		if tag[0] == "p" && len(tag) > 1 {
			pubkey = tag[1]
			break
		}
	}

	if pubkey == "" {
		return event, fmt.Errorf("no pubkey found in kind 11888 event")
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
	allowedUsersSettings, err := config.GetAllowedUsersSettings()
	if err != nil {
		logging.Infof("Error loading allowed users settings: %v", err)
		return event, nil // Return original event if we can't get settings
	}

	// Check if event was created/updated after the last settings change
	eventCreatedAt := time.Unix(int64(event.CreatedAt), 0)
	settingsUpdatedAt := time.Unix(allowedUsersSettings.LastUpdated, 0)

	// If event is newer than the last settings change, it's already up to date
	if allowedUsersSettings.LastUpdated > 0 && eventCreatedAt.After(settingsUpdatedAt) {
		logging.Infof("Event was created/updated after the last settings change, no update needed")
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

	expectedTier := m.findAppropriateTierForUser(pubkey, currentTierObj, allowedUsersSettings)

	if expectedTier == nil {
		// User should not have access in current mode
		if allowedUsersSettings.Mode == "exclusive" {
			logging.Infof("User %s no longer in allowed lists for exclusive mode, but keeping existing allocation", pubkey)
			// Don't remove existing allocations, just don't give new ones
		}
	} else if expectedTier.Name != activeTier {
		// Tier has changed
		logging.Infof("Expected tier changed for %s: %s -> %s", pubkey, activeTier, expectedTier.Name)
		needsUpdate = true
	} else if status == "inactive" || storageInfo.TotalBytes == 0 {
		// User should have active allocation but doesn't
		logging.Infof("User %s should have active %s tier but status is %s", pubkey, expectedTier.Name, status)
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

	logging.Infof("Successfully updated kind 11888 event for pubkey %s", pubkey)
	return updatedEvent, nil
}

// UpdateUserSubscriptionFromDatabase updates a user's kind 11888 event by looking up their tier from the database
// This follows the correct flow: DB lookup -> config lookup -> update event
func (m *SubscriptionManager) UpdateUserSubscriptionFromDatabase(npub string) error {
	logging.Infof("Updating kind 11888 event for npub %s (looking up tier from database)", npub)

	// Load allowed users settings to get tier configuration
	allowedUsersSettings, err := config.GetAllowedUsersSettings()
	if err != nil {
		return fmt.Errorf("failed to load allowed users settings: %v", err)
	}

	// Look up the user's assigned tier from the database
	statsStore := m.store.GetStatsStore()
	if statsStore == nil {
		return fmt.Errorf("statistics store not available")
	}

	// Normalize the pubkey to ensure we're using the right format
	hexKey, _, err := normalizePubkey(npub)
	if err != nil {
		return fmt.Errorf("error normalizing pubkey %s: %v", npub, err)
	}

	allowedUser, err := statsStore.GetAllowedUser(hexKey)
	if err != nil {
		return fmt.Errorf("error getting allowed user: %v", err)
	}

	if allowedUser == nil {
		return fmt.Errorf("user %s not found in allowed users table", npub)
	}

	if allowedUser.Tier == "" {
		return fmt.Errorf("user %s has no tier assigned", npub)
	}

	logging.Infof("Found user %s with assigned tier: %s", npub, allowedUser.Tier)

	// Find the tier configuration that matches the user's assigned tier
	var activeTier *types.SubscriptionTier
	for i := range allowedUsersSettings.Tiers {
		if allowedUsersSettings.Tiers[i].Name == allowedUser.Tier {
			activeTier = &allowedUsersSettings.Tiers[i]
			break
		}
	}

	if activeTier == nil {
		return fmt.Errorf("tier %s not found in configuration", allowedUser.Tier)
	}

	logging.Infof("Found tier configuration: Name=%s, MonthlyLimitBytes=%d", activeTier.Name, activeTier.MonthlyLimitBytes)

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

	// Get current storage usage from existing kind 11888 event if it exists
	var currentUsedBytes int64 = 0
	existingEvents, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{11888},
		Tags: nostr.TagMap{
			"p": []string{npub, hexKey}, // Check both formats
		},
		Limit: 1,
	})
	if err == nil && len(existingEvents) > 0 {
		// Extract current usage from existing event
		existingStorageInfo, err := m.extractStorageInfo(existingEvents[0])
		if err == nil {
			currentUsedBytes = existingStorageInfo.UsedBytes
			logging.Infof("Preserving current used storage for %s: %d bytes", npub, currentUsedBytes)
		} else {
			logging.Infof("Warning: Could not extract existing storage info for %s: %v", npub, err)
		}
	} else {
		logging.Infof("No existing kind 11888 event found for %s, starting with 0 used bytes", npub)
	}

	// Create storage info from tier - check for unlimited storage
	isUnlimited := activeTier.MonthlyLimitBytes == 0 || allowedUsersSettings.Mode == "personal"

	logging.Infof("[DEBUG] Creating storage info for npub %s: tier=%s, monthlyLimitBytes=%d, mode=%s, isUnlimited=%t, preservedUsedBytes=%d",
		npub, activeTier.Name, activeTier.MonthlyLimitBytes, allowedUsersSettings.Mode, isUnlimited, currentUsedBytes)

	storageInfo := &StorageInfo{
		TotalBytes:  activeTier.MonthlyLimitBytes,
		UsedBytes:   currentUsedBytes, // Preserve current usage
		IsUnlimited: isUnlimited,
		UpdatedAt:   time.Now(),
	}

	// Create or update the NIP-88 event
	return m.createOrUpdateNIP88Event(subscriber, activeTier.Name, expirationDate, storageInfo)
}

// UpdateNpubSubscriptionEvent updates the kind 11888 event for a specific npub with new tier information
// This is called when access control lists are updated and we need to sync the kind 11888 events
func (m *SubscriptionManager) UpdateNpubSubscriptionEvent(npub, tierName string) error {
	logging.Infof("Updating kind 11888 event for npub %s with tier %s", npub, tierName)

	// Load allowed users settings to get tier configuration
	allowedUsersSettings, err := config.GetAllowedUsersSettings()
	if err != nil {
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

	logging.Infof("[DEBUG] Creating storage info for npub %s: tier=%s, monthlyLimitBytes=%d, mode=%s, isUnlimited=%t",
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
	allowedUsersSettings, err := config.GetAllowedUsersSettings()
	if err != nil {
		logging.Infof("Error loading allowed users settings: %v", err)
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
		logging.Infof("Skipping paid subscriber record update for free tier: %s", tierName)
		return
	}

	// Try to get the existing subscriber
	existingSubscriber, err := m.store.GetStatsStore().GetPaidSubscriberByNpub(npub)
	if err != nil {
		logging.Infof("Warning: error checking for existing paid subscriber record: %v", err)
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
		logging.Infof("Warning: failed to update paid subscriber record: %v", updateErr)
	} else {
		logging.Infof("Successfully updated paid subscriber record for %s", npub)
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

// CheckWalletServiceHealth checks if the wallet service is reachable
func CheckWalletServiceHealth() (bool, error) {
	// Get API key from config
	apiKey := viper.GetString("external_services.wallet.key")
	logging.Infof("Wallet health check: API key configured: %t", len(apiKey) > 0)

	// Generate JWT token using API key
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"api_key": apiKey,
		"exp":     time.Now().Add(time.Hour * 24).Unix(),
		"iat":     time.Now().Unix(),
	})

	// Sign token with API key
	tokenString, err := token.SignedString([]byte(apiKey))
	if err != nil {
		logging.Infof("Wallet health check: Failed to generate token: %v", err)
		return false, fmt.Errorf("failed to generate token: %v", err)
	}

	// Create request body
	reqBody := map[string]interface{}{
		"request_id": fmt.Sprintf("health-check-%d", time.Now().Unix()),
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		logging.Infof("Wallet health check: Failed to marshal request: %v", err)
		return false, fmt.Errorf("failed to marshal request: %v", err)
	}

	// Prepare HMAC signature
	timestamp := time.Now().UTC().Format(time.RFC3339)
	message := apiKey + timestamp + string(jsonData)
	h := hmac.New(sha256.New, []byte(apiKey))
	h.Write([]byte(message))
	signature := hex.EncodeToString(h.Sum(nil))

	walletAddress := config.GetExternalURL("wallet")
	healthURL := walletAddress + "/health"
	logging.Infof("Wallet health check: Attempting request to %s", healthURL)

	// Create POST request
	req, err := http.NewRequest("POST", healthURL, bytes.NewBuffer(jsonData))
	if err != nil {
		logging.Infof("Wallet health check: Failed to create request: %v", err)
		return false, fmt.Errorf("failed to create request: %v", err)
	}

	// Add all required headers (same as /generate-addresses)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tokenString))
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Signature", signature)

	logging.Infof("Wallet health check: Request headers set, sending request...")

	// Send request
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		logging.Infof("Wallet health check: Request failed: %v", err)
		return false, fmt.Errorf("wallet service unreachable: %v", err)
	}
	defer resp.Body.Close()

	logging.Infof("Wallet health check: Received response with status: %d (%s)", resp.StatusCode, resp.Status)

	if resp.StatusCode != http.StatusOK {
		// Read response body for more details
		body := make([]byte, 1024)
		n, _ := resp.Body.Read(body)
		logging.Infof("Wallet health check: Non-200 response body: %s", string(body[:n]))
		return false, fmt.Errorf("wallet service returned status: %v", resp.Status)
	}

	// Parse the health response for logging purposes
	var healthData struct {
		Status       string `json:"status"`
		Timestamp    string `json:"timestamp"`
		WalletLocked bool   `json:"wallet_locked"`
		ChainSynced  bool   `json:"chain_synced"`
		PeerCount    int    `json:"peer_count"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&healthData); err != nil {
		logging.Infof("Warning: could not parse wallet health response: %v", err)
		// Still return true as the wallet responded with 200
		return true, nil
	}

	// Log wallet status details
	logging.Infof("Wallet health check: status=%s, locked=%v, synced=%v, peers=%d",
		healthData.Status, healthData.WalletLocked, healthData.ChainSynced, healthData.PeerCount)

	// Only check if wallet is responding (status 200)
	// The other fields are informational only
	logging.Infof("Wallet health check: SUCCESS - wallet is responding")
	return true, nil
}
