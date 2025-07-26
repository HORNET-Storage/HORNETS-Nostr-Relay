#!/bin/bash

# Development Mode Build Script for HORNETS-Relay-Panel
# This script builds the panel from local source without pulling from remote

set -e

echo "🚀 Building HORNETS-Relay-Panel (Development Mode)..."
echo "📁 Using local panel source at: ./panel-source"

# Check if local panel-source exists
if [ ! -d "panel-source" ]; then
    echo "❌ Error: Local panel-source directory not found!"
    echo "   Expected path: $(pwd)/panel-source"
    echo ""
    echo "   To set up local development:"
    echo "   1. Clone the panel repo: git clone https://github.com/HORNET-Storage/HORNETS-Relay-Panel.git ./panel-source"
    echo "   2. Make your changes in ./panel-source"
    echo "   3. Run this script to build and deploy"
    exit 1
fi

# Navigate to panel source directory
cd panel-source

# Check if it's a valid panel project
if [ ! -f "package.json" ]; then
    echo "❌ Error: panel-source doesn't appear to be a valid panel project (missing package.json)"
    exit 1
fi

# Install/update dependencies
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
# Create web directory if it doesn't exist
mkdir -p web
# Clear any existing files
rm -rf web/*
# Copy built files
cp -r panel-source/build/* web/

echo "✅ Panel built and deployed successfully from local source!"
echo "🌐 The panel is now available at your relay's root URL"
echo ""
echo "📝 Development workflow:"
echo "1. Make changes in ./panel-source"
echo "2. Run ./build-panel-devmode.sh to rebuild"
echo "3. Refresh your browser to see changes"
echo ""
echo "To test the panel:"
echo "1. Start your relay server: go run services/server/port/main.go"
echo "2. Visit http://localhost:9002 (or your configured port)"
echo "3. The panel should load automatically"
