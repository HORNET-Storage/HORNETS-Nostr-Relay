package kind1059

import (
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	pushService "github.com/HORNET-Storage/hornet-storage/services/push"
)

// BuildKind1059Handler handles Gift Wrap Events (NIP-59)
// Kind 1059 events are the outer envelope for encrypted messages with sealed sender privacy.
// They contain encrypted kind 13 seals which in turn contain the actual message.
// The relay can see the recipient but not the sender or content.
func BuildKind1059Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		// Read data from the stream
		data, err := read()
		if err != nil {
			write("NOTICE", "Error reading data from stream")
			return
		}

		// Unmarshal the nostr envelope
		var env nostr.EventEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			write("NOTICE", "Failed to deserialize the event envelope")
			return
		}

		// Log incoming gift wrap
		logging.Infof("[KIND 1059] Received gift wrap event ID: %s", env.Event.ID)
		logging.Infof("[KIND 1059] Ephemeral pubkey: %s", env.Event.PubKey)
		logging.Infof("[KIND 1059] Event has %d tags", len(env.Event.Tags))

		// Check relay settings for allowed events whilst also verifying signatures and kind number
		success := lib_nostr.ValidateEvent(write, env, 1059)
		if !success {
			logging.Infof("[KIND 1059] Event %s failed validation", env.Event.ID)
			return
		}

		// Validate required tags for Gift Wrap - must have at least one "p" tag for recipient
		hasRecipient := false
		recipientPubkey := ""

		for _, tag := range env.Event.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				hasRecipient = true
				recipientPubkey = tag[1]
				logging.Infof("[KIND 1059] Found recipient: %s", recipientPubkey)
				break
			}
		}

		if !hasRecipient {
			logging.Infof("[KIND 1059] Rejected event %s: missing recipient 'p' tag", env.Event.ID)
			write("OK", env.Event.ID, false, "Gift wrap must have a recipient 'p' tag")
			return
		}

		// Validate content is not empty (should contain encrypted seal)
		if env.Event.Content == "" {
			logging.Infof("[KIND 1059] Rejected event %s: empty content", env.Event.ID)
			write("OK", env.Event.ID, false, "Gift wrap content cannot be empty")
			return
		}

		// Store the gift wrap event
		err = store.StoreEvent(&env.Event)
		if err != nil {
			logging.Infof("[KIND 1059] Failed to store event %s: %v", env.Event.ID, err)
			write("OK", env.Event.ID, false, "Failed to store event")
			return
		}

		// Trigger push notifications for gift wrapped DMs
		// The notification will show as an encrypted message since we can't decrypt the content
		ps := pushService.GetGlobalPushService()
		if ps != nil {
			// Process for push notifications
			// We pass the gift wrap event which will trigger notifications for the recipient
			ps.ProcessEvent(&env.Event)
			logging.Infof("[KIND 1059] Processed gift wrap for push notifications to recipient: %s", recipientPubkey)
		}

		logging.Infof("[KIND 1059] Successfully stored gift wrap event %s for recipient %s", env.Event.ID, recipientPubkey)
		write("OK", env.Event.ID, true, "Gift wrap event stored successfully")
	}

	return handler
}