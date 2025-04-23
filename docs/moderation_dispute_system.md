# Moderation Dispute System

This document explains the moderation dispute system implemented in the HORNETS Nostr Relay. The system allows users to dispute content moderation decisions when they believe their content was incorrectly flagged.

## Overview

The dispute system consists of three main components:

1. **Moderation Tickets (Kind 19841)**: Created automatically when content is blocked by the moderation system.
2. **Disputes (Kind 19842)**: Created by users who want to challenge a moderation decision.
3. **Resolutions (Kind 19843)**: Created by the relay to respond to disputes.

## Workflow

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Content   │     │ Moderation  │     │   Dispute   │
│   Blocked   │────▶│   Ticket    │────▶│  Submitted  │
└─────────────┘     └─────────────┘     └─────────────┘
                                               │
                                               ▼
                                        ┌─────────────┐
                                        │  Admin      │
                                        │  Review     │
                                        └─────────────┘
                                               │
                                               ▼
                                        ┌─────────────┐
                                        │ Resolution  │
                                        │ (Approved/  │
                                        │  Rejected)  │
                                        └─────────────┘
```

### Step 1: Content Moderation

When content is blocked by the moderation system (e.g., due to image moderation), a moderation ticket (Kind 19841) is automatically created. This ticket serves as a record of the moderation action and allows the user to dispute it.

### Step 2: User Disputes

If a user believes their content was incorrectly flagged, they can create a dispute event (Kind 19842) that references the moderation ticket. The dispute should include a reason why the user believes the moderation decision was incorrect.

### Step 3: Admin Review

Relay administrators review the dispute and make a decision:

- If approved, the content is unblocked and made available again.
- If rejected, the content remains blocked.

### Step 4: Resolution

The relay creates a resolution event (Kind 19843) that notifies the user of the decision. This event includes the outcome and optionally a reason for the decision.

## Event Types

### Moderation Ticket (Kind 19841)

Created automatically when content is blocked.

```json
{
  "kind": 19841,
  "pubkey": "<relay_pubkey>",
  "tags": [
    ["e", "<blocked_event_id>"],
    ["p", "<user_pubkey>"],
    ["blocked_reason", "Failed image moderation"],
    ["content_level", "0"],
    ["media_url", "https://example.com/image.jpg"],
    ["status", "blocked"]
  ],
  "content": ""
}
```

### Dispute (Kind 19842)

Created by users to challenge a moderation decision.

```json
{
  "kind": 19842,
  "pubkey": "<user_pubkey>",
  "tags": [
    ["e", "<ticket_event_id>"],
    ["reason", "This image does not violate any content policies"]
  ],
  "content": "I believe this moderation decision was incorrect because..."
}
```

### Resolution (Kind 19843)

Created by the relay to respond to disputes.

```json
{
  "kind": 19843,
  "pubkey": "<relay_pubkey>",
  "tags": [
    ["e", "<dispute_event_id>", "dispute"],
    ["e", "<ticket_event_id>", "ticket"],
    ["e", "<original_event_id>", "original"],
    ["p", "<user_pubkey>"],
    ["resolution", "approved|rejected"],
    ["reason", "Optional reason for the decision"]
  ],
  "content": "Your dispute has been approved. The content has been unblocked and is now available."
}
```

## Client Implementation Guide

### Finding Moderation Tickets

Clients can search for moderation tickets related to a user by querying for Kind 19841 events with the user's pubkey in the "p" tag:

```json
{
  "kinds": [19841],
  "authors": ["<relay_pubkey>"],
  "#p": ["<user_pubkey>"]
}
```

### Creating a Dispute

To create a dispute, the client should:

1. Find the moderation ticket for the blocked content
2. Create a Kind 19842 event that references the ticket
3. Include a reason for the dispute
4. Sign and publish the event

### Checking Resolution Status

Clients can check the status of their disputes by querying for Kind 19843 events that reference their dispute events:

```json
{
  "kinds": [19843],
  "authors": ["<relay_pubkey>"],
  "#e": ["<dispute_event_id>"]
}
```

## Admin Interface

Relay administrators can review and resolve disputes through the admin interface. The interface allows admins to:

1. View all pending disputes
2. Review the disputed content
3. Approve or reject the dispute
4. Provide a reason for the decision

## Integration with Moderation System

The dispute system is integrated with the existing moderation system:

- When content is blocked, a moderation ticket is automatically created
- When a dispute is approved, the content is automatically unblocked
- The system maintains a record of all moderation actions and disputes

## Security Considerations

- Only the relay can create moderation tickets and resolutions
- Only the content owner can dispute their own content
- All events are signed and verified to ensure authenticity
