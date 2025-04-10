# HORNETS Relay Demo Mode

This branch contains modifications to run the HORNETS Relay in a demonstration mode with authentication bypassed for the admin panel.

## Purpose

The demo mode allows you to run a version of the relay where admin panel routes are accessible without authentication. This is useful for:

- Demonstration purposes
- Testing the panel interface
- Showcasing the relay's capabilities with dummy data
- Development and UI testing

## Important Security Note

**NEVER** run this mode in production or on a public-facing server. The demo mode bypasses authentication which makes it unsuitable for any real-world deployment.

## How to Use Demo Mode (Preferred Method)

The recommended way to run the demo mode is using the dedicated demo server:

1. Checkout the `demo-mode` branch:
   ```
   git checkout demo-mode
   ```

2. Copy the demo configuration if needed (though the demo server has defaults):
   ```
   cp demo-config.json config.json
   ```

3. Run the dedicated demo server:
   ```
   go run services/server/demo/main.go
   ```

4. Access the admin panel at:
   ```
   http://localhost:10002
   ```
   (The demo server uses port 10000 by default, making the web panel available at port 10002)

## Alternative: Full Relay with Demo Mode

You can also run the full relay with demo mode enabled:

1. Modify your existing `config.json` by adding:
   ```json
   "demo_mode": true
   ```

2. Start the relay as usual:
   ```
   go run services/server/port/main.go
   ```

3. Access the admin panel at:
   ```
   http://localhost:[PORT+2]
   ```
   Where [PORT] is the port defined in your config.json (default is 9000, making the web panel available at port 9002)

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
