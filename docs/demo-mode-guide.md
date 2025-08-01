# HORNETS Relay Demo Mode

HORNETS Relay includes a demonstration mode where authentication is bypassed for the admin panel, making it easier to showcase the admin interface without requiring login credentials.

## Purpose

The demo mode allows you to run a version of the relay where admin panel routes are accessible without authentication. This is useful for:

- Demonstration purposes
- Testing the panel interface
- Showcasing the relay's capabilities with dummy data
- Development and UI testing

## Important Security Note

**NEVER** run the demo server in production or on a public-facing server. It bypasses authentication which makes it unsuitable for any real-world deployment.

## How to Run the Demo Server

The demo is implemented as a separate server executable:

```
go run services/server/demo/main.go
```

This demo server:
- Always runs with authentication bypassed, regardless of config settings
- Uses a separate data directory ("demo-data") to avoid conflicts
- Uses a separate statistics database ("demo_statistics.db") for analytics and panel data
- Runs on port 10000 (admin panel on 10002) by default
- Disables unnecessary features for a cleaner demo experience

Access the admin panel at:
```
http://localhost:10002
```

## For Production Use

The regular production server always runs with authentication enabled:

```
go run services/server/port/main.go
```

It forces authentication regardless of what's in the config file, ensuring secure deployment.

Access the authenticated admin panel at:
```
http://localhost:9002
```
(assuming default port 9000)

## Populating Demo Data

The demo server uses a separate statistics database (`demo_statistics.db`), which can be pre-populated with sample data:

1. The statistics database is used for most admin panel visualizations and statistics
2. Since it's a separate file, you can directly manipulate it to show demonstration data
3. Tools like SQLite Browser can be used to view and edit the database directly
4. You can also create scripts to automatically populate it with realistic-looking data

### Demo Data Generator

HORNETS Relay includes a built-in demo data generator tool that can populate the demo database with realistic-looking data:

```
./hornet-demo-generator
```

This interactive tool allows you to:
- Generate user profiles with realistic adoption patterns
- Generate event kinds with realistic distribution
- Generate wallet balance history and transactions
- Generate wallet addresses for receiving payments
- Generate payment notifications
- Configure time ranges and growth patterns

For quick population with default settings, use:

```
./hornet-demo-generator --auto
```

This will populate the database with a complete set of demo data spanning 24 months.

Once populated, the demo server will display this data in the admin panel without requiring authentication.

## What's Different in Demo Mode

When running in demo mode:

1. JWT authentication is bypassed for all `/api/*` routes
2. API key authentication is bypassed for wallet-specific routes 
3. The panel and all its features are accessible without login
4. A separate statistics database is used for panel data

The server logs will display warnings when running in demo mode to remind you that authentication is disabled.

## Implementation Details

The demo mode uses several approaches to separate itself from production:

1. **Authentication Bypass**: Conditionally applies authentication middleware based on the `demo_mode` flag in `lib/web/server.go`:

```go
// Only apply JWT middleware if not in demo mode
if !viper.GetBool("demo_mode") {
    secured.Use(func(c *fiber.Ctx) error {
        return jwtMiddleware(c, store)
    })
    logging.Info("JWT authentication enabled for API routes")
} else {
    logging.Info("WARNING: Running in demo mode - API routes are UNSECURED!")
}
```

2. **Separate Statistics Database**: Uses a distinct database file for demo mode:

```go
// Initializing separate demo statistics database
if err := store.UseDemoStatisticsDB(); err != nil {
    logging.Info("Warning: Failed to switch to demo statistics database: %v", err)
} else {
    logging.Info("Demo server is using demo_statistics.db")
}
```

This ensures complete isolation between demo and production data.
