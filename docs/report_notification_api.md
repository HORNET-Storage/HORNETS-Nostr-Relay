# Report Notification API

The Report Notification API provides endpoints for managing and retrieving user-generated reports (kind 1984 events).

## Get Report Notifications

Retrieves report notifications with pagination.

**URL**: `/api/reports/notifications`
**Method**: `GET`
**Auth required**: Yes
**Query Parameters**:
- `page` (optional): Page number for pagination (default: 1)
- `limit` (optional): Number of items per page (default: 10, max: 100)
- `filter` (optional): Filter type ("all" or "unread", default: "all")

### Success Response

**Code**: `200 OK`
**Content example**:

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

### Error Response

**Condition**: Server error
**Code**: `500 Internal Server Error`
**Content**:
```json
{
  "error": "Failed to fetch notifications: [error message]"
}
```

## Mark Report Notification as Read

Marks a specific report notification as read.

**URL**: `/api/reports/notifications/read`
**Method**: `POST`
**Auth required**: Yes
**Data constraints**:

```json
{
  "id": "[notification ID integer]"
}
```

### Success Response

**Code**: `200 OK`
**Content example**:

```json
{
  "success": true,
  "message": "Notification marked as read"
}
```

### Error Responses

**Condition**: Invalid notification ID or not found
**Code**: `400 Bad Request`
**Content**:
```json
{
  "error": "Notification ID is required"
}
```

**Condition**: Server error
**Code**: `500 Internal Server Error`
**Content**:
```json
{
  "error": "Failed to mark notification as read: [error message]"
}
```

## Mark All Report Notifications as Read

Marks all report notifications as read.

**URL**: `/api/reports/notifications/read-all`
**Method**: `POST`
**Auth required**: Yes

### Success Response

**Code**: `200 OK`
**Content example**:

```json
{
  "success": true,
  "message": "All report notifications marked as read"
}
```

### Error Response

**Condition**: Server error
**Code**: `500 Internal Server Error`
**Content**:
```json
{
  "error": "Failed to mark all notifications as read: [error message]"
}
```

## Get Report Statistics

Retrieves statistics about reported content.

**URL**: `/api/reports/stats`
**Method**: `GET`
**Auth required**: Yes

### Success Response

**Code**: `200 OK`
**Content example**:

```json
{
  "total_reported": 57,
  "total_reported_today": 5,
  "by_report_type": [
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
  "most_reported": [
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

### Error Response

**Condition**: Server error
**Code**: `500 Internal Server Error`
**Content**:
```json
{
  "error": "Failed to fetch report statistics: [error message]"
}
```

## Get Reported Event

Retrieves the original event that was reported.

**URL**: `/api/reports/event/:id`
**Method**: `GET`
**Auth required**: Yes
**URL Parameters**:
- `id`: The ID of the event to retrieve

### Success Response

**Code**: `200 OK`
**Content example**:

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

### Error Responses

**Condition**: Event not found
**Code**: `404 Not Found`
**Content**:
```json
{
  "error": "Event not found"
}
```

**Condition**: Server error
**Code**: `500 Internal Server Error`
**Content**:
```json
{
  "error": "Failed to retrieve event: [error message]"
}
