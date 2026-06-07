# Contributing to quickcull

Thank you for your interest in improving `quickcull`! As a performance-focused tool, we value clean code, efficient concurrency, and cross-platform compatibility.

## Local Setup

1. **Prerequisites**:
    - Go 1.26+
    - Exiftool (optional, for RAW/HEIC metadata)

2. **Clone & Build**:
    ```bash
    git clone https://github.com/YOUR_USERNAME/quickcull
    cd quickcull
    wails dev
    ```

3. **Running tests**:
    - **Backend (Go)**: `go test -tags webkit2gtk_4_1 ./...`
    - **Frontend (E2E)**: `npm --prefix ui run test` (requires Playwright)

## Coding Standards

- **Formatting**: Always run `go fmt ./...` before committing.
- **Linting**: We recommend using `staticcheck` and `govulncheck`.
- **i18n errors**: Run `./scripts/lint-go-i18n.sh` to ensure user-facing Go errors are not hardcoded.
- **Logging**: Use `log/slog` exclusively. Do not use `fmt.Print` for logging.
- **Safety**: Ensure paths are validated using `filepath.Abs` and checked against the root directory to prevent path traversal vulnerabilities.

## Development Workflow

1. **Create a branch**: `git checkout -b feature/my-new-feature`
2. **Implement & Test**: Write your code and ensure all tests pass.
3. **Audit**: Run the full quality suite via the pre-commit gate script:
    ```bash
    ./scripts/test-all.sh
    ```
    In environments without a browser (e.g. WSL2), skip the Playwright stage:
    ```bash
    QUICKCULL_SKIP_E2E=1 ./scripts/test-all.sh
    ```
4. **Submit**: Open a Pull Request with a clear description of **what** changed and **why**.

## Testing Requirements

- Any new feature should include a corresponding `_test.go` file.
- Bug fixes must include a reproduction test case.
- Ensure your changes work on both Unix-like systems and Windows (pay attention to path separators).
