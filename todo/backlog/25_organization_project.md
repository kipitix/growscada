# Organization и Project: project_id везде
Status: ready-for-human

Первая задача фундамента (слой A). Термины — `CONTEXT.md` (Organization, Project), решение — ADR 0006.

- Агрегат Project: проект — единица изоляции. Tags, Scenes с Widgets, Draft, Revisions, Journal, Devices принадлежат ровно одному Project. Сервер хостит несколько проектов в обеих ролях (Engineering и Runtime node); проекты не ссылаются на сущности друг друга.
- Organization (tenant) владеет Projects и Libraries. Пока только ключ в данных: одна стартовая Organization из seed, без пользователей и входа (User и права — задачи 43, 44).
- `project_id` во всех таблицах проектных сущностей и в их уникальных ограничениях; `organization_id` у Project (и у Library — задача 27).
- Имя Tag уникально внутри Project: естественный ключ — (Project, имя). Обновить ADR 0003, `UNIQUE` в БД, `GET /tags?name=`.
- REST адресует проект: ресурсы проекта — под `/api/v0/projects/{project}/…`. Поднять SchemaVersion API, `make schemas`.
- growctl: манифесты и команды указывают проект (флаг/контекст, как namespace в kubectl). Контракт манифестов — SchemaVersion.
- devicelink и симулятор: пока без Device Token проект задаётся конфигурацией; в задаче 28 проект будет определяться токеном.
- UI: выбор проекта (список проектов, создание, переименование, удаление).
- Миграция dev-БД и seed: существующие данные — в один проект «Main» стартовой Organization.
- Тесты репозиториев (testcontainers), Bruno, `CHANGELOG.md`.

## Comments

- 2026-10-04: бывшая задача 30. Отменяет решение Q21 бывшей задачи 36 («один неявный проект»): решено, что project_id нужен сразу везде, включая Runtime node, до появления Journal и Revision — иначе это вторая большая миграция.
