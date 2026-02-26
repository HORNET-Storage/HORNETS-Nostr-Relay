package kind72

import (
	"fmt"

	jsoniter "github.com/json-iterator/go"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"
)

// CascadeDeleteKind is the Nostr event kind for cascade resource deletion.
const CascadeDeleteKind = 72

// BuildKind72Handler returns a handler for cascade delete requests.
func BuildKind72Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		// ---- 1. Read & unmarshal ----
		data, err := read()
		if err != nil {
			write("NOTICE", "Error reading from stream.")
			return
		}

		var env nostr.EventEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			write("NOTICE", "Error unmarshaling event.")
			return
		}

		// ---- 2. Validate event (signature, kind, time) ----
		if !lib_nostr.ValidateEvent(write, env, CascadeDeleteKind) {
			return
		}

		// ---- 3. Extract required tags ----
		rTag := getTagValue(env.Event.Tags, "r")
		if rTag == "" {
			write("OK", env.Event.ID, false, "Missing required 'r' tag (resource identifier)")
			return
		}

		kTag := getTagValue(env.Event.Tags, "k")
		if kTag == "" {
			write("OK", env.Event.ID, false, "Missing required 'k' tag (permission event kind)")
			return
		}

		logging.Infof("[Kind72] Cascade delete request: resource=%s, permissionKind=%s, requester=%s",
			rTag, kTag, env.Event.PubKey)

		// ---- 4. Look up the permission resolver for this resource kind ----
		resolver := GetResolver(kTag)
		if resolver == nil {
			write("OK", env.Event.ID, false, fmt.Sprintf("No permission resolver registered for kind %s", kTag))
			return
		}

		// ---- 5. Resolve the resource owner ----
		ownerPubkey, err := resolver.ResolveOwner(store, rTag)
		if err != nil {
			logging.Errorf("[Kind72] Failed to resolve owner for resource %s: %v", rTag, err)
			write("OK", env.Event.ID, false, fmt.Sprintf("Failed to resolve resource owner: %v", err))
			return
		}

		// ---- 6. Authorize — only the resource owner may cascade delete ----
		if env.Event.PubKey != ownerPubkey {
			logging.Infof("[Kind72] Permission denied: requester %s is not owner %s of resource %s",
				env.Event.PubKey, ownerPubkey, rTag)
			write("OK", env.Event.ID, false, "Permission denied: only the resource owner can issue a cascade delete")
			return
		}

		// ---- 7. Query & delete ALL events with matching r tag ----
		// We loop in batches because a single query has a finite limit.
		// Each pass deletes what it finds; we stop when no events remain.
		const batchSize = 1000
		const maxPasses = 20 // safety valve to prevent infinite loops

		type dagOwnerRef struct {
			root   string
			pubkey string
		}
		var dagRefs []dagOwnerRef
		deletedCount := 0
		totalFound := 0

		for pass := 0; pass < maxPasses; pass++ {
			batch, err := store.QueryEvents(nostr.Filter{
				Tags:  nostr.TagMap{"r": []string{rTag}},
				Limit: batchSize,
			})
			if err != nil {
				logging.Errorf("[Kind72] Failed to query events for resource %s (pass %d): %v", rTag, pass, err)
				write("OK", env.Event.ID, false, "Failed to query resource events")
				return
			}

			if len(batch) == 0 {
				break
			}

			totalFound += len(batch)

			for _, evt := range batch {
				// Collect DAG references from push events (kind 73) before deletion
				for _, tag := range evt.Tags {
					if len(tag) >= 2 && (tag[0] == "bundle" || tag[0] == "archive") {
						dagRefs = append(dagRefs, dagOwnerRef{
							root:   tag[1],
							pubkey: evt.PubKey,
						})
					}
				}

				// Also collect DAG references from PR events (kind 74) — dag_root tag
				for _, tag := range evt.Tags {
					if len(tag) >= 2 && tag[0] == "dag_root" {
						dagRefs = append(dagRefs, dagOwnerRef{
							root:   tag[1],
							pubkey: evt.PubKey,
						})
					}
				}

				// Delete the event
				if err := store.DeleteEvent(evt.ID); err != nil {
					logging.Errorf("[Kind72] Failed to delete event %s (kind %d): %v", evt.ID, evt.Kind, err)
				} else {
					deletedCount++
				}
			}

			// If this batch was smaller than the limit, we've exhausted all events
			if len(batch) < batchSize {
				break
			}
		}

		logging.Infof("[Kind72] Deleted %d/%d events for resource %s", deletedCount, totalFound, rTag)

		// ---- 8. Release DAG ownership for each collected reference ----
		releasedCount := 0
		orphanedCount := 0

		// Deduplicate dag refs (same root+pubkey may appear in both bundle and archive)
		seen := map[string]bool{}
		for _, ref := range dagRefs {
			key := ref.root + ":" + ref.pubkey
			if seen[key] || ref.root == "" {
				continue
			}
			seen[key] = true

			if err := store.ReleaseOwnership(ref.root, ref.pubkey); err != nil {
				logging.Errorf("[Kind72] Failed to release ownership for DAG %s pubkey %s: %v",
					ref.root, ref.pubkey, err)
				continue
			}
			releasedCount++

			// Check if DAG is now orphaned
			hasOwners, err := store.HasOwnership(ref.root)
			if err != nil {
				logging.Errorf("[Kind72] Failed to check ownership for DAG %s: %v", ref.root, err)
				continue
			}
			if !hasOwners {
				orphanedCount++
				logging.Infof("[Kind72] DAG %s is now orphaned (zero owners)", ref.root)
			}
		}

		logging.Infof("[Kind72] Released %d DAG ownership records (%d DAGs now orphaned) for resource %s",
			releasedCount, orphanedCount, rTag)

		// ---- 9. Store the kind 72 event as a tombstone ----
		if err := store.StoreEvent(&env.Event); err != nil {
			logging.Errorf("[Kind72] Failed to store cascade delete tombstone: %v", err)
			// Non-fatal — the deletion itself already succeeded
		}

		write("OK", env.Event.ID, true, fmt.Sprintf(
			"Cascade delete complete: %d events deleted, %d DAG ownership records released",
			deletedCount, releasedCount))
	}

	return handler
}

// getTagValue extracts the first value for a given tag key.
func getTagValue(tags nostr.Tags, key string) string {
	for _, tag := range tags {
		if len(tag) >= 2 && tag[0] == key {
			return tag[1]
		}
	}
	return ""
}
