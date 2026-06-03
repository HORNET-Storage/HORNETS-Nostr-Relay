package testing

import (
	"strings"
	"testing"

	"github.com/HORNET-Storage/hornet-storage/lib/access"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
	"github.com/HORNET-Storage/hornet-storage/testing/helpers"
)

// =============================================================================
// Caching Behaviour
// =============================================================================

// TestAccessControlCaching verifies that the access control cache works
// and that UpdateSettings invalidates it properly.
func TestAccessControlCaching(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	settings := &types.AllowedUsersSettings{
		Mode:  "public",
		Read:  "all_users",
		Write: "all_users",
	}

	ac := access.NewAccessControl(store.GetStatsStore(), settings)

	// all_users → should always return nil (allowed)
	err := ac.CanRead("anypubkey")
	if err != nil {
		t.Fatalf("Expected nil error for all_users read, got: %v", err)
	}

	// Switch to invite-only → should fail for unknown key
	settings2 := &types.AllowedUsersSettings{
		Mode:  "invite-only",
		Read:  "allowed_users",
		Write: "allowed_users",
	}
	ac.UpdateSettings(settings2) // this should invalidate cache

	// A random pubkey should now be denied
	kp, _ := helpers.GenerateKeyPair()
	err = ac.CanRead(kp.PublicKey)
	if err == nil {
		t.Errorf("Expected access denied for unknown pubkey in invite-only mode")
	}
}

// TestAccessControlCacheTTL verifies that cached results are reused within TTL
// by confirming that two rapid identical calls produce the same outcome.
func TestAccessControlCacheTTL(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	settings := &types.AllowedUsersSettings{
		Mode:  "invite-only",
		Read:  "allowed_users",
		Write: "allowed_users",
	}
	ac := access.NewAccessControl(store.GetStatsStore(), settings)

	kp, _ := helpers.GenerateKeyPair()

	// First call — should be denied (not in allowed users)
	err1 := ac.CanRead(kp.PublicKey)
	// Second call — should hit cache, same result
	err2 := ac.CanRead(kp.PublicKey)

	if (err1 == nil) != (err2 == nil) {
		t.Errorf("Cache inconsistency: first call err=%v, second call err=%v", err1, err2)
	}
}

// =============================================================================
// Hex Pubkey Validation
// =============================================================================

// TestAccessControl_RejectsInvalidPubkey verifies that the access control
// system rejects non-hex, wrong-length, and bech32 public keys with the
// expected "invalid public key format" error.
func TestAccessControl_RejectsInvalidPubkey(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	settings := &types.AllowedUsersSettings{
		Mode:  "invite-only",
		Read:  "allowed_users",
		Write: "allowed_users",
	}
	ac := access.NewAccessControl(store.GetStatsStore(), settings)

	badKeys := []struct {
		name string
		key  string
	}{
		{"too short", "abcdef"},
		{"too long", "aabbccddee00112233445566778899aabbccddee00112233445566778899aabb00"},
		{"non-hex chars", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"},
		{"bech32 npub", "npub1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq3cj6un"},
		{"empty string", ""},
		{"63 chars", "aabbccddee00112233445566778899aabbccddee001122334455667788990"},
		{"65 chars", "aabbccddee00112233445566778899aabbccddee00112233445566778899aab00"},
	}

	for _, tc := range badKeys {
		t.Run(tc.name, func(t *testing.T) {
			err := ac.CanRead(tc.key)
			if err == nil {
				t.Errorf("Expected error for key %q, got nil", tc.key)
				return
			}
			if !strings.Contains(err.Error(), "invalid public key format") {
				t.Errorf("Expected 'invalid public key format' error, got: %v", err)
			}
		})
	}
}

// TestAccessControl_AcceptsMixedCaseHex verifies that valid 64-char hex
// pubkeys with mixed case pass format validation. The key may still be
// denied by the allowed-users check, but the error should NOT be about
// invalid format.
func TestAccessControl_AcceptsMixedCaseHex(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	settings := &types.AllowedUsersSettings{
		Mode:  "invite-only",
		Read:  "allowed_users",
		Write: "allowed_users",
	}
	ac := access.NewAccessControl(store.GetStatsStore(), settings)

	// Mixed case hex — should pass format validation
	mixedCaseKey := "AABBCCDDEE00112233445566778899aaBBCCDDEE00112233445566778899AABB"
	err := ac.CanRead(mixedCaseKey)
	if err != nil && strings.Contains(err.Error(), "invalid public key format") {
		t.Errorf("Valid mixed-case hex key was rejected as invalid format")
	}
}

// TestAccessControl_AcceptsValidHexPubkey verifies that a real generated
// pubkey passes format validation. It will still be denied by the
// allowed-users check — the error must not be "invalid public key format".
func TestAccessControl_AcceptsValidHexPubkey(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	settings := &types.AllowedUsersSettings{
		Mode:  "invite-only",
		Read:  "allowed_users",
		Write: "allowed_users",
	}
	ac := access.NewAccessControl(store.GetStatsStore(), settings)

	kp, _ := helpers.GenerateKeyPair()

	// This should fail on "user not found" — NOT "invalid pubkey format"
	err := ac.CanRead(kp.PublicKey)
	if err == nil {
		t.Fatal("Expected error (not in allowed list)")
	}
	if strings.Contains(err.Error(), "invalid public key format") {
		t.Errorf("Valid hex key was rejected as invalid format")
	}
}

func TestAccessControl_OnlyMeAllowsOwnerOnly(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	owner, _ := helpers.GenerateKeyPair()
	allowedUser, _ := helpers.GenerateKeyPair()

	statsStore := store.GetStatsStore()
	if err := statsStore.SetRelayOwner(owner.PublicKey, "test"); err != nil {
		t.Fatalf("failed to set relay owner: %v", err)
	}
	if err := statsStore.AddAllowedUser(allowedUser.PublicKey, true, "", "test"); err != nil {
		t.Fatalf("failed to add allowed user: %v", err)
	}

	settings := &types.AllowedUsersSettings{
		Mode:  "only-me",
		Read:  "only-me",
		Write: "only-me",
	}
	ac := access.NewAccessControl(statsStore, settings)

	if err := ac.CanWrite(owner.PublicKey); err != nil {
		t.Fatalf("expected relay owner to write in only-me mode, got: %v", err)
	}
	if err := ac.CanWrite(allowedUser.PublicKey); err == nil {
		t.Fatal("expected allowed_users row to be ignored for only-me write access")
	}
	if err := ac.CanRead(allowedUser.PublicKey); err == nil {
		t.Fatal("expected allowed_users row to be ignored for only-me read access")
	}
}

func TestAccessControl_NormalizesLegacyOnlyMePermission(t *testing.T) {
	settings := &types.AllowedUsersSettings{
		Mode:  "only_me",
		Read:  "only_me",
		Write: "only_me",
	}
	ac := access.NewAccessControl(nil, settings)

	if err := ac.ValidateSettings(settings); err != nil {
		t.Fatalf("expected legacy only_me settings to validate, got: %v", err)
	}

	if settings.Mode != "only-me" || settings.Read != "only-me" || settings.Write != "only-me" {
		t.Fatalf("expected only_me to normalize to only-me, got mode=%q read=%q write=%q", settings.Mode, settings.Read, settings.Write)
	}
}
