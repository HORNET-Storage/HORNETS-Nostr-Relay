package nostr_relay

import (
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

func TestHandleEventRoutesConfigAllowedKindToUniversal(t *testing.T) {
	viper.Reset()
	lib_nostr.ClearHandlers()
	t.Cleanup(func() {
		viper.Reset()
		lib_nostr.ClearHandlers()
		config.InitConfigForTesting()
	})

	viper.Set("event_filtering.allow_unregistered_kinds", false)
	viper.Set("event_filtering.registered_kinds", []int{73})
	viper.Set("event_filtering.kind_whitelist", []string{"kind73"})
	config.InitConfigForTesting()

	calledUniversal := false
	lib_nostr.RegisterHandler("universal", func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		calledUniversal = true
		write("OK", "event-id", true, "handled")
	})

	var ok bool
	handleEvent(&nostr.EventEnvelope{
		Event: nostr.Event{
			ID:     "event-id",
			Kind:   73,
			PubKey: "pubkey",
		},
	}, func(messageType string, params ...interface{}) {
		if messageType == "OK" && len(params) >= 2 {
			ok, _ = params[1].(bool)
		}
	}, nil, jsoniter.ConfigCompatibleWithStandardLibrary)

	if !calledUniversal {
		t.Fatal("expected config-allowed kind without a specific handler to use universal handler")
	}
	if !ok {
		t.Fatal("expected universal handler acknowledgement")
	}
}
