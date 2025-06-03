# Blossom API Guide: Image Upload for Nostr with NIP-98 Authentication

This guide explains how to use the Blossom API with NIP-98 HTTP authentication for secure file uploads in your Nostr applications.

## Overview

Blossom is a Nostr-compatible file storage protocol implemented in the HORNETS Nostr Relay. It provides a simple way to upload and retrieve files, particularly useful for profile pictures and image attachments in notes.

The protocol follows a two-step process:
1. Create a Kind 117 event with file metadata
2. Upload the actual file data to the Blossom API endpoint using NIP-98 authentication

## NIP-98 Authentication

As of this implementation, file uploads require NIP-98 HTTP authentication. This provides cryptographic proof that the uploader owns the private key associated with their public key.

### Creating a NIP-98 Auth Event

```javascript
async function createNIP98AuthEvent(url, method, privateKey, payload = null) {
  const event = {
    kind: 27235,
    created_at: Math.floor(Date.now() / 1000),
    tags: [
      ["u", url],
      ["method", method]
    ],
    content: "",
    pubkey: getPublicKey(privateKey)
  };
  
  // Add payload hash for PUT/POST/PATCH requests
  if (payload && (method === "PUT" || method === "POST" || method === "PATCH")) {
    const hashBuffer = await crypto.subtle.digest('SHA-256', payload);
    const hashHex = Array.from(new Uint8Array(hashBuffer))
      .map(b => b.toString(16).padStart(2, '0'))
      .join('');
    event.tags.push(["payload", hashHex]);
  }
  
  // Sign the event
  const signedEvent = await signEvent(event, privateKey);
  
  // Base64 encode for Authorization header
  const authHeader = "Nostr " + btoa(JSON.stringify(signedEvent));
  return authHeader;
}
```

## API Endpoints

### Upload File
- **URL**: `PUT /blossom/upload`
- **Method**: `PUT`
- **Headers**: 
  - `Authorization: Nostr <base64-encoded-nip98-event>`
- **Body**: Raw file bytes (not FormData)
- **Response**: 
  - `200 OK` with JSON: `{"message": "File uploaded successfully", "hash": "<file-hash>", "size": <bytes>, "type": "<mime-type>"}`
  - `401 Unauthorized` if authentication fails
  - `400 Bad Request` if no matching Kind 117 event exists

### Download File
- **URL**: `GET /blossom/:hash`
- **Method**: `GET`
- **Path Parameters**:
  - `hash`: SHA-256 hash of the file in hex format
- **Headers**: None required (public access)
- **Response**: Raw file bytes with appropriate Content-Type header

## Implementation Guide

### Step 1: Create Kind 117 Event

Before uploading a file, you must create and publish a Kind 117 event containing the file's metadata:

```javascript
// 1. Calculate SHA-256 hash of the file
const arrayBuffer = await imageFile.arrayBuffer();
const hashBuffer = await crypto.subtle.digest('SHA-256', arrayBuffer);
const hashHex = Array.from(new Uint8Array(hashBuffer))
  .map(b => b.toString(16).padStart(2, '0'))
  .join('');

// 2. Create Kind 117 event
const kind117Event = {
  kind: 117,
  created_at: Math.floor(Date.now() / 1000),
  tags: [
    ["blossom_hash", hashHex],
    ["name", imageFile.name],
    ["size", imageFile.size.toString()],
    ["type", imageFile.type]
  ],
  content: `File metadata for ${imageFile.name}`,
  pubkey: userPublicKey
};

// 3. Sign and publish the event
const signedEvent = await signEvent(kind117Event);
relay.publish(signedEvent);
```

### Step 2: Upload File Data with NIP-98 Auth

After the Kind 117 event is published and confirmed by the relay, upload the actual file:

```javascript
// 4. Create NIP-98 auth event
const authHeader = await createNIP98AuthEvent(
  `https://your-relay.com:9002/blossom/upload`,
  "PUT",
  privateKey,
  await imageFile.arrayBuffer()
);

// 5. Upload the file data
const response = await fetch(
  `https://your-relay.com:9002/blossom/upload`, 
  {
    method: 'PUT',
    headers: {
      'Authorization': authHeader
    },
    body: imageFile  // Raw file data
  }
);

if (response.ok) {
  const result = await response.json();
  console.log("File uploaded successfully!", result);
} else {
  console.error("Upload failed:", await response.json());
}
```

### Using the Image in a Profile (Kind 0)

```javascript
const profileEvent = {
  kind: 0,
  content: JSON.stringify({
    name: "User Name",
    picture: `https://your-relay.com:9002/blossom/${hashHex}`,
    // ... other profile fields
  }),
  tags: [],
  pubkey: userPublicKey
};

// Sign and publish
const signedProfileEvent = await signEvent(profileEvent);
relay.publish(signedProfileEvent);
```

### Using the Image in a Note (Kind 1)

```javascript
const noteEvent = {
  kind: 1,
  content: "Check out this image!",
  tags: [
    ["imeta", 
     `url https://your-relay.com:9002/blossom/${hashHex}`,
     `blurhash ${blurhash}`,
     `dim 1024x768`
    ]
  ],
  pubkey: userPublicKey
};

// Sign and publish
const signedNoteEvent = await signEvent(noteEvent);
relay.publish(signedNoteEvent);
```

## Security Considerations

- **NIP-98 Authentication**: Cryptographically proves ownership of the public key
- **Timestamp Validation**: Auth events must be within 60 seconds of current time
- **Replay Protection**: Each auth event is unique to the specific request
- **Hash Verification**: The relay verifies that the uploader owns a matching Kind 117 event
- **File Integrity**: The relay recalculates the hash to ensure file integrity
- **MIME Type Detection**: The relay detects the actual MIME type regardless of extension
- **Public Downloads**: Anyone can download files by hash without authentication

## Error Handling

Common error responses:

- `400 Bad Request`: Invalid public key
- `404 Not Found`: File not found
- `500 Internal Server Error`: Various server-side errors, including:
  - No matching Kind 117 event
  - Hash mismatch
  - Storage failure

## Example Implementation

Here's a complete example using vanilla JavaScript with NIP-98 authentication:

```javascript
async function uploadProfileImage(imageFile, privateKey) {
  // 1. Calculate SHA-256 hash
  const arrayBuffer = await imageFile.arrayBuffer();
  const hashBuffer = await crypto.subtle.digest('SHA-256', arrayBuffer);
  const hashHex = Array.from(new Uint8Array(hashBuffer))
    .map(b => b.toString(16).padStart(2, '0'))
    .join('');
  
  // 2. Derive public key from private key
  const userPublicKey = getPublicKey(privateKey);
  
  // 3. Create and sign Kind 117 event
  const kind117Event = {
    kind: 117,
    created_at: Math.floor(Date.now() / 1000),
    tags: [
      ["blossom_hash", hashHex],
      ["name", imageFile.name],
      ["size", imageFile.size.toString()],
      ["type", imageFile.type]
    ],
    content: `File metadata for ${imageFile.name}`,
    pubkey: userPublicKey
  };
  
  const signedEvent = await signEvent(kind117Event, privateKey);
  
  // 4. Publish to relay
  const relay = new WebSocket('wss://your-relay.com');
  
  await new Promise(resolve => {
    relay.onopen = () => {
      relay.send(JSON.stringify(["EVENT", signedEvent]));
      resolve();
    };
  });
  
  // 5. Wait a moment for the relay to process the event
  await new Promise(resolve => setTimeout(resolve, 1000));
  
  // 6. Create NIP-98 auth event
  const uploadUrl = `https://your-relay.com:9002/blossom/upload`;
  const authEvent = {
    kind: 27235,
    created_at: Math.floor(Date.now() / 1000),
    tags: [
      ["u", uploadUrl],
      ["method", "PUT"],
      ["payload", hashHex]  // Include file hash as payload
    ],
    content: "",
    pubkey: userPublicKey
  };
  
  const signedAuthEvent = await signEvent(authEvent, privateKey);
  const authHeader = "Nostr " + btoa(JSON.stringify(signedAuthEvent));
  
  // 7. Upload the file with NIP-98 auth
  const response = await fetch(uploadUrl, {
    method: 'PUT',
    headers: {
      'Authorization': authHeader
    },
    body: imageFile
  });
  
  if (!response.ok) {
    const error = await response.json();
    throw new Error(`Upload failed: ${error.message || error.error}`);
  }
  
  const result = await response.json();
  
  // 8. Return the result
  return {
    hash: result.hash,
    url: `https://your-relay.com:9002/blossom/${result.hash}`,
    size: result.size,
    type: result.type
  };
}
```

## NIP-98 Event Example

Here's what a NIP-98 auth event looks like:

```json
{
  "id": "fe964e758903360f28d8424d092da8494ed207cba823110be3a57dfe4b578734",
  "pubkey": "63fe6318dc58583cfe16810f86dd09e18bfd76aabc24a0081ce2856f330504ed",
  "content": "",
  "kind": 27235,
  "created_at": 1682327852,
  "tags": [
    ["u", "https://your-relay.com:9002/blossom/upload"],
    ["method", "PUT"],
    ["payload", "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3"]
  ],
  "sig": "5ed9d8ec958bc854f997bdc24ac337d005af372324747efe4a00e24f4c30437ff4dd8308684bed467d9d6be3e5a517bb43b1732cc7d33949a3aaf86705c22184"
}
```

## Limitations

- Maximum file size is determined by your relay's configuration
- File types may be restricted by your relay's moderation settings
- Files are stored on the relay and may be subject to the relay's data retention policies
