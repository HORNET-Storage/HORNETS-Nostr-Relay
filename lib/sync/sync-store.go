package sync

import (
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// GORM models for sync / dht related structs
type SyncAuthor struct {
	gorm.Model
	PublicKey string `gorm:"size:128;uniqueIndex"`
}

type SyncRelay struct {
	gorm.Model
	PublicKey string `gorm:"size:128;uniqueIndex"`
	RelayInfo string `gorm:"type:text"`
}

type DHTUploadable struct {
	gorm.Model
	Payload   []byte
	Pubkey    []byte
	Signature []byte
}

func GetSyncAuthors(db *gorm.DB) ([]SyncAuthor, error) {
	var syncAuthors []SyncAuthor
	if err := db.Find(&syncAuthors).Error; err != nil {
		return nil, err
	}
	return syncAuthors, nil
}

func GetSyncRelays(db *gorm.DB) ([]SyncRelay, error) {
	var syncRelays []SyncRelay
	if err := db.Find(&syncRelays).Error; err != nil {
		return nil, err
	}
	return syncRelays, nil
}

func GetDHTUploadables(db *gorm.DB) ([]DHTUploadable, error) {
	var dhtUploadables []DHTUploadable
	if err := db.Find(&dhtUploadables).Error; err != nil {
		return nil, err
	}
	return dhtUploadables, nil
}

// PutSyncAuthor adds or updates a SyncAuthor
func PutSyncAuthor(db *gorm.DB, publicKey string) error {
	var syncAuthor SyncAuthor
	result := db.Where("public_key = ?", publicKey).First(&syncAuthor)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			// Create new record
			syncAuthor = SyncAuthor{PublicKey: publicKey}
			return db.Create(&syncAuthor).Error
		}
		return result.Error
	}
	// Record exists, no update needed as we only have PublicKey
	return nil
}

// PutSyncRelay adds or updates a SyncRelay
func PutSyncRelay(db *gorm.DB, publicKey string, relayInfo interface{}) error {
	var syncRelay SyncRelay
	result := db.Where("public_key = ?", publicKey).First(&syncRelay)

	relayInfoJSON, err := json.Marshal(relayInfo)
	if err != nil {
		return err
	}

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			// Create new record
			syncRelay = SyncRelay{
				PublicKey: publicKey,
				RelayInfo: string(relayInfoJSON),
			}
			return db.Create(&syncRelay).Error
		}
		return result.Error
	}
	// Update existing record
	syncRelay.RelayInfo = string(relayInfoJSON)
	return db.Save(&syncRelay).Error
}

// PutDHTUploadable adds or updates a DHTUploadable
func PutDHTUploadable(db *gorm.DB, payload []byte, pubkey []byte, signature []byte) error {
	var dhtUploadable DHTUploadable
	result := db.Where("pubkey = ?", pubkey).First(&dhtUploadable)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			// Create new record
			dhtUploadable = DHTUploadable{
				Payload:   payload,
				Pubkey:    pubkey,
				Signature: signature,
			}
			return db.Create(&dhtUploadable).Error
		}
		return result.Error
	}
	// Update existing record
	dhtUploadable.Payload = payload
	dhtUploadable.Signature = signature
	return db.Save(&dhtUploadable).Error
}

// Helper function to initialize database connection
func InitSyncDB() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open("statistics.db"), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to immudb: %v", err)
	}

	// Auto Migrate the schema
	err = db.AutoMigrate(&SyncAuthor{}, &SyncRelay{}, &DHTUploadable{})
	if err != nil {
		return nil, err
	}

	return db, nil
}
