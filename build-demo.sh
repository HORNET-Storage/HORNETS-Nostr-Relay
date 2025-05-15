#!/bin/bash

# Change directory
cd services/server/demo

# Build the Go program
go build -o ../../../hornet-storage-demo

# Copy demo config if it doesn't exist
if [ ! -f ../../../demo-config.json ]; then
    echo "Creating demo-config.json in root directory..."
    cp config.json ../../../demo-config.json
fi

# Pause equivalent (wait for user input)
read -p "Press enter to continue"
