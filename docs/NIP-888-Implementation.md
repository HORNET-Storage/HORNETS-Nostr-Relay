# NIP-888: Robust Implementation of Tiered Subscription Payments

## Introduction to Enhanced Payment Processing

While our subscription system is designed with a straightforward payment flow in the client interface, we recognized the need to make our implementation robust against various payment scenarios. Users might:

- Pay amounts that don't exactly match our tier prices
- Send payments exceeding our highest tier
- Make multiple small payments over time
- Directly interact with their assigned Bitcoin address outside of the intended flow

Rather than hoping users follow the ideal payment path, we've built a resilient system that maximizes value in any payment scenario. Our enhanced payment processing ensures that:

1. Users always get maximum storage capacity for their payments
2. No funds are "lost" in the system or underutilized
3. Even unusual payment patterns are handled gracefully

The following documentation details both the standard flow and the robust payment handling mechanisms that work behind the scenes.

## Bitcoin Payment Implementation

### Step 1: Accessing the Relay List in the App

The user begins by navigating to the Relay List within their app, such as NostrBox or Nestr. This list displays the relays to which the user is already connected and serves as a management interface for interacting with these relays.

### Step 2: Entering a New Relay

If the user wishes to connect to a new relay that is not already on their list, they manually enter the new relay's URL or identifier into the app. This action initiates the connection process to the newly specified relay.

### Step 3: Sending NIP42 Authentication to the Relay

Upon connecting to the new relay, the user's client automatically sends a NIP42 authentication request. This authentication step verifies the user's identity and establishes a secure session with the relay.

### Step 4: Displaying and Selecting a Subscription Plan

Once the authentication is successful, the relay presents the user with available subscription options by issuing a kind 10411 nostr note (previously referred to as kind 88 in early implementations). Each subscription tier is listed in the content section of the note, providing details on the data limit and the corresponding price.

To facilitate payment tracking and automatic user registration, the relay generates a unique Bitcoin address for each user. This unique address is specifically tied to the user's payment and is crucial for correlating the payment with the specific user in the backend system.

The kind 10411 nostr note structure would be as follows:

```json
{
  "id": "<unique_note_id>",
  "pubkey": "<relay_public_key_hex>",
  "created_at": <timestamp>,
  "kind": 10411,
  "tags": [],
  "content": {
    "name": "Relay Name",
    "description": "Relay Description",
    "pubkey": "<relay_public_key_hex>",
    "contact": "admin@relay.com",
    "supported_nips": [1, 11, 2, 9, 18, 23, 24, 25, 51, 56, 57, 42, 45, 50, 65, 116],
    "software": "hornets-relay",
    "version": "1.0.0",
    "dhtkey": "<relay_dht_key>",
    "subscription_tiers": [
      {"data_limit": "1 GB per month", "price": "10000"},
      {"data_limit": "5 GB per month", "price": "40000"},
      {"data_limit": "10 GB per month", "price": "70000"}
    ]
  },
  "sig": "<signature_from_relay>"
}
```

When a user first connects to the relay and authenticates, the system automatically initializes a subscription record by creating a kind 11888 note specifically for that user. This occurs during the initialization process and before any tier selection or payment. The kind 11888 event includes a unique Bitcoin address assigned to the user for payment tracking:

```json
{
  "id": "<unique_note_id>",
  "pubkey": "<relay_public_key_hex>",
  "created_at": <timestamp>,
  "kind": 11888,
  "tags": [
    ["subscription_duration", "1 month"],
    ["p", "<user_pubkey_hex>"],
    ["subscription_status", "inactive"],
    ["relay_bitcoin_address", "<unique_bitcoin_address_for_payment>"],
    ["relay_dht_key", "<relay_dht_key>"],
    ["storage", "0", "0", "<timestamp>"],
    ["credit", "<credit_amount_in_sats>"],
    ["active_subscription", "<tier_name>", "<expiration_timestamp>"],
    ["relay_mode", "public|subscription|invite-only|only-me"]
  ],
  "content": "",
  "sig": "<signature_from_relay>"
}
```

This initial kind 11888 event serves as a subscription record that will track the user's status, allocated storage, payment history, and credit. If the relay doesn't offer a public tier, the event starts with "inactive" status and zero storage allocation until a payment is received. However, if the relay has configured a public tier, the user's subscription would start with "active" status and would be allocated the storage amount specified in the public tier configuration, without requiring any payment.

### Step 5: Enhanced User Payment Process

The user reviews the subscription tiers and sends payment to their assigned Bitcoin address. Our enhanced payment system supports multiple payment scenarios:

#### a) Standard Tier Purchase
If the payment exactly matches a tier price, the system activates that tier, allocating the corresponding storage capacity.

#### b) Cascading Multi-Tier Purchase
If the payment exceeds a tier price, our cascading payment system:
1. First allocates the highest tier the payment can fully purchase
2. If multiple periods of the highest tier can be purchased, it extends the subscription duration accordingly
3. Any remainder is then used to purchase additional lower tiers in descending order
4. Only truly unusable amounts (below the lowest tier price) are stored as credit

For example, if a user pays 85,000 sats with tier prices of 70,000, 40,000, and 10,000 sats:
- The system allocates 1 period of the 70,000 tier (highest tier)
- The remaining 15,000 sats is then used to purchase 1 period of the 10,000 tier
- The final 5,000 sats is stored as credit

#### c) Credit Accumulation and Auto-Application
For payments smaller than any tier price, or for remainders after tier purchases, the system stores the amount as credit. When accumulated credit reaches a tier threshold, it's automatically applied to purchase additional storage.

The updated kind 11888 event includes credit information:

```json
{
  "id": "<unique_note_id>",
  "pubkey": "<relay_public_key_hex>",
  "created_at": <timestamp>,
  "kind": 11888,
  "tags": [
    ["subscription_duration", "1 month"],
    ["p", "<user_pubkey_hex>"],
    ["subscription_status", "active"],
    ["relay_bitcoin_address", "<unique_bitcoin_address_for_payment>"],
    ["relay_dht_key", "<relay_dht_key>"],
    ["storage", "<used_bytes>", "<total_bytes>", "<timestamp>"],
    ["credit", "<credit_amount_in_sats>"],
    ["active_subscription", "10 GB per month", "<expiration_timestamp>"]
  ],
  "content": "",
  "sig": "<signature_from_relay>"
}
```

### Step 6: Intelligent Payment Verification and Allocation

Upon receiving a Bitcoin payment, the relay performs several operations:

1. **Payment Verification**: The system matches the payment to the unique Bitcoin address and verifies the transaction.

2. **Intelligent Allocation Logic**:
   - The system determines the optimal way to allocate the payment across available tiers
   - For payments exceeding the highest tier, it calculates how many periods to extend
   - For payments between tier thresholds, it implements the cascading purchase logic
   - Any unusable remainder is stored as credit for future use

3. **Storage Capacity Calculation**:
   - Each tier translates to a specific storage allocation in bytes
   - For multi-tier purchases, the system accumulates the storage from all purchased tiers
   - The total is added to any existing storage allocation the user may have

4. **Subscription Record Update**:
   - The subscription's expiration date is updated based on the periods purchased
   - For multi-period purchases, the expiration is extended accordingly
   - All changes are recorded in an updated kind 11888 event

## Credit Management System

Our credit management system ensures that no payment is wasted, regardless of amount:

### Credit Accumulation
Any payment amount that cannot purchase a full tier is stored as credit associated with the user's account. This includes:
- Direct payments below any tier threshold
- Remainders after tier purchases
- Unusable fragments from the cascading payment process

### Automatic Credit Application
The system continually evaluates accumulated credit:
1. When a new payment is processed, existing credit is added to the payment amount
2. After any tier purchase, the system checks if remaining credit can purchase additional tiers
3. If credit reaches or exceeds a tier threshold, it's automatically converted to storage
4. The system applies credit to purchase the highest possible tier(s)
5. This process happens recursively until no more tiers can be purchased

### Credit Visibility in NIP-888 Events
Credit information is always included in the user's kind 11888 event, providing transparency:
- A `credit` tag displays the current credit amount in satoshis
- This tag is updated after every transaction or credit application
- The credit is visible to other relays and clients that may need this information

## Unlimited Storage Support

For certain relay configurations (such as only-me mode or invite-only users), unlimited storage may be granted. This is indicated in the kind 11888 event by using "unlimited" as the total bytes value in the storage tag:

### Standard Storage Tag
```json
["storage", "<used_bytes>", "<total_bytes>", "<timestamp>"]
```

### Unlimited Storage Tag
```json
["storage", "<used_bytes>", "unlimited", "<timestamp>"]
```

When a user has unlimited storage:
- The `used_bytes` field still tracks actual usage for monitoring purposes
- The `total_bytes` field is set to the string "unlimited"
- Storage enforcement is bypassed for these users
- This is typically used for only-me relay owners and privileged invite-only users

## Lightning Network Implementation (Future)

*[Note: The Lightning Network implementation remains unchanged from the original gist and is planned for future development.]*

### Step 1: Accessing the Relay List in the App

The user begins by navigating to the Relay List within their app, such as NostrBox or Nestr. This list displays the relays to which the user is already connected and serves as a management interface for interacting with these relays.

### Step 2: Entering a New Relay

If the user wishes to connect to a new relay that is not already on their list, they manually enter the new relay's URL or identifier into the app. This action initiates the connection process to the newly specified relay.

### Step 3: Sending NIP42 Authentication to the Relay

Upon connecting to the new relay, the user's client automatically sends a NIP42 authentication request. This authentication step verifies the user's identity and establishes a secure session with the relay.

### Step 4: Displaying Subscription Plans

Once the authentication is successful, the relay presents the user with available subscription options by issuing a kind 10411 nostr note. Each subscription tier is listed in the content section, providing details on the data limit and the corresponding price.

The kind 10411 nostr note structure would be as follows:

```json
{
  "id": "<unique_note_id>",
  "pubkey": "<relay_public_key_hex>",
  "created_at": <timestamp>,
  "kind": 10411,
  "tags": [],
  "content": {
    "name": "Relay Name",
    "description": "Relay Description",
    "pubkey": "<relay_public_key_hex>",
    "contact": "admin@relay.com",
    "supported_nips": [1, 11, 2, 9, 18, 23, 24, 25, 51, 56, 57, 42, 45, 50, 65, 116],
    "software": "hornets-relay",
    "version": "1.0.0",
    "dhtkey": "<relay_dht_key>",
    "subscription_tiers": [
      {"data_limit": "1 GB per month", "price": "10000"},
      {"data_limit": "5 GB per month", "price": "40000"},
      {"data_limit": "10 GB per month", "price": "70000"}
    ]
  },
  "sig": "<signature_from_relay>"
}
```

### Step 5: User Selection and Event Signing

The user reviews the available subscription tiers and selects their desired tier by creating and signing a kind 11888 nostr event. This event specifies the chosen subscription tier and includes the user's pubkey (in hex format) along with other necessary information. The relay's DHT key is included to identify the relay the user is subscribing to. Once signed, this event is sent to the relay.

The kind 11888 nostr event structure would be as follows:

```json
{
  "id": "<unique_note_id>",
  "pubkey": "<user_pubkey_hex>",
  "created_at": <timestamp>,
  "kind": 11888,
  "tags": [
    ["subscription-tier", "5 GB per month", "40000"],
    ["subscription-duration", "1 month"],
    ["relay-dht-key", "<relay_dht_key>"]
  ],
  "content": "",
  "sig": "<signature_from_user>"
}
```

### Step 6: Generating the Lightning Invoice

Upon receiving the signed kind 11888 event, the relay generates a Lightning Network (LN) invoice corresponding to the selected tier's price. The invoice is dynamically created based on the user's choice and is sent back to the user through the relay.

### Step 7: User Payment Process for Lightning Network Transactions

The user reviews the Lightning invoice received from the relay. The user then pays the invoice using their preferred Lightning wallet. The payment amount must exactly match the amount specified in the invoice for the relay to register it as a successful payment.

### Step 8: Payment Verification and Subscription Activation

Upon receiving the Lightning payment, the relay verifies the transaction. Once verified, the relay activates the subscription by registering the user's pubkey (in hex format) for the selected data size and the 1-month period.

The relay records the subscription's expiration date in the panel when you click on the person's name.
A graviton profile bucket can be made for that user to monitor if they exceed the allocated GB they are assigned, using the same GB counting logic the panel currently utilizes for its charts.
To ensure proper access management, the relay periodically checks each subscriber's expiration date. Users whose subscriptions have expired are automatically removed from the active users' list, suspending their access until they renew their subscription.

## Relay Operating Modes

The HORNETS relay supports four distinct operating modes, each with different access control and subscription behaviors. The active mode is indicated in the `relay_mode` tag of kind 11888 events.

### 1. Subscription Mode (Paid)

**Description**: Users must pay for storage tiers to access the relay.

**Access Control**: 
- Read: `paid_users` (only users with active paid subscriptions)
- Write: `paid_users` (only users with active paid subscriptions)

**Subscription Behavior**:
- Users start with minimal/no storage allocation
- Must make Bitcoin payments to purchase storage tiers
- Storage allocation matches purchased tier specifications
- Bitcoin addresses are automatically allocated for payment tracking
- Credit system handles overpayments and partial payments

**Kind 11888 Example**:
```json
{
  "kind": 11888,
  "pubkey": "<relay_public_key>",
  "created_at": <timestamp>,
  "tags": [
    ["subscription_duration", "1 month"],
    ["p", "<user_pubkey>"],
    ["subscription_status", "active"],
    ["relay_bitcoin_address", "<bitcoin_address>"],
    ["relay_dht_key", "<relay_dht_key>"],
    ["storage", "1073741824", "5368709120", "<timestamp>"],
    ["credit", "0"],
    ["active_subscription", "5GB Tier", "<expiration_timestamp>"],
    ["relay_mode", "subscription"]
  ],
  "content": "",
  "sig": "<relay_signature>"
}
```

### 2. Public Mode (Free)

**Description**: Open access relay where anyone can read and write with predefined storage limits.

**Access Control**:
- Read: `all_users` (anyone can read)
- Write: `all_users` (anyone can write)

**Subscription Behavior**:
- Users automatically receive the configured free tier allocation
- No payment required
- No Bitcoin address allocation
- Storage limits enforced based on free tier configuration

**Kind 11888 Example**:
```json
{
  "kind": 11888,
  "pubkey": "<relay_public_key>",
  "created_at": <timestamp>,
  "tags": [
    ["subscription_duration", "1 month"],
    ["p", "<user_pubkey>"],
    ["subscription_status", "active"],
    ["relay_bitcoin_address", ""],
    ["relay_dht_key", "<relay_dht_key>"],
    ["storage", "52428800", "104857600", "<timestamp>"],
    ["credit", "0"],
    ["active_subscription", "Free Public", "<expiration_timestamp>"],
    ["relay_mode", "public"]
  ],
  "content": "",
  "sig": "<relay_signature>"
}
```

### 3. Invite-Only Mode

**Description**: Curated access where admin manually assigns users to specific tiers.

**Access Control**:
- Read: `allowed_users` or `all_users` (configurable)
- Write: `allowed_users` (only manually approved users)

**Subscription Behavior**:
- Admin manually adds users to allowed list with specific tier assignments
- Users receive storage allocation based on their assigned tier
- Tier assignments stored in database and reflected in kind 11888 events
- No payment required - allocations are administrative decisions
- No Bitcoin address allocation

**Kind 11888 Example**:
```json
{
  "kind": 11888,
  "pubkey": "<relay_public_key>",
  "created_at": <timestamp>,
  "tags": [
    ["subscription_duration", "1 month"],
    ["p", "<user_pubkey>"],
    ["subscription_status", "active"],
    ["relay_bitcoin_address", ""],
    ["relay_dht_key", "<relay_dht_key>"],
    ["storage", "2147483648", "10737418240", "<timestamp>"],
    ["credit", "0"],
    ["active_subscription", "Premium Invite", "<expiration_timestamp>"],
    ["relay_mode", "invite-only"]
  ],
  "content": "",
  "sig": "<relay_signature>"
}
```

### 4. Only-Me Mode (Personal)

**Description**: Private relay for owner use only.

**Access Control**:
- Read: `only-me`, `all_users`, or `allowed_users` (configurable)
- Write: `only-me` (only relay owner)

**Subscription Behavior**:
- Relay owner gets unlimited storage
- Non-owners receive no allocation (0 bytes)
- Owner status determined by database configuration or config fallback
- No payment system active
- No Bitcoin address allocation

**Kind 11888 Example (Owner)**:
```json
{
  "kind": 11888,
  "pubkey": "<relay_public_key>",
  "created_at": <timestamp>,
  "tags": [
    ["subscription_duration", "1 month"],
    ["p", "<owner_pubkey>"],
    ["subscription_status", "active"],
    ["relay_bitcoin_address", ""],
    ["relay_dht_key", "<relay_dht_key>"],
    ["storage", "1073741824", "unlimited", "<timestamp>"],
    ["credit", "0"],
    ["active_subscription", "Owner Unlimited", "<expiration_timestamp>"],
    ["relay_mode", "only-me"]
  ],
  "content": "",
  "sig": "<relay_signature>"
}
```

**Kind 11888 Example (Non-Owner)**:
```json
{
  "kind": 11888,
  "pubkey": "<relay_public_key>",
  "created_at": <timestamp>,
  "tags": [
    ["subscription_duration", "1 month"],
    ["p", "<user_pubkey>"],
    ["subscription_status", "inactive"],
    ["relay_bitcoin_address", ""],
    ["relay_dht_key", "<relay_dht_key>"],
    ["storage", "0", "0", "<timestamp>"],
    ["credit", "0"],
    ["active_subscription", "", "<timestamp>"],
    ["relay_mode", "only-me"]
  ],
  "content": "",
  "sig": "<relay_signature>"
}
```

## Mode Transitions and Event Updates

When relay operators change modes, the system automatically updates existing kind 11888 events to reflect the new access policies:

1. **Immediate Updates**: Mode changes trigger batch updates of all subscription events
2. **Graceful Transitions**: Existing allocations are preserved during free-to-paid transitions until cycle ends
3. **Storage Adjustments**: Paid-to-free transitions update storage caps immediately
4. **Bitcoin Address Management**: Addresses allocated only in subscription mode
5. **Owner Privileges**: Owner status maintained across mode changes
