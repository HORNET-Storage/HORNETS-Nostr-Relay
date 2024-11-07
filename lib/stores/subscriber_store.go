package stores

import (
	"github.com/HORNET-Storage/hornet-storage/lib"
)

// SubscriberStore defines the interface for managing address-related functions
type SubscriberStore interface {
	// Store initialization
	InitStore(basepath string, args ...interface{}) error

	// Address management
	AllocateBitcoinAddress(npub string) (*lib.Address, error)
	AddressExists(address string) (bool, error)
	SaveSubscriberAddress(address *lib.SubscriberAddress) error
}
