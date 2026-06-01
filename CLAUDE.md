# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

GrowSCADA is a web-based SCADA system built in Go, following Domain-Driven Design (DDD). The backend and PWA frontend are compiled from the same Go codebase into a single binary — the server handles the API and serves the WASM frontend.

## Commands

```bash
# Install required tools (goose for migrations)
make install_tools

# Build both server binary and WASM frontend
make build

# Build + run (opens Chromium on localhost:8080)
make run

# Run all tests (unit + integration via testcontainers)
make test

# Run a single test
go test ./internal/domain/tag/... -run TestTagName

# Start/stop the development PostgreSQL database
make db_up
make db_down

# Create a new migration
goose create <migration_name> sql

# Apply migrations manually
export GOOSE_DRIVER=postgres
export GOOSE_DBSTRING="user=growscada password=growscada host=localhost dbname=growscada"
goose up
```

The `make build` step runs two compilations: once with `GOARCH=wasm GOOS=js` to produce `app.wasm`, and once for the host platform to produce the server binary. Both outputs land in `bin/growscada_combined_server/`.

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

Three MCP servers are available and should be used during development:

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

5. **Verify UI changes in the browser.** After any UI change, launch the app and open it in the browser to confirm the result looks and behaves correctly. Use the `chrome-devtools` MCP server (`take_screenshot`, `click`, etc.) to interact with and inspect the running PWA on port 8080.

6. **Send an e-mail notification on completion.** After finishing a task, send a brief summary e-mail to kipitix@gmail.com describing what was done (2–5 bullet points, no prose padding).
