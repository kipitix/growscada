# Checkpoint: хранение и писатели
Status: ready-for-agent
Priority: 3
Blocked by: 35, 42, 72_4
Parent: 44_checkpoint
Part: 1/2

Слой D. Часть 1: запись `runtime.checkpoints`, контракт `recordv0.Checkpoint`, кодек и репозиторий, анкоры в Deploy/Rollback/Deploy файла, фоновый сервис периодических снимков. API не меняется. Спецификация — `44_checkpoint.md`; архитектура — ADR 0019, ADR 0023.

## Работа

1. **Миграция** `internal/server/infrastructure/postgres/migrations/runtime/20261111000000_checkpoints.sql`:
   ```sql
   CREATE TABLE runtime.checkpoints (
       project_id     UUID        NOT NULL,
       seq            BIGINT      NOT NULL,
       taken_at       TIMESTAMPTZ NOT NULL,
       schema_version TEXT        NOT NULL,
       payload        JSONB       NOT NULL,
       PRIMARY KEY (project_id, seq),
       CONSTRAINT fk_runtime_checkpoints_project_id
           FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
   );
   CREATE INDEX idx_runtime_checkpoints_project_seq
       ON runtime.checkpoints(project_id, seq DESC);
   ```
   `payload` — `{deployment, tag_states}` (envelope в колонках, как у Journal). FK на Deployment не нужен: номер пишется в той же транзакции, Deployment неизменяем и удаляется только с Project; `seq` — позиция реальной записи Journal. Down — `DROP TABLE runtime.checkpoints`.
2. **Контракт** `contract/record/v0` (0.4 → 0.5): `Checkpoint` (встраивает `Header`; `seq int64`, `project_id uuid.UUID`, `timestamp` RFC 3339, `deployment *Deployment`, `tag_states []CheckpointTagState`) и `CheckpointTagState{tag_id, value, quality, seq}`. Переиспользовать существующий `Deployment`-payload. `make schemas`.
3. **Кодек и репозиторий** `internal/server/infrastructure/postgres/runtime/checkpoint_codec.go`, `checkpoint_repository_postgres.go`:
   - `insertCheckpoint(tx, projectID, seq, deploymentRef, takenAt)`: читает все `runtime.tag_states` проекта в текущей транзакции (порядок по `tag_id`), собирает `recordv0.Checkpoint`, пишет строку; `ON CONFLICT (project_id, seq) DO NOTHING`.
   - `Nearest(ctx, projectID, position)`: последний Checkpoint с `seq <= position`; `false`, если нет.
   - `DueProjects(ctx, everyEntries)`: Project'ы, у которых `MAX(journal.seq) >= последний Checkpoint.seq + everyEntries` (join `projects` только живых).
   - `Snapshot(ctx, projectID, everyEntries)`: одна `REPEATABLE READ`-транзакция: `MAX(seq)`, текущая Deployment, все состояния; если позиция не созрела — ничего; вставка через `insertCheckpoint`.
   - Кодек: `encodeCheckpoint`/`decodeCheckpoint` с `schema_version` в строке; payload — только `{deployment, tag_states}`.
4. **Анкоры в Deployment.** `insertDeployment` возвращает `(number, seq, err)`; `CreateDeployment` и `Rollback` после `refreshCopies` вставляют Checkpoint на этот `seq` (Deploy Release, Deploy файла, Rollback — все три пути).
5. **Фоновый сервис** `internal/server/application/runtime/checkpoint_service.go`:
   - `CheckpointService` с `Run(ctx)` (тикер `CheckpointScanEvery = time.Minute`) и `RunOnce(ctx)` для детерминированных тестов; константа `CheckpointEveryEntries = 10_000`.
   - `Run` берёт `DueProjects`, для каждого вызывает `Snapshot`; ошибка логируется, следующий проход повторит.
   - Запуск в `cmd/combined_server/main.go` рядом с dispatcher/policy runner; `servertest` тикер не запускает.
6. **Тесты.**
   - Кодек: round-trip, отказ чужой `schema_version`, пустой `tag_states`.
   - Интеграционные (`postgres/runtime`): Deploy Release/файл/Rollback пишут Checkpoint ровно на `seq` записи `deployment`; состояния в снимке совпадают с `runtime.tag_states`; `Nearest` до/после/на позиции; `DueProjects` до и после порога; повторный `Snapshot` не дублирует (`ON CONFLICT`); удаление Project удаляет Checkpoint каскадом.
   - `CheckpointService.RunOnce`: снимок только для созревшего Project, повторный вызов ничего не добавляет.
7. **Проверка.** `make build`, `make test`.

## За рамками

`?at=`, REST, `apiclient`, Bruno — часть 2 (`44_2_checkpoint_operation_at`). History UI (45), PlaybackFile (46), runtime-сущности (48), retention.
