package kind19842

import (
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
)

// IsPaidSubscriber checks if a user is a paid subscriber
func IsPaidSubscriber(store stores.Store, pubkey string) (bool, error) {
	// Get the statistics store
	statsStore := store.GetStatsStore()

	// Check if the user has an active paid subscription
	subscriber, err := statsStore.GetPaidSubscriberByNpub(pubkey)
	if err != nil {
		return false, err
	}

	// If no subscriber record found, they're not a paid subscriber
	if subscriber == nil {
		return false, nil
	}

	// Check if the subscription is still active (not expired)
	return subscriber.ExpirationDate.After(time.Now()), nil
}
