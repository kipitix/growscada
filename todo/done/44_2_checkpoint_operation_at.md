# Checkpoint: `?at=` в Operation
Status: ready-for-agent
Priority: 3
Blocked by: 44_1
Parent: 44_checkpoint
Part: 2/2
Closes parent: yes

Слой D. Часть 2: read-model стартового состояния на прошлую позицию Journal для History. Спецификация — `44_checkpoint.md`; архитектура — ADR 0020, ADR 0023.

## Работа

1. **Сервис.** `application/runtime.OperationStateService` получает `GetAt(ctx, projectID, at int64) (OperationState, error)`:
   - зажимает `at` до текущей позиции (`JournalService.Position`);
   - `Nearest(ctx, projectID, at)`; снимка нет — `OperationState{Position: 0}` (Deployed false, TagStates пуст);
   - иначе `Deployment` по номеру из снимка (`DeploymentService.Find`) и состояния тегов из снимка: `TagState{ID, Value, Quality, Seq}`, `WrittenAt` — nil; Name/Type/Version заполняются из документа Deployment, если тег в нём назван (для API они не нужны);
   - Project не жив — ошибка, оборачивающая `project.ErrProjectNotFound`.
   - `Get` (без `at`) не меняется.
2. **REST.** `GET /api/v0/projects/{project_id}/operation?at=<position>`:
   - без `at` — прежний `Get`;
   - `at` нечисловой или отрицательный — 400 `invalid_input`; будущий — зажим (ответ несёт фактическую `position`);
   - неживой Project — 404 `project_not_found`;
   - ответ — тот же `OperationStateResponse`; новых типов нет. `OperationQueryAt` в `contract/api/v0` (0.30 → 0.31), `make schemas`.
3. **Клиент.** `apiclient.ProjectClient.OperationAt(ctx, position) (apiv0.OperationStateResponse, error)`; `Operation` не меняется.
4. **Bruno.** Запрос `GET …/operation?at=<position>` на seed-данных после Deploy: статус 200, `GrowSCADA-Schema-Version`, `position`, `deployment`, `tag_states`; плюс запрос с нечисловым `at` — 400 `application/problem+json`. Статическая проверка коллекции проходит (GET, `requestBodies` не нужен).
5. **Тесты.**
   - `GetAt`: до первого снимка — пустое состояние на 0; ровно на позиции снимка; между снимками (берётся предыдущий); за концом — зажим; неживой Project.
   - REST: 200 без `at` и с `at`; 400 на `at=x` и `at=-1`; 404 на удалённый Project; заголовок схемы.
   - `apiclient.OperationAt` против `servertest`.
6. **Проверка.** `make build`, `make test`, `make bruno_test`.

## За рамками

History UI (45), PlaybackFile (46), runtime-сущности (48), retention, изменение `Operation` без `at`, права доступа (50).
