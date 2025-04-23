package kind19842

import (
	"log"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
)

// BuildKind19842Handler constructs and returns a handler function for kind 19842 (Dispute) events
func BuildKind19842Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		// Read data from the stream
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

		// Validate the event
		success := lib_nostr.ValidateEvent(write, env, 19842)
		if !success {
			return
		}

		// Extract ticket event ID and dispute reason from tags
		ticketEventID := ""
		disputeReason := ""
		for _, tag := range env.Event.Tags {
			if tag[0] == "e" && len(tag) >= 2 {
				ticketEventID = tag[1]
			} else if tag[0] == "reason" && len(tag) >= 2 {
				disputeReason = tag[1]
			}
		}

		// Log the dispute reason for debugging
		if disputeReason != "" {
			log.Printf("Dispute reason: %s", disputeReason)
		}

		// Validate required fields
		if ticketEventID == "" {
			write("OK", env.Event.ID, false, "Missing ticket event ID")
			return
		}

		// Verify the ticket exists and belongs to this user
		filter := nostr.Filter{
			Kinds: []int{19841},
			IDs:   []string{ticketEventID},
		}

		ticketEvents, err := store.QueryEvents(filter)
		if err != nil || len(ticketEvents) == 0 {
			write("OK", env.Event.ID, false, "Referenced ticket not found")
			return
		}

		// Verify the ticket belongs to this user
		ticketEvent := ticketEvents[0]
		userPubKey := ""
		blockedEventID := ""
		for _, tag := range ticketEvent.Tags {
			if tag[0] == "p" && len(tag) >= 2 {
				userPubKey = tag[1]
			} else if tag[0] == "e" && len(tag) >= 2 {
				blockedEventID = tag[1]
			}
		}

		if userPubKey != env.Event.PubKey {
			write("OK", env.Event.ID, false, "You can only dispute your own content")
			return
		}

		// Check if the blocked event still exists
		isBlocked, err := store.IsEventBlocked(blockedEventID)
		if err != nil || !isBlocked {
			write("OK", env.Event.ID, false, "The referenced event is no longer blocked or does not exist")
			return
		}

		// Update the ticket status to "disputed"
		// Create a new event that's a copy of the ticket but with updated status
		updatedTicket := nostr.Event{
			PubKey:    ticketEvent.PubKey,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      19841,
			Tags:      ticketEvent.Tags,
			Content:   ticketEvent.Content,
		}

		// Update the status tag
		statusUpdated := false
		for i, tag := range updatedTicket.Tags {
			if tag[0] == "status" {
				updatedTicket.Tags[i] = nostr.Tag{"status", "disputed"}
				statusUpdated = true
				break
			}
		}

		if !statusUpdated {
			updatedTicket.Tags = append(updatedTicket.Tags, nostr.Tag{"status", "disputed"})
		}

		// Add reference to the dispute event
		updatedTicket.Tags = append(updatedTicket.Tags, nostr.Tag{"e", env.Event.ID, "dispute"})

		// Get relay private key from viper and deserialize it
		serializedPrivKey := viper.GetString("private_key")

		// Sign and store the updated ticket
		if err := updatedTicket.Sign(serializedPrivKey); err != nil {
			log.Printf("Error signing updated ticket: %v", err)
		} else {
			if err := store.StoreEvent(&updatedTicket); err != nil {
				log.Printf("Error storing updated ticket: %v", err)
			}
		}

		// Store the dispute event
		if err := store.StoreEvent(&env.Event); err != nil {
			write("NOTICE", "Failed to store the dispute event")
			return
		}

		// Successfully processed event
		write("OK", env.Event.ID, true, "Dispute received and will be reviewed")
	}

	return handler
}
