# HORNETS Relay Demo Server

This is a dedicated server for running the HORNETS admin panel in demo mode, with authentication bypassed to allow easy access for demonstration purposes.

## Features

- **Always Runs in Demo Mode**: Authentication is always bypassed regardless of config settings
- **Separate Data Directory**: Uses "demo-data" instead of "data" to avoid conflicts with the production relay
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

## How It Works

The demo server:
1. Sets default `demo_mode` to true
2. Forces `demo_mode` to true after loading config (ignoring any config file settings)
3. Uses the authentication bypass logic in the web server component
4. Disables unnecessary features for a cleaner demo experience
5. Shows clear console warnings about running in demo mode

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

Which enforces authentication regardless of config settings.
