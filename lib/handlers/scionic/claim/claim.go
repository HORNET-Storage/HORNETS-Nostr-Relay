package claim

import (
	"encoding/hex"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"

	lib_types "github.com/HORNET-Storage/go-hornet-storage-lib/lib"
	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
	types "github.com/HORNET-Storage/hornet-storage/lib/types"

	lib_stream "github.com/HORNET-Storage/go-hornet-storage-lib/lib/connmgr"
	hsListener "github.com/HORNET-Storage/go-hornet-storage-lib/lib/connmgr/hyperswarm"
)

func AddClaimOwnershipHandler(listener *hsListener.HyperswarmListener, store stores.Store) {
	listener.SetStreamHandler("/claim-ownership", BuildClaimOwnershipHandler(store))
}

func BuildClaimOwnershipHandler(store stores.Store) hsListener.StreamHandler {
	handler := func(stream lib_types.Stream) {
		defer stream.Close()

		// Read the claim ownership message
		message, err := lib_stream.ReadMessageFromStream[types.ClaimOwnershipMessage](stream)
		if err != nil {
			writeErrorResponse(stream, "Failed to receive claim ownership message")
			return
		}

		// Validate required fields
		if message.Root == "" {
			writeErrorResponse(stream, "Root hash is required")
			return
		}
		if message.PublicKey == "" {
			writeErrorResponse(stream, "Public key is required")
			return
		}
		if message.Signature == "" {
			writeErrorResponse(stream, "Signature is required")
			return
		}

		// Deserialize and verify the public key
		publicKey, err := signing.DeserializePublicKey(message.PublicKey)
		if err != nil {
			writeErrorResponse(stream, "Failed to deserialize public key")
			return
		}

		// Deserialize the signature
		signatureBytes, err := hex.DecodeString(message.Signature)
		if err != nil {
			writeErrorResponse(stream, "Failed to decode signature hex")
			return
		}

		signature, err := schnorr.ParseSignature(signatureBytes)
		if err != nil {
			writeErrorResponse(stream, "Failed to parse signature")
			return
		}

		// Verify signature: public key must have signed the root hash
		err = signing.VerifySerializedCIDSignature(signature, message.Root, publicKey)
		if err != nil {
			writeErrorResponse(stream, "Signature verification failed")
			return
		}

		// Serialize the public key for storage (normalized format)
		serializedPublicKey, err := signing.SerializePublicKey(publicKey)
		if err != nil {
			writeErrorResponse(stream, "Failed to serialize public key")
			return
		}

		// Attempt to claim ownership
		err = store.ClaimOwnership(message.Root, *serializedPublicKey, hex.EncodeToString(signature.Serialize()))
		if err != nil {
			logging.Infof("Claim ownership failed for root %s: %v", message.Root, err)
			writeErrorResponse(stream, err.Error())
			return
		}

		logging.Infof("Ownership claimed for root %s by %s", message.Root, (*serializedPublicKey)[:16]+"...")

		// Send success response
		lib_stream.WriteMessageToStream(stream, lib_stream.BuildResponseMessage(true, "Ownership claimed successfully"))
	}

	return handler
}

func writeErrorResponse(stream lib_types.Stream, message string) {
	lib_stream.WriteErrorToStream(stream, message, nil)
}
