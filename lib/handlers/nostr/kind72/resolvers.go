package kind72

import (
	"fmt"
	"strings"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"
)

// ResourcePermissionResolver determines the owner of a resource identified by
// a GUID (r tag). Each resource kind registers its own resolver, allowing
// cascade deletion to remain fully generic.
type ResourcePermissionResolver interface {
	// ResolveOwner returns the pubkey of the resource owner.
	// The relay uses this to authorize a cascade delete request.
	ResolveOwner(store stores.Store, resourceID string) (ownerPubkey string, err error)
}

// resolverRegistry maps permission event kinds (as strings) to their resolver.
var resolverRegistry = map[string]ResourcePermissionResolver{}

// RegisterResolver registers a ResourcePermissionResolver for a given
// permission event kind. Call this at init time.
func RegisterResolver(kind string, resolver ResourcePermissionResolver) {
	resolverRegistry[kind] = resolver
}

// GetResolver returns the resolver for the given kind, or nil.
func GetResolver(kind string) ResourcePermissionResolver {
	return resolverRegistry[kind]
}

// ---------------------------------------------------------------------------
// Built-in resolver: Kind 16629 (Repository Permission Event)
// ---------------------------------------------------------------------------

func init() {
	RegisterResolver("16629", &RepoPermissionResolver{})
}

// RepoPermissionResolver resolves the owner of a Nestr git repository.
type RepoPermissionResolver struct{}

func (r *RepoPermissionResolver) ResolveOwner(store stores.Store, resourceID string) (string, error) {
	events, err := store.QueryEvents(nostr.Filter{
		Kinds: []int{16629},
		Tags:  nostr.TagMap{"r": []string{resourceID}},
	})
	if err != nil {
		return "", fmt.Errorf("failed to query permission event: %w", err)
	}
	if len(events) == 0 {
		return "", fmt.Errorf("no permission event found for resource %s", resourceID)
	}
	if len(events) > 1 {
		return "", fmt.Errorf("ambiguous ownership: multiple permission events found for resource %s", resourceID)
	}

	event := events[0]

	// Check for org ownership via the "a" tag: "39504:<orgOwnerPubkey>:<dtag>"
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "a" && strings.HasPrefix(tag[1], "39504:") {
			parts := strings.SplitN(tag[1], ":", 3)
			if len(parts) == 3 {
				logging.Infof("[Kind72/Resolver] Resolved org owner %s for resource %s", parts[1], resourceID)
				return parts[1], nil
			}
		}
	}

	// Personal repo — owner is the event signer
	logging.Infof("[Kind72/Resolver] Resolved personal owner %s for resource %s", event.PubKey, resourceID)
	return event.PubKey, nil
}
