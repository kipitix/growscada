# Journal: хранение и запись
Status: ready-for-agent
Blocked by: 32, 35
Parent: 42_journal
Part: 1/2

Часть 1 задачи 42. Источник истины — `todo/backlog/42_journal.md` (гриллинг 2026-11-06, ADR 0019): это **спецификация**, а не отдельная работа. Здесь — контракт записи, миграция и запись Journal в тех же транзакциях, что и изменения состояния. Чтения диапазона и SSE-потока здесь нет — это 42_2.

## Что сделать

### Контракт `contract/record/v0` (MINOR 0.1 → 0.2)

- `Header` остаётся корнем каждого документа формата.
- `Entry` встраивает `Header` (как `ProjectFile` в `contract/project/v0`) и добавляет: `Seq int64` (`seq`), `Timestamp` (RFC 3339, `timestamp`), `ProjectID uuid.UUID` (`project_id`), `Type EntryType` (`type`), `TagState *TagState` (`tag_state`, `omitempty`), `Revision *Revision` (`revision`, `omitempty`). `aggregate_id` нет — идентификатор изменённой сущности в payload.
- `EntryType` — `tag_state`, `revision`; функция `EntryTypes() []string` по образцу `apiv0.EventTypes`, чтобы набор типов был явным.
- `TagState{TagID uuid.UUID, Value string, Quality string}`; `Revision{Number int, Cause RevisionCause, RolledBackFrom *int}`; `RevisionCause` — `deploy`, `rollback`, `file`. Перечисление своё, а не `apiv0.RevisionCause`: запись хранится вечно и не импортирует контракт API (тот же приём, что `devicelink.Quality`, ADR 0004).
- `SchemaVersion()` → `{0, 2}`; в `schema_test.go` `Roots: []any{Entry{}, Header{}}`; `make schemas` (старая версия уходит в `frozen.sha256`).

### Миграция `runtime`: `20261106000000_journal.sql`

- `CREATE SEQUENCE runtime.journal_seq` (явное имя, не `BIGSERIAL`-овский автогенерат) и
  `runtime.journal (seq BIGINT PRIMARY KEY DEFAULT nextval('runtime.journal_seq'), project_id UUID NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE, ts TIMESTAMPTZ NOT NULL, type TEXT NOT NULL CHECK (type IN ('tag_state','revision')), payload JSONB NOT NULL, schema_version TEXT NOT NULL)`; индекс `(project_id, seq)`; таблица append-only (никаких UPDATE/DELETE в коде, кроме каскада с Project).
- **Монотонность при переходе:** до удаления старой последовательности `setval('runtime.journal_seq', COALESCE(MAX(seq), 0) + 1, false)` по `runtime.tag_states`, чтобы `seq` нового изменения был больше любого уже выданного `tag_states.seq` (иначе клиент, держащий старое состояние, отбросил бы новое как устаревшее).
- `DROP SEQUENCE runtime.tag_state_seq`.
- `ALTER TABLE runtime.tag_states ADD COLUMN last_write_at TIMESTAMPTZ` (NULL — состояние Initial, писал только Deploy).
- `runtime.journal_cursors (subscriber TEXT PRIMARY KEY, last_seq BIGINT NOT NULL DEFAULT 0)` — пустая, пишут подписчики 54/56.
- Down: восстановить `tag_state_seq` (значение — из `setval`/max), удалить колонку, таблицы и последовательность.

### Кодек записи

- `postgres/runtime/journal_codec.go` — внутренний кодек `recordv0.Entry` ↔ строки `runtime.journal` (payload по `type`, `schema_version` строкой), по образцу `postgres/outbox/codec.go`; нужен и записи (здесь), и чтению (42_2). Неизвестный `type` при чтении — ошибка, а не паника.

### Запись `tag_state`

- `tagstate.Repository.Write(ctx, state, supplier)` получает третий результат `changed bool`. Реализация — одна транзакция:
  1. `SELECT value, quality FROM runtime.tag_states s JOIN runtime.tags t ON t.id = s.tag_id WHERE s.tag_id = $1 AND t.project_id = $2 AND t.type = $3 AND t.device_id IS NOT DISTINCT FROM $4 FOR UPDATE OF s` — блокировка строки тега; нет строки — прежний `writeMissReason` (`tag.ErrTagNotFound` / `tag.ErrTagNotSuppliedByDevice`), ничего не пишется.
  2. Если value или Quality отличаются — `INSERT INTO runtime.journal (project_id, ts, type, payload, schema_version) VALUES (…, 'tag_state', …) RETURNING seq`, затем `UPDATE runtime.tag_states SET value = $…, quality = $…, seq = <новый seq>, last_write_at = now()`.
  3. Если не отличаются — только `UPDATE runtime.tag_states SET last_write_at = now()`: `seq` не двигается, записи нет.
  Коммит; вернуть сохранённое состояние (с `WrittenAt`) и `changed`.
- `tagstate.TagState` получает `WrittenAt() *time.Time` и участие в `Reconstitute`/`Initial`; Initial — `nil`.
- `application/runtime.TagStateService.SetTagValue`: публикует `tag_updated` только при `changed`; ответ PATCH несёт `seq` состояния (новый или прежний) и `written_at`.
- `application/runtime.TagState` (DTO) получает `WrittenAt *time.Time`; `FindTagStates` заполняет его.
- Тестовые фейки `tagstate.Repository` и `TagStateService` обновить под новые сигнатуры.

### Запись `revision`

- В `postgres/runtime/revision_repository_postgres.go` внутри транзакции `CreateRevision` (Deploy и DeployFile — одна ветка с `cause`) и `Rollback`, после вставки строки Revision: `INSERT INTO runtime.journal` с `type = 'revision'`, `ts = deployed_at` **той же Revision**, payload `{number, cause, rolled_back_from}`. Никаких записей в `UpsertDefinitions`/`Delete`/`refreshCopies`/`RemoveProjectTagStates`: это конфигурация и уборка.
- `RevisionService` не меняется: `revision_created` публикуется как раньше.

### Контракт API (MINOR) и читатели

- `apiv0.TagResponse` и `apiv0.TagStateResponse` получают `WrittenAt *string` (`written_at`, `omitempty`, RFC 3339), `null`/отсутствие — состояния Initial.
- `restapi/dto_mapping.go`: `newTagResponse`/`newTagStateResponse` переносят `WrittenAt`.
- `apiclient` типы берёт из `apiv0` — правок нет; PWA не трогаем (43 начнёт показывать `written_at`).

### Тесты

- Интеграционные (`repositories`/`runtime`, testcontainers): изменение даёт ровно одну запись `tag_state` с `seq` = `tag_states.seq` и payload значения/Quality; повтор без изменения не даёт записи, не двигает `seq`, двигает `last_write_at`; чужой поставщик — ни записи, ни изменения; две конкурентные записи одного тега сериализуются и получают `seq` по порядку блокировки; Deploy, DeployFile и Rollback дают запись `revision` в своей транзакции (откат транзакции не оставляет записи); каскад удаления Project уносит Journal и курсоры не трогает.
- Миграция: `setval` не даёт `seq` уменьшиться относительно прежних `tag_states.seq`.
- Сервис: `tag_updated` публикуется только при изменении; неизменная запись отвечает прежним `seq`; `written_at` доходит до DTO.
- Контракт: `TestSchemaIsCommitted` после `make schemas`; `make schemas` не переписывает старые версии.
- Bruno: `written_at` в ответах тегов (`get_tags`, `patch_tag_value`, `get_tag_states`); новых маршрутов здесь нет.
- `roles_test` остаётся зелёным: SQL записи — только `runtime.*` и `public.projects`.

### Документация

- `docs/adr/0019-journal-records-changes-not-writes.md` (уже записан), `GLOSSARY.md` (Journal, Journal position, Version — уже).
- `AGENTS.md`: в абзаце о Runtime-репозитории — `seq` из `runtime.journal_seq` вместо `runtime.tag_state_seq`, `last_write_at`, запись `tag_state`/`revision` в той же транзакции; в перечне цепочки `runtime` — миграция `journal`.
- `CHANGELOG.md`.

## За рамками

- Чтение диапазона, SSE-поток, `apiclient.Journal`, Bruno-запросы Journal — 42_2.
- Checkpoint (44), Player/Operation (43), History (45), PlaybackFile (46), Alert/Computation (54/56).
- Retention, партиционирование, архивирование перед удалением Project.
