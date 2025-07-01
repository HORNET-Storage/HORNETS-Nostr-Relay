package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"

	ws "github.com/HORNET-Storage/hornet-storage/lib/transports/websocket"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

func CheckSig(relay *ws.NIP11RelayInfo) error {
	packedBytes := ws.PackRelayForSig(relay)
	hash := sha256.Sum256(packedBytes)

	// Parse the public key
	pubKeyBytes, err := hex.DecodeString(relay.Pubkey)
	if err != nil {
		return errors.New("failed to decode public key to bytes")
	}
	pubKey, err := btcec.ParsePubKey(pubKeyBytes)
	if err != nil {
		return errors.New("failed to parse public key")
	}

	// Parse the signature
	sigBytes, err := hex.DecodeString(relay.HornetExtension.Signature)
	if err != nil {
		return errors.New("failed to decode public key")
	}
	sig, err := schnorr.ParseSignature(sigBytes)
	if err != nil {
		return errors.New("failed to parse signature")
	}

	// Verify the signature
	if !sig.Verify(hash[:], pubKey) {
		return errors.New("invalid signature")
	}

	return nil
}

func Equals(nr *ws.NIP11RelayInfo, other *ws.NIP11RelayInfo) bool {
	if nr == nil || other == nil {
		return nr == other
	}

	// Compare ID
	if nr.HornetExtension.LibP2PID != other.HornetExtension.LibP2PID {
		return false
	}

	if len(nr.HornetExtension.LibP2PAddrs) != len(other.HornetExtension.LibP2PAddrs) {
		return false
	}
	for i, addr := range nr.HornetExtension.LibP2PAddrs {
		if addr != other.HornetExtension.LibP2PAddrs[i] {
			return false
		}
	}

	// Compare Name
	if nr.Name != other.Name {
		return false
	}

	// Compare PublicKey
	if nr.Pubkey != other.Pubkey {
		return false
	}

	// Compare Signature
	if nr.HornetExtension.Signature != other.HornetExtension.Signature {
		return false
	}

	// Compare SupportedNIPs
	if len(nr.SupportedNIPs) != len(other.SupportedNIPs) {
		return false
	}
	for i, nip := range nr.SupportedNIPs {
		if nip != other.SupportedNIPs[i] {
			return false
		}
	}

	// All fields are equal
	return true
}
