#!/bin/bash
set -x

# Install xdotool if not already installed
if ! command -v xdotool &> /dev/null; then
    echo "xdotool could not be found, installing it..."
    sudo apt-get install -y xdotool
fi

# Find the window ID of the Sparrow wallet
WINDOW_ID=$(xdotool search --name "Sparrow" | head -n 1)

if [ -z "$WINDOW_ID" ]; then
    echo "Sparrow wallet window not found."
    exit 1
fi

# Send Ctrl+R key press directly to the window to refresh it
xdotool key --window $WINDOW_ID ctrl+r

# Optional: Minimize the window if needed
xdotool windowminimize $WINDOW_ID

echo "Sparrow wallet refreshed."


