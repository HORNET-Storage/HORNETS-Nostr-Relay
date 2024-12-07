package immudb

import (
	"bytes"
	"context"
	"errors"

	"github.com/HORNET-Storage/hornet-storage/lib/stores/kvp"
	"github.com/codenotary/immudb/pkg/api/schema"
	immudb "github.com/codenotary/immudb/pkg/client"
	"github.com/fxamacker/cbor/v2"
)

const PrefixSeperator = '_'
const BucketListPrefix = "mbl"

var (
	ErrEmptyKey    = errors.New("key cannot be empty")
	ErrEmptyPrefix = errors.New("bucket prefix cannot be empty")
	ErrNilValue    = errors.New("value cannot be nil")
	ErrKeyNotFound = errors.New("key not found")
)

// KeyValueStore
type Buckets struct {
	buckets []string

	ctx    context.Context
	client immudb.ImmuClient
}

// KeyValueStoreBucket
type Bucket struct {
	prefix []byte

	buckets *Buckets
}

type BucketList struct {
	buckets []string
}

type Iterator struct {
	entries *schema.Entries
	current int
	prefix  []byte
	err     error
}

func InitBuckets(ctx context.Context, client immudb.ImmuClient) (*Buckets, error) {
	buckets := &Buckets{
		ctx:    ctx,
		client: client,
	}

	return buckets, nil
}

func (b *Buckets) Cleanup() error {
	return b.client.CloseSession(b.ctx)
}

func (b *Buckets) GetBucket(prefix string) kvp.KeyValueStoreBucket {
	if prefix != BucketListPrefix {
		b.UpdateBucketList(prefix)
	}

	return &Bucket{
		prefix:  []byte(prefix),
		buckets: b,
	}
}

func (b *Buckets) UpdateBucketList(prefix string) {
	bucket := b.GetBucket(BucketListPrefix)

	var list BucketList

	bytes, err := bucket.Get("list")
	if err != nil || len(bytes) <= 0 {
		list = BucketList{
			buckets: []string{},
		}
	} else {
		cbor.Unmarshal(bytes, list)
	}

	if !contains(list.buckets, prefix) {
		list.buckets = append(list.buckets, prefix)
	}

	bytes, err = cbor.Marshal(list)
	if err != nil {
		return
	}

	bucket.Put("list", bytes)
}

func (b *Buckets) GetBucketList() []string {
	bucket := b.GetBucket(BucketListPrefix)

	var list BucketList

	bytes, err := bucket.Get("list")
	if err != nil || len(bytes) <= 0 {
		list = BucketList{
			buckets: []string{},
		}
	} else {
		cbor.Unmarshal(bytes, list)
	}

	return list.buckets
}

func (b *Bucket) GetPrefix() string {
	return string(b.prefix)
}

func (b *Bucket) ValidateKeys(keys []string) [][]byte {
	result := make([][]byte, len(keys))

	for i, key := range keys {
		result[i] = []byte(key)
	}

	b.ValidateByteKeys(result)

	return result
}

func (b *Bucket) ValidateByteKeys(keys [][]byte) {
	for _, key := range keys {
		key = b.ValidateByteKey(key)
	}
}

func (b *Bucket) ValidateByteKey(key []byte) []byte {
	if !bytes.HasPrefix(key, b.prefix) {
		return append(append([]byte{}, b.prefix...), key...)
	}

	return key
}

func (b *Bucket) ValidateKey(key string) []byte {
	byteKey := []byte(key)

	return b.ValidateByteKey(byteKey)
}

func (b *Bucket) Put(key string, value []byte) error {
	if len(key) == 0 {
		return ErrEmptyKey
	}

	if value == nil {
		return ErrNilValue
	}

	prefixedKey := b.ValidateKey(key)

	_, err := b.buckets.client.Set(b.buckets.ctx, prefixedKey, value)

	return err
}

func (b *Bucket) Get(key string) ([]byte, error) {
	if len(key) == 0 {
		return nil, ErrEmptyKey
	}

	prefixedKey := b.ValidateKey(key)

	entry, err := b.buckets.client.Get(b.buckets.ctx, prefixedKey)
	if err != nil {
		return nil, err
	}

	return entry.Value, nil
}

func (b *Bucket) Delete(keys []string) error {
	byteKeys := b.ValidateKeys(keys)

	_, err := b.buckets.client.Delete(b.buckets.ctx, &schema.DeleteKeysRequest{
		Keys: byteKeys,
	})
	return err
}

func (b *Bucket) Scan() (kvp.Iterator, error) {
	entries, err := b.buckets.client.Scan(b.buckets.ctx, &schema.ScanRequest{
		Prefix: b.prefix,
		Desc:   false,
	})
	if err != nil {
		return nil, err
	}

	return &Iterator{
		entries: entries,
		current: -1,
		prefix:  b.prefix,
	}, nil
}

func (b *Bucket) ScanWithOptions(limit uint64, desc bool) (*schema.Entries, error) {
	return b.buckets.client.Scan(b.buckets.ctx, &schema.ScanRequest{
		Prefix: b.prefix,
		Limit:  limit,
		Desc:   desc,
	})
}

func (b *Bucket) VerifiedPut(key string, value []byte) error {
	if len(key) == 0 {
		return ErrEmptyKey
	}

	if value == nil {
		return ErrNilValue
	}

	prefixedKey := b.ValidateKey(key)

	_, err := b.buckets.client.VerifiedSet(b.buckets.ctx, prefixedKey, value)

	return err
}

func (b *Bucket) VerifiedGet(key string) ([]byte, error) {
	if len(key) == 0 {
		return nil, ErrEmptyKey
	}

	prefixedKey := b.ValidateKey(key)

	entry, err := b.buckets.client.VerifiedGet(b.buckets.ctx, prefixedKey)
	if err != nil {
		return nil, err
	}

	return entry.Value, nil
}

func (b *Bucket) GetAt(key string, txID uint64) ([]byte, error) {
	if len(key) == 0 {
		return nil, ErrEmptyKey
	}

	prefixedKey := b.ValidateKey(key)

	entry, err := b.buckets.client.GetAt(b.buckets.ctx, prefixedKey, txID)
	if err != nil {
		return nil, err
	}

	return entry.Value, nil
}

func (it *Iterator) Next() bool {
	it.current++
	return it.current < len(it.entries.Entries)
}

func (it *Iterator) Key() []byte {
	if it.current >= len(it.entries.Entries) {
		return nil
	}
	return bytes.TrimPrefix(it.entries.Entries[it.current].Key, it.prefix)
}

func (it *Iterator) Value() []byte {
	if it.current >= len(it.entries.Entries) {
		return nil
	}
	return it.entries.Entries[it.current].Value
}

func (it *Iterator) Error() error {
	return it.err
}

func (it *Iterator) Close() error {
	return nil
}

func contains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}

	return false
}
