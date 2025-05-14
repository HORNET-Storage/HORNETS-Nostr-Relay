# NIP-DHT-Keys: DHT Keys for Finding Relay Lists

## Abstract

This NIP defines a mechanism for using Distributed Hash Tables (DHT) to efficiently discover relay lists associated with Nostr users. It enables users to publish their preferred relay lists to the DHT and allows other users to discover these lists, facilitating more efficient content discovery and retrieval of missing notes.

## Motivation

In the Nostr ecosystem, users often struggle to discover content that exists on relays they are not connected to. When a user encounters a reference to a note they don't have locally, they need a way to discover which relays might have that note. Current approaches are inefficient and often result in "content not found" experiences.

This NIP proposes a solution using DHT to store and retrieve relay lists associated with users. When a user publishes content to specific relays, they can also publish their relay list to the DHT. Other users can then discover these relay lists and use them to find missing content.

## Specification

### Key Derivation

A DHT key is derived from a user's nsec (private key) using the following algorithm:

1. Extract the raw private key bytes from the nsec
2. Apply HKDF-SHA256 with a specific info string "nostr:dht-key"
3. Encode the resulting bytes as a hexadecimal string

```
dht_key = hex(HKDF-SHA256(private_key_bytes, info="nostr:dht-key"))
```

### Relay List Format

A relay list is a JSON object with the following structure:

```json
{
  "pubkey": "hex_encoded_public_key",
  "relays": ["relay_url_1", "relay_url_2", ...],
  "created_at": unix_timestamp,
  "signature": "signature_of_the_relay_list"
}
```

The signature is created by signing the canonical JSON representation of the relay list (without the signature field) using the user's private key.

### Publishing Relay Lists

To publish a relay list to the DHT:

1. Generate a DHT key from the user's nsec
2. Create a relay list object
3. Sign the relay list with the user's private key
4. Send the signed relay list to a relay that supports this NIP
5. The relay verifies the signature and publishes the relay list to the DHT using the DHT key

### Retrieving Relay Lists

To retrieve a relay list from the DHT:

1. Obtain the user's pubkey and DHT key
2. Query the DHT using the DHT key
3. Verify the signature on the retrieved relay list
4. Use the relay list to connect to the specified relays

### Missing Note Retrieval

When a client encounters a reference to a missing note:

1. Extract the author's pubkey from the reference
2. Obtain the DHT key associated with the author (either from local storage or from metadata)
3. Retrieve the author's relay list from the DHT
4. Connect to the relays in the list and request the missing note
5. If the note is found, cache it locally

### Relay Authentication

When a relay needs to request a note from another relay:

1. Establish a connection to the target relay
2. Authenticate using NIP-42
3. Request the missing note
4. If authentication fails, fall back to unauthenticated requests if supported

## Event Kinds

This NIP defines a new event kind:

- Kind 30078: DHT Relay List

This event has the following structure:

```json
{
  "id": "event_id",
  "pubkey": "author_pubkey",
  "created_at": unix_timestamp,
  "kind": 30078,
  "tags": [
    ["r", "relay_url_1"],
    ["r", "relay_url_2"],
    ...
    ["dht", "dht_key"]
  ],
  "content": "",
  "sig": "signature"
}
```

The "r" tags contain the relay URLs in the user's relay list.
The "dht" tag contains the DHT key where the relay list is stored.

## Client Behavior

Clients that support this NIP should:

1. Generate and store DHT keys for their users
2. Publish relay lists to the DHT when users update their preferred relays
3. Query the DHT for relay lists when encountering missing notes
4. Cache retrieved relay lists for efficiency

## Relay Behavior

Relays that support this NIP should:

1. Accept and verify DHT Relay List events (kind 30078)
2. Store relay lists in the DHT when requested
3. Retrieve relay lists from the DHT when requested
4. Implement NIP-42 for relay authentication
5. Handle requests for missing notes from other relays

## Security Considerations

1. **Key Security**: Private keys should never be transmitted or stored insecurely
2. **Signature Verification**: All signatures must be verified before trusting relay lists
3. **DHT Security**: The DHT implementation should be resistant to poisoning and eclipse attacks
4. **Rate Limiting**: Relays should implement rate limiting to prevent abuse
5. **Privacy**: Users should be aware that their relay preferences are publicly discoverable

## Compatibility

This NIP is compatible with existing Nostr clients and relays. Clients and relays that do not support this NIP will simply ignore the new event kind and continue to function as before.

## Implementation Notes

1. DHT implementations should consider caching to improve performance
2. Relay lists should be periodically refreshed to ensure they remain current
3. Fallback mechanisms should be implemented for cases where DHT lookups fail
4. Consideration should be given to the size and frequency of DHT operations to minimize network overhead

## References

1. NIP-01: Basic protocol flow description
2. NIP-42: Authentication of clients to relays
3. HKDF-SHA256: RFC 5869
