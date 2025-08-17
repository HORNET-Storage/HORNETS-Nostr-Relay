#!/bin/bash

# Build and Deploy Panel Script
# This script builds the HORNETS-Relay-Panel and copies it to the web directory

set -e

echo "🚀 Building HORNETS-Relay-Panel..."

# Check if panel-source directory exists and is a git repository
if [ -d "panel-source/.git" ]; then
  echo "🔄 Checking for panel updates..."
  cd panel-source
  git fetch
  LOCAL=$(git rev-parse @)
  REMOTE=$(git rev-parse @{u})

  if [ "$LOCAL" = "$REMOTE" ]; then
    echo "✅ Panel is up to date."
    cd ..
  else
    echo "📥 Pulling latest panel source..."
    git pull
    cd ..
  fi
else
  # Remove old panel source to get latest changes
  echo "🔄 Removing old panel source..."
  rm -rf panel-source

  # Clone fresh copy from local path
  echo "📥 Cloning latest panel source..."
  git clone https://github.com/HORNET-Storage/HORNETS-Relay-Panel.git ./panel-source
fi

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
# Create web directory if it doesn't exist
mkdir -p web
# Clear any existing files
rm -rf web/*
# Copy built files
cp -r panel-source/build/* web/

echo "✅ Panel built and deployed successfully!"
echo "🌐 The panel is now available at your relay's root URL"
echo ""
echo "To test the panel:"
echo "1. Start your relay server: go run services/server/port/main.go"
echo "2. Visit http://localhost:9002 (or your configured port)"
echo "3. The panel should load automatically"
