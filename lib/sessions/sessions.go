package sessions

import (
	"fmt"
	"sync"

	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

var Sessions sync.Map

type Session struct {
	PublicKey     string
	Signature     *string
	Authenticated bool
}

func init() {
	Sessions = sync.Map{}
}

func CreateSession(publicKey string) error {
	Sessions.Store(publicKey, &Session{
		PublicKey:     publicKey,
		Signature:     nil,
		Authenticated: false,
	})

	return nil
}

func GetSession(publicKey string) *Session {
	data, ok := Sessions.Load(publicKey)
	if !ok {
		return nil
	}

	session, ok := data.(*Session)
	if !ok {
		DeleteSession(publicKey)

		return nil
	}

	return session
}

func VerifySession(publicKey string, data []byte, signature []byte) error {
	_, ok := Sessions.Load(publicKey)
	if !ok {
		return fmt.Errorf("session not found")
	}

	sig, err := schnorr.ParseSignature(signature)
	if err != nil {
		return err
	}

	pubKey, err := signing.DeserializePublicKey(publicKey)
	if err != nil {
		return err
	}

	err = signing.VerifySignature(sig, data, pubKey)
	if err != nil {
		return err
	}

	return nil
}

func DeleteSession(publicKey string) error {
	Sessions.Delete(publicKey)

	return nil
}
