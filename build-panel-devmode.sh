#!/usr/bin/env bash
set -e

# Always operate from the script's folder (repo root)
cd "$(dirname "$0")"

# --- Config ---
REPO_URL="https://github.com/HORNET-Storage/HORNETS-Relay-Panel.git"
PANEL_DIR="panel-source"
BACKEND_EXE="./hornet-storage"
export NODE_OPTIONS="--openssl-legacy-provider --max-old-space-size=4096"
# -------------

echo
echo "==============================="
echo "HORNETS-Relay-Panel Dev Runner"
echo "==============================="
echo

# 1) Clone panel if missing (no pull/update if it already exists)
if [ ! -d "$PANEL_DIR" ]; then
  if ! command -v git >/dev/null 2>&1; then
    echo "ERROR: git not found in PATH."
    exit 1
  fi
  echo "Cloning panel to $PANEL_DIR ..."
  git clone "$REPO_URL" "$PANEL_DIR" || { echo "ERROR: clone failed."; exit 1; }
fi

# Sanity check the panel project exists
if [ ! -f "$PANEL_DIR/package.json" ]; then
  echo "ERROR: $PANEL_DIR/package.json not found."
  exit 1
fi

# 2) Build the RELAY using root build.sh
if [ ! -f "build.sh" ]; then
  echo "ERROR: Root build.sh not found."
  exit 1
fi
echo "Running root build.sh (relay)..."
./build.sh

# 3) Run the backend exe from the root
if [ ! -f "$BACKEND_EXE" ]; then
  echo "ERROR: $BACKEND_EXE not found in repo root after build."
  echo "Tip: confirm build.sh outputs the binary to the root, or adjust BACKEND_EXE path."
  exit 1
fi
echo "Starting backend: $BACKEND_EXE"
$BACKEND_EXE &

# 4) Start the panel in dev mode (current process)
echo
echo "Starting panel dev server (dev mode)..."
cd "$PANEL_DIR"

# Install deps (Yarn preferred, fallback to npm)
if command -v yarn >/dev/null 2>&1; then
  yarn install || echo "WARNING: yarn install reported issues."
else
  npm install || echo "WARNING: npm install reported issues."
fi

# Create themes directory if it doesn't exist and build themes
echo "Building themes for development..."
mkdir -p "public/themes"
node_modules/.bin/lessc --js --clean-css="--s1 --advanced" src/styles/themes/main.less public/themes/main.css || {
  echo "WARNING: Theme building failed. Styles may not load properly."
}

# Prefer CRACO if present; else yarn start; else npm start
if [ -f "node_modules/.bin/craco" ]; then
  exec npx craco start
elif command -v yarn >/dev/null 2>&1; then
  NODE_ENV=development exec yarn start
else
  NODE_ENV=development exec npm run start
fi
