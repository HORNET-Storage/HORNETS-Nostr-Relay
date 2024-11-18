package websocket

import (
	"fmt"
	"log"

	"github.com/gofiber/contrib/websocket"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/sessions"
	"github.com/HORNET-Storage/hornet-storage/lib/signing"
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
		write("NOTICE", "Failed to check signature")
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

	// Create user session
	if err := createUserSession(env.Event.PubKey, env.Event.Sig); err != nil {
		log.Printf("Failed to create session for %s: %v", env.Event.PubKey, err)
		write("NOTICE", "Failed to create session")
		return
	}

	// Initialize subscription manager
	subManager, err := initializeSubscriptionManager(store)
	if err != nil {
		log.Printf("Failed to initialize subscription manager: %v", err)
		write("NOTICE", "Failed to initialize subscription")
		return
	}

	// Initialize subscriber
	if err := subManager.InitializeSubscriber(env.Event.PubKey); err != nil {
		log.Printf("Failed to initialize subscriber %s: %v", env.Event.PubKey, err)
		write("NOTICE", fmt.Sprintf("Failed to initialize subscriber: %v", err))
		return
	}

	log.Printf("Successfully initialized subscriber %s", env.Event.PubKey)
	write("OK", env.Event.ID, true, "Subscriber successfully initialized")

	state.authenticated = true

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

func initializeSubscriptionManager(store stores.Store) (*subscription.SubscriptionManager, error) {
	// Load relay private key
	privateKey, _, err := signing.DeserializePrivateKey(viper.GetString("private_key"))
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize private key: %v", err)
	}

	// Define subscription tiers
	subscriptionTiers := []types.SubscriptionTier{
		{DataLimit: "1 GB per month", Price: "8000"},
		{DataLimit: "5 GB per month", Price: "10000"},
		{DataLimit: "10 GB per month", Price: "15000"},
	}

	// Create subscription manager
	return subscription.NewSubscriptionManager(
		store,
		privateKey,
		// viper.GetString("relay_bitcoin_address"),
		viper.GetString("RelayDHTkey"),
		subscriptionTiers,
	), nil
}
