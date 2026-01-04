package claim

import (
	"context"
	"encoding/hex"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"

	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/sessions/libp2p/middleware"
	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
	types "github.com/HORNET-Storage/hornet-storage/lib/types"

	lib_stream "github.com/HORNET-Storage/go-hornet-storage-lib/lib/connmgr"
	libp2p_stream "github.com/HORNET-Storage/go-hornet-storage-lib/lib/connmgr/libp2p"
)

func AddClaimOwnershipHandler(libp2phost host.Host, store stores.Store) {
	libp2phost.SetStreamHandler("/claim-ownership", middleware.SessionMiddleware(libp2phost)(BuildClaimOwnershipHandler(store)))
}

func BuildClaimOwnershipHandler(store stores.Store) func(network.Stream) {
	handler := func(stream network.Stream) {
		ctx := context.Background()
		libp2pStream := libp2p_stream.New(stream, ctx)
		defer stream.Close()

		// Read the claim ownership message
		message, err := lib_stream.ReadMessageFromStream[types.ClaimOwnershipMessage](libp2pStream)
		if err != nil {
			writeErrorResponse(libp2pStream, "Failed to receive claim ownership message")
			return
		}

		// Validate required fields
		if message.Root == "" {
			writeErrorResponse(libp2pStream, "Root hash is required")
			return
		}
		if message.PublicKey == "" {
			writeErrorResponse(libp2pStream, "Public key is required")
			return
		}
		if message.Signature == "" {
			writeErrorResponse(libp2pStream, "Signature is required")
			return
		}

		// Deserialize and verify the public key
		publicKey, err := signing.DeserializePublicKey(message.PublicKey)
		if err != nil {
			writeErrorResponse(libp2pStream, "Failed to deserialize public key")
			return
		}

		// Deserialize the signature
		signatureBytes, err := hex.DecodeString(message.Signature)
		if err != nil {
			writeErrorResponse(libp2pStream, "Failed to decode signature hex")
			return
		}

		signature, err := schnorr.ParseSignature(signatureBytes)
		if err != nil {
			writeErrorResponse(libp2pStream, "Failed to parse signature")
			return
		}

		// Verify signature: public key must have signed the root hash
		err = signing.VerifySerializedCIDSignature(signature, message.Root, publicKey)
		if err != nil {
			writeErrorResponse(libp2pStream, "Signature verification failed")
			return
		}

		// Serialize the public key for storage (normalized format)
		serializedPublicKey, err := signing.SerializePublicKey(publicKey)
		if err != nil {
			writeErrorResponse(libp2pStream, "Failed to serialize public key")
			return
		}

		// Attempt to claim ownership
		err = store.ClaimOwnership(message.Root, *serializedPublicKey, hex.EncodeToString(signature.Serialize()))
		if err != nil {
			logging.Infof("Claim ownership failed for root %s: %v", message.Root, err)
			writeErrorResponse(libp2pStream, err.Error())
			return
		}

		logging.Infof("Ownership claimed for root %s by %s", message.Root, (*serializedPublicKey)[:16]+"...")

		// Send success response
		lib_stream.WriteMessageToStream(libp2pStream, lib_stream.BuildResponseMessage(true, "Ownership claimed successfully"))
	}

	return handler
}

func writeErrorResponse(stream *libp2p_stream.Libp2pStream, message string) {
	lib_stream.WriteErrorToStream(stream, message, nil)
}
