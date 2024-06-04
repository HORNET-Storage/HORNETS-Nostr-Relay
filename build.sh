#!/bin/bash

# Change directory to web/panel
cd web/panel

# Install npm dependencies
yarn install

# Change back to the parent directory
cd ../..

./build_server.sh

# Optional: Print a message before exiting
echo "Build process completed."

# Optional: Pause (press Enter to continue)
read -p "Press Enter to exit."
