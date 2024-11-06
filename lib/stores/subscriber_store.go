package stores

import (
	"fmt"

	types "github.com/HORNET-Storage/hornet-storage/lib"
)

// SubscriberStore defines the interface for managing subscribers and their storage usage
type SubscriberStore interface {
	// Store initialization
	InitStore(basepath string, args ...interface{}) error

	// Subscriber management
	SaveSubscriber(subscriber *types.Subscriber) error
	GetSubscriber(npub string) (*types.Subscriber, error)
	GetSubscriberByAddress(address string) (*types.Subscriber, error)
	DeleteSubscriber(npub string) error
	ListSubscribers() ([]*types.Subscriber, error)

	// Storage quota and statistics
	GetSubscriberStorageStats(npub string) (*types.StorageStats, error)
	UpdateStorageUsage(npub string, sizeBytes int64) error
	GetStorageUsage(npub string) (*types.StorageUsage, error)
	CheckStorageAvailability(npub string, requestedBytes int64) error

	// Subscription periods
	AddSubscriptionPeriod(npub string, period *types.SubscriptionPeriod) error
	GetSubscriptionPeriods(npub string) ([]*types.SubscriptionPeriod, error)
	GetActiveSubscription(npub string) (*types.SubscriptionPeriod, error)
	GetSubscriptionByTransactionID(transactionID string) (*types.SubscriptionPeriod, error)

	// File management
	TrackFileUpload(upload *types.FileUpload) error
	DeleteFile(npub string, fileHash string) error
	GetFilesBySubscriber(npub string) ([]*types.FileUpload, error)
	GetRecentUploads(npub string, limit int) ([]*types.FileUpload, error)

	// Address management
	SaveSubscriberAddress(address *types.SubscriberAddress) error
	AllocateBitcoinAddress(npub string) (*types.Address, error)
	AddressExists(address string) (bool, error)
	SaveSubscriberAddresses(address *types.WalletAddress) error
}

// Convert storage strings to bytes
func ParseStorageLimit(limit string) (int64, error) {
	var bytes int64
	switch limit {
	case "1 GB per month":
		bytes = 1 * 1024 * 1024 * 1024
	case "5 GB per month":
		bytes = 5 * 1024 * 1024 * 1024
	case "10 GB per month":
		bytes = 10 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unknown storage limit: %s", limit)
	}
	return bytes, nil
}
