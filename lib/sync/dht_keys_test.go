package sync

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
)

const relayPrivateKeyHex = "c600149fe1207dd0cf5284d0a4bd767dc192181940d2a2b08f9571445f308a02"

func TestDeriveDHTIdentityFromPrivateKeyIsDeterministic(t *testing.T) {
	first, err := DeriveDHTIdentityFromPrivateKey(relayPrivateKeyHex)
	if err != nil {
		t.Fatalf("expected first derivation to succeed: %v", err)
	}

	second, err := DeriveDHTIdentityFromPrivateKey(relayPrivateKeyHex)
	if err != nil {
		t.Fatalf("expected second derivation to succeed: %v", err)
	}

	if first.Seed != second.Seed {
		t.Fatalf("expected deterministic DHT seed, got %q and %q", first.Seed, second.Seed)
	}

	if first.PublicKey != second.PublicKey {
		t.Fatalf("expected deterministic DHT public key, got %q and %q", first.PublicKey, second.PublicKey)
	}

	if first.PrivateKey != second.PrivateKey {
		t.Fatalf("expected deterministic DHT private key, got %q and %q", first.PrivateKey, second.PrivateKey)
	}

	if len(first.Seed) != 64 {
		t.Fatalf("expected 32-byte DHT seed encoded as 64 hex chars, got %d", len(first.Seed))
	}

	if len(first.PublicKey) != 64 {
		t.Fatalf("expected 32-byte DHT public key encoded as 64 hex chars, got %d", len(first.PublicKey))
	}

	if len(first.PrivateKey) != 128 {
		t.Fatalf("expected 64-byte DHT private key encoded as 128 hex chars, got %d", len(first.PrivateKey))
	}
}

func TestDeriveDHTIdentityFromPrivateKeyMatchesEd25519SeedDerivation(t *testing.T) {
	identity, err := DeriveDHTIdentityFromPrivateKey(relayPrivateKeyHex)
	if err != nil {
		t.Fatalf("expected derivation to succeed: %v", err)
	}

	seed, err := DeriveKeyFromNsec(relayPrivateKeyHex)
	if err != nil {
		t.Fatalf("expected legacy seed derivation to succeed: %v", err)
	}

	if identity.Seed != seed {
		t.Fatalf("expected helper seed %q to match legacy derivation %q", identity.Seed, seed)
	}

	seedBytes, err := hex.DecodeString(identity.Seed)
	if err != nil {
		t.Fatalf("expected derived DHT seed to be valid hex: %v", err)
	}

	privateKey := ed25519.NewKeyFromSeed(seedBytes)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	if identity.PublicKey != hex.EncodeToString(publicKey) {
		t.Fatalf("expected public key %q to match ed25519 derivation %q", identity.PublicKey, hex.EncodeToString(publicKey))
	}

	if identity.PrivateKey != hex.EncodeToString(privateKey) {
		t.Fatalf("expected private key %q to match ed25519 derivation %q", identity.PrivateKey, hex.EncodeToString(privateKey))
	}
}
