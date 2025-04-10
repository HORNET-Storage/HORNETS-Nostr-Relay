package websocket

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/sessions"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/subscription"
)

const (
	AddressStatusAvailable = "available"
	AddressStatusAllocated = "allocated"
	AddressStatusUsed      = "used"
)

func handleAuthMessage(c *websocket.Conn, env *nostr.AuthEnvelope, challenge string, state *connectionState, store stores.Store) {
	write := func(messageType string, params ...interface{}) {
		response := lib_nostr.BuildResponse(messageType, params)
		if len(response) > 0 {
			handleIncomingMessage(c, response)
		}
	}

	log.Printf("Handling auth message for user with pubkey: %s", env.Event.PubKey)

	// Validate auth event kind
	if env.Event.Kind != 22242 {
		log.Printf("Invalid auth event kind: %d", env.Event.Kind)
		write("OK", env.Event.ID, false, "Error auth event kind must be 22242")
		return
	}

	// Check auth time validity
	isValid, errMsg := lib_nostr.AuthTimeCheck(env.Event.CreatedAt.Time().Unix())
	if !isValid {
		log.Printf("Auth time check failed: %s", errMsg)
		write("OK", env.Event.ID, false, errMsg)
		return
	}

	// Verify signature
	success, err := env.Event.CheckSignature()
	if err != nil {
		log.Printf("Failed to check signature: %v", err)
		write("OK", env.Event.ID, false, fmt.Sprintf("Failed to check signature: %v", err))
		return
	}

	if !success {
		log.Printf("Signature verification failed for user: %s", env.Event.PubKey)
		write("OK", env.Event.ID, false, "Signature failed to verify")
		return
	}

	// Verify required tags
	if !verifyAuthTags(env.Event.Tags, challenge) {
		log.Printf("Missing required tags for user %s", env.Event.PubKey)
		write("OK", env.Event.ID, false, "Error event does not have required tags")
		return
	}

	// Check if pubkey is blocked
	isBlocked, err := store.IsBlockedPubkey(env.Event.PubKey)
	if err != nil {
		log.Printf("Error checking if pubkey is blocked: %v", err)
		// Continue processing as normal, don't block due to errors
	} else if isBlocked {
		log.Printf("Blocked pubkey attempted connection: %s", env.Event.PubKey)
		write("OK", env.Event.ID, false, "Relay connection rejected: Pubkey is blocked")
		return
	}

	// Create user session
	if err := createUserSession(env.Event.PubKey, env.Event.Sig); err != nil {
		log.Printf("Failed to create session for %s: %v", env.Event.PubKey, err)
		write("OK", env.Event.ID, false, fmt.Sprintf("Failed to create session: %v", err))
		return
	}

	// Get the global subscription manager
	subManager := subscription.GetGlobalManager()
	if subManager == nil {
		log.Printf("Failed to get global subscription manager")
		write("OK", env.Event.ID, false, "Failed to get subscription manager: Global manager not initialized")
		return
	}

	// Initialize subscriber
	if err := subManager.InitializeSubscriber(env.Event.PubKey); err != nil {
		log.Printf("Failed to initialize subscriber %s: %v", env.Event.PubKey, err)

		// Check for common errors and provide more specific messages
		errMsg := err.Error()
		if strings.Contains(errMsg, "no available addresses") {
			write("OK", env.Event.ID, false, "Failed to allocate Bitcoin address: No addresses available in the pool")
		} else if strings.Contains(errMsg, "failed to allocate Bitcoin address") {
			write("OK", env.Event.ID, false, fmt.Sprintf("Bitcoin address allocation error: %v", err))
		} else {
			write("OK", env.Event.ID, false, fmt.Sprintf("Subscriber initialization failed: %v", err))
		}
		return
	}

	log.Printf("Successfully initialized subscriber %s", env.Event.PubKey)
	write("OK", env.Event.ID, true, "Subscriber successfully initialized")

	// Store the pubkey in connection state for future block checks
	state.pubkey = env.Event.PubKey
	state.authenticated = true
	state.blockedCheck = time.Now()

	if !state.authenticated {
		log.Printf("Session established but subscription inactive for %s", env.Event.PubKey)
		write("NOTICE", "Session established but subscription inactive. Renew to continue access.")
	}
}

// Helper functions

func verifyAuthTags(tags nostr.Tags, challenge string) bool {
	var hasRelayTag, hasChallengeTag bool
	for _, tag := range tags {
		if len(tag) >= 2 {
			if tag[0] == "relay" {
				hasRelayTag = true
			} else if tag[0] == "challenge" {
				hasChallengeTag = true
				if tag[1] != challenge {
					return false
				}
			}
		}
	}
	return hasRelayTag && hasChallengeTag
}

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

// Removed unused function: initializeSubscriptionManager
