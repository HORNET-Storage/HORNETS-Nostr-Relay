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
                                        │  Dispute    │
                                        │   Queue     │
                                        └─────────────┘
                                               │
                                               ▼
                    ┌───────────────────────────────────────┐
                    │                                       │
                    ▼                                       ▼
             ┌─────────────┐                        ┌─────────────┐
             │  Automated  │                        │   Admin     │
             │  Processing │                        │   Review    │
             └─────────────┘                        └─────────────┘
                    │                                       │
                    └───────────────────────────────────────┘
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

#### Free vs. Paid Disputes

The system implements a tiered approach to disputes:

- **First Dispute**: Every user gets one free dispute per blocked event. This allows all users to challenge moderation decisions they believe are incorrect.
- **Subsequent Disputes**: If a user wants to submit additional disputes for the same event (e.g., if their first dispute was rejected), they must be a paid subscriber.

This approach balances accessibility with resource management, ensuring all users have access to the dispute system while preventing abuse.

### Step 3: Dispute Processing

The relay now supports two methods of dispute processing:

#### Automated Processing

The relay automatically processes disputes through the following steps:

1. When a dispute is received, it's added to a pending dispute moderation queue
2. The dispute worker periodically processes the queue
3. The media is re-evaluated with dispute-specific parameters:
   - Lower threshold (0.35 instead of 0.4) to give benefit of the doubt
   - Always uses "full" moderation mode for maximum accuracy
   - Takes into account the user's dispute reason
4. Based on the re-evaluation:
   - If approved, the content is automatically unblocked and the ticket is deleted
   - If rejected, the content remains blocked

#### Admin Review

For complex cases or when automated processing is inconclusive:

- Relay administrators can manually review the dispute
- Admins can override the automated decision if necessary
- If approved, the content is unblocked and made available again
- If rejected, the content remains blocked

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

## Technical Implementation

### Automated Dispute Processing

The automated dispute processing system consists of several components:

1. **Dispute Queue**: When a dispute is received, it's added to a pending dispute moderation queue in the database.

2. **Dispute Worker**: A background worker periodically checks for pending disputes and processes them:
   ```go
   // In lib/moderation/image/worker.go
   func (w *Worker) processDispute(dispute lib.PendingDisputeModeration) {
       // Re-evaluate the media with dispute-specific parameters
       response, err := w.ModerationService.ModerateDisputeURL(dispute.MediaURL, dispute.DisputeReason)
       
       // Determine if the dispute should be approved based on the re-evaluation
       approved := !response.ShouldBlock()
       
       // Create a resolution event
       kind19843.CreateResolutionEvent(
           w.Store,
           dispute.DisputeID,
           dispute.TicketID,
           dispute.EventID,
           dispute.UserPubKey,
           approved,
           response.Explanation,
           relayPubKey,
           relayPrivKey,
       )
   }
   ```

3. **Dispute-Specific Moderation**: The system uses a specialized endpoint for re-evaluating disputed content:
   ```go
   // In lib/moderation/image/service.go
   func (s *ModerationService) ModerateDisputeURL(mediaURL string, disputeReason string) (*ModerationResponse, error) {
       // Use a lower threshold (0.35 instead of 0.4)
       // Always use "full" moderation mode
       // Include the dispute reason in the request
       // ...
   }
   ```

4. **Event Handling**: The system includes handlers for all three event types:
   - `kind19841handler.go`: Creates moderation tickets
   - `kind19842handler.go`: Processes dispute events and adds them to the queue
   - `kind19843handler.go`: Handles resolution events

5. **Data Storage**: The system uses several data structures:
   - `BlockedEvent`: Tracks blocked events with a new `HasDispute` flag
   - `PendingDisputeModeration`: Represents disputes waiting for processing

6. **Paid Subscriber Check**: The system checks if a user has already disputed an event and if they are a paid subscriber for subsequent disputes:
   ```go
   // In lib/handlers/nostr/kind19842/kind19842handler.go
   // Check if this user has already disputed this event
   hasDisputed, err := store.HasUserDisputedEvent(blockedEventID, env.Event.PubKey)
   if err != nil {
       log.Printf("Error checking if user has disputed event: %v", err)
       write("NOTICE", "Error processing dispute. Please try again later.")
       return
   }

   // If this is a subsequent dispute, check if the user is a paid subscriber
   if hasDisputed {
       isPaid, err := IsPaidSubscriber(store, env.Event.PubKey)
       if err != nil {
           log.Printf("Error checking paid subscriber status: %v", err)
           write("NOTICE", "Error processing dispute. Please try again later.")
           return
       }

       // If not a paid subscriber, reject the dispute
       if !isPaid {
           write("OK", env.Event.ID, false, "You have already disputed this event. Only paid subscribers can submit multiple disputes for the same event.")
           return
       }
   }
   ```

### Integration Points

The dispute system integrates with the existing moderation system at several points:

1. When content is blocked, the `MarkEventBlockedWithDetails` method creates a moderation ticket.
2. When a dispute is received, the `AddToPendingDisputeModeration` method adds it to the queue.
3. When a dispute is approved, the `UnmarkEventBlocked` method unblocks the content.
4. The `DeleteBlockedEventsOlderThan` method now skips events with active disputes.

### Configuration

The system uses the following configuration from the relay:

- `RelayPubkey`: Public key of the relay for signing resolution events
- `private_key`: Private key of the relay for signing resolution events
- Image moderation settings for re-evaluation
