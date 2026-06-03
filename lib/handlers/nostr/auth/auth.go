package auth

import (
	"fmt"
	"strings"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/sessions"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/subscription"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

type AccessController interface {
	CanRead(npub string) error
	GetSettings() *types.AllowedUsersSettings
}

type AuthResult struct {
	PubKey string
	Sig    string
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

// verifyAuthTags verifies that the auth tags are valid
func verifyAuthTags(tags nostr.Tags, challenge string) bool {
	var hasRelayTag, hasChallengeTag bool
	for _, tag := range tags {
		if len(tag) >= 2 {
			switch tag[0] {
			case "relay":
				hasRelayTag = true
			case "challenge":
				hasChallengeTag = true
				if challenge != "" && tag[1] != challenge {
					return false
				}
			}
		}
	}
	return hasRelayTag && hasChallengeTag
}

func AuthenticateEvent(event *nostr.Event, challenge string, store stores.Store, accessControl AccessController) (*AuthResult, string, bool) {
	if event == nil {
		return nil, "Error auth event missing", false
	}

	logging.Infof("Handling auth message for user with pubkey: %s", event.PubKey)

	if event.Kind != 22242 {
		logging.Infof("Invalid auth event kind: %d", event.Kind)
		return nil, "Error auth event kind must be 22242", false
	}

	isValid, errMsg := lib_nostr.AuthTimeCheck(event.CreatedAt.Time().Unix())
	if !isValid {
		logging.Infof("Auth time check failed: %s", errMsg)
		return nil, errMsg, false
	}

	success, err := event.CheckSignature()
	if err != nil {
		logging.Infof("Failed to check signature: %v", err)
		return nil, fmt.Sprintf("Failed to check signature: %v", err), false
	}

	if !success {
		logging.Infof("Signature verification failed for user: %s", event.PubKey)
		return nil, "Signature failed to verify", false
	}

	if !verifyAuthTags(event.Tags, challenge) {
		logging.Infof("Missing required tags for user %s", event.PubKey)
		return nil, "Error event does not have required tags", false
	}

	if store != nil {
		isBlocked, err := store.IsBlockedPubkey(event.PubKey)
		if err != nil {
			logging.Infof("Error checking if pubkey is blocked: %v", err)
		} else if isBlocked {
			logging.Infof("Blocked pubkey attempted connection: %s", event.PubKey)
			return nil, "restricted: This pubkey has been blocked from this relay.", false
		}
	}

	_ = accessControl

	if err := createUserSession(event.PubKey, event.Sig); err != nil {
		logging.Infof("Failed to create session for %s: %v", event.PubKey, err)
		return nil, fmt.Sprintf("Failed to create session: %v", err), false
	}

	subManager := subscription.GetGlobalManager()
	if subManager == nil {
		logging.Infof("Failed to get global subscription manager")
		return nil, "Failed to get subscription manager: Global manager not initialized", false
	}

	settings, err := config.GetConfig()
	if err != nil {
		logging.Infof("Failed to get config: %v", err)
		return nil, "Failed to get relay configuration", false
	}

	currentMode := settings.AllowedUsersSettings.Mode
	go func(pubkey string, mode string) {
		if err := subManager.InitializeSubscriber(pubkey, mode); err != nil {
			logging.Infof("Failed to initialize subscriber %s: %v", pubkey, err)

			errMsg := err.Error()
			if strings.Contains(errMsg, "no available addresses") && mode == "subscription" {
				logging.Infof("Warning: No Bitcoin addresses available for subscriber %s", pubkey)
			} else if strings.Contains(errMsg, "failed to allocate Bitcoin address") && mode == "subscription" {
				logging.Infof("Warning: Bitcoin address allocation failed for subscriber %s: %v", pubkey, err)
			}
		}
	}(event.PubKey, currentMode)

	logging.Infof("Successfully authenticated user %s", event.PubKey)
	return &AuthResult{PubKey: event.PubKey, Sig: event.Sig}, "Authentication successful", true
}

func accessDeniedMessage(settings *types.AllowedUsersSettings) string {
	if settings == nil {
		return "restricted: Authentication failed - access denied."
	}

	switch settings.Mode {
	case "invite-only":
		if settings.Read == "allowed_users" {
			return "restricted: This relay is invite-only. Contact the relay administrator to request access."
		}
		return "restricted: Authentication required for this relay."
	case "only-me":
		return "restricted: This relay is private and only accessible to the owner."
	case "subscription":
		return "restricted: This relay requires a paid subscription. Go to the subscription page to subscribe."
	default:
		return "restricted: Authentication failed - access denied."
	}
}

func BuildAuthHandler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	return func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		data, err := read()
		if err != nil {
			logging.Infof("Error reading from stream:%s", err)
			write("NOTICE", "Error reading from stream.")
			return
		}

		var request nostr.AuthEnvelope
		if err := json.Unmarshal(data, &request); err != nil {
			logging.Infof("Error unmarshaling auth request:%s", err)
			write("NOTICE", "Error unmarshaling auth request.")
			return
		}

		_, message, ok := AuthenticateEvent(&request.Event, "", store, nil)
		write("OK", request.Event.ID, ok, message)
	}
}
