package websocket

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/nbd-wtf/go-nostr"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
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

	logging.Infof("Handling auth message for user with pubkey: %s", env.Event.PubKey)

	// Validate auth event kind
	if env.Event.Kind != 22242 {
		logging.Infof("Invalid auth event kind: %d", env.Event.Kind)
		write("OK", env.Event.ID, false, "Error auth event kind must be 22242")
		return
	}

	// Check auth time validity
	isValid, errMsg := lib_nostr.AuthTimeCheck(env.Event.CreatedAt.Time().Unix())
	if !isValid {
		logging.Infof("Auth time check failed: %s", errMsg)
		write("OK", env.Event.ID, false, errMsg)
		return
	}

	// Verify signature
	success, err := env.Event.CheckSignature()
	if err != nil {
		logging.Infof("Failed to check signature: %v", err)
		write("OK", env.Event.ID, false, fmt.Sprintf("Failed to check signature: %v", err))
		return
	}

	if !success {
		logging.Infof("Signature verification failed for user: %s", env.Event.PubKey)
		write("OK", env.Event.ID, false, "Signature failed to verify")
		return
	}

	// Verify required tags
	if !verifyAuthTags(env.Event.Tags, challenge) {
		logging.Infof("Missing required tags for user %s", env.Event.PubKey)
		write("OK", env.Event.ID, false, "Error event does not have required tags")
		return
	}

	// Check if pubkey is blocked
	isBlocked, err := store.IsBlockedPubkey(env.Event.PubKey)
	if err != nil {
		logging.Infof("Error checking if pubkey is blocked: %v", err)
		// Continue processing as normal, don't block due to errors
	} else if isBlocked {
		logging.Infof("Blocked pubkey attempted connection: %s", env.Event.PubKey)
		write("OK", env.Event.ID, false, "restricted: This pubkey has been blocked from this relay.")
		return
	}

	// Check read access permissions using H.O.R.N.E.T Allowed Users system
	if accessControl := GetAccessControl(); accessControl != nil {
		err := accessControl.CanRead(env.Event.PubKey)
		if err != nil {
			logging.Infof("Read access denied for pubkey: %s - %v", env.Event.PubKey, err)
			
			// Get current settings to provide appropriate error message
			settings := accessControl.GetSettings()
			if settings != nil {
				var errorMsg string
				switch settings.Mode {
				case "invite-only":
					if settings.Read == "allowed_users" {
						errorMsg = "restricted: This relay is invite-only. Contact the relay administrator to request access."
					} else {
						errorMsg = "restricted: Authentication required for this relay."
					}
				case "only-me":
					errorMsg = "restricted: This relay is private and only accessible to the owner."
				case "subscription":
					errorMsg = "restricted: This relay requires a paid subscription. Visit the relay website to subscribe."
				default:
					errorMsg = "restricted: Authentication failed - access denied."
				}
				write("OK", env.Event.ID, false, errorMsg)
			} else {
				write("OK", env.Event.ID, false, "restricted: Authentication failed - access denied.")
			}
			return
		}
	}

	// Create user session
	if err := createUserSession(env.Event.PubKey, env.Event.Sig); err != nil {
		logging.Infof("Failed to create session for %s: %v", env.Event.PubKey, err)
		write("OK", env.Event.ID, false, fmt.Sprintf("Failed to create session: %v", err))
		return
	}

	// Get the global subscription manager
	subManager := subscription.GetGlobalManager()
	if subManager == nil {
		logging.Infof("Failed to get global subscription manager")
		write("OK", env.Event.ID, false, "Failed to get subscription manager: Global manager not initialized")
		return
	}

	// Get current relay configuration
	settings, err := config.GetConfig()
	if err != nil {
		logging.Infof("Failed to get config: %v", err)
		write("OK", env.Event.ID, false, "Failed to get relay configuration")
		return
	}

	// Initialize subscriber with current mode asynchronously
	currentMode := settings.AllowedUsersSettings.Mode
	go func(pubkey string, mode string) {
		if err := subManager.InitializeSubscriber(pubkey, mode); err != nil {
			logging.Infof("Failed to initialize subscriber %s: %v", pubkey, err)

			// Log specific error types for monitoring
			errMsg := err.Error()
			if strings.Contains(errMsg, "no available addresses") && mode == "subscription" {
				logging.Infof("Warning: No Bitcoin addresses available for subscriber %s", pubkey)
			} else if strings.Contains(errMsg, "failed to allocate Bitcoin address") && mode == "subscription" {
				logging.Infof("Warning: Bitcoin address allocation failed for subscriber %s: %v", pubkey, err)
			}
		} else {
			logging.Infof("Successfully initialized subscriber %s", pubkey)
		}
	}(env.Event.PubKey, currentMode)

	// Return success after authentication
	logging.Infof("Successfully authenticated user %s", env.Event.PubKey)
	write("OK", env.Event.ID, true, "Authentication successful")

	// Store the pubkey in connection state for future block checks
	state.pubkey = env.Event.PubKey
	state.authenticated = true
	state.blockedCheck = time.Now()

	if !state.authenticated {
		logging.Infof("Session established but subscription inactive for %s", env.Event.PubKey)
		write("NOTICE", "Session established but subscription inactive. Renew to continue access.")
	}
}

// Helper functions

func verifyAuthTags(tags nostr.Tags, challenge string) bool {
	var hasRelayTag, hasChallengeTag bool
	for _, tag := range tags {
		if len(tag) >= 2 {
			switch tag[0] {
			case "relay":
				hasRelayTag = true
			case "challenge":
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
