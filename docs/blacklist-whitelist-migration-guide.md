# Migration from Blacklist/Whitelist Mode to Allow Unregistered Kinds

## Overview

The HORNETS relay has been migrated from a mode-based system (blacklist/whitelist) to a more intuitive approach using an "Allow Unregistered Kind Numbers" toggle. This document provides complete details for both backend understanding and frontend integration.

## Backend Changes Summary

### 1. Configuration Structure Changes

#### Removed Fields
```yaml
# OLD - This field no longer exists
event_filtering:
  mode: "whitelist"  # or "blacklist" - REMOVED
```

#### New Fields
```yaml
# NEW
event_filtering:
  allow_unregistered_kinds: false  # boolean field
  registered_kinds: [0, 1, 3, ...]  # array of integers
  kind_whitelist: ["kind0", "kind1", ...]  # existing field, still used
```

### 2. How The New System Works

```
Event arrives → Check if specific handler exists (kind/{number})
├─ YES (Registered kind) → Check if in whitelist
│   ├─ YES → Process with specific handler ✅
│   └─ NO → Reject "Kind not in whitelist" ❌
└─ NO (Unregistered kind) → Check allow_unregistered_kinds
    ├─ TRUE → Process with universal handler ✅
    └─ FALSE → Reject "Unregistered kind not allowed" ❌
```

## Frontend Integration Guide

### 1. Configuration API Changes

#### Getting Current Settings
```javascript
// GET /api/settings
// Response includes:
{
  "event_filtering": {
    "allow_unregistered_kinds": false,  // NEW FIELD
    "registered_kinds": [0, 1, 3, ...], // NEW FIELD - list of all kinds with handlers
    "kind_whitelist": ["kind0", "kind1", ...],  // Existing - which kinds are enabled
    // "mode" field NO LONGER EXISTS
  }
}
```

#### Updating Settings
```javascript
// POST /api/settings
{
  "settings": {
    "event_filtering": {
      "allow_unregistered_kinds": true,  // Toggle for unregistered kinds
      "kind_whitelist": ["kind0", "kind1", ...]  // Update enabled kinds
      // DO NOT send "mode" field - it's deprecated
    }
  }
}
```

### 2. UI Component Requirements

#### Main Toggle Section
```
┌─────────────────────────────────────────────────┐
│ Event Filtering Settings                        │
├─────────────────────────────────────────────────┤
│                                                  │
│ ⚠️ Allow Unregistered Kind Numbers    [ ] OFF   │
│    └─ Warning: Enables unknown event types      │
│                                                  │
└─────────────────────────────────────────────────┘
```

#### Kind List Display
```
┌─────────────────────────────────────────────────┐
│ Registered Event Kinds                          │
├─────────────────────────────────────────────────┤
│                                                  │
│ ✅ Kind 0 - Profile Metadata                    │
│ ✅ Kind 1 - Short Text Note                     │
│ ❌ Kind 3 - Contact List                        │
│ ❌ Kind 5 - Deletion Request                    │
│ ✅ Kind 7 - Reaction                            │
│ ...                                             │
│                                                  │
└─────────────────────────────────────────────────┘
```

### 3. Frontend Logic Implementation

```javascript
// Helper function to determine if a kind is registered
function isRegisteredKind(kind, registeredKinds) {
  return registeredKinds.includes(kind);
}

// Helper function to determine if a kind is enabled
function isKindEnabled(kind, kindWhitelist) {
  return kindWhitelist.includes(`kind${kind}`);
}

// Display logic for each kind
function getKindStatus(kind, settings) {
  const isRegistered = isRegisteredKind(kind, settings.registered_kinds);
  const isEnabled = isKindEnabled(kind, settings.kind_whitelist);
  
  if (isRegistered) {
    return {
      icon: isEnabled ? '✅' : '❌',
      status: isEnabled ? 'Enabled' : 'Disabled',
      canToggle: true
    };
  } else {
    // Unregistered kinds
    return {
      icon: settings.allow_unregistered_kinds ? '⚠️' : '🚫',
      status: settings.allow_unregistered_kinds ? 'Allowed (Unregistered)' : 'Blocked',
      canToggle: false,
      info: 'This kind has no specific handler'
    };
  }
}
```

### 4. Toggle Behavior

#### When toggling a registered kind:
```javascript
function toggleKind(kind, currentWhitelist) {
  const kindStr = `kind${kind}`;
  
  if (currentWhitelist.includes(kindStr)) {
    // Remove from whitelist (disable)
    return currentWhitelist.filter(k => k !== kindStr);
  } else {
    // Add to whitelist (enable)
    return [...currentWhitelist, kindStr];
  }
}
```

#### When toggling "Allow Unregistered Kinds":
```javascript
function toggleAllowUnregistered(currentValue) {
  // Show warning if enabling
  if (!currentValue) {
    showWarning(
      "⚠️ Warning: This will allow events without specific handlers. " +
      "These events will be stored but not processed with business logic."
    );
  }
  return !currentValue;
}
```

### 5. Migration Handling

If the frontend detects an old config (with `mode` field):

```javascript
function migrateOldConfig(config) {
  if (config.event_filtering.mode) {
    // Old config detected
    const migrated = {
      ...config,
      event_filtering: {
        ...config.event_filtering,
        allow_unregistered_kinds: config.event_filtering.mode === 'blacklist',
        // Remove the old mode field
        mode: undefined
      }
    };
    
    // Show migration notice
    showNotice(
      `Configuration migrated: ${config.event_filtering.mode} mode → ` +
      `Allow Unregistered = ${migrated.event_filtering.allow_unregistered_kinds}`
    );
    
    return migrated;
  }
  return config;
}
```

## Complete List of Registered Kinds

These kinds have specific handlers and can be individually enabled/disabled:

| Kind | Description | NIP |
|------|-------------|-----|
| 0 | Profile Metadata | NIP-01 |
| 1 | Short Text Note | NIP-01 |
| 3 | Contact List | NIP-02 |
| 5 | Event Deletion | NIP-09 |
| 6 | Repost | NIP-18 |
| 7 | Reaction | NIP-25 |
| 8 | Badge Award | NIP-58 |
| 117 | Double Ratchet DM | NIP-117 |
| 1063 | File Metadata | NIP-94 |
| 1808 | Audio Notes | Custom |
| 1984 | Reporting | NIP-56 |
| 9372 | Gift Wrap | NIP-888 |
| 9373 | Gift Wrap Response | NIP-888 |
| 9735 | Zap Receipt | NIP-57 |
| 9802 | Highlight | NIP-84 |
| 10000 | Mute List | NIP-51 |
| 10001 | Pin List | NIP-51 |
| 10002 | Relay List | NIP-65 |
| 10010 | Content Filter | NIP-889 |
| 10411 | Relay Info | NIP-888 |
| 11011 | Relay List | NIP-888 |
| 16629 | Ephemeral Event | NIP-888 |
| 16630 | Ephemeral Event | NIP-888 |
| 19841 | Subscription Request | NIP-888 |
| 19842 | Subscription Response | NIP-888 |
| 19843 | Subscription Update | NIP-888 |
| 22242 | Authentication | NIP-42 |
| 30000 | Categorized People List | NIP-51 |
| 30008 | Profile Badge | NIP-58 |
| 30009 | Badge Definition | NIP-58 |
| 30023 | Long-form Content | NIP-23 |
| 30078 | Application Data | NIP-78 |
| 30079 | Event Paths | NIP-116 |

## Testing Endpoints

### Check if a specific kind is allowed:
```bash
# Test registered kind (should check whitelist)
curl -X GET http://localhost:9000/api/test/kind-allowed/1

# Test unregistered kind (should check allow_unregistered_kinds)
curl -X GET http://localhost:9000/api/test/kind-allowed/99999
```

### Get current filtering status:
```bash
curl -X GET http://localhost:9000/api/settings | jq '.settings.event_filtering'
```

## Common Scenarios

### Scenario 1: Strict Mode (Most Secure)
- `allow_unregistered_kinds: false`
- Only specific kinds in whitelist enabled
- Result: Only known, approved event types processed

### Scenario 2: Development Mode
- `allow_unregistered_kinds: true`
- All registered kinds in whitelist
- Result: All events accepted (like old blacklist mode)

### Scenario 3: Selective Testing
- `allow_unregistered_kinds: true`
- Limited registered kinds in whitelist
- Result: Specific known kinds blocked, but new experimental kinds allowed

## Troubleshooting

### Issue: Old "mode" field still in config
**Solution**: The backend will auto-migrate on startup. Frontend should handle gracefully and update config.

### Issue: Unregistered kinds not being accepted
**Check**: Ensure `allow_unregistered_kinds: true` in config

### Issue: Registered kind not working
**Check**: 
1. Verify kind is in `kind_whitelist` array
2. Format should be "kind{number}" (e.g., "kind1", not "1")

## Notes for Frontend Developers

1. **Always check both fields**: A kind being in `registered_kinds` doesn't mean it's enabled. It must also be in `kind_whitelist`.

2. **Visual distinction**: Make it clear which kinds are registered vs unregistered in the UI.

3. **Batch updates**: When toggling multiple kinds, batch them in a single API call to avoid race conditions.

4. **Validation**: Ensure `kind_whitelist` only contains kinds that make sense (format: "kind{number}").

5. **Performance**: Cache the `registered_kinds` list as it doesn't change during runtime.

6. **Warning for unregistered**: Always show a warning when enabling `allow_unregistered_kinds` as it can accept any event type.

## Migration Checklist for Frontend

- [ ] Remove mode selector UI component
- [ ] Add "Allow Unregistered Kind Numbers" toggle with warning
- [ ] Update settings API calls to use new fields
- [ ] Implement visual indicators (✅/❌) for kind status
- [ ] Add migration logic for old configs
- [ ] Update any mode-dependent logic
- [ ] Test with both registered and unregistered kinds
- [ ] Verify warning appears when enabling unregistered kinds
- [ ] Ensure proper state management for the new fields

## Contact

For questions or issues related to this migration, please refer to the HORNETS relay documentation or contact the development team.