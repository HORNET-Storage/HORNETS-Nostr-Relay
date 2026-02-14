package kind16629

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	jsoniter "github.com/json-iterator/go"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"
)

// Organization event kinds
const (
	OrgEventKind              = 39504
	OrgInvitationKind         = 39505
	OrgInvitationResponseKind = 39506
	DeletionEventKind         = 5
)

// uuidRegex matches UUID v4 format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// isGUID checks if a string is a valid UUID v4 format
func isGUID(s string) bool {
	return uuidRegex.MatchString(strings.ToLower(s))
}

func BuildKind16629Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		// Read data from the stream
		data, err := read()
		if err != nil {
			write("NOTICE", "Error reading data from stream")
			return
		}

		// Unmarshal the nostr envelope
		var env nostr.EventEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			write("NOTICE", "Failed to deserialize the event envelope")
			return
		}

		// Check relay settings for allowed events whilst also verifying signatures and kind number
		success := lib_nostr.ValidateEvent(write, env, -1)
		if !success {
			return
		}

		// Validate tags
		message := validateTags(env.Event.Tags, env.Event.PubKey)
		if len(message) > 0 {
			write("NOTICE", message)
			return
		}

		// Extract r tag (repo identifier - immutable, used as unique key)
		rTag := getTagValue(env.Event.Tags, "r")
		if rTag == "" {
			write("NOTICE", "Missing 'r' tag (repository identifier)")
			return
		}

		// Extract a tag (current ownership - determines if org or regular)
		aTag := getTagValue(env.Event.Tags, "a")

		// Determine ownership type and get the owner pubkey
		isOrgRepo, ownerPubkey, orgDtag := parseOwnership(aTag, rTag, env.Event.PubKey)

		logging.Infof("[Kind16629] Processing event - rTag: %s, aTag: %s, isOrg: %v, owner: %s, publisher: %s",
			rTag, aTag, isOrgRepo, ownerPubkey, env.Event.PubKey)

		// Query for existing events with the same r tag
		existingEvents, err := store.QueryEvents(nostr.Filter{
			Kinds: []int{16629},
			Tags: nostr.TagMap{
				"r": []string{rTag},
			},
		})
		if err != nil {
			logging.Errorf("[Kind16629] Error querying existing events: %v", err)
			write("NOTICE", "Failed to query existing events")
			return
		}

		isFirstEvent := len(existingEvents) == 0

		// Check if this is a migration scenario:
		// - New event has 'a' tag (migrating to org)
		// - Existing event(s) have no 'a' tag (was personal repo)
		isMigrationToOrg := false
		if isOrgRepo && !isFirstEvent {
			// Check if existing events lack the 'a' tag
			existingHasATag := false
			for _, existingEvent := range existingEvents {
				if getTagValue(existingEvent.Tags, "a") != "" {
					existingHasATag = true
					break
				}
			}
			// Migration: new event has 'a' tag, existing events don't
			isMigrationToOrg = !existingHasATag
			if isMigrationToOrg {
				logging.Infof("[Kind16629] Detected migration scenario: repo %s is being migrated to org", rTag)
			}
		}

		logging.Infof("[Kind16629] Found %d existing events for r tag: %s", len(existingEvents), rTag)

		// Verify the publisher has permission to create/update this event
		if !verifyPublisherPermission(store, env.Event.PubKey, isOrgRepo, ownerPubkey, orgDtag, isFirstEvent, isMigrationToOrg) {
			logging.Infof("[Kind16629] Permission denied for pubkey %s on repo %s (isOrg: %v, isFirst: %v, isMigration: %v)",
				env.Event.PubKey, rTag, isOrgRepo, isFirstEvent, isMigrationToOrg)
			write("NOTICE", "Permission denied: you are not authorized to update this repository's permissions")
			return
		}

		// Store the new event first
		if err := store.StoreEvent(&env.Event); err != nil {
			write("NOTICE", "Failed to store the event")
			return
		}

		logging.Infof("[Kind16629] Successfully stored new event %s", env.Event.ID)

		// After successful storage, delete ALL old events with the same r tag
		// (there should only be one, but delete all to fix any previous duplicates)
		deletedCount := 0
		for _, oldEvent := range existingEvents {
			if oldEvent.ID != env.Event.ID {
				if err := store.DeleteEvent(oldEvent.ID); err != nil {
					logging.Errorf("[Kind16629] Warning: failed to delete old event %s: %v", oldEvent.ID, err)
				} else {
					deletedCount++
					logging.Infof("[Kind16629] Deleted old event %s", oldEvent.ID)
				}
			}
		}

		if deletedCount > 0 {
			logging.Infof("[Kind16629] Deleted %d old events for r tag: %s", deletedCount, rTag)
		}

		// Successfully processed event
		write("OK", env.Event.ID, true, "Event stored successfully")
	}

	return handler
}

// getTagValue extracts the first value for a given tag key
func getTagValue(tags nostr.Tags, key string) string {
	for _, tag := range tags {
		if len(tag) >= 2 && tag[0] == key {
			return tag[1]
		}
	}
	return ""
}

// parseOwnership determines if the repo is org-owned and extracts the owner pubkey.
// Returns: (isOrgRepo, ownerPubkey, orgDtag)
// For org repos: ownerPubkey is the org owner, orgDtag is the org's d-tag
// For regular repos: ownerPubkey is the event publisher (signer), orgDtag is empty
//
// With GUID-based r tags, ownership for regular repos is determined by the event signer,
// not by parsing the r tag value.
func parseOwnership(aTag, rTag string, eventPubkey string) (bool, string, string) {
	// If a tag exists, use it to determine ownership
	// a tag format: "39504:<owner_pubkey>:<d_tag>" (colon-separated org address)
	if aTag != "" {
		parts := strings.SplitN(aTag, ":", 3)
		if len(parts) == 3 && parts[0] == "39504" {
			// Org repo: a tag is "39504:orgOwnerPubkey:orgDtag"
			orgOwnerPubkey := parts[1]
			orgDtag := parts[2]
			return true, orgOwnerPubkey, orgDtag
		}
		// Invalid a tag format - fall through to check r tag
	}

	// GUID r tag: ownership is determined by the event signer
	if isGUID(rTag) {
		return false, eventPubkey, ""
	}

	// Legacy format: check if r tag indicates an org repo
	// r tag format for org: "39504_orgOwnerPubkey_orgDtag:reponame" (underscore-encoded)
	// r tag format for regular: "pubkey:reponame"
	if strings.HasPrefix(rTag, "39504_") {
		// Org repo identified by r tag
		parts := strings.SplitN(rTag, ":", 2)
		if len(parts) >= 1 {
			orgAddr := parts[0] // "39504_orgOwnerPubkey_orgDtag"
			orgParts := strings.SplitN(orgAddr, "_", 3)
			if len(orgParts) >= 3 {
				orgOwnerPubkey := orgParts[1]
				orgDtag := orgParts[2]
				return true, orgOwnerPubkey, orgDtag
			}
		}
	}

	// Regular repo in legacy format: "pubkey:reponame"
	parts := strings.SplitN(rTag, ":", 2)
	if len(parts) >= 1 {
		return false, parts[0], ""
	}

	return false, "", ""
}

// verifyPublisherPermission checks if the publisher has permission to create/update the event
func verifyPublisherPermission(store stores.Store, publisherPubkey string, isOrgRepo bool, ownerPubkey string, orgDtag string, isFirstEvent bool, isMigrationToOrg bool) bool {
	if !isOrgRepo {
		// Regular repo: only the owner can create/update
		return publisherPubkey == ownerPubkey
	}

	// Org repo
	if isFirstEvent || isMigrationToOrg {
		// First event OR migration from personal to org: any verified org member can create/migrate
		return isVerifiedOrgMember(store, publisherPubkey, ownerPubkey, orgDtag)
	} else {
		// Replacement: only the org owner can update
		return publisherPubkey == ownerPubkey
	}
}

// isVerifiedOrgMember checks if a pubkey is a verified member of the organization
// This includes the org owner and any users who have accepted invitations
func isVerifiedOrgMember(store stores.Store, pubkey string, orgOwnerPubkey string, orgDtag string) bool {
	// Org owner is always a member
	if pubkey == orgOwnerPubkey {
		logging.Infof("[Kind16629] Pubkey %s is org owner", pubkey)
		return true
	}

	// Build the org address for querying: "39504:orgOwnerPubkey:orgDtag"
	orgAddress := fmt.Sprintf("%d:%s:%s", OrgEventKind, orgOwnerPubkey, orgDtag)

	logging.Infof("[Kind16629] Checking if %s is a verified member of org %s", pubkey, orgAddress)

	// Query for invitations to this user for this organization
	invitations, err := store.QueryEvents(nostr.Filter{
		Kinds:   []int{OrgInvitationKind},
		Authors: []string{orgOwnerPubkey},
		Tags: nostr.TagMap{
			"a": []string{orgAddress},
			"p": []string{pubkey},
		},
	})
	if err != nil {
		logging.Errorf("[Kind16629] Error querying invitations: %v", err)
		return false
	}

	logging.Infof("[Kind16629] Found %d invitations for pubkey %s", len(invitations), pubkey)

	// Check each invitation for a valid acceptance
	for _, invitation := range invitations {
		// Check if the invitation has been deleted
		if isEventDeleted(store, invitation.ID, orgOwnerPubkey) {
			logging.Infof("[Kind16629] Invitation %s has been deleted", invitation.ID)
			continue
		}

		// Query for acceptance responses to this invitation
		responses, err := store.QueryEvents(nostr.Filter{
			Kinds:   []int{OrgInvitationResponseKind},
			Authors: []string{pubkey},
			Tags: nostr.TagMap{
				"e": []string{invitation.ID},
			},
		})
		if err != nil {
			logging.Errorf("[Kind16629] Error querying invitation responses: %v", err)
			continue
		}

		// Check each response for "accepted" status
		for _, response := range responses {
			status := getTagValue(response.Tags, "status")
			if status == "accepted" {
				// Check if the acceptance has been deleted
				if isEventDeleted(store, response.ID, pubkey) {
					logging.Infof("[Kind16629] Acceptance %s has been deleted", response.ID)
					continue
				}

				logging.Infof("[Kind16629] Found valid acceptance for pubkey %s (invitation: %s, acceptance: %s)",
					pubkey, invitation.ID, response.ID)
				return true
			}
		}
	}

	logging.Infof("[Kind16629] Pubkey %s is NOT a verified org member", pubkey)
	return false
}

// isEventDeleted checks if an event has been deleted via a kind 5 deletion event
func isEventDeleted(store stores.Store, eventID string, authorPubkey string) bool {
	deletions, err := store.QueryEvents(nostr.Filter{
		Kinds:   []int{DeletionEventKind},
		Authors: []string{authorPubkey},
		Tags: nostr.TagMap{
			"e": []string{eventID},
		},
	})
	if err != nil {
		logging.Errorf("[Kind16629] Error checking deletion status: %v", err)
		return false
	}

	return len(deletions) > 0
}

// isValidHexPubkey checks if a string is a valid 64-character hex public key
func isValidHexPubkey(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// validateRepoIdentifier validates a repository identifier which can be in one of three formats:
// - GUID: UUID v4 format (e.g., "550e8400-e29b-41d4-a716-446655440000")
// - Regular repo (legacy): "pubkey:reponame" where pubkey is a 64-char hex string
// - Org repo (legacy): "39504_orgOwnerPubkey_orgDtag:reponame"
func validateRepoIdentifier(value string, tagName string) string {
	if value == "" {
		return fmt.Sprintf("'%s' tag value cannot be empty", tagName)
	}

	// Accept GUID format (UUID v4)
	if isGUID(value) {
		return ""
	}

	// Check if it's an org repo format
	if strings.HasPrefix(value, "39504_") {
		// Org repo format: "39504_orgOwnerPubkey_orgDtag:reponame"
		colonParts := strings.SplitN(value, ":", 2)
		if len(colonParts) != 2 {
			return fmt.Sprintf("Invalid '%s' tag format for org repo: expected '39504_pubkey_dtag:reponame'", tagName)
		}

		orgAddr := colonParts[0]  // "39504_orgOwnerPubkey_orgDtag"
		reponame := colonParts[1] // "reponame"

		if reponame == "" {
			return fmt.Sprintf("Invalid '%s' tag: repository name cannot be empty", tagName)
		}

		// Parse the org address part
		orgParts := strings.SplitN(orgAddr, "_", 3)
		if len(orgParts) < 3 {
			return fmt.Sprintf("Invalid '%s' tag format for org repo: expected '39504_pubkey_dtag:reponame'", tagName)
		}

		orgPubkey := orgParts[1]
		orgDtag := orgParts[2]

		if !isValidHexPubkey(orgPubkey) {
			return fmt.Sprintf("Invalid '%s' tag: org owner pubkey must be a 64-character hex string", tagName)
		}

		if orgDtag == "" {
			return fmt.Sprintf("Invalid '%s' tag: org d-tag cannot be empty", tagName)
		}

		return ""
	}

	// Regular repo format (legacy): "pubkey:reponame"
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		return fmt.Sprintf("Invalid '%s' tag format: expected GUID, 'pubkey:reponame', or '39504_pubkey_dtag:reponame'", tagName)
	}

	pubkey := parts[0]
	reponame := parts[1]

	if !isValidHexPubkey(pubkey) {
		return fmt.Sprintf("Invalid '%s' tag: pubkey must be a 64-character hex string", tagName)
	}

	if reponame == "" {
		return fmt.Sprintf("Invalid '%s' tag: repository name cannot be empty", tagName)
	}

	return ""
}

// validateRTag validates the format of the r tag
func validateRTag(value string) string {
	return validateRepoIdentifier(value, "r")
}

// validateATag validates the format of the a tag if present
// The 'a' tag is an org address in the format: "39504:<owner_pubkey>:<d_tag>"
// This is different from the repo identifier format used in 'r' tags
func validateATag(value string) string {
	if value == "" {
		return "" // a tag is optional
	}

	// The a tag should be an org address: "39504:<owner_pubkey>:<d_tag>"
	parts := strings.SplitN(value, ":", 3)
	if len(parts) != 3 {
		return "Invalid 'a' tag format: expected '39504:<owner_pubkey>:<d_tag>'"
	}

	kind := parts[0]
	ownerPubkey := parts[1]
	dTag := parts[2]

	if kind != "39504" {
		return fmt.Sprintf("Invalid 'a' tag: expected kind 39504, got %s", kind)
	}

	if !isValidHexPubkey(ownerPubkey) {
		return "Invalid 'a' tag: owner pubkey must be a 64-character hex string"
	}

	if dTag == "" {
		return "Invalid 'a' tag: d_tag cannot be empty"
	}

	return ""
}

// validateTags checks if the tags array contains the expected structure for a Kind 16629 event.
func validateTags(tags nostr.Tags, eventPubkey string) string {
	var rTagValue string
	var aTagValue string
	var nTagValue string
	var cloneTagValue string
	var relayTagValue string
	hasPermissionTag := false

	for _, tag := range tags {
		if len(tag) < 2 {
			continue
		}

		// Extract and validate the repository tag
		if tag[0] == "r" && len(tag) == 2 {
			rTagValue = tag[1]
		}

		// Extract the a tag for later validation
		if tag[0] == "a" && len(tag) >= 2 {
			aTagValue = tag[1]
		}

		// Extract the n tag (repo name for efficient queries)
		if tag[0] == "n" && len(tag) >= 2 {
			nTagValue = tag[1]
		}

		// Extract clone and relay tags
		if tag[0] == "clone" && len(tag) >= 2 {
			cloneTagValue = tag[1]
		}
		if tag[0] == "relay" && len(tag) >= 2 {
			relayTagValue = tag[1]
		}

		// Ensure at least one valid permission tag is present
		if tag[0] == "p" && len(tag) == 3 {
			permissionLevel := tag[2]
			if permissionLevel == "maintainer" || permissionLevel == "write" || permissionLevel == "triage" {
				hasPermissionTag = true
			} else {
				return "Invalid permission level: " + permissionLevel
			}
		}
	}

	// Validate required r tag
	if rTagValue == "" {
		return "Missing 'r' tag (repository identifier)."
	}

	// Validate r tag format
	if errMsg := validateRTag(rTagValue); errMsg != "" {
		return errMsg
	}

	// Validate required n tag (repo name for efficient relay queries)
	if nTagValue == "" {
		return "Missing 'n' tag (repository name for indexing)."
	}

	// For legacy r tag formats (not GUID), validate that n tag matches the repo name in r tag
	if !isGUID(rTagValue) {
		rParts := strings.SplitN(rTagValue, ":", 2)
		if len(rParts) == 2 && rParts[1] != nTagValue {
			return "The 'n' tag value must match the repository name in 'r' tag."
		}
	}

	// Validate a tag format if present
	if errMsg := validateATag(aTagValue); errMsg != "" {
		return errMsg
	}

	// Validate required clone tag
	if cloneTagValue == "" {
		return "Missing 'clone' tag (repository clone URL)."
	}
	if errMsg := validateCloneTag(cloneTagValue, rTagValue, nTagValue, aTagValue, eventPubkey); errMsg != "" {
		return errMsg
	}

	// Validate required relay tag
	if relayTagValue == "" {
		return "Missing 'relay' tag (relay WebSocket URL)."
	}
	if errMsg := validateRelayTag(relayTagValue); errMsg != "" {
		return errMsg
	}

	if !hasPermissionTag {
		return "Missing valid 'p' tag (authorized user and permission level)."
	}

	return ""
}

// validateCloneTag validates the clone tag format and consistency with other tags.
// Expected format: nestr://<host>[:<port>][/path]?id=<GUID>&repo_author=<author>&repo_name=<name>
func validateCloneTag(cloneURL, rTag, nTag, aTag, eventPubkey string) string {
	parsed, err := url.Parse(cloneURL)
	if err != nil {
		return fmt.Sprintf("Invalid 'clone' tag: failed to parse URL: %s", err)
	}

	// Must use nestr:// scheme
	if parsed.Scheme != "nestr" {
		return fmt.Sprintf("Invalid 'clone' tag: expected 'nestr://' scheme, got '%s://'", parsed.Scheme)
	}

	// Must have a host
	if parsed.Host == "" {
		return "Invalid 'clone' tag: missing host"
	}

	query := parsed.Query()

	// Must have required query parameters
	cloneID := query.Get("id")
	cloneRepoAuthor := query.Get("repo_author")
	cloneRepoName := query.Get("repo_name")

	if cloneID == "" {
		return "Invalid 'clone' tag: missing 'id' query parameter"
	}
	if cloneRepoAuthor == "" {
		return "Invalid 'clone' tag: missing 'repo_author' query parameter"
	}
	if cloneRepoName == "" {
		return "Invalid 'clone' tag: missing 'repo_name' query parameter"
	}

	// Cross-validate: clone URL 'id' must match the 'r' tag
	if cloneID != rTag {
		return fmt.Sprintf("Clone tag 'id' parameter (%s) does not match 'r' tag (%s)", cloneID, rTag)
	}

	// Cross-validate: clone URL 'repo_name' must match the 'n' tag
	if cloneRepoName != nTag {
		return fmt.Sprintf("Clone tag 'repo_name' parameter (%s) does not match 'n' tag (%s)", cloneRepoName, nTag)
	}

	// Cross-validate: clone URL 'repo_author' must be consistent with ownership
	if aTag != "" {
		// Org repo: repo_author should be the org address (e.g. 39504:pubkey:dtag)
		if cloneRepoAuthor != aTag {
			return fmt.Sprintf("Clone tag 'repo_author' parameter (%s) does not match 'a' tag (%s) for org repo", cloneRepoAuthor, aTag)
		}
	} else {
		// Personal repo: repo_author should be the event publisher's pubkey
		if cloneRepoAuthor != eventPubkey {
			return fmt.Sprintf("Clone tag 'repo_author' parameter (%s) does not match event pubkey (%s)", cloneRepoAuthor, eventPubkey)
		}
	}

	return ""
}

// validateRelayTag validates the relay tag format.
// Expected format: ws://<host>:<port>[/path] or wss://<host>[/path]
func validateRelayTag(relayURL string) string {
	parsed, err := url.Parse(relayURL)
	if err != nil {
		return fmt.Sprintf("Invalid 'relay' tag: failed to parse URL: %s", err)
	}

	if parsed.Scheme != "ws" && parsed.Scheme != "wss" {
		return fmt.Sprintf("Invalid 'relay' tag: expected 'ws://' or 'wss://' scheme, got '%s://'", parsed.Scheme)
	}

	if parsed.Host == "" {
		return "Invalid 'relay' tag: missing host"
	}

	return ""
}
