# AGENTS.md

This file provides guidance to AI coding agents (Claude Code, Codex, etc.) working in this repository.

## Language

Communicate with the user in Russian.

## Project Overview

GrowSCADA is a web-based SCADA system built in Go, following Domain-Driven Design (DDD). The backend and PWA frontend are compiled from the same Go codebase into a single binary — the server handles the API and serves the WASM frontend.

## Makefile Reference

All primary workflows go through `make`. **Always prefer `make <target>` over running raw commands directly.**

| Target | What it does |
|--------|-------------|
| `make install_tools` | Installs `goose` migration tool via `go install` |
| `make build` | Builds everything: `build_server` + `build_growctl` + `build_simulator` |
| `make build_server` | Compiles WASM frontend (`app.wasm`) and server binary to `bin/growscada_combined_server/` |
| `make build_growctl` | Compiles the `growctl` CLI (declarative tag manifests, kubectl-style) to `bin/growctl/` |
| `make build_simulator` | Compiles the device simulator to `bin/growscada_device_simulator/` |
| `make run_simulator` | `build_simulator` + runs it with `tests/device_simulator/example.yaml` (needs a running server and the tags from `tests/manifests/example_tags.yaml`) |
| `make run_simulator_dashboard` | `build_simulator` + runs it with `tests/device_simulator/seed_dashboard.yaml`: drives the seeded tags bound to the "Main Dashboard" scene, visible in Operation right after `make db_up` / `make full_restart` |
| `make run` | `build_server` + starts server + opens Chromium at `localhost:8080` |
| `make db_up` | Starts the dev PostgreSQL container via Docker Compose; applies migrations and seeds automatically |
| `make db_down` | Stops the container and **deletes** the data volume |
| `make test` | Runs all tests with coverage (`go test --cover ./...`) |
| `make bench` | Runs benchmarks only (`go test -run=^$ -bench=. -benchmem ./...`) |
| `make schemas` | Regenerates the committed JSON schemas `schemas/<contract>/<MAJOR.MINOR>.json` from the `contract/` types; run it after raising a contract's SchemaVersion. Refuses to rewrite a version already on `path`, freezes older versions in `schemas/<contract>/frozen.sha256`, and while MAJOR is 0 prints a NOTE for what in a new MINOR would need a MAJOR after 1.0 |
| `make full_restart` | Clean-slate restart: `db_down` → `db_up` → `run` (use when the DB state is stale or corrupted) |

`build_server` runs two compilations of the same `cmd/combined_server/main.go`: `GOARCH=wasm GOOS=js` for `app.wasm`, then host-arch for the server binary.

## Commands

```bash
# Run a single test
go test ./internal/server/domain/tag/... -run TestTagName

# Create a new migration
goose create <migration_name> sql

# Apply migrations manually
export GOOSE_DRIVER=postgres
export GOOSE_DBSTRING="user=growscada password=growscada host=localhost dbname=growscada"
goose up
```

## Architecture

### Package Layout

`internal/` is split by application. The server's DDD layers live under `internal/server/`; the tools that talk to the server from outside sit next to it and talk to it over the REST API only. The shapes of everything that crosses a process or time boundary live in the public `contract/` package (ADR 0005), which every side may import. What else they may import depends on their role:

- **Engineer tools** (`growctl`) may also import value objects from `internal/server/domain/` (tag name, type, quality, name matchers) to validate input the same way the server does. Never `application/`, `infrastructure/` or `interface/`.
- **The Device side** (`devicelink`, `devicesim`, future protocol adapters) depends only on the REST contract (`apiclient` → `contract/api/v0`), never on `internal/server/domain/`: its own enums (`devicelink.TagType`, `devicelink.Quality`) mirror the API's strings, so adapters stay independent of the server's internals (ADR 0004).

```
contract/           # Versioned contracts, public (ADR 0005): SchemaVersion + one package per contract and MAJOR
  api/v0/           # Server API: REST bodies and live events (package apiv0)
  project/v0/       # Project format: ProjectFile = Revision (not modelled yet)
  record/v0/        # Operational record format: Journal, Checkpoint, PlaybackFile (not modelled yet)
  manifest/v0/      # growctl manifests
schemas/            # Generated JSON schemas, schemas/<contract>/<MAJOR.MINOR>.json (make schemas)
internal/
  server/           # GrowSCADA server + WASM UI (DDD layers below)
    domain/         # Core business logic — no external dependencies
    application/    # Use-case services, orchestrate domain objects
    infrastructure/ # Concrete implementations (Postgres repositories, outbox)
    interface/      # Delivery mechanisms (REST API + SSE, EventBus, PWA UI)
  apiclient/        # Go client of the REST API (JSON shapes from contract/api/v0)
  growctl/          # growctl CLI: manifests, planner
  devicelink/       # Contract by which a Device delivers Tag values to the server (ADR 0004)
  devicesim/        # Lower-level device simulator: patterns, virtual Device, control API
cmd/
  combined_server/  # Entry point — wires everything together
  growctl/          # CLI entry point; logic in internal/growctl
  device_simulator/ # Simulator entry point; logic in internal/devicesim
```

### Domain Layer (`internal/server/domain/`)

Each subdomain owns its aggregate, value objects, and repository **interface**:

| Package    | Aggregate / Concept |
|------------|---------------------|
| `tag/`     | `Tag` — atomic SCADA data point with type, value, quality (`Good/Bad/Uncertain`); its events (`TagCreatedEvent`, …) |
| `scene/`   | `Scene` — named canvas (mnemonic screen); immutable aggregate whose methods (`AddWidget`, `UpdateWidget`, `RemoveWidget`, `Update`, `ReconcileWith`, `Delete`) return a new Scene; Scene and Widget events |
| `scene/`   | `Widget` — entity of the Scene aggregate: instance of a WidgetType placed on a Scene, with position/size/rotation/transform matrix |
| `library/` | `WidgetType` — reusable template: HTML template + JavaScript script, InputPorts; immutable (`Update`, `Delete`); its events |
| `event/`   | Base of domain events: `Event`, `Base`, `EventType` (every event name), `EventTimestamp`, `Recorder`. Imports no aggregate |
| `id/`      | Generic `ID[T]` UUID value object; type parameter prevents mixing IDs of different aggregates |
| `version/` | Optimistic-concurrency version value object; Generic `Version[T]` |

### Application Layer (`internal/server/application/`)

Services (`TagService`, `WidgetTypeService`, `SceneService`) accept repository interfaces injected from `main.go`. A mutation: parse input → load the aggregate → call its method → `Save`/`Delete` → `appdto`. Services publish no events (ADR 0008).

**Events (ADR 0008).** One transaction changes one aggregate. The aggregate records its own events (`Create…` records Created, `Reconstitute…` — for repositories — nothing; changes and `Delete()` record theirs; see `event.Recorder`). The repository's `Save`/`Delete` writes the state (CAS on the loaded Version) and the pending events into the `outbox` table in one transaction; `outbox.Dispatcher` delivers them after commit, in the order of their transactions (`tx_id`; a row waits for every older transaction to end, so writers take no lock), to the `EventBus` (SSE) and deletes each row — at-least-once. Operations that state no expected Version (deletes) go through `retryOnRace`: on a write race they reread and retry, after 3 lost races → conflict (409). A Tag is deleted without CAS: after creation only its value changes.

`appdto/` structs are the boundary between the domain and the outside world — not the REST DTOs.

### Infrastructure Layer (`internal/server/infrastructure/`)

- **PostgreSQL repositories** implement domain repository interfaces using `database/sql` + `lib/pq`.
- **Migrations** live in `internal/server/infrastructure/postgres/migrations/` and are managed with [Goose](https://github.com/pressly/goose). The debug `docker-compose.yaml` automatically applies migrations and seed data on `make db_up`.
- **Outbox** (`postgres/outbox/`): `Append` (called by the repositories' shared `aggregateStore` template), the internal event codec (not a contract) and the `Dispatcher` (`Run`, `Notify` — wired to repositories via `repositories.NotifyOnCommit`, `DrainOnce` for deterministic tests). One dispatcher per database.

### Interface Layer (`internal/server/interface/`)

**REST API** (`internal/server/interface/restapi/`, port `:9090`):
- Uses stdlib `http.ServeMux` with method-prefixed patterns (Go 1.22+).
- All endpoints live under `apiv0.PathPrefix` (`/api/v0`); every response carries the `GrowSCADA-Schema-Version` header.
- CORS middleware applied globally.
- Errors follow RFC 7807 Problem Details (`problems.go`).
- Each resource has its own handler file (`tags.go`, `widgets.go`, etc.). JSON shapes are `contract/api/v0`; the conversions to/from `appdto` are in `dto_mapping.go`.
- Request bodies are decoded strictly with `decodeRequest` (unknown field → 400); clients read responses tolerantly.

**EventBus** (`internal/server/interface/eventbus/`): in-process bus feeding the SSE stream (`GET /api/v0/events`): domain events handed over by the outbox dispatcher, plus delivery's own events (`client_connected`/`client_disconnected`, `system_ready`). `event_bus_mqtt.go` is a stub.

**PWA UI** (`internal/server/interface/ui/`, port `:8080`):
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

Tests in `internal/server/infrastructure/postgres/repositories/` use **testcontainers-go** to spin up a real Postgres container, apply migrations via Goose, and run assertions. No manual database setup is needed for `go test`.

Tests of the tools that talk to the server (`internal/devicelink`, `internal/devicesim` end-to-end) run the real REST API over Postgres via `internal/server/servertest`: the container starts lazily on first use, so the unit tests of those packages don't need Docker.

## Key Ubiquitous Language

| Term | Meaning |
|------|---------|
| **Tag** | Single process variable with quality metadata |
| **TagType** | A Tag's data type: `String`, `Boolean`, `Integer` |
| **WidgetType** | Reusable visual component definition: HTML + JS |
| **Widget** | Placed instance of a WidgetType on a Scene, bound to Tag IDs |
| **Scene** | A mnemonic/synoptic display canvas |
| **Quality** | Data reliability: `Bad`, `Uncertain`, `Good` |

## API Collections

Manual/exploratory API tests are maintained as [Bruno](https://www.usebruno.com/) collections in `tests/api/bruno_collections/`: `growscada_server/` for the server REST API (the `localhost` environment points to `http://localhost:9090`) and `device_simulator/` for the simulator's control API (`localhost:9191`). Automated tests are Go tests only.

## MCP Servers

Three MCP servers are available (via `.mcp.json`) and should be used during development:

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

## Development Rules

1. **Work in a separate branch.** Never make changes directly on the `path` branch. If the current branch is `path`, stop and ask the user to create or switch to a feature branch before proceeding.

2. **Never commit without explicit user request.** Do not run `git commit` on your own initiative. Only commit when the user explicitly asks ("commit this", "make a commit", etc.). Preparing and staging changes is fine; committing is not.

3. **Always verify build and tests.** After any code change, run `make build` to confirm the build succeeds and `make test` to confirm all tests pass. Do not report a task as done until both commands exit cleanly.

4. **Raise the SchemaVersion when a contract changes.** Any change to a type in `contract/` changes a contract (ADR 0005): raise that contract's `SchemaVersion` — MINOR if old readers can ignore the change (an `omitempty` field, a new event type), MAJOR otherwise (removed/renamed/retyped field, new enum value) — and run `make schemas`. While MAJOR is 0 every change only bumps MINOR. `make test` fails if the types and the committed schema disagree.

5. **Keep Bruno collections in sync.** When any REST API endpoint is added, removed, or modified (URL, method, request/response shape), update the corresponding Bruno collection in `tests/api/bruno_collections/` to reflect the change.

6. **Completion notification is automatic — no action needed.** A `Stop` hook (`.claude/settings.json`) fires a desktop notification (`notify-send`) whenever a turn ends. It's a no-op if `notify-send` isn't installed (e.g. non-Linux, or a Linux desktop without a notification daemon), so it's safe on any machine. Don't try to send an e-mail or otherwise notify the user yourself — the old `mail-mcp`-based e-mail step has been removed.

7. **Start the test environment before debugging.** Before any debugging session or manual API testing, ensure the dev database is running with `make db_up`. If the database state looks stale or you hit unexpected data errors, use `make full_restart` to get a clean slate. Never run the server or hit the API endpoints without first confirming the DB container is up.

8. **Ask questions one at a time, interactively.** When interviewing the user (grilling, design sessions, clarifications), ask exactly one question per turn via the interactive question tool (`AskUserQuestion`), with a recommended option first. This overrides any skill instruction to batch several questions into one round.

## Agent skills

### Issue tracker

Задачи ведутся markdown-файлами в `todo/` (`backlog/` → `done/`). See `docs/agents/issue-tracker.md`.

### Triage labels

Пять канонических ролей, строки совпадают с именами (`Status:` в файле задачи). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `CONTEXT.md` + `docs/adr/` в корне. See `docs/agents/domain.md`.
