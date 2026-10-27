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
| `make build_growctl` | Compiles the `growctl` CLI (declarative project and tag manifests, kubectl-style) to `bin/growctl/` |
| `make build_simulator` | Compiles the device simulator to `bin/growscada_device_simulator/` |
| `make run_simulator` | `build_simulator` + runs it with `tests/device_simulator/example.yaml` (needs a running server and the tags from `tests/manifests/example_tags.yaml` in project `Main`) |
| `make run_simulator_dashboard` | `build_simulator` + runs it with `tests/device_simulator/seed_dashboard.yaml`: drives the seeded tags bound to the "Main Dashboard" scene, visible in Operation right after `make db_up` / `make full_restart` |
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

- **Engineer tools** (`growctl`) may also import value objects from `internal/server/domain/` (tag name, type, quality, name matchers, project name) to validate input the same way the server does. Never `application/`, `infrastructure/` or `interface/`.
- **The roles of the server** (ADR 0011): the Engineering node (`domain/scene`, `domain/library`, `application`, `application/appdto`, `postgres/repositories`, `postgres/outbox`) and the Runtime node (`domain/tagstate`, `application/runtime`, `postgres/runtime`) never import each other; the Runtime node builds only on the shared domain (`domain/tag`, `project`, `organization`, `id`, `event`, `version`). The Engineering node reaches the Runtime node only through its port `application.RuntimeNode`, implemented in-process by `internal/server/inprocess` (wired in `main.go` and `servertest`; HTTP — task 52). `internal/server/roles_test.go` checks the imports and that no SQL string of a role names the other role's schema.
- **The Device side** (`devicelink`, `devicesim`, future protocol adapters) depends only on the REST contract (`apiclient` → `contract/api/v0`), never on `internal/server/domain/`: its own enums (`devicelink.TagType`, `devicelink.Quality`) mirror the API's strings, so adapters stay independent of the server's internals (ADR 0004).

```
contract/           # Versioned contracts, public (ADR 0005): SchemaVersion + one package per contract and MAJOR
  api/v0/           # Server API: REST bodies and live events (package apiv0)
  project/v0/       # Project format: ProjectFile = Revision (not modelled yet)
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

### Domain Layer (`internal/server/domain/`)

Each subdomain owns its aggregate, value objects, and repository **interface**:

| Package    | Aggregate / Concept |
|------------|---------------------|
| `project/` | `Project` — unit of isolation (ADR 0006): name unique among the live Projects of its Organization; immutable (`Rename`, `Delete` marks it deleted); `ProjectEvent` (its own events) and `ContentEvent` (events of its Tags, Scenes, Widgets, which name the Project by embedding `project.Content`; no event is both) |
| `organization/` | `Organization` — identity marker of the tenant; the server runs with the one the migrations create |
| `tag/`     | `Tag` — the Tag definition (name, TagType, Version) of one Project (`ProjectID`), the Engineering node's; immutable (`Delete` returns a new Tag); `TagCreatedEvent`, `TagDeletedEvent`; the value objects both roles share (`TagName`, `TagType` with `ZeroValue`, `TagValue`, `TagQuality` — `Good/Bad/Uncertain`) |
| `tagstate/` | `TagState` — the Tag state (value, Quality, `Seq`) with its `Definition`'s copy, the Runtime node's (ADR 0011); immutable (`Write` returns a new one); `Initial` = the TagType's zero value with Quality `Bad`; `TagUpdatedEvent` (a `tag.TagEvent`, never stored in the outbox); `Repository` |
| `scene/`   | `Scene` — named canvas (mnemonic screen) of one Project, binding only Tags of it (`TagTypes` are read through the Project's scope, `ProjectScope.ProjectID()`; TagTypes of another Project are `ErrTagTypesOfAnotherProject`, never taken for missing Tags); immutable aggregate whose methods (`AddWidget`, `UpdateWidget`, `RemoveWidget`, `Update`, `ReconcileWith`, `Delete`) return a new Scene; Scene and Widget events |
| `scene/`   | `Widget` — entity of the Scene aggregate: instance of a WidgetType placed on a Scene, with position/size/rotation/transform matrix |
| `library/` | `WidgetType` — reusable template: HTML template + JavaScript script, InputPorts; immutable (`Update`, `Delete`); its events |
| `event/`   | Base of domain events: `Event`, `Base`, `EventType` (every event name), `EventTimestamp`, `Recorder`. Imports no aggregate |
| `id/`      | Generic `ID[T]` UUID value object; type parameter prevents mixing IDs of different aggregates |
| `version/` | Optimistic-concurrency version value object; Generic `Version[T]` |

### Application Layer (`internal/server/application/`)

Services (`ProjectService`, `TagService`, `WidgetTypeService`, `SceneService`) accept repository interfaces injected from `main.go`. A mutation: parse input → load the aggregate → call its method → `Save`/`Delete` → `appdto`. Services publish no events (ADR 0008).

**Tag definition and Tag state (ADR 0011).** `TagService` keeps the Tag definitions of the Draft and reads each with its state through `RuntimeNode` (`FindTagStates`; a definition without a state shows the initial one, and so does every one when the Runtime node fails to answer). `application/runtime.TagStateService` is the Runtime node's: `SetTagValue` (the hot path, `PATCH …/value`: the state read with the check of its Project in one query, `IsProjectLive` only on a miss; no expected Version, the latest write wins, `seq` from the one sequence `runtime.tag_state_seq` taken in the UPDATE; the write runs apart from the request's cancellation, and after it is stored it publishes `tag_updated` to the EventBus itself, through the queue of `eventbus.AsyncPublisher` (one goroutine does the SSE fan-out; a full queue makes the write wait) — no outbox row, at-most-once), `FindTagStates`, `UpsertTagDefinitions` (idempotent: a Tag without a state gets its initial one; a state of another Project or TagType is an error), `RemoveTagStates`, `RemoveProjectTagStates`; its own `ErrInvalidInput`. Bridge until Deploy (task 35): `CreateTag` upserts the definition right after commit, apart from the request's cancellation, then rereads the Draft and removes the state if the Tag was deleted meanwhile (a 201 Tag accepts values; a failed upsert answers its error, the Tag created), `DeleteTagByID` removes the state at once, and `NewRuntimeTagDefinitionsPolicy` (`sync_runtime_tag_definitions`, key `project:<id>/tag:<id>` on `tag_created`/`tag_deleted`) upserts the Tag's definition if the Draft holds it, else removes its state — making up for a failed call.

**Projects (ADR 0006).** The repositories of a Project's content are opened per Project: `ProjectScopes.ForProject(p)` → `Tags()`, `Scenes()`, every query scoped by `project_id`; saving an aggregate of another Project through them is an error. `ProjectContent` (Tag and Scene services) first checks that the Project is live and of the server's Organization (`ErrProjectNotFound` → 404 of the Project), then opens its scope; the hot path of Tag values (`TagStateService.SetTagValue`, the Runtime node's) checks it in the state's own query and reads the Project only on a miss, and so do the lists the UI reloads on every event (Tags and Scenes by `FindAllInLiveProject`, a Scene's Widgets by the Scene's `FindByIDInLiveProject`), the Project read only when nothing is found. Deleting a Project only marks it; `NewProjectCleanupPolicy` deletes its Scenes, then its Tags, each with its own events, then its Tag states through `RuntimeNode`, and the Project's row last (FKs without CASCADE; a state upserted late holds the row, and the next run removes it); a creation of a Tag or Scene locks the Project's row, live, for share in its transaction (`aggregateStore.lockLiveProject`), so marking it waits for creations and none follows the mark. Job keys of a Project's content start with `project:<id>`.

**Events (ADR 0008).** One transaction changes one aggregate. The aggregate records its own events (`Create…` records Created, `Reconstitute…` — for repositories — nothing; changes and `Delete()` record theirs; see `event.Recorder`). The repository's `Save`/`Delete` writes the state (CAS on the loaded Version) and the pending events into the `outbox` table in one transaction; `outbox.Dispatcher` delivers them after commit, in the order of their transactions (`tx_id`; a row waits for every older transaction to end, so writers take no lock), to the `EventBus` (SSE) and deletes each row — at-least-once. Changes spanning aggregates are **policies** (`application.Policy`: `Name`, `JobKey(event)`, `Run(ctx, key)`): in the transaction that deletes a delivered row the dispatcher enqueues a job per policy that keys the event into `policy_jobs`; jobs with the same key merge (`generation`), and the `outbox.PolicyRunner` runs them apart from delivery, retrying a failure forever with backoff (1 s ×2 → 5 min). A policy brings state into line with what it reads when it runs, never with the event's payload (e.g. `NewWidgetReconciliationPolicy`: Widgets ↔ their changed WidgetType and the Tags they bind, each Scene in its own transaction; `NewUnbindDeletedTagPolicy`: a deleted Tag's PortBindings removed from every Scene of its Project, ADR 0010 — nothing in a Project marked deleted, whose cleanup deletes the Scenes; `NewProjectCleanupPolicy`: see Projects). Operations that state no expected Version (deletes, policies) go through `retryOnRace`: on a write race they reread and retry, after 3 lost races → conflict (409). A Tag definition is deleted without CAS. Writes of Tag states are the exception to all of this: no aggregate of people, no outbox (ADR 0011).

`appdto/` structs are the boundary between the domain and the outside world — not the REST DTOs.

### Infrastructure Layer (`internal/server/infrastructure/`)

- **PostgreSQL repositories** implement domain repository interfaces using `database/sql` + `lib/pq`.
- **Migrations** live in `internal/server/infrastructure/postgres/migrations/` and are managed with [Goose](https://github.com/pressly/goose): one database, three schemas, one chain each (ADR 0011) — `public/` (the server's: organizations, projects, outbox, policy_jobs; goose table `goose_db_version`), `engineering/` (WidgetTypes, Scenes, Widgets, Tag definitions; `engineering.goose_db_version`), `runtime/` (Tag states with the copy of the definitions, `runtime.tag_state_seq`; `runtime.goose_db_version`), applied in that order; the public migration `20261026000000` moved the tables and marks the baseline of the two others. Go code applies them with `migrations.Up` (embedded); the debug `docker-compose.yaml` applies the three chains (a goose service each: the `goose-docker` image runs `goose -dir=/migrations up` whatever the command, the table goes in `GOOSE_EXTRA_ARGS`) and the seed data (`test_data/`, a chain on `goose_db_version`) on `make db_up`. Queries name the role schemas (`engineering.tags`, `runtime.tags`); `public` stays unqualified.
- **Repositories of a Project's content** (`NewProjectScopes`, `NewTagRepositoryPostgres(db, project)`, `NewSceneRepositoryPostgres(db, project)`) carry the Project in their `aggregateStore`; `FindSoleOrganization` reads the one Organization the migrations create (`main.go` fails to start without exactly one).
- **The Runtime node's repository** (`postgres/runtime.NewTagStateRepositoryPostgres`) over `runtime.tags`: reads `public.projects`, never `engineering.*`.
- **Outbox** (`postgres/outbox/`): `Append` (called by the repositories' shared `aggregateStore` template), the internal event codec (not a contract; the payload carries the `project_id` of a content event) and the `Dispatcher` (`Run`, `Notify` — wired to repositories via `repositories.NotifyOnCommit`, `DrainOnce` for deterministic tests; `WithPolicyRunner` makes it enqueue policy jobs) and the `PolicyRunner` (`Run` — woken by the dispatcher and by the timer of the next due job, `RunDueOnce` for deterministic tests); the policy registry is assembled in `main.go`. One dispatcher and one runner per database.

### Interface Layer (`internal/server/interface/`)

**REST API** (`internal/server/interface/restapi/`, port `:9090`):
- Uses stdlib `http.ServeMux` with method-prefixed patterns (Go 1.22+).
- All endpoints live under `apiv0.PathPrefix` (`/api/v0`); every response carries the `GrowSCADA-Schema-Version` header.
- The content of a Project (Tags, Scenes, Widgets, its event stream) lives under `apiv0.ProjectPath(id)` (`/api/v0/projects/{project_id}/…`); Projects (`/projects`, `?name=`) and WidgetTypes are the server's.
- CORS middleware applied globally.
- Errors follow RFC 7807 Problem Details (`problems.go`).
- Each resource has its own handler file (`tags.go`, `widgets.go`, etc.). JSON shapes are `contract/api/v0`; the conversions to/from `appdto` are in `dto_mapping.go`.
- Request bodies are decoded strictly with `decodeRequest` (unknown field → 400); clients read responses tolerantly.
- `NewRouter` takes the services of both roles: `PATCH …/tags/{id}/value` goes to the Runtime node's `TagStateService` (no `version`, answers `seq`), everything else to the Engineering node's; `newAPIEvent` translates `tagstate.TagUpdatedEvent` (the Tag's full state, `seq` included).

**EventBus** (`internal/server/interface/eventbus/`): in-process bus feeding the SSE streams: domain events handed over by the outbox dispatcher, the Runtime node's `tag_updated` (published by `TagStateService` itself through `AsyncPublisher`, ADR 0011), plus delivery's own events (`client_connected`/`client_disconnected`, `system_ready`). The `eventHub` routes a `ContentEvent` to the clients following its Project and everything else — Projects, WidgetTypes, connections — to those following the server's stream: `GET /api/v0/events` is the server's, joined by one Project's with `?project=<id>` (a Project not found adds nothing), `GET /api/v0/projects/{id}/events` one Project's alone. `event_bus_mqtt.go` is a stub.

**PWA UI** (`internal/server/interface/ui/`, port `:8080`):
- Built with [go-app v11](https://go-app.dev/) — compiled to WebAssembly, served by the same binary.
- Four top-level modes rendered by `root/root.go`:
  - **Library** — create and edit WidgetTypes (HTML template + JS script, live sandboxed preview via `<iframe srcdoc>`).
  - **Project** — configure Scenes and place Widget instances on them.
  - **Operation** — runtime operator view.
  - **History** — event log viewer.
- Root's header holds the project switcher (`ui/projects.Switcher`: list, create, rename, delete with confirmation; the selection in LocalStorage, the first Project when it is gone — left at once on `project_deleted` or `uiutil.ActionProjectGone`, which a 404 of a Project's Scene or Tag list (`uiutil.LoadOfProject`) fires instead of a toast). Project and Operation show the selected Project (`SetProject`, its `*apiclient.ProjectClient`; loads are keyed by it, so a response of another Project is dropped); Library does not depend on it. The event Bar keeps one SSE connection, `/events?project=<selected>`, reopened on a switch of Project; on open it fires `EventResync`, then `EventProjectResync` while a Project is selected. On a switch while connected the old connection stays (delivering only the server's events) until the new one opens, which then fires `EventProjectResync` alone: the server's data is not reloaded.
- All REST calls go through one `*apiclient.Client` created in `root` and handed to the modes (`Client.Project(id)` for a Project's content); components work on the `contract/api/v0` types directly (no UI copies) and keep IDs as `uuid.UUID` (`uuid.Nil` = none), parsing text only at LocalStorage/SSE/DOM boundaries (`uiutil.LoadID`, `StoreID`, `ParseID`).
- A call is wrapped by `uiutil.Call(compoCtx, call, onOK, onErr)`: Async → call → Dispatch; `onErr == nil` shows `toast.FromError(err)` (the server's Problem Details, a Bad Response, a Network Error, or a Client Error). `uiutil.Do` hands value and error to one callback; `uiutil.Load` reloads a list through its `Reloader`, `uiutil.LoadFor` a list that belongs to a key (the Widgets of the selected Scene: a response for another key is dropped and the load reruns). The version a write states comes from a `uiutil.Versions` (max of every response; forgotten on a 409 and on resync; a request takes a `Mark` when sent, and a response sent before a forget does not bring the old version back): Project keeps one per Scene (`sceneVersions`), Library one per WidgetType. Lookups by ID go through `uiutil.FindByID` / `IdxByID`.
- Root shows a non-dismissable banner while the server's `GrowSCADA-Schema-Version` differs from the UI's (`apiclient.OnServerVersion`).
- UI tests mount components in go-app's `TestEngine` (`ui/uitest`; on the host go-app calls `OnPreRender`, not `OnMount`, so tests mount a `mountOnHost` wrapper) against the real API from `servertest.StartAPI`.

### Single Binary / Dual Compilation

`cmd/combined_server/main.go` calls `app.RunWhenOnBrowser()` near the top — when compiled to WASM this call takes over and runs the PWA; when compiled for the server it is a no-op, and the rest of `main` starts the HTTP servers. Both compilation targets share the same source file.

### Repository Integration Tests

Tests in `internal/server/infrastructure/postgres/repositories/` use **testcontainers-go** to spin up a real Postgres container, apply migrations via Goose, and run assertions. No manual database setup is needed for `go test`.

Tests of the tools that talk to the server (`internal/devicelink`, `internal/devicesim` end-to-end) run the real REST API over Postgres via `internal/server/servertest`: the container starts lazily on first use, so the unit tests of those packages don't need Docker.

## Key Ubiquitous Language

| Term | Meaning |
|------|---------|
| **Tag** | Single process variable with quality metadata: its Tag definition and its Tag state under one identity |
| **Tag definition** | A Tag's name and TagType: configuration of the Draft, the Engineering node's |
| **Tag state** | A Tag's value and Quality, numbered by `seq`: the Runtime node's; latest write wins; initially the TagType's zero value with Quality `Bad` |
| **TagType** | A Tag's data type: `String`, `Boolean`, `Integer` |
| **WidgetType** | Reusable visual component definition: HTML + JS |
| **Widget** | Placed instance of a WidgetType on a Scene, bound to Tag IDs |
| **Scene** | A mnemonic/synoptic display canvas |
| **Quality** | Data reliability: `Bad`, `Uncertain`, `Good` |
| **Project** | Unit of isolation owning Tags and Scenes; owned by an Organization |
| **Organization** | The tenant owning Projects (one per server for now) |

## API Collections

Manual/exploratory API tests are maintained as [Bruno](https://www.usebruno.com/) collections in `tests/api/bruno_collections/`: `growscada_server/` for the server REST API (the `localhost` environment points to `http://localhost:9090`) and `device_simulator/` for the simulator's control API (`localhost:9191`, `TAG_NAME` is the seed tag `max_connections` driven by `tests/device_simulator/seed_dashboard.yaml`). The main automated tests are Go tests; two checks keep the collections from falling behind the API:

- **Static check** (`TestBrunoCollection` in `restapi` and `devicesim`, part of `make test`, no Docker): via `internal/brunotest`, every request must hit a route of the `routes()` table, every route must be covered by a request, and every body must decode strictly into the type that `requestBodies` (in the test) maps to its route.
- **Run** (`make bruno_test`, outside `make test`: GitHub CI and the end of every task, rule 10): both collections against the application in containers on a fresh seeded database, in `seq` order. Each request asserts its status, the `GrowSCADA-Schema-Version` header (server), `application/problem+json` on errors and one or two key fields on reads.

Bruno tags: `manual` — left out of the run (`get_events`, `get_project_events`: the SSE streams never end); `invalid-body` — a body meant to be rejected, skipped by the body check (it still has to cover its route).

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

5. **Keep Bruno collections in sync.** When any REST API endpoint is added, removed, or modified (URL, method, request/response shape), update the corresponding Bruno collection in `tests/api/bruno_collections/` to reflect the change: every new request has assertions (status, schema header, error content type, key fields), the `seq` order holds (reads, then changes, deletes last; IDs and versions in the bodies follow that order on the seed data), and a new route that takes a body needs an entry in the test's `requestBodies`.

6. **Completion notification is automatic — no action needed.** A `Stop` hook (`.claude/settings.json`) fires a desktop notification (`notify-send`) whenever a turn ends. It's a no-op if `notify-send` isn't installed (e.g. non-Linux, or a Linux desktop without a notification daemon), so it's safe on any machine. Don't try to send an e-mail or otherwise notify the user yourself — the old `mail-mcp`-based e-mail step has been removed.

7. **Start the test environment before debugging.** Before any debugging session or manual API testing, ensure the dev database is running with `make db_up`. If the database state looks stale or you hit unexpected data errors, use `make full_restart` to get a clean slate. Never run the server or hit the API endpoints without first confirming the DB container is up.

8. **Ask questions one at a time, interactively.** When interviewing the user (grilling, design sessions, clarifications), ask exactly one question per turn via the interactive question tool (`AskUserQuestion`), with a recommended option first. This overrides any skill instruction to batch several questions into one round.

9. **Audit grilling decisions against DDD.** When a grilling round ends, check every decision it produced against the domain model: aggregate boundaries and invariants, one transaction per aggregate with policies across aggregates (ADR 0008), the layer and role dependencies above (ADR 0011), and the terms of `CONTEXT.md` and `docs/adr/`. Each mismatch opens another grilling round on that decision (rule 8 applies). The grilling is done when an audit finds no mismatch.

10. **Verify a finished task in the browser and with Bruno.** Once `make build` and `make test` pass (rule 3), bring up the environment (rule 7), run the server and walk every UI flow the task touches in the PWA through `chrome-devtools` (screenshots, console free of errors), then run `make bruno_test`. A task is done when both checks pass; report what each one showed.

## Agent skills

### Issue tracker

Задачи ведутся markdown-файлами в `todo/` (`backlog/` → `done/`). See `docs/agents/issue-tracker.md`.

### Triage labels

Пять канонических ролей, строки совпадают с именами (`Status:` в файле задачи). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `CONTEXT.md` + `docs/adr/` в корне. See `docs/agents/domain.md`.
