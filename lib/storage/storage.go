package storage

import (
	"context"
	"strings"

	"github.com/HORNET-Storage/hornet-storage/lib/database/bbolt"

	keys "github.com/HORNET-Storage/hornet-storage/lib/context"
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

func CreateUserStorage(ctx context.Context, pubkey string) error {
	storage := ctx.Value(keys.ContentDatabase).(*Storage)

	// Anything uploaded that does not specify an app name should use this bucket
	err := storage.UserDatabase.CreateBucket("default")
	if err != nil {
		return err
	}

	return nil
}

func CreateUserAppStorage(ctx context.Context, pubkey string, app string) error {
	storage := ctx.Value(keys.ContentDatabase).(*Storage)

	err := storage.ContentDatabase.CreatedNestedBucket(pubkey, app)

	return err
}

func UpdateUserAppData(ctx context.Context, pubkey string, app string, key string, value []byte) error {
	storage := ctx.Value(keys.ContentDatabase).(*Storage)

	err := storage.ContentDatabase.UpdateNestedValue(pubkey, app, key, value)

	return err
}

func GetUserAppData(ctx context.Context, pubkey string, app string, key string) ([]byte, error) {
	storage := ctx.Value(keys.ContentDatabase).(*Storage)

	bytes, err := storage.ContentDatabase.GetNestedValue(pubkey, app, key)
	if err != nil {
		return nil, err
	}

	return bytes, nil
}
