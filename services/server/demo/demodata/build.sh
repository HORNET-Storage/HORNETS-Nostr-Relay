#!/bin/bash

# Build the demo data generator
echo "Building demo data generator..."
cd "$(dirname "$0")"
go build -o demo-generator ./cmd/generator
echo "Build complete!"

echo
echo "To run the demo data generator, use:"
echo "./demo-generator [--db path/to/statistics.db] [--auto]"
echo
echo "Options:"
echo "  --db    Path to SQLite database (default: demo_statistics.db)"
echo "  --auto  Run with default settings without interaction"
