# NIP-09 Cascade Delete Extension

An extension to NIP-09 (Event Deletion Request) adding support for cascade deletions of related events and DAGs.

## Motivation

NIP-09 allows deleting individual events by ID. However, composite resources like git repositories consist of many related events (pushes, branches, PRs, issues) and DAG data (bundles, archives). Deleting these one-by-one is impractical. This extension enables deleting all resources associated with an identifier in a single request.

## New Tag Types

### `c` - Cascade Delete Events by Tag Match

```json
["c", "<tag-name>", "<tag-value>"]
```

Deletes all events containing the specified tag with the matching value. Single-letter tag enables client querying via `#c`.

### `d` - Cascade Delete DAGs

```json
["d", "<tag-name>"]
```

Before deleting events, extracts DAG roots from the specified tag in matching events and deletes those DAGs. Single-letter tag enables client querying via `#d`.

## Security Model

### Ownership Verification

The tag value **must** begin with the requestor's pubkey followed by a separator (`:` or `/`):

```
<owner-pubkey>:<resource-name>
<owner-pubkey>/resource/path
```

The relay extracts the pubkey prefix and verifies it matches the deletion event's signer. This ensures only the resource owner can trigger cascade deletion.

### Implicit Consent

When a user creates an event with a tag like `["r", "someone-else:repo"]`, they are:
1. Associating their event with a resource owned by `someone-else`
2. Implicitly granting that owner the ability to cascade-delete their event
3. Acknowledging the owner controls the resource lifecycle

This is analogous to contributing to someone's GitHub repository—if they delete the repo, your contributions go with it.

### Temporal Boundary (Tombstone)

The deletion event is stored as a tombstone. Only events created **before** the tombstone's `created_at` are deleted. This:
- Prevents deletion of future events
- Allows the same identifier to be reused for a new resource
- Provides an audit trail for clients

## Example: Git Repository Deletion

### Repository Structure

A git repository creates multiple event types:

| Kind | Purpose | Tags |
|------|---------|------|
| 16629 | Permission/Metadata | `["r", "pubkey:repo-name"]` |
| 73 | Push Events | `["r", "pubkey:repo-name"]`, `["bundle", "<dag-root>"]`, `["archive", "<dag-root>"]` |
| 16630 | Branch Events | `["r", "pubkey:repo-name"]` |

### Deletion Request

```json
{
  "kind": 5,
  "pubkey": "<owner-pubkey>",
  "created_at": 1734889200,
  "tags": [
    ["c", "r", "<owner-pubkey>:my-repo"],
    ["d", "bundle"],
    ["d", "archive"]
  ],
  "content": "Deleting repository",
  "sig": "<signature>"
}
```

### Execution Flow

1. **Verify signature** - Standard NIP-01 signature validation
2. **Verify ownership** - Extract `<owner-pubkey>` from tag value, confirm it matches event pubkey
3. **Collect DAG roots** - Query events with `["r", "<owner-pubkey>:my-repo"]`, extract `bundle` and `archive` tag values
4. **Delete events** - Remove all events with matching `r` tag created before tombstone
5. **Delete DAGs** - Remove all collected DAG data
6. **Store tombstone** - Persist the Kind 5 event for future reference

### Result

All repository events and data are deleted. If the owner creates a new repository with the same name afterward, it's treated as a separate resource (events have `created_at` after the tombstone).

## Supported Tag Value Formats

```
pubkey:name           → owner: pubkey
pubkey/name           → owner: pubkey
pubkey:name:branch    → owner: pubkey
pubkey/path/to/thing  → owner: pubkey
```

The first segment before any `:` or `/` is always the owner pubkey.

## Client Usage

Clients can query for Kind 5 tombstones to determine if a resource was deleted:

```json
{
  "kinds": [5],
  "authors": ["<owner-pubkey>"],
  "#c": ["r", "<owner-pubkey>:repo-name"]
}
```

If a tombstone exists, the client should hide cached data for that resource created before the tombstone's timestamp.
