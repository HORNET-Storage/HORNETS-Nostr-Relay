# Push Notifications V2 - Implementation Details

## Overview
This document details the major upgrade to the Push Notification service implemented in December 2025. The update shifts the service from a simple "notify device" model to a "context-aware" model that can query the database for authored events.

## Key Architecture Changes

### 1. Store Access
- **Old**: `PushService` only had access to `StatisticsStore` (device tokens only).
- **New**: `PushService` now has access to the full `stores.Store` interface.
- **Reason**: Required to perform lookups for "Who wrote this note?" to send notifications to the correct author.

### 2. Event Handling Logic

The following event kinds now trigger "smart" notifications:

| Kind | Description | Logic |
| :--- | :--- | :--- |
| **1** | Text Reply | Notifies `p` tags (Mentions) AND the author of the parent note. |
| **6** | Repost | Notifies the author of the original note. |
| **7** | Reaction | Notifies the author of the original note. **Filters out `-` (dislike).** |
| **1808** | Audio Reply | Notifies `p` tags AND the author of the parent note. |
| **1809** | Audio Repost | Notifies the author of the original audio note. |

### 3. Rich Notifications
We now perform a **Kind 0 (Metadata)** lookup for the sender of the event before dispatching the notification.
- **Before**: "Someone replied to your note"
- **After**: "**Siphiwe** replied to your note" (or "npub...1234" if no name found).

### 4. Code Structure
- **Helper Added**: `getAuthorOfRefEvent(event)` - Finds the author of the parent event (handles `reply` markers and NIP-10 fallbacks).
- **Helper Added**: `getAuthorName(pubkey)` - Looks up `display_name` or `name` from Kind 0 events.
- **Refactoring**: Test tools (`get_devices`, `test_apns`) were moved from `tools/` to `test/` to keep the root clean.

## Configuration
No changes to `config.yaml` structure were made, but ensure `apns` and `firebase` sections are populated for notifications to work.

## Verification
To verify these changes:
1.  **Build**: `go build`
2.  **Restart**: `./hornet-storage`
3.  **Test**: Reply to a post from a different user and check the recipient's device.
