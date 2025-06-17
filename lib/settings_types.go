package lib

// ImageModerationSettings represents the image moderation configuration
type ImageModerationSettings struct {
	API           string  `json:"image_moderation_api" mapstructure:"image_moderation_api"`
	CheckInterval int     `json:"image_moderation_check_interval" mapstructure:"image_moderation_check_interval"`
	Concurrency   int     `json:"image_moderation_concurrency" mapstructure:"image_moderation_concurrency"`
	Enabled       bool    `json:"image_moderation_enabled" mapstructure:"image_moderation_enabled"`
	Mode          string  `json:"image_moderation_mode" mapstructure:"image_moderation_mode"`
	TempDir       string  `json:"image_moderation_temp_dir" mapstructure:"image_moderation_temp_dir"`
	Threshold     float64 `json:"image_moderation_threshold" mapstructure:"image_moderation_threshold"`
	Timeout       int     `json:"image_moderation_timeout" mapstructure:"image_moderation_timeout"`
}

// ContentFilterSettings represents the content filter configuration
type ContentFilterSettings struct {
	CacheSize     int   `json:"content_filter_cache_size" mapstructure:"content_filter_cache_size"`
	CacheTTL      int   `json:"content_filter_cache_ttl" mapstructure:"content_filter_cache_ttl"`
	Enabled       bool  `json:"content_filter_enabled" mapstructure:"content_filter_enabled"`
	FullTextKinds []int `json:"full_text_kinds" mapstructure:"full_text_kinds"`
}

// NestFeederSettings represents the nest feeder configuration
type NestFeederSettings struct {
	CacheSize int    `json:"nest_feeder_cache_size" mapstructure:"nest_feeder_cache_size"`
	CacheTTL  int    `json:"nest_feeder_cache_ttl" mapstructure:"nest_feeder_cache_ttl"`
	Enabled   bool   `json:"nest_feeder_enabled" mapstructure:"nest_feeder_enabled"`
	Timeout   int    `json:"nest_feeder_timeout" mapstructure:"nest_feeder_timeout"`
	URL       string `json:"nest_feeder_url" mapstructure:"nest_feeder_url"`
}

// OllamaSettings represents the Ollama configuration
type OllamaSettings struct {
	Model   string `json:"ollama_model" mapstructure:"ollama_model"`
	Timeout int    `json:"ollama_timeout" mapstructure:"ollama_timeout"`
	URL     string `json:"ollama_url" mapstructure:"ollama_url"`
}

// RelayInfoSettings represents the relay information configuration
type RelayInfoSettings struct {
	Contact       string `json:"relaycontact" mapstructure:"relaycontact"`
	Description   string `json:"relaydescription" mapstructure:"relaydescription"`
	DHTKey        string `json:"relaydhtkey" mapstructure:"relaydhtkey"`
	Name          string `json:"relayname" mapstructure:"relayname"`
	PubKey        string `json:"relaypubkey" mapstructure:"relaypubkey"`
	Software      string `json:"relaysoftware" mapstructure:"relaysoftware"`
	SupportedNIPs []int  `json:"relaysupportednips" mapstructure:"relaysupportednips"`
	Version       string `json:"relayversion" mapstructure:"relayversion"`
}

// WalletSettings represents the wallet configuration
type WalletSettings struct {
	APIKey string `json:"wallet_api_key" mapstructure:"wallet_api_key"`
	Name   string `json:"wallet_name" mapstructure:"wallet_name"`
}

// GeneralSettings represents general configuration settings
type GeneralSettings struct {
	Port         string `json:"port" mapstructure:"port"`
	PrivateKey   string `json:"private_key" mapstructure:"private_key"`
	Proxy        bool   `json:"proxy" mapstructure:"proxy"`
	DemoMode     bool   `json:"demo_mode" mapstructure:"demo_mode"`
	Web          bool   `json:"web" mapstructure:"web"`
	ServiceTag   string `json:"service_tag" mapstructure:"service_tag"`
	RelayStatsDB string `json:"relay_stats_db" mapstructure:"relay_stats_db"`
}

// AllowedUsersSettings represents the unified access control configuration
type AllowedUsersSettings struct {
	Mode        string             `json:"mode" mapstructure:"mode"` // "free", "paid", "exclusive"
	ReadAccess  ReadAccessConfig   `json:"read_access" mapstructure:"read_access"`
	WriteAccess WriteAccessConfig  `json:"write_access" mapstructure:"write_access"`
	Tiers       []SubscriptionTier `json:"tiers" mapstructure:"tiers"` // Moved from RelaySettings
	LastUpdated int64              `json:"last_updated" mapstructure:"last_updated"`
}

// ReadAccessConfig represents read access permissions
type ReadAccessConfig struct {
	Enabled bool   `json:"enabled" mapstructure:"enabled"`
	Scope   string `json:"scope" mapstructure:"scope"` // "all_users", "paid_users", "allowed_users"
}

// WriteAccessConfig represents write access permissions
type WriteAccessConfig struct {
	Enabled bool `json:"enabled" mapstructure:"enabled"`
	// Scope is mode-dependent:
	// Free: "all_users" when enabled
	// Paid: "paid_users" when enabled
	// Exclusive: "allowed_users" when enabled
}
