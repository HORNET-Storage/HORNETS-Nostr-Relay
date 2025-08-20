package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"

	"github.com/HORNET-Storage/hornet-storage/lib/types"
)

var (
	// Cache the configuration after first load
	cachedConfig    atomic.Value // stores *types.Config
	configLoadOnce  sync.Once
	configLoadError error

	// Only protect write operations
	writeMutex sync.Mutex

	// Debounce timer for config file changes
	debounceTimer *time.Timer
	debounceMutex sync.Mutex
)

// InitConfig initializes the global viper configuration
func InitConfig() error {
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

	// Set defaults (will check internally if config exists)
	setDefaults()

	// Try to read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found, create it with defaults
			fmt.Println("No config.yaml found, creating default configuration...")
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
	} else {
		// Config file exists
		fmt.Println("Using existing config.yaml - preserving user configurations")
	}

	// Load initial configuration into cache
	if err := reloadConfigCache(); err != nil {
		return fmt.Errorf("failed to load initial config: %w", err)
	}

	// Watch for config file changes with debouncing
	viper.WatchConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		// Debounce file changes to avoid reading partial writes on slower machines
		debounceMutex.Lock()
		defer debounceMutex.Unlock()

		// Cancel any existing timer
		if debounceTimer != nil {
			debounceTimer.Stop()
		}

		// Set a new timer to reload config after 500ms of no changes
		debounceTimer = time.AfterFunc(500*time.Millisecond, func() {
			log.Printf("Config file changed (debounced): %s", e.Name)
			writeMutex.Lock()
			defer writeMutex.Unlock()

			if err := reloadConfigCache(); err != nil {
				log.Printf("Error reloading config cache after file change: %v", err)
			} else {
				log.Printf("Config cache refreshed after file change")
			}
		})
	})

	return nil
}

// reloadConfigCache loads the configuration from viper into the cache
func reloadConfigCache() error {
	config := &types.Config{}
	if err := viper.Unmarshal(config); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}
	cachedConfig.Store(config)
	return nil
}

// GetConfig returns the cached configuration struct
// This is extremely fast as it only reads from atomic.Value
func GetConfig() (*types.Config, error) {
	// Try to get cached config
	if cfg := cachedConfig.Load(); cfg != nil {
		return cfg.(*types.Config), nil
	}

	// If not loaded yet, load it once
	configLoadOnce.Do(func() {
		configLoadError = reloadConfigCache()
	})

	if configLoadError != nil {
		return nil, configLoadError
	}

	cfg := cachedConfig.Load()
	if cfg == nil {
		return nil, fmt.Errorf("configuration not loaded")
	}

	return cfg.(*types.Config), nil
}

// GetPort returns the calculated port for a service
func GetPort(service string) int {
	cfg, err := GetConfig()
	if err != nil || cfg.Server.Port == 0 {
		return 9000 // fallback
	}

	basePort := cfg.Server.Port
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
	cfg, err := GetConfig()
	if err != nil {
		return false
	}

	switch feature {
	case "demo":
		return cfg.Server.Demo
	case "web":
		return cfg.Server.Web
	case "nostr":
		return cfg.Server.Nostr
	case "hornets":
		return cfg.Server.Hornets
	default:
		// For other features, we need to check viper
		// This is rare, so the lock is acceptable
		writeMutex.Lock()
		defer writeMutex.Unlock()
		return viper.GetBool(feature)
	}
}

// GetDataDir returns the data directory path
func GetDataDir() string {
	cfg, err := GetConfig()
	if err != nil || cfg.Server.DataPath == "" {
		return "./data" // fallback
	}
	return cfg.Server.DataPath
}

// GetPath returns a path relative to the data directory
func GetPath(subPath string) string {
	return filepath.Join(GetDataDir(), subPath)
}

// SaveConfig saves the current configuration to file
func SaveConfig() error {
	writeMutex.Lock()
	defer writeMutex.Unlock()

	err := viper.WriteConfig()
	if err != nil {
		return err
	}

	// Reload cache after save
	return reloadConfigCache()
}

// UpdateConfig updates a configuration value and optionally saves it
// Now with change detection to avoid unnecessary writes
func UpdateConfig(key string, value interface{}, save bool) error {
	writeMutex.Lock()
	defer writeMutex.Unlock()

	// Check if the value actually changed
	currentValue := viper.Get(key)
	if isConfigValueEqual(currentValue, value) {
		log.Printf("No change for %s, skipping update", key)
		return nil
	}

	log.Printf("Updating %s: %v -> %v", key, currentValue, value)
	viper.Set(key, value)

	if save {
		if err := viper.WriteConfig(); err != nil {
			return err
		}
	}

	// Reload cache after update
	return reloadConfigCache()
}

// UpdateMultipleSections updates multiple configuration sections intelligently
// Only saves once at the end, and only if there were changes
func UpdateMultipleSections(settings map[string]interface{}) error {
	writeMutex.Lock()
	defer writeMutex.Unlock()

	hasChanges := false

	// Process each section
	for sectionName, sectionValue := range settings {
		sectionMap, ok := sectionValue.(map[string]interface{})
		if !ok {
			// Handle simple top-level values
			currentValue := viper.Get(sectionName)
			if !isConfigValueEqual(currentValue, sectionValue) {
				log.Printf("Updating %s: %v -> %v", sectionName, currentValue, sectionValue)
				viper.Set(sectionName, sectionValue)
				hasChanges = true
			}
			continue
		}

		// Process section fields
		log.Printf("Processing section: %s", sectionName)
		for fieldName, fieldValue := range sectionMap {
			fullKey := sectionName + "." + fieldName

			// Handle nested sections
			if nestedMap, ok := fieldValue.(map[string]interface{}); ok {
				for nestedField, nestedValue := range nestedMap {
					nestedKey := fullKey + "." + nestedField
					currentValue := viper.Get(nestedKey)
					if !isConfigValueEqual(currentValue, nestedValue) {
						log.Printf("  Updating %s: %v -> %v", nestedKey, currentValue, nestedValue)
						viper.Set(nestedKey, nestedValue)
						hasChanges = true
					}
				}
			} else {
				currentValue := viper.Get(fullKey)
				if !isConfigValueEqual(currentValue, fieldValue) {
					log.Printf("  Updating %s: %v -> %v", fullKey, currentValue, fieldValue)
					viper.Set(fullKey, fieldValue)
					hasChanges = true
				}
			}
		}
	}

	// Only save if there were changes
	if hasChanges {
		log.Println("Saving configuration changes...")
		if err := viper.WriteConfig(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		// Reload cache after save
		if err := reloadConfigCache(); err != nil {
			return fmt.Errorf("failed to reload cache: %w", err)
		}

		log.Println("Configuration saved successfully")
	} else {
		log.Println("No configuration changes detected, skipping save")
	}

	return nil
}

// isConfigValueEqual compares two configuration values for equality
// Handles type normalization for JSON numbers (float64 vs int)
func isConfigValueEqual(a, b interface{}) bool {
	// Handle nil cases
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Normalize and compare
	normalizedA := normalizeConfigValue(a)
	normalizedB := normalizeConfigValue(b)

	// Use string comparison as a simple but effective equality check
	return fmt.Sprintf("%v", normalizedA) == fmt.Sprintf("%v", normalizedB)
}

// normalizeConfigValue ensures consistent types for comparison
func normalizeConfigValue(v interface{}) interface{} {
	switch val := v.(type) {
	case float64:
		// JSON numbers come as float64, convert to int if whole number
		if val == float64(int(val)) {
			return int(val)
		}
		return val
	case []interface{}:
		// Normalize array elements
		normalized := make([]interface{}, len(val))
		for i, item := range val {
			normalized[i] = normalizeConfigValue(item)
		}
		return normalized
	case map[string]interface{}:
		// Normalize map values
		normalized := make(map[string]interface{})
		for k, v := range val {
			normalized[k] = normalizeConfigValue(v)
		}
		return normalized
	default:
		return v
	}
}

// RefreshConfig forces a reload of the configuration cache
// This should be called after external changes to the configuration (e.g., via web UI)
func RefreshConfig() error {
	writeMutex.Lock()
	defer writeMutex.Unlock()

	return reloadConfigCache()
}

// GetAllowedUsersSettings returns the allowed users settings from cached config
func GetAllowedUsersSettings() (*types.AllowedUsersSettings, error) {
	cfg, err := GetConfig()
	if err != nil {
		return nil, err
	}
	return &cfg.AllowedUsersSettings, nil
}

// GetExternalURL returns external service URLs
func GetExternalURL(service string) string {
	cfg, err := GetConfig()
	if err != nil {
		return ""
	}

	switch service {
	case "ollama":
		return cfg.ExternalServices.Ollama.URL
	case "moderator":
		return cfg.ExternalServices.Moderator.URL
	case "wallet":
		return cfg.ExternalServices.Wallet.URL
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
	// NIP mappings are stored separately in viper, not in the Config struct
	// For read operations, we can safely read from viper with minimal locking
	writeMutex.Lock()
	defer writeMutex.Unlock()

	mappings := viper.GetStringMapString("nip_mappings")
	if mappings == nil {
		return make(map[string]string)
	}
	return mappings
}

// UpdateNIPMapping updates or adds a single NIP mapping
func UpdateNIPMapping(kind, nip string) error {
	writeMutex.Lock()
	defer writeMutex.Unlock()

	mappings := viper.GetStringMapString("nip_mappings")
	if mappings == nil {
		mappings = make(map[string]string)
	}
	mappings[kind] = nip
	viper.Set("nip_mappings", mappings)

	if err := viper.WriteConfig(); err != nil {
		return err
	}

	// Reload cache after update
	return reloadConfigCache()
}

// RemoveNIPMapping removes a NIP mapping for a specific kind
func RemoveNIPMapping(kind string) error {
	writeMutex.Lock()
	defer writeMutex.Unlock()

	mappings := viper.GetStringMapString("nip_mappings")
	if mappings == nil {
		return nil // Nothing to remove
	}
	delete(mappings, kind)
	viper.Set("nip_mappings", mappings)

	if err := viper.WriteConfig(); err != nil {
		return err
	}

	// Reload cache after update
	return reloadConfigCache()
}

// GetNIPForKind returns the NIP number for a given kind
func GetNIPForKind(kind int) (int, error) {
	kindStr := strconv.Itoa(kind)
	mappings := GetNIPMappings()

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

// setDefaults sets all default values only if config doesn't exist
func setDefaults() {
	// Simply check if config.yaml exists in project root
	if _, err := os.Stat("config.yaml"); err == nil {
		// Config exists, don't set defaults
		fmt.Println("Config file exists, skipping defaults to preserve user settings")
		return
	}

	fmt.Println("No existing config found, setting defaults for new installation")

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
	viper.SetDefault("relay.icon", "http://localhost:9002/logo-dark-192.png")
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
	viper.SetDefault("content_filtering.image_moderation.timeout_seconds", 600)
	viper.SetDefault("content_filtering.image_moderation.check_interval_seconds", 30)
	viper.SetDefault("content_filtering.image_moderation.concurrency", 5)

	// Event filtering defaults
	viper.SetDefault("event_filtering.allow_unregistered_kinds", false) // Default to false for security
	viper.SetDefault("event_filtering.registered_kinds", []int{
		0, 1, 3, 5, 6, 7, 8, // Basic kinds (NO kind 2, 4, or 16 handlers in main.go)
		117, 1063, 1808, 1809, 1984, // Special kinds (NO 1060 handler)
		9372, 9373, 9735, 9802, // Payment/Zap kinds (NO 9803 handler)
		10000, 10001, 10002, 10010, // List kinds
		10411,        // Relay info kind (NO 10011 or 10022 handlers)
		11011,        // Relay list kind
		16629, 16630, // Ephemeral kinds
		19841, 19842, 19843, // Subscription kinds
		22242,               // Auth kind
		30000, 30008, 30009, // Parameterized replaceable kinds
		30023, 30078, 30079, // Long-form content kinds
	})
	viper.SetDefault("event_filtering.moderation_mode", "strict")
	viper.SetDefault("event_filtering.kind_whitelist", []string{"kind0", "kind1", "kind22242", "kind10010", "kind19841", "kind19842", "kind19843", "kind10002", "kind1808", "kind1809"})
	viper.SetDefault("event_filtering.dynamic_kinds.enabled", false)
	viper.SetDefault("event_filtering.dynamic_kinds.allowed_kinds", []int{})
	viper.SetDefault("event_filtering.protocols.enabled", false)
	viper.SetDefault("event_filtering.protocols.allowed_protocols", []string{})

	// Allowed users defaults - free mode for rapid testing
	viper.SetDefault("allowed_users.mode", "public")
	viper.SetDefault("allowed_users.read_access.enabled", true)
	viper.SetDefault("allowed_users.read_access.scope", "all_users")
	viper.SetDefault("allowed_users.write_access.enabled", true)
	viper.SetDefault("allowed_users.write_access.scope", "all_users")
	viper.SetDefault("allowed_users.last_updated", 0)

	// Default free tier with 100MB monthly storage
	viper.SetDefault("allowed_users.tiers", []map[string]interface{}{
		{
			"name":                "Basic",
			"price_sats":          0,
			"monthly_limit_bytes": 104857600, // 100MB
			"unlimited":           false,
		},
	})

	// NIP mappings defaults
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

		// NIP-78: Application-specific Data
		"30078": "78", // Application-specific Data

		// NIP-94: File Metadata
		"1063": "94", // File metadata

		// Custom HORNETS NIPs
		"117":   "888", // Blossom blob
		"10411": "888", // Subscription info
		"11888": "888", // Custom HORNETS protocol
		"11011": "888", // Custom HORNETS
		"555":   "555", // X-Nostr bridge

		// Custom kinds
		"9372":  "888", // Custom application
		"9373":  "888", // Custom application
		"16629": "888", // Custom HORNETS
		"16630": "888", // Custom HORNETS

		// Additional kinds
		"10010": "51",  // Additional list type
		"10011": "51",  // Additional list type
		"10022": "51",  // Additional list type
		"9803":  "84",  // Additional highlight type
		"22242": "42",  // Client Authentication
		"19841": "888", // Payment subscription
		"19842": "888", // Payment subscription
		"19843": "888", // Payment subscription
	})

	// Push notification defaults
	viper.SetDefault("push_notifications.enabled", false)

	// APNs Configuration defaults
	viper.SetDefault("push_notifications.apns.enabled", false)
	viper.SetDefault("push_notifications.apns.key_id", "")
	viper.SetDefault("push_notifications.apns.team_id", "")
	viper.SetDefault("push_notifications.apns.bundle_id", "")
	viper.SetDefault("push_notifications.apns.key_path", "")
	viper.SetDefault("push_notifications.apns.production", false)

	// FCM Configuration defaults
	viper.SetDefault("push_notifications.fcm.enabled", false)
	viper.SetDefault("push_notifications.fcm.project_id", "")
	viper.SetDefault("push_notifications.fcm.credentials_path", "")

	// Service Configuration defaults
	viper.SetDefault("push_notifications.service.queue_size", 1000)
	viper.SetDefault("push_notifications.service.worker_count", 10)
	viper.SetDefault("push_notifications.service.batch_size", 100)
	viper.SetDefault("push_notifications.service.retry_attempts", 3)
	viper.SetDefault("push_notifications.service.retry_delay", "5s")
}

// GetAllSettingsAsMap returns all configuration settings as a map
// This is a thread-safe alternative to viper.AllSettings() which can cause
// concurrent map read/write errors when viper.WatchConfig() is enabled
func GetAllSettingsAsMap() (map[string]interface{}, error) {
	// Get the cached config struct
	cfg, err := GetConfig()
	if err != nil {
		return nil, err
	}

	// Manually build the settings map using the original viper key structure
	// This ensures compatibility with existing code that expects specific key names
	settings := make(map[string]interface{})

	// Server settings
	settings["server"] = map[string]interface{}{
		"port":         cfg.Server.Port,
		"bind_address": cfg.Server.BindAddress,
		"upnp":         cfg.Server.UPNP,
		"nostr":        cfg.Server.Nostr,
		"hornets":      cfg.Server.Hornets,
		"web":          cfg.Server.Web,
		"demo":         cfg.Server.Demo,
		"data_path":    cfg.Server.DataPath,
	}

	// External services settings
	settings["external_services"] = map[string]interface{}{
		"ollama": map[string]interface{}{
			"url":     cfg.ExternalServices.Ollama.URL,
			"model":   cfg.ExternalServices.Ollama.Model,
			"timeout": cfg.ExternalServices.Ollama.Timeout,
		},
		"moderator": map[string]interface{}{
			"url": cfg.ExternalServices.Moderator.URL,
		},
		"wallet": map[string]interface{}{
			"url":  cfg.ExternalServices.Wallet.URL,
			"key":  cfg.ExternalServices.Wallet.Key,
			"name": cfg.ExternalServices.Wallet.Name,
		},
	}

	// Logging settings
	settings["logging"] = map[string]interface{}{
		"level":  cfg.Logging.Level,
		"format": cfg.Logging.Format,
		"output": cfg.Logging.Output,
		"path":   cfg.Logging.Path,
	}

	// Relay settings
	settings["relay"] = map[string]interface{}{
		"name":           cfg.Relay.Name,
		"description":    cfg.Relay.Description,
		"contact":        cfg.Relay.Contact,
		"icon":           cfg.Relay.Icon,
		"software":       cfg.Relay.Software,
		"version":        cfg.Relay.Version,
		"service_tag":    cfg.Relay.ServiceTag,
		"supported_nips": cfg.Relay.SupportedNIPs,
		"secret_key":     cfg.Relay.SecretKey,
		"private_key":    cfg.Relay.PrivateKey,
		"dht_key":        cfg.Relay.DHTKey,
	}

	// Content filtering settings
	settings["content_filtering"] = map[string]interface{}{
		"text_filter": map[string]interface{}{
			"enabled":                cfg.ContentFiltering.TextFilter.Enabled,
			"cache_size":             cfg.ContentFiltering.TextFilter.CacheSize,
			"cache_ttl_seconds":      cfg.ContentFiltering.TextFilter.CacheTTLSeconds,
			"full_text_search_kinds": cfg.ContentFiltering.TextFilter.FullTextSearchKinds,
		},
		"image_moderation": map[string]interface{}{
			"enabled":                cfg.ContentFiltering.ImageModeration.Enabled,
			"mode":                   cfg.ContentFiltering.ImageModeration.Mode,
			"threshold":              cfg.ContentFiltering.ImageModeration.Threshold,
			"timeout_seconds":        cfg.ContentFiltering.ImageModeration.TimeoutSeconds,
			"check_interval_seconds": cfg.ContentFiltering.ImageModeration.CheckIntervalSeconds,
			"concurrency":            cfg.ContentFiltering.ImageModeration.Concurrency,
		},
	}

	// Event filtering settings
	settings["event_filtering"] = map[string]interface{}{
		"allow_unregistered_kinds": cfg.EventFiltering.AllowUnregisteredKinds,
		"registered_kinds":         cfg.EventFiltering.RegisteredKinds,
		"moderation_mode":          cfg.EventFiltering.ModerationMode,
		"kind_whitelist":           cfg.EventFiltering.KindWhitelist,
		"media_definitions":        cfg.EventFiltering.MediaDefinitions,
		"dynamic_kinds": map[string]interface{}{
			"enabled":       cfg.EventFiltering.DynamicKinds.Enabled,
			"allowed_kinds": cfg.EventFiltering.DynamicKinds.AllowedKinds,
		},
		"protocols": map[string]interface{}{
			"enabled":           cfg.EventFiltering.Protocols.Enabled,
			"allowed_protocols": cfg.EventFiltering.Protocols.AllowedProtocols,
		},
	}

	// Allowed users settings
	settings["allowed_users"] = map[string]interface{}{
		"mode":         cfg.AllowedUsersSettings.Mode,
		"read":         cfg.AllowedUsersSettings.Read,
		"write":        cfg.AllowedUsersSettings.Write,
		"tiers":        cfg.AllowedUsersSettings.Tiers,
		"last_updated": cfg.AllowedUsersSettings.LastUpdated,
	}

	// Push notifications settings
	settings["push_notifications"] = map[string]interface{}{
		"enabled": cfg.PushNotifications.Enabled,
		"apns": map[string]interface{}{
			"enabled":    cfg.PushNotifications.APNS.Enabled,
			"key_id":     cfg.PushNotifications.APNS.KeyID,
			"team_id":    cfg.PushNotifications.APNS.TeamID,
			"bundle_id":  cfg.PushNotifications.APNS.BundleID,
			"key_path":   cfg.PushNotifications.APNS.KeyPath,
			"production": cfg.PushNotifications.APNS.Production,
		},
		"fcm": map[string]interface{}{
			"enabled":          cfg.PushNotifications.FCM.Enabled,
			"project_id":       cfg.PushNotifications.FCM.ProjectID,
			"credentials_path": cfg.PushNotifications.FCM.CredentialsPath,
		},
		"service": map[string]interface{}{
			"queue_size":     cfg.PushNotifications.Service.QueueSize,
			"worker_count":   cfg.PushNotifications.Service.WorkerCount,
			"batch_size":     cfg.PushNotifications.Service.BatchSize,
			"retry_attempts": cfg.PushNotifications.Service.RetryAttempts,
			"retry_delay":    cfg.PushNotifications.Service.RetryDelay,
		},
	}

	// Add NIP mappings separately as they're not in the Config struct
	settings["nip_mappings"] = GetNIPMappings()

	// Note: There is no "subscriptions" section in the config - this was likely an old/unused key
	// that the frontend was checking for but doesn't exist in the actual configuration

	return settings, nil
}

// GetSettingValue returns a specific setting value by key path (e.g., "relay.name")
// This is a thread-safe alternative to viper.Get()
func GetSettingValue(key string) (interface{}, error) {
	settings, err := GetAllSettingsAsMap()
	if err != nil {
		return nil, err
	}

	// Navigate through nested keys
	keys := strings.Split(key, ".")
	var value interface{} = settings

	for _, k := range keys {
		if m, ok := value.(map[string]interface{}); ok {
			value = m[k]
		} else {
			return nil, fmt.Errorf("key not found: %s", key)
		}
	}

	return value, nil
}
