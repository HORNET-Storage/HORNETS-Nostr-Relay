# NIP-DHT-Keys: DHT Keys for Finding Relay Lists

## Abstract

This NIP defines a mechanism for using Distributed Hash Tables (DHT) to efficiently discover relay lists associated with Nostr users. It enables users to publish their preferred relay lists to the DHT and allows other users to discover these lists, facilitating more efficient content discovery and retrieval of missing notes.

## End-to-End Process Flow

Here's how the complete DHT Keys system works from start to finish:

### 1. User Setup
- User generates DHT key from their nsec (private key)
- User publishes Kind 30078 event with their relay list
- Relay stores relay list in DHT using the user's DHT key

### 2. Content Discovery (Missing Note Scenario)
- Client sends REQ for specific event ID: `["REQ", "sub1", {"ids": ["abc123..."]}]`
- Relay queries local storage - event not found
- Relay detects missing event ID and initiates DHT retrieval
- If REQ includes author filters, relay uses those pubkeys for targeted DHT lookup
- Relay derives DHT key from author's pubkey
- Relay queries DHT to get author's relay list
- Relay connects to author's relays and requests the missing event
- If found, relay stores event locally and includes in response
- Client receives EVENT message with the previously missing note
- Client receives EOSE as normal

### 3. Result
- Missing content is automatically retrieved and delivered
- Future requests for the same event are served from local storage
- No client-side DHT implementation required

## Motivation

In the Nostr ecosystem, users often struggle to discover content that exists on relays they are not connected to. When a user encounters a reference to a note they don't have locally, they need a way to discover which relays might have that note. Current approaches are inefficient and often result in "content not found" experiences.

This NIP proposes a solution using DHT to store and retrieve relay lists associated with users. When a user publishes content to specific relays, they can also publish their relay list to the DHT. Other users can then discover these relay lists and use them to find missing content.

## Specification

### Key Derivation

A DHT key is derived from a user's nsec (private key) using the following algorithm:

1. Extract the raw private key bytes from the nsec
2. Apply SHA1 hash to the private key bytes
3. Encode the resulting bytes as a hexadecimal string

```
dht_key = hex(SHA1(private_key_bytes))
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

**Note**: With real-time missing note retrieval, clients benefit automatically from DHT functionality without needing to implement DHT lookups themselves. The relay handles missing note retrieval transparently.

## Relay Behavior

Relays that support this NIP should:

1. Accept and verify DHT Relay List events (kind 30078)
2. Store relay lists in the DHT when requested
3. Retrieve relay lists from the DHT when requested
4. Implement NIP-42 for relay authentication
5. Handle requests for missing notes from other relays
6. **Integrate missing note retrieval into REQ processing** (HORNETS enhancement)
7. **Apply performance limits** to prevent DHT lookup abuse

## Real-time Missing Note Retrieval

When a client sends a REQ message requesting specific event IDs that are not found locally, the relay automatically attempts to retrieve them via DHT:

### REQ Processing Flow

1. **Local Query**: Process filters and query local storage first
2. **Missing Event Detection**: Identify event IDs that were requested but not found
3. **DHT Retrieval**: For missing events, attempt retrieval using author's relay lists
4. **Response**: Include both local and retrieved events in the normal EVENT/EOSE response

### Performance Safeguards

- **Event Limit**: Maximum 5 missing events per REQ to prevent delays
- **Author Limit**: Maximum 3 author lookups per missing event
- **Timeout Protection**: 10-second timeout per relay connection
- **Strategy Prioritization**: Use author filters when available for targeted DHT lookups

### Integration Points

The missing note retrieval is integrated into the main filter handler (`lib/handlers/nostr/filter/filter.go`):

```go
// After local query, check for missing event IDs
if len(filter.IDs) > 0 {
    foundIDs := make(map[string]bool)
    for _, event := range events {
        foundIDs[event.ID] = true
    }
    
    // Find missing event IDs
    for _, requestedID := range filter.IDs {
        if !foundIDs[requestedID] {
            missingEventIDs = append(missingEventIDs, requestedID)
        }
    }
}

// Attempt DHT retrieval for missing events
if len(missingEventIDs) > 0 && relayStore != nil {
    // Use author filters to target specific relay lists
    for _, authorPubkey := range potentialAuthors {
        response, err := sync.RetrieveMissingNote(eventID, authorPubkey, relayStore, store)
        if response.Found && response.Event != nil {
            combinedEvents = append(combinedEvents, response.Event)
            break
        }
    }
}
```

## Implementation

The HORNETS-Nostr-Relay implements this NIP with the following components:

### DHT Key Derivation

The implementation uses SHA1 for DHT key derivation, consistent with the existing DHT implementation:

```go
// DeriveKeyFromNsec derives a DHT key from a user's nsec (private key)
func DeriveKeyFromNsec(nsec string) (string, error) {
    // Extract private key bytes using the signing package
    privateKeyBytes, err := signing.DecodeKey(nsec)
    if err != nil {
        return "", fmt.Errorf("invalid nsec: %w", err)
    }
    
    // Use SHA1 for target generation (consistent with existing code)
    emptySalt := []byte{}
    target := CreateMutableTarget(privateKeyBytes, emptySalt)
    
    return hex.EncodeToString(target[:]), nil
}

// CreateMutableTarget derives the target (dht-input) for a given pubKey and salt
func CreateMutableTarget(pubKey []byte, salt []byte) krpc.ID {
    return sha1.Sum(append(pubKey[:], salt...))
}
```

### Relay List Signing

The implementation uses the existing signing package for relay list signing:

```go
// SignRelayList signs a relay list with the user's private key
func SignRelayList(relayList []string, nsec string) (string, error) {
    // Use existing DeserializePrivateKey
    privateKey, _, err := signing.DeserializePrivateKey(nsec)
    if err != nil {
        return "", fmt.Errorf("invalid nsec: %w", err)
    }
    
    // Create a canonical JSON representation of the relay list
    relayListJSON, err := json.Marshal(relayList)
    if err != nil {
        return "", fmt.Errorf("failed to marshal relay list: %w", err)
    }
    
    // Use existing SignData function
    signature, err := signing.SignData(relayListJSON, privateKey)
    if err != nil {
        return "", fmt.Errorf("failed to sign relay list: %w", err)
    }
    
    return hex.EncodeToString(signature.Serialize()), nil
}
```

### DHT Storage and Retrieval

The implementation extends the existing `RelayStore` for DHT operations:

```go
// StoreRelayList stores a signed relay list in the DHT
func (rs *RelayStore) StoreRelayList(dhtKey string, relayList []string, pubkey string, signature string) error {
    // Convert the relay list to JSON
    relayListJSON, err := json.Marshal(relayList)
    if err != nil {
        return fmt.Errorf("failed to marshal relay list: %w", err)
    }
    
    // Add to uploadables table using existing method
    return rs.AddUploadable(
        hex.EncodeToString(relayListJSON),
        pubkey,
        signature,
        true, // Upload now
    )
}
```

### Kind 30078 Handler

The implementation includes a handler for kind 30078 events:

```go
// BuildKind30078Handler creates a handler for DHT Relay List events
func BuildKind30078Handler(store stores.Store, relayStore *sync.RelayStore) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
    return func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
        // Read and validate event
        
        // Extract relay list and DHT key
        var relayURLs []string
        var dhtKey string
        
        for _, tag := range event.Tags {
            if len(tag) >= 2 {
                if tag[0] == "r" {
                    relayURLs = append(relayURLs, tag[1])
                } else if tag[0] == "dht" {
                    dhtKey = tag[1]
                }
            }
        }
        
        // Store the relay list in the DHT
        err := relayStore.StoreRelayList(dhtKey, relayURLs, event.PubKey, event.Sig)
        if err != nil {
            write("NOTICE", "Failed to store relay list in DHT")
            return
        }
        
        // Successfully processed event
        write("OK", event.ID, true, "Relay list stored in DHT")
    }
}
```

### Missing Note Retrieval

The implementation includes functionality for retrieving missing notes:

```go
// RetrieveMissingNote retrieves a missing note from relays associated with a user
func RetrieveMissingNote(eventID string, authorPubkey string, relayStore *RelayStore) (*nostr.Event, error) {
    // Get the DHT key for the author
    dhtKey, err := GetDHTKeyForPubkey(authorPubkey)
    if err != nil {
        return nil, fmt.Errorf("failed to get DHT key: %w", err)
    }
    
    // Retrieve the relay list from DHT
    relayInfos, err := relayStore.GetRelayListFromDHT(&dhtKey)
    if err != nil {
        return nil, fmt.Errorf("failed to retrieve relay list: %w", err)
    }
    
    // For each relay, try to retrieve the note
    // ...
}
```

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
5. **Real-time retrieval should be limited** to prevent performance degradation during high-traffic periods
6. **Author-based targeting** improves DHT lookup efficiency when author filters are available
7. **Automatic storage** of retrieved events reduces future DHT lookups for the same content

## HORNETS-Specific Enhancements

The HORNETS-Nostr-Relay implementation includes several enhancements beyond the base NIP specification:

### Automatic Missing Note Detection
- Integrated into standard REQ processing
- No client-side DHT implementation required
- Transparent to existing Nostr clients

### Performance Optimizations
- Configurable limits on DHT lookups per request
- Author-based targeting when filters include author pubkeys
- Automatic local storage of retrieved events
- Timeout protection for all external relay connections

### Logging and Monitoring
- Detailed logging of DHT operations with color-coded output
- Success/failure tracking for retrieved events
- Performance metrics for DHT lookup times

This implementation makes DHT functionality accessible to all Nostr clients without requiring client-side modifications, while maintaining compatibility with the standard Nostr protocol.

## References

1. NIP-01: Basic protocol flow description
2. NIP-42: Authentication of clients to relays
3. SHA1: Secure Hash Algorithm 1
