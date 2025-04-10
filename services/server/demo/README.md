# HORNETS Relay Demo Server

This is a special version of the HORNETS relay designed for demonstration purposes only. It runs just the admin panel portion of the relay with authentication bypassed, allowing easy demonstration of the relay's administration interface without requiring login.

## Important Security Note

**NEVER** run this demo server in a production environment or on a public-facing server. This version bypasses all authentication which makes it unsuitable for any real-world deployment.

## Features

- **Authentication Bypass**: All panel routes are open without requiring login
- **Isolated Environment**: Uses separate data directory and database files
- **Port Management**: Automatically runs on a different port than the main relay
- **Simplified**: Runs only the web panel without the full relay functionality

## How to Run

1. Copy the demo configuration:
   ```
   cp demo-config.json config.json
   ```
   Or create a configuration file with `"demo_mode": true`

2. Run the demo server:
   ```
   go run services/server/demo/main.go
   ```

3. Access the admin panel at:
   ```
   http://localhost:10002
   ```
   (Default port is 10000 for the demo server, and the web panel runs on port+2)

## Configuration

The demo server uses these key settings:

- `demo_mode: true` - Enables authentication bypass
- `port: 10000` - Uses a different port than the main relay (9000)
- `proxy: false` - Disables the websocket proxy since it's not needed for panel demo
- Various feature settings (content filtering, moderation) disabled for simplicity

## Implementation Details

The demo server implementation:

1. Sets up a dedicated data directory to avoid conflicts with the main relay
2. Loads same configuration but with demo mode enabled by default
3. Utilizes the security bypass code in the web server component
4. Properly cleans up resources on exit
5. Shows clear console warnings about running in demo mode

This implementation allows you to demonstrate the admin panel UI and functionality without needing to run the full relay infrastructure.
