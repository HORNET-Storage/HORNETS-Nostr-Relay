# NIP-24 Implementation

This document describes the implementation of [NIP-24: Extra metadata fields and tags](https://github.com/nostr-protocol/nips/blob/master/24.md) in the HORNETS Nostr Relay.

## Overview

NIP-24 defines additional optional fields for metadata events (kind 0) and standardizes the meaning of certain tags across different event kinds. This implementation ensures that the relay properly handles these extra fields and tags.

## Metadata Fields (kind 0)

The following additional fields are supported in kind 0 events:

| Field | Type | Description |
|-------|------|-------------|
| `display_name` | string | An alternative, bigger name with richer characters than `name` |
| `website` | string | A web URL related in any way to the event author |
| `banner` | string | An URL to a wide (~1024x768) picture to be optionally displayed in the background of a profile screen |
| `bot` | boolean | A flag to clarify that the content is entirely or partially the result of automation |
| `birthday` | object | An object representing the author's birth date with optional `year`, `month`, and `day` fields |

### Deprecated Fields

The following deprecated fields are handled for backward compatibility:

| Field | Replacement | Description |
|-------|-------------|-------------|
| `displayName` | `display_name` | Use `display_name` instead |
| `username` | `name` | Use `name` instead |

## Tags

The following tags are standardized across different event kinds:

| Tag | Description |
|-----|-------------|
| `r` | A web URL the event is referring to in some way |
| `i` | An external ID the event is referring to in some way |
| `title` | Name of NIP-51 sets, NIP-52 calendar event, NIP-53 live event or NIP-99 listing |
| `t` | A hashtag. The value MUST be a lowercase string |

## Implementation Details

### Metadata Handling

The `UserMetadata` struct in `lib/web/handler_paid_subscribers.go` has been extended to include all NIP-24 fields:

```go
type UserMetadata struct {
    Name        string `json:"name,omitempty"`
    DisplayName string `json:"display_name,omitempty"`
    About       string `json:"about,omitempty"`
    Picture     string `json:"picture,omitempty"`
    
    // New fields from NIP-24
    Website  string `json:"website,omitempty"`
    Banner   string `json:"banner,omitempty"`
    Bot      bool   `json:"bot,omitempty"`
    Birthday struct {
        Year  int `json:"year,omitempty"`
        Month int `json:"month,omitempty"`
        Day   int `json:"day,omitempty"`
    } `json:"birthday,omitempty"`
    
    // Deprecated fields (for backward compatibility)
    DeprecatedDisplayName string `json:"displayName,omitempty"`
    DeprecatedUsername    string `json:"username,omitempty"`
}
```

The profile handling logic has been updated to handle deprecated fields as fallbacks:

```go
// Check for display_name first, then fall back to name if not present
if metadata.DisplayName != "" {
    profile.Name = metadata.DisplayName
} else if metadata.Name != "" {
    profile.Name = metadata.Name
} else if metadata.DeprecatedDisplayName != "" {
    // Fall back to deprecated displayName if no other name is available
    profile.Name = metadata.DeprecatedDisplayName
} else if metadata.DeprecatedUsername != "" {
    // Fall back to deprecated username as last resort
    profile.Name = metadata.DeprecatedUsername
}
```

### Tag Handling

The kind0 handler has been extended with functions to extract and validate NIP-24 tags:

```go
// NIP24Tags represents the tags defined in NIP-24
type NIP24Tags struct {
    ReferenceURLs []string // 'r' tags - web URLs the event is referring to
    ExternalIDs   []string // 'i' tags - external IDs the event is referring to
    Titles        []string // 'title' tags - names of NIP-51 sets, NIP-52 calendar events, etc.
    Hashtags      []string // 't' tags - hashtags (always lowercase)
}

// ExtractNIP24Tags extracts the tags defined in NIP-24 from a Nostr event
func ExtractNIP24Tags(event *nostr.Event) *NIP24Tags {
    // Implementation details...
}

// ValidateNIP24Tags validates that the NIP-24 tags in an event follow the specification
func ValidateNIP24Tags(event *nostr.Event) bool {
    // Implementation details...
}
```

The handler validates that hashtags are lowercase as required by the NIP-24 specification, but still processes events with invalid tags, logging a warning message.

## Usage

When a kind 0 event is received, the relay:

1. Validates the event signature and kind
2. Validates NIP-24 tags if present (but still processes the event even if NIP-24 tags are missing or invalid)
3. Extracts and logs NIP-24 tags for debugging (if any are present)
4. Processes the event normally

**Important Note**: All NIP-24 fields and tags are completely optional. Kind 0 events without any NIP-24 fields or tags will still be processed normally. The implementation is designed to be fully backward compatible with basic metadata events.

When retrieving user profiles, the relay:

1. Extracts all NIP-24 fields from the metadata (if present)
2. Uses deprecated fields as fallbacks when needed
3. Logs additional NIP-24 fields for debugging

## Future Enhancements

Potential future enhancements to the NIP-24 implementation:

1. Add support for searching profiles by NIP-24 fields
2. Extend the profile response structure to include additional NIP-24 fields
3. Implement tag-based filtering for events
