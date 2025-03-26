package contentmoderation

import (
	"log"
	"time"

	"github.com/spf13/viper"
)

// Config defines the configuration options for the content moderation system
type Config struct {
	// API settings
	APIEndpoint string        `json:"api_endpoint"` // URL for the moderation API
	APITimeout  time.Duration `json:"api_timeout"`  // Timeout for API requests

	// Processing settings
	NumWorkers   int           `json:"num_workers"`   // Number of concurrent worker goroutines
	PollInterval time.Duration `json:"poll_interval"` // How often workers check for new content
	MaxAttempts  int           `json:"max_attempts"`  // Maximum number of attempts for processing

	// Storage settings
	TempStoragePath string        `json:"temp_storage_path"` // Where to temporarily store media
	RetentionPeriod time.Duration `json:"retention_period"`  // How long to keep rejected content

	// Cache settings
	CacheSize int           `json:"cache_size"` // Size of various caches
	CacheTTL  time.Duration `json:"cache_ttl"`  // Time-to-live for cached items

	// Policy settings
	DefaultMode        string             `json:"default_mode"`        // Default moderation mode (basic, strict, full)
	ThresholdOverrides map[string]float64 `json:"threshold_overrides"` // Custom thresholds for specific content types

	// External media settings
	CheckExternalMedia   bool          `json:"check_external_media"`   // Whether to check external media
	ExternalMediaTimeout time.Duration `json:"external_media_timeout"` // Timeout for fetching external media
	MaxExternalSize      int64         `json:"max_external_size"`      // Maximum size for external media (bytes)

	// Debug settings
	Debug bool `json:"debug"` // Enable debug logging
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		APIEndpoint:          "http://localhost:8000/moderate",
		APITimeout:           10 * time.Second,
		NumWorkers:           5,
		PollInterval:         5 * time.Second,
		MaxAttempts:          3,
		TempStoragePath:      "./temp_media",
		RetentionPeriod:      48 * time.Hour,
		CacheSize:            10000,
		CacheTTL:             24 * time.Hour,
		DefaultMode:          "full",
		ThresholdOverrides:   make(map[string]float64),
		CheckExternalMedia:   true,
		ExternalMediaTimeout: 5 * time.Second,
		MaxExternalSize:      10 * 1024 * 1024, // 10MB
		Debug:                false,
	}
}

// LoadConfig loads the content moderation configuration from Viper
func LoadConfig() *Config {
	// Start with default config
	config := DefaultConfig()

	// Check if content_moderation section exists in config
	if !viper.IsSet("content_moderation") {
		log.Println("Warning: No content_moderation configuration found, using defaults")
		return config
	}

	// API settings
	if viper.IsSet("content_moderation.api_endpoint") {
		config.APIEndpoint = viper.GetString("content_moderation.api_endpoint")
	}
	if viper.IsSet("content_moderation.api_timeout") {
		config.APITimeout = viper.GetDuration("content_moderation.api_timeout")
	}

	// Processing settings
	if viper.IsSet("content_moderation.num_workers") {
		config.NumWorkers = viper.GetInt("content_moderation.num_workers")
	}
	if viper.IsSet("content_moderation.poll_interval") {
		config.PollInterval = viper.GetDuration("content_moderation.poll_interval")
	}
	if viper.IsSet("content_moderation.max_attempts") {
		config.MaxAttempts = viper.GetInt("content_moderation.max_attempts")
	}

	// Storage settings
	if viper.IsSet("content_moderation.temp_storage_path") {
		config.TempStoragePath = viper.GetString("content_moderation.temp_storage_path")
	}
	if viper.IsSet("content_moderation.retention_period") {
		config.RetentionPeriod = viper.GetDuration("content_moderation.retention_period")
	}

	// Cache settings
	if viper.IsSet("content_moderation.cache_size") {
		config.CacheSize = viper.GetInt("content_moderation.cache_size")
	}
	if viper.IsSet("content_moderation.cache_ttl") {
		config.CacheTTL = viper.GetDuration("content_moderation.cache_ttl")
	}

	// Policy settings
	if viper.IsSet("content_moderation.default_mode") {
		config.DefaultMode = viper.GetString("content_moderation.default_mode")
	}
	if viper.IsSet("content_moderation.threshold_overrides") {
		thresholds := viper.GetStringMap("content_moderation.threshold_overrides")
		for k, v := range thresholds {
			if val, ok := v.(float64); ok {
				config.ThresholdOverrides[k] = val
			}
		}
	}

	// External media settings
	if viper.IsSet("content_moderation.check_external_media") {
		config.CheckExternalMedia = viper.GetBool("content_moderation.check_external_media")
	}
	if viper.IsSet("content_moderation.external_media_timeout") {
		config.ExternalMediaTimeout = viper.GetDuration("content_moderation.external_media_timeout")
	}
	if viper.IsSet("content_moderation.max_external_size") {
		config.MaxExternalSize = viper.GetInt64("content_moderation.max_external_size")
	}

	// Debug settings
	if viper.IsSet("content_moderation.debug") {
		config.Debug = viper.GetBool("content_moderation.debug")
	}

	return config
}

// NOTE: IsMediaMimeType function is now in utils.go
