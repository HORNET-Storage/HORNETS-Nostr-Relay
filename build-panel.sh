#!/bin/bash

# Build and Deploy Panel Script
# This script builds the HORNETS-Relay-Panel and copies it to the web directory

set -e

# --- Config ---
CONFIG_FILE="config.yaml"
# -------------

echo "🚀 Building HORNETS-Relay-Panel..."

# Read base port from config.yaml and calculate web port (+2)
BASE_PORT="9000"
if [ -f "$CONFIG_FILE" ]; then
  echo "📖 Reading port from $CONFIG_FILE..."
  PARSED_PORT=$(grep -E "^\s*port:" "$CONFIG_FILE" | head -1 | sed 's/.*port:\s*//' | tr -d '[:space:]')
  if [ -n "$PARSED_PORT" ]; then
    BASE_PORT="$PARSED_PORT"
  fi
fi
WEB_PORT=$((BASE_PORT + 2))
echo "🔌 Base port: $BASE_PORT - Web panel port: $WEB_PORT"

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

# Update .env files with the correct web port
echo "📝 Updating .env files with port $WEB_PORT..."
for envfile in .env.development .env.production; do
  if [ -f "$envfile" ]; then
    if grep -q "REACT_APP_BASE_URL=" "$envfile"; then
      sed -i "s|REACT_APP_BASE_URL=http://localhost:[0-9]*|REACT_APP_BASE_URL=http://localhost:$WEB_PORT|g" "$envfile"
    else
      echo "REACT_APP_BASE_URL=http://localhost:$WEB_PORT" >> "$envfile"
    fi
  fi
done

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
echo "1. Start your relay server: ./hornet-storage"
echo "2. Visit http://localhost:$WEB_PORT"
echo "3. The panel should load automatically"
