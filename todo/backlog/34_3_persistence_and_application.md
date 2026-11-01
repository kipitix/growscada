# Persistence и application: устройства, окружения, поставщик Tag'а
Status: ready-for-agent
Blocked by: 34_2

Часть 3 задачи 34. Репозитории, сервисы, мост Engineering↔Runtime. Без REST и без симулятора.

Источник истины — `todo/backlog/34_device_gateway.md` (часть A), разделы «Связь с Tag», «Авторизация», «Что меняется в коде» — это **спецификация**, а не отдельная работа: она уже согласована, её делать не нужно.

## Что сделать

### Persistence (Engineering-нода)

- `internal/server/infrastructure/postgres/repositories/` — репозитории Device, Environment, EnvironmentDevice по образцу существующих (`aggregateStore`: запись состояния и pending-событий в **одной** транзакции — ADR 0008; CAS по загруженной Version).
- Области проекта (`ProjectScopes` в `internal/server/application/`) получают `Devices()`, `Environments()`, `EnvironmentDevices()` — все запросы ограничены `project_id` (ADR 0006); сохранение агрегата чужого проекта — ошибка.
- `engineering.tags` — колонка `device_id` (миграция — часть 1); репозиторий Tag'ов читает и пишет её.

### Application (Engineering-нода)

- `internal/server/application/` — сервисы по образцу `TagService`/`LibraryService`: `DeviceService`, `EnvironmentService` (CRUD + привязка пары `(Environment, Device)` с её параметрами). Мутации: разбор входа → загрузка агрегата → его метод → `Save`/`Delete` → `appdto`. События публикуются через outbox, не сервисами (ADR 0008).
- `appdto` — DTO'ы для Device, Environment, EnvironmentDevice.
- **Мост в Runtime**: определение Tag'а теперь несёт поставщика.
  - `appdto.TagDefinition` получает `DeviceID`.
  - `application.RuntimeNode` (порт) — `UpsertTagDefinitions` передаёт `DeviceID`; `RemoveTagStates` — без изменений.
  - `application/runtime` — `runtime.TagDefinition` получает `DeviceID`; `tagstate.Definition` получает `DeviceID`.
  - `internal/server/inprocess/runtime_node.go` — переносит `DeviceID`.
  - `postgres/runtime` — `UpsertDefinitions` пишет `device_id` в копию определения.
- **Поставщик — часть определения**: `engineering.tags.device_id` → копия `runtime.tags.device_id` (миграция — часть 1). Проверка «Tag принадлежит этой Device» — в Runtime-ноде (часть 4).

### Проверка поставщика в Runtime-ноде (application)

- `application/runtime` — `SetTagValueInput` получает `DeviceID` писателя.
- `tagstate.Repository.Write` получает поставщика и **отказывает**, если он не поставщик Tag'а: проверка в том же UPDATE, что и живость проекта и тип Tag'а (ADR 0011: «the hot path pays no extra read»). Домен-ошибка — «Tag не принадлежит этой Device» (не 404: Tag есть, писать нельзя).
- Ошибка отзывается наверх как 403 (маппинг — часть 4).

## Проверка

- `make build` и `make test` проходят.
- Интеграционные тесты репозиториев (testcontainers-go) — на новые таблицы: CRUD, CAS по версии, область проекта, удаление Device, которое обнуляет `device_id` у его Tag'ов.
- Тесты `tagstate`: запись Tag'а чужой Device отклоняется; запись Tag'а без поставщика — по-прежнему разрешена (поставщика нет, писать может человек).

## Вне объёма

- Миграции — часть 1; домен — часть 2; REST, токены, симулятор — части 4–5.
