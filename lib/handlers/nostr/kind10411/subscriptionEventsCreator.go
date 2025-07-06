package kind10411

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"
)

// Address status constants
const (
	AddressStatusAvailable = "available"
	AddressStatusAllocated = "allocated"
	AddressStatusUsed      = "used"
	envFile                = ".env"
	nostrPrivateKeyVar     = "NOSTR_PRIVATE_KEY"
)

type RelayInfo struct {
	Name              string                 `json:"name"`
	Description       string                 `json:"description,omitempty"`
	Pubkey            string                 `json:"pubkey"`
	Contact           string                 `json:"contact"`
	Icon              string                 `json:"icon,omitempty"`
	SupportedNIPs     []int                  `json:"supported_nips"`
	Software          string                 `json:"software"`
	Version           string                 `json:"version"`
	DHTkey            string                 `json:"dhtkey,omitempty"`
	SubscriptionTiers []SubscriptionTierInfo `json:"subscription_tiers,omitempty"`
}

type SubscriptionTierInfo struct {
	DataLimit string `json:"datalimit"`
	Price     string `json:"price"`
}

func formatDataLimit(bytes int64, unlimited bool) string {
	if unlimited {
		return "Unlimited"
	}

	const (
		GB = 1024 * 1024 * 1024
		MB = 1024 * 1024
	)

	if bytes >= GB {
		return fmt.Sprintf("%d GB per month", bytes/GB)
	}
	return fmt.Sprintf("%d MB per month", bytes/MB)
}

func CreateKind10411Event(privateKey *secp256k1.PrivateKey, publicKey *secp256k1.PublicKey, store stores.Store) error {
	// Get subscription tiers from allowed_users.tiers
	var allTiers []types.SubscriptionTier
	if err := viper.UnmarshalKey("allowed_users.tiers", &allTiers); err != nil {
		return fmt.Errorf("error getting subscription tiers from allowed_users.tiers: %v", err)
	}

	// Transform to relay info format, excluding free tier
	var tiers []types.SubscriptionTier
	for _, tier := range allTiers {
		// Skip free tier (price = 0)
		if tier.PriceSats <= 0 {
			continue
		}

		// Convert bytes to human-readable format for the datalimit field
		datalimit := formatDataLimit(tier.MonthlyLimitBytes, tier.Unlimited)

		// Create a custom tier structure that matches the expected format
		tiers = append(tiers, types.SubscriptionTier{
			Name:              datalimit,
			PriceSats:         tier.PriceSats,
			MonthlyLimitBytes: tier.MonthlyLimitBytes,
			Unlimited:         tier.Unlimited,
		})
	}

	logging.Infof("Paid Tiers for kind 10411: %+v", tiers)

	// Delete existing kind 10411 events
	filter := nostr.Filter{
		Kinds: []int{10411},
	}
	existingEvents, err := store.QueryEvents(filter)
	if err != nil {
		return fmt.Errorf("error querying existing kind 10411 events: %v", err)
	}

	for _, oldEvent := range existingEvents {
		if err := store.DeleteEvent(oldEvent.ID); err != nil {
			return fmt.Errorf("error deleting old kind 10411 event %s: %v", oldEvent.ID, err)
		}
		logging.Infof("Deleted existing kind 10411 event with ID: %s", oldEvent.ID)
	}

	// Convert tiers to the expected format
	var tierInfos []SubscriptionTierInfo
	for _, tier := range tiers {
		tierInfos = append(tierInfos, SubscriptionTierInfo{
			DataLimit: tier.Name, // We stored the formatted data limit in Name
			Price:     fmt.Sprintf("%d", tier.PriceSats),
		})
	}

	// Get relay info
	relayInfo := RelayInfo{
		Name:              viper.GetString("relay.name"),
		Description:       viper.GetString("relay.description"),
		Pubkey:            viper.GetString("relay.public_key"),
		Contact:           viper.GetString("relay.contact"),
		Icon:              viper.GetString("relay.icon"),
		SupportedNIPs:     viper.GetIntSlice("relay.supported_nips"),
		Software:          viper.GetString("relay.software"),
		Version:           viper.GetString("relay.version"),
		DHTkey:            viper.GetString("relay.dht_key"),
		SubscriptionTiers: tierInfos,
	}

	// Convert relay info to JSON
	content, err := json.Marshal(relayInfo)
	if err != nil {
		return fmt.Errorf("error marshaling relay info: %v", err)
	}

	// Create the event
	event, err := createAnyEvent(privateKey, publicKey, 10411, string(content), []nostr.Tag{})
	if err != nil {
		return fmt.Errorf("error creating kind 10411 event: %v", err)
	}

	// Store the new event
	if err := store.StoreEvent(event); err != nil {
		return fmt.Errorf("error storing kind 10411 event: %v", err)
	}

	// Print the event for verification
	eventJSON, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		logging.Infof("Error marshaling event for printing: %v", err)
	} else {
		logging.Infof("Created and stored kind 10411 event:\n%s", string(eventJSON))
	}

	logging.Info("Kind 10411 event created and stored successfully")
	return nil
}

func createAnyEvent(privateKey *secp256k1.PrivateKey, publicKey *secp256k1.PublicKey, kind int, content string, tags []nostr.Tag) (*nostr.Event, error) {
	stringKey := hex.EncodeToString(schnorr.SerializePubKey(publicKey))
	logging.Infof("Public Key: %s", stringKey)

	event := &nostr.Event{
		PubKey:    stringKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      kind,
		Tags:      tags,
		Content:   content,
	}

	serializedEvent := serializeEventForID(event)
	hash := sha256.Sum256([]byte(serializedEvent))
	eventID := hex.EncodeToString(hash[:])
	event.ID = eventID

	signature, err := schnorr.Sign(privateKey, hash[:])
	if err != nil {
		return nil, err
	}

	sigHex := hex.EncodeToString(signature.Serialize())
	event.Sig = sigHex

	err = signing.VerifySignature(signature, hash[:], publicKey)
	if err != nil {
		logging.Infof("error verifying signature, %s", err)
		return nil, fmt.Errorf("error verifying signature, %s", err)
	} else {
		logging.Info("Signature is valid.")
	}

	return event, nil
}

func serializeEventForID(event *nostr.Event) string {
	// Assuming the Tags and other fields are already correctly filled except ID and Sig
	serialized, err := json.Marshal([]interface{}{
		0,
		event.PubKey,
		event.CreatedAt,
		event.Kind,
		event.Tags,
		event.Content,
	})
	if err != nil {
		panic(err) // Handle error properly in real code
	}

	// Convert the JSON array to a string
	compactSerialized := string(serialized)

	return compactSerialized
}

func CreateNIP88Event(relayPrivKey *btcec.PrivateKey, userPubKey string, store stores.Store) (*nostr.Event, error) {

	// Allocate a new address for this subscription
	addr, err := store.AllocateAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate address: %v", err)
	}

	tags := []nostr.Tag{
		{"subscription-duration", "1 month"},
		{"npub", userPubKey},
		{"relay-bitcoin-address", addr.Address},
		// Add Lightning invoice if applicable
		{"relay-dht-key", viper.GetString("relay.dht_key")},
	}

	event := &nostr.Event{
		PubKey:    hex.EncodeToString(relayPrivKey.PubKey().SerializeCompressed()),
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      764,
		Tags:      tags,
		Content:   "",
	}

	hash := sha256.Sum256(event.Serialize())
	sig, err := schnorr.Sign(relayPrivKey, hash[:])
	if err != nil {
		return nil, fmt.Errorf("error signing event: %v", err)
	}
	event.ID = hex.EncodeToString(hash[:])
	event.Sig = hex.EncodeToString(sig.Serialize())

	return event, nil
}
