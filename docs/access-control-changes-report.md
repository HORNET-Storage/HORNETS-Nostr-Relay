# Access Control System Changes Report

## Overview

The access control system has been significantly refactored to simplify user management and integrate better with the subscription system. This report outlines all changes that affect the frontend panel.

## Key Changes Made

### 1. Unified User Management

**Before:**
- Separate read/write tables for user permissions
- Individual read/write permissions per user
- Complex permission matrix

**After:**
- Single `AllowedUser` table
- Users have a `tier` field instead of read/write permissions
- Global settings control access permissions

### 2. New Database Schema

#### AllowedUser Table Structure
```go
type AllowedUser struct {
    Npub      string    `json:"npub"`      // User's public key
    Tier      string    `json:"tier"`      // Subscription tier name
    CreatedAt time.Time `json:"created_at"` // When user was added
    CreatedBy string    `json:"created_by"` // Who added the user
}
```

### 3. Configuration Changes

#### New Config Structure
```yaml
allowed_users:
  mode: "public" # only-me, invite-only, public, subscription
  read: "all_users" # all_users, paid_users, allowed_users, only_me
  write: "all_users" # all_users, paid_users, allowed_users, only_me
  tiers:
    - name: "Starter"
      price_sats: 1000
      monthly_limit_bytes: 1073741824  # 1 GB
      unlimited: false
    - name: "Professional"
      price_sats: 5000
      monthly_limit_bytes: 5368709120  # 5 GB
      unlimited: false
```

## API Changes

### Updated Endpoints

#### 1. Get Allowed Users (Paginated)
```
GET /api/allowed-users?page=1&pageSize=20
```

**Response:**
```json
{
  "allowed_users": [
    {
      "npub": "npub1...",
      "tier": "Professional",
      "created_at": "2025-01-01T00:00:00Z",
      "created_by": "admin"
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total_pages": 5,
    "total_items": 100
  }
}
```

#### 2. Add Allowed User
```
POST /api/allowed-users
```

**Request Body:**
```json
{
  "npub": "npub1...",
  "tier": "Professional"
}
```

**Response:**
```json
{
  "success": true,
  "message": "NPUB added to allowed users successfully"
}
```

#### 3. Remove Allowed User
```
DELETE /api/allowed-users
```

**Request Body:**
```json
{
  "npub": "npub1..."
}
```

**Response:**
```json
{
  "success": true,
  "message": "User removed from allowed users successfully"
}
```

## Permission Logic Changes

### Access Control Modes

The system supports four main modes with specific validation rules:

#### 1. **only_me**
- **Description**: Restricts access to the relay owner only
- **Write Permission**: Must be "only_me" (forced)
- **Read Permission**: Can be "only_me", "all_users", or "allowed_users"
- **Use Case**: Personal relay for single user

#### 2. **invite-only**
- **Description**: Access limited to manually invited users
- **Write Permission**: Must be "allowed_users" (forced)
- **Read Permission**: Can be "all_users" or "allowed_users"
- **Use Case**: Private community relay

#### 3. **public**
- **Description**: Open access for everyone
- **Write Permission**: Must be "all_users" (forced)
- **Read Permission**: Must be "all_users" (forced)
- **Use Case**: Public relay with no restrictions

#### 4. **subscription**
- **Description**: Access based on paid subscriptions
- **Write Permission**: Must be "paid_users" (forced)
- **Read Permission**: Can be "all_users" or "paid_users"
- **Use Case**: Commercial relay with subscription tiers

### Permission Types

The system uses four permission types for read/write access:

#### 1. **all_users**
- **Description**: Everyone can access (no restrictions)
- **Implementation**: Returns immediately without any checks

#### 2. **allowed_users**
- **Description**: Only users in the AllowedUser table can access
- **Implementation**: Checks if user exists in database

#### 3. **paid_users** (NEW IMPLEMENTATION)
- **Description**: Only users with active paid subscriptions can access
- **Implementation**: 
  - Checks if user exists in `AllowedUser` table
  - Checks if user exists in `PaidSubscriber` table
  - Verifies subscription hasn't expired
  - Confirms user has a valid tier assigned

#### 4. **only_me**
- **Description**: Only the relay owner can access
- **Implementation**: Compares user's public key with relay owner's key

### Mode Validation Logic

The system automatically validates and corrects invalid combinations:

- **only_me**: Forces write to "only_me", allows read to be "only_me", "all_users", or "allowed_users"
- **invite-only**: Forces write to "allowed_users", allows read to be "all_users" or "allowed_users"
- **public**: Forces both read and write to "all_users"
- **subscription**: Forces write to "paid_users", allows read to be "all_users" or "paid_users"

## Frontend Implementation Guide

### 1. User Management Interface Changes

#### Remove These Elements:
- ❌ Individual read/write checkboxes per user
- ❌ Separate read/write user lists
- ❌ Read/write permission toggles

#### Add These Elements:
- ✅ Tier selection dropdown when adding users
- ✅ Tier display column in user list
- ✅ Global mode/read/write settings controls

### 2. User Management Workflow

#### Adding a User:
1. User enters npub
2. User selects tier from dropdown (populated from config tiers)
3. Call `POST /api/allowed-users` with npub and tier
4. Refresh user list

#### Updating a User's Tier:
1. Call `DELETE /api/allowed-users` to remove user
2. Call `POST /api/allowed-users` with new tier
3. Refresh user list

#### Removing a User:
1. Call `DELETE /api/allowed-users` with npub
2. Refresh user list

### 3. Settings Interface

#### Global Access Control Settings:
```javascript
const accessSettings = {
  mode: "subscription", // dropdown: only-me, invite-only, public, subscription
  read: "paid_users",   // dropdown: all_users, paid_users, allowed_users, only_me
  write: "paid_users"   // dropdown: all_users, paid_users, allowed_users, only_me
};
```

#### Tier Management:
- Display available tiers from config
- Show tier pricing and storage limits
- Allow editing tier configurations

### 4. Validation Rules

#### Mode Validation:
The system automatically validates and corrects invalid combinations:

- **only_me**: Forces write to "only_me", allows read to be "only_me", "all_users", or "allowed_users"
- **invite-only**: Forces write to "allowed_users", allows read to be "all_users" or "allowed_users"
- **public**: Forces both read and write to "all_users"
- **subscription**: Forces write to "paid_users", allows read to be "all_users" or "paid_users"

### 5. UI/UX Recommendations

#### User List Display:
```
| NPUB (truncated) | Tier         | Added By | Date Added | Actions |
|------------------|--------------|----------|------------|---------|
| npub1abc...xyz   | Professional | admin    | 2025-01-01 | Remove  |
| npub1def...uvw   | Starter      | admin    | 2025-01-02 | Remove  |
```

#### Add User Form:
```
┌─────────────────────────────────────┐
│ Add Allowed User                    │
├─────────────────────────────────────┤
│ NPUB: [________________]            │
│ Tier: [Professional ▼]             │
│       [Add User] [Cancel]           │
└─────────────────────────────────────┘
```

#### Global Settings:
```
┌─────────────────────────────────────┐
│ Access Control Settings             │
├─────────────────────────────────────┤
│ Mode:  [subscription ▼]             │
│ Read:  [paid_users ▼]               │
│ Write: [paid_users ▼]               │
│       [Save Settings]               │
└─────────────────────────────────────┘
```

## Subscription Integration

### Paid Users Check
When `read` or `write` is set to `"paid_users"`, the system:

1. Checks if user exists in `AllowedUser` table
2. Checks if user exists in `PaidSubscriber` table
3. Verifies subscription hasn't expired
4. Confirms user has a valid tier assigned

### Kind 888 Events
- Updated when relay starts (if config changed)
- Updated when access settings change via panel
- Contains subscription status and storage information

## Migration Notes

### Database Changes:
- The system now uses a single `AllowedUser` table
- Old read/write tables are no longer used
- `PaidSubscriber` table is used for subscription validation

### Configuration Migration:
- Update config structure to use new `allowed_users` format
- Migrate existing read/write permissions to tier-based system

## Testing Checklist

### Frontend Testing:
- [ ] Add user with different tiers
- [ ] Remove users
- [ ] Update global settings
- [ ] Validate mode combinations
- [ ] Test pagination
- [ ] Verify tier dropdown population

### Integration Testing:
- [ ] Test `paid_users` permission with active subscriptions
- [ ] Test `paid_users` permission with expired subscriptions
- [ ] Test `allowed_users` permission
- [ ] Test mode validation and auto-correction

## Support

For questions about these changes, contact the backend development team. The implementation is complete and ready for frontend integration.

---

**Document Version:** 1.0  
**Last Updated:** 2025-01-27  
**Author:** Backend Development Team
