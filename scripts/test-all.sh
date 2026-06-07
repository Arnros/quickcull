#!/usr/bin/env bash
set -euo pipefail

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ORIGINAL_DIR="$(pwd)"
trap 'cd "$ORIGINAL_DIR"' EXIT

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo -e "${RED}❌ Missing required command: $1${NC}"
    exit 1
  fi
}

has_npm_script() {
  local script_name="$1"
  node -e "const p=require('./package.json'); process.exit(p.scripts && p.scripts['$script_name'] ? 0 : 1)" >/dev/null 2>&1
}

require_cmd go
require_cmd npm
require_cmd node

# Keep tests deterministic in restricted/sandboxed environments.
if [[ -z "${HOME:-}" || ! -w "$HOME" ]]; then
  export HOME="${QUICKCULL_TEST_HOME:-/tmp/quickcull-home}"
fi
mkdir -p "$HOME"
export QUICKCULL_TEST_CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/quickcull-test"
mkdir -p "$QUICKCULL_TEST_CACHE_DIR"
export XDG_CACHE_HOME="${XDG_CACHE_HOME:-$HOME/.cache}"
mkdir -p "$XDG_CACHE_HOME"
export GOCACHE="${GOCACHE:-/tmp/go-build}"
mkdir -p "$GOCACHE"

# Some domain config tests intentionally create/expect config temp paths.
# Clean stale leftovers from previous interrupted runs to keep the suite deterministic.
rm -rf "$HOME/Library/Caches/quickcull/config.json.tmp" "$XDG_CACHE_HOME/quickcull/config.json.tmp" 2>/dev/null || true

cd "$ROOT_DIR"

# macOS-only linker warning suppression.
# On Linux, propagating the macOS flag into `go run` breaks tool builds such as
# staticcheck because GNU ld does not support `-no_warn_duplicate_libraries`.
if [[ "$(uname -s)" == "Darwin" ]]; then
  # 1. Align CGO and linker deployment target: bridge objects compiled with the
  #    current SDK embed its version (e.g. 26.0) which triggers
  #    "built for newer macOS than being linked" when the linker targets 10.13.
  #    Setting this to our minimum supported macOS makes both sides agree.
  export MACOSX_DEPLOYMENT_TARGET="${MACOSX_DEPLOYMENT_TARGET:-12.0}"
  # 2. Suppress "ignoring duplicate libraries: '-lpthread'": pthreads are part
  #    of libSystem on macOS and the flag being passed twice by CGO is harmless
  #    but noisy.  -Wl,-no_warn_duplicate_libraries requires Xcode 15+.
  export CGO_LDFLAGS="${CGO_LDFLAGS:+$CGO_LDFLAGS }-Wl,-no_warn_duplicate_libraries"
fi

echo -e "${BLUE}🚀 Starting full test suite for quickcull...${NC}
"

# 1. Linters
echo -e "${BLUE}--- Checking Go Print functions ---${NC}"
./scripts/lint-go-fmt.sh
echo -e "${GREEN}✅ Go Print linter OK${NC}
"

echo -e "${BLUE}--- Checking i18n (Go) ---${NC}"
./scripts/lint-go-i18n.sh
echo -e "${GREEN}✅ Go i18n OK${NC}
"

echo -e "${BLUE}--- Checking i18n (UI) ---${NC}"
./scripts/lint-ui-i18n.sh
echo -e "${GREEN}✅ UI i18n OK${NC}
"

if [[ "${QUICKCULL_RUN_GOSEC:-0}" == "1" ]]; then
  echo -e "${BLUE}--- Running Go Security Scan (gosec) ---${NC}"
  go run github.com/securego/gosec/v2/cmd/gosec@latest -quiet ./...
  echo -e "${GREEN}✅ Go Security OK${NC}
"
else
  echo -e "${BLUE}--- Skipping Go Security Scan (set QUICKCULL_RUN_GOSEC=1 to enable) ---${NC}"
fi

# 2. Backend Validation (Vet & Staticcheck)
echo -e "${BLUE}--- Running Go Vet ---${NC}"
go mod download
go vet ./...
echo -e "${GREEN}✅ Go Vet OK${NC}
"

echo -e "${BLUE}--- Running Go Staticcheck (Advanced Analysis) ---${NC}"
if command -v staticcheck >/dev/null 2>&1; then
  staticcheck ./...
else
  go run honnef.co/go/tools/cmd/staticcheck@latest ./...
fi
echo -e "${GREEN}✅ Go Staticcheck OK${NC}
"

# 3. Backend Tests
echo -e "${BLUE}--- Running Go Backend Tests ---${NC}"
go test -count=1 ./...
echo -e "${BLUE}--- Running Go Backend Shuffle Check (review) ---${NC}"
go test -shuffle=on -count=1 ./internal/review/...
if [[ "${QUICKCULL_RUN_RACE:-0}" == "1" ]]; then
  echo -e "${BLUE}--- Running Go Backend Race Detector ---${NC}"
  go test -race -count=1 ./...
fi
echo -e "${GREEN}✅ Backend Tests OK${NC}
"

# 4. Frontend Unit Tests
echo -e "${BLUE}--- Running UI Unit Tests (Vitest) ---${NC}"
cd ui
if has_npm_script "test:unit"; then
  npm run test:unit
elif has_npm_script "test"; then
  echo -e "${BLUE}No test:unit script found, using npm test as unit suite${NC}"
  npm run test
else
  echo -e "${RED}❌ No UI unit test script found (expected test:unit or test)${NC}"
  exit 1
fi
echo -e "${GREEN}✅ UI Unit Tests OK${NC}
"

# 5. Frontend Type Check
echo -e "${BLUE}--- Running UI Type Checks (Svelte Check) ---${NC}"
npm run check
echo -e "${GREEN}✅ UI Type Checks OK${NC}
"

# 6. Frontend Dead Code Check
echo -e "${BLUE}--- Running UI Dead Code Check (Knip) ---${NC}"
npm run knip
echo -e "${GREEN}✅ UI Dead Code Check OK${NC}
"

# 6. Frontend Production Build
echo -e "${BLUE}--- Running UI Production Build ---${NC}"
npm run build
echo -e "${GREEN}✅ UI Build OK${NC}
"

# 7. Frontend E2E Tests
if [[ "${QUICKCULL_SKIP_E2E:-0}" == "1" ]]; then
  echo -e "${BLUE}--- Skipping UI E2E Tests (QUICKCULL_SKIP_E2E=1) ---${NC}"
else
  if has_npm_script "test:e2e"; then
    echo -e "${BLUE}--- Running UI E2E Tests (test:e2e) ---${NC}"
    npm run test:e2e
    echo -e "${GREEN}✅ UI E2E Tests OK${NC}
"
  else
    echo -e "${BLUE}--- Skipping UI E2E Tests (no test:e2e script) ---${NC}"
  fi
fi

if [[ "${QUICKCULL_RUN_COVERAGE:-0}" == "1" ]]; then
  echo -e "${BLUE}--- Running Coverage Collection (Go + UI) ---${NC}"
  cd "$ROOT_DIR"
  mkdir -p coverage/go coverage/ui

  echo -e "${BLUE}Collecting Go coverage profile...${NC}"
  if go test -count=1 -covermode=atomic -coverprofile=coverage/go/coverage.out ./...; then
    go tool cover -func=coverage/go/coverage.out | tee coverage/go/coverage.txt
  else
    echo -e "${RED}⚠️  Go coverage collection failed; continuing (non-blocking phase).${NC}"
  fi

  echo -e "${BLUE}Collecting UI coverage profile (optional provider)...${NC}"
  cd ui
  if node -e "require.resolve('@vitest/coverage-v8')" >/dev/null 2>&1; then
    if npm run test:unit -- --coverage --coverage.provider=v8 --coverage.reportsDirectory=../coverage/ui --coverage.reporter=text --coverage.reporter=lcov; then
      echo -e "${GREEN}✅ UI coverage profile generated${NC}"
    else
      echo -e "${RED}⚠️  UI coverage collection failed; continuing (non-blocking phase).${NC}"
    fi
  else
    echo -e "${BLUE}Skipping UI coverage: install @vitest/coverage-v8 to enable${NC}"
  fi
  cd "$ROOT_DIR"
  echo -e "${GREEN}✅ Coverage Collection OK${NC}
"
fi

if [[ "${QUICKCULL_RUN_AUDIT:-0}" == "1" ]]; then
  echo -e "${BLUE}--- Running Optional UI Security Audit ---${NC}"
  npm audit --omit=dev --audit-level=high
  echo -e "${GREEN}✅ UI Security Audit OK${NC}
"
fi

if [[ "${QUICKCULL_ENFORCE_CLEAN_TREE:-0}" == "1" ]]; then
  require_cmd git
  echo -e "${BLUE}--- Verifying Clean Working Tree ---${NC}"
  cd "$ROOT_DIR"
  if ! git diff --quiet -- .; then
    echo -e "${RED}❌ Working tree changed during tests${NC}"
    git status --short
    exit 1
  fi
  echo -e "${GREEN}✅ Working Tree Clean OK${NC}
"
fi

echo -e "${GREEN}=====================================${NC}"
echo -e "${GREEN}✨ ALL TESTS PASSED SUCCESSFULLY ✨${NC}"
echo -e "${GREEN}=====================================${NC}"
