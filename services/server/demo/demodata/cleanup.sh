#!/bin/bash

echo "HORNETS-Nostr-Relay Demo Database Cleanup"
echo "========================================"
echo "This script will clean up unused database files to avoid confusion."
echo "The demo server and generator tool will now use demo_statistics.db in the root directory."

cd $(dirname "$0")/..
ROOT_DIR=$(pwd)
echo "Working in directory: $ROOT_DIR"
echo

# List all SQLite database files
echo "Found SQLite database files:"
find . -name "*.db" -o -name "*.db-shm" -o -name "*.db-wal" | sort
echo

# Confirm before proceeding
read -p "Do you want to keep only demo_statistics.db and remove other database files? (y/n): " confirm
if [[ $confirm != [yY] ]]; then
    echo "Cleanup cancelled."
    exit 0
fi

# Keep demo_statistics.db in the root directory, remove others
echo "Cleaning up..."

# Keep these files
KEEP_FILES=(
    "./demo_statistics.db"
    "./demo_statistics.db-shm"
    "./demo_statistics.db-wal"
)

# Remove everything else
COUNT=0
for file in $(find . -name "*.db" -o -name "*.db-shm" -o -name "*.db-wal"); do
    keep=false
    for keep_file in "${KEEP_FILES[@]}"; do
        if [[ "$file" == "$keep_file" ]]; then
            keep=true
            break
        fi
    done
    
    if [[ $keep == false ]]; then
        echo "Removing: $file"
        rm "$file"
        COUNT=$((COUNT+1))
    else
        echo "Keeping: $file"
    fi
done

echo
echo "Cleanup complete! Removed $COUNT database files."
echo "The demo server and generator tool will now use a single shared database:"
echo "$ROOT_DIR/demo_statistics.db"

# Ask if user wants to rebuild the generator
echo
read -p "Do you want to rebuild the generator tool now? (y/n): " rebuild
if [[ $rebuild == [yY] ]]; then
    echo "Rebuilding generator tool..."
    cd "$(dirname "$0")"
    ./build.sh
    echo
    echo "Generator rebuilt! You can now run it with:"
    echo "./demo-generator"
    echo
    echo "It will automatically use the demo_statistics.db in the project root."
fi
