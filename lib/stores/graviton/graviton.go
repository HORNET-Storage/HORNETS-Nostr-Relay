package graviton

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/deroproject/graviton"
	"github.com/fxamacker/cbor/v2"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"

	jsoniter "github.com/json-iterator/go"

	types "github.com/HORNET-Storage/hornet-storage/lib"
)

type GravitonStore struct {
	Database *graviton.Store
}

func (store *GravitonStore) InitStore(args ...interface{}) error {
	db, err := graviton.NewDiskStore("gravitondb")
	if err != nil {
		return err
	}

	store.Database = db

	snapshot, err := db.LoadSnapshot(0)
	if err != nil {
		return err
	}

	tree, err := snapshot.GetTree("content")
	if err != nil {
		return err
	}

	_, err = graviton.Commit(tree)
	if err != nil {
		return err
	}

	return nil
}

// Scionic Merkletree's (Chunked data)
// Query scionic merkletree's by providing a key and a value, key being the bucket and value being the key of how the tree is cached
// An example would be "npub:app": "filetype" because the trees are cached in buckets per user per app and filetypes
// You can only query trees that have been cached through supported caching methods as itterating all of the trees
// Would create a significant performance impact if the data set gets too big
func (store *GravitonStore) QueryDag(filter map[string]string) ([]string, error) {
	snapshot, err := store.Database.LoadSnapshot(0)
	if err != nil {
		return nil, err
	}

	keys := []string{}

	for bucket, key := range filter {
		cacheBucket := fmt.Sprintf("cache:%s", bucket)
		cacheTree, err := snapshot.GetTree(cacheBucket)
		if err == nil {
			value, err := cacheTree.Get([]byte(key))
			if err == nil {
				var cacheData *types.CacheData = &types.CacheData{}
				err = cbor.Unmarshal(value, cacheData)
				if err == nil {
					keys = append(keys, cacheData.Keys...)
				}
			}
		}
	}

	return keys, nil
}

// Store an individual scionic merkletree leaf
// If the root leaf is the leaf being stored, the root will be cached depending on the data in the root leaf
func (store *GravitonStore) StoreLeaf(root string, leafData *types.DagLeafData) error {
	// Don't allow a leaf to be submitted without content if it contains a content hash
	if leafData.Leaf.ContentHash != nil && leafData.Leaf.Content == nil {
		return fmt.Errorf("leaf has content hash but no content")
	}

	snapshot, err := store.Database.LoadSnapshot(0)
	if err != nil {
		return err
	}

	var contentTree *graviton.Tree = nil

	// TODO: Block leaves that have content over the configured chunk limit
	leafContentSize := len(hex.EncodeToString(leafData.Leaf.Content))

	// Store the content of the leaf in the content bucket if the leaf has any
	// Remove the data from the leaf so we aren't storing double the data for no reason
	// Content gets added back to the leaf on retrieval
	if leafData.Leaf.Content != nil {
		contentTree, err = snapshot.GetTree("content")
		if err != nil {
			return err
		}

		err = contentTree.Put(leafData.Leaf.ContentHash, leafData.Leaf.Content)
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
		_rootLeaf, err := store.RetrieveLeaf(root, root, false)
		if err != nil {
			return err
		}

		rootLeaf = &_rootLeaf.Leaf
	}

	// Determine what bucket the leaf gets stored in based on the root leaf file type
	bucket := GetBucket(rootLeaf)

	cborData, err := cbor.Marshal(leafData)
	if err != nil {
		return err
	}

	key := leafData.Leaf.Hash

	tree, err := snapshot.GetTree(bucket)
	if err != nil {
		return err
	}

	err = tree.Put([]byte(key), cborData)
	if err != nil {
		return err
	}

	trees := []*graviton.Tree{}

	trees = append(trees, tree)

	// We only perform certain actions on the root leaf such as caching etc as everything should stem from the root
	if rootLeaf.Hash == leafData.Leaf.Hash {
		// Store bucket against root hash in the index so the bucket can always be found from the root hash
		indexTree, err := snapshot.GetTree("scionic_index")
		if err != nil {
			return err
		}

		indexTree.Put([]byte(root), []byte(bucket))
		trees = append(trees, indexTree)

		// Cache the root against the user and the file type
		if leafData.PublicKey != "" {
			_trees, err := store.cacheKey(leafData.PublicKey, bucket, root)
			if err == nil {
				trees = append(trees, _trees...)
			}
		}

		// Cache the root against the provided user and application if found
		folder, ok := rootLeaf.AdditionalData["f"]
		if ok {
			appName := GetAppNameFromPath(folder)
			if appName != "" {
				// TODO: Check if app is supported by this relay
				_trees, err := store.cacheKey(fmt.Sprintf("%s:%s", leafData.PublicKey, appName), folder, rootLeaf.Hash)
				if err == nil {
					trees = append(trees, _trees...)
				}
			}
		}

		// Store photo or video based on file extension if it's a root leaf
		itemName := rootLeaf.ItemName
		leafCount := rootLeaf.LeafCount
		hash := rootLeaf.Hash

		kindName := GetKindFromItemName(itemName)

		ChunkSize := 2048 * 1024

		var relaySettings types.RelaySettings
		if err := viper.UnmarshalKey("relay_settings", &relaySettings); err != nil {
			log.Fatalf("Error unmarshaling relay settings: %v", err)
		}

		var sizeMB float64
		if leafCount > 0 {
			sizeMB = float64(leafCount*ChunkSize) / (1024 * 1024) // Convert to MB
		} else {
			sizeBytes := leafContentSize
			sizeMB = float64(sizeBytes) / (1024 * 1024) // Convert to MB
		}

		gormDB, err := InitGorm()
		if err != nil {
			return err
		}

		mode := relaySettings.Mode

		// Process file according to the mode (smart or unlimited)
		if mode == "smart" {
			// In smart mode, check if the file type is blocked
			if contains(append(append(relaySettings.Photos, relaySettings.Videos...), relaySettings.Audio...), strings.ToLower(kindName)) {
				return fmt.Errorf("file type not permitted: %s", kindName)
			}

			// Save the file under the correct category if not blocked
			if contains(relaySettings.Photos, strings.ToLower(kindName)) {
				photo := types.Photo{
					Hash:      hash,
					LeafCount: leafCount,
					KindName:  kindName,
					Size:      sizeMB,
				}
				gormDB.Create(&photo)
			} else if contains(relaySettings.Videos, strings.ToLower(kindName)) {
				video := types.Video{
					Hash:      hash,
					LeafCount: leafCount,
					KindName:  kindName,
					Size:      sizeMB,
				}
				gormDB.Create(&video)
			} else if contains(relaySettings.Audio, strings.ToLower(kindName)) {
				audio := types.Audio{
					Hash:      hash,
					LeafCount: leafCount,
					KindName:  kindName,
					Size:      sizeMB,
				}
				gormDB.Create(&audio)
			} else {
				// Save the file under Misc if it doesn't fall under any specific category
				misc := types.Misc{
					Hash:      hash,
					LeafCount: leafCount,
					KindName:  itemName,
					Size:      sizeMB,
				}
				gormDB.Create(&misc)
			}
		} else if mode == "unlimited" {
			// In unlimited mode, check if the file type is blocked
			if contains(append(append(relaySettings.Photos, relaySettings.Videos...), relaySettings.Audio...), strings.ToLower(kindName)) {
				return fmt.Errorf("blocked file type: %s", kindName)
			}

			// Save the file under the correct category if not blocked
			if contains(relaySettings.Photos, strings.ToLower(kindName)) {
				photo := types.Photo{
					Hash:      hash,
					LeafCount: leafCount,
					KindName:  kindName,
					Size:      sizeMB,
				}
				gormDB.Create(&photo)
			} else if contains(relaySettings.Videos, strings.ToLower(kindName)) {
				video := types.Video{
					Hash:      hash,
					LeafCount: leafCount,
					KindName:  kindName,
					Size:      sizeMB,
				}
				gormDB.Create(&video)
			} else if contains(relaySettings.Audio, strings.ToLower(kindName)) {
				audio := types.Audio{
					Hash:      hash,
					LeafCount: leafCount,
					KindName:  kindName,
					Size:      sizeMB,
				}
				gormDB.Create(&audio)
			} else {
				// Save the file under Misc if it doesn't fall under any specific category
				misc := types.Misc{
					Hash:      hash,
					LeafCount: leafCount,
					KindName:  itemName,
					Size:      sizeMB,
				}
				gormDB.Create(&misc)
			}
		}
	}

	if contentTree != nil {
		trees = append(trees, contentTree)
	}

	_, err = graviton.Commit(trees...)
	if err != nil {
		return err
	}

	return nil
}

// Retrieve an individual scionic merkletree leaf from the tree's root hash and the leaf hash
func (store *GravitonStore) RetrieveLeaf(root string, hash string, includeContent bool) (*types.DagLeafData, error) {
	key := []byte(hash) // merkle_dag.GetHash(hash)

	snapshot, err := store.Database.LoadSnapshot(0)
	if err != nil {
		return nil, err
	}

	bucket, err := store.retrieveBucket(root)
	if err != nil {
		return nil, err
	}

	tree, err := snapshot.GetTree(bucket)
	if err != nil {
		return nil, err
	}

	//log.Printf("Searching for leaf with key: %s\nFrom bucket: %s", key, bucket)
	bytes, err := tree.Get(key)
	if err != nil {
		return nil, err
	}

	var data *types.DagLeafData = &types.DagLeafData{}

	err = cbor.Unmarshal(bytes, data)
	if err != nil {
		return nil, err
	}

	if includeContent && data.Leaf.ContentHash != nil {
		//fmt.Println("Fetching  leaf content")

		content, err := store.RetrieveLeafContent(data.Leaf.ContentHash)
		if err != nil {
			return nil, err
		}

		data.Leaf.Content = content
	}

	//fmt.Println("Leaf found")

	return data, nil
}

// Retrieve the content for a scionic merkletree leaf based on the hash of the content
// We can reduce the total data stored by ensuring all data is content addressed as sometimes
// leaves will have different data which changes the root hash but the actual content could be
// the same as other leaves already stored on this relay
func (store *GravitonStore) RetrieveLeafContent(contentHash []byte) ([]byte, error) {
	snapshot, err := store.Database.LoadSnapshot(0)
	if err != nil {
		return nil, err
	}

	contentTree, err := snapshot.GetTree("content")
	if err != nil {
		return nil, err
	}

	bytes, err := contentTree.Get(contentHash)
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
func (store *GravitonStore) retrieveBucket(root string) (string, error) {
	snapshot, err := store.Database.LoadSnapshot(0)
	if err != nil {
		return "", err
	}

	tree, err := snapshot.GetTree("scionic_index")
	if err != nil {
		return "", err
	}

	bytes, err := tree.Get([]byte(root))
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

// Retrieve and build an entire scionic merkletree from the root hash
func (store *GravitonStore) BuildDagFromStore(root string, includeContent bool) (*types.DagData, error) {
	return stores.BuildDagFromStore(store, root, includeContent)
}

// Store an entire scionic merkltree (not implemented currently as not required, leaves are stored as received)
func (store *GravitonStore) StoreDag(dag *types.DagData) error {
	return stores.StoreDag(store, dag)
}

// Nostr events
// Query nostr events based on given filters utilizing the cacheing buckets to increase query speed
func (store *GravitonStore) QueryEvents(filter nostr.Filter) ([]*nostr.Event, error) {
	var events []*nostr.Event

	snapshot, err := store.Database.LoadSnapshot(0)
	if err != nil {
		return nil, err
	}

	// Check for any single letter tag queries per author and check cached
	// buckets first as this should be massively quicker as the data set grows
	for _, author := range filter.Authors {
		for key, _ := range filter.Tags {
			if IsTagQueryTag(key) {
				hashes, err := store.getCache(author, key)
				if err == nil {
					queryFilter := nostr.Filter{
						IDs: hashes,
					}

					queryEvents, err := store.QueryEvents(queryFilter)
					if err == nil {
						for _, event := range queryEvents {
							if filter.Matches(event) {
								events = append(events, event)
							}

							for f, v := range filter.Tags {
								if v != nil && (ContainsAny(event.Tags, f, v) || ContainsAnyWithWildcard(event.Tags, f, v)) {
									events = append(events, event)
								}
							}
						}
					}
				}
			}
		}
	}

	// We should only perform the full check if there are no tags and there are no authors
	// because if there was the above cache check should have found it which will be
	// a lot faster than the full check below
	if len(filter.Authors) <= 0 || len(filter.Tags) <= 0 {
		// Convert search term to lowercase for case-insensitive comparison
		searchTerm := strings.ToLower(filter.Search)

		masterBucketList, err := store.GetMasterBucketList("kinds")
		if err != nil {
			return nil, err
		}

		for _, bucket := range masterBucketList {
			if strings.HasPrefix(bucket, "kind") {
				tree, err := snapshot.GetTree(bucket)
				if err != nil {
					continue // Skip this bucket if there's an error
				}

				c := tree.Cursor()
				for _, v, err := c.First(); err == nil; _, v, err = c.Next() {
					var event nostr.Event
					if err := jsoniter.Unmarshal(v, &event); err != nil {
						continue // Skip on unmarshal error
					}

					// Check tags first because pretty sure go-nostr doesn't take the # in filters into consideration
					// and we want to use the wildcard system for the f and d tag paths
					for f, v := range filter.Tags {
						if v != nil && (ContainsAny(event.Tags, f, v) || ContainsAnyWithWildcard(event.Tags, f, v)) {
							events = append(events, &event)
							continue
						}
					}

					// Check if the event matches the filter criteria, including kind
					if !filter.Matches(&event) {
						continue
					}

					// Implement search logic here, after ensuring the event matches the filter
					if searchTerm != "" && !strings.Contains(strings.ToLower(event.Content), searchTerm) {
						continue // If the lowercase content doesn't contain the lowercase search term, skip
					}

					// If the event passes both the filter and search, add it to the results
					events = append(events, &event)
				}
			}
		}
	}

	// Sort the events based on creation time, most recent first
	sort.Slice(events, func(i, j int) bool {
		return events[i].CreatedAt > events[j].CreatedAt
	})

	// Apply the limit, if specified
	if filter.Limit > 0 && len(events) > filter.Limit {
		events = events[:filter.Limit]
	}

	jsonFilter, err := json.Marshal(filter)
	if err != nil {
		log.Println("Found", len(events), "matching events")
	} else {
		log.Println("Found", len(events), "matching events for filter: ", string(jsonFilter))
	}
	return events, nil
}

func (store *GravitonStore) StoreEvent(event *nostr.Event) error {
	eventData, err := jsoniter.Marshal(event)
	if err != nil {
		return err
	}

	bucket := fmt.Sprintf("kind:%d", event.Kind)

	trees := []*graviton.Tree{}

	ss, _ := store.Database.LoadSnapshot(0)
	tree, _ := ss.GetTree(bucket)

	trees = append(trees, tree)

	// Cache event against pubkey and kind bucket
	_trees, err := store.cacheKey(event.PubKey, bucket, event.ID)
	if err == nil {
		trees = append(trees, _trees...)
	}

	// Cache any single letter tags against pubkey and the #tag letter
	for _, tag := range event.Tags {
		key := tag.Key()
		if IsSingleLetter(key) {
			_trees, err := store.cacheKey(event.PubKey, fmt.Sprintf("#%s", key), event.ID)
			if err == nil {
				trees = append(trees, _trees...)
			}
		}
	}

	err = tree.Put([]byte(event.ID), eventData)
	if err != nil {
		return err
	}

	masterBucketListTree, err := store.UpdateMasterBucketList("kinds", bucket)
	if err != nil {
		return err
	}

	if masterBucketListTree != nil {
		trees = append(trees, masterBucketListTree)
	}

	_, err = graviton.Commit(trees...)
	if err != nil {
		return err
	}

	// Store event in Gorm SQLite database
	storeInGorm(event)

	return nil
}

func (store *GravitonStore) DeleteEvent(eventID string) error {
	snapshot, err := store.Database.LoadSnapshot(0)
	if err != nil {
		return err
	}

	event, err := store.QueryEvents(nostr.Filter{IDs: []string{eventID}})
	if err != nil {
		return err
	}

	// event kind number is an integer
	kindInt, _ := strconv.ParseInt(fmt.Sprintf("%d", event[0].Kind), 10, 64)

	bucket := fmt.Sprintf("kind:%d", kindInt)

	tree, err := snapshot.GetTree(bucket)
	if err == nil {
		err := tree.Delete([]byte(eventID))
		if err != nil {
			return err
		} else {
			log.Println("Deleted event", eventID)
		}

	}
	graviton.Commit(tree)

	gormDB, err := InitGorm()
	if err != nil {
		return err
	}

	// Delete event from Gorm SQLite database
	gormDB.Delete(&types.Kind{}, "event_id = ?", eventID)

	return nil
}

func (store *GravitonStore) CountFileLeavesByType() (map[string]int, error) {
	snapshot, err := store.Database.LoadSnapshot(0)
	if err != nil {
		return nil, err
	}

	treeNames := []string{"content"}

	fileTypeCounts := make(map[string]int)

	for _, treeName := range treeNames {
		tree, err := snapshot.GetTree(treeName)
		if err != nil {
			continue
		}

		c := tree.Cursor()

		for _, v, err := c.First(); err == nil; _, v, err = c.Next() {
			var leaf *merkle_dag.DagLeaf
			err := cbor.Unmarshal(v, &leaf)
			if err != nil {
				continue
			}

			if leaf.Type == merkle_dag.FileLeafType {
				splitName := strings.Split(leaf.ItemName, ".")
				if len(splitName) > 1 {
					extension := strings.ToLower(splitName[len(splitName)-1])
					fileTypeCounts[extension]++
				}
			}
		}
	}

	return fileTypeCounts, nil
}

// Blossom Blobs (unchunked data)
func (store *GravitonStore) StoreBlob(data []byte, hash []byte, publicKey string) error {
	snapshot, err := store.Database.LoadSnapshot(0)
	if err != nil {
		return err
	}

	contentTree, err := snapshot.GetTree("content")
	if err != nil {
		return err
	}

	encodedHash := hex.EncodeToString(hash[:])

	cacheTrees, err := store.cacheKey(publicKey, "blossom", encodedHash)
	if err != nil {
		return err
	}

	contentTree.Put(hash[:], data)

	cacheTrees = append(cacheTrees, contentTree)

	graviton.Commit(cacheTrees...)

	return nil
}

func (store *GravitonStore) GetBlob(hash string) ([]byte, error) {
	snapshot, err := store.Database.LoadSnapshot(0)
	if err != nil {
		return nil, err
	}

	contentTree, err := snapshot.GetTree("content")
	if err != nil {
		return nil, err
	}

	hashBytes, err := hex.DecodeString(hash)
	if err != nil {
		return nil, err
	}

	content, err := contentTree.Get(hashBytes)
	if err != nil {
		return nil, err
	}

	return content, nil
}

func (store *GravitonStore) DeleteBlob(hash string) error {
	snapshot, err := store.Database.LoadSnapshot(0)
	if err != nil {
		return err
	}

	contentTree, err := snapshot.GetTree("content")
	if err != nil {
		return err
	}

	hashBytes, err := hex.DecodeString(hash)
	if err != nil {
		return err
	}

	contentTree.Delete(hashBytes)

	graviton.Commit(contentTree)

	return nil
}

// This is used to create / update cache buckets with hashes that point to nostr notes or
// scionic merkletree data depending on where it is called from
// All cache buckets are prefixed with cache: and stored in the "cache" master bucket list
func (store *GravitonStore) cacheKey(bucket string, key string, root string) ([]*graviton.Tree, error) {
	snapshot, err := store.Database.LoadSnapshot(0)
	if err != nil {
		return nil, err
	}

	trees := []*graviton.Tree{}

	cacheBucket := fmt.Sprintf("cache:%s", bucket)

	cacheTree, err := snapshot.GetTree(cacheBucket)
	if err == nil {
		value, err := cacheTree.Get([]byte(key))

		if err == nil && value != nil {
			var cacheData *types.CacheData = &types.CacheData{}

			err = cbor.Unmarshal(value, cacheData)
			if err == nil && !contains(cacheData.Keys, root) {
				cacheData.Keys = append(cacheData.Keys, root)

				serializedData, err := cbor.Marshal(cacheData)
				if err == nil {
					cacheTree.Put([]byte(key), serializedData)

					trees = append(trees, cacheTree)
				}
			}
		} else {
			cacheData := &types.CacheData{
				Keys: []string{root},
			}

			serializedData, err := cbor.Marshal(cacheData)
			if err == nil {
				cacheTree.Put([]byte(key), serializedData)

				trees = append(trees, cacheTree)
			}
		}
	}

	masterBucketListTree, err := store.UpdateMasterBucketList("cache", cacheBucket)
	if err != nil {
		return trees, nil
	}

	if masterBucketListTree != nil {
		trees = append(trees, masterBucketListTree)
	}

	return trees, nil
}

// Retrieve a cache (list of hashes) given the bucket and key
func (store *GravitonStore) getCache(bucket string, key string) ([]string, error) {
	snapshot, err := store.Database.LoadSnapshot(0)
	if err == nil {
		cacheBucket := fmt.Sprintf("cache:%s", bucket)
		cacheTree, err := snapshot.GetTree(cacheBucket)
		if err == nil {
			value, err := cacheTree.Get([]byte(key))
			if err == nil {
				if err == nil && value != nil {
					var cacheData *types.CacheData = &types.CacheData{}

					err = cbor.Unmarshal(value, cacheData)
					if err == nil {
						return cacheData.Keys, nil
					}
				}
			}
		}
	}

	fmt.Printf("Failed to unmrashal cache bucket %s with key %s\n", bucket, key)
	return nil, nil
}

// The master bucket list is a bucket that contains lists of all other buckets
// This allows us to retrieve and itterate buckets without the need for graviton to support it
func (store *GravitonStore) UpdateMasterBucketList(key string, bucket string) (*graviton.Tree, error) {
	snapshot, _ := store.Database.LoadSnapshot(0)

	tree, err := snapshot.GetTree("mbl")
	if err != nil {
		return nil, err
	}

	var masterBucketList []string

	bytes, err := tree.Get([]byte(fmt.Sprintf("mbl_%s", key)))
	if bytes == nil || err != nil {
		masterBucketList = []string{}
	} else {
		err = cbor.Unmarshal(bytes, &masterBucketList)
		if err != nil {
			return nil, err
		}
	}

	if contains(masterBucketList, bucket) {
		return nil, nil
	} else {
		masterBucketList = append(masterBucketList, bucket)

		bytes, err = cbor.Marshal(masterBucketList)
		if err != nil {
			return nil, err
		}

		err = tree.Put([]byte(fmt.Sprintf("mbl_%s", key)), bytes)
		if err != nil {
			return nil, err
		}
	}

	return tree, nil
}

// You can get an array of bucket keys by specifying which list of buckets you want
// We break the master bucket list up to speed up itteration depending on what buckets you want
// An example of this would be to pass in "cache" as the key to get all the cache buckets
func (store *GravitonStore) GetMasterBucketList(key string) ([]string, error) {
	snapshot, _ := store.Database.LoadSnapshot(0)

	tree, err := snapshot.GetTree("mbl")
	if err != nil {
		return nil, err
	}

	var masterBucketList []string

	bytes, err := tree.Get([]byte(fmt.Sprintf("mbl_%s", key)))
	if bytes == nil || err != nil {
		masterBucketList = []string{}
	} else {
		err = cbor.Unmarshal(bytes, &masterBucketList)
		if err != nil {
			return nil, err
		}
	}

	return masterBucketList, nil
}

// This determines what bucket a scionic merkletree should be stored in based on its file type
// The root hashes may also be cached in several other cache buckets depending on the AdditionalData fields
func GetBucket(leaf *merkle_dag.DagLeaf) string {
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
