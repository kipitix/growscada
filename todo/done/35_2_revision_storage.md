# Хранение Revision: `runtime.revisions` и `engineering.project_files`
Status: ready-for-agent
Blocked by: 35_1

Часть 2 задачи 35. **Схема и репозитории:** обе миграции, репозиторий ревизий с транзакцией создания и репозиторий копии ProjectFile у Engineering. Сервисов, порта и событий здесь нет.

Источник истины — `todo/backlog/35_projectfile_deploy_revision.md` (гриллинг 2026-11-03, Q5, Q6, Q8): это **спецификация**, а не отдельная работа.

## Что сделать

### Миграция `runtime/`

- `runtime.revisions(pk_id SERIAL PRIMARY KEY, project_id UUID NOT NULL → projects(id) ON DELETE CASCADE, n INTEGER NOT NULL, deployed_at TIMESTAMPTZ NOT NULL, config JSONB NOT NULL, UNIQUE (project_id, n))`; комментарий в миграции: строка неизменяема, `config` — истина, копии Runtime-схемы производные (ADR 0011, amendment 2026-11-03); удаляется каскадом с Project.
- Индекс под чтение текущей ревизии не нужен: `UNIQUE (project_id, n)` уже индексирует `project_id` первым, `ORDER BY n DESC LIMIT 1` идёт по нему.

### Миграция `engineering/`

- `engineering.project_files(project_id UUID PRIMARY KEY → projects(id) ON DELETE CASCADE, revision_n INTEGER NOT NULL, deployed_at TIMESTAMPTZ NOT NULL, config JSONB NOT NULL)`; комментарий: вторичная копия последнего отправленного ProjectFile, нужна для бейджа «Revision N · deployed at T» и индикатора неразвёрнутых изменений; истина — на Runtime-ноде (Q6).

### `postgres/runtime.NewRevisionRepositoryPostgres`

- `CreateRevision(ctx, projectID uuid.UUID, file projectv0.ProjectFile, now time.Time) (appdto.Revision, error)` — **одна транзакция**:
  1. `pg_advisory_xact_lock` по `projectID` (сериализует одновременные Deploy одного проекта);
  2. `n = COALESCE(max(n), 0) + 1` (`UNIQUE (project_id, n)` — страховка);
  3. `INSERT runtime.revisions`;
  4. пересобрать копии **диффом**: upsert `runtime.tags`, `runtime.devices`, `runtime.environments`, `runtime.environment_devices` по ID, удалить те, которых в документе нет;
  5. состояния: оставшемуся Tag — ничего, новому — initial (нулевое значение типа, Quality `bad`), смене TagType — сброс в initial, исчезнувшему — уходит каскадом с копией; `runtime.device_tokens` не трогать.
- `FindCurrent(ctx, projectID) (appdto.Revision, bool, error)` — `ORDER BY n DESC LIMIT 1`, `bool` = есть ли вообще.
- Не делать `DELETE` всех копий с последующей вставкой: каскад `fk_runtime_tag_states_tag_id` унёс бы значения, а Diff обязан их сохранить (Q5).
- `appdto.Revision` — `{Number int, DeployedAt time.Time, Content projectv0.ProjectFile}` (используется портом и REST в частях 3 и 5).

### `postgres.NewProjectFileRepositoryPostgres`

- `Save(ctx, projectID uuid.UUID, revision appdto.Revision) error` — upsert строки копии.
- `FindByProject(ctx, projectID uuid.UUID) (appdto.Revision, bool, error)`.

### Тесты

- Интеграционные тесты репозиториев (testcontainers, как у соседних): нумерация `1, 2, 3`; параллельный `CreateRevision` не даёт дублей; повторная запись той же Revision запрещена; дифф сохраняет `tag_states` и токены, удаляет копию исчезнувшего Device и каскадом уносит состояния исчезнувшего Tag; смена TagType сбрасывает состояние в initial; удаление Project уносит ревизии и копию ProjectFile.
- Миграции применяются на чистой БД (`make db_up`) и на сидовой без ошибок.

## Проверка

- `make build`, `make test`; `make db_up` проходит с нуля; `goose` Down/Up обеих цепочек на сидовой БД.

## Вне объёма

- Порт `RuntimeNode`, сервис Deploy, событие `revision_created` — часть 3.
- Сборка ProjectFile из Draft, preview, Discard — часть 4; REST и UI — часть 5.

## Comments

- 2026-11-03 (реализация). `appdto.Revision` создан для порта и для копии Engineering-ноды. Репозиторий Runtime-ноды по правилам ролей (ADR 0011, `internal/server/roles_test.go`) не может импортировать `application/appdto` — это пакет Engineering-ноды, — поэтому он возвращает `application/runtime.Revision`. Часть 3 конвертирует один в другой в `inprocess`, как уже делает для `appdto.TagState` ↔ `runtime.TagState`. Сигнатуры методов — как в описании (`uuid.UUID`, `projectv0.ProjectFile`, `time.Time`).
