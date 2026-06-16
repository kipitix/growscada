# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

GrowSCADA is a web-based SCADA system built in Go, following Domain-Driven Design (DDD). The backend and PWA frontend are compiled from the same Go codebase into a single binary — the server handles the API and serves the WASM frontend.

## Makefile Reference

All primary workflows go through `make`. **Always prefer `make <target>` over running raw commands directly.**

| Target | What it does |
|--------|-------------|
| `make install_tools` | Installs `goose` migration tool via `go install` |
| `make build` | Compiles WASM frontend (`app.wasm`) and server binary; outputs to `bin/growscada_combined_server/` |
| `make run` | `build` + starts server + opens Chromium at `localhost:8080` |
| `make db_up` | Starts the dev PostgreSQL container via Docker Compose; applies migrations and seeds automatically |
| `make db_down` | Stops the container and **deletes** the data volume |
| `make test` | Runs all tests with coverage (`go test --cover ./...`) |
| `make full_restart` | Clean-slate restart: `db_down` → `db_up` → `run` (use when the DB state is stale or corrupted) |

`make build` runs two compilations: `GOARCH=wasm GOOS=js` for `app.wasm`, then host-arch for the server binary.

## Commands

```bash
# Run a single test
go test ./internal/domain/tag/... -run TestTagName

# Create a new migration
goose create <migration_name> sql

# Apply migrations manually
export GOOSE_DRIVER=postgres
export GOOSE_DBSTRING="user=growscada password=growscada host=localhost dbname=growscada"
goose up
```

## Architecture

### DDD Layer Structure

```
internal/
  domain/         # Core business logic — no external dependencies
  application/    # Use-case services, orchestrate domain objects
  infrastructure/ # Concrete implementations (Postgres, MQTT)
  interface/      # Delivery mechanisms (REST API, PWA UI)
cmd/
  combined_server/ # Entry point — wires everything together
```

### Domain Layer (`internal/domain/`)

Each subdomain owns its aggregate, value objects, and repository **interface**:

| Package    | Aggregate / Concept |
|------------|---------------------|
| `tag/`     | `Tag` — atomic SCADA data point with type, value, quality (`Good/Bad/Uncertain`) |
| `widget/`  | `Widget` — instance of a WidgetType placed on a Scene, with position/size/rotation/transform matrix |
| `widget/`  | `WidgetType` — reusable template: HTML template + JavaScript script |
| `scene/`   | `Scene` — named canvas (mnemonic screen) that hosts Widgets |
| `event/`   | `EventBus` and domain events (created/updated/deleted per aggregate) |
| `id/`      | Generic `ID[T]` UUID value object; type parameter prevents mixing IDs of different aggregates |
| `version/` | Optimistic-concurrency version value object; Generic `Version[T]` |

### Application Layer (`internal/application/`)

Services (`TagService`, `WidgetService`, `WidgetTypeService`, `SceneService`) each accept repository and event bus interfaces injected from `main.go`. They:
1. Construct domain objects from `appdto` input DTOs.
2. Delegate persistence to the repository.
3. Publish domain events via `EventBus` after mutations.

`appdto/` structs are the boundary between the domain and the outside world — not the REST DTOs.

### Infrastructure Layer (`internal/infrastructure/`)

- **PostgreSQL repositories** implement domain repository interfaces using `database/sql` + `lib/pq`.
- **Migrations** live in `internal/infrastructure/postgres/migrations/` and are managed with [Goose](https://github.com/pressly/goose). The debug `docker-compose.yaml` automatically applies migrations and seed data on `make db_up`.
- **MQTT event bus** (`event_bus_mqtt.go`) is a stub — the in-memory `EventBus` is used in production for now.

### Interface Layer (`internal/interface/`)

**REST API** (`internal/interface/restapi/`, port `:9090`):
- Uses stdlib `http.ServeMux` with method-prefixed patterns (Go 1.22+).
- CORS middleware applied globally.
- Errors follow RFC 7807 Problem Details (`problems.go`).
- Each resource has its own handler file (`tags.go`, `widgets.go`, etc.) and a `restdto/` package for JSON shapes.

**PWA UI** (`internal/interface/ui/`, port `:8080`):
- Built with [go-app v10](https://go-app.dev/) — compiled to WebAssembly, served by the same binary.
- Four top-level modes rendered by `root/root.go`:
  - **Library** — create and edit WidgetTypes (HTML template + JS script, live sandboxed preview via `<iframe srcdoc>`).
  - **Project** — configure Scenes and place Widget instances on them.
  - **Operation** — runtime operator view.
  - **History** — event log viewer.
- UI components communicate with the REST API directly using `http.Get/Post/…` inside `ctx.Async(…)` callbacks.

### Single Binary / Dual Compilation

`cmd/combined_server/main.go` calls `app.RunWhenOnBrowser()` near the top — when compiled to WASM this call takes over and runs the PWA; when compiled for the server it is a no-op, and the rest of `main` starts the HTTP servers. Both compilation targets share the same source file.

### Repository Integration Tests

Tests in `internal/infrastructure/postgres/repositories/` use **testcontainers-go** to spin up a real Postgres container, apply migrations via Goose, and run assertions. No manual database setup is needed for `go test`.

## Key Ubiquitous Language

| Term | Meaning |
|------|---------|
| **Tag** | Single process variable (analog or discrete) with quality metadata |
| **WidgetType** | Reusable visual component definition: HTML + JS |
| **Widget** | Placed instance of a WidgetType on a Scene, bound to Tag IDs |
| **Scene** | A mnemonic/synoptic display canvas |
| **Quality** | Data reliability: `Good`, `Bad`, `Uncertain` |

## API Collections

Manual/exploratory API tests are maintained as [Bruno](https://www.usebruno.com/) collections in `tests/api/bruno_collections/`. The `localhost` environment points to `http://localhost:9090`.

## MCP Servers

Four MCP servers are available and should be used during development:

### codegraph

**Use codegraph BEFORE grep/find or reading files** when you need to understand or locate code. The index is a pre-built SQLite knowledge graph of every symbol, call edge, and file in the workspace — sub-millisecond reads, updated within ~1s of file changes.

- `codegraph_explore` — PRIMARY tool. Pass a natural-language question or a bag of symbol/file names; returns the verbatim source of all relevant symbols grouped by file, plus call paths between them. Equivalent to running Read on multiple files at once — treat returned source as already read. Most questions need only this one call.
- `codegraph_node` — Read a single file (pass `file` only) or look up one named symbol (pass `symbol`). Returns source with line numbers + caller/callee trail so you see the blast radius before editing.
- `codegraph_search` — Quick symbol lookup by name. Returns locations only (no code). Use `codegraph_explore` to get the actual source.

**When to use it:**
- Before any edit — run `codegraph_node` on the symbol you're about to change to see who calls it.
- When exploring an unfamiliar area — one `codegraph_explore` call replaces a grep + multi-file Read loop.
- When asking "how does X work" or "where is Y defined" — answer directly from codegraph, no file reading needed.

### gopls
Go language server. Use it for accurate code intelligence instead of plain-text grep:
- `go_diagnostics` — compiler and vet errors for a file
- `go_file_context` — symbols, imports, and structure of a file
- `go_package_api` — exported API of any package
- `go_search` — workspace-wide symbol search
- `go_symbol_references` — find all usages of a symbol
- `go_rename_symbol` — safe rename across the workspace
- `go_vulncheck` — check dependencies for known vulnerabilities

### chrome-devtools
Browser automation. Use for verifying UI changes in the running PWA (port 8080):
- `navigate_page`, `take_screenshot` — open pages and capture state
- `click`, `fill`, `press_key`, `type_text` — interact with the UI
- `evaluate_script` — run JavaScript in the page context
- `list_console_messages`, `list_network_requests` — inspect runtime behavior
- `lighthouse_audit` — performance and accessibility audit

### mail-mcp
E-mail send/receive via IMAP/SMTP. Use to send task-completion notifications (see Development Rule 5):
- Call `list_all_accounts` first to see configured accounts and their send method.
- Send with `smtp_send_message` (or `graph_send_message` / `ews_send_message` depending on account config).
- Always show a full preview to the user and wait for confirmation before sending.

## Development Rules

1. **Work in a separate branch.** Never make changes directly on the `path` branch. If the current branch is `path`, stop and ask the user to create or switch to a feature branch before proceeding.

2. **Never commit without explicit user request.** Do not run `git commit` on your own initiative. Only commit when the user explicitly asks ("commit this", "make a commit", etc.). Preparing and staging changes is fine; committing is not.

3. **Always verify build and tests.** After any code change, run `make build` to confirm the build succeeds and `make test` to confirm all tests pass. Do not report a task as done until both commands exit cleanly.

4. **Keep Bruno collections in sync.** When any REST API endpoint is added, removed, or modified (URL, method, request/response shape), update the corresponding Bruno collection in `tests/api/bruno_collections/` to reflect the change.

5. **Send an e-mail notification on completion.** After finishing a task, send a brief summary e-mail to kipitix@gmail.com describing what was done (2–5 bullet points, no prose padding).

6. **Start the test environment before debugging.** Before any debugging session or manual API testing, ensure the dev database is running with `make db_up`. If the database state looks stale or you hit unexpected data errors, use `make full_restart` to get a clean slate. Never run the server or hit the API endpoints without first confirming the DB container is up.
