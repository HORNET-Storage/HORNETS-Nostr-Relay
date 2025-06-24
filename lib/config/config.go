package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"

	"github.com/HORNET-Storage/hornet-storage/lib/types"
)

// InitConfig initializes the global viper configuration
func InitConfig() error {
	// Set defaults
	setDefaults()

	// Configuration file settings
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/app")
	viper.AddConfigPath("./config")

	// Environment variable settings
	viper.SetEnvPrefix("HORNETS")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Try to read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found, create default
			if err := viper.WriteConfigAs("config.yaml"); err != nil {
				return fmt.Errorf("failed to create default config: %w", err)
			}
			fmt.Println("Created default config.yaml")
			// Try to read it again
			if err := viper.ReadInConfig(); err != nil {
				return fmt.Errorf("failed to read created config: %w", err)
			}
		} else {
			return fmt.Errorf("error reading config file: %w", err)
		}
	}

	return nil
}

// setDefaults sets all default values
func setDefaults() {
	// Server defaults
	viper.SetDefault("server.port", 9000)
	viper.SetDefault("server.bind_address", "0.0.0.0")
	viper.SetDefault("server.upnp", false)
	viper.SetDefault("server.nostr", true)
	viper.SetDefault("server.hornets", true)
	viper.SetDefault("server.web", true)
	viper.SetDefault("server.demo", false)
	viper.SetDefault("server.data_path", "/data")
	viper.SetDefault("server.stats_db", "statistics.db")

	// External services defaults
	viper.SetDefault("external_services.ollama.url", "http://ollama:11434")
	viper.SetDefault("external_services.ollama.model", "gemma2:2b")
	viper.SetDefault("external_services.ollama.timeout", 10000)
	viper.SetDefault("external_services.moderator.url", "http://moderator:8000")
	viper.SetDefault("external_services.wallet.url", "http://wallet:9003")
	viper.SetDefault("external_services.wallet.key", "")
	viper.SetDefault("external_services.wallet.name", "default")

	// Logging defaults
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.output", "both")
	viper.SetDefault("logging.path", "/logs")

	// Relay defaults
	viper.SetDefault("relay.name", "HORNETS")
	viper.SetDefault("relay.description", "The best relay ever.")
	viper.SetDefault("relay.contact", "support@hornets.net")
	viper.SetDefault("relay.software", "golang")
	viper.SetDefault("relay.version", "0.1.0")
	viper.SetDefault("relay.service_tag", "hornet-storage-service")
	viper.SetDefault("relay.supported_nips", []int{1, 2, 9, 11, 18, 23, 24, 25, 42, 45, 50, 51, 56, 57, 65, 116, 555, 888})
	viper.SetDefault("relay.secret_key", "hornets-secret-key")
	viper.SetDefault("relay.private_key", "")
	viper.SetDefault("relay.public_key", "")
	viper.SetDefault("relay.dht_key", "")

	// Content filtering defaults
	viper.SetDefault("content_filtering.text_filter.enabled", true)
	viper.SetDefault("content_filtering.text_filter.cache_size", 10000)
	viper.SetDefault("content_filtering.text_filter.cache_ttl_seconds", 60)
	viper.SetDefault("content_filtering.text_filter.full_text_search_kinds", []int{1})

	viper.SetDefault("content_filtering.image_moderation.enabled", true)
	viper.SetDefault("content_filtering.image_moderation.mode", "full")
	viper.SetDefault("content_filtering.image_moderation.threshold", 0.4)
	viper.SetDefault("content_filtering.image_moderation.timeout_seconds", 60)
	viper.SetDefault("content_filtering.image_moderation.check_interval_seconds", 30)
	viper.SetDefault("content_filtering.image_moderation.concurrency", 5)

	// Event filtering defaults
	viper.SetDefault("event_filtering.mode", "whitelist")
	viper.SetDefault("event_filtering.moderation_mode", "strict")
	viper.SetDefault("event_filtering.kind_whitelist", []string{"kind0", "kind1", "kind22242", "kind10010", "kind19841", "kind19842", "kind19843"})
	// Note: media_definitions defaults removed to prevent field name conflicts
	// The config.yaml file contains the complete media definitions
	viper.SetDefault("event_filtering.dynamic_kinds.enabled", false)
	viper.SetDefault("event_filtering.dynamic_kinds.allowed_kinds", []int{})
	viper.SetDefault("event_filtering.protocols.enabled", false)
	viper.SetDefault("event_filtering.protocols.allowed_protocols", []string{})

	// Allowed users defaults
	viper.SetDefault("allowed_users.mode", "free")
	viper.SetDefault("allowed_users.read_access.enabled", true)
	viper.SetDefault("allowed_users.read_access.scope", "all_users")
	viper.SetDefault("allowed_users.write_access.enabled", true)
}

// GetConfig returns the configuration struct marshaled from viper
func GetConfig() (*types.Config, error) {
	config := &types.Config{}
	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return config, nil
}

// GetPort returns the calculated port for a service
func GetPort(service string) int {
	basePort := viper.GetInt("server.port")
	if basePort == 0 {
		basePort = 9000
	}

	switch service {
	case "hornets":
		return basePort
	case "nostr":
		return basePort + 1
	case "web":
		return basePort + 2
	default:
		return basePort
	}
}

// IsEnabled checks if a service/feature is enabled
func IsEnabled(feature string) bool {
	switch feature {
	case "demo":
		return viper.GetBool("server.demo")
	case "web":
		return viper.GetBool("server.web")
	case "nostr":
		return viper.GetBool("server.nostr")
	case "hornets":
		return viper.GetBool("server.hornets")
	default:
		return viper.GetBool(feature)
	}
}

// GetDataDir returns the data directory path
func GetDataDir() string {
	return viper.GetString("server.data_path")
}

// GetPath returns a path relative to the data directory
func GetPath(subPath string) string {
	return filepath.Join(GetDataDir(), subPath)
}

// SaveConfig saves the current configuration to file
func SaveConfig() error {
	return viper.WriteConfig()
}

// UpdateConfig updates a configuration value and optionally saves it
func UpdateConfig(key string, value interface{}, save bool) error {
	viper.Set(key, value)
	if save {
		return SaveConfig()
	}
	return nil
}

// GetExternalURL returns external service URLs
func GetExternalURL(service string) string {
	switch service {
	case "ollama":
		return viper.GetString("external_services.ollama.url")
	case "moderator":
		return viper.GetString("external_services.moderator.url")
	case "wallet":
		return viper.GetString("external_services.wallet.url")
	default:
		return ""
	}
}

// GenerateRandomAPIKey generates a random 32-byte hexadecimal key
func GenerateRandomAPIKey() (string, error) {
	bytes := make([]byte, 32) // 32 bytes = 256 bits
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
