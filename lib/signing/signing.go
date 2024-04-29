package signing

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/ipfs/go-cid"
)

func DecodeKey(serializedKey string) ([]byte, error) {
	fmt.Println(TrimPrivateKey(TrimPublicKey(serializedKey)))

	bytes, err := hex.DecodeString(TrimPrivateKey(TrimPublicKey(serializedKey)))
	if err != nil {
		return nil, err
	}

	return bytes, nil
}

func DeserializePrivateKey(serializedKey string) (*secp256k1.PrivateKey, *secp256k1.PublicKey, error) {
	privateKeyBytes, err := DecodeKey(serializedKey)
	if err != nil {
		return nil, nil, err
	}

	privateKey, publicKey := btcec.PrivKeyFromBytes(privateKeyBytes)

	return privateKey, publicKey, nil
}

func DeserializePublicKey(serializedKey string) (*secp256k1.PublicKey, error) {
	publicKeyBytes, err := DecodeKey(serializedKey)
	if err != nil {
		return nil, err
	}

	publicKey, err := schnorr.ParsePubKey(publicKeyBytes)
	if err != nil {
		return nil, err
	}

	return publicKey, nil
}

func TrimPrivateKey(privateKey string) string {
	return strings.TrimPrefix(privateKey, "nsec1")
}

func TrimPublicKey(publicKey string) string {
	return strings.TrimPrefix(publicKey, "npub1")
}

func SignData(data []byte, privateKey *btcec.PrivateKey) (*schnorr.Signature, error) {
	signature, err := schnorr.Sign(privateKey, data)
	if err != nil {
		return nil, err
	}

	return signature, nil
}

func SignCID(cid cid.Cid, privateKey *btcec.PrivateKey) (*schnorr.Signature, error) {
	hashed := sha256.Sum256(cid.Bytes())

	signature, err := SignData(hashed[:], privateKey)
	if err != nil {
		return nil, err
	}

	return signature, nil
}

func VerifySignature(signature *schnorr.Signature, data []byte, publicKey *secp256k1.PublicKey) error {
	result := signature.Verify(data, publicKey)
	if !result {
		return fmt.Errorf("data failed to verify")
	}

	return nil
}

func VerifyCIDSignature(signature *schnorr.Signature, cid cid.Cid, publicKey *secp256k1.PublicKey) error {
	hashed := sha256.Sum256(cid.Bytes())

	err := VerifySignature(signature, hashed[:], publicKey)
	if err != nil {
		return err
	}

	return nil
}

func GeneratePrivateKey() (*secp256k1.PrivateKey, error) {
	privateKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, err
	}

	return privateKey, nil
}

func SerializePrivateKey(privateKey *secp256k1.PrivateKey) (*string, error) {
	privateKeyBytes := privateKey.Serialize()

	encodedKey := hex.EncodeToString(privateKeyBytes)

	return &encodedKey, nil
}

func SerializePublicKey(publicKey *secp256k1.PublicKey) (*string, error) {
	publicKeyBytes := schnorr.SerializePubKey(publicKey)

	encodedKey := hex.EncodeToString(publicKeyBytes)

	return &encodedKey, nil
}
