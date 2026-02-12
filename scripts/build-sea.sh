#!/usr/bin/env bash
# build-sea.sh — Build proton-drive-cli as a Node.js Single Executable Application.
#
# Prerequisites:
#   - Node.js >= 22
#   - proton-drive-cli already compiled (yarn build / npm run build)
#   - npx available on PATH
#
# Output: proton-drive-cli (or proton-drive-cli.exe on Windows) in DRIVE_CLI_DIR.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DRIVE_CLI_DIR="${ROOT_DIR}/submodules/proton-drive-cli"
OUTPUT_DIR="${ROOT_DIR}/bin"

cd "$DRIVE_CLI_DIR"

echo "==> Step 1: esbuild bundle"
if [ ! -f dist/index.js ]; then
  echo "Error: dist/index.js not found. Run 'npm run build' first." >&2
  exit 1
fi
npx esbuild dist/index.js --bundle --platform=node --target=node22 --outfile=sea-bundle.cjs

echo "==> Step 2: Generate SEA blob"
node --experimental-sea-config sea-config.json

echo "==> Step 3: Copy node binary and inject blob"
OUTPUT_NAME="proton-drive-cli"
if [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" || "$OSTYPE" == "win32" ]]; then
  OUTPUT_NAME="proton-drive-cli.exe"
fi

NODE_BIN="$(command -v node)"
cp "$NODE_BIN" "$OUTPUT_NAME"

# macOS: strip existing code signature before injection
if [[ "$OSTYPE" == "darwin"* ]]; then
  codesign --remove-signature "$OUTPUT_NAME" 2>/dev/null || true
fi

npx postject "$OUTPUT_NAME" NODE_SEA_BLOB sea-prep.blob \
  --sentinel-fuse NODE_SEA_FUSE_fce680ab2cc467b6e072b8b5df1996b2

# macOS: re-sign with ad-hoc signature
if [[ "$OSTYPE" == "darwin"* ]]; then
  codesign --sign - "$OUTPUT_NAME"
fi

# Move to output directory
mkdir -p "$OUTPUT_DIR"
mv "$OUTPUT_NAME" "$OUTPUT_DIR/$OUTPUT_NAME"

# Clean up intermediate files
rm -f sea-bundle.cjs sea-prep.blob

echo "==> SEA binary ready: $OUTPUT_DIR/$OUTPUT_NAME"
