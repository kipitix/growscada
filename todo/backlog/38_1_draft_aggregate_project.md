# Draft-агрегат и DraftChange: Draft проекта

Status: ready-for-agent
Blocked by: 35, 37
Parent: 38_draftchange_undo_redo
Part: 1/4

Часть 1 задачи 38. Источник истины — `todo/backlog/38_draftchange_undo_redo.md`, решения — ADR 0016 и ADR 0017. Здесь — фундамент (схема, домен Draft, DraftChange, слияние EditLock) и весь Draft проекта.

**За рамками:** Draft Library (38_2), маршруты undo/redo (38_3), кнопки и горячие клавиши UI (38_4). Записи Draft проекта после этой части уже пишут DraftChange в лог, но откатить их ещё нельзя.

## Схема

Миграции в двух цепочках, public первой (AGENTS.md):

- `public/20261105000001_drop_project_discard.sql`: удалить `projects.discarding`; политика и события Discard проекта уходят (см. ниже).
- `engineering/20261105000001_drafts.sql`:
  - `engineering.drafts` — `id uuid PK`, `project_id uuid UNIQUE REFERENCES projects(id) ON DELETE CASCADE`, `library_id uuid UNIQUE REFERENCES engineering.libraries(id) ON DELETE CASCADE`, CHECK «ровно одна область», `version bigint NOT NULL DEFAULT 1`, `lock_version bigint NOT NULL DEFAULT 1`, `holder_key uuid`, `holder_name text`, `held_since timestamptz`, `last_seen timestamptz`, CHECK «держатель целиком или никак», `stack_position bigint NOT NULL DEFAULT 0`, `created_at`, `updated_at`. Backfill по одной строке на каждый существующий Project и Library; новые строки создаются при первом обращении (`INSERT … ON CONFLICT DO NOTHING`).
  - `engineering.draft_changes` — `draft_id uuid REFERENCES engineering.drafts(id) ON DELETE CASCADE`, `seq bigint NOT NULL`, `description text NOT NULL`, `before jsonb NOT NULL`, `after jsonb NOT NULL`, `created_at`, PK `(draft_id, seq)`.
  - Удалить `engineering.edit_locks` (данные замка не переносятся: после миграции все Draft свободны, вкладка берёт замок заново).
  - Удалить `version` у `engineering.tags`, `engineering.scenes`, `engineering.devices`, `engineering.environments`, `engineering.environment_devices`, `engineering.library_pins`. `engineering.widgets` версии не имел.
- `libraries.discarding_to` остаётся до 38_2.

## Домен

- Новый пакет `domain/draft` (инженерная нода, `roles_test`): `Draft` со `Scope` (переиспользовать `editlock.Scope`), `Version`, состоянием замка, `StackPosition`, `CanUndo`/`CanRedo`/описаниями шагов и операциями над содержимым проекта. Сущности `scene`, `tag`, `device`, `librarypin` остаются в своих пакетах, не импортируют `draft` и теряют `Version()`/`CheckVersion`/параметр `expected`; версию проверяет Draft один раз за операцию.
- `DraftChange` — значение: `Seq`, `Description`, `Before`/`After` — списки снимков `{Kind, ID, State}` (полное состояние строки; у созданной/удалённой сущности одна сторона пуста). Кодировка снимков — внутренний кодек `draftchange` с версией формата, как у outbox (ADR 0005), не контракт.
- Операции Draft проекта: `CreateTag`, `DeleteTag` (в той же транзакции снимает привязки во всех Scenes проекта и пишет `widget_updated` для затронутых Scene), `CreateScene`, `UpdateScene`, `DeleteScene` (с Widgets), `AddWidget`, `UpdateWidget`, `RemoveWidget`, `CreateDevice`, `UpdateDevice`, `DeleteDevice`, `CreateEnvironment`, `UpdateEnvironment`, `DeleteEnvironment`, `PutEnvironmentDevice`, `DeleteEnvironmentDevice`, `PutLibraryPin`/switch, `DeleteLibraryPin`. Каждая возвращает новый Draft, события и снимки затронутых строк.
- Инварианты, которые были на блокировках строк, проверяются в транзакции Draft: смена/снятие Pin снимает сломанные привязки (`librarypin.ImpactOf`) в той же операции; добавление/смена Widget проверяет WidgetType в закреплённом релизе (`checkPinned` переезжает в транзакцию Draft); Tag, Device, Environment и пара проверяются на принадлежность проекту. Удаление Library против Pin проекта остаётся вне Draft (ADR 0012).
- Ошибки: `draft.ErrDraftConflict` (устаревшая Version Draft) заменяет `ErrSceneConflict`, `ErrTagConflict`, `ErrDeviceConflict`, `ErrEnvironmentConflict`, `ErrEnvironmentDeviceConflict` в записях содержимого; REST отдаёт 409 с Draft в Problem. `project.ErrProjectDiscarding` и пометка Discard уходят; `library.ErrLibraryDiscarding` — в 38_2.
- EditLock: `domain/editlock` остаётся, но его состояние — поля `Draft`; `Acquire`/`Release`/`TakeOver`/`Expire`/`Seen` применяются к Draft, `lock_version` растёт только при смене держателя, `last_seen` — без версии. `editlock.Repository` заменяется методами Draft-репозитория; presence и реапер в `application.EditLockService` не меняются.

## Инфраструктура

- `draftRepositoryPostgres` (`postgres/repositories/draft_repository_postgres.go`): `Update(ctx, scope, version, fn)` открывает одну транзакцию, читает строку Draft `FOR UPDATE`, проверяет EditLock (ключ из context; `editlock.System` проходит), сверяет `version` (CAS), даёт загрузчику прочитать только нужные сущности, применяет `fn`, пишет затронутые строки, снимки before/after, `draft_changes`, outbox и `stack_position`, коммитит. `Read`/`State` — для чтений без записи.
- Загрузчик (`draft_loader_postgres.go`): Scene с Widgets, Tag, Device, Environment, пара, Pin, закреплённые WidgetTypes и TagTypes — по запросу операции; ничего лишнего.
- `aggregateStore` теряет запись содержимого; `ProjectScopes`/`ProjectScope` остаются для чтений (`Tags()`, `Scenes()`, …), их `Save`/`Delete` уходят. `retryOnRace` остаётся только там, где запись не DraftChange (очистка Project, служебные операции).
- `edit_lock_guard.go` читает `engineering.drafts` (`project_id`/`library_id`); `edit_lock_repository_postgres.go` заменяется Draft-репозиторием. Реапер обновляет `last_seen` строк Draft.
- Outbox: события DraftChange пишутся в транзакции Draft; `NotifyOnCommit` работает как раньше.

## Приложение

- `TagService`, `SceneService`, `DeviceService`, `EnvironmentService`, `LibraryPinService` сохраняют публичные интерфейсы; каждая запись принимает Version Draft, открывает Draft-репозиторий и вызывает операцию Draft. Чтения остаются на существующих репозиториях (по `FindAllInLiveProject`).
- `DiscardService` проекта: читает текущую Revision через `RuntimeNode`, одной транзакцией Draft заменяет содержимое из неё, чистит стек и коммитит; ответ 200. `ProjectContent.openNotDiscarding`, `Project.StartDiscard`/`FinishDiscard`/`IsDiscarding`, `SaveDiscarding`, `NewDiscardProjectPolicy`, событие `project_discard_started` и поле `Discarding` удаляются.
- `NewUnbindDeletedTagPolicy` и `NewWidgetReconciliationPolicy` удаляются из `main.go`; `NewProjectCleanupPolicy` остаётся и пишет как `editlock.System`.
- Deploy: гейт — EditLock (как сейчас); после успешного `RuntimeNode.Deploy` стек чистится отдельной транзакцией, сбой очистки — 500 при уже созданной Revision.

## REST и контракт

- `version` в телах update — Version Draft; create-тела получают `version`, DELETE-маршруты — параметр `?version=`. Устаревшая — 409.
- `Project` теряет `discarding`; `POST …/discard` отвечает 200 состоянием Draft (не 202 `{discarding}`); Problem `project_discarding` уходит.
- `api/v0` — MINOR: `make schemas`; обновить `dto_mapping.go`, `problems.go`, apiclient (create/delete версии), Bruno-коллекцию (create/delete, Discard, убрать `discarding`), `CHANGELOG.md`.
- UI Project-режима — минимальная адаптация под контракт: убрать поле `discarding` и ответ 202 у Discard (диалог и тосты), не добавляя кнопок undo; кнопки и клавиши — в 38_4.
- `AGENTS.md`: Draft как агрегат, `drafts`/`draft_changes`, одна Version, слияние замка, синхронный Discard, удалённые политики.

## Тесты

- Домен: операции Draft, удаление Tag с привязками, смена Pin с чисткой, конфликт Version Draft, снимки before/after.
- Репозиторий: одна транзакция на запись, CAS по `drafts.version`, строки `draft_changes`, гейт замка, отсутствие записи при отказе, backfill и CASCADE.
- Сервисы: каждая операция пишет одну DraftChange, Discard одной транзакцией, Deploy чистит стек после успеха, cleanup как System.
- REST: create/delete с версией, 409 устаревшей, Discard 200, отсутствие `discarding`, гейт замка.
- Bruno: обновлённые тела и порядок; `make bruno_test` в конце задачи.
