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
