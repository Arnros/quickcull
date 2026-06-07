# quickcull - Testing Strategy

This document defines the testing strategy for the `quickcull` project. The goal is to maximize reliability and iteration speed through a multi-layered verification approach.

## Target Test Pyramid

- **Unit tests (~70%)**: Fast, deterministic tests for business logic (Go) and UI state management (Vitest).
- **Integration tests (~20%)**: Backend server flows and persistence layer validation.
- **E2E tests (~10%)**: Critical UX regression tests using Playwright with a mocked backend for speed and stability.

## Strategic Matrix

| Objective | Test Type | Implementation | Frequency |
| :--- | :--- | :--- | :--- |
| **Business Logic & I/O** | Unit Tests (Go) | `go test` in `internal/review/` | Commit / `test-all.sh` |
| **UI State & Layouts** | Unit Tests (Vitest) | `vitest` in `ui/src/lib/` | Commit / `test-all.sh` |
| **E2E Golden Path** | E2E (Playwright) | `vitest` + `testUtils.ts` | Commit / `test-all.sh` |
| **Concurrency (Go)** | Race Detector | `go test -race` | PRs / `test-full.sh` |
| **Performance (Go)** | Benchmarks | `go test -bench` | Nightly / Releases |

## Key Testing Principles

1. **Mandatory Pre-Commit Gate**: Every change MUST pass `./scripts/test-all.sh`.
2. **Mocked Backend for UI E2E**: Playwright tests run against a robust mock of the Go bridge to avoid flaky filesystem I/O in CI.
3. **Double Verification**: Backend tests must aggressively validate that filenames match indices before any destructive operation.
4. **Race Detection**: Critical for our modular concurrency model. Run with `QUICKCULL_RUN_RACE=1`.

## Streaming & Discovery Robustness

Specialized tests ensure stability during the non-blocking ingest phase:
- `TestLoadStateStreaming_PropagatesScanError`: Verifies that I/O failures during discovery are surfaced correctly.
- `TestStressBoundedDiscovery_NoGoroutineLeakTrend`: Ensures that repeated folder scanning does not leak background workers.

## Script Controls

`./scripts/test-all.sh` supports:
- `QUICKCULL_RUN_RACE=1`: Enable the Go race detector.
- `QUICKCULL_SKIP_E2E=1`: Skip browser-based tests (useful in minimal environments).
- `QUICKCULL_RUN_COVERAGE=1`: Generate coverage artifacts under `coverage/`.

For release sign-off, use [`docs/release-checklist.md`](./release-checklist.md).
