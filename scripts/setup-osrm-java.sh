#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DATA_DIR="$ROOT_DIR/data/osrm"
IMAGE="ghcr.io/project-osrm/osrm-backend:v6.0.0"
PBF="$DATA_DIR/java-latest.osm.pbf"
URL="https://download.geofabrik.de/asia/indonesia/java-latest.osm.pbf"

mkdir -p "$DATA_DIR"

if [ ! -f "$PBF" ]; then
  echo "Downloading Java OpenStreetMap extract..."
  if command -v curl >/dev/null 2>&1; then
    curl -L --fail --retry 3 "$URL" -o "$PBF"
  elif command -v wget >/dev/null 2>&1; then
    wget -O "$PBF" "$URL"
  else
    echo "curl or wget is required" >&2
    exit 1
  fi
else
  echo "Using existing $PBF"
fi

echo "OSRM extract..."
docker run --rm -t -v "$DATA_DIR:/data" "$IMAGE" \
  osrm-extract -p /opt/car.lua /data/java-latest.osm.pbf

echo "OSRM partition..."
docker run --rm -t -v "$DATA_DIR:/data" "$IMAGE" \
  osrm-partition /data/java-latest.osrm

echo "OSRM customize..."
docker run --rm -t -v "$DATA_DIR:/data" "$IMAGE" \
  osrm-customize /data/java-latest.osrm

echo
echo "OSRM data ready. Start routing with:"
echo "  docker compose --profile routing up -d --build"
echo
echo "Health check example:"
echo '  curl "http://localhost:5000/route/v1/driving/106.8272,-6.1754;106.7990,-6.2615?overview=false"'
