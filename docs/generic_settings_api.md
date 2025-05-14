# Generic Settings API Documentation

## Introduction

The Generic Settings API provides a unified interface for managing different configuration groups in the HORNETS Nostr Relay. This API allows frontend applications to retrieve and update various settings without requiring specific handlers for each settings group, while preserving the existing structure in the config.json file.

## Key Benefits

- **Unified Interface**: Consistent API pattern for all settings groups
- **Type Safety**: Each settings group has a defined Go struct type
- **Flexibility**: Easy to add new settings groups without creating new handlers
- **Backward Compatibility**: Maintains existing config.json structure
- **Validation**: Automatic validation and type conversion

## Settings Types

The following settings types are defined in `lib/settings_types.go`:

### ImageModerationSettings

```go
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
```

### ContentFilterSettings

```go
type ContentFilterSettings struct {
    CacheSize     int   `json:"content_filter_cache_size" mapstructure:"content_filter_cache_size"`
    CacheTTL      int   `json:"content_filter_cache_ttl" mapstructure:"content_filter_cache_ttl"`
    Enabled       bool  `json:"content_filter_enabled" mapstructure:"content_filter_enabled"`
    FullTextKinds []int `json:"full_text_kinds" mapstructure:"full_text_kinds"`
}
```

### NestFeederSettings

```go
type NestFeederSettings struct {
    CacheSize int    `json:"nest_feeder_cache_size" mapstructure:"nest_feeder_cache_size"`
    CacheTTL  int    `json:"nest_feeder_cache_ttl" mapstructure:"nest_feeder_cache_ttl"`
    Enabled   bool   `json:"nest_feeder_enabled" mapstructure:"nest_feeder_enabled"`
    Timeout   int    `json:"nest_feeder_timeout" mapstructure:"nest_feeder_timeout"`
    URL       string `json:"nest_feeder_url" mapstructure:"nest_feeder_url"`
}
```

### OllamaSettings

```go
type OllamaSettings struct {
    Model   string `json:"ollama_model" mapstructure:"ollama_model"`
    Timeout int    `json:"ollama_timeout" mapstructure:"ollama_timeout"`
    URL     string `json:"ollama_url" mapstructure:"ollama_url"`
}
```

### XNostrSettings

```go
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

type XNostrNitterSettings struct {
    FailureThreshold  int                    `json:"failure_threshold" mapstructure:"failure_threshold"`
    Instances         []XNostrNitterInstance `json:"instances" mapstructure:"instances"`
    RecoveryThreshold int                    `json:"recovery_threshold" mapstructure:"recovery_threshold"`
    RequestsPerMinute int                    `json:"requests_per_minute" mapstructure:"requests_per_minute"`
}

type XNostrNitterInstance struct {
    Priority int    `json:"priority" mapstructure:"priority"`
    URL      string `json:"url" mapstructure:"url"`
}

type XNostrIntervalSettings struct {
    FollowerUpdateIntervalDays   int `json:"follower_update_interval_days" mapstructure:"follower_update_interval_days"`
    FullVerificationIntervalDays int `json:"full_verification_interval_days" mapstructure:"full_verification_interval_days"`
    MaxVerificationAttempts      int `json:"max_verification_attempts" mapstructure:"max_verification_attempts"`
}
```

### RelayInfoSettings

```go
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
```

### WalletSettings

```go
type WalletSettings struct {
    APIKey string `json:"wallet_api_key" mapstructure:"wallet_api_key"`
    Name   string `json:"wallet_name" mapstructure:"wallet_name"`
}
```

### GeneralSettings

```go
type GeneralSettings struct {
    Port         string `json:"port" mapstructure:"port"`
    PrivateKey   string `json:"private_key" mapstructure:"private_key"`
    Proxy        bool   `json:"proxy" mapstructure:"proxy"`
    DemoMode     bool   `json:"demo_mode" mapstructure:"demo_mode"`
    Web          bool   `json:"web" mapstructure:"web"`
    ServiceTag   string `json:"service_tag" mapstructure:"service_tag"`
    RelayStatsDB string `json:"relay_stats_db" mapstructure:"relay_stats_db"`
}
```

## API Endpoints

### URL Structure

The Generic Settings API follows a RESTful pattern with the following URL structure:

```
/api/settings/{group}
```

Where `{group}` is one of the following:
- `image_moderation`
- `content_filter`
- `nest_feeder`
- `ollama`
- `xnostr`
- `relay_info`
- `wallet`
- `general`
- `query_cache`
- `relay_settings`

For backward compatibility, the relay_settings group also has dedicated endpoints:
```
/api/relay-settings
```

### GET /api/settings/{group}

Retrieves the current settings for the specified group.

**Example Request:**
```
GET /api/settings/image_moderation
```

**Example Response:**
```json
{
  "image_moderation": {
    "api": "http://localhost:8000/moderate",
    "check_interval": 30,
    "concurrency": 5,
    "enabled": true,
    "mode": "full",
    "temp_dir": "./data/moderation/temp",
    "threshold": 0.4,
    "timeout": 300
  }
}
```

### POST /api/settings/{group}

Updates the settings for the specified group.

**Example Request:**
```
POST /api/settings/content_filter
Content-Type: application/json

{
  "content_filter": {
    "cache_size": 10000,
    "cache_ttl": 60,
    "enabled": true,
    "full_text_kinds": [1, 30023]
  }
}
```

**Example Response:**
```
200 OK
```

## Implementation Details

The Generic Settings API is implemented in two main files:

1. `lib/settings_types.go` - Defines the Go struct types for each settings group
2. `lib/web/handler_config_settings.go` - Implements the handler functions

### Settings Registry

The handler uses a registry to map between settings group names and their Go types:

```go
var settingsRegistry = map[string]interface{}{
    "relay_settings":   types.RelaySettings{},
    "image_moderation": types.ImageModerationSettings{},
    "content_filter":   types.ContentFilterSettings{},
    "nest_feeder":      types.NestFeederSettings{},
    "ollama":           types.OllamaSettings{},
    "xnostr":           types.XNostrSettings{},
    "relay_info":       types.RelayInfoSettings{},
    "wallet":           types.WalletSettings{},
    "general":          types.GeneralSettings{},
    "query_cache":      map[string]interface{}{},
}
```

### Settings Storage

The handler handles both nested settings (like relay_settings) and flat settings with common prefixes (like image_moderation_*) while maintaining the existing structure in config.json.

For example:
- `relay_settings` is stored as a single nested object in config.json
- `image_moderation_*` settings are stored as separate keys with a common prefix

The handler automatically handles this mapping in both directions.

### Special Handling

Some settings groups require special handling after updates. This is implemented through a hooks registry:

```go
var settingsUpdateHooks = map[string]func(interface{}, stores.Store) error{
    "relay_settings": handleRelaySettingsUpdate,
    // Add more hooks as needed
}
```

For example, when relay_settings are updated, the handler checks if subscription tiers have changed and creates a kind 411 event if necessary.

## Examples

### Example 1: Getting Image Moderation Settings

**Request:**
```
GET /api/settings/image_moderation
```

**Response:**
```json
{
  "image_moderation": {
    "api": "http://localhost:8000/moderate",
    "check_interval": 30,
    "concurrency": 5,
    "enabled": true,
    "mode": "full",
    "temp_dir": "./data/moderation/temp",
    "threshold": 0.4,
    "timeout": 300
  }
}
```

### Example 2: Updating Content Filter Settings

**Request:**
```
POST /api/settings/content_filter
Content-Type: application/json

{
  "content_filter": {
    "cache_size": 10000,
    "cache_ttl": 60,
    "enabled": true,
    "full_text_kinds": [1, 30023]
  }
}
```

**Response:**
```
200 OK
```

### Example 3: Getting XNostr Settings

**Request:**
```
GET /api/settings/xnostr
```

**Response:**
```json
{
  "xnostr": {
    "browser_path": "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
    "browser_pool_size": 3,
    "check_interval": 30,
    "concurrency": 3,
    "enabled": true,
    "temp_dir": "/tmp/xnostr-verification",
    "update_interval": 24,
    "nitter": {
      "failure_threshold": 3,
      "instances": [
        {
          "priority": 1,
          "url": "https://nitter.net/"
        },
        {
          "priority": 2,
          "url": "https://nitter.lacontrevoie.fr/"
        }
      ],
      "recovery_threshold": 2,
      "requests_per_minute": 10
    },
    "verification_intervals": {
      "follower_update_interval_days": 7,
      "full_verification_interval_days": 30,
      "max_verification_attempts": 5
    }
  }
}
```

## Frontend Integration

When implementing the admin panel, you should:

1. Create UI components for each settings group
2. Use the GET endpoint to fetch current settings
3. Use the POST endpoint to update settings
4. Handle validation on the frontend before submitting

The API will handle type conversion and validation on the backend, but providing good frontend validation improves the user experience.

## Adding New Settings Groups

To add a new settings group:

1. Define a new struct type in `lib/settings_types.go`
2. Add the new group to the `settingsRegistry` in `lib/web/handler_config_settings.go`
3. Add any special handling to the `settingsUpdateHooks` if needed
4. Create UI components for the new settings group in the admin panel

No changes to the API endpoints are needed, as they are already generic.
