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

## What's Different in Demo Mode

When running in demo mode:

1. JWT authentication is bypassed for all `/api/*` routes
2. API key authentication is bypassed for wallet-specific routes
3. The panel and all its features are accessible without login

The server logs will display warnings when running in demo mode to remind you that authentication is disabled.

## Implementation Details

The demo mode works by conditionally applying authentication middleware based on the `demo_mode` configuration flag. In the main server code (`lib/web/server.go`), we check this flag before applying JWT middleware to secured routes.

```go
// Only apply JWT middleware if not in demo mode
if !viper.GetBool("demo_mode") {
    secured.Use(func(c *fiber.Ctx) error {
        return jwtMiddleware(c, store)
    })
    log.Println("JWT authentication enabled for API routes")
} else {
    log.Println("WARNING: Running in demo mode - API routes are UNSECURED!")
}
