# Moderation Notification API Documentation

This document provides details on how to interact with the moderation notification system API. This API allows the web panel to retrieve and manage notifications related to content moderation.

## API Endpoints

### 1. Get Moderation Notifications

**Endpoint:** `GET /api/moderation/notifications`

**Description:** Retrieves moderation notifications with filtering and pagination options.

**Authentication:** Required (JWT)

**Query Parameters:**
- `filter` - Type of notifications to retrieve:
  - `all` - All notifications (default)
  - `unread` - Only unread notifications
  - `user` - Notifications for a specific user
- `pubkey` - User's public key (required when filter is "user")
- `page` - Page number (default: 1)
- `limit` - Number of items per page (default: 10, max: 100)

**Response:**

For successful requests with notifications (`200 OK`):
```json
{
  "notifications": [
    {
      "id": 123,
      "pubkey": "user_public_key",
      "event_id": "blocked_event_id",
      "reason": "Explicit content detected",
      "created_at": "2025-03-27T15:30:45Z",
      "is_read": false,
      "content_type": "image",
      "media_url": "https://example.com/image.jpg",
      "thumbnail_url": "https://example.com/thumbnail.jpg"
    }
  ],
  "pagination": {
    "currentPage": 1,
    "pageSize": 10,
    "totalItems": 45,
    "totalPages": 5,
    "hasNext": true,
    "hasPrevious": false
  }
}
```

**Special Case - No Unread Notifications:**
When querying for unread notifications with `filter=unread` and there are none available, the API returns:
- Status Code: `204 No Content`
- Empty response body

This is deliberate to optimize client-side handling. The client should interpret 204 as "no unread notifications" 
rather than as an error.

### 2. Mark Notification as Read

**Endpoint:** `POST /api/moderation/notifications/read`

**Description:** Marks a specific notification as read.

**Authentication:** Required (JWT)

**Request Body:**
```json
{
  "id": 123
}
```

**Response (200 OK):**
```json
{
  "success": true,
  "message": "Notification marked as read"
}
```

### 3. Mark All Notifications as Read

**Endpoint:** `POST /api/moderation/notifications/read-all`

**Description:** Marks all notifications for a specific user as read.

**Authentication:** Required (JWT)

**Request Body:**
```json
{
  "pubkey": "user_public_key"
}
```

**Response (200 OK):**
```json
{
  "success": true,
  "message": "All notifications marked as read"
}
```

### 4. Get Moderation Statistics

**Endpoint:** `GET /api/moderation/stats`

**Description:** Retrieves statistics about moderated content.

**Authentication:** Required (JWT)

**Response (200 OK):**
```json
{
  "total_blocked": 152,
  "total_blocked_today": 8,
  "by_content_type": [
    {"type": "image", "count": 120},
    {"type": "video", "count": 32}
  ],
  "by_user": [
    {"pubkey": "user1_pubkey", "count": 15},
    {"pubkey": "user2_pubkey", "count": 9}
  ],
  "recent_reasons": [
    "Explicit content detected",
    "Violence depicted",
    "Hate speech identified"
  ]
}
```

## Client-Side LLM Integration Guide

This section provides guidance on how to prompt a client-side LLM to interact with the moderation notification API.

### Fetching Notifications

```
Make a GET request to /api/moderation/notifications with these parameters:
- filter: "unread" (or "all", "user")
- page: 1
- limit: 10
- pubkey: (include only if filter is "user")

Handle these possible response scenarios:
1. Status 200 (OK): The response contains an array of notifications and pagination metadata.
   Extract and display each notification with:
   - Created date in a readable format
   - The reason for blocking
   - The content type (image/video)
   - A link to the media URL
   
2. Status 204 (No Content): This means there are no unread notifications.
   Display a message like "No new notifications" and disable any "Mark all as read" buttons.
   
3. Status 4xx/5xx: An error occurred. Extract the error message from the response and display it.
```

### Marking Notifications as Read

```
When a user views or dismisses a notification:
1. Send a POST request to /api/moderation/notifications/read with:
   {
     "id": [notification_id]
   }
   
2. On successful response, update the UI to reflect that this notification has been read by:
   - Changing its visual appearance (e.g., reducing opacity)
   - Moving it to a "Read" section if your UI separates read/unread notifications
   - Updating any notification counters
   
3. If the user chooses to mark all notifications as read, send a POST request to 
   /api/moderation/notifications/read-all with:
   {
     "pubkey": "[user_public_key]"
   }
```

### Error Handling

```
When interacting with the notification API, handle these common errors:
- 400 Bad Request: Check that your request parameters are correct
- 401 Unauthorized: The authentication token is missing or invalid
- 403 Forbidden: The authenticated user doesn't have permission
- 500 Internal Server Error: A server-side error occurred

For all errors, extract the "error" field from the response body and display it to the user.
```

### Polling for New Notifications

```
To check for new notifications periodically:
1. Set up a polling interval (e.g., every 60 seconds)
2. Make a GET request to /api/moderation/notifications?filter=unread&page=1&limit=5
3. If the response is 200 OK, update the UI with the new notifications
4. If the response is 204 No Content, no action is needed (there are no new notifications)
5. Reset the polling timer after each check

Adjust the polling frequency based on your application's needs.
