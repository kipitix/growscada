# Journal: запись операционных событий
Status: ready-for-agent
Blocked by: 32, 35
Parts: 42_1_journal_storage, 42_2_journal_read_stream

Слой D. Термины — `GLOSSARY.md` (Journal, Journal position); архитектура — ADR 0002, ADR 0019.

Источник истины — этот файл. Работы — `42_1_journal_storage` и `42_2_journal_read_stream`, в этом порядке: каждая самодостаточна и проверяема, вторая блокирована первой, у второй `Closes parent: yes`.

## Модель

**Journal** — append-only запись операционного состояния Project на Runtime-ноде: таблица `runtime.journal`, одна последовательность `seq` на сервер, строка принадлежит одному Project (`project_id`, FK на `public.projects` с `ON DELETE CASCADE`). `seq` — позиция: тотальный порядок всех записей, по которому догоняет Player (43), фиксируется Checkpoint (44) и читается History (45).

**Что записывается — только два вида изменений** операционного состояния:

- `tag_state` — изменились value или Quality тега; payload `{tag_id, value, quality}`. Имени, TagType и версии в записи нет: это конфигурация, её адрес — Revision.
- `revision` — появилась Revision: Deploy, DeployFile (cause `file`) или Rollback (cause `rollback`); payload `{number, cause, rolled_back_from?}`, **без документа** (документ неизменяемо лежит в `runtime.revisions`, ADR 0014).

Конфигурация (Tag definition, Device, Environment, Scene, Widget) отдельной записи не даёт: её меняет только Deploy, и номер Revision в записи — адрес полного нового состояния конфигурации. Правки Draft не пишутся. Runtime-сущности задачи 48 добавят свои типы, когда появятся.

**Запись — только изменение.** Повтор того же значения с тем же Quality записи не даёт: Journal — журнал изменений, а не журнал вызовов API. Факт записи виден в `runtime.tag_states.last_write_at` и в поле `written_at` API; `seq` состояния при неизменной записи не двигается, и `PATCH …/tags/{id}/value` отвечает прежним `seq`, не публикуя `tag_updated`. Стабильный сигнал не даёт ~86 млн строк в сутки на 1000 тегов @ 1 Гц, а «жив ли Device» отвечает `written_at`, а не поток дублей.

**Транзакция.** Запись Journal пишется в той же транзакции, что и изменение состояния, — как строка outbox у агрегата (ADR 0008), но в Journal:

- tag state: одна транзакция над `runtime.tag_states`; строка тега блокируется (`FOR UPDATE`), под блокировкой сравниваются value и Quality и решается, писать ли запись, затем UPDATE состояния (`seq` = новая позиция, `last_write_at = now()`). Блокировка строки сериализует двух писателей одного тега, поэтому их позиции идут в порядке блокировки.
- Revision: INSERT записи внутри транзакции `CreateRevision`/`Rollback`, уже держащей advisory-lock проекта; `ts` записи = `deployed_at`.

Outbox не участвует, второй писатель не появляется, политика не нужна: агрегат один. Запись Journal не трогает `runtime.tag_states` иначе, чем вместе с самим изменением, и никогда не пишется при `UpsertDefinitions`, `Delete`, `RemoveTagStates`, `RemoveProjectTagStates` или в `refreshCopies` Rollback: это конфигурация и уборка, адрес им — Revision или исчезнувший Project.

**`seq` состояния.** `runtime.tag_states.seq` — позиция Journal последнего изменения; отдельная последовательность `runtime.tag_state_seq` удаляется, её роль берёт Journal (ADR 0011). Отсюда `seq` в `TagResponse`, `TagStateResponse` и `tag_updated` — позиция Journal, а не отдельный счётчик.

**События.** `tag_updated` публикуется после коммита и только при изменении; его `Tag.Seq` — позиция Journal. `revision_created` не меняется. Существующий поток `/events` не меняется: оба события остаются, их судьбу решит 43, когда Operation перейдёт на Player.

**Чтение и живой поток (Runtime-нода).**

- `GET /api/v0/projects/{id}/journal?from=<seq>&limit=<n>` — записи проекта по возрастанию `seq`; `from` исключающая (по умолчанию 0), `limit` по умолчанию 100 и не больше 1000; 200 `{entries: []}`; проект не жив — 404 `project_not_found`; нечисловой `from`/`limit` — 400.
- `GET /api/v0/projects/{id}/journal/events` — SSE: каждое сообщение `id: <seq>` и `data: <Entry>` (документ `contract/record/v0` как есть). Старт — `Last-Event-ID` (заголовок SSE) или `?from=` (параметр важнее), иначе с текущей позиции проекта (только новое). Сначала догон из таблицы, потом живое: после каждого `tag_updated`/`revision_created` своего проекта поток дочитывает `seq > last`. Доставка at-most-once (ADR 0019); догон по `Last-Event-ID` точен, потому что читает таблицу.

**Подписчики.** `runtime.journal_cursors(subscriber text PRIMARY KEY, last_seq bigint NOT NULL DEFAULT 0)` — позиция серверного подписчика (Alert — 54, Computation — 56). Заводится здесь, пишут её подписчики; они Runtime-ноды, потому что Journal её (ADR 0011, ADR 0008).

**Удаление и хранение.** Journal удаляется вместе с Project (`ON DELETE CASCADE`), как Tag states, Revisions и Device Tokens: Project — единица владения операционной записью. Архивирование периода — PlaybackFile (46) по запросу, не автоматика; retention, партиционирование и time-series store остаются отложенными (ADR 0002).

## Контракт

`contract/record/v0` (0.1 → 0.2): `Entry` (встраивает `Header`, как `ProjectFile`; поля `seq int64`, `timestamp` RFC 3339, `project_id uuid.UUID`, `type EntryType`, `tag_state *TagState`, `revision *Revision` — оба `omitempty`), `EntryType` (`tag_state`, `revision`), payload-типы `TagState{tag_id, value, quality}` и `Revision{number, cause, rolled_back_from?}`, `RevisionCause` (`deploy`, `rollback`, `file`) — зеркало перечисления API, потому что запись хранится вечно и не импортирует контракт API (`devicelink.Quality` — тот же приём, ADR 0004). `aggregate_id` нет: идентификатор изменённой сущности в payload. `schema_version` пишется в каждую строку БД.

`contract/api/v0` (MINOR, двумя шагами): `WrittenAt *string` (`written_at`, `omitempty`) в `TagResponse` и `TagStateResponse`, потому что «момент последней записи» — операторский факт, а не только внутренняя колонка (шаг 1: 0.20 → 0.21); `JournalResponse{entries []recordv0.Entry}` для чтения диапазона, встраивание чужого контракта — как `RevisionResponse.Content` с `projectv0.ProjectFile` (шаг 2: 0.21 → 0.22). `make schemas` после каждого шага.

## За рамками

- Checkpoint (44) и его таблица/контракт; PlaybackFile (46); Player и Operation (43); History (45).
- Судьба `tag_updated`/`revision_created` в `/events` и переход UI на Journal (43).
- Runtime-сущности задачи 48 и их типы записей; Adopt (48).
- Retention, партиционирование, Timescale, автоматическое архивирование перед удалением Project.
- Права доступа к Journal (50).
- Начальное состояние Player «на позицию» как одна согласованная модель чтения — 43 (при нужде 58).

## Comments

- 2026-09-26: поглотила задачу «Запись и воспроизведение событий» (бывшая 24): здесь запись, воспроизведение — задачи 44–46.
- 2026-10-04: бывший тикет 02 задачи 36. Решения: Q4 (состояние в таблицах + Journal с полным новым состоянием в каждой записи), Q17 (один Journal для всего, включая значения тегов). Запись Deploy/Rollback перенесена сюда, чтобы Deploy не зависел от Journal.
- 2026-10-09 (grilling задачи 21, ADR 0008): серверные реакции на каждое операционное событие (уведомление по Alert, счётчики Computation) читают Journal со своей позицией (`journal_cursors(subscriber, last_seq)`), а не через `policy_jobs`: задания политик сливаются и годятся только для сведения состояния к текущему.
- 2026-10-25 (grilling задачи 31, ADR 0006): удаление Project уборщиком удалит и его операционную запись (Journal, Checkpoints, Revisions). Решить, должна ли она сохраняться (требования к хранению записей SCADA) — например, отказ в удалении или архивирование в PlaybackFile.
- 2026-11-06 (grilling; ADR 0019). Решения:
  - **Что пишется.** Только `tag_state` и `revision` (Deploy/DeployFile/Rollback). Изменения конфигурации отдельными записями не идут: их адрес — номер Revision. Runtime-сущности задачи 48 добавят типы позже.
  - **Payload.** `tag_state` — `{tag_id, value, quality}`, без name/type/version. `revision` — `{number, cause, rolled_back_from?}`, без документа.
  - **Поверхность.** Отдельная пара Runtime-ноды: чтение диапазона и SSE-поток; `/events` не меняется.
  - **Когда запись.** Только при изменении value или Quality; повтор даёт только `last_write_at`/`written_at` и прежний `seq`. Deadband отвергнут.
  - **Удаление Project.** Journal удаляется вместе с Project; архивирование/retention — не здесь.
  - **Курсоры.** `journal_cursors` заводятся сразу, пишут их 54/56.
  - **Типы.** `tag_state` / `revision`, не имена живых событий: запись не должна меняться при отставке `tag_updated` в 43.
  - **Конверт.** Без `aggregate_id`: ID сущности в payload.
  - **Гарантия потока.** At-most-once + точный догон по `Last-Event-ID` из таблицы; gap-free вариант с `tx_id` отвергнут как цена без нужды.
  - **`written_at`.** Колонка `last_write_at` плюс поле в API (`TagResponse`, `TagStateResponse`, а значит и в `tag_updated`).
  - **Границы.** Без PWA: Project-режим живёт на `tag_updated`, Operation переводит 43.
  - **Разбиение.** Две части: `42_1_journal_storage`, `42_2_journal_read_stream`.
- 2026-11-06 (аудит DDD по правилу 8). Расхождений нет: Journal — не агрегат, а append-only запись рядом с состоянием (тот же приём, что строка outbox в ADR 0008), поэтому «одна транзакция на агрегат» не нарушена; агрегатных инвариантов Journal не несёт и версии не имеет. Обе транзакции записи — внутри Runtime-ноды (`runtime.tag_states`, `runtime.revisions` + `runtime.journal`), Engineering-нода в 42 не участвует, `application.RuntimeNode` новых методов не получает; чтение и поток — `application/runtime.JournalService` + `postgres/runtime`, REST-слой трогает только сервисы Runtime-ноды (ADR 0011), `roles_test` остаётся зелёным. Политик не появляется. Курсоры — позиции подписчиков Runtime-ноды; если подписчику понадобится другая роль, он получит Journal через порт, а позицию — в своей схеме. Термины: Journal и Journal position внесены в `GLOSSARY.md`, строка Version исправлена; решения записаны в ADR 0019. Записи журнала не являются событиями outbox и не публикуются в EventBus — живой поток читает таблицу.
