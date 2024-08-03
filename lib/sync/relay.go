package sync

import (
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"sort"
)

type NostrRelay struct {
	URL           string   `json:"url"`
	Name          string   `json:"name"`
	PublicKey     [32]byte `json:"public_key"`
	Signature     []byte   `json:"signature"`
	SupportedNIPs []int    `json:"supported_nips"`
}

func CreateSelfRelay(url string, name string, pubKey [32]byte, privKey *btcec.PrivateKey, supportedNIPs []int) (*NostrRelay, error) {
	self := &NostrRelay{
		URL:           url,
		Name:          name,
		PublicKey:     pubKey,
		SupportedNIPs: supportedNIPs,
	}

	err := SignRelay(self, privKey)
	if err != nil {
		return nil, err
	}
	return self, nil
}

func SignRelay(relay *NostrRelay, privKey *btcec.PrivateKey) error {
	relayBytes := relay.PackBytes()
	hash := sha1.Sum(relayBytes)

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
	hash := sha1.Sum(packedBytes)

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
