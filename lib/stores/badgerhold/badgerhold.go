package badgerhold

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/dgraph-io/badger/v4"
	"github.com/fxamacker/cbor/v2"
	"github.com/gabriel-vasile/mimetype"
	"github.com/nbd-wtf/go-nostr"
	"go.uber.org/multierr"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/search"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics"
	statistics_gorm_sqlite "github.com/HORNET-Storage/hornet-storage/lib/stores/statistics/gorm/sqlite"
	"github.com/HORNET-Storage/hornet-storage/lib/transports/websocket"
	"github.com/timshannon/badgerhold/v4"
)

const (
	AddressStatusAvailable = "available"
	AddressStatusAllocated = "allocated"
	AddressStatusUsed      = "used"
)

type BadgerholdStore struct {
	Ctx context.Context

	DatabasePath string
	Database     *badgerhold.Store

	StatsDatabase statistics.StatisticsStore

	closed bool
	mu     sync.RWMutex
}

func cborEncode(value interface{}) ([]byte, error) {
	return cbor.Marshal(value)
}

func cborDecode(data []byte, value interface{}) error {
	return cbor.Unmarshal(data, value)
}

func InitStore(basepath string, args ...interface{}) (*BadgerholdStore, error) {
	// We no longer need to set the top-level moderation_mode as we're using the one in relay_settings

	store := &BadgerholdStore{}

	var err error

	store.Ctx = context.Background()

	store.DatabasePath = basepath

	options := badgerhold.DefaultOptions
	options.Encoder = cborEncode
	options.Decoder = cborDecode
	options.Dir = store.DatabasePath
	options.ValueDir = store.DatabasePath

	store.Database, err = badgerhold.Open(options)
	if err != nil {
		logging.Fatalf("Failed to open main database: %v", err)
	}

	// Check if a custom statistics database path was provided
	var statsDbPath string
	if len(args) > 0 {
		if path, ok := args[0].(string); ok {
			statsDbPath = path
		}
	}

	// Initialize statistics database with optional custom path
	if statsDbPath != "" {
		store.StatsDatabase, err = statistics_gorm_sqlite.InitStore(statsDbPath)
	} else {
		store.StatsDatabase, err = statistics_gorm_sqlite.InitStore()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gorm statistics database: %v", err)
	}

	// Start background garbage collection for Badger value logs
	// This prevents disk space from growing indefinitely due to old/deleted values
	go runBadgerGC(store.Database.Badger(), store.Ctx, "main", 10*time.Minute)

	return store, nil
}

func (store *BadgerholdStore) Cleanup() error {
	store.mu.Lock()
	defer store.mu.Unlock()

	if store.closed {
		return nil
	}
	store.closed = true

	var result error

	result = multierr.Append(result, store.Database.Close())
	result = multierr.Append(result, store.StatsDatabase.Close())

	return result
}

// IsClosed returns true if the store has been closed
func (store *BadgerholdStore) IsClosed() bool {
	store.mu.RLock()
	defer store.mu.RUnlock()
	return store.closed
}

// runBadgerGC runs Badger's value log garbage collection periodically.
func runBadgerGC(db *badger.DB, ctx context.Context, name string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logging.Infof("Started Badger value log GC for %s database (interval: %v)", name, interval)

	for {
		select {
		case <-ticker.C:
			// Run GC with 0.5 discard ratio (recommended by Badger team)
			// This means: compact vlog files that are >50% garbage
			err := db.RunValueLogGC(0.5)
			switch err {
			case nil:
				logging.Infof("Badger GC completed successfully for %s database", name)
			case badger.ErrNoRewrite:
				// No files needed compaction - this is normal and fine
				logging.Debugf("Badger GC: No rewrite needed for %s database", name)
			default:
				logging.Infof("Badger GC error for %s database: %v", name, err)
			}
		case <-ctx.Done():
			logging.Infof("Stopping Badger GC for %s database", name)
			return
		}
	}
}

func (store *BadgerholdStore) GetStatsStore() statistics.StatisticsStore {
	return store.StatsDatabase
}

func (store *BadgerholdStore) QueryEvents(filter nostr.Filter) ([]*nostr.Event, error) {
	// Check if store is closed before attempting database operations
	if store.IsClosed() {
		return nil, fmt.Errorf("database is closed")
	}

	var results []types.NostrEvent

	jd, _ := json.Marshal(filter)
	logging.Infof("%s", string(jd))

	query := badgerhold.Where("ID").Ne("")
	first := true

	if len(filter.Kinds) > 0 {
		kindsAsInterface := make([]interface{}, len(filter.Kinds))
		for i, kind := range filter.Kinds {
			kindsAsInterface[i] = strconv.Itoa(kind)
		}

		if first {
			query = badgerhold.Where("Kind").In(kindsAsInterface...)
			first = false
		} else {
			query = query.And("Kind").In(kindsAsInterface...)
		}
	}

	if len(filter.Authors) > 0 {
		authorsAsInterface := make([]interface{}, len(filter.Authors))
		for i, author := range filter.Authors {
			authorsAsInterface[i] = author
		}

		if first {
			query = badgerhold.Where("PubKey").In(authorsAsInterface...)
			first = false
		} else {
			query = query.And("PubKey").In(authorsAsInterface...)
		}
	}

	if filter.Since != nil {
		query = query.And("CreatedAt").Ge(*filter.Since)
	}
	if filter.Until != nil {
		query = query.And("CreatedAt").Le(*filter.Until)
	}

	if len(filter.Tags) > 0 {
		eventIDSet := make(map[string]struct{})

		isFirst := true

		for tagName, tagValues := range filter.Tags {
			var tagEntries []types.TagEntry

			err := store.Database.Find(&tagEntries, badgerhold.Where("TagName").Eq(strings.ReplaceAll(tagName, "#", "")).And("TagValue").In(toInterfaceSlice(tagValues)...))
			if err != nil && err != badgerhold.ErrNotFound {
				return nil, fmt.Errorf("failed to query tag entries for %s: %w", tagName, err)
			}

			tempEventIDs := make(map[string]struct{})
			for _, entry := range tagEntries {
				tempEventIDs[entry.EventID] = struct{}{}
			}

			if isFirst {
				eventIDSet = tempEventIDs
				isFirst = false
			} else {
				for id := range eventIDSet {
					if _, exists := tempEventIDs[id]; !exists {
						delete(eventIDSet, id)
					}
				}
			}
		}

		eventIDs := make([]string, 0, len(eventIDSet))
		for id := range eventIDSet {
			eventIDs = append(eventIDs, id)
		}

		if len(eventIDs) == 0 {
			logging.Infof("No matching events from tags")
			return []*nostr.Event{}, nil
		}

		logging.Infof("Found %d events from tags\n", len(eventIDs))

		if first {
			query = badgerhold.Where("ID").In(toInterfaceSlice(eventIDs)...)
			first = false
		} else {
			query = query.And("ID").In(toInterfaceSlice(eventIDs)...)
		}
	}

	err := store.Database.Find(&results, query)
	if err != nil && err != badgerhold.ErrNotFound {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}

	var events []*nostr.Event
	for _, event := range results {
		events = append(events, UnwrapEvent(&event))
	}

	filteredEvents := postFilterEvents(events, filter)

	sortEventsByCreatedAt(filteredEvents)

	if filter.Limit > 0 && len(filteredEvents) > filter.Limit {
		filteredEvents = filteredEvents[:filter.Limit]
	}

	return filteredEvents, nil
}

func sortEventsByCreatedAt(events []*nostr.Event) {
	sort.Slice(events, func(i, j int) bool {
		return events[i].CreatedAt.Time().After(events[j].CreatedAt.Time())
	})
}

func toInterfaceSlice[T any](items []T) []interface{} {
	interfaceSlice := make([]interface{}, len(items))
	for i, item := range items {
		interfaceSlice[i] = item
	}
	return interfaceSlice
}

func postFilterEvents(events []*nostr.Event, filter nostr.Filter) []*nostr.Event {
	var filtered []*nostr.Event

	// Parse search query if present
	var searchQuery search.SearchQuery
	if filter.Search != "" {
		searchQuery = search.ParseSearchQuery(filter.Search)
	}

	for _, event := range events {
		// Match event ID (if specified)
		if len(filter.IDs) > 0 && !contains(filter.IDs, event.ID) {
			continue
		}

		// Match event tags (handling OR conditions)
		if len(filter.Tags) > 0 {
			matchesTag := false
			for tagName, tagValues := range filter.Tags {
				if eventHasTag(event, tagName, tagValues) {
					matchesTag = true
					break
				}
			}
			if !matchesTag {
				continue
			}
		}

		// Match search term (if specified)
		if searchQuery.Text != "" && !strings.Contains(strings.ToLower(event.Content), strings.ToLower(searchQuery.Text)) {
			continue
		}

		// If the event passes all checks, add it to the results
		filtered = append(filtered, event)
	}

	return filtered
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func eventHasTag(event *nostr.Event, tagName string, tagValues []string) bool {
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == tagName {
			if contains(tagValues, tag[1]) {
				return true
			}
		}
	}
	return false
}

// ExtractMediaURLsFromEvent extracts all media (image and video) URLs from a Nostr event
func ExtractMediaURLsFromEvent(event *nostr.Event) []string {
	// Common media file extensions
	imageExtensions := []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg", ".avif"}
	videoExtensions := []string{".mp4", ".webm", ".mov", ".avi", ".mkv", ".m4v", ".ogv", ".mpg", ".mpeg"}
	mediaExtensions := append(append([]string{}, imageExtensions...), videoExtensions...)

	// Common media hosting services
	mediaHostingPatterns := []string{
		// Image hosting services
		"imgur.com",
		"nostr.build/i/",
		"nostr.build/p/",
		"image.nostr.build",
		"i.nostr.build",

		// Video hosting services
		"nostr.build/v/",
		"v.nostr.build",
		"video.nostr.build",
		"youtube.com/watch",
		"youtu.be/",
		"vimeo.com/",

		// Generic hosting
		"void.cat",
		"primal.net/",
		"pbs.twimg.com",
	}

	// URL extraction regex
	urlRegex := regexp.MustCompile(`https?://[^\s<>"']+`)

	var urls []string
	seen := make(map[string]bool) // Track seen URLs to avoid duplicates

	// Extract from content text
	contentURLs := urlRegex.FindAllString(event.Content, -1)
	for _, url := range contentURLs {
		url = strings.Split(url, "?")[0] // Remove query parameters
		urlLower := strings.ToLower(url)

		// Check for file extensions
		for _, ext := range mediaExtensions {
			if strings.HasSuffix(urlLower, ext) && !seen[url] {
				urls = append(urls, url)
				seen[url] = true
				break
			}
		}

		// Check for common media hosting services
		for _, pattern := range mediaHostingPatterns {
			if strings.Contains(urlLower, pattern) && !seen[url] {
				urls = append(urls, url)
				seen[url] = true
				break
			}
		}
	}

	// Extract from r tags (common in Nostr Build)
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "r" {
			url := tag[1]
			url = strings.Split(url, "?")[0] // Remove query parameters
			urlLower := strings.ToLower(url)

			// Check extensions
			for _, ext := range mediaExtensions {
				if strings.HasSuffix(urlLower, ext) && !seen[url] {
					urls = append(urls, url)
					seen[url] = true
					break
				}
			}

			// Check hosting services
			for _, pattern := range mediaHostingPatterns {
				if strings.Contains(urlLower, pattern) && !seen[url] {
					urls = append(urls, url)
					seen[url] = true
					break
				}
			}
		}
	}

	// Extract from imeta and vmeta tags
	for _, tag := range event.Tags {
		if len(tag) >= 2 && (tag[0] == "imeta" || tag[0] == "vmeta") {
			for _, value := range tag[1:] {
				if strings.HasPrefix(value, "url ") {
					mediaURL := strings.TrimPrefix(value, "url ")
					if !seen[mediaURL] {
						urls = append(urls, mediaURL)
						seen[mediaURL] = true
					}
				}
			}
		}
	}

	return urls
}

// For backward compatibility
func ExtractImageURLsFromEvent(event *nostr.Event) []string {
	return ExtractMediaURLsFromEvent(event)
}

func (store *BadgerholdStore) StoreEvent(ev *nostr.Event) error {
	event := WrapEvent(ev)

	err := store.Database.Upsert(event.ID, event)
	if err != nil {
		return fmt.Errorf("failed to store nostr event: %w", err)
	}

	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}

		if len(tag[0]) != 1 {
			continue
		}

		entry := types.TagEntry{
			EventID:  event.ID,
			TagName:  tag[0],
			TagValue: tag[1],
		}

		key := fmt.Sprintf("tag:%s:%s:%s", tag[0], tag[1], event.ID)

		err := store.Database.Upsert(key, entry)
		if err != nil {
			return fmt.Errorf("failed to store tag entry: %w", err)
		}
	}

	// Record event statistics
	if store.StatsDatabase != nil {
		err = store.StatsDatabase.SaveEventKind(ev)
		if err != nil {
			// Log the error but don't fail the operation
			logging.Infof("Failed to record event statistics: %v\n", err)
		}
	}

	// Update search index for text events
	if err := store.UpdateSearchIndex(ev); err != nil {
		// Log the error but don't fail the operation
		logging.Infof("Failed to update search index for event %s: %v\n", ev.ID, err)
	}

	// Check for images that need moderation - only if image moderation is enabled
	if cfg, err := config.GetConfig(); err != nil {
		logging.Infof("Failed to get config for image moderation check: %v", err)
	} else if cfg.ContentFiltering.ImageModeration.Enabled {
		// Extract image URLs from the event using our image extractor
		imageURLs := ExtractImageURLsFromEvent(ev)
		if len(imageURLs) > 0 {
			// Check if we should bypass moderation for exclusive mode
			if ac := websocket.GetAccessControl(); ac != nil {
				if settings := ac.GetSettings(); settings != nil && strings.ToLower(settings.Mode) == "exclusive" {
					logging.Infof("Event %s contains %d images, but skipping moderation in exclusive mode", ev.ID, len(imageURLs))
					// Skip moderation entirely for exclusive mode
				} else {
					// Continue with moderation for free and paid modes
					logging.Infof("Event %s contains %d images, adding to moderation queue", ev.ID, len(imageURLs))
					err = store.AddToPendingModeration(ev.ID, imageURLs)
					if err != nil {
						logging.Infof("Failed to add event %s to pending moderation: %v", ev.ID, err)
					}
				}
			} else {
				// Fallback to current behavior if access control not available
				logging.Infof("Event %s contains %d images, adding to moderation queue (fallback)", ev.ID, len(imageURLs))
				err = store.AddToPendingModeration(ev.ID, imageURLs)
				if err != nil {
					logging.Infof("Failed to add event %s to pending moderation: %v", ev.ID, err)
				}
			}
		}
	}

	return nil
}

func (store *BadgerholdStore) DeleteEvent(eventID string) error {
	err := store.Database.Delete(eventID, types.NostrEvent{})
	if err != nil {
		return fmt.Errorf("failed to find event to delete: %w", err)
	}

	// Remove event from statistics
	if store.StatsDatabase != nil {
		err = store.StatsDatabase.DeleteEventByID(eventID)
		if err != nil {
			// Log the error but don't fail the operation
			logging.Infof("Failed to delete event from statistics: %v\n", err)
		}
	}

	// Remove from search index
	if err := store.RemoveFromSearchIndex(eventID); err != nil {
		// Log the error but don't fail the operation
		logging.Infof("Failed to remove event %s from search index: %v\n", eventID, err)
	}

	return nil
}

// Blossom Blobs (unchunked data)
func (store *BadgerholdStore) StoreBlob(data []byte, hash []byte, publicKey string) error {
	encodedHash := hex.EncodeToString(hash)

	mtype := mimetype.Detect(data)

	content := types.BlobContent{
		Hash:    encodedHash,
		PubKey:  publicKey,
		Content: data,
	}

	err := store.Database.Upsert(encodedHash, content)
	if err != nil {
		return err
	}

	// Record file statistics
	if store.StatsDatabase != nil {
		err = store.StatsDatabase.SaveFile(
			encodedHash,      // Using hash as root
			encodedHash,      // Using hash as hash too
			"",               // No filename available for blobs
			mtype.String(),   // MIME type
			1,                // Leaf count is 1 for blobs
			int64(len(data)), // Size in bytes
		)
		if err != nil {
			// Log the error but don't fail the operation
			logging.Infof("Failed to record blob statistics: %v\n", err)
		}
	}

	return nil
}

func (store *BadgerholdStore) GetBlob(hash string) ([]byte, error) {
	var content types.BlobContent

	err := store.Database.Get(hash, &content)
	if err != nil {
		return nil, err
	}

	return content.Content, nil
}

func (store *BadgerholdStore) DeleteBlob(hash string) error {
	err := store.Database.Delete(hash, types.BlobContent{})
	if err != nil {
		return err
	}

	return nil
}

func (store *BadgerholdStore) QueryBlobs(mimeType string) ([]string, error) {

	return nil, nil
}

func GetKindFromItemName(itemName string) string {
	parts := strings.Split(itemName, ".")
	return parts[len(parts)-1]
}

func GetAppNameFromPath(path string) string {
	path = strings.TrimPrefix(path, "/")

	parts := strings.Split(path, "/")

	if len(parts) > 0 {
		return parts[0]
	}

	return ""
}

// Helper functions for dealing with event tags
func IsSingleLetter(s string) bool {
	if len(s) != 1 {
		return false
	}
	r := rune(s[0])
	return unicode.IsLower(r) && unicode.IsLetter(r)
}

func IsTagQueryTag(s string) bool {
	return len(s) == 2 && s[0] == '#' && IsSingleLetter(string(s[1]))
}

func ContainsAnyWithWildcard(tags nostr.Tags, tagName string, values []string) bool {
	tagName = strings.TrimPrefix(tagName, "#")
	for _, tag := range tags {
		if len(tag) < 2 {
			continue
		}

		if tag[0] != tagName {
			continue
		}

		for _, value := range values {
			if tagName == "f" || tagName == "d" {
				if matchWildcard(value, tag[1]) {
					return true
				}
			} else {
				if value == tag[1] {
					return true
				}
			}
		}
	}

	return false
}

func matchWildcard(pattern, value string) bool {
	patternParts := strings.Split(pattern, "/")
	valueParts := strings.Split(value, "/")

	patternIndex, valueIndex := 0, 0

	for patternIndex < len(patternParts) && valueIndex < len(valueParts) {
		switch patternParts[patternIndex] {
		case "*":
			patternIndex++
			if patternIndex == len(patternParts) {
				return true // "*" at the end matches everything remaining
			}
			// Find the next matching part
			for valueIndex < len(valueParts) && valueParts[valueIndex] != patternParts[patternIndex] {
				valueIndex++
			}
		case valueParts[valueIndex]:
			patternIndex++
			valueIndex++
		default:
			return false
		}
	}

	// Check if we've matched all parts
	return patternIndex == len(patternParts) && valueIndex == len(valueParts)
}

func ContainsAny(tags nostr.Tags, tagName string, values []string) bool {
	tagName = strings.TrimPrefix(tagName, "#")
	for _, tag := range tags {
		if len(tag) < 2 {
			continue
		}

		if tag[0] != tagName {
			continue
		}

		if slices.Contains(values, tag[1]) {
			return true
		}
	}

	return false
}

func (store *BadgerholdStore) SaveSubscriber(subscriber *types.Subscriber) error {
	// Store the subscriber data in the tree
	if err := store.Database.Upsert(subscriber.Npub, subscriber); err != nil {
		return fmt.Errorf("failed to put subscriber in Graviton store: %v", err)
	}

	return nil
}

func (store *BadgerholdStore) GetSubscriberByAddress(address string) (*types.Subscriber, error) {
	var results []types.Subscriber

	err := store.Database.Find(&results, badgerhold.Where("Address").Eq(address).Index("Address"))
	if err != nil {
		return nil, err
	}

	if len(results) > 0 {
		return &results[0], nil
	}

	return nil, fmt.Errorf("subscriber not found for address: %s", address)
}

func (store *BadgerholdStore) GetSubscriber(npub string) (*types.Subscriber, error) {
	var results []types.Subscriber

	err := store.Database.Find(&results, badgerhold.Where("Npub").Eq(npub).Index("Npub"))
	if err != nil {
		return nil, err
	}

	if len(results) > 0 {
		return &results[0], nil
	}

	// If no subscriber was found with the matching npub, return an error
	return nil, fmt.Errorf("subscriber not found for npub: %s", npub)
}

// AllocateBitcoinAddress allocates an available Bitcoin address to a subscriber.
func (store *BadgerholdStore) AllocateBitcoinAddress(npub string) (*types.Address, error) {
	var results []types.Address

	err := store.Database.Find(&results, badgerhold.Where("Status").Eq(AddressStatusAvailable).Index("Status"))
	if err != nil {
		return nil, err
	}

	if len(results) > 0 {
		addr := results[0]

		now := time.Now()
		addr.AllocatedAt = &now
		addr.Status = AddressStatusAllocated
		addr.Npub = npub

		err = store.Database.Upsert(addr.IndexHornets, addr)
		if err != nil {
			return nil, err
		}

		return &addr, nil
	}

	return nil, fmt.Errorf("no available addresses")
}

func (store *BadgerholdStore) AllocateAddress() (*types.Address, error) {
	var results []types.Address

	err := store.Database.Find(&results, badgerhold.Where("Status").Eq(AddressStatusAvailable).Index("Status"))
	if err != nil {
		return nil, err
	}

	if len(results) > 0 {
		addr := results[0]

		now := time.Now()
		addr.AllocatedAt = &now
		addr.Status = AddressStatusAllocated

		err = store.Database.Upsert(addr.IndexHornets, addr)
		if err != nil {
			return nil, err
		}

		return &addr, nil
	}

	return nil, fmt.Errorf("no available addresses")
}

func (store *BadgerholdStore) SaveAddress(addr *types.Address) error {
	err := store.Database.Upsert(addr.IndexHornets, addr)
	if err != nil {
		return fmt.Errorf("failed to put address in Graviton store: %v", err)
	}

	return nil
}

func WrapEvent(event *nostr.Event) *types.NostrEvent {
	kind := strconv.Itoa(event.Kind)

	return &types.NostrEvent{
		ID:        event.ID,
		PubKey:    event.PubKey,
		CreatedAt: event.CreatedAt,
		Kind:      kind,
		Tags:      event.Tags,
		Content:   event.Content,
		Sig:       event.Sig,
	}
}

func UnwrapEvent(event *types.NostrEvent) *nostr.Event {
	kind, err := strconv.Atoi(event.Kind)
	if err != nil {
		logging.Infof("This just means it's failing but this never actually gets printed")
	}

	return &nostr.Event{
		ID:        event.ID,
		PubKey:    event.PubKey,
		CreatedAt: event.CreatedAt,
		Kind:      int(kind),
		Tags:      event.Tags,
		Content:   event.Content,
		Sig:       event.Sig,
	}
}
