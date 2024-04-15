package proxy

import (
	"encoding/hex"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
)

func DeserializePrivateKey(serializedKey string) (*btcec.PrivateKey, error) {
	// Remove the "nsec" prefix if it exists
	trimmedKey := strings.TrimPrefix(serializedKey, "nsec")

	// Decode the hex-encoded private key
	keyBytes, err := hex.DecodeString(trimmedKey)
	if err != nil {
		return nil, err
	}

	// Convert bytes to a private key
	privateKey, _ := btcec.PrivKeyFromBytes(keyBytes)

	return privateKey, nil
}
