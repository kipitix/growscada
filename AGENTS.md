# AGENTS.md

This file provides guidance to AI coding agents working in this repository.

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
| `make build_growctl` | Compiles the `growctl` CLI (declarative project and tag manifests, kubectl-style) to `bin/growctl/` |
| `make build_simulator` | Compiles the device simulator to `bin/growscada_device_simulator/` |
| `make run_simulator` | `build_simulator` + runs it with `tests/device_simulator/example.yaml`: the seeded Device `sim`, its Environment `bench` and its token drive the seeded `device.speed` tag (needs a running server **with a saved state deployed**: a Device and its Tags exist on the Runtime node only from a Deployment; the seed's Project must be saved and that saved state deployed first) |
| `make run_simulator_dashboard` | `build_simulator` + runs it with `tests/device_simulator/seed_dashboard.yaml`: drives the seeded tags bound to the "Main Dashboard" scene, visible in Operation after a Deploy of a saved state (Project mode; `make db_up` alone creates no Deployment) |
| `make run` | `build_server` + starts server + opens Chromium at `localhost:8080` |
| `make db_up` | Starts the dev PostgreSQL container via Docker Compose; applies migrations and seeds automatically |
| `make db_down` | Stops the container and **deletes** the data volume |
| `make test` | Runs all tests with coverage under the race detector (`go test -race --cover ./...`) |
| `make bruno_test` | Builds the server and simulator images (`Dockerfile`), starts them on a fresh seeded database (`tests/api/docker-compose.yaml`, ports 9090/9191 on the host), runs both Bruno collections with a pinned `@usebruno/cli` via `npx` (`--env localhost --exclude-tags=manual`) and removes the containers; needs Docker and Node |
| `make bench` | Runs benchmarks only (`go test -run=^$ -bench=. -benchmem ./...`) |
| `make schemas` | Regenerates the committed JSON schemas `schemas/<contract>/<MAJOR.MINOR>.json` from the `contract/` types; run it after raising a contract's SchemaVersion. Refuses to rewrite a version already on `path`, freezes older versions in `schemas/<contract>/frozen.sha256`, and while MAJOR is 0 prints a NOTE for what in a new MINOR would need a MAJOR after 1.0 |
| `make full_restart` | Clean-slate restart: `db_down` → `db_up` → `run` (use when the DB state is stale or corrupted) |

`build_server` runs two compilations of the same `cmd/combined_server/main.go`: `GOARCH=wasm GOOS=js` for `app.wasm`, then host-arch for the server binary.

## Commands

```bash
# Run a single test
go test ./internal/server/domain/tag/... -run TestTagName

# Create a new migration in a schema's chain (public, engineering or runtime)
goose -dir internal/server/infrastructure/postgres/migrations/engineering create <migration_name> sql

# Apply migrations manually: the three chains, public first
export GOOSE_DRIVER=postgres
export GOOSE_DBSTRING="user=growscada password=growscada host=localhost dbname=growscada"
cd internal/server/infrastructure/postgres/migrations
goose -dir public -allow-missing up   # the seed data shares its goose table
goose -dir engineering -table engineering.goose_db_version up
goose -dir runtime -table runtime.goose_db_version up
```

## Architecture

### Package Layout

`internal/` is split by application. The server's DDD layers live under `internal/server/`; the tools that talk to the server from outside sit next to it and talk to it over the REST API only. The shapes of everything that crosses a process or time boundary live in the public `contract/` package (ADR 0005), which every side may import. What else they may import depends on their role:

- **Engineer tools** (`growctl`) may also import value objects from `internal/server/domain/` (tag name, type, quality, name matchers, project name) to validate input the same way the server does. Never `application/`, `infrastructure/` or `interface/`. `growctl` edits the Draft with `apply`/`diff`/`delete` and follows the same order as the UI: `save <name> --title [--author]` → `deploy <project> --saved-state <n>`, with `saved-states` listing the saved states and `labels`/`label assign|unassign` managing their labels (`--library` reads a Library's history, otherwise a Project is looked up first); a Library's own path is `apply` → `save`, and a Project names its saved state by number in `spec.libraryImports`. The Author is `--author`, else `GROWSCADA_AUTHOR`, else the OS user name; the manifests stay declarative and get no saved-state or Label kind.
- **The roles of the server** (ADR 0011): the Engineering node (`domain/scene`, `domain/library`, `domain/libraryimport`, `domain/label`, `domain/draft`, `application`, `application/appdto`, `postgres/repositories`, `postgres/outbox`) and the Runtime node (`domain/tagstate`, `application/runtime`, `postgres/runtime`) never import each other; the Runtime node builds only on the shared domain (`domain/tag`, `project`, `organization`, `id`, `event`, `version`). The Engineering node reaches the Runtime node only through its port `application.RuntimeNode`, implemented in-process by `internal/server/inprocess` (wired in `main.go` and `servertest`; a networked deployment is not built yet). `internal/server/roles_test.go` checks the imports and that no SQL string of a role names the other role's schema.
- **The Device side** (`devicelink`, `devicesim`, future protocol adapters) depends only on the REST contract (`apiclient` → `contract/api/v0`), never on `internal/server/domain/`: its own enums (`devicelink.TagType`, `devicelink.Quality`) mirror the API's strings, so adapters stay independent of the server's internals (ADR 0004).

```
contract/           # Versioned contracts, public (ADR 0005): SchemaVersion + one package per contract and MAJOR
  api/v0/           # Server API: REST bodies and live events (package apiv0)
  project/v0/       # Project format: ProjectFile, the document a Deployment stores
  record/v0/        # Operational record format: Journal, Checkpoint, PlaybackFile (not modelled yet)
  manifest/v0/      # growctl manifests
  widgetscript/v0/  # WidgetType script contract: the `inputs` of render(inputs) (docs/widget_type_contract.md)
schemas/            # Generated JSON schemas, schemas/<contract>/<MAJOR.MINOR>.json (make schemas)
internal/
  server/           # GrowSCADA server + WASM UI (DDD layers below)
    domain/         # Core business logic — no external dependencies
    application/    # Use-case services, orchestrate domain objects (runtime/: the Runtime node's)
    infrastructure/ # Concrete implementations (Postgres repositories, outbox; postgres/runtime/: the Runtime node's)
    interface/      # Delivery mechanisms (REST API + SSE, EventBus, PWA UI)
    inprocess/      # The roles wired in one process: RuntimeNode over the Runtime node's services
  apiclient/        # Go client of the REST API, apiv0 types as they are; used by growctl, devicelink and the PWA UI
  growctl/          # growctl CLI: manifests, planner
  devicelink/       # Contract by which a Device delivers Tag values to the server (ADR 0004)
  devicesim/        # Lower-level device simulator: patterns, virtual Device, control API
cmd/
  combined_server/  # Entry point — wires everything together
  growctl/          # CLI entry point; logic in internal/growctl
  device_simulator/ # Simulator entry point; logic in internal/devicesim
```

### Layers at a glance

Each layer has its own reference document. **Read the one you are about to change**: `AGENTS.md` keeps only what changes how you work, the detail lives beside the code.

| Layer | Package | Reference |
|-------|---------|-----------|
| Domain | `internal/server/domain/` | `docs/architecture/domain.md` — the aggregates, value objects and repository interfaces, one package per subdomain |
| Application | `internal/server/application/` | `docs/architecture/application.md` — the use-case services, the Draft's transactions, Save and Deploy, Discard, Journal, Checkpoints, labels, settings |
| Infrastructure | `internal/server/infrastructure/` | `docs/architecture/infrastructure.md` — the Postgres repositories, the three migration chains, the outbox and its policy runner |
| Interface | `internal/server/interface/` | `docs/architecture/interface.md` — the REST API and its SSE streams, the EventBus, the PWA UI, the Operation Player |

### Invariants

Violating one of these is a bug even when the code compiles and the tests pass.

- **One transaction, one aggregate** (ADR 0008). A change spanning aggregates is a policy, not a second write; services publish no events; the aggregate records its own and the repository writes them into the outbox with the state.
- **The Draft's Version is the only write token** (ADR 0016, ADR 0022). There is no editing lock: every write of a Project's or a Library's content states the Version it read, and a stale one is 409 `draft_conflict`. Undo and redo are shared between Engineers.
- **A Draft's content is one aggregate.** One write is one transaction, one `DraftChange`, one Version raise; undo and redo apply the change's snapshots, never replay its operation.
- **The order is Save → Deploy.** A Save fixes a saved state, a Deploy makes a Project's saved state the running Deployment, and a Project *imports* a Library's saved state. There are no Releases: no Project Release, no Library Release.
- **Labels, per-user settings and Tag states are not content.** They state no Version, write no `DraftChange`, ride no outbox row, and appear in no ProjectFile, saved state or Deployment (ADR 0011, ADR 0024, ADR 0025).
- **The two roles never import each other** (ADR 0011): the Engineering node reaches the Runtime node only through `application.RuntimeNode`, and `internal/server/roles_test.go` enforces the imports and the schemas each role's SQL may name.
- **A change of a `contract/` type raises its `SchemaVersion`** and runs `make schemas` (ADR 0005, rule 4).
- **A change of a route updates the Bruno collection** (rule 5): the static check in `make test` refuses a route without a request and a request without a route.
- **Migrations go in the chain that owns the table**, applied `public` → `engineering` → `runtime`.

### Single Binary / Dual Compilation

`cmd/combined_server/main.go` calls `app.RunWhenOnBrowser()` near the top — when compiled to WASM this call takes over and runs the PWA; when compiled for the server it is a no-op, and the rest of `main` starts the HTTP servers. Both compilation targets share the same source file.

### Repository Integration Tests

Tests in `internal/server/infrastructure/postgres/repositories/` use **testcontainers-go** to spin up a real Postgres container, apply migrations via Goose, and run assertions. No manual database setup is needed for `go test`.

Tests of the tools that talk to the server (`internal/devicelink`, `internal/devicesim` end-to-end) run the real REST API over Postgres via `internal/server/servertest`: the container starts lazily on first use, so the unit tests of those packages don't need Docker.

## The words that matter

`GLOSSARY.md` is the full vocabulary — read it before naming anything in code, a comment or the UI. The distinctions agents get wrong most often:

- **Tag definition vs Tag state**: the definition (name, TagType, supplier) is content of the Draft; the state (value, Quality, `seq`) is the Runtime node's alone and never part of a ProjectFile.
- **Draft vs saved state**: the Draft is the working content under one `Version`; a saved state is an immutable numbered snapshot a Save fixes (`draft.SavedState` in code), shown as "Saved state #N".
- **Deploy**: making a Project's saved state the running Deployment. A Library is never deployed: a Project imports one of its saved states.
- **Library Import**: the saved state of one Library a Project's Draft uses; the WidgetTypes of that saved state are what the Project sees.
- **Label**: a named mark with an optional color placed on saved states — metadata, never content.

## API Collections

Manual/exploratory API tests are maintained as [Bruno](https://www.usebruno.com/) collections in `tests/api/bruno_collections/`: `growscada_server/` for the server REST API (the `localhost` environment points to `http://localhost:9090`) and `device_simulator/` for the simulator's control API (`localhost:9191`, `TAG_NAME` is the seed tag `max_connections` driven by `tests/device_simulator/seed_dashboard.yaml`). The main automated tests are Go tests; two checks keep the collections from falling behind the API:

- **Static check** (`TestBrunoCollection` in `restapi` and `devicesim`, part of `make test`, no Docker): via `internal/brunotest`, every request must hit a route of the `routes()` table, every route must be covered by a request, and every body must decode strictly into the type that `requestBodies` (in the test) maps to its route.
- **Run** (`make bruno_test`, outside `make test`: GitHub CI and the end of every task, rule 9): both collections against the application in containers on a fresh seeded database, in `seq` order. Each request asserts its status, the `GrowSCADA-Schema-Version` header (server), `application/problem+json` on errors and one or two key fields on reads.

The server collection walks the model's flows in order — a Project created, its Draft edited, saved, its saved state deployed, and a Library saved and imported into a Project; labels created, assigned and filtered. The routes removed with the editing locks, the old `POST …/deploy` and the Releases keep one request each asserting `410`, or none when no route of `routes()` names them.

A request that needs an ID the server generates (the group move's Scene and Widgets) takes it from the answer in an `after-response` script with `bru.setVar`; the `localhost` environment declares the name with a placeholder UUID, which the static check substitutes and a run overwrites.

Bruno tags: `manual` — left out of the run (`get_events`, `get_project_events`: the SSE streams never end); `invalid-body` — a body meant to be rejected, skipped by the body check (it still has to cover its route).

## MCP Servers

Three MCP servers are available (via `opencode.json`) and should be used during development. Keep `opencode.json` plain JSON with no comments and no trailing commas: the `rtk` OpenCode plugin parses it strictly to honour `permissions`, and a JSONC comment only earns a `failed to parse permissions` warning on every shell call.

### codegraph

**Use codegraph BEFORE grep/find or reading files** when you need to understand or locate code. The index is a pre-built SQLite knowledge graph of every symbol, call edge, and file in the workspace — sub-millisecond reads, updated within ~1s of file changes. It lives in `.codegraph/` and is **gitignored**: a new dev host has none until `codegraph init -y` runs there (rule 10).

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

1. **Change code only in a separate branch.** Never change code on the `path` branch: `internal/`, `cmd/`, `contract/`, `schemas/`, `tests/`, migrations, `Makefile` and build or config files. If the current branch is `path`, stop and ask the user to create or switch to a feature branch before touching any of them. Task descriptions and service documents may be edited on `path` directly — `todo/` (backlog, priorities, comments), `docs/`, `GLOSSARY.md`, `AGENTS.md`, `CHANGELOG.md`, `.opencode/`. A change that mixes code with documents goes to a feature branch.

2. **Never commit without explicit user request.** Do not run `git commit` on your own initiative. Only commit when the user explicitly asks ("commit this", "make a commit", etc.). Preparing and staging changes is fine; committing is not.

3. **Always verify build and tests.** After any code change, run `make build` to confirm the build succeeds and `make test` to confirm all tests pass. Do not report a task as done until both commands exit cleanly.

4. **Raise the SchemaVersion when a contract changes.** Any change to a type in `contract/` changes a contract (ADR 0005): raise that contract's `SchemaVersion` — MINOR if old readers can ignore the change (an `omitempty` field, a new event type), MAJOR otherwise (removed/renamed/retyped field, new enum value) — and run `make schemas`. While MAJOR is 0 every change only bumps MINOR. `make test` fails if the types and the committed schema disagree.

5. **Keep Bruno collections in sync.** When any REST API endpoint is added, removed, or modified (URL, method, request/response shape), update the corresponding Bruno collection in `tests/api/bruno_collections/` to reflect the change: every new request has assertions (status, schema header, error content type, key fields), the `seq` order holds (reads, then changes, deletes last; IDs and versions in the bodies follow that order on the seed data), and a new route that takes a body needs an entry in the test's `requestBodies`.

6. **Start the test environment before debugging.** Before any debugging session or manual API testing, ensure the dev database is running with `make db_up`. If the database state looks stale or you hit unexpected data errors, use `make full_restart` to get a clean slate. Never run the server or hit the API endpoints without first confirming the DB container is up.

7. **Ask questions one at a time, interactively.** When interviewing the user (grilling, design sessions, clarifications), ask exactly one question per turn via the interactive question tool (`AskUserQuestion`), with a recommended option first. This overrides any skill instruction to batch several questions into one round.

8. **Audit grilling decisions against DDD.** When a grilling round ends, check every decision it produced against the domain model: aggregate boundaries and invariants, one transaction per aggregate with policies across aggregates (ADR 0008), the layer and role dependencies above (ADR 0011), and the terms of `GLOSSARY.md` and `docs/adr/`. Each mismatch opens another grilling round on that decision (rule 7 applies). The grilling is done when an audit finds no mismatch.

9. **Verify a finished task in the browser and with Bruno.** Once `make build` and `make test` pass (rule 3), bring up the environment (rule 6), run the server and walk every UI flow the task touches in the PWA through `chrome-devtools` (screenshots, console free of errors), then run `make bruno_test`. A task is done when both checks pass; report what each one showed.

10. **Rebuild the codegraph index on a new dev host.** `.codegraph/` is gitignored, so a fresh clone or a new machine has no index: the MCP server starts without a default project and every codegraph call returns nothing until you index. Check once, early in the work — `test -d .codegraph`, or `codegraph status` (`⚠ Not initialized`) — and if it is missing, tell the user plainly that codegraph is not indexed here and that you will fall back to grep/Read until it is, then offer to run `codegraph init -y` in the repository root (about a second; it creates only the gitignored `.codegraph/`). Keep the index current with `codegraph sync` after a pull or a branch switch, and rebuild with `codegraph index` when results look stale.

11. **Notify the user when a turn ends.** There is no notification hook: the agent sends the push itself through [ntfy](https://ntfy.sh) to the topic `kipitix_growscada`, just before the final message of a turn:

```bash
curl -sS -X POST https://ntfy.sh/kipitix_growscada \
  -H "Title: GrowSCADA: <что сделано>" \
  -H "Tags: white_check_mark" \
  -d "<одна-две строки: что сделано и что осталось>"
```

The title and the message are short and in Russian. The topic is public: anyone who knows its name can read it, so put no secrets, tokens, credentials or file contents in the message. A failed request (offline, ntfy unavailable) is not a failure of the task — report it in one line and finish. When the user names another topic, use it for the rest of the session.

12. **Keep `AGENTS.md` small.** This file is loaded into every session, so every line is a cost paid by every agent that works here. It holds what changes how an agent works: rules, commands, dependency constraints, invariants and pointers. Everything else lives in the document that owns it — a layer's architecture in `docs/architecture/<layer>.md`, a decision in `docs/adr/`, a term in `GLOSSARY.md`, a task's reasoning in its `todo/` note — and this file gets at most a one-line pointer to it. Never record history here — what was removed, which task changed what — and keep task numbers out of the layer documents too: a decision names its ADR, work that is not built is said plainly, and `todo/` and `git log` carry the rest. Stay under 25 KB; a change that would push it past that moves an existing section out in the same commit.

## Agent skills

### Issue tracker

Задачи ведутся markdown-файлами в `todo/` (`backlog/` → `done/`). See `docs/agents/issue-tracker.md`.

### Triage labels

Пять канонических ролей, строки совпадают с именами (`Status:` в файле задачи). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `GLOSSARY.md` + `docs/adr/` + `docs/architecture/` в корне. See `docs/agents/domain.md`.
