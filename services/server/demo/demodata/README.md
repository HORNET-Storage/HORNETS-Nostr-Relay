# HORNETS-Nostr-Relay Demo Data Generator

This tool generates realistic demo data for the HORNETS-Nostr-Relay demo mode. It populates the statistics database with synthetic data that can be used for visualizing various charts and metrics in the relay's admin panel.

## Features

- Generates user profiles with configurable adoption rates of lightning addresses and DHT keys
- Creates event kinds (notes) with realistic size distributions
- Supports both text and media notes for "Notes vs Media" charts
- Generates payment notifications with subscription tiers
- Provides month-by-month data generation for historical trend visualization
- Customizable growth patterns and distributions

## Generated Data Types

The generator creates the following types of data:

1. **User Profiles**
   - With or without lightning addresses
   - With or without DHT keys
   - Month-by-month growth patterns

2. **Event Kinds (Notes)**
   - Various note kinds (1, 3, 4, 5, 7, 1063, 30023)
   - Text notes and media notes
   - Realistic size distributions

3. **Payment Notifications**
   - Various subscription tiers (1GB, 5GB, 10GB, 50GB, 100GB)
   - New and returning subscribers

## Usage

### Building

To build the generator, run the included build script:

```bash
./build.sh
```

This will create an executable named `demo-generator` in the current directory.

### Running

Run the generator using:

```bash
./demo-generator [options]
```

By default, the generator uses the same database file as the demo server (`demo_statistics.db` in the project root), so you don't need to specify any options.

Options:
- `--db path/to/statistics.db` - Path to SQLite database (default: `demo_statistics.db` in project root)
- `--auto` - Run with default settings without interaction

### Interactive Mode

By default, the generator runs in interactive mode, providing a menu-driven interface to:

1. Generate all types of demo data
2. Generate specific types of data (user profiles, event kinds, payment notifications)
3. Configure time ranges for data generation
4. Configure growth patterns and distributions
5. View current settings

Example of the interactive menu:

```
HORNETS-Nostr-Relay Demo Data Generator
=======================================
Main Menu:
1. Generate All Demo Data (User Profiles, Events, Files)
2. Generate User Profiles Only (for Address Adoption charts)
3. Generate Event Kinds Only (for Gigabytes Per Month charts)
4. Generate Payment Notifications
5. Configure Time Range
6. Configure Growth Patterns
7. Configure Kind Distribution
8. Show Current Settings
9. Reset to Defaults
0. Exit
```

### Auto Mode

For automated usage, you can use the `--auto` flag to generate data with default settings:

```bash
./demo-generator --auto
```

## Default Settings

The default settings generate:
- 2 years of data ending at the current month
- Starting with 100 users, growing at 15% monthly
- 30% lightning address adoption rate
- 20% DHT key adoption rate
- 10% of users having both
- 50 initial notes per day, growing at 10% monthly
- 20% of notes containing media
- Various kinds of notes with different distributions

These settings can be customized through the interactive menu if needed.

## Integration with Demo Mode

This generator is designed to work with the HORNETS-Nostr-Relay demo mode. The data it produces will populate the charts and statistics displayed in the relay's admin panel, providing a realistic view of how the relay would look in a production environment.
