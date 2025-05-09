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

// XNostrSettings represents the X-Nostr verification configuration
type XNostrSettings struct {
	BrowserPath     string                 `json:"xnostr_browser_path" mapstructure:"xnostr_browser_path"`
	BrowserPoolSize int                    `json:"xnostr_browser_pool_size" mapstructure:"xnostr_browser_pool_size"`
	CheckInterval   int                    `json:"xnostr_check_interval" mapstructure:"xnostr_check_interval"`
	Concurrency     int                    `json:"xnostr_concurrency" mapstructure:"xnostr_concurrency"`
	Enabled         bool                   `json:"xnostr_enabled" mapstructure:"xnostr_enabled"`
	TempDir         string                 `json:"xnostr_temp_dir" mapstructure:"xnostr_temp_dir"`
	UpdateInterval  int                    `json:"xnostr_update_interval" mapstructure:"xnostr_update_interval"`
	Nitter          XNostrNitterSettings   `json:"xnostr_nitter" mapstructure:"xnostr_nitter"`
	Intervals       XNostrIntervalSettings `json:"xnostr_verification_intervals" mapstructure:"xnostr_verification_intervals"`
}

// XNostrNitterSettings represents the Nitter configuration for X-Nostr
type XNostrNitterSettings struct {
	FailureThreshold  int                    `json:"failure_threshold" mapstructure:"failure_threshold"`
	Instances         []XNostrNitterInstance `json:"instances" mapstructure:"instances"`
	RecoveryThreshold int                    `json:"recovery_threshold" mapstructure:"recovery_threshold"`
	RequestsPerMinute int                    `json:"requests_per_minute" mapstructure:"requests_per_minute"`
}

// XNostrNitterInstance represents a Nitter instance configuration
type XNostrNitterInstance struct {
	Priority int    `json:"priority" mapstructure:"priority"`
	URL      string `json:"url" mapstructure:"url"`
}

// XNostrIntervalSettings represents the verification intervals for X-Nostr
type XNostrIntervalSettings struct {
	FollowerUpdateIntervalDays   int `json:"follower_update_interval_days" mapstructure:"follower_update_interval_days"`
	FullVerificationIntervalDays int `json:"full_verification_interval_days" mapstructure:"full_verification_interval_days"`
	MaxVerificationAttempts      int `json:"max_verification_attempts" mapstructure:"max_verification_attempts"`
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
