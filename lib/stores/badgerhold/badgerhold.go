package badgerhold

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/fxamacker/cbor/v2"
	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/dag"
	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics"
	statistics_gorm_sqlite "github.com/HORNET-Storage/hornet-storage/lib/stores/statistics/gorm/sqlite"

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

func cborEncode(value interface{}) ([]byte, error) {
	return cbor.Marshal(value)
}

func cborDecode(data []byte, value interface{}) error {
	return cbor.Unmarshal(data, value)
}

func InitStore(basepath string, args ...interface{}) (*BadgerholdStore, error) {
	store := &BadgerholdStore{}

	var err error

	store.Ctx = context.Background()

	store.DatabasePath = basepath
	store.TempDatabasePath = filepath.Join(filepath.Dir(basepath), fmt.Sprintf("%s-%s", "temp", uuid.New()))

	options := badgerhold.DefaultOptions
	options.Encoder = cborEncode
	options.Decoder = cborDecode
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

	err := store.GetDatabase(temp).Get(hex.EncodeToString(contentHash), &content)

	return content.Content, err
}

func (store *BadgerholdStore) QueryDag(filter types.QueryFilter, temp bool) ([]string, error) {
	var results []types.WrappedLeaf

	fmt.Println("Searching for dags with filter: ")
	bytes, _ := json.Marshal(filter)
	fmt.Println(string(bytes))

	// Start query with a dummy condition
	query := badgerhold.Where("Hash").Ne("") // Ensures chaining works
	first := true

	// Add filtering by PublicKey
	if len(filter.PubKeys) > 0 {
		pubKeysAsInterface := make([]interface{}, len(filter.PubKeys))
		for i, pubKey := range filter.PubKeys {
			pubKeysAsInterface[i] = pubKey
		}

		if first {
			query = badgerhold.Where("PublicKey").In(pubKeysAsInterface...)
			first = false
		} else {
			query = query.And("PublicKey").In(pubKeysAsInterface...)
		}
	}

	// Add filtering by ItemName
	if len(filter.Names) > 0 {
		namesAsInterface := make([]interface{}, len(filter.Names))
		for i, name := range filter.Names {
			namesAsInterface[i] = name
		}

		if first {
			query = badgerhold.Where("ItemName").In(namesAsInterface...)
			first = false
		} else {
			query = query.And("ItemName").In(namesAsInterface...)
		}
	}

	// Execute the primary query
	err := store.GetDatabase(temp).Find(&results, query)
	if err != nil && err != badgerhold.ErrNotFound {
		return nil, fmt.Errorf("failed to query WrappedLeaf: %w", err)
	}

	// Extract hashes from primary results
	hashSet := make(map[string]struct{})
	for _, leaf := range results {
		hashSet[leaf.Hash] = struct{}{}
	}

	var entries []types.AdditionalDataEntry
	err = store.GetDatabase(temp).Find(&entries, badgerhold.Where("Key").Ne(""))
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		fmt.Println("Entry: " + entry.Key + " | " + entry.Value)
	}

	// If we have tag filters, run a secondary query to filter based on tags
	if len(filter.Tags) > 0 {
		for tagKey, tagValue := range filter.Tags {
			var tagEntries []types.AdditionalDataEntry

			fmt.Printf("searching for tags: ")

			err := store.GetDatabase(temp).Find(&tagEntries, badgerhold.Where("Key").Eq(tagKey).And("Value").Eq(tagValue))
			if err != nil && err != badgerhold.ErrNotFound {
				return nil, fmt.Errorf("failed to query AdditionalDataEntry for key=%s, value=%s: %w", tagKey, tagValue, err)
			}

			// Keep only hashes that match the tag query
			tempHashSet := make(map[string]struct{})
			for _, entry := range tagEntries {
				if _, exists := hashSet[entry.Hash]; exists { // Keep only those already in our result set
					tempHashSet[entry.Hash] = struct{}{}
				}
			}
			hashSet = tempHashSet // Update result set to only include tag-matching hashes
		}
	}

	// Convert hashSet to a slice of strings
	hashes := make([]string, 0, len(hashSet))
	for hash := range hashSet {
		hashes = append(hashes, hash)
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
		err = store.StoreContent(hex.EncodeToString(leafData.Leaf.ContentHash), leafData.Leaf.Content, temp)
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

	fmt.Println("Storing Leaf")
	if len(leafData.Leaf.AdditionalData) > 0 {
		fmt.Println("WITH DATA")
	}

	for key, value := range leafData.Leaf.AdditionalData {
		fmt.Println(key + " | " + value)
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

// Retrieve and build an entire scionic merkletree from the root hash
func (store *BadgerholdStore) BuildDagFromStore(root string, includeContent bool, temp bool) (*types.DagData, error) {
	return stores.BuildDagFromStore(store, root, includeContent, temp)
}

// Store an entire scionic merkltree (not implemented currently as not required, leaves are stored as received)
func (store *BadgerholdStore) StoreDag(dag *types.DagData, temp bool) error {
	return stores.StoreDag(store, dag, temp)
}

func (store *BadgerholdStore) QueryEvents(filter nostr.Filter) ([]*nostr.Event, error) {
	var results []types.NostrEvent

	fmt.Println("\n\nSearching for events with filter")
	jd, _ := json.Marshal(filter)
	fmt.Println(string(jd))

	query := badgerhold.Where("ID").Ne("")
	first := true

	if len(filter.Kinds) > 0 {
		kindsAsInterface := make([]interface{}, len(filter.Kinds))
		for i, kind := range filter.Kinds {
			kindsAsInterface[i] = strconv.Itoa(kind)
			fmt.Println("Searching for kinds: " + strconv.Itoa(kind))
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
			fmt.Println("Searching for authors: " + author)
		}

		if first {
			query = badgerhold.Where("PubKey").In(authorsAsInterface...)
			first = false
		} else {
			query = query.And("PubKey").In(authorsAsInterface...)
		}
	}

	if filter.Since != nil {
		query = query.And("CreatedAt").Ge(filter.Since.Time())
	}
	if filter.Until != nil {
		query = query.And("CreatedAt").Le(filter.Until.Time())
	}

	if len(filter.Tags) > 0 {
		eventIDSet := make(map[string]struct{})

		isFirst := true

		for tagName, tagValues := range filter.Tags {
			var tagEntries []types.TagEntry

			//tagValues = append(tagValues, "c25aedfd38f9fed72b383f6eefaea9f21dd58ec2c9989e0cc275cb5296adec17:nestr")

			fmt.Printf("Searching for tag with values: " + tagName + "\n")
			for _, v := range tagValues {
				fmt.Println(v)
			}
			fmt.Println("")

			err := store.Database.Find(&tagEntries, badgerhold.Where("TagName").Eq(strings.ReplaceAll(tagName, "#", "")).And("TagValue").In(toInterfaceSlice(tagValues)...))
			if err != nil && err != badgerhold.ErrNotFound {
				return nil, fmt.Errorf("failed to query tag entries for %s: %w", tagName, err)
			}

			fmt.Printf("Found %d tag entries from tags\n", len(tagEntries))

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
			fmt.Println("No matching events from tags")
			return []*nostr.Event{}, nil
		}

		fmt.Printf("Found %d events from tags\n", len(eventIDs))

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

	fmt.Println("First check")
	for _, ev := range events {
		fmt.Printf("Found event of kind: %d\n", ev.Kind)
	}

	// Step 8: Apply additional filters (search term, etc.)
	filteredEvents := postFilterEvents(events, filter)

	// Step 9: Sort events (newest first)
	sortEventsByCreatedAt(filteredEvents)

	// Step 10: Apply limit if necessary
	if filter.Limit > 0 && len(filteredEvents) > filter.Limit {
		filteredEvents = filteredEvents[:filter.Limit]
	}

	fmt.Println("Last check")
	for _, ev := range events {
		fmt.Printf("Found event of kind: %d\n", ev.Kind)
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
		if filter.Search != "" && !strings.Contains(strings.ToLower(event.Content), strings.ToLower(filter.Search)) {
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
	if len(leaf.Leaf.ClassicMerkleRoot) <= 0 {
		leaf.Leaf.ClassicMerkleRoot = nil
	}

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
	if len(leaf.ClassicMerkleRoot) <= 0 {
		leaf.ClassicMerkleRoot = nil
	}

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
		fmt.Println("This just means it's failing but this never actually gets printed")
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
