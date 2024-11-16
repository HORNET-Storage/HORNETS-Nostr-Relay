package subscription_gorm

import (
	"fmt"
	"log"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// GormSubscriberStore provides a GORM-based implementation of the SubscriberStore interface
type GormSubscriberStore struct {
	DB *gorm.DB
}

const (
	AddressStatusAvailable = "available"
	AddressStatusAllocated = "allocated"
)

// InitStore initializes the GORM subscriber store
func (store *GormSubscriberStore) InitStore(basepath string, args ...interface{}) error {
	var err error

	// Initialize the database connection
	store.DB, err = gorm.Open(sqlite.Open(basepath), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}

	// Run migrations
	err = store.DB.AutoMigrate(
		&types.SubscriberAddress{},
	)
	if err != nil {
		return fmt.Errorf("failed to run migrations: %v", err)
	}

	return nil
}

// NewGormSubscriberStore creates a new instance of GormSubscriberStore
func NewGormSubscriberStore() *GormSubscriberStore {
	return &GormSubscriberStore{}
}

func (store *GormSubscriberStore) AllocateBitcoinAddress(npub string) (*types.Address, error) {
	tx := store.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Step 1: Check if the npub already has an allocated address
	var existingAddressRecord types.SubscriberAddress
	err := tx.Where("npub = ?", npub).First(&existingAddressRecord).Error
	if err == nil {
		// If an existing record is found, return it
		return &types.Address{
			Index:       existingAddressRecord.Index,
			Address:     existingAddressRecord.Address,
			WalletName:  existingAddressRecord.WalletName,
			Status:      existingAddressRecord.Status,
			AllocatedAt: existingAddressRecord.AllocatedAt,
			Npub:        npub,
		}, nil
	} else if err != gorm.ErrRecordNotFound {
		// If another error occurred (not record not found), rollback and return error
		tx.Rollback()
		return nil, fmt.Errorf("failed to query existing address for npub: %v", err)
	}

	// Step 2: Allocate a new address if no existing address is found
	var addressRecord types.SubscriberAddress
	err = tx.Where("status = ? AND (npub IS NULL OR npub = '')", AddressStatusAvailable).
		Order("id").
		First(&addressRecord).Error

	if err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("no available addresses")
		}
		return nil, fmt.Errorf("failed to query available addresses: %v", err)
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":       AddressStatusAllocated,
		"allocated_at": &now,
		"npub":         npub,
	}

	if err := tx.Model(&addressRecord).Updates(updates).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update address allocation: %v", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &types.Address{
		Index:       addressRecord.Index,
		Address:     addressRecord.Address,
		WalletName:  addressRecord.WalletName,
		Status:      addressRecord.Status,
		AllocatedAt: addressRecord.AllocatedAt,
		Npub:        npub,
	}, nil
}

// AddressExists checks if a given Bitcoin address exists in the database
func (store *GormSubscriberStore) AddressExists(address string) (bool, error) {
	var count int64
	err := store.DB.Model(&types.SubscriberAddress{}).
		Where("address = ?", address).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("failed to check address existence: %v", err)
	}

	return count > 0, nil
}

// SaveSubscriberAddress saves or updates a subscriber address in the database
func (store *GormSubscriberStore) SaveSubscriberAddress(address *types.SubscriberAddress) error {
	// Directly create a new address record
	if err := store.DB.Create(address).Error; err != nil {
		log.Printf("Error saving new address: %v", err)
		return err
	}

	log.Printf("Address %s saved successfully.", address.Address)
	return nil
}

// CountAvailableAddresses counts the number of available addresses in the database
func (store *GormSubscriberStore) CountAvailableAddresses() (int64, error) {
	var count int64
	err := store.DB.Model(&types.SubscriberAddress{}).
		Where("status = ?", AddressStatusAvailable).
		Count(&count).Error

	if err != nil {
		return 0, fmt.Errorf("failed to count available addresses: %v", err)
	}

	return count, nil
}

// GetSubscriberByAddress retrieves subscriber information by Bitcoin address
func (store *GormSubscriberStore) GetSubscriberByAddress(address string) (*types.SubscriberAddress, error) {
	var subscriber types.SubscriberAddress

	err := store.DB.Where("address = ?", address).First(&subscriber).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("no subscriber found for address: %s", address)
		}
		return nil, fmt.Errorf("failed to query subscriber: %v", err)
	}

	return &subscriber, nil
}
