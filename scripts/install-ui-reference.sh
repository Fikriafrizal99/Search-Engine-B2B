#!/usr/bin/env bash
set -euo pipefail

ZIP_PATH="${1:-$HOME/Downloads/bukupay-ui-reference-high.zip}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST="$REPO_ROOT/docs/ui-reference"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

if [[ ! -f "$ZIP_PATH" ]]; then
  echo "UI reference archive not found: $ZIP_PATH" >&2
  echo "Usage: bash scripts/install-ui-reference.sh /path/to/bukupay-ui-reference-high.zip" >&2
  exit 1
fi

mkdir -p "$DEST"
unzip -q "$ZIP_PATH" -d "$TMP_DIR"
SRC="$TMP_DIR/bukupay-ui-reference-high"

for name in dashboard area-planner merchant-pipeline visit-session database-scraper; do
  cp "$SRC/$name.jpg" "$DEST/$name.jpg"
done

cp "$SRC/README.md" "$DEST/LOCAL-IMAGE-PACK.md"

echo "Installed Bukupay UI references into: $DEST"
ls -lh "$DEST"/*.jpg
