package test

import (
	"encoding/hex"
	"testing"

	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/sync"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/stretchr/testify/assert"
)

func TestDHTKeyDerivation(t *testing.T) {
	// Generate a test private key
	privateKey, err := btcec.NewPrivateKey()
	assert.NoError(t, err)

	// Serialize the private key
	privateKeyHex := hex.EncodeToString(privateKey.Serialize())

	// Generate a DHT key using the GenerateDHTKey function
	dhtKey1, err := sync.GenerateDHTKey(privateKeyHex)
	assert.NoError(t, err)
	assert.NotEmpty(t, dhtKey1)

	// Generate a DHT key using the CreateDHTKeyFromPrivateKey function
	dhtKey2, err := sync.CreateDHTKeyFromPrivateKey(privateKey)
	assert.NoError(t, err)
	assert.NotEmpty(t, dhtKey2)

	// The two keys should be the same
	assert.Equal(t, dhtKey1, dhtKey2)

	// Test with a known private key
	knownPrivateKeyHex := "c9c8482ac7268391fb14e0cc2a6b475b7ffb50ae62ffa5f59deee350d1a94e5a"
	dhtKey3, err := sync.GenerateDHTKey(knownPrivateKeyHex)
	assert.NoError(t, err)
	assert.NotEmpty(t, dhtKey3)
}

func TestDHTKeyFromNsec(t *testing.T) {
	// Generate a test private key
	privateKey, err := btcec.NewPrivateKey()
	assert.NoError(t, err)

	// Serialize the private key to nsec format
	nsecPtr, err := signing.SerializePrivateKey(privateKey)
	assert.NoError(t, err)
	nsec := *nsecPtr

	// Generate a DHT key using the DeriveKeyFromNsec function
	dhtKey1, err := sync.DeriveKeyFromNsec(nsec)
	assert.NoError(t, err)
	assert.NotEmpty(t, dhtKey1)

	// Generate a DHT key using the CreateDHTKeyFromPrivateKey function
	dhtKey2, err := sync.CreateDHTKeyFromPrivateKey(privateKey)
	assert.NoError(t, err)
	assert.NotEmpty(t, dhtKey2)

	// The two keys should be the same
	assert.Equal(t, dhtKey1, dhtKey2)
}

func TestRelayListSigningAndVerification(t *testing.T) {
	// Generate a test private key
	privateKey, err := btcec.NewPrivateKey()
	assert.NoError(t, err)

	// Serialize the private key to nsec format
	nsecPtr, err := signing.SerializePrivateKey(privateKey)
	assert.NoError(t, err)
	nsec := *nsecPtr

	// Get the public key
	publicKey := privateKey.PubKey()
	// Use schnorr.SerializePubKey to get a 32-byte public key
	pubkeyHex := hex.EncodeToString(schnorr.SerializePubKey(publicKey))

	// Create a test relay list
	relayList := []string{
		"wss://relay1.example.com",
		"wss://relay2.example.com",
		"wss://relay3.example.com",
	}

	// Sign the relay list
	signature, err := sync.SignRelayList(relayList, nsec)
	assert.NoError(t, err)
	assert.NotEmpty(t, signature)

	// Verify the signature
	valid, err := sync.VerifyRelayList(relayList, signature, pubkeyHex)
	assert.NoError(t, err)
	assert.True(t, valid)

	// Modify the relay list and verify that the signature is invalid
	modifiedRelayList := append(relayList, "wss://relay4.example.com")
	valid, err = sync.VerifyRelayList(modifiedRelayList, signature, pubkeyHex)
	assert.NoError(t, err)
	assert.False(t, valid)
}

func TestGetDHTKeyForPubkey(t *testing.T) {
	// Generate a test private key
	privateKey, err := btcec.NewPrivateKey()
	assert.NoError(t, err)

	// Get the public key
	publicKey := privateKey.PubKey()
	// Use schnorr.SerializePubKey to get a 32-byte public key
	pubkeyHex := hex.EncodeToString(schnorr.SerializePubKey(publicKey))

	// Generate a DHT key using the GetDHTKeyForPubkey function
	dhtKey1, err := sync.GetDHTKeyForPubkey(pubkeyHex)
	assert.NoError(t, err)
	assert.NotEmpty(t, dhtKey1)

	// Generate a DHT key using the CreateDHTKeyFromPublicKey function
	dhtKey2, err := sync.CreateDHTKeyFromPublicKey(publicKey)
	assert.NoError(t, err)
	assert.NotEmpty(t, dhtKey2)

	// The two keys should be the same
	assert.Equal(t, dhtKey1, dhtKey2)
}
