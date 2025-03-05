package badgerhold

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"

	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics"
	statistics_gorm_sqlite "github.com/HORNET-Storage/hornet-storage/lib/stores/statistics/gorm/sqlite"
	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"

	types "github.com/HORNET-Storage/hornet-storage/lib"

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

	TempDatabasePath string
	TempDatabase     *badgerhold.Store

	StatsDatabase statistics.StatisticsStore
}

func InitStore(basepath string, args ...interface{}) (*BadgerholdStore, error) {
	store := &BadgerholdStore{}

	var err error

	store.Ctx = context.Background()

	store.DatabasePath = basepath
	store.TempDatabasePath = filepath.Join(filepath.Dir(basepath), fmt.Sprintf("%s-%s", "temp", uuid.New()))

	options := badgerhold.DefaultOptions
	options.Dir = "data"
	options.ValueDir = "data"

	store.Database, err = badgerhold.Open(options)
	if err != nil {
		log.Fatal(err)
	}

	options.Dir = "temp"
	options.ValueDir = "temp"

	store.TempDatabase, err = badgerhold.Open(options)
	if err != nil {
		log.Fatal(err)
	}

	store.StatsDatabase, err = statistics_gorm_sqlite.InitStore()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gorm statistics database: %v", err)
	}

	return store, nil
}

func (store *BadgerholdStore) Cleanup() error {
	store.Database.Close()
	store.TempDatabase.Close()

	//err := os.RemoveAll(store.TempDatabasePath)
	//if err != nil {
	//return err
	//}

	return nil
}

func (store *BadgerholdStore) GetStatsStore() statistics.StatisticsStore {
	return store.StatsDatabase
}

func (store *BadgerholdStore) GetDatabase(temp bool) *badgerhold.Store {
	if temp {
		return store.TempDatabase
	} else {
		return store.Database
	}
}

func (store *BadgerholdStore) StoreContent(hash string, content []byte, temp bool) error {
	return store.GetDatabase(temp).Upsert(hash, &types.DagContent{
		Hash:    hash,
		Content: content,
	})
}

func (store *BadgerholdStore) RetrieveLeafContent(contentHash []byte, temp bool) ([]byte, error) {
	var content types.DagContent

	err := store.GetDatabase(temp).Get(string(contentHash), &content)

	return content.Content, err
}

func (store *BadgerholdStore) QueryDag(filter types.QueryFilter, temp bool) ([]string, error) {
	var results []types.WrappedLeaf

	for _, queryKey := range filter.PubKeys {
		err := store.GetDatabase(temp).Find(&results, badgerhold.Where("PublicKey").Eq(queryKey).Index("PublicKey"))
		if err != nil {
			fmt.Println("Failed to query for pub key in query dag")
			continue
		}
	}

	for _, queryKey := range filter.Names {
		err := store.GetDatabase(temp).Find(&results, badgerhold.Where("ItemName").Eq(queryKey).Index("ItemName"))
		if err != nil {
			fmt.Println("Failed to query for pub key in query dag")
			continue
		}
	}

	hashes := []string{}

	for _, leaf := range results {
		hashes = append(hashes, leaf.Hash)
	}

	for queryKey, queryValue := range filter.Tags {
		var entries []types.AdditionalDataEntry

		err := store.GetDatabase(temp).Find(&entries, badgerhold.Where("Key").Eq(queryKey).And("Value").Eq(queryValue).Index("Key"))
		if err != nil {
			fmt.Println("Failed to query for key value pair entries in query dag")
			continue
		}

		for _, entry := range entries {
			hashes = append(hashes, entry.Hash)
		}
	}

	return hashes, nil
}

func (store *BadgerholdStore) StoreLeaf(root string, leafData *types.DagLeafData, temp bool) error {
	// Don't allow a leaf to be submitted without content if it contains a content hash
	if leafData.Leaf.ContentHash != nil && leafData.Leaf.Content == nil {
		return fmt.Errorf("leaf has content hash but no content")
	}

	var err error

	if leafData.Leaf.Content != nil {
		err = store.StoreContent(string(leafData.Leaf.ContentHash), leafData.Leaf.Content, temp)
		if err != nil {
			return err
		}

		leafData.Leaf.Content = nil
	}

	wrappedLeaf := WrapLeaf(leafData)

	err = store.GetDatabase(temp).Upsert(leafData.Leaf.Hash, wrappedLeaf)
	if err != nil {
		return err
	}

	for key, value := range leafData.Leaf.AdditionalData {
		entry := types.AdditionalDataEntry{
			Hash:  leafData.Leaf.Hash,
			Key:   key,
			Value: value,
		}

		store.GetDatabase(temp).Upsert(fmt.Sprintf("%s:%s", leafData.Leaf.Hash, key), entry)
	}

	return nil
}

// Retrieve an individual scionic merkletree leaf from the tree's root hash and the leaf hash
func (store *BadgerholdStore) RetrieveLeaf(root string, hash string, includeContent bool, temp bool) (*types.DagLeafData, error) {
	var wrappedLeaf types.WrappedLeaf

	err := store.GetDatabase(temp).Get(hash, &wrappedLeaf)
	if err != nil {
		return nil, err
	}

	data := UnwrapLeaf(&wrappedLeaf)

	if includeContent && data.Leaf.ContentHash != nil {
		content, err := store.RetrieveLeafContent(data.Leaf.ContentHash, temp)
		if err != nil {
			return nil, err
		}

		data.Leaf.Content = content
	}

	return data, nil
}

func (store *BadgerholdStore) QueryEventsByTag(tagName, tagValue string) ([]string, error) {
	var entries []types.TagEntry

	// Query for tag entries matching the tag name and value
	err := store.Database.Find(&entries, badgerhold.Where("TagName").Eq(tagName).And("TagValue").Eq(tagValue).Index("TagName"))
	if err != nil {
		return nil, fmt.Errorf("failed to query tag entries: %w", err)
	}

	// Extract the event IDs
	eventIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		eventIDs = append(eventIDs, entry.EventID)
	}

	return eventIDs, nil
}

// QueryEventsByMultipleTags finds events matching multiple tag conditions
func (store *BadgerholdStore) QueryEventsByMultipleTags(tagConditions map[string][]string) ([]string, error) {
	// Map to collect unique event IDs
	eventIDMap := make(map[string]struct{})

	// For each tag name and its possible values
	for tagName, tagValues := range tagConditions {
		// Skip empty tag values
		if len(tagValues) == 0 {
			continue
		}

		for _, tagValue := range tagValues {
			// Query for this specific tag name and value
			var entries []types.TagEntry
			err := store.Database.Find(&entries, badgerhold.Where("TagName").Eq(tagName).And("TagValue").Eq(tagValue).Index("TagName"))
			if err != nil && err != badgerhold.ErrNotFound {
				return nil, fmt.Errorf("failed to query tag entries for %s=%s: %w", tagName, tagValue, err)
			}

			// Add event IDs to our map
			for _, entry := range entries {
				eventIDMap[entry.EventID] = struct{}{}
			}
		}
	}

	// Convert map to slice of event IDs
	eventIDs := make([]string, 0, len(eventIDMap))
	for id := range eventIDMap {
		eventIDs = append(eventIDs, id)
	}

	return eventIDs, nil
}

// Retrieve and build an entire scionic merkletree from the root hash
func (store *BadgerholdStore) BuildDagFromStore(root string, includeContent bool, temp bool) (*types.DagData, error) {
	return stores.BuildDagFromStore(store, root, includeContent, temp)
}

// Store an entire scionic merkltree (not implemented currently as not required, leaves are stored as received)
func (store *BadgerholdStore) StoreDag(dag *types.DagData, temp bool) error {
	return stores.StoreDag(store, dag, temp)
}

func (store *BadgerholdStore) QueryEvents(filter nostr.Filter) ([]*nostr.Event, error) {
	var allEvents []*nostr.Event
	eventMap := make(map[string]*types.NostrEvent) // For deduplication

	// Query by IDs
	for _, id := range filter.IDs {
		var results []types.NostrEvent
		err := store.Database.Find(&results, badgerhold.Where("ID").Eq(id).Index("ID"))
		if err != nil && err != badgerhold.ErrNotFound {
			fmt.Printf("Query error for ID %s: %v\n", id, err)
			continue
		}
		for _, event := range results {
			eventMap[event.ID] = &event
		}
	}

	// Query by authors
	for _, author := range filter.Authors {
		var results []types.NostrEvent
		err := store.Database.Find(&results, badgerhold.Where("PubKey").Eq(author).Index("PubKey"))
		if err != nil && err != badgerhold.ErrNotFound {
			fmt.Printf("Query error for author %s: %v\n", author, err)
			continue
		}
		for _, event := range results {
			eventMap[event.ID] = &event
		}
	}

	// Query by kinds
	for _, kind := range filter.Kinds {
		var results []types.NostrEvent
		err := store.Database.Find(&results, badgerhold.Where("Kind").Eq(kind).Index("Kind"))
		if err != nil && err != badgerhold.ErrNotFound {
			fmt.Printf("Query error for kind %d: %v\n", kind, err)
			continue
		}
		for _, event := range results {
			eventMap[event.ID] = &event
		}
	}

	// Convert the map to a slice
	for _, event := range eventMap {
		allEvents = append(allEvents, UnwrapEvent(event))
	}

	// Post-filtering for time range, search, and complex criteria
	var filteredEvents []*nostr.Event
	for _, event := range allEvents {
		// Check if event passes all filter conditions
		if !eventMatchesFilter(event, filter) {
			continue
		}
		filteredEvents = append(filteredEvents, event)
	}

	// Sort by created_at (newest first)
	sortEventsByCreatedAt(filteredEvents)

	// Apply limit
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

// eventMatchesFilter checks if an event matches all criteria in the filter
func eventMatchesFilter(event *nostr.Event, filter nostr.Filter) bool {
	/*
		// Check IDs if specified
		if len(filter.IDs) > 0 {
			matched := false
			for _, id := range filter.IDs {
				if event.ID == id {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}

		// Check authors if specified
		if len(filter.Authors) > 0 {
			matched := false
			for _, author := range filter.Authors {
				if event.PubKey == author {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}

		// Check kinds if specified
		if len(filter.Kinds) > 0 {
			matched := false
			for _, kind := range filter.Kinds {
				if event.Kind == kind {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}

		// Check time range
		if filter.Since != nil {
			sinceTime := time.Unix(int64(*filter.Since), 0)
			if event.CreatedAt.Time().Before(sinceTime) {
				return false
			}
		}

		if filter.Until != nil {
			untilTime := time.Unix(int64(*filter.Until), 0)
			if event.CreatedAt.Time().After(untilTime) {
				return false
			}
		}

		// Check search term
		if filter.Search != "" {
			if !strings.Contains(
				strings.ToLower(event.Content),
				strings.ToLower(filter.Search),
			) {
				return false
			}
		}
	*/

	return true
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

	return nil
}

func (store *BadgerholdStore) DeleteEvent(eventID string) error {
	err := store.Database.Delete(eventID, types.NostrEvent{})
	if err != nil {
		return fmt.Errorf("failed to find event to delete: %w", err)
	}

	return nil
}

// Blossom Blobs (unchunked data)
func (store *BadgerholdStore) StoreBlob(data []byte, hash []byte, publicKey string) error {
	encodedHash := hex.EncodeToString(hash)

	//mtype := mimetype.Detect(data)

	content := types.BlobContent{
		Hash:    encodedHash,
		PubKey:  publicKey,
		Content: data,
	}

	err := store.Database.Upsert(encodedHash, content)
	if err != nil {
		return err
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
		if patternParts[patternIndex] == "*" {
			patternIndex++
			if patternIndex == len(patternParts) {
				return true // "*" at the end matches everything remaining
			}
			// Find the next matching part
			for valueIndex < len(valueParts) && valueParts[valueIndex] != patternParts[patternIndex] {
				valueIndex++
			}
		} else if patternParts[patternIndex] == valueParts[valueIndex] {
			patternIndex++
			valueIndex++
		} else {
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

func WrapLeaf(leaf *types.DagLeafData) *types.WrappedLeaf {
	return &types.WrappedLeaf{
		PublicKey:         leaf.PublicKey,
		Signature:         leaf.Signature,
		Hash:              leaf.Leaf.Hash,
		ItemName:          leaf.Leaf.ItemName,
		Type:              leaf.Leaf.Type,
		ContentHash:       leaf.Leaf.ContentHash,
		ClassicMerkleRoot: leaf.Leaf.ClassicMerkleRoot,
		CurrentLinkCount:  leaf.Leaf.CurrentLinkCount,
		LatestLabel:       leaf.Leaf.LatestLabel,
		LeafCount:         leaf.Leaf.LeafCount,
		Links:             leaf.Leaf.Links,
		ParentHash:        leaf.Leaf.ParentHash,
		AdditionalData:    leaf.Leaf.AdditionalData,
	}
}

func UnwrapLeaf(leaf *types.WrappedLeaf) *types.DagLeafData {
	return &types.DagLeafData{
		PublicKey: leaf.PublicKey,
		Signature: leaf.Signature,
		Leaf: merkle_dag.DagLeaf{
			Hash:              leaf.Hash,
			ItemName:          leaf.ItemName,
			Type:              leaf.Type,
			ContentHash:       leaf.ContentHash,
			ClassicMerkleRoot: leaf.ClassicMerkleRoot,
			CurrentLinkCount:  leaf.CurrentLinkCount,
			LatestLabel:       leaf.LatestLabel,
			LeafCount:         leaf.LeafCount,
			Links:             leaf.Links,
			ParentHash:        leaf.ParentHash,
			AdditionalData:    leaf.AdditionalData,
		},
	}
}

func WrapEvent(event *nostr.Event) *types.NostrEvent {
	return &types.NostrEvent{
		ID:        event.ID,
		PubKey:    event.PubKey,
		CreatedAt: event.CreatedAt,
		Kind:      event.Kind,
		Tags:      event.Tags,
		Content:   event.Content,
		Sig:       event.Sig,
	}
}

func UnwrapEvent(event *types.NostrEvent) *nostr.Event {
	return &nostr.Event{
		ID:        event.ID,
		PubKey:    event.PubKey,
		CreatedAt: event.CreatedAt,
		Kind:      event.Kind,
		Tags:      event.Tags,
		Content:   event.Content,
		Sig:       event.Sig,
	}
}
