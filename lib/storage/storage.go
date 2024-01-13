package storage

import (
	"strings"

	"github.com/HORNET-Storage/hornet-storage/lib/database/bbolt"
)

type Storage struct {
	// Content specific database to minimise duplicate data
	ContentDatabase *bbolt.Database

	// Main database broken down into buckets per app per user
	UserDatabase *bbolt.Database
}

// Prefix allows for multiple storages to be created if ever wanted or needed
func CreateStorage(prefix string) (*Storage, error) {
	content := strings.Join([]string{prefix, "content"}, "-")
	users := strings.Join([]string{prefix, "users"}, "-")

	contentDatabase, err := bbolt.CreateDatabase(content)
	if err != nil {
		return nil, err
	}

	userDatabase, err := bbolt.CreateDatabase(users)
	if err != nil {
		return nil, err
	}

	storage := &Storage{
		ContentDatabase: contentDatabase,
		UserDatabase:    userDatabase,
	}

	return storage, nil
}

func (storage *Storage) CreateUserStorage(pubkey string) error {
	err := storage.UserDatabase.CreateBucket(pubkey)
	if err != nil {
		return err
	}

	return nil
}

func (storage *Storage) CreateUserAppStorage(pubkey string, app string) error {
	err := storage.UserDatabase.CreatedNestedBucket(pubkey, app)

	return err
}

func (storage *Storage) UpdateUserAppData(pubkey string, app string, key string, value []byte) error {
	err := storage.UserDatabase.UpdateNestedValue(pubkey, app, key, value)

	return err
}

func (storage *Storage) GetUserAppData(pubkey string, app string, key string) ([]byte, error) {
	bytes, err := storage.UserDatabase.GetNestedValue(pubkey, app, key)
	if err != nil {
		return nil, err
	}

	return bytes, nil
}

func (storage *Storage) UpdateContentData(key string, value []byte) error {
	err := storage.UserDatabase.UpdateValue("default", key, value)

	return err
}

func (storage *Storage) GetContentData(key string) ([]byte, error) {
	bytes, err := storage.UserDatabase.GetValue("default", key)
	if err != nil {
		return nil, err
	}

	return bytes, nil
}
