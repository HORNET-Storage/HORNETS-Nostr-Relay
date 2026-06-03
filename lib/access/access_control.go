package access

import (
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/nbd-wtf/go-nostr"
)

const (
	// accessCacheTTL is how long access check results are cached.
	accessCacheTTL                = 30 * time.Second
	repositoryPermissionEventKind = 16629
	repositoryPushEventKind       = 73
	repositoryPullRequestKind     = 74
	repositoryVisibilityPrivate   = "private"
	repositoryVisibilityPublic    = "public"
)

// cachedResult stores an access check result with expiry.
type cachedResult struct {
	err       error
	expiresAt time.Time
}

// AccessControl handles permission checking for H.O.R.N.E.T Allowed Users
type AccessControl struct {
	statsStore statistics.StatisticsStore
	settings   *types.AllowedUsersSettings

	// accessCache caches IsAllowed results keyed by "permission:hex"
	accessCache sync.Map
}

// NewAccessControl creates a new access control instance
func NewAccessControl(statsStore statistics.StatisticsStore, settings *types.AllowedUsersSettings) *AccessControl {
	return &AccessControl{
		statsStore: statsStore,
		settings:   settings,
	}
}

func (ac *AccessControl) CanRead(npub string) error {
	return ac.IsAllowed(ac.settings.Read, npub, false)
}

func (ac *AccessControl) CanWrite(npub string) error {
	return ac.IsAllowed(ac.settings.Write, npub, true)
}

func (ac *AccessControl) CanWriteEvent(event *nostr.Event, store stores.Store) error {
	if event == nil {
		return fmt.Errorf("event is required")
	}

	writeErr := ac.CanWrite(event.PubKey)
	if writeErr == nil {
		return nil
	}

	if !ac.repoAccessOverrideEnabled() || store == nil {
		return writeErr
	}

	if err := ac.canWriteRepositoryEvent(event, store); err != nil {
		logging.Debugf("[ACCESS CONTROL] Repository access override denied for pubkey %s on kind %d: %v", event.PubKey, event.Kind, err)
		return writeErr
	}

	logging.Debugf("[ACCESS CONTROL] Repository access override granted for pubkey %s on kind %d", event.PubKey, event.Kind)
	return nil
}

func (ac *AccessControl) CanReadEvent(event *nostr.Event, requesterPubkey string, store stores.Store) error {
	if event == nil {
		return fmt.Errorf("event is required")
	}

	globalReadErr := ac.CanRead(requesterPubkey)
	if globalReadErr == nil {
		return nil
	}

	if store == nil || !ac.repositoryAccessKindsConfigured() || !ac.isRepositoryEventEligible(event) {
		return globalReadErr
	}

	permissionEvent, err := ac.getRepositoryPermissionEvent(event, store)
	if err != nil {
		return globalReadErr
	}

	if repositoryPermissionVisibility(permissionEvent) != repositoryVisibilityPrivate {
		return nil
	}

	requesterPubkey = strings.ToLower(strings.TrimSpace(requesterPubkey))
	if requesterPubkey != "" && repoPermissionAllowsRead(permissionEvent, requesterPubkey) {
		return nil
	}

	return globalReadErr
}

func (ac *AccessControl) CanReadDag(root string, requesterPubkey string, requesterSignature string, store stores.Store) error {
	globalReadErr := ac.CanRead(requesterPubkey)
	if globalReadErr == nil {
		return nil
	}

	if store == nil || !ac.repositoryAccessKindsConfigured() {
		return globalReadErr
	}

	permissionEvent, err := ac.findRepositoryPermissionEventByRoot(root, store)
	if err != nil {
		return globalReadErr
	}

	if repositoryPermissionVisibility(permissionEvent) != repositoryVisibilityPrivate {
		return nil
	}

	requesterPubkey = strings.ToLower(strings.TrimSpace(requesterPubkey))
	if requesterPubkey == "" || requesterSignature == "" {
		return globalReadErr
	}

	if err := verifyRootRequestSignature(root, requesterPubkey, requesterSignature); err != nil {
		return globalReadErr
	}

	if repoPermissionAllowsRead(permissionEvent, requesterPubkey) {
		return nil
	}

	return globalReadErr
}

func (ac *AccessControl) IsAllowed(readOrWrite string, npub string, requireWriteCapability bool) error {
	readOrWrite = normalizeAccessSetting(readOrWrite)

	// Everyone is allowed if all_users is set
	if readOrWrite == "all_users" {
		return nil
	}

	// Validate that the public key is a valid 64-char hex string.
	// Per NIP-01, clients must send hex-encoded 32-byte pubkeys.
	if !isValidHexPubkey(npub) {
		return fmt.Errorf("invalid public key format: expected 64-character hex string")
	}
	hex := strings.ToLower(npub)

	// Check cache first
	cacheMode := "read"
	if requireWriteCapability {
		cacheMode = "write"
	}
	cacheKey := readOrWrite + ":" + cacheMode + ":" + hex
	if cached, ok := ac.accessCache.Load(cacheKey); ok {
		entry := cached.(*cachedResult)
		if time.Now().Before(entry.expiresAt) {
			return entry.err
		}
		// Expired — delete and re-check
		ac.accessCache.Delete(cacheKey)
	}

	// Perform the actual access check
	result := ac.isAllowedUncached(readOrWrite, hex, requireWriteCapability)

	// Cache the result
	ac.accessCache.Store(cacheKey, &cachedResult{
		err:       result,
		expiresAt: time.Now().Add(accessCacheTTL),
	})

	return result
}

func (ac *AccessControl) repoAccessOverrideEnabled() bool {
	if ac.settings == nil {
		return false
	}

	return normalizeAccessSetting(ac.settings.Mode) == "invite-only" &&
		normalizeAccessSetting(ac.settings.Write) == "allowed_users" &&
		len(ac.settings.RepoAccessOverrideKinds) > 0
}

func (ac *AccessControl) repositoryAccessKindsConfigured() bool {
	return ac.settings != nil && len(ac.settings.RepoAccessOverrideKinds) > 0
}

func (ac *AccessControl) isRepositoryEventEligible(event *nostr.Event) bool {
	if event == nil || !ac.repositoryAccessKindsConfigured() {
		return false
	}

	if firstTagValue(event.Tags, "r") == "" {
		return false
	}

	return containsKind(ac.settings.RepoAccessOverrideKinds, event.Kind)
}

func (ac *AccessControl) getRepositoryPermissionEvent(event *nostr.Event, store stores.Store) (*nostr.Event, error) {
	if event == nil {
		return nil, fmt.Errorf("event is required")
	}

	repoID := firstTagValue(event.Tags, "r")
	if repoID == "" {
		return nil, fmt.Errorf("missing repository identifier tag")
	}

	if event.Kind == repositoryPermissionEventKind {
		return event, nil
	}

	return ac.findRepositoryPermissionEvent(repoID, store)
}

func (ac *AccessControl) findRepositoryPermissionEvent(repoID string, store stores.Store) (*nostr.Event, error) {
	permissionEvents, err := store.QueryEvents(nostr.Filter{
		Kinds: []int{repositoryPermissionEventKind},
		Tags:  nostr.TagMap{"r": []string{repoID}},
		Limit: 1,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query repository permission event: %w", err)
	}
	if len(permissionEvents) == 0 {
		return nil, fmt.Errorf("repository permission event not found")
	}

	return permissionEvents[0], nil
}

func (ac *AccessControl) findRepositoryPermissionEventByRoot(root string, store stores.Store) (*nostr.Event, error) {
	lookupFilters := []nostr.Filter{
		{Kinds: []int{repositoryPushEventKind}, Tags: nostr.TagMap{"bundle": []string{root}}, Limit: 1},
		{Kinds: []int{repositoryPushEventKind}, Tags: nostr.TagMap{"archive": []string{root}}, Limit: 1},
		{Kinds: []int{repositoryPullRequestKind}, Tags: nostr.TagMap{"dag_root": []string{root}}, Limit: 1},
	}

	for _, filter := range lookupFilters {
		events, err := store.QueryEvents(filter)
		if err != nil || len(events) == 0 {
			continue
		}

		repoID := firstTagValue(events[0].Tags, "r")
		if repoID == "" {
			continue
		}

		return ac.findRepositoryPermissionEvent(repoID, store)
	}

	return nil, fmt.Errorf("repository permission event not found for dag root")
}

func (ac *AccessControl) canWriteRepositoryEvent(event *nostr.Event, store stores.Store) error {
	if !containsKind(ac.settings.RepoAccessOverrideKinds, event.Kind) {
		return fmt.Errorf("event kind %d is not eligible for repository access override", event.Kind)
	}

	pubkey := strings.ToLower(event.PubKey)
	if !isValidHexPubkey(pubkey) {
		return fmt.Errorf("invalid public key format: expected 64-character hex string")
	}

	repoID := firstTagValue(event.Tags, "r")
	if repoID == "" {
		return fmt.Errorf("missing repository identifier tag")
	}

	permissionEvents, err := store.QueryEvents(nostr.Filter{
		Kinds: []int{repositoryPermissionEventKind},
		Tags:  nostr.TagMap{"r": []string{repoID}},
		Limit: 1,
	})
	if err != nil {
		return fmt.Errorf("failed to query repository permission event: %w", err)
	}
	if len(permissionEvents) == 0 {
		return fmt.Errorf("repository permission event not found")
	}

	if repositoryEventRequiresDagWrite(event) {
		if !repoPermissionAllowsDagWrite(permissionEvents[0], pubkey) {
			return fmt.Errorf("pubkey does not have repository DAG write access")
		}
		return nil
	}

	if !repoPermissionAllowsEventWrite(permissionEvents[0], pubkey) {
		return fmt.Errorf("pubkey does not have repository event write access")
	}

	return nil
}

func firstTagValue(tags nostr.Tags, key string) string {
	for _, tag := range tags {
		if len(tag) >= 2 && tag[0] == key {
			return tag[1]
		}
	}
	return ""
}

func containsKind(kinds []int, kind int) bool {
	for _, allowedKind := range kinds {
		if allowedKind == kind {
			return true
		}
	}
	return false
}

func repoPermissionAllowsRead(permissionEvent *nostr.Event, pubkey string) bool {
	if permissionEvent == nil {
		return false
	}

	if strings.ToLower(permissionEvent.PubKey) == pubkey {
		return true
	}

	for _, tag := range permissionEvent.Tags {
		if len(tag) < 2 || tag[0] != "p" || strings.ToLower(tag[1]) != pubkey {
			continue
		}
		return true
	}

	return false
}

func repositoryPermissionVisibility(permissionEvent *nostr.Event) string {
	if permissionEvent == nil {
		return repositoryVisibilityPublic
	}

	visibility := strings.ToLower(strings.TrimSpace(firstTagValue(permissionEvent.Tags, "visibility")))
	if visibility == repositoryVisibilityPrivate {
		return repositoryVisibilityPrivate
	}

	return repositoryVisibilityPublic
}

func repoPermissionAllowsEventWrite(permissionEvent *nostr.Event, pubkey string) bool {
	if permissionEvent == nil {
		return false
	}

	if strings.ToLower(permissionEvent.PubKey) == pubkey {
		return true
	}

	for _, tag := range permissionEvent.Tags {
		if len(tag) < 3 || tag[0] != "p" || strings.ToLower(tag[1]) != pubkey {
			continue
		}

		for _, roleValue := range tag[2:] {
			switch strings.ToLower(strings.TrimSpace(roleValue)) {
			case "maintainer", "write", "triage", "encrypted-write":
				return true
			}
		}
	}

	return false
}

func repoPermissionAllowsDagWrite(permissionEvent *nostr.Event, pubkey string) bool {
	if permissionEvent == nil {
		return false
	}

	if strings.ToLower(permissionEvent.PubKey) == pubkey {
		return true
	}

	for _, tag := range permissionEvent.Tags {
		if len(tag) < 3 || tag[0] != "p" || strings.ToLower(tag[1]) != pubkey {
			continue
		}

		for _, roleValue := range tag[2:] {
			switch strings.ToLower(strings.TrimSpace(roleValue)) {
			case "maintainer", "write":
				return true
			}
		}
	}

	return false
}

func repositoryEventRequiresDagWrite(event *nostr.Event) bool {
	if event == nil {
		return false
	}

	switch event.Kind {
	case repositoryPushEventKind, repositoryPullRequestKind:
		return true
	}

	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}

		switch tag[0] {
		case "bundle", "archive", "dag_root":
			return true
		}
	}

	return false
}

func verifyRootRequestSignature(root string, requesterPubkey string, requesterSignature string) error {
	publicKey, err := signing.DeserializePublicKey(requesterPubkey)
	if err != nil {
		return err
	}

	signatureBytes, err := hex.DecodeString(requesterSignature)
	if err != nil {
		return err
	}

	parsedSignature, err := schnorr.ParseSignature(signatureBytes)
	if err != nil {
		return err
	}

	return signing.VerifySerializedCIDSignature(parsedSignature, root, publicKey)
}

// isAllowedUncached performs the actual DB-backed access check (no cache).
func (ac *AccessControl) isAllowedUncached(readOrWrite string, hex string, requireWriteCapability bool) error {
	logging.Debugf("Access check - Permission: %s, Mode: %s", readOrWrite, ac.settings.Mode)

	// The owner is always allowed
	if ac.isOwner(hex) {
		logging.Debugf("[ACCESS CONTROL] User %s is the relay owner, granting access", hex)
		return nil
	}

	if readOrWrite == "only-me" {
		return fmt.Errorf("user does not have permission")
	}

	// Get the allowed user from the database
	user, err := ac.statsStore.GetAllowedUser(hex)
	if err != nil {
		logging.Debugf("[ACCESS CONTROL] Error looking up user %s: %v", hex, err)
		return err
	}

	// User is not allowed if they don't exist
	if user == nil {
		logging.Debugf("[ACCESS CONTROL] User %s not found in allowed_users table", hex)
		return fmt.Errorf("user does not have permission to read")
	}

	if requireWriteCapability && user.ReadOnly {
		logging.Debugf("[ACCESS CONTROL] User %s is allowed for read but not write", hex)
		return fmt.Errorf("user does not have permission to write")
	}

	logging.Debugf("[ACCESS CONTROL] User %s found with tier: %s", hex, user.Tier)

	// Check if user has a paid tier if set to paid_users
	if readOrWrite == "paid_users" {
		logging.Debugf("[ACCESS CONTROL] Checking paid subscriber status for user: %s", hex)
		paidSubscriber, err := ac.statsStore.GetPaidSubscriberByNpub(hex)
		if err != nil {
			logging.Debugf("[ACCESS CONTROL] Error checking paid subscriber status: %v", err)
			return fmt.Errorf("user does not have permission")
		}

		if paidSubscriber == nil {
			logging.Debugf("[ACCESS CONTROL] User %s not found in paid subscribers table", hex)
			return fmt.Errorf("user does not have a paid subscription")
		}

		// Check if subscription is still valid
		if time.Now().After(paidSubscriber.ExpirationDate) {
			logging.Debugf("[ACCESS CONTROL] User %s subscription expired on %v", hex, paidSubscriber.ExpirationDate)
			return fmt.Errorf("user subscription has expired")
		}

		if paidSubscriber.Tier == "" {
			logging.Debugf("[ACCESS CONTROL] User %s has empty tier", hex)
			return fmt.Errorf("user does not have a valid subscription tier")
		}

		// Check if the tier is actually a paid tier
		cfg, err := config.GetConfig()
		if err == nil && cfg != nil {
			tierIsPaid := false
			for _, tier := range cfg.AllowedUsersSettings.Tiers {
				if tier.Name == paidSubscriber.Tier && tier.PriceSats > 0 {
					tierIsPaid = true
					break
				}
			}
			if !tierIsPaid {
				logging.Debugf("[ACCESS CONTROL] User %s has free/unpaid tier: %s", hex, paidSubscriber.Tier)
				return fmt.Errorf("user has a free tier, not a paid subscription")
			}
		} else {
			if strings.Contains(strings.ToLower(paidSubscriber.Tier), "free") || strings.Contains(strings.ToLower(paidSubscriber.Tier), "basic") {
				logging.Debugf("[ACCESS CONTROL] User %s has free tier: %s", hex, paidSubscriber.Tier)
				return fmt.Errorf("user has a free tier, not a paid subscription")
			}
		}

		logging.Debugf("[ACCESS CONTROL] User %s has valid paid subscription: %s", hex, paidSubscriber.Tier)
	}

	return nil
}

func (ac *AccessControl) AddAllowedUser(npub string, read bool, write bool, tier string, createdBy string) error {
	canWrite := write
	if !read && !write {
		canWrite = false
	}
	return ac.statsStore.AddAllowedUser(npub, canWrite, tier, createdBy)
}

func (ac *AccessControl) RemoveAllowedUser(npub string) error {
	return ac.statsStore.RemoveAllowedUser(npub)
}

// isValidHexPubkey checks if s is a valid 64-character hex-encoded public key.
func isValidHexPubkey(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// Is the incoming pub key the owner of the relay
func (ac *AccessControl) isOwner(hex string) bool {
	// First check database for relay owner
	if ac.statsStore != nil {
		owner, err := ac.statsStore.GetRelayOwner()
		if err == nil && owner != nil {
			ownerHex := strings.ToLower(owner.Npub)
			if hex == ownerHex {
				return true
			}
		}
	}

	// Fallback to config-based owner (for backwards compatibility)
	_, err := config.GetConfig()
	if err != nil {
		return false
	}

	// Note: The relay public key is not in the Config struct,
	// so we need to get it from the relay settings
	// For now, we'll skip this check if we can't get the config
	// The database check above should be sufficient
	return false
}

// ValidateSettings validates the access control settings for consistency
func (ac *AccessControl) ValidateSettings(settings *types.AllowedUsersSettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
	}

	// Validate mode
	mode := normalizeAccessSetting(settings.Mode)
	read := normalizeAccessSetting(settings.Read)
	write := normalizeAccessSetting(settings.Write)

	logging.Debugf("Write setting %s", write)
	// This ensures the correct options are selected for each mode and sets defaults when incorrect values are set
	// Not all read/write values are valid for each mode so this ensures that the read/write values are in line with the selected mode
	// mode: 		only-me, invite_only, public, subscription
	// read/write: 	all_users, paid_users, allowed_users, only-me

	switch mode {
	case "only-me":
		write = "only-me"
		switch read {
		case "only-me":
		case "all_users":
		case "allowed_users":
		default:
			read = "only-me"
		}
	case "invite-only":
		write = "allowed_users"
		switch read {
		case "all_users":
		case "allowed_users":
		default:
			read = "allowed_users"
		}
	case "public":
		write = "all_users"
		read = "all_users"
	case "subscription":
		write = "paid_users"
		switch read {
		case "all_users":
		case "paid_users":
		default:
			read = "paid_users"
		}
	default:
		mode = "only-me"
		read = "only-me"
		write = "only-me"
	}

	settings.Mode = mode
	settings.Read = read
	settings.Write = write

	return nil
}

func normalizeAccessSetting(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "only_me":
		return "only-me"
	case "invite_only":
		return "invite-only"
	default:
		return normalized
	}
}

// GetSettings returns the current access control settings
func (ac *AccessControl) GetSettings() *types.AllowedUsersSettings {
	return ac.settings
}

// UpdateSettings updates the access control settings and invalidates the access cache
func (ac *AccessControl) UpdateSettings(settings *types.AllowedUsersSettings) {
	logging.Infof("Updating access control settings - Mode: %s, Read: %s, Write: %s",
		settings.Mode, settings.Read, settings.Write)
	ac.settings = settings
	// Invalidate all cached access results since settings changed
	ac.InvalidateCache()
}

// InvalidateCache clears all cached access check results.
// Call this when allowed users, subscriptions, or settings change.
func (ac *AccessControl) InvalidateCache() {
	ac.accessCache.Range(func(key, value interface{}) bool {
		ac.accessCache.Delete(key)
		return true
	})
}
