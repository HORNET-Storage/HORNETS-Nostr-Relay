package auth

import (
	"fmt"
	"log"
	"strings"

	"github.com/HORNET-Storage/hornet-storage/lib/sessions"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/subscription"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// createUserSession creates a new session with the given pubkey and signature
func createUserSession(pubKey string, sig string) error {
	if err := sessions.CreateSession(pubKey); err != nil {
		return err
	}

	userSession := sessions.GetSession(pubKey)
	userSession.Signature = &sig
	userSession.Authenticated = true
	return nil
}

// verifyAuthTags verifies that the auth tags are valid
func verifyAuthTags(tags nostr.Tags, challenge string) bool {
	var hasRelayTag, hasChallengeTag bool
	for _, tag := range tags {
		if len(tag) >= 2 {
			if tag[0] == "relay" {
				hasRelayTag = true
			} else if tag[0] == "challenge" {
				hasChallengeTag = true
				if challenge != "" && tag[1] != challenge {
					return false
				}
			}
		}
	}
	return hasRelayTag && hasChallengeTag
}

func BuildAuthHandler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	return func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		data, err := read()
		if err != nil {
			log.Println("Error reading from stream:", err)
			write("NOTICE", "Error reading from stream.")
			return
		}

		var request nostr.AuthEnvelope
		if err := json.Unmarshal(data, &request); err != nil {
			log.Println("Error unmarshaling auth request:", err)
			write("NOTICE", "Error unmarshaling auth request.")
			return
		}

		log.Printf("Handling auth message for user with pubkey: %s", request.Event.PubKey)

		// Validate auth event kind
		if request.Event.Kind != 22242 {
			log.Printf("Invalid auth event kind: %d", request.Event.Kind)
			write("OK", request.Event.ID, false, "Error auth event kind must be 22242")
			return
		}

		// Check auth time validity
		isValid, errMsg := lib_nostr.AuthTimeCheck(request.Event.CreatedAt.Time().Unix())
		if !isValid {
			log.Printf("Auth time check failed: %s", errMsg)
			write("OK", request.Event.ID, false, errMsg)
			return
		}

		// Verify signature
		success, err := request.Event.CheckSignature()
		if err != nil {
			log.Printf("Failed to check signature: %v", err)
			write("OK", request.Event.ID, false, fmt.Sprintf("Failed to check signature: %v", err))
			return
		}

		if !success {
			log.Printf("Signature verification failed for user: %s", request.Event.PubKey)
			write("OK", request.Event.ID, false, "Signature failed to verify")
			return
		}

		// Verify required tags
		if !verifyAuthTags(request.Event.Tags, "") {
			log.Printf("Missing required tags for user %s", request.Event.PubKey)
			write("OK", request.Event.ID, false, "Error event does not have required tags")
			return
		}

		// Check if pubkey is blocked
		isBlocked, err := store.IsBlockedPubkey(request.Event.PubKey)
		if err != nil {
			log.Printf("Error checking if pubkey is blocked: %v", err)
			// Continue processing as normal, don't block due to errors
		} else if isBlocked {
			log.Printf("Blocked pubkey attempted connection: %s", request.Event.PubKey)
			write("OK", request.Event.ID, false, "Relay connection rejected: Pubkey is blocked")
			return
		}

		// Create user session
		if err := createUserSession(request.Event.PubKey, request.Event.Sig); err != nil {
			log.Printf("Failed to create session for %s: %v", request.Event.PubKey, err)
			write("OK", request.Event.ID, false, fmt.Sprintf("Failed to create session: %v", err))
			return
		}

		// Get the global subscription manager
		subManager := subscription.GetGlobalManager()
		if subManager == nil {
			log.Printf("Failed to get global subscription manager")
			write("OK", request.Event.ID, false, "Failed to get subscription manager: Global manager not initialized")
			return
		}

		// Initialize subscriber
		if err := subManager.InitializeSubscriber(request.Event.PubKey); err != nil {
			log.Printf("Failed to initialize subscriber %s: %v", request.Event.PubKey, err)

			// Check for common errors and provide more specific messages
			errMsg := err.Error()
			if strings.Contains(errMsg, "no available addresses") {
				write("OK", request.Event.ID, false, "Failed to allocate Bitcoin address: No addresses available in the pool")
			} else if strings.Contains(errMsg, "failed to allocate Bitcoin address") {
				write("OK", request.Event.ID, false, fmt.Sprintf("Bitcoin address allocation error: %v", err))
			} else {
				write("OK", request.Event.ID, false, fmt.Sprintf("Subscriber initialization failed: %v", err))
			}
			return
		}

		log.Printf("Successfully initialized subscriber %s", request.Event.PubKey)
		write("OK", request.Event.ID, true, "Subscriber successfully initialized")
	}
}
