package immudb

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/fxamacker/cbor/v2"
	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"

	"github.com/HORNET-Storage/hornet-storage/lib/database/immudb/documents"
	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/kvp"
	kvp_bbolt "github.com/HORNET-Storage/hornet-storage/lib/stores/kvp/bbolt"
	kvp_immudb "github.com/HORNET-Storage/hornet-storage/lib/stores/kvp/immudb"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics"
	statistics_gorm_immudb "github.com/HORNET-Storage/hornet-storage/lib/stores/statistics/gorm/immudb"
	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"

	types "github.com/HORNET-Storage/hornet-storage/lib"

	immudb "github.com/codenotary/immudb/pkg/client"
)

const (
	AddressStatusAvailable = "available"
	AddressStatusAllocated = "allocated"
	AddressStatusUsed      = "used"
)

type ImmudbStore struct {
	Ctx context.Context

	Client immudb.ImmuClient

	DatabasePath string
	Database     kvp.KeyValueStore

	TempDatabasePath string
	TempDatabase     kvp.KeyValueStore

	StatsDatabase statistics.StatisticsStore

	NostrEventDatabase documents.Client
}

func InitStore(basepath string, args ...interface{}) (*ImmudbStore, error) {
	store := &ImmudbStore{}

	store.Ctx = context.Background()

	opts := immudb.DefaultOptions().WithAddress("127.0.0.1").WithPort(3322)

	store.Client = immudb.NewClient().WithOptions(opts)

	err := store.Client.OpenSession(store.Ctx, []byte("immudb"), []byte("immudb"), "defaultdb")
	if err != nil {
		log.Fatal(err)
	}

	store.DatabasePath = basepath
	store.TempDatabasePath = filepath.Join(filepath.Dir(basepath), fmt.Sprintf("%s-%s", "temp", uuid.New()))

	store.Database, err = kvp_immudb.InitBuckets(store.Ctx, store.Client)
	if err != nil {
		log.Fatal(err)
	}

	store.TempDatabase, err = kvp_bbolt.InitBuckets(store.TempDatabasePath)
	if err != nil {
		log.Fatal(err)
	}

	store.StatsDatabase, err = statistics_gorm_immudb.InitStore(store.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gorm statistics database: %v", err)
	}

	store.NostrEventDatabase = *documents.NewClient(store.Client.GetSessionID())

	err = store.InitializeNostrCollections()
	if err != nil {
		return nil, err
	}

	return store, nil
}

func (store *ImmudbStore) Cleanup() error {
	store.Database.Cleanup()
	store.TempDatabase.Cleanup()

	err := os.RemoveAll(store.TempDatabasePath)
	if err != nil {
		return err
	}

	return nil
}

func (store *ImmudbStore) GetStatsStore() statistics.StatisticsStore {
	return store.StatsDatabase
}

func (store *ImmudbStore) QueryDag(filter map[string]string, temp bool) ([]string, error) {
	keys := []string{}

	return keys, nil
}

func (store *ImmudbStore) QueryDagAdvanced(filter types.QueryFilter, temp bool) ([]string, error) {
	results := []string{}

	return results, nil
}

// Store an individual scionic merkletree leaf
// If the root leaf is the leaf being stored, the root will be cached depending on the data in the root leaf
func (store *ImmudbStore) StoreLeaf(root string, leafData *types.DagLeafData, temp bool) error {
	// Don't allow a leaf to be submitted without content if it contains a content hash
	if leafData.Leaf.ContentHash != nil && leafData.Leaf.Content == nil {
		return fmt.Errorf("leaf has content hash but no content")
	}

	var err error

	// Store the content of the leaf in the content bucket if the leaf has any
	// Remove the data from the leaf so we aren't storing double the data for no reason
	// Content gets added back to the leaf on retrieval
	if leafData.Leaf.Content != nil {
		contentBucket := store.GetBucket("content", temp)

		err = contentBucket.Put(string(leafData.Leaf.ContentHash), leafData.Leaf.Content)
		if err != nil {
			return err
		}

		leafData.Leaf.Content = nil
	}

	var rootLeaf *merkle_dag.DagLeaf

	// Retrieve the root leaf if the leaf being stored is not the root leaf
	if leafData.Leaf.Hash == root {
		// If it is the root leafthen just assign it and skip a retrieval
		rootLeaf = &leafData.Leaf
	} else {
		_rootLeaf, err := store.RetrieveLeaf(root, root, false, temp)
		if err != nil {
			return err
		}

		rootLeaf = &_rootLeaf.Leaf
	}

	// Determine what bucket the leaf gets stored in based on the root leaf file type
	prefix := GetPrefix(rootLeaf)

	cborData, err := cbor.Marshal(leafData)
	if err != nil {
		return err
	}

	bucket := store.GetBucket(prefix, temp)

	err = bucket.Put(leafData.Leaf.Hash, cborData)
	if err != nil {
		return err
	}

	// We only perform certain actions on the root leaf such as caching etc as everything should stem from the root
	if rootLeaf.Hash == leafData.Leaf.Hash {
		// Store bucket against root hash in the index so the bucket can always be found from the root hash
		indexBucket := store.GetBucket("scionicindex", temp)

		err = indexBucket.Put(root, []byte(prefix))
		if err != nil {
			return err
		}
	}

	return nil
}

// Retrieve an individual scionic merkletree leaf from the tree's root hash and the leaf hash
func (store *ImmudbStore) RetrieveLeaf(root string, hash string, includeContent bool, temp bool) (*types.DagLeafData, error) {
	prefix, err := store.GetBucketPrefix(root, temp)
	if err != nil {
		return nil, err
	}

	bucket := store.GetBucket(prefix, temp)

	bytes, err := bucket.Get(hash)
	if err != nil {
		return nil, err
	}

	var data *types.DagLeafData = &types.DagLeafData{}

	err = cbor.Unmarshal(bytes, data)
	if err != nil {
		return nil, err
	}

	if includeContent && data.Leaf.ContentHash != nil {
		content, err := store.RetrieveLeafContent(data.Leaf.ContentHash, temp)
		if err != nil {
			return nil, err
		}

		data.Leaf.Content = content
	}

	return data, nil
}

// Retrieve the content for a scionic merkletree leaf based on the hash of the content
// We can reduce the total data stored by ensuring all data is content addressed as sometimes
// leaves will have different data which changes the root hash but the actual content could be
// the same as other leaves already stored on this relay
func (store *ImmudbStore) RetrieveLeafContent(contentHash []byte, temp bool) ([]byte, error) {
	var err error

	contentBucket := store.GetBucket("content", temp)
	if err != nil {
		return nil, err
	}

	bytes, err := contentBucket.Get(string(contentHash))
	if err != nil {
		return nil, err
	}

	if len(bytes) > 0 {
		return bytes, nil
	} else {
		return nil, fmt.Errorf("content not found")
	}
}

// This is for finding which bucket a scionic merkletree leaf belongs to
// This is required due to the root leaf being the only leaf that can determine the bucket
func (store *ImmudbStore) GetBucketPrefix(root string, temp bool) (string, error) {
	var err error

	bucket := store.GetBucket("scionicindex", temp)
	if err != nil {
		return "", err
	}

	bytes, err := bucket.Get(root)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func (store *ImmudbStore) GetBucket(prefix string, temp bool) kvp.KeyValueStoreBucket {
	var bucket kvp.KeyValueStoreBucket
	if temp {
		bucket = store.TempDatabase.GetBucket(prefix)
	} else {
		bucket = store.Database.GetBucket(prefix)
	}

	return bucket
}

// Retrieve and build an entire scionic merkletree from the root hash
func (store *ImmudbStore) BuildDagFromStore(root string, includeContent bool, temp bool) (*types.DagData, error) {
	return stores.BuildDagFromStore(store, root, includeContent, temp)
}

// Store an entire scionic merkltree (not implemented currently as not required, leaves are stored as received)
func (store *ImmudbStore) StoreDag(dag *types.DagData, temp bool) error {
	return stores.StoreDag(store, dag, temp)
}

// Nostr events
func (store *ImmudbStore) InitializeNostrCollections() error {
	// Create the collection if it doesn't exist
	collection := &documents.Collection{
		Name: "nostr_events",
		// Fields needs to be defined as an array of field objects
		Fields: []*documents.Field{ // Changed from map to array
			{
				Name: "id",
				Type: string(documents.FieldTypeString),
			},
			{
				Name: "pubkey",
				Type: string(documents.FieldTypeString),
			},
			{
				Name: "created_at",
				Type: string(documents.FieldTypeInteger),
			},
			{
				Name: "kind",
				Type: string(documents.FieldTypeInteger),
			},
		},
		Indexes: []*documents.Index{
			{
				Fields:   []string{"id"},
				IsUnique: true,
			},
			{
				Fields:   []string{"pubkey"},
				IsUnique: false,
			},
			{
				Fields:   []string{"kind"},
				IsUnique: false,
			},
			{
				Fields:   []string{"created_at"},
				IsUnique: false,
			},
		},
	}

	err := store.NostrEventDatabase.CreateCollection(context.Background(), collection)
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("failed to create nostr_events collection: %w", err)
		}
	}

	return nil
}

// Query nostr events based on given filters
func (store *ImmudbStore) QueryEvents(filter nostr.Filter) ([]*nostr.Event, error) {
	// For each type of filter that used IN, we'll create multiple EQ expressions
	fields := []documents.FieldComparison{}

	// Handle IDs
	if len(filter.IDs) > 0 {
		for _, id := range filter.IDs {
			fields = append(fields, documents.FieldComparison{
				Field:    "id",
				Operator: "EQ",
				Value:    id,
			})
		}
	}

	// Handle Authors (pubkeys)
	if len(filter.Authors) > 0 {
		// For authors, we might want to use LIKE for prefix matching
		for _, author := range filter.Authors {
			fields = append(fields, documents.FieldComparison{
				Field:    "pubkey",
				Operator: "LIKE",
				Value:    fmt.Sprintf("^%s", author), // Prefix match
			})
		}
	}

	// Handle Kinds
	if len(filter.Kinds) > 0 {
		for _, kind := range filter.Kinds {
			fields = append(fields, documents.FieldComparison{
				Field:    "kind",
				Operator: "EQ",
				Value:    kind,
			})
		}
	}

	// Handle time range - these are already using supported operators
	if filter.Since != nil {
		fields = append(fields, documents.FieldComparison{
			Field:    "created_at",
			Operator: "GE",
			Value:    filter.Since,
		})
	}

	if filter.Until != nil {
		fields = append(fields, documents.FieldComparison{
			Field:    "created_at",
			Operator: "LE",
			Value:    filter.Until,
		})
	}

	// Handle tag filters
	// Handle tag filters
	for tagName, tagValues := range filter.Tags {
		if len(tagValues) > 0 {
			for _, tagValue := range tagValues {
				fields = append(fields, documents.FieldComparison{
					Field:    fmt.Sprintf("tags.%s", tagName),
					Operator: "CONTAINS",
					Value:    tagValue,
				})
			}
		}
	}

	page := 1
	perPage := 100
	if filter.Limit > 0 {
		perPage = filter.Limit
	}

	query := &documents.SearchQuery{
		Query: &documents.QueryExpression{
			Expressions: []documents.Expression{
				{
					FieldComparisons: fields,
				},
			},
		},
		Page:    page,
		PerPage: perPage,
	}

	if query.Page < 1 {
		query.Page = 1
	}
	if query.PerPage < 1 {
		query.PerPage = 100
	}
	if query.PerPage > 1000 {
		query.PerPage = 1000
	}

	result, err := store.NostrEventDatabase.SearchDocuments(context.Background(), "nostr_events", query)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))

	// Convert results back to nostr.Event objects
	events := make([]*nostr.Event, 0, len(result.Revisions))
	for _, rev := range result.Revisions {
		// Reconstruct the nostr.Event
		event := &nostr.Event{
			ID:        rev.Document["id"].(string),
			PubKey:    rev.Document["pubkey"].(string),
			CreatedAt: nostr.Timestamp(int64(rev.Document["created_at"].(float64))),
			Kind:      int(rev.Document["kind"].(float64)),
			Tags:      make(nostr.Tags, 0),
			Content:   rev.Document["content"].(string),
			Sig:       rev.Document["sig"].(string),
		}

		// Handle tags conversion from our stored format
		if tagMap, ok := rev.Document["tags"].(map[string]interface{}); ok {
			for tagName, values := range tagMap {
				if tagValues, ok := values.([]interface{}); ok {
					for _, value := range tagValues {
						if strValue, ok := value.(string); ok {
							tag := []string{tagName, strValue}
							event.Tags = append(event.Tags, tag)
						}
					}
				}
			}
		}

		events = append(events, event)
	}

	return events, nil
}

func (store *ImmudbStore) StoreEvent(event *nostr.Event) error {
	formattedTags := make(map[string][]string)
	for _, tag := range event.Tags {
		if len(tag) >= 2 {
			tagName := tag[0]
			tagValue := tag[1]
			if existing, ok := formattedTags[tagName]; ok {
				formattedTags[tagName] = append(existing, tagValue)
			} else {
				formattedTags[tagName] = []string{tagValue}
			}
		}
	}

	doc := documents.Document{
		"id":         event.ID,        // Store ID for lookups
		"pubkey":     event.PubKey,    // Store pubkey for filtering
		"created_at": event.CreatedAt, // Store as unix timestamp for range queries
		"kind":       event.Kind,      // Store kind for filtering
		"tags":       formattedTags,   // Store converted tags
		"content":    event.Content,   // Store the content
		"sig":        event.Sig,       // Store the signature
	}

	_, err := store.NostrEventDatabase.InsertDocuments(store.Ctx, "nostr_events", []documents.Document{doc})
	if err != nil {
		return fmt.Errorf("failed to store nostr event: %w", err)
	}

	return nil
}

func (store *ImmudbStore) DeleteEvent(eventID string) error {
	query := &documents.QueryExpression{
		Expressions: []documents.Expression{{
			FieldComparisons: []documents.FieldComparison{{
				Field:    "id",
				Operator: "EQ",
				Value:    eventID,
			}},
		}},
	}

	_, err := store.NostrEventDatabase.DeleteDocuments(store.Ctx, "nostr_events", query)
	if err != nil {
		return fmt.Errorf("failed to find event to delete: %w", err)
	}

	return nil
}

// Blossom Blobs (unchunked data)
func (store *ImmudbStore) StoreBlob(data []byte, hash []byte, publicKey string) error {
	contentBucket := store.GetBucket("content", false)

	encodedHash := hex.EncodeToString(hash)

	//mtype := mimetype.Detect(data)

	err := contentBucket.Put(encodedHash, data)
	if err != nil {
		return err
	}

	return nil
}

func (store *ImmudbStore) GetBlob(hash string) ([]byte, error) {
	contentBucket := store.GetBucket("content", false)

	content, err := contentBucket.Get(hash)
	if err != nil {
		return nil, err
	}

	return content, nil
}

func (store *ImmudbStore) DeleteBlob(hash string) error {
	contentBucket := store.GetBucket("content", false)

	err := contentBucket.Delete([]string{hash})
	if err != nil {
		return err
	}

	return nil
}

func (store *ImmudbStore) QueryBlobs(mimeType string) ([]string, error) {

	return nil, nil
}

// This determines what bucket a scionic merkletree should be stored in based on its file type
// The root hashes may also be cached in several other cache buckets depending on the AdditionalData fields
func GetPrefix(leaf *merkle_dag.DagLeaf) string {
	split := strings.Split(leaf.ItemName, ".")

	if len(split) > 1 {
		return split[1]
	} else {
		if leaf.Type == merkle_dag.DirectoryLeafType {
			return "directory"
		} else {
			return "file"
		}
	}
}

func contains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
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

func (store *ImmudbStore) SaveSubscriber(subscriber *types.Subscriber) error {
	subscriberBucket := store.GetBucket("subscribers", false)

	// Marshal the subscriber into JSON
	subscriberData, err := json.Marshal(subscriber)
	if err != nil {
		return fmt.Errorf("failed to marshal subscriber: %v", err)
	}

	// Use the npub as the key for storing the subscriber
	key := subscriber.Npub

	// Store the subscriber data in the tree
	if err := subscriberBucket.Put(key, subscriberData); err != nil {
		return fmt.Errorf("failed to put subscriber in Graviton store: %v", err)
	}

	return nil
}

func (store *ImmudbStore) GetSubscriberByAddress(address string) (*types.Subscriber, error) {
	subscriberBucket := store.GetBucket("subscribers", false)

	iterator, err := subscriberBucket.Scan()
	if err != nil {
		return nil, fmt.Errorf("subscriber not found for address: %s", address)
	}

	for {
		value := iterator.Value()

		var subscriber types.Subscriber
		if err := json.Unmarshal(value, &subscriber); err != nil {
			return nil, fmt.Errorf("failed to unmarshal subscriber data: %v", err)
		}

		if subscriber.Address == address {
			return &subscriber, nil
		}

		if !iterator.Next() {
			break
		}
	}

	return nil, fmt.Errorf("subscriber not found for address: %s", address)
}

func (store *ImmudbStore) GetSubscriber(npub string) (*types.Subscriber, error) {
	subscriberBucket := store.GetBucket("subscribers", false)

	iterator, err := subscriberBucket.Scan()
	if err != nil {
		return nil, fmt.Errorf("subscriber not found for npub: %s", npub)
	}

	for {
		value := iterator.Value()

		var subscriber types.Subscriber
		if err := json.Unmarshal(value, &subscriber); err != nil {
			return nil, fmt.Errorf("failed to unmarshal subscriber data: %v", err)
		}

		// Check if the current subscriber's npub matches the provided npub
		if subscriber.Npub == npub {
			return &subscriber, nil
		}

		if !iterator.Next() {
			break
		}
	}

	// If no subscriber was found with the matching npub, return an error
	return nil, fmt.Errorf("subscriber not found for npub: %s", npub)
}

// AllocateBitcoinAddress allocates an available Bitcoin address to a subscriber.
func (store *ImmudbStore) AllocateBitcoinAddress(npub string) (*types.Address, error) {
	addressBucket := store.GetBucket("relay_addresses", false)

	iterator, err := addressBucket.Scan()
	if err != nil {
		return nil, fmt.Errorf("no available addresses")
	}

	for {
		value := iterator.Value()

		var addr types.Address
		if err := json.Unmarshal(value, &addr); err != nil {
			log.Printf("Error unmarshaling address: %v. Skipping this address.", err)
			continue
		}
		if addr.Status == AddressStatusAvailable {
			// Allocate the address to the subscriber
			now := time.Now()
			addr.Status = AddressStatusAllocated
			addr.AllocatedAt = &now
			addr.Npub = npub

			// Marshal the updated address and store it back in the database
			value, err := json.Marshal(addr)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal address: %v", err)
			}
			if err := addressBucket.Put(addr.IndexHornets, value); err != nil {
				return nil, fmt.Errorf("failed to put address in tree: %v", err)
			}

			return &addr, nil
		}

		if !iterator.Next() {
			break
		}
	}

	return nil, fmt.Errorf("no available addresses")
}

func (store *ImmudbStore) AllocateAddress() (*types.Address, error) {
	addressBucket := store.GetBucket("relay_addresses", false)

	iterator, err := addressBucket.Scan()
	if err != nil {
		return nil, fmt.Errorf("no available addresses")
	}

	for {
		value := iterator.Value()

		var addr types.Address
		if err := json.Unmarshal(value, &addr); err != nil {
			return nil, err
		}
		if addr.Status == AddressStatusAvailable {
			now := time.Now()
			addr.Status = AddressStatusAllocated
			addr.AllocatedAt = &now

			value, err := json.Marshal(addr)
			if err != nil {
				return nil, err
			}

			if err := addressBucket.Put(addr.IndexHornets, value); err != nil {
				return nil, err
			}

			return &addr, nil
		}

		if !iterator.Next() {
			break
		}
	}

	return nil, fmt.Errorf("no available addresses")
}

func (store *ImmudbStore) SaveAddress(addr *types.Address) error {
	addressBucket := store.GetBucket("relay_addresses", false)

	// Marshal the address into JSON
	addressData, err := json.Marshal(addr)
	if err != nil {
		return fmt.Errorf("failed to marshal address: %v", err)
	}

	// Use the index as the key for storing the address
	key := addr.IndexHornets

	// Store the address data in the tree
	if err := addressBucket.Put(key, addressData); err != nil {
		return fmt.Errorf("failed to put address in Graviton store: %v", err)
	}

	return nil
}
