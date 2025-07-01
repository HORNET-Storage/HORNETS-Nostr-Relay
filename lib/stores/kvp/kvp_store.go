package kvp

import "io"

type KeyValueStore interface {
	GetBucket(prefix string) KeyValueStoreBucket
	UpdateBucketList(prefix string)
	GetBucketList() []string
	Cleanup() error
}

type KeyValueStoreBucket interface {
	GetPrefix() string
	Get(key string) ([]byte, error)
	Put(key string, value []byte) error
	Delete(keys []string) error
	Scan() (Iterator, error)
}

type Iterator interface {
	// Next advances the iterator to the next key-value pair
	// Returns false when there are no more items or if an error occurred
	Next() bool

	// Key returns the current key
	Key() []byte

	// Value returns the current value
	Value() []byte

	// Error returns any error encountered during iteration
	Error() error

	// Close releases any resources associated with the iterator
	io.Closer
}
