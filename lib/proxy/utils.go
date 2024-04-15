package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"
)

func serializeEventForID(event *nostr.Event) ([]byte, error) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	serializableArray := []interface{}{
		0,
		event.PubKey,
		event.CreatedAt,
		event.Kind,
		event.Tags,
		event.Content,
	}

	serialized, err := json.Marshal(serializableArray)
	if err != nil {
		return nil, err
	}

	return serialized, nil
}

// HashAndCompare hashes the serialized event and compares it with the event ID
func HashAndCompare(serialized []byte, id string) (bool, [32]byte) {
	hash := sha256.Sum256(serialized)
	hashString := hex.EncodeToString(hash[:])
	log.Println("Serialized event hash:", hashString)
	log.Println("Event ID:", id)
	return hashString == id, hash
}

func verifyNote(event *nostr.Event) bool {
	serialized, err := serializeEventForID(event)
	if err != nil {
		fmt.Println("Error serializing event:", err)
		return false
	}
	log.Println("The Event ID is:", event.ID)
	match, hash := HashAndCompare(serialized, event.ID)
	if match {
		fmt.Println("Hash matches ID:", event.ID)
	} else {
		fmt.Println("Hash does not match ID")
	}
	signatureBytes, _ := hex.DecodeString(event.Sig)
	cleanSignature, _ := schnorr.ParseSignature(signatureBytes)
	publicSignatureBytes, _ := hex.DecodeString(event.PubKey)

	cleanPublicKey, _ := schnorr.ParsePubKey(publicSignatureBytes)

	is_valid := cleanSignature.Verify(hash[:], cleanPublicKey)

	if is_valid {
		fmt.Println("Signature is valid from my implementation")
	} else {
		fmt.Println("Signature is invalid from my implementation")
	}

	log.Println("Event tags: ", event.Tags)

	isvalid, err := event.CheckSignature()
	if err != nil {
		log.Println("Error checking signature:", err)
		return false
	}
	if isvalid {
		fmt.Println("Signature is valid")
	} else {
		fmt.Println("Signature is invalid")
	}

	if is_valid && match {
		return true
	} else {
		return false
	}
}
