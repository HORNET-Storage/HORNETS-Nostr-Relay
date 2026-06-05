package main

import "testing"

func TestSyncBootstrapAccessSettingsInviteOnlyForcesInviteOnlyReadAndWrite(t *testing.T) {
	relayConfig := map[string]interface{}{
		"allowed_users": map[string]interface{}{
			"mode":  "invite-only",
			"read":  "all_users",
			"write": "all_users",
		},
	}

	if err := syncBootstrapAccessSettings(relayConfig); err != nil {
		t.Fatalf("syncBootstrapAccessSettings returned error: %v", err)
	}

	allowedUsers, ok := relayConfig["allowed_users"].(map[string]interface{})
	if !ok {
		t.Fatal("expected allowed_users map to exist")
	}

	if got := allowedUsers["mode"]; got != "invite-only" {
		t.Fatalf("expected mode invite-only, got %#v", got)
	}
	if got := allowedUsers["read"]; got != "allowed_users" {
		t.Fatalf("expected read allowed_users, got %#v", got)
	}
	if got := allowedUsers["write"]; got != "allowed_users" {
		t.Fatalf("expected write allowed_users, got %#v", got)
	}
}

func TestSyncBootstrapAccessSettingsOnlyMeForcesPrivateReadAndWrite(t *testing.T) {
	relayConfig := map[string]interface{}{
		"allowed_users": map[string]interface{}{
			"mode":  "only-me",
			"read":  "all_users",
			"write": "allowed_users",
		},
	}

	if err := syncBootstrapAccessSettings(relayConfig); err != nil {
		t.Fatalf("syncBootstrapAccessSettings returned error: %v", err)
	}

	allowedUsers := relayConfig["allowed_users"].(map[string]interface{})
	if got := allowedUsers["read"]; got != "only-me" {
		t.Fatalf("expected read only-me, got %#v", got)
	}
	if got := allowedUsers["write"]; got != "only-me" {
		t.Fatalf("expected write only-me, got %#v", got)
	}
}
