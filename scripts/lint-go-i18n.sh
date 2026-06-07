#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

TARGETS=(
  "internal/domain/errors.go"
  "internal/domain/config.go"
  "internal/review/app.go"
  "internal/review/server.go"
  "internal/review/state.go"
  "internal/review/rotate.go"
)

PATTERN='errors\.New\("([^"]+)"\)|fmt\.Errorf\("([^"]+)"|http\.Error\([^,]+,\s*"([^"]+)"'

MATCHES="$(grep -En "$PATTERN" "${TARGETS[@]}" || true)"
if [[ -n "$MATCHES" ]]; then
  echo "Found hardcoded user-facing text in Go:"
  echo "$MATCHES"
  echo
  echo "Use domain.CodedError values (QCERR:...) or http.StatusText(...) instead of raw text."
  exit 1
fi

echo "OK: no hardcoded user-facing Go text found."
