#!/bin/bash

# Build and Deploy Panel Script
# This script builds the HORNETS-Relay-Panel and copies it to the web directory

set -e

echo "🚀 Building HORNETS-Relay-Panel..."

# Build the RELAY using root build.sh
if [ ! -f "build.sh" ]; then
  echo "ERROR: Root build.sh not found."
  exit 1
fi
echo "Running root build.sh (relay)..."
./build.sh

# Remove old panel source to get latest changes
echo "🔄 Removing old panel source..."
rm -rf panel-source

# Clone fresh copy from local path
echo "📥 Cloning latest panel source..."
git clone https://github.com/HORNET-Storage/HORNETS-Relay-Panel.git ./panel-source

# Navigate to panel source directory
cd panel-source

# Install dependencies
echo "📦 Installing dependencies..."
yarn install

# Build the project
echo "🔨 Building panel..."
# Clear any existing build first
rm -rf build
# Build with production optimizations disabled to avoid syntax errors
GENERATE_SOURCEMAP=false NODE_ENV=production yarn build

# Copy built files to web directory
echo "📋 Copying files to web directory..."
cd ..
mkdir -p web
rm -rf web/*
cp -r panel-source/build/* web/

echo "✅ Panel built and deployed successfully!"
echo "🌐 The panel is now available at your relay's root URL"
echo ""
echo "To test the panel:"
echo "1. Start your relay server: go run services/server/port/main.go"
echo "2. Visit http://localhost:9002 (or your configured port)"
echo "3. The panel should load automatically"