#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "❌ Missing required command: $1"
    exit 1
  fi
}

require_cmd go
require_cmd npm

# Keep tests deterministic in restricted/sandboxed environments.
if [[ -z "${HOME:-}" || ! -w "$HOME" ]]; then
  export HOME="/tmp/quickcull-home"
fi
mkdir -p "$HOME"
export XDG_CACHE_HOME="${XDG_CACHE_HOME:-$HOME/.cache}"
mkdir -p "$XDG_CACHE_HOME"
export GOCACHE="${GOCACHE:-/tmp/go-build}"
mkdir -p "$GOCACHE"

echo "======================================"
echo "    🧪 quickcull - Full Test Suite    "
echo "======================================"
echo ""

echo "[1/3] Running default gate (test-all.sh)..."
./scripts/test-all.sh

if [[ "${QUICKCULL_RUN_COVERAGE:-0}" == "1" ]]; then
  echo ""
  echo "[coverage] Coverage artifacts requested via QUICKCULL_RUN_COVERAGE=1"
  echo "[coverage] Artifacts expected under ./coverage/go and ./coverage/ui"
fi

echo ""
echo "[2/3] Running Go race detector..."
go test -race ./...

echo ""
echo "[3/3] Running Go benchmarks (review package)..."
go test -bench=. ./internal/review/... -run=^$

echo ""
echo "✅ All comprehensive tests passed successfully!"
