# Payment Notification API Documentation

This document provides details on how to interact with the payment notification system API. This API allows the web panel to retrieve and manage notifications related to subscription payments.

## API Endpoints

### 1. Get Payment Notifications

**Endpoint:** `GET /api/payment/notifications`

**Description:** Retrieves payment notifications with filtering and pagination options.

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
      "pubkey": "subscriber_public_key",
      "tx_id": "transaction_id",
      "amount": 210000,
      "subscription_tier": "5GB",
      "is_new_subscriber": true,
      "expiration_date": "2025-04-27T15:30:45Z",
      "created_at": "2025-03-27T15:30:45Z",
      "is_read": false
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

### 2. Mark Payment Notification as Read

**Endpoint:** `POST /api/payment/notifications/read`

**Description:** Marks a specific payment notification as read.

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

### 3. Mark All Payment Notifications as Read

**Endpoint:** `POST /api/payment/notifications/read-all`

**Description:** Marks all payment notifications for a specific user as read.

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

### 4. Get Payment Statistics

**Endpoint:** `GET /api/payment/stats`

**Description:** Retrieves statistics about payments and subscriptions.

**Authentication:** Required (JWT)

**Response (200 OK):**
```json
{
  "total_revenue": 15600000,
  "revenue_today": 1200000,
  "active_subscribers": 78,
  "new_subscribers_today": 5,
  "by_tier": [
    {"tier": "1GB", "count": 45, "revenue": 4500000},
    {"tier": "5GB", "count": 25, "revenue": 7500000},
    {"tier": "10GB", "count": 8, "revenue": 3600000}
  ],
  "recent_transactions": [
    {
      "pubkey": "subscriber1_pubkey",
      "amount": 100000,
      "tier": "1GB",
      "date": "2025-03-28T11:45:23Z"
    },
    {
      "pubkey": "subscriber2_pubkey",
      "amount": 300000,
      "tier": "5GB",
      "date": "2025-03-28T10:30:15Z"
    }
  ]
}
```

## Client-Side LLM Integration Guide

This section provides guidance on how to prompt a client-side LLM to interact with the payment notification API.

### Fetching Notifications

```
Make a GET request to /api/payment/notifications with these parameters:
- filter: "unread" (or "all", "user")
- page: 1
- limit: 10
- pubkey: (include only if filter is "user")

Handle these possible response scenarios:
1. Status 200 (OK): The response contains an array of notifications and pagination metadata.
   Extract and display each notification with:
   - Payment amount (formatted in sats or BTC)
   - Transaction date in a readable format
   - The subscription tier
   - Whether it's a new subscription or a renewal
   - Expiration date
   
2. Status 204 (No Content): This means there are no unread notifications.
   Display a message like "No new payment notifications" and disable any "Mark all as read" buttons.
   
3. Status 4xx/5xx: An error occurred. Extract the error message from the response and display it.
```

### Marking Notifications as Read

```
When a user views or dismisses a notification:
1. Send a POST request to /api/payment/notifications/read with:
   {
     "id": [notification_id]
   }
   
2. On successful response, update the UI to reflect that this notification has been read by:
   - Changing its visual appearance (e.g., reducing opacity)
   - Moving it to a "Read" section if your UI separates read/unread notifications
   - Updating any notification counters
   
3. If the user chooses to mark all notifications as read, send a POST request to 
   /api/payment/notifications/read-all with:
   {
     "pubkey": "[user_public_key]"
   }
```

### Displaying Payment Statistics

```
To show payment statistics on the admin dashboard:
1. Make a GET request to /api/payment/stats
2. From the response, create a dashboard that shows:
   - Total revenue (formatted in sats and BTC)
   - Revenue received today
   - Number of active subscribers
   - Number of new subscribers today
   - Revenue breakdown by tier (using a pie or bar chart)
   - A list of recent transactions
   
3. Consider adding time-based filtering options to view statistics for different periods.
```

### Polling for New Notifications

```
To check for new payment notifications periodically:
1. Set up a polling interval (e.g., every 60 seconds)
2. Make a GET request to /api/payment/notifications?filter=unread&page=1&limit=5
3. If the response is 200 OK, update the UI with the new notifications
4. If the response is 204 No Content, no action is needed (there are no new notifications)
5. Reset the polling timer after each check

Adjust the polling frequency based on your application's needs.
```

### UI Presentation Guidelines

```
When presenting payment notifications:
1. For new subscribers, use a distinct visual treatment (e.g., a "New" badge)
2. Format the payment amount in both sats and BTC (e.g., "210,000 sats (0.0021 BTC)")
3. Show the tier information prominently
4. Include the expiration date in a user-friendly format (e.g., "Expires on April 27, 2025")
5. For the notification list, sort by date with newest first
6. Consider grouping by user if showing notifications for multiple subscribers
```
