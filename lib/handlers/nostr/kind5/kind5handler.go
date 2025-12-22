package kind5

import (
	"fmt"
	"strings"

	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func BuildKind5Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		data, err := read()
		if err != nil {
			write("NOTICE", "Error reading from stream.")
			return
		}

		// Unmarshal the received data into a Nostr event
		var env nostr.EventEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			write("NOTICE", "Error unmarshaling event.")
			return
		}

		// Check relay settings for allowed events whilst also verifying signatures and kind number
		success := lib_nostr.ValidateEvent(write, env, 5)
		if !success {
			return
		}

		// Track if any deletion was processed
		anyDeletionProcessed := false

		// Process standard "e" tag deletions (NIP-09)
		for _, tag := range env.Event.Tags {
			if tag[0] == "e" && len(tag) > 1 {
				eventID := tag[1]
				// Retrieve the public key of the event to be deleted
				pubKey, err := extractPubKeyFromEventID(store, eventID)
				if err != nil {
					logging.Infof("Failed to extract public key for event %s: %v", eventID, err)
					write("NOTICE", fmt.Sprintf("Failed to extract public key for event %s: %v, the event doesn't exist", eventID, err))
					continue
				}

				logging.Infof("Found Public key:%s", pubKey)

				// Validate that the deletion request and the event have the same public key
				if pubKey == env.Event.PubKey {
					if err := store.DeleteEvent(eventID); err != nil {
						logging.Infof("Error deleting event %s: %v", eventID, err)
					} else {
						anyDeletionProcessed = true
						logging.Infof("Deleted event %s", eventID)
					}
				} else {
					logging.Infof("Public key mismatch for event %s, deletion request ignored", eventID)
					write("NOTICE", fmt.Sprintf("Public key mismatch for event %s, deletion request ignored", eventID))
				}
			}
		}

		// Process cascade deletions
		// Format: ["c", "<tag-name>", "<tag-value>"] (c = cascade)
		// Example: ["c", "r", "pubkey:reponame"]
		// This deletes all events with the specified tag that were created before this deletion event
		cascadeDagTags := extractCascadeDagTags(env.Event.Tags)

		for _, tag := range env.Event.Tags {
			if tag[0] == "c" && len(tag) >= 3 {
				tagName := tag[1]
				tagValue := tag[2]

				// Verify ownership: requestor pubkey must match the owner portion of the tag value
				// For format "owner-pubkey:resource-name", the owner is the first part before ":"
				if !verifyOwnership(env.Event.PubKey, tagValue) {
					logging.Infof("Cascade delete denied: pubkey %s does not own resource %s", env.Event.PubKey, tagValue)
					write("NOTICE", fmt.Sprintf("Not authorized to cascade delete: %s", tagValue))
					continue
				}

				logging.Infof("Processing cascade delete for tag [%s=%s] by owner %s", tagName, tagValue, env.Event.PubKey)

				// First, query events to collect DAG roots before deleting
				var dagRootsToDelete []string
				if len(cascadeDagTags) > 0 {
					dagRootsToDelete = collectDagRootsFromEvents(store, tagName, tagValue, int64(env.Event.CreatedAt), cascadeDagTags)
				}

				// Delete all events with this tag value created before this deletion event
				deletedEventIDs, err := store.DeleteEventsByTag(tagName, tagValue, int64(env.Event.CreatedAt))
				if err != nil {
					logging.Infof("Error during cascade delete for [%s=%s]: %v", tagName, tagValue, err)
					write("NOTICE", fmt.Sprintf("Error during cascade delete: %v", err))
					continue
				}

				logging.Infof("Cascade deleted %d events with tag [%s=%s]", len(deletedEventIDs), tagName, tagValue)

				// Delete the collected DAG roots
				dagRootsDeleted := 0
				for _, dagRoot := range dagRootsToDelete {
					if err := store.DeleteDag(dagRoot); err != nil {
						logging.Infof("Error deleting DAG %s: %v", dagRoot, err)
					} else {
						dagRootsDeleted++
						logging.Infof("Deleted DAG: %s", dagRoot)
					}
				}

				if len(cascadeDagTags) > 0 {
					logging.Infof("Deleted %d DAGs for cascade delete", dagRootsDeleted)
				}

				anyDeletionProcessed = true
			}
		}

		// Store the deletion event as a tombstone record
		if err := store.StoreEvent(&env.Event); err != nil {
			write("NOTICE", "Failed to store the deletion event")
			return
		}

		if anyDeletionProcessed {
			write("OK", env.Event.ID, true, "Deletion processed and tombstone stored")
		} else {
			write("OK", env.Event.ID, true, "Deletion event stored as tombstone")
		}
	}

	return handler
}

// verifyOwnership checks if the requestor owns the resource identified by tagValue.
// For tag values in the format "owner-pubkey:resource-name" or "owner-pubkey/resource-name",
// verifies that the requestor's pubkey matches the owner-pubkey portion.
// This is a generic ownership model that works for any "<owner>:<resource>" or "<owner>/<resource>" pattern.
// Supports multiple separators and nested paths like:
//   - pubkey:repo-name
//   - pubkey/repo-name
//   - pubkey:repo:branch
//   - pubkey/repo/sub/path
func verifyOwnership(requestorPubkey, tagValue string) bool {
	// Find the first separator (either ":" or "/")
	colonIdx := strings.Index(tagValue, ":")
	slashIdx := strings.Index(tagValue, "/")

	var separatorIdx int
	if colonIdx < 0 && slashIdx < 0 {
		// No separator found, can't verify ownership
		return false
	} else if colonIdx < 0 {
		separatorIdx = slashIdx
	} else if slashIdx < 0 {
		separatorIdx = colonIdx
	} else {
		// Both found, use whichever comes first
		if colonIdx < slashIdx {
			separatorIdx = colonIdx
		} else {
			separatorIdx = slashIdx
		}
	}

	ownerPubkey := tagValue[:separatorIdx]
	return ownerPubkey == requestorPubkey
}

// extractCascadeDagTags extracts tag names from ["d", "<tag-name>"] entries (d = DAG cascade)
// These specify which tags in the deleted events contain DAG roots to also delete
func extractCascadeDagTags(tags nostr.Tags) []string {
	var dagTags []string
	for _, tag := range tags {
		if tag[0] == "d" && len(tag) >= 2 {
			dagTags = append(dagTags, tag[1])
		}
	}
	return dagTags
}

// collectDagRootsFromEvents queries events with the specified tag and extracts DAG roots
// from the specified dag tag names (e.g., "bundle", "archive")
func collectDagRootsFromEvents(store stores.Store, tagName string, tagValue string, beforeTimestamp int64, dagTagNames []string) []string {
	// Query events with this tag
	filter := nostr.Filter{
		Tags: map[string][]string{
			tagName: {tagValue},
		},
		Until: func() *nostr.Timestamp {
			t := nostr.Timestamp(beforeTimestamp)
			return &t
		}(),
	}

	events, err := store.QueryEvents(filter)
	if err != nil {
		logging.Infof("Error querying events for DAG collection: %v", err)
		return nil
	}

	// Build a set of DAG roots to delete
	dagRoots := make(map[string]struct{})
	dagTagSet := make(map[string]struct{})
	for _, tagName := range dagTagNames {
		dagTagSet[tagName] = struct{}{}
	}

	for _, event := range events {
		for _, tag := range event.Tags {
			if len(tag) >= 2 {
				if _, isDagTag := dagTagSet[tag[0]]; isDagTag {
					dagRoots[tag[1]] = struct{}{}
				}
			}
		}
	}

	// Convert to slice
	result := make([]string, 0, len(dagRoots))
	for root := range dagRoots {
		result = append(result, root)
	}

	logging.Infof("Collected %d unique DAG roots from %d events", len(result), len(events))
	return result
}

func extractPubKeyFromEventID(store stores.Store, eventID string) (string, error) {
	events, err := store.QueryEvents(nostr.Filter{
		IDs: []string{eventID},
	})

	if err != nil {
		return "", err
	}

	if len(events) == 0 {
		return "", fmt.Errorf("no events found for ID: %s", eventID)
	}

	event := events[0]
	return event.PubKey, nil
}
