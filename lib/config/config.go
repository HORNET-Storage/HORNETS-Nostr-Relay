package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strconv"
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
	viper.SetDefault("server.data_path", "./data")

	// External services defaults
	viper.SetDefault("external_services.ollama.url", "http://ollama:11434")
	viper.SetDefault("external_services.ollama.model", "gemma2:2b")
	viper.SetDefault("external_services.ollama.timeout", 10000)
	viper.SetDefault("external_services.moderator.url", "http://moderator:8000")
	viper.SetDefault("external_services.wallet.url", "http://localhost:9003")
	viper.SetDefault("external_services.wallet.key", "")
	viper.SetDefault("external_services.wallet.name", "default")

	// Logging defaults
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.output", "file")

	// Relay defaults
	viper.SetDefault("relay.name", "HORNETS")
	viper.SetDefault("relay.description", "HORNETS relay, the home of GitNestr")
	viper.SetDefault("relay.contact", "support@hornets.net")
	viper.SetDefault("relay.icon", "")
	viper.SetDefault("relay.software", "HORNETS")
	viper.SetDefault("relay.version", "0.0.1")
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
	viper.SetDefault("event_filtering.kind_whitelist", []string{"kind0", "kind1", "kind22242", "kind10010", "kind19841", "kind19842", "kind19843", "kind10002"})
	// Note: media_definitions defaults removed to prevent field name conflicts
	// The config.yaml file contains the complete media definitions
	viper.SetDefault("event_filtering.dynamic_kinds.enabled", false)
	viper.SetDefault("event_filtering.dynamic_kinds.allowed_kinds", []int{})
	viper.SetDefault("event_filtering.protocols.enabled", false)
	viper.SetDefault("event_filtering.protocols.allowed_protocols", []string{})

	// Allowed users defaults - free mode for rapid testing
	viper.SetDefault("allowed_users.mode", "public")
	viper.SetDefault("allowed_users.read_access.enabled", true)
	viper.SetDefault("allowed_users.read_access.scope", "all_users")
	viper.SetDefault("allowed_users.write_access.enabled", true)
	viper.SetDefault("allowed_users.write_access.scope", "all_users") // Free mode allows all users to write
	viper.SetDefault("allowed_users.last_updated", 0)

	// Default free tier with 100MB monthly storage (matches working config)
	viper.SetDefault("allowed_users.tiers", []map[string]interface{}{
		{
			"name":                "Basic",
			"price_sats":          0,
			"monthly_limit_bytes": 104857600, // 100MB (1.048576e+08)
			"unlimited":           false,
		},
	})

	// NIP mappings defaults - maps Nostr kinds to their corresponding NIP numbers
	viper.SetDefault("nip_mappings", map[string]string{
		// NIP-01: Basic Protocol
		"0": "1", // Profile metadata
		"1": "1", // Short text note
		"2": "1", // Recommend relay (deprecated)

		// NIP-02: Contact List
		"3": "2", // Contact list

		// NIP-09: Event Deletion
		"5": "9", // Deletion request

		// NIP-18: Reposts
		"6":  "18", // Repost
		"16": "18", // Generic repost

		// NIP-25: Reactions
		"7": "25", // Reaction

		// NIP-58: Badges
		"8":     "58", // Badge award
		"30008": "58", // Profile badge
		"30009": "58", // Badge definition

		// NIP-23: Long-form Content
		"30023": "23", // Long-form content

		// NIP-51: Lists
		"10000": "51", // Mute list
		"10001": "51", // Pin list
		"30000": "51", // Categorized people list

		// NIP-56: Reporting
		"1984": "56", // Reporting

		// NIP-57: Lightning Zaps
		"9735": "57", // Zap receipt

		// NIP-65: Relay List Metadata
		"10002": "65", // Relay list metadata

		// NIP-84: Highlights
		"9802": "84", // Highlight

		// NIP-116: Event Paths
		"30079": "116", // Event paths

		// NIP-117: Double Ratchet DM
		"1060": "117", // Message event

		// NIP-118: Double Ratchet DM Invite
		"30078": "118", // Invite event

		// Custom HORNETS NIPs
		"117":   "888", // Blossom blob
		"10411": "888", // Subscription info
		"11888": "888", // Custom HORNETS protocol
		"555":   "555", // X-Nostr bridge

		// Additional kinds
		"10010": "51",  // Additional list type
		"10011": "51",  // Additional list type
		"10022": "51",  // Additional list type
		"9803":  "84",  // Additional highlight type
		"22242": "888", // Custom HORNETS kind
		"19841": "888", // Payment subscription
		"19842": "888", // Payment subscription
		"19843": "888", // Payment subscription
	})
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

// GetNIPMappings returns the current NIP mappings from configuration
func GetNIPMappings() map[string]string {
	return viper.GetStringMapString("nip_mappings")
}

// UpdateNIPMapping updates or adds a single NIP mapping
func UpdateNIPMapping(kind, nip string) error {
	mappings := GetNIPMappings()
	if mappings == nil {
		mappings = make(map[string]string)
	}
	mappings[kind] = nip
	viper.Set("nip_mappings", mappings)
	return SaveConfig()
}

// RemoveNIPMapping removes a NIP mapping for a specific kind
func RemoveNIPMapping(kind string) error {
	mappings := GetNIPMappings()
	if mappings == nil {
		return nil // Nothing to remove
	}
	delete(mappings, kind)
	viper.Set("nip_mappings", mappings)
	return SaveConfig()
}


// GetNIPForKind returns the NIP number for a given kind by reading directly from config
func GetNIPForKind(kind int) (int, error) {
	kindStr := strconv.Itoa(kind)

	// Read mappings directly from config
	mappings := GetNIPMappings()
	if len(mappings) == 0 {
		return 0, fmt.Errorf("no NIP mappings found in configuration")
	}

	nipStr, exists := mappings[kindStr]
	if !exists {
		return 0, fmt.Errorf("no NIP mapping found for kind %d", kind)
	}

	nip, err := strconv.Atoi(nipStr)
	if err != nil {
		return 0, fmt.Errorf("invalid NIP number for kind %d: %v", kind, err)
	}

	return nip, nil
}

// GetSupportedNIPsFromKinds returns unique NIP numbers for given kinds
func GetSupportedNIPsFromKinds(kinds []string) ([]int, error) {
	nipSet := make(map[int]struct{})

	// Always include system-critical NIPs
	systemCriticalKinds := []int{555, 10411, 11888}
	for _, kind := range systemCriticalKinds {
		if nip, err := GetNIPForKind(kind); err == nil {
			nipSet[nip] = struct{}{}
		}
	}

	// Process user-configured kinds
	for _, kindStr := range kinds {
		// Remove "kind" prefix if present
		kindStr = strings.TrimPrefix(kindStr, "kind")

		kind, err := strconv.Atoi(kindStr)
		if err != nil {
			log.Printf("Warning: Invalid kind number '%s': %v", kindStr, err)
			continue
		}

		nip, err := GetNIPForKind(kind)
		if err != nil {
			log.Printf("Warning: No NIP mapping found for kind %d: %v", kind, err)
			continue
		}

		nipSet[nip] = struct{}{}
	}

	// Convert set to sorted slice
	nips := make([]int, 0, len(nipSet))
	for nip := range nipSet {
		nips = append(nips, nip)
	}
	sort.Ints(nips)

	return nips, nil
}

// AddKindToNIPMapping adds or updates a kind-to-NIP mapping
func AddKindToNIPMapping(kind int, nip int) error {
	kindStr := strconv.Itoa(kind)
	nipStr := strconv.Itoa(nip)

	err := UpdateNIPMapping(kindStr, nipStr)
	if err != nil {
		return fmt.Errorf("failed to add kind-to-NIP mapping: %v", err)
	}

	log.Printf("Added kind-to-NIP mapping: kind=%d, nip=%d", kind, nip)

	return nil
}
