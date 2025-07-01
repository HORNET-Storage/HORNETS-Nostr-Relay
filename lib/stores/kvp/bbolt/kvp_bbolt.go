package bbolt

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/HORNET-Storage/hornet-storage/lib/stores/kvp"
	"github.com/fxamacker/cbor/v2"
	"go.etcd.io/bbolt"
)

const BucketListPrefix = "mbl"

type Buckets struct {
	db *bbolt.DB
	mu sync.RWMutex
}

type Bucket struct {
	prefix  []byte
	buckets *Buckets
}

type BucketList struct {
	buckets []string
}

type Iterator struct {
	cursor *bbolt.Cursor
	prefix []byte
	k, v   []byte
	err    error
}

func InitBuckets(path string) (*Buckets, error) {
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("could not open db: %v", err)
	}

	return &Buckets{
		db: db,
	}, nil
}

func (b *Buckets) Cleanup() error {
	return b.db.Close()
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
	b.mu.Lock()
	defer b.mu.Unlock()

	b.db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(BucketListPrefix))
		if err != nil {
			return err
		}

		var list BucketList
		data := bucket.Get([]byte("list"))
		if data != nil {
			if err := cbor.Unmarshal(data, &list); err != nil {
				return err
			}
		}

		for _, b := range list.buckets {
			if b == prefix {
				return nil
			}
		}

		list.buckets = append(list.buckets, prefix)
		encoded, err := cbor.Marshal(list)
		if err != nil {
			return err
		}

		return bucket.Put([]byte("list"), encoded)
	})
}

func (b *Buckets) GetBucketList() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var list BucketList
	b.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketListPrefix))
		if bucket == nil {
			return nil
		}

		data := bucket.Get([]byte("list"))
		if data == nil {
			return nil
		}

		return cbor.Unmarshal(data, &list)
	})

	return list.buckets
}

func (b *Bucket) GetPrefix() string {
	return string(b.prefix)
}

func (b *Bucket) Get(key string) ([]byte, error) {
	var value []byte
	err := b.buckets.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(b.prefix))
		if bucket == nil {
			return fmt.Errorf("bucket %s not found", b.prefix)
		}

		result := bucket.Get([]byte(key))
		if result == nil {
			return fmt.Errorf("key %s not found", key)
		}

		value = make([]byte, len(result))
		copy(value, result)
		return nil
	})

	return value, err
}

func (b *Bucket) Put(key string, value []byte) error {
	return b.buckets.db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(b.prefix))
		if err != nil {
			return fmt.Errorf("could not create bucket: %v", err)
		}

		return bucket.Put([]byte(key), value)
	})
}

func (b *Bucket) Delete(keys []string) error {
	return b.buckets.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(b.prefix))
		if bucket == nil {
			return fmt.Errorf("bucket %s not found", b.prefix)
		}

		for _, key := range keys {
			err := bucket.Delete([]byte(key))
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func (b *Bucket) Scan() (kvp.Iterator, error) {
	tx, err := b.buckets.db.Begin(false)
	if err != nil {
		return nil, err
	}

	bucket := tx.Bucket([]byte(b.prefix))
	if bucket == nil {
		tx.Rollback()
		return nil, bbolt.ErrBucketNotFound
	}

	cursor := bucket.Cursor()
	iter := &Iterator{
		cursor: cursor,
		prefix: []byte(b.prefix),
	}

	// Set up the initial position
	iter.k, iter.v = cursor.Seek([]byte(b.prefix))

	return iter, nil
}

func (it *Iterator) Next() bool {
	if it.k == nil {
		return false
	}

	if it.prefix != nil && !bytes.HasPrefix(it.k, it.prefix) {
		it.k, it.v = nil, nil
		return false
	}

	if it.k != nil {
		it.k, it.v = it.cursor.Next()
	}

	return it.k != nil && (it.prefix == nil || bytes.HasPrefix(it.k, it.prefix))
}

func (it *Iterator) Key() []byte {
	return it.k
}

func (it *Iterator) Value() []byte {
	return it.v
}

func (it *Iterator) Error() error {
	return it.err
}

func (it *Iterator) Close() error {
	return it.cursor.Bucket().Tx().Rollback()
}
