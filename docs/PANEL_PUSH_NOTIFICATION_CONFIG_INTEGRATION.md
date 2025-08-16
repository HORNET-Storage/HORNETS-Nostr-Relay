# Push Notification Configuration - Panel Integration Guide

## Overview

This document describes the push notification configuration structure that has been added to the HORNETS-Nostr-Relay settings system. The panel developers need to integrate these settings into the advanced settings section of the admin panel.

## Configuration Structure

The push notification configuration follows the same pattern as other settings groups (e.g., `image_moderation`, `content_filter`, `wallet`, `relay_info`). It is managed through the same `/api/settings` endpoints and can be integrated using your existing `useGenericSettings` hook.

## API Response Structure

### Getting Settings
**Endpoint**: `GET /api/settings`

The response now includes a new `push_notifications` section:

```json
{
  "settings": {
    "push_notifications": {
      "enabled": true,
      "service": {
        "worker_count": 5,
        "queue_size": 1000,
        "retry_max_attempts": 3,
        "retry_base_delay": "1s"
      },
      "apns": {
        "enabled": true,
        "key_file": "path/to/apns-key.p8",
        "key_id": "YOUR_KEY_ID",
        "team_id": "YOUR_TEAM_ID",
        "topic": "com.your.app",
        "production": false
      },
      "fcm": {
        "enabled": true,
        "credentials_file": "path/to/fcm-credentials.json"
      }
    }
  }
}
```

### Updating Settings
**Endpoint**: `POST /api/settings`

To update push notification settings, include the `push_notifications` object in the settings payload.

## Implementation Steps

### 1. Update useGenericSettings Hook

Add support for the new push_notifications group in `panel-source/src/hooks/useGenericSettings.ts`:

**In extractSettingsForGroup function (around line 13), add:**
```typescript
case 'push_notifications':
  rawData = settings?.push_notifications || {};
  break;
```

**In buildNestedUpdate function (around line 207), add:**
```typescript
case 'push_notifications':
  return {
    settings: {
      push_notifications: data
    }
  };
```

### 2. Add TypeScript Types

Update `panel-source/src/types/settings.types.ts`:

```typescript
export interface PushNotificationSettings {
  enabled: boolean;
  service: {
    worker_count: number;
    queue_size: number;
    retry_max_attempts: number;
    retry_base_delay: string;
  };
  apns: {
    enabled: boolean;
    key_file: string;
    key_id: string;
    team_id: string;
    topic: string;
    production: boolean;
  };
  fcm: {
    enabled: boolean;
    credentials_file: string;
  };
}

// Add to SettingsGroupName type
export type SettingsGroupName = 
  | 'image_moderation'
  | 'content_filter'
  | 'ollama'
  | 'wallet'
  | 'relay_info'
  | 'general'
  | 'push_notifications'; // Add this

// Update SettingsGroupType mapping
export type SettingsGroupType<T extends SettingsGroupName> = 
  T extends 'push_notifications' ? PushNotificationSettings :
  // ... rest of existing mappings
```

### 3. Create Component Structure

Following your existing patterns, create a push notifications settings component similar to how you handle other settings groups.

The component should use the `useGenericSettings('push_notifications')` hook and provide form fields for:

**Main Configuration:**
- `enabled` (boolean) - Master toggle

**Service Configuration:**
- `worker_count` (number, 1-100) - Concurrent workers
- `queue_size` (number, 100-10000) - Queue capacity
- `retry_max_attempts` (number, 1-10) - Retry attempts
- `retry_base_delay` (string) - Delay format (1s, 500ms, etc.)

**APNs Configuration:**
- `enabled` (boolean) - Enable iOS push
- `key_file` (string) - Path to .p8 key file
- `key_id` (string, 10 chars) - APNs Key ID
- `team_id` (string, 10 chars) - Team ID
- `topic` (string) - Bundle identifier
- `production` (boolean) - Production vs sandbox

**FCM Configuration:**
- `enabled` (boolean) - Enable Android push
- `credentials_file` (string) - Service account JSON path

## Field Descriptions

### Service Configuration
- **worker_count**: Number of concurrent workers processing notifications (1-100, default: 5)
- **queue_size**: Maximum notifications that can be queued (100-10000, default: 1000)
- **retry_max_attempts**: Maximum retry attempts for failed notifications (1-10, default: 3)
- **retry_base_delay**: Base delay between retries using Go duration format ("1s", "500ms", "2m")

### APNs Configuration (iOS)
- **enabled**: Enable Apple Push Notification service
- **key_file**: Path to APNs .p8 key file (required if enabled)
- **key_id**: APNs Key ID, exactly 10 characters (required if enabled)
- **team_id**: Apple Developer Team ID, exactly 10 characters (required if enabled)
- **topic**: App's bundle identifier (required if enabled)
- **production**: Use production APNs servers (default: false for sandbox)

### FCM Configuration (Android)
- **enabled**: Enable Firebase Cloud Messaging
- **credentials_file**: Path to FCM service account JSON file (required if enabled)

## Validation Rules

1. **Conditional Requirements:**
   - If `apns.enabled` is true, all APNs fields except `production` are required
   - If `fcm.enabled` is true, `credentials_file` is required
   - At least one service (APNs or FCM) should be enabled if push notifications are enabled

2. **Format Validation:**
   - `retry_base_delay`: Must be valid Go duration (e.g., "1s", "500ms")
   - `key_id`: Exactly 10 characters
   - `team_id`: Exactly 10 characters

## Backend Behavior

When push notification settings are updated through the panel:

1. Settings are saved to the configuration file
2. The push notification service automatically reloads with new configuration
3. Existing queued notifications are processed before reload
4. No restart of the main relay service is required

The settings handler at `/lib/web/handlers/settings/handler_relay_settings.go` already recognizes the `push_notifications` section and automatically reloads the push service when settings are updated.

## Integration Testing

1. **Get Settings:**
   ```bash
   curl -X GET http://localhost:3000/api/settings
   ```

2. **Update Settings:**
   ```bash
   curl -X POST http://localhost:3000/api/settings \
     -H "Content-Type: application/json" \
     -d '{"settings": {"push_notifications": {"enabled": true}}}'
   ```

3. **Verify in Logs:**
   - Look for: "Reloading push notification service with new configuration..."
   - Confirm: "Push notification service reloaded successfully"

## Summary

The push notification configuration:
- Uses existing `/api/settings` endpoints
- Follows the `useGenericSettings` hook pattern  
- Is nested under `settings.push_notifications` in the API
- Automatically triggers service reload on save
- Requires no special handling - works like other settings groups

The panel implementation should be straightforward following your existing patterns for settings management.