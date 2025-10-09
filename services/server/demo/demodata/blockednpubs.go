package demodata

import (
	"fmt"

	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/badgerhold"
)

// GeneratePaymentNotifications creates payment notifications
func (g *DemoDataGenerator) GenerateBlockedNpubs(store *badgerhold.BadgerholdStore, count int) error {
	logging.Infof("Generating %d blocked public keys...\n", count)

	for i := 0; i < count; i++ {
		key, err := signing.GeneratePrivateKey()
		if err != nil {
			continue
		}

		pubKey, err := signing.SerializePublicKey(key.PubKey())
		if err != nil {
			continue
		}

		if err := store.BlockPubkey(*pubKey, "Automatically blocked by content moderation system"); err != nil {
			return fmt.Errorf("error blocking pubkey %d: %v", i, err)
		}
	}

	logging.Infof("Blocked npub generation complete!")
	return nil
}
