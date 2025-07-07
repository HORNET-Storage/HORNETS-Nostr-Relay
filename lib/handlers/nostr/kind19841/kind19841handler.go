package kind19841

import (
	"strconv"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
)

// BuildKind19841Handler constructs and returns a handler function for kind 19841 (Moderation Ticket) events.
// This handler is primarily for receiving and validating tickets, as tickets are created by the relay itself.
func BuildKind19841Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		// Read data from the stream.
		data, err := read()
		if err != nil {
			write("NOTICE", "Error reading from stream.")
			return
		}

		// Unmarshal the received data into a Nostr event
		var env nostr.EventEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			write("NOTICE", "Error unmarshaling event.")
			return
		}

		// Check relay settings for allowed events whilst also verifying signatures and kind number
		success := lib_nostr.ValidateEvent(write, env, 19841)
		if !success {
			return
		}

		// Only the relay should be able to create tickets
		// In a production environment, we would check if the pubkey matches the relay's pubkey
		// For now, we'll just store the event
		if err := store.StoreEvent(&env.Event); err != nil {
			write("NOTICE", "Failed to store the ticket event")
			return
		}

		// Successfully processed event
		write("OK", env.Event.ID, true, "Ticket processed successfully")
	}

	return handler
}

// CreateModerationTicket creates a new moderation ticket for a blocked event
// This function is called by the moderation system when content is blocked
func CreateModerationTicket(store stores.Store, blockedEventID string, userPubKey string, reason string, contentLevel int, mediaURL string, relayPubKey string, relayPrivKey string) (*nostr.Event, error) {
	// Create the ticket event
	ticketEvent := nostr.Event{
		PubKey:    relayPubKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      19841,
		Tags: nostr.Tags{
			{"e", blockedEventID},
			{"p", userPubKey},
			{"blocked_reason", reason},
			{"content_level", strconv.Itoa(contentLevel)},
			{"media_url", mediaURL},
			{"status", "blocked"},
		},
		Content: "",
	}

	// Sign the event with the relay's private key
	if err := ticketEvent.Sign(relayPrivKey); err != nil {
		logging.Infof("Error signing moderation ticket: %v", err)
		return nil, err
	}

	// Store the ticket event
	if err := store.StoreEvent(&ticketEvent); err != nil {
		logging.Infof("Error storing moderation ticket: %v", err)
		return nil, err
	}

	return &ticketEvent, nil
}
