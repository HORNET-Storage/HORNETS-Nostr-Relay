# iOS Push Notification Payload Reference

> **Last updated:** 2026-02-24
> **Relay version:** `ed25d7c` (feature/move-push-routes-to-relay-service)

## Overview

The relay sends APNs push notifications when certain Nostr events are received that target a registered device's owner. This document describes the exact JSON payload structure the iOS app will receive.

---

## APNs JSON Structure

```json
{
  "aps": {
    "alert": {
      "title": "⚡ alice liked your note",
      "body": "❤️"
    },
    "badge": 1,
    "sound": "default",
    "mutable-content": 1
  },
  "category": "kind_7",
  "event_id": "abc123...",
  "author_pubkey": "def456...",
  "kind": 7,
  "created_at": 1708000000,
  "referenced_event_id": "original789...",
  "referenced_event_kind": 1,
  "referenced_event_content": "This is the original note that was liked..."
}
```

---

## Field Reference

### Always Present (in `aps`)

| Field | Type | Description |
|-------|------|-------------|
| `aps.alert.title` | string | Human-readable notification title (includes emoji + author name) |
| `aps.alert.body` | string | Notification body text (reaction content, message preview, etc.) |
| `aps.badge` | int | Always `1` — signals unread notifications |
| `aps.sound` | string | Always `"default"` |
| `aps.mutable-content` | int | Always `1` — enables Notification Service Extension to modify before display |

### Always Present (custom data)

| Field | Type | Description |
|-------|------|-------------|
| `category` | string | `"kind_X"` where X is the Nostr event kind (e.g., `"kind_7"`, `"kind_1809"`) |
| `event_id` | string | The Nostr event ID that triggered this notification |
| `author_pubkey` | string | Public key (hex) of the event author |
| `kind` | int | Nostr event kind number |
| `created_at` | int | Unix timestamp of the event |

### Always Present for Reactions/Reposts/Replies (kinds 1, 6, 7, 1808, 1809)

| Field | Type | Description |
|-------|------|-------------|
| `referenced_event_id` | string | Nostr event ID of the original event being reacted to / reposted / replied to. Extracted directly from the event's `e` tag — **always available** for these kinds. |

### Optional — DB-Enriched Fields (may be absent)

These fields require the referenced event to exist in the relay's database. They are included when the DB lookup succeeds, omitted otherwise.

| Field | Type | Description |
|-------|------|-------------|
| `referenced_event_kind` | int | Kind of the referenced event (e.g., `1` for text note, `1808` for audio note) |
| `referenced_event_content` | string | First 100 characters of the referenced event's content (truncated with `...` if longer) |

**⚠️ Important:** The iOS app should treat `referenced_event_kind` and `referenced_event_content` as optional. They will be absent when:
- The relay doesn't have the original event in its database (but `referenced_event_id` will still be present)
- A database query error occurs (rare)

`referenced_event_id` itself is absent only for kinds that don't reference other events (kind 3 follow, kind 1059 DM).

---

## Notification Types by Event Kind

### Kind 7 — Reaction (Like)

```json
{
  "aps": {
    "alert": {
      "title": "⚡ alice liked your note",
      "body": "❤️"
    }
  },
  "category": "kind_7",
  "kind": 7,
  "referenced_event_id": "...",
  "referenced_event_kind": 1,
  "referenced_event_content": "The original note text..."
}
```

- **Title format:** `⚡ {author_name} liked your note` (for `+` or `❤️` content) or `⚡ {author_name} reacted to your note` (for custom emoji like `🔥`)
- **Body:** The reaction content itself (`❤️`, `+`, `🔥`, custom emoji, etc.)
- **Referenced event:** The note/event being reacted to

### Kind 6 — Repost

```json
{
  "aps": {
    "alert": {
      "title": "🔄 bob reposted your note",
      "body": "Your note was reposted"
    }
  },
  "category": "kind_6",
  "kind": 6,
  "referenced_event_id": "...",
  "referenced_event_kind": 1,
  "referenced_event_content": "The original note text..."
}
```

### Kind 1809 — Audio Repost

```json
{
  "aps": {
    "alert": {
      "title": "🔄 charlie reposted your note",
      "body": "Your note was reposted"
    }
  },
  "category": "kind_1809",
  "kind": 1809,
  "referenced_event_id": "...",
  "referenced_event_kind": 1808,
  "referenced_event_content": "Audio note description or metadata..."
}
```

- Same title/body format as kind 6
- Referenced event is typically a kind 1808 (audio note)

### Kind 1 — Text Note (Reply/Mention)

```json
{
  "aps": {
    "alert": {
      "title": "💬 dave sent you a note",
      "body": "Hey, check this out! Here's what I was thinking about..."
    }
  },
  "category": "kind_1",
  "kind": 1,
  "referenced_event_id": "...",
  "referenced_event_kind": 1,
  "referenced_event_content": "The parent note being replied to..."
}
```

- **Body:** First ~100 chars of the note content
- **Referenced event:** Present when it's a reply (the parent note)

### Kind 1808 — Audio Note

```json
{
  "aps": {
    "alert": {
      "title": "🎵 eve shared an audio note",
      "body": "New audio content available"
    }
  },
  "category": "kind_1808",
  "kind": 1808
}
```

### Kind 3 — Follow

```json
{
  "aps": {
    "alert": {
      "title": "👤 frank started following you",
      "body": "You have a new follower"
    }
  },
  "category": "kind_3",
  "kind": 3
}
```

- No referenced event (follows don't reference other events)

### Kind 1059 — Encrypted Direct Message (Gift Wrap)

```json
{
  "aps": {
    "alert": {
      "title": "🔒 You received an encrypted message",
      "body": "You have a new encrypted message"
    }
  },
  "category": "kind_1059",
  "kind": 1059
}
```

- **No author name shown** (privacy — DMs don't reveal sender in the notification)
- No referenced event

---

## Author Name Resolution

The `{author_name}` in titles is resolved by looking up the author's Kind 0 (profile/metadata) event. The relay checks these fields in order:

1. `display_name`
2. `displayName`
3. `name`
4. `username`
5. `handle`

If no profile is found, falls back to the first 8 characters of the pubkey (e.g., `a1b2c3d4...`).

---

## iOS App Implementation Notes

### 1. Handle Fields

```swift
// Always available
let eventId = userInfo["event_id"] as? String
let kind = userInfo["kind"] as? Int
let authorPubkey = userInfo["author_pubkey"] as? String
let createdAt = userInfo["created_at"] as? Int
let category = userInfo["category"] as? String

// Always present for kinds 1, 6, 7, 1808, 1809 (comes from the event's e-tag)
let refEventId = userInfo["referenced_event_id"] as? String

// Optional — only present when the relay has the referenced event in its DB
let refEventKind = userInfo["referenced_event_kind"] as? Int
let refEventContent = userInfo["referenced_event_content"] as? String
```

### 2. Deep Linking

For reactions/reposts, `referenced_event_id` is always available — use it to navigate to the original content:

```swift
if let refId = userInfo["referenced_event_id"] as? String {
    // Navigate to the original note/event being reacted to (always present for kinds 1,6,7,1808,1809)
    navigateToEvent(id: refId)
} else if let eventId = userInfo["event_id"] as? String {
    // For kinds without references (kind 3 follow, kind 1059 DM)
    navigateToEvent(id: eventId)
}
```

### 3. Category-Based Actions

Use the `category` field to configure notification actions:

```swift
// In AppDelegate or Notification Service Extension
let likeCategory = UNNotificationCategory(
    identifier: "kind_7",
    actions: [viewAction, replyAction],
    intentIdentifiers: []
)
```

### 4. Mutable Content / Notification Service Extension

`mutable-content: 1` is always set, so you can use a Notification Service Extension to:
- Fetch and attach the author's profile picture
- Decrypt kind 1059 messages (if you have the keys)
- Enrich the notification with additional context from local cache

---

## Notifiable Event Kinds

Only these event kinds trigger push notifications:

| Kind | Description |
|------|-------------|
| 1 | Text note (reply/mention) |
| 3 | Follow (contact list) |
| 6 | Repost |
| 7 | Reaction (like) |
| 1059 | Encrypted DM (gift wrap) |
| 1808 | Audio note |
| 1809 | Audio repost |

All other kinds (e.g., 30311, 22242, 10002) are silently ignored.