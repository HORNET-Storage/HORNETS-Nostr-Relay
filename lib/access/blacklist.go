package access

import (
	"strings"
	"sync"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"
)

const (
	// blacklistPathPrefix is the irisdb path prefix for repository blacklist entries.
	// Full path: apps/git/repos/<repoGUID>/blacklist/<blockedPubkey>
	blacklistPathPrefix = "apps/git/repos/"
	blacklistPathSegment = "/blacklist/"
)

// RepoBlacklist maintains an in-memory cache of blacklisted pubkeys per repository.
// Entries are populated from kind 30078 events whose d-tag contains /blacklist/.
type RepoBlacklist struct {
	// repos maps repoGUID → set of blocked pubkeys
	repos sync.Map
}

// NewRepoBlacklist creates an empty blacklist cache.
func NewRepoBlacklist() *RepoBlacklist {
	return &RepoBlacklist{}
}

// IsBlacklisted checks whether a pubkey is blocked for the given repository.
func (bl *RepoBlacklist) IsBlacklisted(repoGUID string, pubkey string) bool {
	if repoGUID == "" || pubkey == "" {
		return false
	}
	pubkey = strings.ToLower(strings.TrimSpace(pubkey))

	entryRaw, ok := bl.repos.Load(repoGUID)
	if !ok {
		return false
	}

	blocked := entryRaw.(*sync.Map)
	_, isBlocked := blocked.Load(pubkey)
	return isBlocked
}

// ProcessBlacklistEvent inspects a kind 30078 event to determine if it is a
// blacklist entry and updates the cache accordingly. Only events from authorized
// publishers (owner + maintainers) should be processed — the caller is responsible
// for that validation.
//
// Returns true if the event was a blacklist entry (whether blocked or unblocked).
func (bl *RepoBlacklist) ProcessBlacklistEvent(event *nostr.Event) bool {
	if event == nil || event.Kind != repositoryAppDataKind {
		return false
	}

	dTag := firstTagValue(event.Tags, "d")
	if dTag == "" {
		return false
	}

	// Parse the d-tag path: apps/git/repos/<repoGUID>/blacklist/<blockedPubkey>
	repoGUID, blockedPubkey := parseBlacklistDTag(dTag)
	if repoGUID == "" || blockedPubkey == "" {
		return false
	}

	blockedPubkey = strings.ToLower(blockedPubkey)

	// Determine if this is a block (content has "true") or unblock (content is null/empty/false)
	isBlocked := isBlacklistBlockContent(event.Content)

	// Get or create the repo's blocked set
	entryRaw, _ := bl.repos.LoadOrStore(repoGUID, &sync.Map{})
	blocked := entryRaw.(*sync.Map)

	if isBlocked {
		blocked.Store(blockedPubkey, true)
		logging.Debugf("[BLACKLIST] Blocked pubkey %s from repo %s", blockedPubkey, repoGUID)
	} else {
		blocked.Delete(blockedPubkey)
		logging.Debugf("[BLACKLIST] Unblocked pubkey %s from repo %s", blockedPubkey, repoGUID)
	}

	return true
}

// PopulateFromStore scans existing kind 30078 events in the store to build
// the initial blacklist cache on relay startup.
func (bl *RepoBlacklist) PopulateFromStore(store stores.Store) {
	if store == nil {
		return
	}

	events, err := store.QueryEvents(nostr.Filter{
		Kinds: []int{repositoryAppDataKind},
	})
	if err != nil {
		logging.Warnf("[BLACKLIST] Failed to query blacklist events on startup: %v", err)
		return
	}

	processed := 0
	for _, event := range events {
		dTag := firstTagValue(event.Tags, "d")
		if strings.Contains(dTag, blacklistPathSegment) {
			bl.ProcessBlacklistEvent(event)
			processed++
		}
	}

	if processed > 0 {
		logging.Infof("[BLACKLIST] Loaded %d blacklist entries from store", processed)
	}
}

// IsBlacklistPublisherAuthorized checks whether the event publisher is authorized
// to manage the blacklist for the given permission event (owner + maintainers only).
func IsBlacklistPublisherAuthorized(publisherPubkey string, permissionEvent *nostr.Event) bool {
	if permissionEvent == nil || publisherPubkey == "" {
		return false
	}

	publisherPubkey = strings.ToLower(strings.TrimSpace(publisherPubkey))

	// Owner is always authorized
	if strings.ToLower(permissionEvent.PubKey) == publisherPubkey {
		return true
	}

	// Only maintainers (not triage, not write, not read)
	for _, tag := range permissionEvent.Tags {
		if len(tag) >= 3 && tag[0] == "p" && strings.ToLower(tag[1]) == publisherPubkey && strings.ToLower(tag[2]) == "maintainer" {
			return true
		}
	}

	return false
}

// parseBlacklistDTag extracts repoGUID and blockedPubkey from an irisdb d-tag path.
// Expected format: apps/git/repos/<repoGUID>/blacklist/<blockedPubkey>
// Returns empty strings if the path doesn't match the expected format.
func parseBlacklistDTag(dTag string) (repoGUID string, blockedPubkey string) {
	// Find the blacklist segment
	blacklistIdx := strings.Index(dTag, blacklistPathSegment)
	if blacklistIdx < 0 {
		return "", ""
	}

	// Extract the blocked pubkey (everything after /blacklist/)
	blockedPubkey = dTag[blacklistIdx+len(blacklistPathSegment):]
	if blockedPubkey == "" || strings.Contains(blockedPubkey, "/") {
		return "", "" // Invalid: empty or has additional path segments
	}

	// Extract the repo GUID (between apps/git/repos/ and /blacklist/)
	pathBefore := dTag[:blacklistIdx]
	if !strings.HasPrefix(pathBefore, blacklistPathPrefix) {
		return "", ""
	}
	repoGUID = pathBefore[len(blacklistPathPrefix):]
	if repoGUID == "" {
		return "", ""
	}

	return repoGUID, blockedPubkey
}

// isBlacklistBlockContent determines if the event content represents a block or unblock.
// irisdb stores true for blocked and null for unblocked. The content field in the
// kind 30078 event carries this value.
func isBlacklistBlockContent(content string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(content))
	// irisdb serializes boolean true as "true" and unblock as null/empty/"null"/"false"
	return trimmed == "true" || trimmed == "1"
}
