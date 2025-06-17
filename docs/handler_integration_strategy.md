# Handler Integration Strategy for Allowed Users

## The Problem

We have overlapping functionality between handlers:

1. **`handler_relay_settings.go`** - Manages `subscription_tiers` in `relay_settings`
2. **`handler_config_settings.go`** - Now manages `tiers` in `allowed_users`
3. **Potential conflicts** - Both locations store tier information

## Current State

### RelaySettings struct (lib/types.go)
```go
type RelaySettings struct {
    // ... other fields ...
    SubscriptionTiers   []SubscriptionTier `json:"subscription_tiers"`
    FreeTierEnabled     bool               `json:"freeTierEnabled"`
    FreeTierLimit       string             `json:"freeTierLimit"`
}
```

### AllowedUsersSettings struct (lib/settings_types.go)
```go
type AllowedUsersSettings struct {
    Mode        string             `json:"mode"`
    ReadAccess  ReadAccessConfig   `json:"read_access"`
    WriteAccess WriteAccessConfig  `json:"write_access"`
    Tiers       []SubscriptionTier `json:"tiers"`  // MOVED HERE!
}
```

## Integration Strategy

### Option 1: Keep Both, But Make Them Sync (NOT RECOMMENDED)
- Keep tiers in both places
- Sync them on every update
- Leads to complexity and potential bugs

### Option 2: Migrate and Redirect (RECOMMENDED)
1. **Migration on startup**: Check if `relay_settings.subscription_tiers` exists but `allowed_users.tiers` doesn't
2. **Copy tiers**: Move subscription_tiers to allowed_users.tiers
3. **Update relay settings handler**: 
   - GET: Return tiers from allowed_users
   - POST: Update tiers in allowed_users
4. **Remove from RelaySettings struct**: Eventually deprecate

### Option 3: Full Separation
- Keep subscription tiers in relay_settings for backward compatibility
- Use allowed_users.tiers for new functionality
- Document which one to use when

## Recommended Implementation (Option 2)

### 1. Update main.go to migrate on startup:
```go
// Migrate subscription tiers from relay_settings to allowed_users if needed
func migrateSubscriptionTiers() {
    var relaySettings types.RelaySettings
    var allowedUsers types.AllowedUsersSettings
    
    viper.UnmarshalKey("relay_settings", &relaySettings)
    viper.UnmarshalKey("allowed_users", &allowedUsers)
    
    // If relay has tiers but allowed_users doesn't, migrate
    if len(relaySettings.SubscriptionTiers) > 0 && len(allowedUsers.Tiers) == 0 {
        allowedUsers.Tiers = relaySettings.SubscriptionTiers
        viper.Set("allowed_users.tiers", allowedUsers.Tiers)
        
        // Optionally remove from relay_settings
        relaySettings.SubscriptionTiers = nil
        viper.Set("relay_settings.subscription_tiers", nil)
        
        viper.WriteConfig()
        log.Println("Migrated subscription tiers to allowed_users")
    }
}
```

### 2. Update handler_relay_settings.go:
```go
func getRelaySettings(c *fiber.Ctx) error {
    var relaySettings types.RelaySettings
    viper.UnmarshalKey("relay_settings", &relaySettings)
    
    // Get tiers from allowed_users instead
    var allowedUsers types.AllowedUsersSettings
    viper.UnmarshalKey("allowed_users", &allowedUsers)
    relaySettings.SubscriptionTiers = allowedUsers.Tiers
    
    return c.JSON(fiber.Map{
        "relay_settings": relaySettings,
    })
}

func updateRelaySettings(c *fiber.Ctx, store stores.Store) error {
    // ... existing code ...
    
    // If tiers are in the update, redirect to allowed_users
    if len(relaySettings.SubscriptionTiers) > 0 {
        var allowedUsers types.AllowedUsersSettings
        viper.UnmarshalKey("allowed_users", &allowedUsers)
        allowedUsers.Tiers = relaySettings.SubscriptionTiers
        viper.Set("allowed_users.tiers", allowedUsers.Tiers)
        
        // Don't save tiers in relay_settings
        relaySettings.SubscriptionTiers = nil
    }
    
    // ... rest of the code ...
}
```

### 3. Update subscription system to read from allowed_users:
- Update any code that reads `relay_settings.subscription_tiers`
- Make it read from `allowed_users.tiers` instead
- Or create a helper function that checks both locations

## Benefits of This Approach

1. **Backward Compatibility**: Existing API calls continue to work
2. **Single Source of Truth**: Tiers are only stored in allowed_users
3. **Smooth Migration**: No breaking changes for frontend
4. **Clear Separation**: Each handler has clear responsibilities

## Implementation Order

1. Add migration function to main.go
2. Update handler_relay_settings.go to redirect tier operations
3. Test with existing frontend
4. Update documentation
5. Eventually deprecate subscription_tiers in RelaySettings struct

## Testing Strategy

1. Test with existing relay_settings that has subscription_tiers
2. Verify migration moves them to allowed_users
3. Test that relay_settings API still returns tiers
4. Test that updating via relay_settings updates allowed_users
5. Test that both handlers don't conflict
