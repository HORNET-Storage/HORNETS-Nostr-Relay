package wot

import (
	"strings"
	"sync"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
)

const (
	// DefaultMaxHops is the default follow-distance limit when wot_hops is not
	// configured on the permission event.
	DefaultMaxHops = 3

	// MaxAllowedHops is the absolute upper bound for follow-distance checks.
	MaxAllowedHops = 5

	// DefaultMaxCacheEntries is the default LRU capacity.
	// Each entry is ~50KB (pubkey→distance map), so 100 entries ≈ 5MB.
	DefaultMaxCacheEntries = 100
)

// CachedGraph holds a pre-computed follow-distance map for a single WOT binary,
// keyed by the DAG root hash of the uploaded file.
type CachedGraph struct {
	// OwnerPubkey is the hex-encoded pubkey of the repository owner who uploaded this WOT.
	OwnerPubkey string

	// Distances maps hex-encoded pubkeys to their shortest follow-distance from the owner.
	Distances map[string]int
}

// LoaderFunc loads a WOT binary from persistent storage on cache miss.
// It receives the DAG root hash and returns the raw binary data, the owner
// pubkey, and any error. Returning an error means the graph cannot be loaded
// and the permission check should deny.
type LoaderFunc func(dagRootHash string) (binaryData []byte, ownerPubkey string, err error)

// Cache provides thread-safe, LRU-bounded storage and lookup of parsed WOT graphs,
// keyed by DAG root hash. On cache miss it calls the optional LoaderFunc to
// transparently reload from the store, so WOT gating survives relay restarts
// and LRU evictions without manual intervention.
type Cache struct {
	lru    *lru.Cache[string, *CachedGraph]
	loader LoaderFunc
	mu     sync.Mutex // serializes loader calls for the same key
}

// NewCache creates an LRU-bounded WOT cache.
func NewCache() *Cache {
	return NewCacheWithSize(DefaultMaxCacheEntries)
}

// NewCacheWithSize creates an LRU-bounded WOT cache with a custom capacity.
func NewCacheWithSize(maxEntries int) *Cache {
	if maxEntries <= 0 {
		maxEntries = DefaultMaxCacheEntries
	}
	l, _ := lru.New[string, *CachedGraph](maxEntries)
	return &Cache{lru: l}
}

// SetLoader sets the function used to load WOT binaries from persistent
// storage on cache miss. Must be called before any permission checks.
func (c *Cache) SetLoader(loader LoaderFunc) {
	c.loader = loader
}

// Store parses a WOT binary, computes distances from the owner, and caches the result.
// Returns an error if the binary cannot be parsed.
func (c *Cache) Store(dagRootHash string, ownerPubkey string, binaryData []byte) error {
	ownerPubkey = strings.ToLower(strings.TrimSpace(ownerPubkey))
	dagRootHash = strings.TrimSpace(dagRootHash)

	graph, err := ParseBinary(binaryData)
	if err != nil {
		return err
	}

	// Pre-compute distances up to MaxAllowedHops so lookups are O(1).
	distances := graph.ComputeAllDistances(ownerPubkey, MaxAllowedHops)
	if distances == nil {
		logging.Warnf("[WOT] Owner pubkey %s not found in WOT binary (root hash: %s)", ownerPubkey, dagRootHash)
		// Still cache an empty map so we don't re-parse on every check.
		distances = make(map[string]int)
	}

	c.lru.Add(dagRootHash, &CachedGraph{
		OwnerPubkey: ownerPubkey,
		Distances:   distances,
	})

	logging.Infof("[WOT] Cached WOT graph for owner %s (root hash: %s, %d reachable pubkeys)",
		ownerPubkey, dagRootHash, len(distances))

	return nil
}

// Lookup returns the cached WOT graph for the given DAG root hash.
// On cache miss, if a LoaderFunc is set, it transparently loads from the store.
// Returns nil if the graph is not cached and cannot be loaded.
func (c *Cache) Lookup(dagRootHash string) *CachedGraph {
	dagRootHash = strings.TrimSpace(dagRootHash)

	// Fast path: LRU hit
	if entry, ok := c.lru.Get(dagRootHash); ok {
		return entry
	}

	// Slow path: try to load from store
	if c.loader == nil {
		return nil
	}

	// Serialize loader calls for the same key to avoid duplicate work
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring lock (another goroutine may have loaded it)
	if entry, ok := c.lru.Get(dagRootHash); ok {
		return entry
	}

	binaryData, ownerPubkey, err := c.loader(dagRootHash)
	if err != nil {
		logging.Debugf("[WOT] Lazy load failed for root hash %s: %v", dagRootHash, err)
		return nil
	}

	if err := c.Store(dagRootHash, ownerPubkey, binaryData); err != nil {
		logging.Warnf("[WOT] Lazy load parse failed for root hash %s: %v", dagRootHash, err)
		return nil
	}

	logging.Infof("[WOT] Lazy-loaded WOT graph from store (root hash: %s)", dagRootHash)
	entry, _ := c.lru.Get(dagRootHash)
	return entry
}

// Invalidate removes a cached WOT graph by its DAG root hash.
func (c *Cache) Invalidate(dagRootHash string) {
	dagRootHash = strings.TrimSpace(dagRootHash)
	c.lru.Remove(dagRootHash)
	logging.Infof("[WOT] Invalidated WOT cache for root hash: %s", dagRootHash)
}

// IsWithinHops checks whether targetPubkey is within maxHops follow-distance
// of the owner in the WOT graph identified by dagRootHash.
// On cache miss, transparently loads from the store via the LoaderFunc.
// Returns false if the graph cannot be loaded or the target is unreachable.
func (c *Cache) IsWithinHops(dagRootHash string, targetPubkey string, maxHops int) bool {
	cached := c.Lookup(dagRootHash)
	if cached == nil {
		return false
	}

	targetPubkey = strings.ToLower(strings.TrimSpace(targetPubkey))

	// Owner is always within 0 hops
	if targetPubkey == cached.OwnerPubkey {
		return true
	}

	distance, ok := cached.Distances[targetPubkey]
	if !ok {
		return false
	}

	return distance <= maxHops
}

// Len returns the number of entries currently in the cache.
func (c *Cache) Len() int {
	return c.lru.Len()
}
