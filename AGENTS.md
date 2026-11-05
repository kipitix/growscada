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
| `make run_simulator` | `build_simulator` + runs it with `tests/device_simulator/example.yaml`: the seeded Device `sim`, its Environment `bench` and its token drive the seeded `device.speed` tag (needs a running server **with its Project deployed**: a Device and its Tags exist on the Runtime node only from a Revision) |
| `make run_simulator_dashboard` | `build_simulator` + runs it with `tests/device_simulator/seed_dashboard.yaml`: drives the seeded tags bound to the "Main Dashboard" scene, visible in Operation after a Deploy of the Project (Project mode; `make db_up` alone creates no Revision) |
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
- **The roles of the server** (ADR 0011): the Engineering node (`domain/scene`, `domain/library`, `domain/librarypin`, `domain/editlock`, `application`, `application/appdto`, `postgres/repositories`, `postgres/outbox`) and the Runtime node (`domain/tagstate`, `application/runtime`, `postgres/runtime`) never import each other; the Runtime node builds only on the shared domain (`domain/tag`, `project`, `organization`, `id`, `event`, `version`). The Engineering node reaches the Runtime node only through its port `application.RuntimeNode`, implemented in-process by `internal/server/inprocess` (wired in `main.go` and `servertest`; HTTP — task 52). `internal/server/roles_test.go` checks the imports and that no SQL string of a role names the other role's schema.
- **The Device side** (`devicelink`, `devicesim`, future protocol adapters) depends only on the REST contract (`apiclient` → `contract/api/v0`), never on `internal/server/domain/`: its own enums (`devicelink.TagType`, `devicelink.Quality`) mirror the API's strings, so adapters stay independent of the server's internals (ADR 0004).

```
contract/           # Versioned contracts, public (ADR 0005): SchemaVersion + one package per contract and MAJOR
  api/v0/           # Server API: REST bodies and live events (package apiv0)
  project/v0/       # Project format: ProjectFile, the document a Revision stores
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
| `project/` | `Project` — unit of isolation (ADR 0006): name unique among the live Projects of its Organization; immutable (`Rename`, `Delete` marks it deleted); `ProjectUpdatedEvent`; `ProjectEvent` (its own events) and `ContentEvent` (events of its Tags, Scenes, Widgets, which name the Project by embedding `project.Content`; no event is both) |
| `organization/` | `Organization` — identity marker of the tenant; the server runs with the one the migrations create |
| `tag/`     | `Tag` — the Tag definition (name, TagType, supplier) of one Project (`ProjectID`), an entity of its Draft, the Engineering node's; immutable (`Delete` returns a new Tag); `TagCreatedEvent`, `TagDeletedEvent`; the value objects both roles share (`TagName`, `TagType` with `ZeroValue`, `TagValue`, `TagQuality` — `Good/Bad/Uncertain`) |
| `tagstate/` | `TagState` — the Tag state (value, Quality, `Seq`) with its `Definition`'s copy, the Runtime node's (ADR 0011); immutable (`Write` returns a new one); `Initial` = the TagType's zero value with Quality `Bad`; `TagUpdatedEvent` (a `tag.TagEvent`, never stored in the outbox); `Repository` |
| `draft/`   | `Draft` — the working state of one Project or one Library, the unit of consistency of its content (ADR 0016, ADR 0017): its `Scope`, the one `Version` of its content, its EditLock (`lock_version`, holder), its `StackPosition`, and the operations over the content (`CreateTag`, `DeleteTag` unbinding Scenes, Scenes, Widgets, Devices, Environments, pairs, pins, `Replace`, `Purge`); `draft.Session` is one write in the Draft's transaction, `draft.Snapshot` the before/after state of one touched entity; `DraftChange` rows live in `engineering.draft_changes` |
| `scene/`   | `Scene` — named canvas (mnemonic screen) of one Project, binding only Tags of it (`TagTypes` are read through the Project's scope, `ProjectScope.ProjectID()`; TagTypes of another Project are `ErrTagTypesOfAnotherProject`, never taken for missing Tags); immutable aggregate whose methods (`AddWidget`, `UpdateWidget`, `RemoveWidget`, `Update`, `ReconcileWith`, `Delete`) return a new Scene; Scene and Widget events |
| `scene/`   | `Widget` — entity of the Scene aggregate: instance of a WidgetType placed on a Scene, with position/size/rotation/transform matrix |
| `library/` | `Library` — reusable collection of WidgetTypes of an Organization, outside every Project (ADR 0007): name unique among its live Libraries; immutable (`Rename`, `Delete` marks it deleted, refused while a live Project pins it; `StartDiscard`/`FinishDiscard` mark it as being reset to its latest release, its Draft read-only meanwhile, ADR 0012); `LibraryEvent` (its own events and `library_released`) |
| `library/` | `WidgetType` — reusable template of one Library's Draft (`LibraryID`, name unique in it): HTML template + JavaScript script, InputPorts; immutable (`Update`, `ResetTo` and `RecreateWidgetType` for a Discard, `Delete` — touches no Scene); its events name the Library. Opened per Library (`WidgetTypeRepositories.ForLibrary`, `ForDiscard`) |
| `library/` | `LibraryRelease` — immutable snapshot of a Draft, never removed but with its Library: number (latest + 1), time, note, copies `ReleasedWidgetType` (value object, no Version — what Projects see and Widgets are checked against); `CreateRelease` refuses an unchanged Draft (`DraftDiffers`, also the "unreleased changes" flag) and a Library being discarded |
| `librarypin/` | `LibraryPin` — the Library Release of one Library a Project's Draft uses, one per pair (`ContentEvent`); `Switch` with an expected Version, `Delete`; `ImpactOf` — the one function computing what a switch or removal does to the Project's Widgets (orphaned → refused, `OrphansError`; broken PortBindings → removed by a policy), used by the preview and the check |
| `editlock/` | `EditLock` — the marker that one holder — today a Client (a tab's key plus a declared name), later a User — holds one Draft for editing (ADR 0015): one `Scope` (Project or Library), `Acquire`, `Release`, `TakeOver(expectedVersion)`, `Expire`, `Seen` (raises `last_seen` without a Version); the Version grows monotonically across holders, the row staying when the lock is free; `Repository` (`Find`, `Create`, `Save` with CAS, `FindAllHeld`, `Touch`); events `edit_lock_acquired`/`_released`/`_taken_over`/`_expired` carry the `Scope` and the new state |
| `event/`   | Base of domain events: `Event`, `Base`, `EventType` (every event name), `EventTimestamp`, `Recorder`. Imports no aggregate |
| `id/`      | Generic `ID[T]` UUID value object; type parameter prevents mixing IDs of different aggregates |
| `version/` | Optimistic-concurrency version value object; Generic `Version[T]` |

### Application Layer (`internal/server/application/`)

Services (`ProjectService`, `TagService`, `LibraryService`, `WidgetTypeService`, `LibraryPinService`, `SceneService`, `DeviceService`, `EnvironmentService`, `DeployService`, `DiscardService`) accept repository interfaces injected from `main.go`. A mutation: parse input → load the aggregate → call its method → `Save`/`Delete` → `appdto`. Services publish no events (ADR 0008).

**Deploy, Rollback and Discard (task 35, part 4; task 36).** `ProjectFileBuilder` assembles the ProjectFile of a Draft over its `ProjectScope` and pins — the Tags with their supplier, the Devices, the Environments with their pairs, the Scenes with their Widgets, the WidgetTypes those Widgets use from the pinned Releases and the pins as provenance — canonicalized (lists by ID, JSON keys sorted), so one Draft is always the same bytes; a dangling reference (a Widget of a type no pinned release holds, a Tag supplied by a Device of another Project, a PortBinding naming a Tag the Draft lacks) fails the build. `DeployService` compares the document with the Revision the Runtime node runs: unchanged → 409 `ErrNothingToDeploy`; otherwise `RuntimeNode.Deploy`. It keeps no copy of what it sent: the Revision is the only truth (ADR 0014), and `Preview` (current Revision, `unchanged`, `dropped_widgets` — always empty until task 48) is what the Project mode reads. The Runtime node's `RevisionService` also serves `List` (newest first, no documents, empty for a Project never deployed), `Find` (by number, with the document; `ErrRevisionNotFound`) and `Rollback` — a new Revision with an earlier Revision's document untouched (`cause` `rollback`, `rolled_back_from` the copied number), committed with the same derived-copy diff as a Deploy; a document equal to the running one is 409 `ErrNothingToRollback`. A Rollback publishes the same `revision_created`. A Project that is not live is `project.ErrProjectNotFound` (409 for a Deploy too). After a successful Deploy the DraftChange stack is cleared in a transaction of its own (a failure to clear is 500 while the Revision stands, ADR 0017). `DiscardService` answers 409 `ErrNoRevisionToDiscard` without a Revision, else reads the running Revision through `RuntimeNode` and replaces the whole Draft from it in one transaction (`draft.Repository.Replace`, which also clears the stack), answering 200 with the Draft's `{version}`. The Tag states and the Device Tokens are the Runtime node's and stay.

**Tag definition and Tag state (ADR 0011).** `TagService` keeps the Tag definitions of the Draft (entities of it, ADR 0016) and reads each with its state through `RuntimeNode` (`FindTagStates`; a definition without a state shows the initial one, and so does every one when the Runtime node fails to answer). `application/runtime.TagStateService` is the Runtime node's: `SetTagValue` (the hot path, `PATCH …/value`: the state read with the check of its Project in one query, `IsProjectLive` only on a miss; no expected Version, the latest write wins; the writer's Device is checked against the Tag's supplier in the same UPDATE (`tag.ErrTagNotSuppliedByDevice`; a Tag people write has `tag.NoSupplier`); `seq` from the one sequence `runtime.tag_state_seq` taken in the UPDATE; the write runs apart from the request's cancellation, and after it is stored it publishes `tag_updated` to the EventBus itself, through the queue of `eventbus.AsyncPublisher` (one goroutine does the SSE fan-out; a full queue makes the write wait) — no outbox row, at-most-once), `FindTagStates`, `UpsertTagDefinitions` (idempotent: a Tag without a state gets its initial one; a state of another Project or TagType is an error), `RemoveTagStates`, `RemoveProjectTagStates`; its own `ErrInvalidInput`. The copies are the running Revision's alone: `CreateTag` and `DeleteTagByID` write nothing to the Runtime node, and `UpsertDefinitions` and `Delete` take the advisory lock of `CreateRevision` and check for a Revision in the same transaction, so a Tag created in the Draft takes no value (404) until the next Deploy names it, and a Tag deleted in the Draft keeps its state until the next Deploy drops it (ADR 0011, amendment 2026-11-04; the bridge of the Draft is gone, task 35, part 4).

**Projects (ADR 0006).** The repositories of a Project's content are opened per Project: `ProjectScopes.ForProject(p)` → `Tags()`, `Scenes()`, `LibraryPins()`, `Devices()`, `Environments()`, `EnvironmentDevices()`, every query scoped by `project_id`; saving an aggregate of another Project through them is an error. `ProjectContent` (the Tag, Scene, Device and Environment services) first checks that the Project is live and of the server's Organization (`ErrProjectNotFound` → 404 of the Project), then opens its scope; the hot path of Tag values (`TagStateService.SetTagValue`, the Runtime node's) checks it in the state's own query and reads the Project only on a miss, and so do the lists the UI reloads on every event (Tags, Scenes, Devices and Environments by `FindAllInLiveProject`, a Scene's Widgets by the Scene's `FindByIDInLiveProject`), the Project read only when nothing is found. Deleting a Project only marks it; `NewProjectCleanupPolicy` purges the whole Draft in one transaction over it (`draft.Repository.Purge`, ADR 0016) with the events of the removal, then its Tag states through `RuntimeNode`, and the Project's row last (a deleted Project has no EditLock check; its Device Tokens go with it by the foreign key, and `RuntimeNode` forgets the ones its cache resolved, or a token of the removed Project would keep authenticating; a state upserted late holds the row, and the next run removes it). A Draft's content is written through its repository, which locks the Draft's row, checks the Project is live and the EditLock, and writes the touched rows in the same transaction; no separate row locks remain. Job keys of a Project's content start with `project:<id>`.

**Libraries (ADR 0007, ADR 0012).** A Project sees WidgetTypes only through its Library Pins: `SceneService` finds a Widget's WidgetType among the releases the Project pins (`ProjectScope.LibraryPins().FindPinnedWidgetType`), and the Scene's save locks that pin's row `FOR SHARE` and checks the type in the pinned release in its transaction (`checkPinned`) for each Widget added or retyped; a switch or removal of a pin (`LibraryPinService.PutLibraryPin`/`DeleteLibraryPin`, the presence of a pin decides add or switch and `version` is the Draft's) runs `ImpactOf` on the Project's Widgets read in the Draft's transaction, in the same DraftChange that switches the pin. A write of a WidgetType, and a Release, lock the Library's row `FOR SHARE` and check the Discard mark; a pin locks the live Library's row `FOR SHARE`, so a Library is marked deleted only while no live Project pins it. `LibraryService.Release` takes the next number and retries on `ErrReleaseNumberTaken`; `Discard` only marks (`NewDiscardLibraryPolicy` resets each WidgetType through `ForDiscard` and clears the mark last); `NewLibraryCleanupPolicy` removes a deleted Library with its Draft and releases once no Project pins it. A Library's Draft and EditLock sit on `engineering.drafts` too (ADR 0016); the WidgetType and Release locks stay until task 38, part 2.

**EditLock (ADR 0015, ADR 0016).** The EditLock is a field of the Draft's row (`engineering.drafts`): `application.EditLockService` is the Engineering node's one editor per Draft — a Project's or a Library's, named by an `editlock.Scope`: `State`, `CheckOut` (201 when free, 200 for the same client, 409 with the holder), `CheckIn` (200, idempotent when free), `TakeOver` (states the observed `lock_version`; stale or free is 409), `CheckHeld` (the gate Deploy reads before it builds), and `Attach`/`Detach` — presence, the keys with an open SSE stream, refcounted in memory of the node. The Draft's live scope is checked first (`ProjectContent`; `LibraryContent` for a Library, which also refuses one of another Organization). `ReapOnce` (run every 30 s by `RunEditLockReaper`, from `main.go` and `servertest`) raises `last_seen` of every present holder and frees a lock of an absent one not seen for `EditLockTTL = 5 min` with `edit_lock_expired`. `lock_version` grows only when the holder changes, so a Take over stays possible while the holder edits. The check of a write lives in its transaction: `draftRepositoryPostgres.Update` reads the Draft's row `FOR UPDATE`, compares the holder with the key of the context — a request names its client (`X-GrowSCADA-Client`), a request that names none is `ErrNoClient` (400), a context without identity refuses, and `editlock.System` (policies, cleanup) passes — and raises `last_seen` for the holder. Deploy checks before the build (it has no single transaction); a Library's Release is gated in `LibraryReleaseRepository.Save`, with the row lock and `ErrDraftChanged`'s check. A Discard is one transaction over the Draft and the check is its own.

**Events (ADR 0008).** One transaction changes one aggregate. The aggregate records its own events (`Create…` records Created, `Reconstitute…` — for repositories — nothing; changes and `Delete()` record theirs; see `event.Recorder`). The repository's `Save`/`Delete` writes the state (CAS on the loaded Version) and the pending events into the `outbox` table in one transaction; `outbox.Dispatcher` delivers them after commit, in the order of their transactions (`tx_id`; a row waits for every older transaction to end, so writers take no lock), to the `EventBus` (SSE) and deletes each row — at-least-once. Changes spanning aggregates are **policies** (`application.Policy`: `Name`, `JobKey(event)`, `Run(ctx, key)`): in the transaction that deletes a delivered row the dispatcher enqueues a job per policy that keys the event into `policy_jobs`; jobs with the same key merge (`generation`), and the `outbox.PolicyRunner` runs them apart from delivery, retrying a failure forever with backoff (1 s ×2 → 5 min). A policy brings state into line with what it reads when it runs, never with the event's payload (`NewProjectCleanupPolicy`: see Projects; `NewLibraryCleanupPolicy`: see Libraries). `NewUnbindDeletedTagPolicy` and `NewWidgetReconciliationPolicy` are gone (ADR 0016): a Tag's bindings are removed by the DraftChange that deletes the Tag, and a pin switch removes the bindings it breaks in its own change. A Draft's content is one aggregate, so its writes state the Draft's Version, and `retryOnRace` remains only for writes that are not DraftChanges (the cleanup of a Project, service operations). Writes of Tag states are the exception to all of this: no aggregate of people, no outbox (ADR 0011).

`appdto/` structs are the boundary between the domain and the outside world — not the REST DTOs.

**Devices and Environments (ADR 0004, amendment 2026-11-01).** `DeviceService` and `EnvironmentService` are the Engineering node's: a Project's Devices (protocol and configuration), its Environments and the pairs `(Environment, Device)` with their connection parameters. `DeviceConfigService` (`application/runtime`, the Runtime node's) serves a Device Gateway the configuration of its Device in one Environment from the running Revision: the copies `runtime.devices`, `runtime.environments`, `runtime.environment_devices` and `runtime.tags`, read in one REPEATABLE READ transaction, never the Draft — a Device outside the Revision is 403, an Environment or a pair it lacks is 404 (task 35, Q5). The Device Token is the Runtime node's: `application/runtime.DeviceTokenService` issues, rotates and revokes the one token a Device has, resolves `Authorization: Bearer gsc_dev_…` to its Device and Project (hash looked up in `runtime.device_tokens`, result cached in memory until the token changes), and holds no secret: the token is stored as its SHA-256 hash and its fingerprint and shown once.

### Infrastructure Layer (`internal/server/infrastructure/`)

- **PostgreSQL repositories** implement domain repository interfaces using `database/sql` + `lib/pq`.
- **Migrations** live in `internal/server/infrastructure/postgres/migrations/` and are managed with [Goose](https://github.com/pressly/goose): one database, three schemas, one chain each (ADR 0011) — `public/` (the server's: organizations, projects with their deleted mark, outbox, policy_jobs; goose table `goose_db_version`; `20261105000001_drop_project_discard` removed `projects.discarding`), `engineering/` (Libraries, WidgetTypes, Library Releases with `released_widget_types`, Library Pins, Scenes, Widgets, Tag definitions, Devices, Environments with the parameters of a pair, and the Drafts with their DraftChanges; `engineering.goose_db_version`; `20261105000001_drafts` added `engineering.drafts` and `engineering.draft_changes`, dropped `engineering.edit_locks` and the per-entity `version` columns, ADR 0016, ADR 0017; `20261027000000_libraries` put the WidgetTypes into the Library "Main", `20261101000000_devices_and_environments` added the Devices, the Environments, `tags.device_id` and the index of the pairs by Device, `20261103000000_project_files` added the copy of the last ProjectFile a Deploy sent and `20261104000000_drop_project_files` removed it with the copy's readers, ADR 0014; `20261105000000_edit_locks` added `engineering.edit_locks`, one row per Draft with two nullable foreign keys, ADR 0015), `runtime/` (the copies of the Tag definitions, Devices and Environment Devices — the pairs with foreign keys to both copies, cascading, and the project foreign keys of the copies cascading —, the Tag states apart from them in `runtime.tag_states`, `runtime.device_tokens`, `runtime.tag_state_seq`, the Revisions in `runtime.revisions`; `runtime.goose_db_version`; `20261101000000_tag_states_and_devices` split the state from the definition, `20261103000001_revisions` added the Revisions, `20261103000002_deferrable_environment_names` made the uniqueness of an Environment's name checkable at a commit, `20261104000000_revision_cause` added a Revision's `cause` and the `rolled_back_from` a rollback copies, applied in that order; the public migration `20261026000000` moved the tables and marks the baseline of the two others. Go code applies them with `migrations.Up` (embedded); the debug `docker-compose.yaml` applies the three chains (a goose service each: the `goose-docker` image runs `goose -dir=/migrations up` whatever the command, the table goes in `GOOSE_EXTRA_ARGS`) and the seed data (`test_data/`, a chain on `goose_db_version`) on `make db_up`. Queries name the role schemas (`engineering.tags`, `runtime.tags`); `public` stays unqualified.
- **Repositories of a Project's content** (`NewProjectScopes`, `NewTagRepositoryPostgres(db, project)`, `NewSceneRepositoryPostgres(db, project)`, `NewLibraryPinRepositoryPostgres(db, project)`, `NewDeviceRepositoryPostgres(db, project)`, `NewEnvironmentRepositoryPostgres(db, project)`, `NewEnvironmentDeviceRepositoryPostgres(db, project)`) carry the Project in their `aggregateStore` (the pair's is keyed by `(environment_id, device_id)`, with no ID of its own); those of a Library's Draft (`NewWidgetTypeRepositories`, `NewWidgetTypeRepositoryPostgres(db, library)`) the Library, and lock its row before every write (`aggregateStore.lock`); `FindSoleOrganization` reads the one Organization the migrations create (`main.go` fails to start without exactly one).
- **The Runtime node's repository** (`postgres/runtime.NewTagStateRepositoryPostgres`) over `runtime.tags` (the copy of a Tag definition, its supplying Device in `device_id`) and `runtime.tag_states` (its state), joined on every read; a write checks the writer against `device_id` in its UPDATE: reads `public.projects`, never `engineering.*`. `postgres/runtime.NewDeviceTokenRepositoryPostgres` keeps `runtime.device_tokens` (the token's hash, fingerprint, Device and that Device's Project) for the Runtime node's `DeviceTokenService`. `postgres/runtime.NewRevisionRepositoryPostgres` keeps `runtime.revisions` (the numbered ProjectFile: what the Project runs, with the `cause` of a deploy or a rollback and the `rolled_back_from` a rollback copied) and, in the transaction that numbers one, rebuilds the copies of the Tag definitions, the Devices, the Environments and their pairs from the document by diff, so the Tag states and the Device Tokens survive; `CreateRevision` and `Rollback` share the advisory lock, the numbering and that diff, and `FindAll`/`FindByNumber` refuse a Project that is not live. The Engineering node keeps no copy (ADR 0014).
- **The Drafts** (`postgres/repositories/draft_repository_postgres.go`): `engineering.drafts` and `engineering.draft_changes` — `Read`, `Update` (locks the Draft's row, checks the EditLock and the stated Version, runs a `draft.Session`, writes the touched rows from its snapshots, one DraftChange and the outbox, raises the Version and moves the stack), `Replace` (Discard: same, clearing the stack, no DraftChange), `Purge` (the cleanup of a deleted Project), `ClearStack` (after Deploy/Release), and the EditLock store (`Find`, `Create`, `Save` on `lock_version`, `FindAllHeld`, `Touch`); the loader reads only the entities an operation touches (`draft_repository_snapshots.go`), and the snapshots are encoded by `postgres/draftchange`.

- **Outbox** (`postgres/outbox/`): `Append` (called by the repositories' shared `aggregateStore` template), the internal event codec (not a contract; the payload carries the `project_id` of a content event) and the `Dispatcher` (`Run`, `Notify` — wired to repositories via `repositories.NotifyOnCommit`, `DrainOnce` for deterministic tests; `WithPolicyRunner` makes it enqueue policy jobs) and the `PolicyRunner` (`Run` — woken by the dispatcher and by the timer of the next due job, `RunDueOnce` for deterministic tests); the policy registry is assembled in `main.go`. One dispatcher and one runner per database.

### Interface Layer (`internal/server/interface/`)

**REST API** (`internal/server/interface/restapi/`, port `:9090`):
- Uses stdlib `http.ServeMux` with method-prefixed patterns (Go 1.22+).
- All endpoints live under `apiv0.PathPrefix` (`/api/v0`); every response carries the `GrowSCADA-Schema-Version` header.
- The content of a Project (Tags, Scenes, Widgets, Library Pins with their `impact` preview, the pinned WidgetTypes, its event stream) lives under `apiv0.ProjectPath(id)`; `version` in a content DTO and every write of the content is the Version of the Project's Draft (ADR 0016), create bodies state it, and a DELETE states it in `?version=` (a stale one is 409 `draft.ErrDraftConflict`) (`/api/v0/projects/{project_id}/…`); Projects (`/projects`, `?name=`) and Libraries (`/libraries`, `?name=`) are the server's, a Library's Draft (`widget-types`), `releases` and `discard` under `apiv0.LibraryPath(id)`. A 409 of a pin's switch names the Widgets in `orphaned_widgets`.
- The Draft's EditLock (ADR 0015) is under the Project's path too: `GET …/edit-lock` (200 free or held), `POST …/edit-lock` (Check out: 201 when it was free, 200 for the same client, 409 with the holder), `DELETE …/edit-lock` (Check in: 200, a free lock is 200 too, another client's 409) and `POST …/edit-lock/takeover` (200; a stale `version` or a free lock is 409). The same four are under a Library's path (`…/libraries/{id}/edit-lock`), a Check out there refused `library_discarding` while its Discard runs. The client key travels in `X-GrowSCADA-Client` (allowed by CORS) and is put in the request's context by a middleware; a gated write without it is 400. The four types are published as `edit_lock_*`, carry `edit_lock` in `Event` and reach the Project's stream (a Library's lock has the server's, naming the Library in `id` and `library_id`); the 409 of a gated write carries it in `Problem`.
- Deployment is under the Project's path: `GET …/revision` (the Revision it runs, 404 until the first Deploy), `GET …/tag-states` (the snapshot of its Tag states, 404 likewise), `GET …/deploy/preview` (`current_revision`, `unchanged`, `dropped_widgets`), `POST …/deploy` (201 `{number, deployed_at}`, 409 `nothing_to_deploy`) and `POST …/discard` (200 `{version}` — the Draft reset to the Revision in one transaction, 409 without a Revision). `GET …/draft` answers the state of the Draft (`{version}`), the Version every content write states. The Revision and the Tag states are the Runtime node's; Deploy and Discard the Engineering node's. The Runtime node's history is `GET …/revisions` (200 `{revisions: [{number, deployed_at, cause, rolled_back_from?}]}`, newest first, `[]` for a Project never deployed, 404 for a deleted one), `GET …/revisions/{n}` (the running Revision's shape plus `cause`/`rolled_back_from` and `content`; 404 for a number the Project lacks, 400 for a non-numeric one) and `POST …/revisions/{n}/rollback` (201 with the new Revision's summary, 404, 409 `nothing_to_rollback` or `project_discarding`) — no Draft is read (ADR 0014).
- CORS middleware applied globally.
- Errors follow RFC 7807 Problem Details (`problems.go`).
- Each resource has its own handler file (`tags.go`, `widgets.go`, etc.). JSON shapes are `contract/api/v0`; the conversions to/from `appdto` are in `dto_mapping.go`.
- Request bodies are decoded strictly with `decodeRequest` (unknown field → 400); clients read responses tolerantly.
- `NewRouter` takes the services of both roles (`restapi.Services`): `PATCH …/tags/{id}/value` and the Revision/Tag-states reads go to the Runtime node (`TagStateService`, `RevisionService`) — a Tag write has no `version` and answers `seq` —, everything else, Deploy and Discard included, to the Engineering node's; `newAPIEvent` translates `tagstate.TagUpdatedEvent` (the Tag's full state, `seq` included) and `revision_created` (the Project's ID, the Revision's number and time).
- Devices, Environments and the pairs `(Environment, Device)` are content of a Project (`…/devices`, `…/environments`, `…/environments/{id}/devices/{device_id}`); the token routes (`…/devices/{device_id}/token`, `…/token/rotate`) are the Runtime node's and answer 404 on a server without it; issuing and rotating check that the Device exists in the Draft, while reading and revoking do not — an orphaned token stays visible and revocable (task 66). A Device Gateway authenticates with `Authorization: Bearer gsc_dev_…`: `GET /api/v0/device/config?environment=<id>` answers its configuration and `GET /api/v0/device/events` its Project's configuration events; `PATCH …/tags/{id}/value` with a token writes as that Device (a Tag it does not supply is 403, an unknown or revoked token 401, a token of another Project 403). A bearer without the `gsc_dev_` prefix is not a Device Token (user authentication — task 50).

SSE streams name their client with `?client=` (`apiv0.EventsQueryClient`): while the stream is open the handler counts the key as present (`Attach`/`Detach`), so the client's EditLock survives without requests. The parameter is optional — a Device Gateway, History and any reader that edits nothing go without it.

**EventBus** (`internal/server/interface/eventbus/`): in-process bus feeding the SSE streams: domain events handed over by the outbox dispatcher, the Runtime node's `tag_updated` and `revision_created` (published by `TagStateService` and `RevisionService` themselves through `AsyncPublisher`, ADR 0011), plus delivery's own events (`client_connected`/`client_disconnected`, `system_ready`). The `eventHub` routes a `ContentEvent` to the clients following its Project and everything else — Projects, Libraries with their WidgetTypes and releases, connections — to those following the server's stream: `GET /api/v0/events` is the server's, joined by one Project's with `?project=<id>` (a Project not found adds nothing), `GET /api/v0/projects/{id}/events` one Project's alone. A Device Gateway's `GET /api/v0/device/events` follows its Project's stream with the configuration events alone (`device_*`, `environment_*`, `environment_device_*`), plus the events of that Project itself, which go to the server's stream. `event_bus_mqtt.go` is a stub.

**PWA UI** (`internal/server/interface/ui/`, port `:8080`):
- Built with [go-app v11](https://go-app.dev/) — compiled to WebAssembly, served by the same binary.
- Four top-level modes rendered by `root/root.go`:
  - **Library** — the Libraries (create, rename, delete), the WidgetTypes of the selected one's Draft (HTML template + JS script, live sandboxed preview via `<iframe srcdoc>`), its release state, Release (with a note), Discard (read-only while under way) and its releases, viewed read-only. The selected Library's Draft has the same EditLock row as a Project's (ADR 0015): `Check out` (the name comes from the Project mode's `editlock:name`), `Editing as <name> — Check in`, `<name> is editing since HH:MM — Take over`. Without the lock its WidgetTypes, Release and Discard are read-only (the buttons are shown disabled with the hint), while the Library list, its creation, rename and deletion, and the releases stay available; losing the lock is a toast and read-only, and a switch of Library sends a Check in to the one left. The state is reread on `EventResync` and on the `edit_lock_*` events of that Library (the server's stream, which the Bar holds always).
  - **Project** — the Library Pins (pin, switch after a preview, unpin), configure Scenes and place Widget instances on them from the WidgetTypes of the pinned releases, grouped by Library. Its header has the EditLock row (ADR 0015): `Check out` (the name is asked once and kept in LocalStorage `editlock:name`), `Editing as <name> — Check in`, `<name> is editing since HH:MM — Take over` (a confirmation, then the POST with the version seen). The tab's key is a UUID in sessionStorage (`editlock:client`), created by `root` and given to the client and to the Bar (`?client=`). Without the lock the Draft is read-only — Scenes, Widgets, Tags, pins, Deploy and Discard are refused with a hint; the Revisions dialog and Rollback stay available. Losing the lock (a takeover, its expiry) is a toast and read-only; switching the Project or leaving the mode sends a Check in to the one left. The header also shows the Revision the Project runs (or "Not deployed"), whether the Draft has unreleased changes, Deploy (a dialog over the preview: current Revision, dropped Widgets, then confirmation), Discard (confirmation; the server's refusal without a Revision is a toast) and Revisions (always available: the list newest first with each number, time and cause — "Deployed" or "Rolled back from #N" — the running one marked and not rollable; selecting a row loads its document and shows the counts of its Scenes, Widgets, Tags, Devices and Environments, and Roll back confirms, then closes the dialog and reloads the preview and the list; refusals are toasts).
  - **Operation** — runtime operator view: the Scenes of the running Revision only, with live Tag values. It reads `GET …/revision` and `GET …/tag-states` and reloads on `revision_created` and resync; Draft and Library events change nothing. A Project without a Revision shows "Project is not deployed".
  - **History** — event log viewer.
- Project, Operation and History each have their own project switcher at their top (`ui/projects.Switcher`, one per mode: list, create, rename, delete with confirmation; the selection in LocalStorage under `<mode>:projectID`, the first Project when it is gone — left at once on `project_deleted` or `uiutil.ActionProjectGone`, which a 404 of a Project's Scene or Tag list (`uiutil.LoadOfProject`) fires instead of a toast). Project and Operation show their selected Project (`SetProject`, its `*apiclient.ProjectClient`; loads are keyed by it, so a response of another Project is dropped); Library has no switcher. The event Bar keeps one SSE connection, `/events?project=<id>`, following the Project of the mode shown (Library leaves it as it is) and reopened when that changes; on open it fires `EventResync`, then `EventProjectResync` while it follows a Project. Content events and `EventProjectResync` carry the Project of their connection in the action tag `eventlog.ProjectTag`; a mode drops those of another Project (`eventlog.ForProject`), so a hidden mode showing another Project is stale until shown again. A mode whose Project the Bar already follows is resynced at once (`Bar.ResyncProject`). On a switch while connected the old connection stays (delivering only the server's events) until the new one opens, which then fires `EventProjectResync` alone: the server's data is not reloaded.
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
| **WidgetType** | Reusable visual component definition: HTML + JS, of one Library's Draft |
| **Library** | Reusable collection of WidgetTypes of an Organization, outside every Project |
| **Library Release** | Immutable, numbered snapshot of a Library's Draft; never removed |
| **Library Pin** | The Library Release of one Library a Project's Draft uses |
| **Widget** | Placed instance of a WidgetType on a Scene, bound to Tag IDs |
| **Device** | Configuration of a piece of equipment: its protocol and how the values are read (the Draft's, the Engineering node's) |
| **Environment** | A named set of one Project's Devices with their connection parameters; a Deploy does not choose one |
| **Device Token** | The secret by which a Device Gateway acts for one Device; issued on the Runtime node, stored as a hash, shown once |
| **Scene** | A mnemonic/synoptic display canvas |
| **Quality** | Data reliability: `Bad`, `Uncertain`, `Good` |
| **Project** | Unit of isolation owning Tags and Scenes; owned by an Organization |
| **Organization** | The tenant owning Projects (one per server for now) |

## API Collections

Manual/exploratory API tests are maintained as [Bruno](https://www.usebruno.com/) collections in `tests/api/bruno_collections/`: `growscada_server/` for the server REST API (the `localhost` environment points to `http://localhost:9090`) and `device_simulator/` for the simulator's control API (`localhost:9191`, `TAG_NAME` is the seed tag `max_connections` driven by `tests/device_simulator/seed_dashboard.yaml`). The main automated tests are Go tests; two checks keep the collections from falling behind the API:

- **Static check** (`TestBrunoCollection` in `restapi` and `devicesim`, part of `make test`, no Docker): via `internal/brunotest`, every request must hit a route of the `routes()` table, every route must be covered by a request, and every body must decode strictly into the type that `requestBodies` (in the test) maps to its route.
- **Run** (`make bruno_test`, outside `make test`: GitHub CI and the end of every task, rule 9): both collections against the application in containers on a fresh seeded database, in `seq` order. Each request asserts its status, the `GrowSCADA-Schema-Version` header (server), `application/problem+json` on errors and one or two key fields on reads.

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

1. **Work in a separate branch.** Never make changes directly on the `path` branch. If the current branch is `path`, stop and ask the user to create or switch to a feature branch before proceeding.

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

## Agent skills

### Issue tracker

Задачи ведутся markdown-файлами в `todo/` (`backlog/` → `done/`). See `docs/agents/issue-tracker.md`.

### Triage labels

Пять канонических ролей, строки совпадают с именами (`Status:` в файле задачи). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `GLOSSARY.md` + `docs/adr/` в корне. See `docs/agents/domain.md`.
