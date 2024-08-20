package sync

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	ws "github.com/HORNET-Storage/hornet-storage/lib/transports/websocket"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/gofiber/fiber/v2/log"
	ma "github.com/multiformats/go-multiaddr"
	"sort"
	"time"
)

func CreateSelfRelay(id string, multiAddrs []ma.Multiaddr, name string, pubKey []byte, privKey *btcec.PrivateKey, supportedNIPs []int) (*ws.NIP11RelayInfo, error) {
	addrStrings := []string{}
	for _, multiAddr := range multiAddrs {
		addrStrings = append(addrStrings, multiAddr.String())
	}

	self := &ws.NIP11RelayInfo{
		Name:          name,
		Pubkey:        hex.EncodeToString(pubKey),
		SupportedNIPs: supportedNIPs,
		HornetExtension: &ws.HornetExtension{
			LibP2PID:    id,
			LibP2PAddrs: addrStrings,
			LastUpdated: time.Now().UTC(),
		},
	}

	err := SignRelay(self, privKey)
	if err != nil {
		return nil, err
	}
	return self, nil
}

func SignRelay(relay *ws.NIP11RelayInfo, privKey *btcec.PrivateKey) error {
	relayBytes := PackBytes(relay)
	hash := sha256.Sum256(relayBytes)

	signature, err := schnorr.Sign(privKey, hash[:])
	if err != nil {
		return err
	}

	relay.HornetExtension.Signature = hex.EncodeToString(signature.Serialize())
	return nil
}

func PackBytes(nr *ws.NIP11RelayInfo) []byte {
	var packed []byte

	// Pack ID
	packed = append(packed, []byte(nr.HornetExtension.LibP2PID)...)
	packed = append(packed, 0) // null terminator

	// Pack Name
	packed = append(packed, []byte(nr.Name)...)
	packed = append(packed, 0) // null terminator

	// Pack Addrs
	for _, addr := range nr.HornetExtension.LibP2PAddrs {
		packed = append(packed, []byte(addr)...)
		packed = append(packed, 0) // null terminator
	}
	packed = append(packed, 0) // double null terminator to indicate end of Addrs

	// Pack PublicKey
	pubkeyBytes, err := hex.DecodeString(nr.Pubkey)
	if err != nil {
		log.Warnf("Skipping packing invalid pubkey %s", nr.Pubkey)
	} else {
		packed = append(packed, pubkeyBytes...)
	}

	// Pack SupportedNIPs (sorted)
	sort.Ints(nr.SupportedNIPs)
	for _, nip := range nr.SupportedNIPs {
		nipBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(nipBytes, uint32(nip))
		packed = append(packed, nipBytes...)
	}

	return packed
}

func CheckSig(relay *ws.NIP11RelayInfo) error {
	packedBytes := PackBytes(relay)
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
