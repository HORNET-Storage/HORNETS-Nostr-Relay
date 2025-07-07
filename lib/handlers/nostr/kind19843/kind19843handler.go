package kind19843

import (
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
)

// BuildKind19843Handler constructs and returns a handler function for kind 19843 (Resolution) events.
// This handler is primarily for receiving and validating resolutions, as resolutions are created by the relay itself.
func BuildKind19843Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
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
		success := lib_nostr.ValidateEvent(write, env, 19843)
		if !success {
			return
		}

		// Only the relay should be able to create resolutions
		// In a production environment, we would check if the pubkey matches the relay's pubkey
		// For now, we'll just store the event
		if err := store.StoreEvent(&env.Event); err != nil {
			write("NOTICE", "Failed to store the resolution event")
			return
		}

		// Successfully processed event
		write("OK", env.Event.ID, true, "Resolution processed successfully")
	}

	return handler
}

// CreateResolutionEvent creates a new resolution event for a dispute
func CreateResolutionEvent(
	store stores.Store,
	disputeEventID string,
	ticketEventID string,
	originalEventID string,
	userPubKey string,
	approved bool,
	reason string,
	relayPubKey string,
	relayPrivKey string,
) (*nostr.Event, error) {
	// Determine resolution status
	resolutionStatus := "approved"
	if !approved {
		resolutionStatus = "rejected"
	}

	// Create resolution content
	resolutionContent := ""
	if approved {
		resolutionContent = "Your dispute has been approved. The content has been unblocked and is now available."
	} else {
		resolutionContent = "Your dispute has been rejected. The content remains blocked."
	}

	if reason != "" {
		resolutionContent += " Reason: " + reason
	}

	// Create the resolution event
	resolutionEvent := nostr.Event{
		PubKey:    relayPubKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      19843,
		Tags: nostr.Tags{
			{"e", disputeEventID, "dispute"},
			{"e", ticketEventID, "ticket"},
			{"e", originalEventID, "original"},
			{"p", userPubKey},
			{"resolution", resolutionStatus},
		},
		Content: resolutionContent,
	}

	if reason != "" {
		resolutionEvent.Tags = append(resolutionEvent.Tags, nostr.Tag{"reason", reason})
	}

	// Sign the resolution event
	if err := resolutionEvent.Sign(relayPrivKey); err != nil {
		logging.Infof("Error signing resolution event: %v", err)
		return nil, err
	}

	// Store the resolution event
	if err := store.StoreEvent(&resolutionEvent); err != nil {
		logging.Infof("Error storing resolution event: %v", err)
		return nil, err
	}

	// If the dispute is approved, unblock the original event and delete the ticket
	if approved {
		// Unblock the event
		if err := store.UnmarkEventBlocked(originalEventID); err != nil {
			logging.Infof("Error unblocking event %s: %v", originalEventID, err)
			// Continue anyway as the resolution is still valid
		} else {
			logging.Infof("Event %s has been unblocked due to approved dispute", originalEventID)
		}

		// Delete the ticket event
		if err := store.DeleteEvent(ticketEventID); err != nil {
			logging.Infof("Error deleting ticket event %s: %v", ticketEventID, err)
			// Continue anyway as the resolution is still valid
		} else {
			logging.Infof("Ticket event %s has been deleted after successful dispute", ticketEventID)
		}
	}

	return &resolutionEvent, nil
}
