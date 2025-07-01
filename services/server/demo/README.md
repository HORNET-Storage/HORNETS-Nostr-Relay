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

### Using Build Scripts

Two build scripts are provided to simplify setting up and running the demo tools:

1. **Build the demo server:**

```bash
./build-demo.sh
```

This script builds the demo server and places the executable `hornet-storage-demo` in the root directory, and creates a `demo-config.json` in the root directory if it doesn't exist.

2. **Build the demo data generator:**

```bash
./build-demo-generator.sh
```

This script builds the generator executable as `hornet-demo-generator` in the root directory.

### Running the Demo Server

After building, you can run the demo server from the root directory:

```bash
./hornet-storage-demo
```

Or run it directly with:

```bash
go run services/server/demo/main.go
```

Then access the admin panel at:

```
http://localhost:10002
```

The demo server will:
- Read configuration from `demo-config.json` in the project root (if available)
- Use port 10000 (or the port specified in config + 1000)
- Serve the web panel on port 10002 (base port + 2)
- Store data in `demo_statistics.db` in the project root

## Adding Dummy Data

The demo server includes a built-in demo data generator that can populate the `demo_statistics.db` database with realistic sample data:

### Using the Demo Data Generator

After building with `build-demo-generator.sh`, run:

```bash
./hornet-demo-generator [--auto]
```

Or navigate to the generator directory and run:

```bash
cd services/server/demo/demodata
./demo-generator [--auto]
```

The generator:
- Automatically uses `demo_statistics.db` in the project root directory
- With `--auto` flag, it generates data using default settings
- Without flags, it provides an interactive menu to customize data generation

### Demo Data Includes

The generator creates realistic sample data for:
- User profiles with lightning addresses and DHT keys
- Event kinds (notes) with text and media content
- Payment notifications with subscription tiers
- Month-by-month data for historical trend visualization

### Cleanup Tool

If you end up with multiple database files in different directories, you can use the cleanup script:

```bash
cd services/server/demo/demodata
./cleanup.sh
```

This script will:
- List all SQLite database files in the project
- Keep only `demo_statistics.db` in the project root
- Remove all other database files

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
