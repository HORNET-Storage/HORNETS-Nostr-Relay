# HORNETS Relay Demo Server

This is a dedicated server for running the HORNETS admin panel in demo mode, with authentication bypassed to allow easy access for demonstration purposes.

## Features

- **Always Runs in Demo Mode**: Authentication is always bypassed regardless of config settings
- **Separate Data Directory**: Uses "demo-data" instead of "data" to avoid conflicts with the production relay
- **Separate Statistics Database**: Uses "demo_statistics.db" instead of the production statistics database
- **Different Port**: Runs on port 10000 by default (web panel on 10002)
- **Simplified Setup**: Only runs the web panel, not the full relay functionality
- **Clear Warning Logs**: Shows prominent warnings that this is a demo version

## Usage

Simply run:

```bash
go run services/server/demo/main.go
```

Then access the admin panel at:

```
http://localhost:10002
```

## Adding Dummy Data

The demo server uses a separate statistics database file (`demo_statistics.db`), which you can pre-populate with dummy data for demonstration purposes:

1. Create a script to generate dummy data in the demo database
2. Run the script before starting the demo server
3. The panel will display this dummy data when accessed

Example of a script that could add dummy data (concept only):
```go
// Connect to demo_statistics.db
// Insert sample kinds, events, transactions, etc.
// Save and close connection
```

## How It Works

The demo server:
1. Sets default `demo_mode` to true
2. Forces `demo_mode` to true after loading config (ignoring any config file settings)
3. Uses the authentication bypass logic in the web server component
4. Initializes a separate statistics database (`demo_statistics.db`)
5. Disables unnecessary features for a cleaner demo experience
6. Shows clear console warnings about running in demo mode

## Security Warning

**NEVER** run this server in production or on a public-facing server. It deliberately bypasses security features to make demonstration easier.

This should only be used for:
- Feature demonstrations
- UI testing
- Training sessions
- Local development

## Production Use

For production deployment, always use the regular server:

```bash
go run services/server/port/main.go
```

Which enforces authentication and uses the production database regardless of config settings.
