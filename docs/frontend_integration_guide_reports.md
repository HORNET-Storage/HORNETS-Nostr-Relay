# Report Notification System: Frontend Integration Guide

## Overview

This guide describes the backend implementation for handling user reports (kind 1984 events) in our Nostr relay. It explains what the backend provides, data structures, and how to integrate with the API endpoints.

## Data Models

### ReportNotification Object

The core data model for report notifications has the following structure:

```json
{
  "id": 1,                                         // Unique ID for the notification
  "pubkey": "abcdef123...",                        // Public key of the content author
  "event_id": "98765abcdef...",                    // ID of the reported event
  "report_type": "nudity",                         // Type of report (from NIP-56)
  "report_content": "This content is inappropriate", // Content from the report event
  "reporter_pubkey": "12345abcdef...",             // Public key of the first reporter
  "report_count": 3,                               // Number of reports for this content
  "created_at": "2025-04-02T08:15:30Z",            // When the report was first received
  "updated_at": "2025-04-02T09:20:15Z",            // When the report was last updated
  "is_read": false                                 // Whether the notification has been read
}
```

### Report Types (from NIP-56)

Our backend tracks the following report types according to NIP-56:

* `nudity` - depictions of nudity, porn, etc.
* `malware` - virus, trojan horse, spyware, etc.
* `profanity` - profanity, hateful speech, etc.
* `illegal` - something which may be illegal
* `spam` - spam
* `impersonation` - someone pretending to be someone else
* `other` - for reports that don't fit above categories

## API Endpoints

The backend provides the following REST API endpoints:

### 1. List Report Notifications

```
GET /api/reports/notifications
```

**Query Parameters**:
- `page` (optional): Page number (default: 1)
- `limit` (optional): Items per page (default: 10, max: 100)
- `filter` (optional): Filter type (values: "all" or "unread", default: "all")

**Response Example**:
```json
{
  "notifications": [
    {
      "id": 1,
      "pubkey": "abcdef123...",
      "event_id": "98765abcdef...",
      "report_type": "nudity",
      "report_content": "This content contains inappropriate images",
      "reporter_pubkey": "12345abcdef...",
      "report_count": 3,
      "created_at": "2025-04-02T08:15:30Z",
      "updated_at": "2025-04-02T09:20:15Z",
      "is_read": false
    },
    {
      "id": 2,
      "pubkey": "abcdef456...",
      "event_id": "12345abcdef...",
      "report_type": "spam",
      "report_content": "This account is posting spam",
      "reporter_pubkey": "67890abcdef...",
      "report_count": 1,
      "created_at": "2025-04-01T15:30:20Z",
      "updated_at": "2025-04-01T15:30:20Z",
      "is_read": true
    }
  ],
  "pagination": {
    "currentPage": 1,
    "pageSize": 10,
    "totalItems": 2,
    "totalPages": 1,
    "hasNext": false,
    "hasPrevious": false
  }
}
```

### 2. Mark a Report as Read

```
POST /api/reports/notifications/read
```

**Request Body**:
```json
{
  "id": 1  // ID of the notification to mark as read
}
```

**Response Example**:
```json
{
  "success": true,
  "message": "Notification marked as read"
}
```

### 3. Mark All Reports as Read

```
POST /api/reports/notifications/read-all
```

**Response Example**:
```json
{
  "success": true,
  "message": "All report notifications marked as read"
}
```

### 4. Get Report Statistics

```
GET /api/reports/stats
```

**Response Example**:
```json
{
  "total_reported": 57,              // Total number of reported events
  "total_reported_today": 5,         // Number of events reported today
  "by_report_type": [                // Breakdown by report type
    {
      "type": "nudity",
      "count": 18
    },
    {
      "type": "spam",
      "count": 24
    },
    {
      "type": "illegal",
      "count": 15
    }
  ],
  "most_reported": [                 // Most frequently reported content
    {
      "event_id": "abcdef123...",
      "pubkey": "12345abcdef...",
      "report_count": 15,
      "report_type": "spam",
      "created_at": "2025-03-28T14:25:30Z"
    },
    {
      "event_id": "12345abcdef...",
      "pubkey": "abcdef456...",
      "report_count": 12,
      "report_type": "nudity",
      "created_at": "2025-03-29T09:15:30Z"
    }
  ]
}
```

### 5. Get Reported Event Content

```
GET /api/reports/event/:id
```

**URL Parameters**:
- `id`: The ID of the event to retrieve

**Response Example**:
```json
{
  "event": {
    "id": "abcdef123...",
    "pubkey": "12345abcdef...",
    "created_at": 1701234567,
    "kind": 1,
    "tags": [
      ["t", "nostr"],
      ["t", "relay"]
    ],
    "content": "This is the reported content that needs to be reviewed.",
    "sig": "signature123..."
  }
}
```

### 6. Delete Reported Event

```
DELETE /api/reports/event/:id
```

**URL Parameters**:
- `id`: The ID of the event to delete

**Response Example**:
```json
{
  "success": true,
  "message": "Event successfully deleted"
}
```

## Implementation Notes

1. **Authentication**: All endpoints require JWT authentication using the same mechanism as other admin endpoints.

2. **Report Counting**: The backend tracks how many times an event has been reported. When multiple users report the same event, the report_count is incremented rather than creating duplicate notifications.

3. **Sorting**: Reports are automatically sorted by report_count in descending order, so the most reported content appears first.

4. **Report Types**: The backend categorizes reports according to NIP-56 types (nudity, malware, profanity, illegal, spam, impersonation, other).

5. **Event Deletion**: The DELETE endpoint permanently removes the event from the relay and also cleans up associated report notifications.

6. **Error Handling**: All endpoints return appropriate error codes and messages that should be handled by the frontend.

7. **Event Filtering**: The API supports filtering between "all" and "unread" reports.

8. **No Content Response**: When requesting unread notifications and none exist, the API returns a 204 No Content status.

## Typical Workflow Integration

1. Display a notification badge showing the count of unread reports
2. List reports sorted by count (most-reported first)
3. Allow admins to view the original reported content
4. Provide options to mark reports as read
5. Enable deletion of offending content
6. Show report statistics on a dashboard

For further details, refer to the complete [Report Notification API Documentation](./report_notification_api.md).
