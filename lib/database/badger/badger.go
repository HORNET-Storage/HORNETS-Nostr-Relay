package badger

import (
	"log"

	"github.com/dgraph-io/badger/v4"
)

type BadgerDB struct {
	Db *badger.DB
}

func Open(name string) (*BadgerDB, error) {
	db, err := badger.Open(badger.DefaultOptions(name))
	if err != nil {
		return nil, err
	}

	badgerDB := &BadgerDB{
		Db: db,
	}

	return badgerDB, nil
}

func (db *BadgerDB) Update(key string, value []byte) error {
	err := db.Db.Update(func(txn *badger.Txn) error {
		e := txn.Set([]byte(key), value)
		if e != nil {
			log.Printf("Failed to update database: %e\n", e)
		}
		return e
	})

	if err != nil {
		log.Printf("Failed to update database: %e\n", err)
	}

	return err
}

func (db *BadgerDB) UpdateFromByteKey(key []byte, value []byte) error {
	err := db.Db.Update(func(txn *badger.Txn) error {
		e := txn.Set(key, value)
		return e
	})

	return err
}

func (db *BadgerDB) Get(key string) ([]byte, error) {
	var valCopy []byte

	err := db.Db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}

		valCopy, err = item.ValueCopy(nil)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return valCopy, nil
}

func (db *BadgerDB) GetFromByteKey(key []byte) ([]byte, error) {
	var valCopy []byte

	err := db.Db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}

		valCopy, err = item.ValueCopy(nil)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return valCopy, nil
}
