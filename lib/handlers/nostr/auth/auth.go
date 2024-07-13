package auth

import (
	"log"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// Not being used until the session system is finished to unify the transports
// as the current listeners / subscriptions are dependant on the web socket connection
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
			log.Println("Error unmarshaling count request:", err)
			write("NOTICE", "Error unmarshaling auth request.")
			return
		}

		if request.Event.Kind != 22242 {
			write("OK", request.Event.ID, false, "Error auth event kind must be 22242")
			return
		}

		isValid, errMsg := lib_nostr.AuthTimeCheck(request.Event.CreatedAt.Time().Unix())
		if !isValid {
			write("OK", request.Event.ID, false, errMsg)
			return
		}

		result, err := request.Event.CheckSignature()
		if err != nil {
			write("OK", request.Event.ID, false, "Error checking event signature")
			return
		}

		if !result {
			write("OK", request.Event.ID, false, "Error signature verification failed")
			return
		}

		var hasRelayTag, hasChallengeTag bool
		for _, tag := range request.Event.Tags {
			if len(tag) >= 2 {
				if tag[0] == "relay" {
					hasRelayTag = true
				} else if tag[0] == "challenge" {
					hasChallengeTag = true

					// GET SESSION AND CHECK CHALLENGE MATCHES
				}
			}
		}

		if !hasRelayTag || !hasChallengeTag {
			write("CLOSE", request.Event.ID, false, "Error event does not have required tags")
			return
		}

		// GET SESSION AND SET IT TO AUTHORIZED

		write("OK", request.Event.ID, true, "")
	}
}
