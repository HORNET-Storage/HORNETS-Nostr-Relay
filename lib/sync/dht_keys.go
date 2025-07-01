package sync

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

// DeriveKeyFromNsec derives a DHT key from a user's nsec (private key)
// This provides a consistent way to generate DHT keys from private keys
// using the same approach as GenerateDHTKey for consistency
func DeriveKeyFromNsec(nsec string) (string, error) {
	// Extract private key bytes using the signing package
	privateKeyBytes, err := signing.DecodeKey(nsec)
	if err != nil {
		return "", fmt.Errorf("invalid nsec: %w", err)
	}

	// Ensure we have the correct length
	if len(privateKeyBytes) != 32 {
		return "", fmt.Errorf("invalid private key length: expected 32 bytes, got %d", len(privateKeyBytes))
	}

	// Create a copy for clamping
	clampedPrivateKey := make([]byte, len(privateKeyBytes))
	copy(clampedPrivateKey, privateKeyBytes)

	// Apply clamping as per Ed25519 specification
	clampedPrivateKey[0] &= 248  // Clear the lowest 3 bits
	clampedPrivateKey[31] &= 127 // Clear the highest bit
	clampedPrivateKey[31] |= 64  // Set the second highest bit

	// Calculate hash using SHA1
	hash := sha1.Sum(clampedPrivateKey[:32])

	// Return the hash as a hexadecimal string
	return hex.EncodeToString(hash[:]), nil
}

// GetDHTKeyForPubkey derives a DHT key for a given public key
// This is useful when you only have the public key and need to find
// the corresponding DHT key to retrieve data
func GetDHTKeyForPubkey(pubkey string) (string, error) {
	// Decode the public key from hex
	pubkeyBytes, err := hex.DecodeString(pubkey)
	if err != nil {
		return "", fmt.Errorf("invalid pubkey hex: %w", err)
	}

	// For public keys, we use SHA1 directly
	hash := sha1.Sum(pubkeyBytes)

	// Return the hash as a hexadecimal string
	return hex.EncodeToString(hash[:]), nil
}

// SignRelayList signs a relay list with the user's private key
// The signature can be verified to ensure the relay list was created by the owner
func SignRelayList(relayList []string, nsec string) (string, error) {
	// Use existing DeserializePrivateKey
	privateKey, _, err := signing.DeserializePrivateKey(nsec)
	if err != nil {
		return "", fmt.Errorf("invalid nsec: %w", err)
	}

	// Create a canonical JSON representation of the relay list
	relayListBytes, err := encodeRelayList(relayList)
	if err != nil {
		return "", fmt.Errorf("failed to encode relay list: %w", err)
	}

	// Use existing SignData function
	signature, err := signing.SignData(relayListBytes, privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign relay list: %w", err)
	}

	// Return the signature as a hexadecimal string
	return hex.EncodeToString(signature.Serialize()), nil
}

// VerifyRelayList verifies the signature on a relay list
// Returns true if the signature is valid, false otherwise
func VerifyRelayList(relayList []string, signature string, pubkey string) (bool, error) {
	// Decode the signature from hex
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return false, fmt.Errorf("invalid signature hex: %w", err)
	}

	// Parse the signature
	sig, err := schnorr.ParseSignature(sigBytes)
	if err != nil {
		return false, fmt.Errorf("failed to parse signature: %w", err)
	}

	// Deserialize the public key
	publicKey, err := signing.DeserializePublicKey(pubkey)
	if err != nil {
		return false, fmt.Errorf("invalid pubkey: %w", err)
	}

	// Encode the relay list
	relayListBytes, err := encodeRelayList(relayList)
	if err != nil {
		return false, fmt.Errorf("failed to encode relay list: %w", err)
	}

	// Verify the signature
	err = signing.VerifySignature(sig, relayListBytes, publicKey)
	return err == nil, nil
}

// encodeRelayList encodes a relay list as a byte slice for signing
// This ensures consistent encoding for signature verification
func encodeRelayList(relayList []string) ([]byte, error) {
	// Convert the relay list to JSON for consistent encoding
	jsonBytes, err := json.Marshal(relayList)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal relay list: %w", err)
	}

	// Hash the JSON bytes to get a fixed-size hash (32 bytes for SHA256)
	hash := sha256.Sum256(jsonBytes)

	// Return the hash as a byte slice
	return hash[:], nil
}

// RelayList represents a user's list of preferred relays
// This structure is used for storing and retrieving relay lists from the DHT
type RelayList struct {
	Pubkey    string   `json:"pubkey"`
	Relays    []string `json:"relays"`
	CreatedAt int64    `json:"created_at"`
	Signature string   `json:"signature,omitempty"`
}

// StoreRelayList stores a signed relay list in the DHT
// This extends the existing RelayStore to support relay list storage
func (rs *RelayStore) StoreRelayList(dhtKey string, relayList []string, pubkey string, signature string) error {
	// Convert the relay list to a canonical format
	relayListBytes, err := encodeRelayList(relayList)
	if err != nil {
		return fmt.Errorf("failed to encode relay list: %w", err)
	}

	// Add the uploadable to the relay store directly
	return rs.AddUploadable(hex.EncodeToString(relayListBytes), pubkey, signature, true)
}

// RetrieveRelayList retrieves a relay list from the DHT
// Returns the relay list and any error that occurred
func (rs *RelayStore) RetrieveRelayList(dhtKey string) ([]string, error) {
	// Use the existing GetRelayListFromDHT method
	relayInfos, err := rs.GetRelayListFromDHT(&dhtKey)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve relay list: %w", err)
	}

	// Extract URLs from relay infos
	var relayURLs []string
	for _, info := range relayInfos {
		// Use the correct field from NIP11RelayInfo
		relayURLs = append(relayURLs, info.Software)
	}

	return relayURLs, nil
}

// CreateDHTKeyFromPrivateKey creates a DHT key from a private key
// This is a convenience function for creating a DHT key from a btcec.PrivateKey
// using the same approach as GenerateDHTKey for consistency
func CreateDHTKeyFromPrivateKey(privateKey *btcec.PrivateKey) (string, error) {
	// Serialize the private key
	privateKeyBytes := privateKey.Serialize()

	// Ensure we have the correct length
	if len(privateKeyBytes) != 32 {
		return "", fmt.Errorf("invalid private key length: expected 32 bytes, got %d", len(privateKeyBytes))
	}

	// Create a copy for clamping
	clampedPrivateKey := make([]byte, len(privateKeyBytes))
	copy(clampedPrivateKey, privateKeyBytes)

	// Apply clamping as per Ed25519 specification
	clampedPrivateKey[0] &= 248  // Clear the lowest 3 bits
	clampedPrivateKey[31] &= 127 // Clear the highest bit
	clampedPrivateKey[31] |= 64  // Set the second highest bit

	// Calculate hash using SHA1
	hash := sha1.Sum(clampedPrivateKey[:32])

	// Return the hash as a hexadecimal string
	return hex.EncodeToString(hash[:]), nil
}

// CreateDHTKeyFromPublicKey creates a DHT key from a public key
// This is a convenience function for creating a DHT key from a btcec.PublicKey
func CreateDHTKeyFromPublicKey(publicKey *btcec.PublicKey) (string, error) {
	// Serialize the public key
	publicKeyBytes := schnorr.SerializePubKey(publicKey)

	// For public keys, we use SHA1 directly
	hash := sha1.Sum(publicKeyBytes)

	// Return the hash as a hexadecimal string
	return hex.EncodeToString(hash[:]), nil
}

// CreateMutableTargetFromHex creates a mutable target from a hex string
// This is a convenience function for creating a target from a hex string
func CreateMutableTargetFromHex(hexString string) (krpc.ID, error) {
	// Decode the hex string
	bytes, err := hex.DecodeString(hexString)
	if err != nil {
		return krpc.ID{}, fmt.Errorf("invalid hex string: %w", err)
	}

	// Use SHA1 for target generation (consistent with existing code)
	emptySalt := []byte{}
	target := CreateMutableTarget(bytes, emptySalt)

	return target, nil
}

// GenerateDHTKey generates a DHT key from a private key hex string
// This is the implementation that was previously in services/server/port/main.go
func GenerateDHTKey(privateKeyHex string) (string, error) {
	// Convert hex string to bytes
	privateKeyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return "", fmt.Errorf("failed to decode private key hex: %v", err)
	}

	// Ensure we have the correct length
	if len(privateKeyBytes) != 32 {
		return "", fmt.Errorf("invalid private key length: expected 32 bytes, got %d", len(privateKeyBytes))
	}

	// Create a copy for clamping
	clampedPrivateKey := make([]byte, len(privateKeyBytes))
	copy(clampedPrivateKey, privateKeyBytes)

	// Apply clamping as per Ed25519 specification
	clampedPrivateKey[0] &= 248  // Clear the lowest 3 bits
	clampedPrivateKey[31] &= 127 // Clear the highest bit
	clampedPrivateKey[31] |= 64  // Set the second highest bit

	// Calculate hash using SHA1 (consistent with existing code)
	hash := sha1.Sum(clampedPrivateKey[:32])

	// Use the full SHA1 hash (20 bytes)
	scalar := hash[:]

	// For DHT key, we'll use the hex encoding of the scalar
	// This matches the behavior of the TypeScript implementation
	dhtKey := hex.EncodeToString(scalar)

	return dhtKey, nil
}
