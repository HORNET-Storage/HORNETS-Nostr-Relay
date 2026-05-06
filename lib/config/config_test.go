package config

import (
	"testing"

	"github.com/spf13/viper"
)

func TestNormalizeAllowedUsersConfigValuesFromLegacyNestedScope(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("allowed_users.mode", "public")
	viper.Set("allowed_users.read_access.scope", "all_users")
	viper.Set("allowed_users.write_access.scope", "paid_users")

	normalizeAllowedUsersConfigValues()

	if got := viper.GetString("allowed_users.read"); got != "all_users" {
		t.Fatalf("expected allowed_users.read to be normalized to all_users, got %q", got)
	}

	if got := viper.GetString("allowed_users.write"); got != "paid_users" {
		t.Fatalf("expected allowed_users.write to be normalized to paid_users, got %q", got)
	}
}

func TestNormalizeAllowedUsersConfigValuesKeepsFlatValues(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("allowed_users.read", "allowed_users")
	viper.Set("allowed_users.write", "only_me")
	viper.Set("allowed_users.read_access.scope", "all_users")
	viper.Set("allowed_users.write_access.scope", "paid_users")

	normalizeAllowedUsersConfigValues()

	if got := viper.GetString("allowed_users.read"); got != "allowed_users" {
		t.Fatalf("expected flat allowed_users.read to be preserved, got %q", got)
	}

	if got := viper.GetString("allowed_users.write"); got != "only_me" {
		t.Fatalf("expected flat allowed_users.write to be preserved, got %q", got)
	}
}

func TestNormalizeRelayDHTConfigValuesMigratesLegacyKey(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("relay.dht_key", "legacy-seed")

	normalizeRelayDHTConfigValues()

	if got := viper.GetString("relay.dht_seed"); got != "legacy-seed" {
		t.Fatalf("expected relay.dht_seed to be normalized from relay.dht_key, got %q", got)
	}
}

func TestNormalizeRelayDHTConfigValuesKeepsNewSeed(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("relay.dht_seed", "new-seed")
	viper.Set("relay.dht_key", "legacy-seed")

	normalizeRelayDHTConfigValues()

	if got := viper.GetString("relay.dht_seed"); got != "new-seed" {
		t.Fatalf("expected relay.dht_seed to be preserved, got %q", got)
	}
}
