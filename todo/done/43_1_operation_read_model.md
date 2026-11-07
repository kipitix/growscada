# Модель чтения Operation на Runtime-ноде
Status: ready-for-agent
Parent: 43_player_operation
Part: 1/2

Часть 1 задачи 43. Источник истины — `todo/backlog/43_player_operation.md` (грилинг 2026-11-07, ADR 0020). Здесь только серверная модель чтения: контракт, сервис, маршрут, клиент, Bruno и тесты. Player и перевод Operation на Journal — часть 2.

## Что сделать

### Контракт `contract/api/v0` (MINOR 0.22 → 0.23)

- `OperationPath = "/operation"` — маршрут под путём Project.
- `OperationStateResponse`:
  - `Position int64` (`position`) — текущая позиция Journal проекта;
  - `Revision *RevisionResponse` (`revision`, без `omitempty`) — документ работающей Revision; `null` до первого Deploy. Внутри — `content: projectv0.ProjectFile`;
  - `TagStates map[string]TagStateResponse` (`tag_states`) — состояния **всех** тегов Revision по ID: ненаписанный тег — zero-значение его TagType с Quality `bad`, `seq` 0. Форма `TagStateResponse` уже есть. Нет Revision — пустой объект `{}`, не `null`.
- Документ не собирает рендер-формы: конфигурация — `projectv0.ProjectFile` как есть. CSS-матрица и WidgetType — домен `scene` Engineering-ноды (ADR 0011), Runtime-ноде его импортировать нельзя.
- `make schemas` (старая версия уходит в `schemas/api/frozen.sha256`).

### Прикладной сервис (`application/runtime`)

- `OperationStateService` (новый файл `operation_state_service.go`), метод `Get(ctx, projectID uuid.UUID) (OperationState, error)`.
- `OperationState{Position int64; Revision Revision; Deployed bool; TagStates []TagState}` — сервисный тип, не DTO API; HTTP-слой маппит его в `apiv0`.
- Читает **в этом порядке**, каждый шаг — существующий сервис/репозиторий:
  1. `JournalService.Position` — позиция;
  2. `RevisionService.FindCurrent` — Revision с документом (нет Revision — `Deployed=false`, пустые состояния, позиция всё равно отдаётся);
  3. `TagStateService.FindTagStates` по ID тегов из `Revision.Content.Tags` — только существующие состояния.
- Ненаписанные теги добивает сам: `tag.NewTagType(definition.Type)` → `InitialState()` (zero + Quality bad), `seq` 0, `WrittenAt` nil. Невалидный тип в документе — ошибка (документ собирал сервер, но чтение не должно молча врать).
- Удалённый/чужой Project — ошибка, оборачивающая `project.ErrProjectNotFound` (404). Journal и Revision уже проверяют живость сами; порядок чтения обязателен: позиция раньше Revision и состояний (Deploy между чтениями не должен пропасть; повторы безвредны, ADR 0020).
- Сервис — Runtime-ноды: в порт `application.RuntimeNode` **не** добавляется (Engineering-нода его не читает). Импортов `domain/scene`, `engineering`-схем и Draft нет. Новых репозиториев и миграций нет — только композиция существующих чтений.
- Проводка: `main.go`, `servertest`, `restapi.Services` (как остальные сервисы Runtime-ноды).

### REST

- `GET /api/v0/projects/{project_id}/operation` — handler в `restapi` (новый файл или рядом с deploy; Runtime-ноды), в `routes()`, заголовок `GrowSCADA-Schema-Version` как у всех ответов.
- Живой Project без Revision — **200** с `revision: null` и `position` (обычно 0), а не 404: Player должен поймать первый Deploy из потока.
- Удалённый Project — 404 `project_not_found`; нечисловой/битый ID — 404/400 по общим правилам `pathUUID`.

### apiclient

- `(*ProjectClient).Operation(ctx) (apiv0.OperationStateResponse, error)`.
- Ошибка 404 остаётся `ErrNotFound` (удалённый Project); «нет Revision» — это успешный ответ с `revision == nil`.

### Bruno и тесты

- Bruno-коллекция сервера: запрос `GET …/operation` в разделе deploy — после Deploy, до первых записей значений; проверяет 200, заголовок схемы, `position`, `revision.number`, наличие тега с state и тега с zero/bad. `requestBodies` не нужен (GET), маршрут покрывается статической проверкой.
- Тесты сервиса с фейками: порядок чтений; ненаписанный тег → zero/bad/seq 0; ненаписанный integer/boolean; отсутствие Revision → 200-состояние; удалённый Project → `ErrProjectNotFound`; пустой Project.
- E2E через `servertest`: Deploy → `GET …/operation` содержит Revision, позицию и состояния; `PATCH …/value` двигает позицию и состояние; Project без Revision → 200 с `revision: null`; удалённый Project → 404; заголовок схемы.

### Документация

- `AGENTS.md`: маршрут и модель чтения в разделе REST/Runtime; `CHANGELOG.md`.

## Проверка

- `make build`, `make test` проходят.
- `make bruno_test` проходит с новым запросом.
