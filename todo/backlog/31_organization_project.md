# Organization и Project: project_id везде
Status: ready-for-human

Первая задача фундамента (слой A). Термины — `CONTEXT.md` (Organization, Project), решение — ADR 0006.

- Агрегат Project: проект — единица изоляции. Tags, Scenes с Widgets, Draft, Revisions, Journal, Devices принадлежат ровно одному Project. Сервер хостит несколько проектов в обеих ролях (Engineering и Runtime node); проекты не ссылаются на сущности друг друга.
- Organization (tenant) владеет Projects и Libraries. Пока только ключ в данных: одна стартовая Organization из seed, без пользователей и входа (User и права — задачи 49, 50).
- `project_id` во всех таблицах проектных сущностей и в их уникальных ограничениях; `organization_id` у Project (и у Library — задача 33).
- Имя Tag уникально внутри Project: естественный ключ — (Project, имя). Обновить ADR 0003, `UNIQUE` в БД, `GET /tags?name=`.
- REST адресует проект: ресурсы проекта — под `/api/v0/projects/{project}/…`. Поднять SchemaVersion API, `make schemas`.
- growctl: манифесты и команды указывают проект (флаг/контекст, как namespace в kubectl). Контракт манифестов — SchemaVersion.
- devicelink и симулятор: пока без Device Token проект задаётся конфигурацией; в задаче 34 проект будет определяться токеном.
- UI: выбор проекта (список проектов, создание, переименование, удаление).
- Миграция dev-БД и seed: существующие данные — в один проект «Main» стартовой Organization.
- Тесты репозиториев (testcontainers), Bruno, `CHANGELOG.md`.

## Comments

- 2026-10-04: бывшая задача 30. Отменяет решение Q21 бывшей задачи 36 («один неявный проект»): решено, что project_id нужен сразу везде, включая Runtime node, до появления Journal и Revision — иначе это вторая большая миграция.
- 2026-10-06: из grilling задачи 20 (ADR 0008). Многоагрегатной единицы работы нет, поэтому шов, через который `project_id` попадает в запросы (ADR 0006), решается здесь. Варианты: фабрика репозиториев по области (`ForProject(p)` → `Tags()`/`Scenes()`, `ForLibrary(l)` → `WidgetTypes()` в задаче 33; интерфейсы репозиториев не меняются, забытый проект — ошибка компиляции) или `projectID` в `context.Context` (меньше diff, но ошибка только во время выполнения, политики должны класть проект в ctx сами). Требование в любом случае: строка outbox несёт `project_id`, политика берёт репозитории проекта события.
- 2026-10-11 (обсуждение архитектуры после grilling задачи 21): отложен вопрос, не признать ли Engineering и Runtime двумя Bounded Context (свои CONTEXT.md, свой смысл Tag/Widget; CQRS — только в Runtime: Journal на запись, Checkpoint + Player на чтение). Отложен из-за дублирования моделей и перевода на границе (Deploy/ProjectFile). Смягчение, если вернёмся: общие value objects (TagName, TagType, Quality, ID) — Shared Kernel, разделять только то, что расходится по смыслу.
- 2026-10-25 (grilling, ADR 0006 обновлён). Сценарий: вендор Library V, интегратор I, завод P; серверы и раздельные, и общий. Решения:
  - Идентичность Project (UUID) глобальна и переживает Deploy/ProjectFile; имя и Organization-владелец локальны для сервера, в ProjectFile не входят.
  - `widget_types` без ключа до задачи 33 (временное отступление от изоляции, см. ADR 0006 Consequences).
  - Шов — фабрика `ForProject(p)` → `Tags()`, `Scenes()`. Tag и Scene несут неизменяемый `ProjectID` — источник истины; фабрика — область запросов; `Save` через чужую область — программная ошибка. Scene при привязке сверяет `ProjectID` Tag со своим. Строка outbox несёт `project_id`; ключ задания политики включает проект.
  - Удаление Project — политика-уборщик: `DELETE` помечает и пишет `project_deleted`; для API проект сразу не существует (нет в списке, ресурсы — 404, имя свободно: уникальность среди живых). Политика удаляет Scenes, затем Tags (каждое — своя транзакция и события), строку `projects` — последней; FK без CASCADE. Политика ключует и `tag_created`/`scene_created` удалённого проекта (гонка с созданием). Переименование — PUT с ожидаемой Version.
  - REST: `/api/v0/projects/{uuid}/…`, имя уникально в Organization, `GET /projects?name=`.
  - SSE: `/projects/{p}/events` — содержимое проекта; `/events` — список Projects и WidgetTypes.
  - growctl: `--project`, файл контекста (`growctl config use-project`) или необязательное `metadata.project` (расхождение — ошибка); новый `kind: Project` (ключ — имя): apply создаёт до Tags, `--prune` проекты не трогает, `delete -f` удаляет явно; MINOR контракта манифестов.
  - Симулятор: обязательное `project:` в YAML, резолв имени при старте, нет проекта — `ExitConfig`.
  - UI: переключатель проекта в шапке root (создание, переименование, удаление с подтверждением), выбор в LocalStorage; Library от выбора не зависит; удалённый выбранный проект — выбор сбрасывается.
  - `widgetReconciliationPolicy` по `widget_type_updated` обходит Scenes всех проектов: запрос «Scenes с типом X» отдаёт (проект, Scene).
