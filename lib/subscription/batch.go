// batch.go - Batch operations and scheduling

package subscription

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
)

// ScheduleBatchUpdateAfter schedules a batch update of all kind 11888 events after
// the specified delay. If called multiple times, it cancels any previous scheduled update
// and restarts the timer with a new delay (sliding window approach).
func ScheduleBatchUpdateAfter(delay time.Duration) {
	scheduledUpdateMutex.Lock()
	defer scheduledUpdateMutex.Unlock()

	// Cancel any existing scheduled update
	if scheduledUpdateTimer != nil {
		scheduledUpdateTimer.Stop()
		logging.Infof("Rescheduling batch update of kind 11888 events (settings changed again)")
	} else {
		logging.Infof("Scheduling batch update of kind 11888 events in %v", delay)
	}

	// Schedule new update
	scheduledUpdateTimer = time.AfterFunc(delay, func() {
		scheduledUpdateMutex.Lock()
		scheduledUpdateTimer = nil
		scheduledUpdateMutex.Unlock()

		logging.Infof("Starting batch update of kind 11888 events after %v cooldown", delay)
		manager := GetGlobalManager()
		if manager != nil {
			logging.Infof("DEBUG: BatchUpdateAllSubscriptionEvents called via scheduled update")
			if err := manager.BatchUpdateAllSubscriptionEvents(); err != nil {
				logging.Infof("Error in batch update: %v", err)
			} else {
				logging.Infof("Successfully completed batch update of kind 11888 events")
			}
		} else {
			logging.Infof("ERROR: Global subscription manager is nil, cannot run batch update")
		}
	})
}

// BatchUpdateAllSubscriptionEvents processes all kind 11888 events in batches
// to update storage allocations after allowed users settings have changed
func (m *SubscriptionManager) BatchUpdateAllSubscriptionEvents() error {
	logging.Infof("Starting batch update of all kind 11888 subscription events")

	// Get the current allowed users settings
	var allowedUsersSettings types.AllowedUsersSettings
	if err := viper.UnmarshalKey("allowed_users", &allowedUsersSettings); err != nil {
		return fmt.Errorf("failed to load allowed users settings: %v", err)
	}

	currentMode := strings.ToLower(allowedUsersSettings.Mode)

	// Process events in batches of 50
	batchSize := 50
	processed := 0

	for {
		// Query the next batch of events
		filter := nostr.Filter{
			Kinds: []int{11888},
			Limit: batchSize,
		}

		events, err := m.store.QueryEvents(filter)
		if err != nil {
			return fmt.Errorf("error querying events: %v", err)
		}

		// Exit if no more events
		if len(events) == 0 {
			break
		}

		// Process each event in the batch
		for _, event := range events {
			if err := m.processSingleSubscriptionEvent(event); err != nil {
				logging.Infof("Error processing event %s: %v", event.ID, err)
				// Continue with next event even if this one fails
			}

			processed++
		}

		logging.Infof("Processed %d kind 11888 events so far", processed)

		// If we received fewer events than requested, we've reached the end of available events
		if len(events) < batchSize {
			break
		}
	}

	logging.Infof("Completed batch update, processed %d kind 11888 events", processed)

	// Only allocate Bitcoin addresses in subscription (paid) mode
	// Never allocate addresses in free modes
	if currentMode == "subscription" {
		logging.Infof("Relay is in subscription mode, allocating Bitcoin addresses for users without them")
		if err := m.AllocateBitcoinAddressesForExistingUsers(); err != nil {
			logging.Infof("Error allocating Bitcoin addresses: %v", err)
			return fmt.Errorf("failed to allocate Bitcoin addresses: %v", err)
		}
	} else {
		logging.Infof("Relay is in %s mode, skipping Bitcoin address allocation", currentMode)
	}

	return nil
}

// processSingleSubscriptionEvent handles updating relay_mode tag and storage limits for existing kind 11888 events
func (m *SubscriptionManager) processSingleSubscriptionEvent(event *nostr.Event) error {
	// Extract pubkey
	pubkey := getTagValue(event.Tags, "p")
	if pubkey == "" {
		return fmt.Errorf("no pubkey found in event")
	}

	// Get current allowed users settings
	var allowedUsersSettings types.AllowedUsersSettings
	if err := viper.UnmarshalKey("allowed_users", &allowedUsersSettings); err != nil {
		return fmt.Errorf("failed to load allowed users settings: %v", err)
	}

	currentMode := strings.ToLower(allowedUsersSettings.Mode)
	
	// Check if user should be cleaned up in only-me or invite-only modes
	if currentMode == "only-me" || currentMode == "invite-only" {
		shouldDelete, err := m.shouldDeleteUserEvent(pubkey, currentMode, &allowedUsersSettings)
		if err != nil {
			logging.Infof("Error checking if user %s should be deleted: %v", pubkey, err)
			// Continue with normal processing if we can't determine
		} else if shouldDelete {
			// Delete the event for unauthorized users
			if err := m.deleteSubscriptionEvent(event); err != nil {
				logging.Infof("Error deleting subscription event for unauthorized user %s: %v", pubkey, err)
			} else {
				logging.Infof("Deleted kind 11888 event for unauthorized user %s in %s mode", pubkey, currentMode)
			}
			return nil // Skip further processing for deleted events
		}
	}

	// Check if relay_mode tag already exists and is current
	currentRelayMode := getTagValue(event.Tags, "relay_mode")
	expectedRelayMode := m.getRelayMode()

	// Get all existing event details
	storageInfo, err := m.extractStorageInfo(event)
	if err != nil {
		return fmt.Errorf("failed to extract storage info: %v", err)
	}

	// Get subscription details
	activeTier := getTagValue(event.Tags, "active_subscription")
	expirationUnix := getTagUnixValue(event.Tags, "active_subscription")
	expirationDate := time.Unix(expirationUnix, 0)
	address := getTagValue(event.Tags, "relay_bitcoin_address")
	status := getTagValue(event.Tags, "subscription_status")

	// Determine if this is a mode transition
	isTransition := currentRelayMode != "" && currentRelayMode != expectedRelayMode

	// Handle mode-specific logic
	needsUpdate := false

	if isTransition {
		logging.Infof("Mode transition detected for %s: %s -> %s", pubkey, currentRelayMode, expectedRelayMode)
		needsUpdate = true

		// Apply transition-specific rules
		needsUpdate = m.applyModeTransitionRules(
			pubkey,
			currentRelayMode,
			expectedRelayMode,
			&storageInfo,
			&activeTier,
		) || needsUpdate
	}

	// Check if user should have a tier based on current settings
	var currentTierObj *types.SubscriptionTier
	if activeTier != "" {
		for i := range allowedUsersSettings.Tiers {
			if allowedUsersSettings.Tiers[i].Name == activeTier {
				currentTierObj = &allowedUsersSettings.Tiers[i]
				break
			}
		}
	}

	logging.Infof("DEBUG processSingleSubscriptionEvent: pubkey=%s, currentRelayMode=%s, expectedRelayMode=%s, activeTier=%s, isTransition=%v",
		pubkey, currentRelayMode, expectedRelayMode, activeTier, isTransition)

	expectedTier := m.findAppropriateTierForUser(pubkey, currentTierObj, &allowedUsersSettings)

	if expectedTier != nil {
		logging.Infof("DEBUG: Expected tier for %s: Name=%s, Bytes=%d", pubkey, expectedTier.Name, expectedTier.MonthlyLimitBytes)
	} else {
		logging.Infof("DEBUG: No expected tier found for %s", pubkey)
	}

	// Check if relay mode needs update (if not already handled by transition)
	if !isTransition && currentRelayMode != expectedRelayMode {
		logging.Infof("Updating relay_mode for %s from '%s' to '%s'", pubkey, currentRelayMode, expectedRelayMode)
		needsUpdate = true
	}

	// Update storage limits based on mode and transition rules
	if expectedTier != nil {
		expectedBytes := expectedTier.MonthlyLimitBytes

		// Special handling for free-to-paid transitions
		if isTransition && isFreeMode(currentRelayMode) && !isFreeMode(expectedRelayMode) {
			// Keep current allocation until cycle ends
			logging.Infof("Free-to-paid transition for %s: keeping current allocation until cycle ends", pubkey)
			expectedBytes = storageInfo.TotalBytes
		} else if storageInfo.TotalBytes != expectedBytes {
			// For all other cases, update immediately
			logging.Infof("Updating storage limit for %s from %d to %d bytes", pubkey, storageInfo.TotalBytes, expectedBytes)
			storageInfo.TotalBytes = expectedBytes
			needsUpdate = true
		}

		// Update tier if it changed
		if activeTier != expectedTier.Name {
			logging.Infof("Updating tier for %s from '%s' to '%s'", pubkey, activeTier, expectedTier.Name)
			activeTier = expectedTier.Name
			needsUpdate = true
		}

		// Update status if needed
		if status == "inactive" && expectedBytes > 0 {
			logging.Infof("Activating subscription for %s", pubkey)
			needsUpdate = true
		}
	}

	// If nothing needs update, skip
	if !needsUpdate {
		logging.Infof("Event for %s already up to date (no changes needed)", pubkey)
		return nil
	}

	// Update the event with new values
	return m.createOrUpdateNIP88Event(&types.Subscriber{
		Npub:    pubkey,
		Address: address,
	}, activeTier, expirationDate, &storageInfo)
}

// AllocateBitcoinAddressesForExistingUsers allocates Bitcoin addresses for users who don't have them
func (m *SubscriptionManager) AllocateBitcoinAddressesForExistingUsers() error {
	logging.Infof("Starting batch Bitcoin address allocation for existing users")

	// Query all kind 11888 events first
	allEvents, err := m.store.QueryEvents(nostr.Filter{
		Kinds: []int{11888},
	})

	if err != nil {
		return fmt.Errorf("failed to query events: %v", err)
	}

	logging.Infof("Found %d total kind 11888 events, checking for empty Bitcoin addresses", len(allEvents))

	// Filter events that have empty Bitcoin addresses
	var eventsNeedingAddresses []*nostr.Event
	for _, event := range allEvents {
		bitcoinAddress := getTagValue(event.Tags, "relay_bitcoin_address")
		if bitcoinAddress == "" {
			eventsNeedingAddresses = append(eventsNeedingAddresses, event)
			pubkey := getTagValue(event.Tags, "p")
			logging.Infof("User %s needs Bitcoin address allocation", pubkey)
		}
	}

	usersNeedingAddresses := len(eventsNeedingAddresses)
	logging.Infof("Found %d users without Bitcoin addresses", usersNeedingAddresses)

	if usersNeedingAddresses == 0 {
		logging.Infof("No users need Bitcoin addresses, batch allocation complete")
		return nil
	}

	// Check if we have enough available addresses before starting
	if err := m.ensureSufficientAddresses(usersNeedingAddresses); err != nil {
		return fmt.Errorf("failed to ensure sufficient Bitcoin addresses: %v", err)
	}

	successCount := 0
	for _, event := range eventsNeedingAddresses {
		// Extract user pubkey
		pubkey := getTagValue(event.Tags, "p")
		if pubkey == "" {
			logging.Infof("Skipping event %s: no pubkey found", event.ID)
			continue
		}

		// Allocate Bitcoin address
		addressObj, err := m.store.GetStatsStore().AllocateBitcoinAddress(pubkey)
		if err != nil {
			logging.Infof("Failed to allocate address for %s: %v", pubkey, err)
			continue
		}

		// Update the kind 11888 event with the new address
		if err := m.updateEventWithBitcoinAddress(event, addressObj.Address); err != nil {
			logging.Infof("Failed to update kind 11888 event for %s: %v", pubkey, err)
			continue
		}

		successCount++
		logging.Infof("Allocated address %s for user %s", addressObj.Address, pubkey)
	}

	logging.Infof("Batch allocation complete: %d/%d successful", successCount, len(eventsNeedingAddresses))
	return nil
}

// updateEventWithBitcoinAddress updates an existing kind 11888 event to include a Bitcoin address
func (m *SubscriptionManager) updateEventWithBitcoinAddress(originalEvent *nostr.Event, bitcoinAddress string) error {
	// Extract current event data
	pubkey := getTagValue(originalEvent.Tags, "p")
	if pubkey == "" {
		return fmt.Errorf("no pubkey found in event")
	}

	// Extract storage info
	storageInfo, err := m.extractStorageInfo(originalEvent)
	if err != nil {
		return fmt.Errorf("failed to extract storage info: %v", err)
	}

	// Extract tier information
	var activeTier string
	var expirationDate time.Time
	for _, tag := range originalEvent.Tags {
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

	// Create updated subscriber with Bitcoin address
	subscriber := &types.Subscriber{
		Npub:    pubkey,
		Address: bitcoinAddress,
	}

	// Create a new event with the Bitcoin address
	return m.createOrUpdateNIP88Event(subscriber, activeTier, expirationDate, &storageInfo)
}

// ensureSufficientAddresses checks if enough addresses are available and requests more if needed
func (m *SubscriptionManager) ensureSufficientAddresses(usersNeedingAddresses int) error {
	// Add 20% buffer or minimum 50 addresses, whichever is higher
	bufferSize := int(float64(usersNeedingAddresses) * 0.2)
	if bufferSize < 50 {
		bufferSize = 50
	}

	requiredAddresses := usersNeedingAddresses + bufferSize
	maxRetries := 30 // Maximum 5 minutes of waiting (30 * 10 seconds)
	retryCount := 0

	for retryCount < maxRetries {
		// Check current available address count
		statsStore := m.store.GetStatsStore()
		availableCount, err := statsStore.GetAvailableBitcoinAddressCount()
		if err != nil {
			return fmt.Errorf("failed to check available address count: %v", err)
		}

		logging.Infof("Address check: %d available, %d required (%d users + %d buffer)",
			availableCount, requiredAddresses, usersNeedingAddresses, bufferSize)

		if availableCount >= requiredAddresses {
			logging.Infof("Sufficient addresses available (%d >= %d), proceeding with batch allocation",
				availableCount, requiredAddresses)
			return nil
		}

		// Calculate how many more addresses we need
		addressesNeeded := requiredAddresses - availableCount
		logging.Infof("Insufficient addresses, requesting %d more from wallet service...", addressesNeeded)

		// Request additional addresses
		if err := m.RequestNewAddresses(addressesNeeded); err != nil {
			logging.Infof("Warning: Failed to request addresses: %v", err)
		} else {
			logging.Infof("Successfully requested %d additional addresses from wallet", addressesNeeded)
		}

		// Wait 10 seconds before checking again
		logging.Infof("Waiting 10 seconds for wallet to generate addresses... (attempt %d/%d)", retryCount+1, maxRetries)
		time.Sleep(10 * time.Second)
		retryCount++
	}

	// If we've exhausted all retries, return an error
	return fmt.Errorf("timeout waiting for sufficient Bitcoin addresses after %d attempts (5 minutes)", maxRetries)
}

// shouldDeleteUserEvent checks if a user's subscription event should be deleted based on current access control
func (m *SubscriptionManager) shouldDeleteUserEvent(pubkey string, mode string, _ *types.AllowedUsersSettings) (bool, error) {
	switch mode {
	case "only-me":
		// Only the relay owner should have access in only-me mode
		if !m.isRelayOwner(pubkey) {
			return true, nil
		}
		return false, nil
		
	case "invite-only":
		// Check if user is in the allowed users list
		if !m.isUserInAllowedLists(pubkey) {
			return true, nil
		}
		return false, nil
		
	default:
		// For other modes, don't delete
		return false, nil
	}
}

// deleteSubscriptionEvent deletes a kind 11888 subscription event from the database
func (m *SubscriptionManager) deleteSubscriptionEvent(event *nostr.Event) error {
	// Use the store's delete method to remove the event
	if err := m.store.DeleteEvent(event.ID); err != nil {
		return fmt.Errorf("failed to delete event %s: %v", event.ID, err)
	}
	return nil
}

// isFreeMode checks if a mode is a free mode
func isFreeMode(mode string) bool {
	return mode == "public" || mode == "invite-only" || mode == "only-me"
}

// applyModeTransitionRules applies specific rules for mode transitions
func (m *SubscriptionManager) applyModeTransitionRules(
	pubkey string,
	oldMode string,
	newMode string,
	storageInfo *StorageInfo,
	activeTier *string,
) bool {
	needsUpdate := false

	// Handle only-me mode transitions
	if newMode == "only-me" {
		// Check if any owner is configured in database
		statsStore := m.store.GetStatsStore()
		if statsStore != nil {
			owner, err := statsStore.GetRelayOwner()
			if err != nil || owner == nil {
				logging.Infof("WARNING: Transitioning to only-me mode but no relay owner is set in database! Use /admin/owner API to set owner.")
			}
		}

		if m.isRelayOwner(pubkey) {
			// Owner gets unlimited storage
			logging.Infof("Setting unlimited storage for relay owner %s in only-me mode", pubkey)
			storageInfo.IsUnlimited = true
			storageInfo.TotalBytes = 0 // 0 indicates unlimited when IsUnlimited is true
			needsUpdate = true
		} else {
			// Non-owners lose access
			logging.Infof("Removing access for non-owner %s in only-me mode", pubkey)
			storageInfo.TotalBytes = 0
			storageInfo.IsUnlimited = false
			*activeTier = ""
			needsUpdate = true
		}
		return needsUpdate
	}

	// Handle transitions FROM only-me mode
	if oldMode == "only-me" && newMode != "only-me" {
		// Reset unlimited flag if it was set
		if storageInfo.IsUnlimited {
			storageInfo.IsUnlimited = false
			needsUpdate = true
		}
	}

	// Handle free-to-free transitions
	if isFreeMode(oldMode) && isFreeMode(newMode) {
		// Storage caps update immediately for free-to-free transitions
		logging.Infof("Free-to-free transition: updating storage caps immediately")
		// Force tier reassignment to pick up new tier configuration
		*activeTier = ""
		needsUpdate = true
	}

	// Handle free-to-paid transitions
	if isFreeMode(oldMode) && !isFreeMode(newMode) {
		// Keep current allocation until cycle ends
		logging.Infof("Free-to-paid transition: preserving current allocation until cycle ends")
		// Storage update will be handled in the main function
	}

	// Handle paid-to-free transitions
	if !isFreeMode(oldMode) && isFreeMode(newMode) {
		// Update storage caps immediately
		logging.Infof("Paid-to-free transition: updating storage caps immediately")
		needsUpdate = true
	}

	return needsUpdate
}

// isRelayOwner checks if the given pubkey is the relay owner
func (m *SubscriptionManager) isRelayOwner(pubkey string) bool {
	// Normalize user key first
	userHex, _, err := normalizePubkey(pubkey)
	if err != nil {
		logging.Infof("Error normalizing user key: %v", err)
		return false
	}

	// Check database for relay owner
	statsStore := m.store.GetStatsStore()
	if statsStore == nil {
		logging.Infof("WARNING: Statistics store not available, cannot check relay owner")
		return false
	}

	owner, err := statsStore.GetRelayOwner()
	if err != nil {
		// No owner set in database
		return false
	}

	if owner == nil {
		// No owner configured
		return false
	}

	// Normalize owner key and compare
	ownerHex, _, err := normalizePubkey(owner.Npub)
	if err != nil {
		logging.Infof("Error normalizing owner key from database: %v", err)
		return false
	}

	return ownerHex == userHex
}
