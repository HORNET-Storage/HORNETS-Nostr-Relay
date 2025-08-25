#!/bin/bash

echo "🔨 HORNETS Relay Build Script"
echo "============================"

# Change directory
echo "📁 Changing to build directory..."
cd services/server/port

# Build the Go program
echo "🚀 Starting Go build process..."
echo "   Building: services/server/port -> hornet-storage"

# Show build progress
go build -v -o ../../../hornet-storage

# Check if build was successful
if [ $? -eq 0 ]; then
    echo "✅ Build completed successfully!"
    echo "   Executable: ./hornet-storage"
else
    echo "❌ Build failed!"
    exit 1
fi

# Pause equivalent (wait for user input)
echo ""
read -p "Press enter to continue..."