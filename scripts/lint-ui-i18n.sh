#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Check if node is available
if ! command -v node &> /dev/null; then
    echo "Node.js is not found. Skipping UI i18n lint."
    exit 0
fi

node scripts/lint-ui-i18n.mjs
