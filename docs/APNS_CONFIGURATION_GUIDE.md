# APNs Configuration Guide for HORNETS Relay

## Current State
The push notification service is fully implemented but currently uses **mock APNs/FCM clients** for testing. This means:
- Device registration works ✅
- Notification triggering works ✅  
- Notification queuing works ✅
- **Actual delivery to devices does NOT work** ❌ (mock mode)

## Why Notifications Aren't Arriving
The APNs client in `services/push/apns.go` is a mock implementation that only logs what it would send. Real APNs integration requires:

1. **APNs Authentication** (one of these):
   - `.p8` certificate file from Apple Developer Portal
   - Team ID and Key ID
   - Bundle ID for your app

2. **Environment Configuration**:
   - **Development**: For apps installed via Xcode/TestFlight
   - **Production**: For apps from the App Store
   
   **COMMON ISSUE**: Using production tokens with development certificates or vice versa causes silent failures.

## Required Configuration

### 1. APNs Setup (config.yaml)
```yaml
push:
  enabled: true
  apns:
    enabled: true
    bundle_id: "com.your.app"  # Your iOS app's bundle ID
    team_id: "XXXXXXXXXX"       # Your Apple Developer Team ID
    key_id: "XXXXXXXXXX"        # Your APNs Key ID
    key_path: "/path/to/AuthKey_XXXXXX.p8"  # Path to .p8 file
    production: false           # false for development, true for production
```

### 2. Environment Mismatch Troubleshooting

**Symptoms of environment mismatch:**
- Device registers successfully
- Test notifications show as "sent" in logs
- No notification arrives on device
- No error messages

**How to verify your environment:**
1. **Development environment** (Sandbox):
   - Apps installed via Xcode
   - Apps installed via TestFlight
   - Use `production: false` in config

2. **Production environment**:
   - Apps from App Store
   - Use `production: true` in config

### 3. Device Token Format
- iOS device tokens should be 64 hexadecimal characters
- Example: `a1b2c3d4e5f6789...` (64 chars)
- If using development build, token is for sandbox APNs
- If using production build, token is for production APNs

## Implementing Real APNs Client

To implement real APNs delivery, replace the mock client in `services/push/apns.go`:

```go
// Example using github.com/sideshow/apns2
import "github.com/sideshow/apns2"

func NewAPNSClient(config *types.APNSConfig) (APNSClient, error) {
    authKey, err := token.AuthKeyFromFile(config.KeyPath)
    if err != nil {
        return nil, err
    }

    token := &token.Token{
        AuthKey: authKey,
        KeyID:   config.KeyID,
        TeamID:  config.TeamID,
    }

    client := apns2.NewTokenClient(token)
    
    if config.Production {
        client = client.Production()
    } else {
        client = client.Development()
    }
    
    return &RealAPNSClient{client: client}, nil
}
```

## Quick Checklist

- [ ] Obtained `.p8` file from Apple Developer Portal
- [ ] Added Team ID and Key ID to config
- [ ] Verified app bundle ID matches config
- [ ] Confirmed environment (dev/prod) matches app installation method
- [ ] Device tokens are 64 hex characters
- [ ] Implemented real APNs client (not mock)
- [ ] Tested with matching environment

## Common Solutions

1. **"Invalid device token"**: Environment mismatch - check if using dev token with prod certificate
2. **"Missing provider token"**: Check .p8 file path and permissions
3. **Silent failures**: Usually environment mismatch - dev/prod confusion
4. **"BadDeviceToken"**: Token is for wrong environment

## Testing

1. Register device: `POST /api/push/register`
2. Send test: `POST /api/push/test`
3. Check logs for any APNs errors
4. Verify environment matches your app build

## FCM (Android) Configuration

Similar setup needed for FCM:
```yaml
push:
  fcm:
    enabled: true
    credentials_path: "/path/to/firebase-credentials.json"
```

## Summary

The push notification infrastructure is complete and working. The only missing piece is replacing the mock APNs/FCM clients with real implementations and ensuring the environment configuration matches your app's build type (development vs production).