package signing

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

func DeserializePrivateKey(serializedKey string) (*btcec.PrivateKey, *btcec.PublicKey, error) {
	trimmedKey := strings.TrimPrefix(serializedKey, "nsec")

	keyBytes, err := hex.DecodeString(trimmedKey)
	if err != nil {
		return nil, nil, err
	}

	privateKey, publicKey := btcec.PrivKeyFromBytes(keyBytes)

	return privateKey, publicKey, nil
}

func DeserializePublicKey(serializedKey string) (*btcec.PublicKey, error) {
	trimmedKey := strings.TrimPrefix(serializedKey, "npub")

	keyBytes, err := hex.DecodeString(trimmedKey)
	if err != nil {
		return nil, err
	}

	publicKey, err := btcec.ParsePubKey(keyBytes)
	if err != nil {
		return nil, err
	}

	return publicKey, nil
}

func SerializePrivateKey(key *btcec.PrivateKey) string {
	keyBytes := key.Serialize()

	encodedKey := hex.EncodeToString(keyBytes)

	return fmt.Sprintf("nsec:%s", encodedKey)
}

func SerializePublicKey(key *btcec.PublicKey) string {
	keyBytes := key.SerializeCompressed()

	encodedKey := hex.EncodeToString(keyBytes)

	return fmt.Sprintf("npub:%s", encodedKey)
}

func TrimPrivateKey(privateKey string) string {
	return strings.Trim(privateKey, "nsec")
}

func TrimPublicKey(publicKey string) string {
	return strings.Trim(publicKey, "npub")
}

func SignData(data []byte, privateKey *btcec.PrivateKey) (*schnorr.Signature, error) {
	signature, err := schnorr.Sign(privateKey, data)
	if err != nil {
		return nil, err
	}

	return signature, nil
}

func VerifySignature(signature *schnorr.Signature, data []byte, publicKey *btcec.PublicKey) error {
	result := signature.Verify(data, publicKey)
	if !result {
		return fmt.Errorf("data failed to verify")
	}

	return nil
}
