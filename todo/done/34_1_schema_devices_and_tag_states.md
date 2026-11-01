# Схема: Device, Environment и разделение состояния Tag'а
Status: ready-for-agent
Blocked by: 32

Часть 1 задачи 34. **Схема без поведения:** миграции обеих цепочек и переезд состояния Tag'а в отдельную таблицу. Слой домена и API — в следующих частях; здесь проверяется, что БД в порядке и тесты проходят.

Источник истины — `todo/backlog/34_device_gateway.md` (часть A) — это **спецификация**, а не отдельная работа: она уже согласована, её делать не нужно.

## Что сделать

### Цепочка `engineering/` (новый файл миграции)

- `engineering.devices(id UUID PK, project_id UUID NOT NULL → projects(id), name TEXT NOT NULL, comment TEXT NOT NULL DEFAULT '', protocol VARCHAR(50) NOT NULL, config JSONB NOT NULL DEFAULT '{}', version INTEGER NOT NULL DEFAULT 0, created_at, updated_at)`; `UNIQUE (project_id, name)`.
- `engineering.environments(id, project_id, name, comment, version, created_at, updated_at)`; `UNIQUE (project_id, name)`.
- `engineering.environment_devices(id, environment_id → engineering.environments(id), device_id → engineering.devices(id), params JSONB NOT NULL DEFAULT '{}', version INTEGER NOT NULL DEFAULT 0)`; `UNIQUE (environment_id, device_id)`.
- `engineering.tags` — колонка `device_id UUID NULL REFERENCES engineering.devices(id)` + индекс.

### Цепочка `runtime/`

- `runtime.tags` — **копия определения**: остаются `id`, `project_id`, `name`, `type`, `version`; **добавляется** `device_id UUID NULL`; **удаляются** `value`, `quality`, `seq` (ADR 0011, amendment 2026-11-01: определение и состояние в разных таблицах, `seq` — часть состояния).
- `runtime.tag_states(tag_id UUID PK → runtime.tags(id), value TEXT NOT NULL, quality VARCHAR(50) NOT NULL, seq BIGINT NOT NULL DEFAULT 0)`; `CHECK` на `quality` (`'bad'`, `'uncertain'`, `'good'`), как у `runtime.tags` сегодня; индекс не нужен — `tag_id` PK.
- `runtime.devices`, `runtime.environments`, `runtime.environment_devices` — копии соответствующих таблиц черновика (те же колонки, кроме `version` у `environment_devices`; токенов в копиях нет — токены в `runtime.device_tokens`, задача 3). Имя окружения уникально в проекте и в копии: Gateway называет окружение по имени, и имя резолвится в пару внутри runtime-схемы. `runtime.device_tokens` без FK на копию Device: токен переживает Deploy и Rollback и спит, пока Device нет в ревизии.
- `runtime.device_tokens(device_id UUID PK → runtime.devices(id), token_hash TEXT NOT NULL, fingerprint TEXT NOT NULL, issued_at TIMESTAMPTZ NOT NULL)` — токен на Device, в ревизию не едет (ADR 0004, amendment 2026-11-01).
- `runtime.tag_state_seq` — остаётся как есть; `seq` переезжает в `runtime.tag_states`.

### Перенос данных миграцией

- `runtime.tag_states` наполняется из `runtime.tags`: `INSERT INTO runtime.tag_states (tag_id, value, quality, seq) SELECT id, value, quality, seq FROM runtime.tags`.
- `runtime.tags` теряет `value`/`quality` после переноса.

## Проверка

- `make build` и `make test` проходят. Тесты репозитория Runtime-ноды (`internal/server/infrastructure/postgres/runtime/`) после переезда **упадут** — они читают `runtime.tags` одной строкой. Их надо поправить **в этой же части** (это следствие переезда, а не отдельная задача): `FindInLiveProject`/`FindByIDs` — `JOIN` `runtime.tags` и `runtime.tag_states`; `Write` — `UPDATE runtime.tag_states … FROM runtime.tags WHERE …`; `UpsertDefinitions` — вставка копии определения **и** начального состояния в одной транзакции; `Delete`/`DeleteAllOfProject` — удаляют и копию, и состояние.
- Миграции применяются на чистой БД (`make db_up`) и на БД с данными (seed) без ошибок.

## Вне объёма

- Домен (`domain/device`), application, REST, симулятор — части 2–4.
