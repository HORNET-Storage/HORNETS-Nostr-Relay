#!/bin/bash

# Change directory to web/panel
cd web/panel

# Install npm dependencies
npm install

# Change back to the parent directory
cd ../..

# Change directory to services/server
cd services/server

# Build the Go binary
go build -o ../../hornet-storage

# Optional: Print a message before exiting
echo "Build process completed."

# Optional: Pause (press Enter to continue)
read -p "Press Enter to exit."
