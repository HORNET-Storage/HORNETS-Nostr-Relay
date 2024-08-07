package sync

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	ma "github.com/multiformats/go-multiaddr"
	"sort"
)

type NostrRelay struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Addrs         []string `json:"addrs"`
	PublicKey     []byte   `json:"public_key"`
	Signature     []byte   `json:"signature"`
	SupportedNIPs []int    `json:"supported_nips"`
}

func CreateSelfRelay(id string, multiAddrs []ma.Multiaddr, name string, pubKey []byte, privKey *btcec.PrivateKey, supportedNIPs []int) (*NostrRelay, error) {
	addrStrings := []string{}
	for _, multiAddr := range multiAddrs {
		addrStrings = append(addrStrings, multiAddr.String())
	}

	self := &NostrRelay{
		ID:            id,
		Addrs:         addrStrings,
		Name:          name,
		PublicKey:     pubKey,
		SupportedNIPs: supportedNIPs,
	}

	err := self.SignRelay(privKey)
	if err != nil {
		return nil, err
	}
	return self, nil
}

func (relay *NostrRelay) SignRelay(privKey *btcec.PrivateKey) error {
	relayBytes := relay.PackBytes()
	hash := sha256.Sum256(relayBytes)

	signature, err := schnorr.Sign(privKey, hash[:])
	if err != nil {
		return err
	}

	relay.Signature = signature.Serialize()
	return nil
}

func (nr *NostrRelay) PackBytes() []byte {
	var packed []byte

	// Pack URL
	packed = append(packed, []byte(nr.URL)...)
	packed = append(packed, 0) // null terminator

	// Pack Name
	packed = append(packed, []byte(nr.Name)...)
	packed = append(packed, 0) // null terminator

	// Pack PublicKey
	packed = append(packed, nr.PublicKey[:]...)

	// Pack SupportedNIPs (sorted)
	sort.Ints(nr.SupportedNIPs)
	for _, nip := range nr.SupportedNIPs {
		nipBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(nipBytes, uint32(nip))
		packed = append(packed, nipBytes...)
	}

	return packed
}

func (relay *NostrRelay) CheckSig() error {
	packedBytes := relay.PackBytes()
	hash := sha256.Sum256(packedBytes)

	// Parse the public key
	pubKey, err := btcec.ParsePubKey(relay.PublicKey[:])
	if err != nil {
		return errors.New("failed to parse public key")
	}

	// Parse the signature
	sig, err := schnorr.ParseSignature(relay.Signature)
	if err != nil {
		return errors.New("failed to parse signature")
	}

	// Verify the signature
	if !sig.Verify(hash[:], pubKey) {
		return errors.New("invalid signature")
	}

	return nil
}

func (nr *NostrRelay) Equals(other *NostrRelay) bool {
	if nr == nil || other == nil {
		return nr == other
	}

	// Compare URL
	if nr.URL != other.URL {
		return false
	}

	// Compare Name
	if nr.Name != other.Name {
		return false
	}

	// Compare PublicKey
	if !bytes.Equal(nr.PublicKey, other.PublicKey) {
		return false
	}

	// Compare Signature
	if !bytes.Equal(nr.Signature, other.Signature) {
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
