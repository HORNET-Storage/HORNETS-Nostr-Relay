# H.O.R.N.E.T Allowed Users - Frontend Integration Guide

## Overview

The H.O.R.N.E.T Allowed Users feature introduces a new unified access control system that replaces and extends the existing subscription model. This guide explains how to implement the frontend integration and how it relates to existing relay settings.

## New Page Structure: "Allowed Users"

### Navigation
- **Location**: Hamburger menu → "Allowed Users"
- **Purpose**: Centralized user permission management
- **Replaces**: The subscription management scattered across different areas

### Page Layout

The Allowed Users page features three distinct operational modes with a consistent interface:

```
┌─────────────────────────────────────────────────────┐
│ H.O.R.N.E.T Allowed Users                          │
├─────────────────────────────────────────────────────┤
│ Mode: [Free Mode ▼] [Paid Mode ▼] [Exclusive Mode ▼]│
├─────────────────────────────────────────────────────┤
│ Permissions:                                        │
│ Read:  ☐ [Mode-specific options]                    │
│ Write: ☐ [Mode-specific options]                    │
├─────────────────────────────────────────────────────┤
│ Tiers: (applies to all modes)                      │
│ [Tier management interface]                        │
├─────────────────────────────────────────────────────┤
│ User Lists: (when applicable)                      │
│ [NPUB management interface]                        │
└─────────────────────────────────────────────────────┘
```

## API Integration

### 1. Settings Management

**Endpoint**: `/api/settings/allowed_users`

```javascript
// GET - Retrieve current settings
const response = await fetch('/api/settings/allowed_users');
const data = await response.json();
const settings = data.allowed_users;

// POST - Update settings
await fetch('/api/settings/allowed_users', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    allowed_users: {
      mode: "free", // "free", "paid", "exclusive"
      read_access: {
        enabled: true,
        scope: "all_users" // "all_users", "paid_users", "allowed_users"
      },
      write_access: {
        enabled: true,
        scope: "allowed_users" // "paid_users", "allowed_users"
      },
      tiers: [
        { data_limit: "1 GB per month", price: "1000" },
        { data_limit: "5 GB per month", price: "5000" }
      ]
    }
  })
});
```

### 2. NPUB Management

**Read List Management**:
```javascript
// GET paginated read NPUBs
const readNpubs = await fetch('/api/allowed-npubs/read?page=1&pageSize=20');

// POST add NPUB to read list
await fetch('/api/allowed-npubs/read', {
  method: 'POST',
  body: JSON.stringify({ npub: "npub1...", tier: "basic" })
});

// DELETE remove from read list
await fetch('/api/allowed-npubs/read/npub1...', { method: 'DELETE' });
```

**Write List Management**:
```javascript
// Similar endpoints for write list
const writeNpubs = await fetch('/api/allowed-npubs/write?page=1&pageSize=20');
// POST /api/allowed-npubs/write
// DELETE /api/allowed-npubs/write/:npub
```

**Bulk Import**:
```javascript
await fetch('/api/allowed-npubs/bulk-import', {
  method: 'POST',
  body: JSON.stringify({
    type: "read", // or "write"
    npubs: ["npub1...:basic", "npub2...:premium"]
  })
});
```

## Migration from Relay Settings

### What Moved Where

| **Old Location** | **New Location** | **Notes** |
|------------------|------------------|-----------|
| `relay_settings.subscription_tiers` | `allowed_users.tiers` | **MOVED**: Tiers now apply to all modes |
| `relay_settings.FreeTierEnabled` | `allowed_users.mode = "free"` | **REPLACED**: Free tier is now a mode |
| `relay_settings.FreeTierLimit` | `allowed_users.tiers[0]` (when free mode) | **INTEGRATED**: Free tier is first tier in free mode |
| Whitelist management (config files) | `allowed_users` + NPUB APIs | **REPLACED**: Database-stored NPUBs with UI management |

### Backward Compatibility

The backend maintains backward compatibility:
- Old `relay_settings.subscription_tiers` will continue to work
- New `allowed_users.tiers` takes precedence when present
- Subscription system reads from both locations with fallback

### Implementation Strategy

1. **Phase 1**: Implement the new Allowed Users page alongside existing settings
2. **Phase 2**: Add migration helper to move existing subscription tiers
3. **Phase 3**: Update existing subscription displays to reference new location

## Mode-Specific Implementation

### Free Mode
```javascript
const freeModeConfig = {
  mode: "free",
  read_access: {
    enabled: true,
    scope: "all_users" // or "allowed_users"
  },
  write_access: {
    enabled: true,
    scope: "all_users" // or "allowed_users"
  },
  tiers: [
    { data_limit: "100 MB per month", price: "0" }, // Free tier
    { data_limit: "1 GB per month", price: "0" },   // Optional additional free tiers
  ]
};
```

**UI Elements**:
- Read/Write checkboxes with dropdown options
- Warning message when enabling public read access
- Free tier configuration (optional)
- No NPUB lists needed for "all users" scope

### Paid Mode
```javascript
const paidModeConfig = {
  mode: "paid",
  read_access: {
    enabled: true,
    scope: "all_users" // "all_users" or "paid_users"
  },
  write_access: {
    enabled: true,
    scope: "paid_users" // Always "paid_users"
  },
  tiers: [
    { data_limit: "1 GB per month", price: "1000" },
    { data_limit: "5 GB per month", price: "5000" }
  ]
};
```

**UI Elements**:
- Read scope: dropdown with "All Users" or "Paid Users"
- Write scope: fixed to "Paid Users"
- Paid tier configuration (no free tiers allowed)
- Automatic NPUB management via payments
- Payment integration display

### Exclusive Mode
```javascript
const exclusiveModeConfig = {
  mode: "exclusive",
  read_access: {
    enabled: true,
    scope: "allowed_users" // "allowed_users" or "all_users"
  },
  write_access: {
    enabled: true,
    scope: "allowed_users" // Always "allowed_users"
  },
  tiers: [
    { data_limit: "5 GB per month", price: "0" },   // Exclusive tiers
    { data_limit: "50 GB per month", price: "0" }
  ]
};
```

**UI Elements**:
- Read scope: dropdown with "Allowed Users" or "All Users"
- Write scope: fixed to "Allowed Users"
- Manual NPUB list management
- Bulk import/export functionality
- Tier assignment to NPUBs

## Component Architecture

### 1. AllowedUsersPage Component
```javascript
const AllowedUsersPage = () => {
  const [settings, setSettings] = useState(null);
  const [mode, setMode] = useState('free');
  
  return (
    <div>
      <ModeSelector mode={mode} onModeChange={setMode} />
      <PermissionsConfig mode={mode} settings={settings} />
      <TiersConfig settings={settings} />
      {(mode === 'exclusive') && <NPubManagement />}
    </div>
  );
};
```

### 2. PermissionsConfig Component
```javascript
const PermissionsConfig = ({ mode, settings }) => {
  const readOptions = getReadOptions(mode);
  const writeOptions = getWriteOptions(mode);
  
  return (
    <div className="permissions-grid">
      <div>
        <label>Read:</label>
        <input type="checkbox" checked={settings.read_access.enabled} />
        <select value={settings.read_access.scope}>
          {readOptions.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
        </select>
      </div>
      <div>
        <label>Write:</label>
        <input type="checkbox" checked={settings.write_access.enabled} />
        <select value={settings.write_access.scope}>
          {writeOptions.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
        </select>
      </div>
    </div>
  );
};
```

### 3. NPubManagement Component
```javascript
const NPubManagement = () => {
  const [readNpubs, setReadNpubs] = useState([]);
  const [writeNpubs, setWriteNpubs] = useState([]);
  const [showBulkImport, setShowBulkImport] = useState(false);
  
  return (
    <div>
      <div className="npub-section">
        <h3>Read Access List</h3>
        <NPubList npubs={readNpubs} type="read" />
        <button onClick={() => setShowBulkImport(true)}>Bulk Import</button>
      </div>
      
      <div className="npub-section">
        <h3>Write Access List</h3>
        <NPubList npubs={writeNpubs} type="write" />
      </div>
      
      {showBulkImport && <BulkImportModal onClose={() => setShowBulkImport(false)} />}
    </div>
  );
};
```

## State Management

### Redux/Context Pattern
```javascript
const allowedUsersSlice = createSlice({
  name: 'allowedUsers',
  initialState: {
    settings: null,
    readNpubs: [],
    writeNpubs: [],
    loading: false
  },
  reducers: {
    setSettings: (state, action) => {
      state.settings = action.payload;
    },
    setReadNpubs: (state, action) => {
      state.readNpubs = action.payload;
    },
    addReadNpub: (state, action) => {
      state.readNpubs.push(action.payload);
    },
    removeReadNpub: (state, action) => {
      state.readNpubs = state.readNpubs.filter(n => n.npub !== action.payload);
    }
  }
});
```

## Error Handling

### Common Error Scenarios
```javascript
const handleSettingsUpdate = async (newSettings) => {
  try {
    await updateSettings(newSettings);
    showSuccess('Settings updated successfully');
  } catch (error) {
    if (error.status === 400) {
      showError('Invalid settings configuration');
    } else if (error.message.includes('access control not initialized')) {
      showError('Please restart the relay after configuration changes');
    } else {
      showError('Failed to update settings');
    }
  }
};
```

## Validation

### Frontend Validation Rules
```javascript
const validateSettings = (settings) => {
  const errors = [];
  
  // Mode validation
  if (!['free', 'paid', 'exclusive'].includes(settings.mode)) {
    errors.push('Invalid mode selected');
  }
  
  // Tier validation
  if (settings.mode === 'paid' && settings.tiers.some(t => t.price === '0')) {
    errors.push('Paid mode cannot have free tiers');
  }
  
  // Scope validation
  if (settings.mode === 'paid' && settings.write_access.scope !== 'paid_users') {
    errors.push('Paid mode write access must be limited to paid users');
  }
  
  return errors;
};
```

## Testing Considerations

### Integration Tests
```javascript
describe('Allowed Users Integration', () => {
  test('should update mode and reflect in WebSocket connections', async () => {
    // Test mode changes affect real-time connections
  });
  
  test('should maintain backward compatibility with relay settings', async () => {
    // Test that old subscription tiers still work
  });
  
  test('should handle NPUB bulk import correctly', async () => {
    // Test bulk operations
  });
});
```

## Migration Helper

Consider implementing a migration helper component:

```javascript
const MigrationHelper = () => {
  const [oldSettings, setOldSettings] = useState(null);
  
  const migrateFromRelaySettings = async () => {
    // Read relay_settings.subscription_tiers
    // Convert to allowed_users.tiers
    // Show preview and confirm migration
  };
  
  return (
    <div className="migration-helper">
      <h3>Migrate from Relay Settings</h3>
      <p>We detected existing subscription tiers in your relay settings.</p>
      <button onClick={migrateFromRelaySettings}>Migrate Now</button>
    </div>
  );
};
```

## Notes for Frontend Team

1. **Tiers are now universal**: Unlike before where tiers were only for paid mode, tiers now apply to all three modes
2. **NPUB storage changed**: NPUBs are now stored in the database with pagination, not in JSON config files
3. **Settings structure**: Use the generic settings API pattern already established
4. **Real-time updates**: Consider WebSocket integration for live NPUB list updates
5. **Access control feedback**: The system now provides real-time feedback when users are denied access

This implementation provides a comprehensive foundation for the three-mode access control system while maintaining backward compatibility with existing functionality.
