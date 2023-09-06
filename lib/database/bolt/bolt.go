package bolt

import (
	"log"
	"time"

	"github.com/boltdb/bolt"
)

type BoltDatabase struct {
	Db *bolt.DB
}

func CreateDatabase(name string) (*BoltDatabase, error) {
	db, err := bolt.Open(name+".db", 0600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		log.Printf("Failed to create bolt database: %v", err)
		return nil, err
	}

	boltDb := &BoltDatabase{
		Db: db,
	}

	return boltDb, nil
}

func (bdb *BoltDatabase) CreateBucket(name string) error {
	err := bdb.Db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(name))
		return err
	})

	if err != nil {
		return err
	}

	return nil
}

func (bdb *BoltDatabase) GetValue(name string, key string) ([]byte, error) {
	var value []byte

	err := bdb.Db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(name))
		value = bucket.Get([]byte(key))
		return nil
	})

	if err != nil {
		return nil, err
	}

	return value, nil
}

func (bdb *BoltDatabase) UpdateValue(name string, key string, value []byte) error {
	err := bdb.Db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(name))
		err := bucket.Put([]byte(key), value)
		return err
	})

	if err != nil {
		return err
	}

	return nil
}
