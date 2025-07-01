#!/bin/bash

# Change directory to the generator directory
cd services/server/demo/demodata

# Build the generator with output in the root directory
echo "Building demo data generator..."
go build -o ../../../../hornet-demo-generator ./cmd/generator

echo "Build complete!"
echo
echo "To run the demo data generator, use:"
echo "./hornet-demo-generator [--auto]"
echo
echo "The generator will automatically use demo_statistics.db in the project root"

# Pause equivalent (wait for user input)
read -p "Press enter to continue"
