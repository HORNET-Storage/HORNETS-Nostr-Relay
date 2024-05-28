#!/bin/bash
# This script sets the necessary environment variable and starts the React app

echo "Setting OpenSSL legacy provider and starting the app..."
export NODE_OPTIONS=--openssl-legacy-provider
yarn start

