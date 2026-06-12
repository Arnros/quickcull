# QuickCull — Claude Instructions

## Architecture

Native desktop app: **Go 1.26+ backend** (Wails v2.12+) + **Svelte 5 frontend** (Node 26).

- **Event sourcing**: immutable `AppState`, pure `Reduce()` function, persistent undo stack (BoltDB, capped 100 entries).
- **Zero-ingestion**: no catalog, on-the-fly scan + in-memory index with O(1) lookups.
- **Authoritative Sync**: unified `SyncState` (full or partial/structural) + `SyncDelta` (metadata). Ultra-optimized chunking for large sets.
- **Background workers**: capped at 50% CPU cores, viewport-priority queue for thumbnails/EXIF.
- **Memory Safety**: automatic `runtime.GC()` watchdog (2GB threshold) + on-demand chunk building.
- **Pure Photo Focus**: image formats only; videos are strictly ignored.

## Key files

| File | Role |
|---|---|
| `internal/review/state_immutable.go` | Pure reducer + undo logic |
| `internal/review/sync_delivery.go` | Optimized chunked delivery + `RefreshVisibleOrder` |
| `internal/review/state_deltas.go` | Incremental metadata updates + persistence |
| `internal/review/event_engine.go` | Event bus subscriber loop |
| `internal/review/analysis_policy.go` | Concurrency & prioritization logic |
| `internal/review/analysis_workers.go` | Task processing loop |
| `internal/review/analysis_telemetry.go` | GC watchdog + progress reporting |
| `internal/review/app.go` | Wails-bound API methods |
| `internal/utils/logging.go` | Unified categorized logging system |
| `ui/src/lib/appState.svelte.ts` | Frontend state + actions |

## Standards

- **Logging**: use `internal/utils/logging.go` helpers (`LogNav`, `LogAnalysis`, `LogCore`).
- **Atomic saves**: metadata writes = write temp → rename.
- **Lock Discipline**: strictly follow `stateMu` → `appStateMu` → `perfMu`.
- **Concurrency**: use `atomic.Bool` for shared policy flags.
- **Payload efficiency**: never transmit `History` or `InitialState` to the UI; use `UndoLen`.
- **i18n**: all UI text via `ui/src/lib/i18n.svelte.ts`.
- **Shortcuts**: use `ShortcutService`; never hardcode keys in components.

## Validation mandatory

Before any commit: `./scripts/test-all.sh`
(Go tests + Race detector + Vitest + svelte-check + build)

# context-mode — MANDATORY routing rules

You have context-mode MCP tools available. These rules are NOT optional — they protect your context window from flooding. A single unrouted command can dump 56 KB into context and waste the entire session.

## BLOCKED commands — do NOT attempt these

### curl / wget — BLOCKED
Any Bash command containing `curl` or `wget` is intercepted and replaced with an error message. Do NOT retry.
Instead use:
- `ctx_fetch_and_index(url, source)` to fetch and index web pages
- `ctx_execute(language: "javascript", code: "const r = await fetch(...)")` to run HTTP calls in sandbox

### Inline HTTP — BLOCKED
Any Bash command containing `fetch('http`, `requests.get(`, `requests.post(`, `http.get(`, or `http.request(` is intercepted and replaced with an error message. Do NOT retry with Bash.
Instead use:
- `ctx_execute(language, code)` to run HTTP calls in sandbox — only stdout enters context

### WebFetch — BLOCKED
WebFetch calls are denied entirely. The URL is extracted and you are told to use `ctx_fetch_and_index` instead.
Instead use:
- `ctx_fetch_and_index(url, source)` then `ctx_search(queries)` to query the indexed content

## REDIRECTED tools — use sandbox equivalents

### Bash (>20 lines output)
Bash is ONLY for: `git`, `mkdir`, `rm`, `mv`, `cd`, `ls`, `npm install`, `pip install`, and other short-output commands.
For everything else, use:
- `ctx_batch_execute(commands, queries)` — run multiple commands + search in ONE call
- `ctx_execute(language: "shell", code: "...")` — run in sandbox, only stdout enters context

### Read (for analysis)
If you are reading a file to **Edit** it → Read is correct (Edit needs content in context).
If you are reading to **analyze, explore, or summarize** → use `ctx_execute_file(path, language, code)` instead. Only your printed summary enters context. The raw file content stays in the sandbox.

### Grep (large results)
Grep results can flood context. Use `ctx_execute(language: "shell", code: "grep ...")` to run searches in sandbox. Only your printed summary enters context.

## Tool selection hierarchy

1. **GATHER**: `ctx_batch_execute(commands, queries)` — Primary tool. Runs all commands, auto-indexes output, returns search results. ONE call replaces 30+ individual calls.
2. **FOLLOW-UP**: `ctx_search(queries: ["q1", "q2", ...])` — Query indexed content. Pass ALL questions as array in ONE call.
3. **PROCESSING**: `ctx_execute(language, code)` | `ctx_execute_file(path, language, code)` — Sandbox execution. Only stdout enters context.
4. **WEB**: `ctx_fetch_and_index(url, source)` then `ctx_search(queries)` — Fetch, chunk, index, query. Raw HTML never enters context.
5. **INDEX**: `ctx_index(content, source)` — Store content in FTS5 knowledge base for later search.

## Subagent routing

When spawning subagents (Agent/Task tool), the routing block is automatically injected into their prompt. Bash-type subagents are upgraded to general-purpose so they have access to MCP tools. You do NOT need to manually instruct subagents about context-mode.

## Output constraints

- Keep responses under 500 words.
- Write artifacts (code, configs, PRDs) to FILES — never return them as inline text. Return only: file path + 1-line description.
- When indexing content, use descriptive source labels so others can `ctx_search(source: "label")` later.

## ctx commands

| Command | Action |
|---------|--------|
| `ctx stats` | Call the `ctx_stats` MCP tool and display the full output verbatim |
| `ctx doctor` | Call the `ctx_doctor` MCP tool, run the returned shell command, display as checklist |
| `ctx upgrade` | Call the `ctx_upgrade` MCP tool, run the returned shell command, display as checklist |
