# Kind 888 Multi-Mode Relay System

## Overview

Kind 888 events serve as the subscription and storage management system for the HORNETS Nostr relay. This system handles four distinct relay modes and provides clients with comprehensive information about user storage allocations, subscription status, and payment requirements.

## Relay Modes

### Public Mode (was "Free Mode")
- **Storage**: Fixed allocation per user (configured by relay operator)
- **Payment**: Not required
- **Expiration**: Long-term (Month to month)
- **Storage Policy**: Resets monthly
- **UI Behavior**: No payment options, storage info only

### Subscription Mode (was "Paid Mode")
- **Storage**: Tiered based on payment amount
- **Payment**: Required for storage allocation
- **Expiration**: Monthly billing cycles
- **Storage Policy**: Accumulates (unused storage carries over)
- **UI Behavior**: Full payment and upgrade options available

### Invite-Only Mode (was "Exclusive Mode")
- **Storage**: Varies by assigned tier (can be limited or unlimited)
- **Payment**: Not required (invite-only access)
- **Expiration**: Set by admin or indefinite
- **Storage Policy**: Based on tier configuration
- **UI Behavior**: Storage info only, no payment options

### Only-Me Mode (was "Personal Mode")
- **Storage**: Always unlimited
- **Payment**: Not required
- **Expiration**: Indefinite
- **Storage Policy**: No limits
- **UI Behavior**: Simple storage usage display

## Kind 888 Event Structure

```json
{
  "kind": 888,
  "pubkey": "<relay_public_key>",
  "created_at": <timestamp>,
  "tags": [
    ["subscription_duration", "1 month"],
    ["p", "<user_pubkey>"],
    ["subscription_status", "active|inactive|expired"],
    ["relay_bitcoin_address", "<bitcoin_address_or_empty>"],
    ["relay_dht_key", "<relay_dht_key>"],
    ["storage", "<used_bytes>", "<total_bytes|unlimited>", "<last_updated_timestamp>"],
    ["credit", "<credit_amount_in_sats>"],
    ["active_subscription", "<tier_name>", "<expiration_timestamp>"],
    ["relay_mode", "public|subscription|invite-only|only-me"]
  ],
  "content": "",
  "sig": "<relay_signature>"
}
```

## Key Tags Explained

### relay_mode
- **Purpose**: Informs clients about the relay's operational mode
- **Values**: `public`, `subscription`, `invite-only`, `only-me`
- **Client Usage**: Determines which UI components to display

### storage
- **Format**: `["storage", "used_bytes", "total_bytes", "timestamp"]`
- **Unlimited**: `["storage", "used_bytes", "unlimited", "timestamp"]`
- **Updates**: Real-time as users post content or upload files

### subscription_status
- **active**: User has valid subscription
- **inactive**: User has no active subscription (new users)
- **expired**: User's subscription has expired

### credit
- **Purpose**: Tracks accumulated credit from overpayments
- **Usage**: Automatically applied to future storage purchases
- **Only Present**: When user has credit balance > 0

## Storage Management Policies

### Public Mode Users
- Storage allocation resets monthly
- Fixed allocation (e.g., 100MB per month)
- No accumulation of unused storage

### Subscription Mode Users
- Storage accumulates month-to-month
- Unused storage carries over to next billing cycle
- Credit system handles overpayments

### Invite-Only/Only-Me Users
- Storage based on assigned tier
- Only-me mode users always get unlimited storage
- Invite-only users vary by tier configuration

## Payment Processing

### Overpayment Handling
1. System calculates highest tier user can afford
2. Extends subscription for multiple periods if applicable
3. Applies cascading tier logic for remainder
4. Stores unusable amounts as credit

### Credit Application
- Credit automatically applied on next payment
- Credit used to purchase additional storage when threshold reached
- Visible in kind 888 events for transparency

## Client Integration Guidelines

### Reading relay_mode Tag
```javascript
function parseKind888Event(event) {
  const relayMode = getTagValue(event.tags, 'relay_mode');
  const storage = getTagValues(event.tags, 'storage');
  const isUnlimited = storage[1] === 'unlimited';
  
  return {
    mode: relayMode,
    usedBytes: parseInt(storage[0]),
    totalBytes: isUnlimited ? Infinity : parseInt(storage[1]),
    isUnlimited: isUnlimited
  };
}
```

### UI Decision Making
```javascript
function shouldShowPaymentOptions(relayMode) {
  return relayMode === 'subscription';
}

function shouldShowUpgradeOptions(relayMode, subscription) {
  return relayMode === 'subscription' && subscription.status === 'active';
}

function getStorageDisplayText(storage) {
  if (storage.isUnlimited) {
    return `${formatBytes(storage.usedBytes)} used (unlimited)`;
  }
  return `${formatBytes(storage.usedBytes)} / ${formatBytes(storage.totalBytes)} used`;
}
```

## Mode Transition Handling

When relay operators change modes, the system:

1. **Preserves existing subscriptions** until natural expiration
2. **Updates new kind 888 events** with current mode
3. **Batch updates existing events** to include relay_mode tag
4. **Maintains storage allocations** during transition period

### Transition Examples

**Public → Subscription**: Existing public allocations remain until expiration, then payment required  
**Subscription → Public**: Immediate downgrade to public tier limits  
**Any → Only-Me**: Immediate unlimited storage for all users  
**Any → Invite-Only**: Admin manually manages access, existing subscriptions honored

## Real-Time Storage Updates

### Update Triggers
- Kind 1 events (text notes)
- Blossom file uploads
- DAG storage operations
- Event deletions

### Update Process
1. Calculate storage delta for operation
2. Check against current limits
3. Update kind 888 event with new usage
4. Enforce limits if exceeded

## Backend Implementation Notes

### Mode Detection
- Mode read from `allowed_users.mode` in configuration
- Loaded via `viper.UnmarshalKey("allowed_users", &settings)`
- Default to "unknown" if config unavailable

### Event Creation
- All kind 888 events automatically include relay_mode tag
- Mode read fresh from config on each event creation
- Ensures consistency even during mode transitions

### Storage Enforcement
- Only-me mode: No limits enforced
- Other modes: Limits enforced based on tier allocation
- Real-time checking prevents storage overuse

## Migration and Compatibility

### Existing Events
- Old kind 888 events without relay_mode tag remain valid
- Batch update process adds relay_mode to existing events
- Backward compatibility maintained for clients

### Client Updates
- Clients should gracefully handle missing relay_mode tags
- Default behavior when relay_mode absent
- Progressive enhancement for mode-aware features

## Monitoring and Debugging

### Log Messages
- Mode loading: `"[DEBUG] Creating storage info for npub X: mode=Y"`
- Event creation: `"Creating/updating NIP-88 event for X with tier Y"`
- Storage updates: `"[DEBUG] Creating kind 888 event: totalBytes=X"`

### Common Issues
- "Unknown mode" logs indicate config loading problems
- Check `allowed_users.mode` in relay configuration
- Verify viper configuration is properly loaded