package contentfilter

import (
	"fmt"
	"sync"
	"time"
)

// Cache implements a thread-safe in-memory cache for filter results
type Cache struct {
	items    map[string]CacheItem
	maxItems int
	ttl      time.Duration
	mutex    sync.RWMutex
}

// NewCache creates a new filter cache with the specified size and TTL
func NewCache(maxItems int, ttl time.Duration) *Cache {
	return &Cache{
		items:    make(map[string]CacheItem),
		maxItems: maxItems,
		ttl:      ttl,
		mutex:    sync.RWMutex{},
	}
}

// Get retrieves a cached filter result if it exists and is not expired
func (c *Cache) Get(eventID, instructionsHash string) (FilterResult, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	key := fmt.Sprintf("%s:%s", eventID, instructionsHash)
	item, exists := c.items[key]
	if !exists {
		return FilterResult{}, false
	}

	// Check if the item has expired
	if time.Since(item.Timestamp) > c.ttl {
		return FilterResult{}, false
	}

	return item.Result, true
}

// Set adds or updates a filter result in the cache
func (c *Cache) Set(eventID, instructionsHash string, result FilterResult) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	key := fmt.Sprintf("%s:%s", eventID, instructionsHash)
	c.items[key] = CacheItem{
		Result:    result,
		Timestamp: time.Now(),
	}

	// If we're over capacity, remove oldest items
	if len(c.items) > c.maxItems {
		c.evictOldest()
	}
}

// Cleanup removes all expired items from the cache
func (c *Cache) Cleanup() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := time.Now()
	for key, item := range c.items {
		if now.Sub(item.Timestamp) > c.ttl {
			delete(c.items, key)
		}
	}
}

// evictOldest removes the oldest items from the cache when it's at capacity
func (c *Cache) evictOldest() {
	type keyAge struct {
		key       string
		timestamp time.Time
	}

	// Find the oldest 10% of items (or at least 1)
	itemsToRemove := max(c.maxItems/10, 1)
	if itemsToRemove > len(c.items) {
		itemsToRemove = len(c.items)
	}

	// Get all items sorted by age
	items := make([]keyAge, 0, len(c.items))
	for k, v := range c.items {
		items = append(items, keyAge{k, v.Timestamp})
	}

	// Simple selection sort to find the oldest N items
	for i := 0; i < itemsToRemove; i++ {
		oldest := i
		for j := i + 1; j < len(items); j++ {
			if items[j].timestamp.Before(items[oldest].timestamp) {
				oldest = j
			}
		}
		if oldest != i {
			items[i], items[oldest] = items[oldest], items[i]
		}

		// Remove this item from the cache
		delete(c.items, items[i].key)
	}
}

// Size returns the current number of items in the cache
func (c *Cache) Size() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return len(c.items)
}
