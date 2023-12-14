package bbolt

import (
	"log"
	"time"

	"go.etcd.io/bbolt"
)

type Database struct {
	Db *bbolt.DB
}

func CreateDatabase(name string) (*Database, error) {
	db, err := bbolt.Open(name+".db", 0600, &bbolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		log.Printf("Failed to create bbolt database: %v", err)
		return nil, err
	}

	bboltDb := &Database{
		Db: db,
	}

	return bboltDb, nil
}

func (bdb *Database) CreateBucket(name string) error {
	err := bdb.Db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(name))
		return err
	})

	if err != nil {
		return err
	}

	return nil
}

func (bdb *Database) GetValue(name string, key string) ([]byte, error) {
	var value []byte

	err := bdb.Db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(name))
		value = bucket.Get([]byte(key))
		return nil
	})

	if err != nil {
		return nil, err
	}

	return value, nil
}

func (bdb *Database) UpdateValue(name string, key string, value []byte) error {
	err := bdb.Db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(name))
		err := bucket.Put([]byte(key), value)
		return err
	})

	if err != nil {
		return err
	}

	return nil
}

// Should really make something smarter than this but it will do for now (for managing data in nested app buckets)
func (bdb *Database) CreatedNestedBucket(name string, nestedName string) error {
	err := bdb.Db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(name))

		if err == nil {
			bucket := tx.Bucket([]byte(name))

			_, err := bucket.CreateBucketIfNotExists([]byte(nestedName))

			return err
		}

		return err
	})

	if err != nil {
		return err
	}

	return nil
}

func (bdb *Database) GetNestedValue(name string, nestedName string, key string) ([]byte, error) {
	var value []byte

	err := bdb.Db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(name))
		nestedBucket := bucket.Bucket([]byte(nestedName))

		value = nestedBucket.Get([]byte(key))

		return nil
	})

	if err != nil {
		return nil, err
	}

	return value, nil
}

func (bdb *Database) UpdateNestedValue(name string, nestedName string, key string, value []byte) error {
	err := bdb.Db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(name))
		nestedBucket := bucket.Bucket([]byte(nestedName))

		err := nestedBucket.Put([]byte(key), value)
		return err
	})

	if err != nil {
		return err
	}

	return nil
}
