# Journal: чтение и живой поток
Status: ready-for-agent
Blocked by: 42_1
Parent: 42_journal
Part: 2/2
Closes parent: yes

Часть 2 задачи 42. Источник истины — `todo/backlog/42_journal.md` (гриллинг 2026-11-06, ADR 0019). Здесь — то, что делает Journal видимым снаружи: чтение диапазона, SSE-поток с `Last-Event-ID`, клиент, Bruno, документация. Запись и контракт записи уже сделаны в 42_1.

## Что сделать

### Прикладной сервис

- `application/runtime/journal_service.go`: `JournalService`
  - `Read(ctx, projectID uuid.UUID, from int64, limit int) ([]recordv0.Entry, error)` — записи проекта по возрастанию `seq`, `from` исключающая; проект не жив или чужой — ошибка, оборачивающая `project.ErrProjectNotFound`; `limit` вне `(0, JournalMaxPageSize]` или `from < 0` — `ErrInvalidInput` (400).
  - `Position(ctx, projectID uuid.UUID) (int64, error)` — текущая позиция проекта (`COALESCE(MAX(seq), 0)`), нужна потоку для старта «только новое».
  - Константы страницы: `JournalPageSize = 100`, `JournalMaxPageSize = 1000`.
- Сервис — Runtime-ноды: в `application.RuntimeNode` он **не** появляется (Engineering-нода Journal не читает).
- `postgres/runtime/journal_repository_postgres.go`: `Read` (`WHERE project_id = $1 AND seq > $2 ORDER BY seq LIMIT $3`, декодирование кодеком из 42_1) и `Position`; живость проекта проверяется в запросе (как у `FindAll` Revisions — удалённый проект даёт `project.ErrProjectNotFound`, а не пустой список).

### REST (Runtime-ноды)

- `restapi.Services` получает `Journal runtimeapp.JournalService`; `main.go` и `servertest` проводят его.
- `restapi/journal.go`, маршруты в `routes()`:
  - `GET /api/v0/projects/{project_id}/journal?from=<seq>&limit=<n>` → 200 `apiv0.JournalResponse` (`contract/api/v0`, MINOR: `JournalResponse{Entries []recordv0.Entry}`); `from` по умолчанию 0, исключающая; `limit` по умолчанию 100; нечисловой или вне диапазона — 400 `invalid_input`; проект не жив — 404 `project_not_found`; каждый ответ несёт `GrowSCADA-Schema-Version`.
  - `GET /api/v0/projects/{project_id}/journal/events` — SSE. До открытия потока: проект не жив — 404, плохие `from`/`Last-Event-ID` — 400 (ошибка ещё может быть Problem Details, а не начатый поток). Дальше — `Content-Type: text/event-stream`, комментарий `: connected`, и на каждую запись `id: <seq>` + `data: <Entry>` (документ `contract/record/v0` как есть, со своим `schema_version`).
    - Старт: `?from=` важнее заголовка `Last-Event-ID`; иначе — `Position` проекта (только новые записи). Подписка на пробуждения регистрируется **до** чтения позиции, чтобы коммит между ними не потерялся.
    - Цикл: дочитать `Read` страницами до пустой страницы (записи по порядку `seq`), затем ждать пробуждения; пробуждения — подписка `eventbus` на `tag_updated` и `revision_created` этого проекта (payload не используется, только как сигнал «дочитай»), плюс страховочный тикер (5 с) на случай пропущенного сигнала. Нет пробуждений — нет чтения; сигналы схлопываются (канал ёмкостью 1).
    - Доставка at-most-once (ADR 0019): поток читает таблицу, поэтому догон по `Last-Event-ID` точен, а живая гонка двух коммитов может дать запись не по порядку или пропуск — это записано в ADR и в комментарии обработчика.
    - Поток read-only: `?client=` не нужен, EditLock не трогается; `maxClients` хаба на него не распространяется (свой обработчик, не `eventHub`). `/events` не меняется. Пробуждение держится на `tag_updated`/`revision_created`: если 43 снимет `tag_updated` с шины, пробуждение перевешивается на то событие, которое его заменит, — записи в таблице от этого не зависят.

### Клиент

- `apiclient.ProjectClient.Journal(ctx, from int64, limit int) ([]recordv0.Entry, error)` — чтение диапазона, `ErrNotFound` на 404. Клиент SSE-потока появится с Player (43).

### Bruno

- `get_journal` — после `patch_tag_value`: 200, заголовок схемы, `entries` не пуст, последняя запись `type = tag_state` с `tag_state.tag_id` записанного тега и `seq > 0`; запрос с `?from=<последний seq>` — пустой список; `limit=0` — 400; неизвестный проект — 404 `project_not_found`.
- `get_journal_events` — тег `manual` (поток не заканчивается), проверяет `Content-Type: text/event-stream` и первую строку `: connected`.
- Порядок `seq` коллекции: чтение Journal идёт после записи значения, до удалений.

### Тесты

- Интеграционные: диапазон по возрастанию `seq`, `from` исключающая, `limit` усекает, чужой проект не виден, удалённый проект — `project.ErrProjectNotFound`, пустой Journal — пустой список.
- Сервис: 404 чужого/удалённого проекта, `ErrInvalidInput` на `limit`/`from`.
- REST: 200/400/404 чтения; SSE получает записи с `id:` и декодируемым `data`; переподключение с `Last-Event-ID` догоняет пропущенное (записать значение, пока клиент отключён); новая связь без `from` несёт только новое; `?from=` важнее заголовка; запись другого проекта не будит поток; отключение клиента снимает подписку (нет утечки обработчиков в `eventBus`).
- `servertest.StartAPI` с Journal-сервисом; `roles_test` зелёный.

### Документация

- `AGENTS.md`: оба маршрута в перечне REST, `Last-Event-ID`/`: from`, at-most-once and exact replay, `JournalRepositoryPostgres`/`JournalService` в абзацах Runtime-ноды, что `/events` не изменился и судьбу `tag_updated` решает 43.
- `CHANGELOG.md`.
- `GLOSSARY.md` и ADR 0019 — уже записаны в 42_1.

## За рамками

- Player/Operation (43), Checkpoint (44), History (45), PlaybackFile (46).
- Клиент SSE-потока в `apiclient`/PWA и переход UI на Journal — 43.
- Права доступа к Journal — 50; ограничение числа одновременных потоков — вместе с ним.
