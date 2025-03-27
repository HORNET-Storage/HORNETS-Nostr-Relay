# Image Moderation System

The image moderation system in HORNETS-Nostr-Relay allows relay operators to automatically moderate image content shared through the relay. This document explains how the system works and how to configure it.

## Overview

The image moderation system works as follows:

1. When Nostr events containing images are received, they are stored but marked as "pending moderation"
2. Images are extracted from the event content (URLs, image tags, etc.)
3. A background worker processes these pending events by sending image URLs to a moderation API
4. Based on moderation results, the system either allows the event to be displayed or blocks it
5. Blocked events are hidden from query results but retained for 48 hours for dispute resolution
6. After the retention period, blocked events are permanently deleted

## Design Features

### Atomic Queue Management

The moderation system uses an atomic "get and remove" operation to handle the pending moderation queue:

- The `GetAndRemovePendingModeration` method retrieves a batch of events and removes them from the queue in a single operation
- This prevents race conditions where multiple workers might process the same event
- Each event is only processed once, eliminating duplicate processing

### Blocked Content Retention

Blocked content follows this lifecycle:

1. Event is detected as containing problematic content
2. Event is marked as blocked with a timestamp
3. Event remains in the database but is filtered from query results
4. After 48 hours, a cleanup job removes the event permanently

This retention period allows time for dispute resolution if content was incorrectly blocked.

### Temporary File Management

Image files downloaded for moderation are handled in a dedicated temporary directory:

- Files are created in `./data/moderation/temp` (configurable)
- Each file is automatically deleted after processing
- A background job cleans up any leaked files older than 24 hours
- The directory is excluded from git via `.gitignore` rules

## Configuration

In your `config.json` file, you can configure the image moderation system:

```json
{
  "image_moderation_enabled": true,
  "image_moderation_api": "http://localhost:8080/api/moderate",
  "image_moderation_threshold": 0.4,
  "image_moderation_mode": "full",
  "image_moderation_temp_dir": "./data/moderation/temp",
  "image_moderation_check_interval": 30,
  "image_moderation_timeout": 300,
  "image_moderation_concurrency": 5
}
```

### Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `image_moderation_enabled` | Enable/disable the moderation system | `true` |
| `image_moderation_api` | URL of the moderation API endpoint | `http://localhost:8080/api/moderate` |
| `image_moderation_threshold` | Confidence threshold (0.0-1.0) for blocking content | `0.4` |
| `image_moderation_mode` | Moderation mode (`full` or `fast`) | `full` |
| `image_moderation_temp_dir` | Directory for temporary files | `./data/moderation/temp` |
| `image_moderation_check_interval` | Interval in seconds to check for pending events | `30` |
| `image_moderation_timeout` | Timeout in seconds for API requests | `300` |
| `image_moderation_concurrency` | Number of concurrent moderation tasks | `5` |

## Moderation Process Flow

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│ Event       │     │ Pending     │     │ Decision    │
│ Received    │────>│ Moderation  │────>│ Based on    │
│             │     │ Queue       │     │ API Result  │
└─────────────┘     └─────────────┘     └──────┬──────┘
                                                │
                                                ▼
                    ┌─────────────┐     ┌──────────────┐
                    │ Permanently │     │ Hidden but   │
                    │ Deleted     │<────┤ Retained for │
                    │ After 48hrs │     │ 48 Hours     │
                    └─────────────┘     └──────────────┘
```

## Moderation API

The moderation system requires an external API that analyzes images and returns moderation decisions. You can:

1. Use a third-party content moderation API (Google Cloud Vision, Azure Content Moderator, etc.)
2. Run your own AI-based moderation service
3. Use the included sample implementation with customizations

### API Protocol

#### Request

```json
{
  "url": "https://example.com/image.jpg",
  "mode": "full",
  "context": "nostr"
}
```

#### Response

```json
{
  "url": "https://example.com/image.jpg",
  "content_level": 0,
  "decision": "allow",
  "confidence": 0.95,
  "explanation": "Image appears to be safe",
  "processed_at": "2025-03-27T07:45:00Z"
}
```

The `content_level` determines how the content is classified:
- 0: Safe
- 1: Low risk
- 2: Medium risk
- 3: High risk (blocked)
- 4: Explicit (blocked)
- 5: Illegal (blocked)

The `decision` field must be either `"allow"` or `"block"`.

## Testing and Debugging

A sample test script is included in `scripts/modtest/main.go` which demonstrates how to use the moderation API:

```bash
cd scripts/modtest
go run main.go --image https://example.com/image.jpg
```

The script outputs:
- The API request
- The full response
- The decision and confidence level
- The explanation for the decision

## Troubleshooting

Common issues and solutions:

### Multiple processing of the same event

If you see logs showing the same event being processed multiple times, check:
- An old version might be running that doesn't use atomic queue operations
- There could be multiple instances of the relay running

### "No data found for this key" errors

These errors are expected when:
- Multiple worker processes attempt to remove the same event from the queue
- The system now handles these errors gracefully and continues processing
