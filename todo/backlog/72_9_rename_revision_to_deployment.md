# Переименовать Runtime Revision в Deployment

Status: ready-for-agent
Priority: 1

Слой C/D. Термины — `GLOSSARY.md` (Release, Deploy, Deployment). Часть 9 из `72_save_release_deploy.md`; выполняется до 72_4.

## Задача

Убрать четвёртый неоднозначный термин: Runtime-сущность **Revision** переименовать в **Deployment**. Commit — сохранение Draft, Release — публикация Commit'а, Deployment — запись Runtime node о том, что реально работает. Модель становится Commit → Release → Deployment; слово Revision остаётся только историческим (ADR/CHANGELOG/GLOSSARY как прежнее имя).

Это переименование, а не изменение поведения: Deploy/Rollback/файловый деплой работают как раньше, но создают Deployment.

## Домен и приложение

- `application/runtime`:
  - `Revision` → `Deployment`, `RevisionSummary` → `DeploymentSummary`, `RevisionCause` → `DeploymentCause` (значения `deploy`/`rollback`/`file` те же), `RevisionRepository` → `DeploymentRepository`, `RevisionService` → `DeploymentService`, `RevisionNotFound`/`NothingToRollback` → `DeploymentNotFound`/`NothingToRollback` (или `ErrNothingToDeploy` в 72_4).
  - `NewRevisionRepositoryPostgres` → `NewDeploymentRepositoryPostgres`; файл `revision_repository_postgres.go` → `deployment_repository_postgres.go`; `revision_service.go` → `deployment_service.go`.
  - `NewRevisionCreatedEvent` → `NewDeploymentCreatedEvent`; событие `revision_created` → `deployment_created`.
- `application/appdto.Revision` → `Deployment`; `appdto.RevisionSummary` → `DeploymentSummary`.
- `RuntimeNode` порт: `Deploy`, `DeployFile`, `FindCurrentRevision`, `FindRevision`, `ListRevisions`, `Rollback` → `Deploy`, `DeployFile`, `FindCurrentDeployment`, `FindDeployment`, `ListDeployments`, `Rollback`.
- `inprocess/runtime_node.go`, `servertest` — переименовать реализации.

## БД (runtime)

- Миграция: `ALTER TABLE runtime.revisions RENAME TO runtime.deployments`; `ALTER TABLE runtime.deployments RENAME COLUMN n TO number`; переименовать constraint/index (`uq_runtime_revisions_project_n`, `idx_runtime_revisions_*`) в `uq_runtime_deployments_project_number`, `idx_runtime_deployments_*`. `cause`, `rolled_back_from`, `deployed_at`, `config` остаются.
- `runtime.journal`: записи с `type='revision'` перевести в `type='deployment'` (`UPDATE runtime.journal SET type='deployment' WHERE type='revision'`); payload (`number`, `cause`, `rolled_back_from`) не меняется. Новые записи — `deployment`.
- Сид/миграционные тесты: таблица и записи переименованы; старые данные читаются.
- `runtime.journal_seq` не трогать.

## Контракты (`contract/api/v0`, `contract/record/v0`) — MINOR, `make schemas`

- `api/v0`:
  - `RevisionResponse` → `DeploymentResponse`, `RevisionSummaryResponse` → `DeploymentSummaryResponse`; поле `revision` в `OperationResponse` → `deployment`.
  - `GET …/revision` → `GET …/deployment`; `GET …/revisions` → `GET …/deployments`; `GET …/revisions/{n}` → `GET …/deployments/{n}`; `GET …/revisions/{n}/projectfile` → `GET …/deployments/{n}/projectfile`; `POST …/revisions/{n}/rollback` → `POST …/deployments/{n}/rollback`.
  - Старые маршруты `/revision`, `/revisions…` — 410 Gone в формате Problem Details с подсказкой на новые (как `/api/v1`), удалить при выходе API 1.0.
  - Событие `revision_created` → `deployment_created`; старые SSE-клиенты его больше не получают — зафиксировать в CHANGELOG.
- `record/v0`:
  - `Revision` → `Deployment`; тип записи Journal `revision` → `deployment`; поле `release_number` добавит 72_4.
  - Кодек/`apiclient.ParseEntry` читает и старый `revision` (для старых Journal/PlaybackFile), пишет `deployment`. `SchemaVersion` — MINOR.
- `apiclient`: `RevisionState` → `DeploymentState`; `ProjectClient.Revision` → `Deployment`; `Revisions` → `Deployments`; `RevisionProjectFile` → `DeploymentProjectFile`; `Rollback` → `RollbackDeployment`.

## UI

- `ui/player`: `Model.Revision()` → `Deployment()`; `GET …/operation` читает `deployment`; Player применяет `deployment_created`.
- Project-режим: «История деплоев» читает `GET …/deployments`; `GET …/deployment` вместо `/revision`; Deploy bar показывает работающую Deployment/Release; событие `deployment_created`.
- Operation: подписка на `deployment_created`; `revision` в `OperationResponse` → `deployment`.
- History/бэкенд: записи Journal типа `deployment`; старые `revision` читаются.
- Bruno/симулятор: маршруты `/deployments…`; старые 410.

## Документы

- ADR 0013, 0014, 0019, 0020 — поправка: «Deployment» вместо «Revision»; ADR 0021 — новая терминология.
- `GLOSSARY.md`: **Deployment** — запись Runtime node о запуске конфигурации (номер, время, причина, `release_number`, документ); **Revision** — прежнее имя Deployment (историческое).
- `AGENTS.md`: все места про Revision (Runtime) → Deployment; не путать с Commit/Release.
- `CHANGELOG.md`: переименование, старые маршруты 410, событие `deployment_created`, Journal `deployment`.

## Тесты

- Все тесты runtime/application/restapi/UI переименованы; `go test ./...` зелёный.
- Миграция: старые `runtime.revisions` и Journal `revision` читаются как `deployments`/`deployment`.
- REST: новые маршруты, старые — 410, `GrowSCADA-Schema-Version`.
- Operation/Player: `OperationResponse.deployment`, событие `deployment_created`.
- Bruno: `/deployments…`, 410 старых.
- `make build`, `make test`, `make bruno_test`.

## Приёмка

- В коде/API/UI нет Runtime-сущности Revision (кроме исторических ADR/CHANGELOG/GLOSSARY и чтения старых Journal/PlaybackFile).
- `Commit`, `Release`, `Deployment` однозначны: сохранение, публикация, запуск.
- 72_4 добавляет `release_number` уже в Deployment.

## Comments

- 2026-11-07: решение пользователя: Runtime Revision переименовать в Deployment. Выполнять до 72_4, чтобы не добавлять `release_number` в `runtime.revisions` и сразу не переименовывать.
