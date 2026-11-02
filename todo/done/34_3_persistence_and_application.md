# Persistence и application: устройства, окружения, поставщик Tag'а
Status: ready-for-agent
Blocked by: 34_2

Часть 3 задачи 34. Репозитории, сервисы, мост Engineering↔Runtime. Без REST и без симулятора.

Источник истины — `todo/backlog/34_device_gateway.md` (часть A), разделы «Связь с Tag», «Авторизация», «Что меняется в коде» — это **спецификация**, а не отдельная работа: она уже согласована, её делать не нужно.

## Что сделать

### Persistence (Engineering-нода)

- `internal/server/infrastructure/postgres/repositories/` — репозитории Device, Environment, EnvironmentDevice по образцу существующих (`aggregateStore`: запись состояния и pending-событий в **одной** транзакции — ADR 0008; CAS по загруженной Version).
- Области проекта (`ProjectScopes` в `internal/server/application/`) получают `Devices()`, `Environments()`, `EnvironmentDevices()` — все запросы ограничены `project_id` (ADR 0006); сохранение агрегата чужого проекта — ошибка. У пары нет своего ID и `NextID`: её ключ — `(environment_id, device_id)`, `Save` вставляет и обновляет по этой паре.
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

### Решения гриллинга (блок 2, 2026-11-02)

- **Поставщик — value object `tag.Supplier`**, а не голый нулевой `id.ID`: `NoSupplier()` (Tag пишут люди) и `SuppliedByDevice(id.ID[device.Device])`; `Tag.Supplier()` заменяет `DeviceID()`. В `engineering.tags.device_id` — NULL, в контракте — отсутствующее поле (`omitempty`), если часть 4 его отдаёт.
- **`domain/device` остаётся в `sharedDomain`** (`roles_test`): общий `tag` называет ID поставщика как `id.ID[device.Device]`, поэтому пакет не может принадлежать одной роли; сам агрегат по-прежнему Engineering-ноды. Нужна поправка ADR 0011 (состав общего домена) и усиление `roles_test`: пакеты `sharedDomain` тоже проверяются на импорт `runtimeRole`.
- **Смена поставщика с ожидаемой Version отложена** до API привязки Tag→Device (задача 68): сейчас поставщик задаётся только `CreateTag`/`ReconstituteTag`, и инвариант «не меняется без версии» выполняется тривиально.
- **Проверка «поставщик из того же Project» — в репозитории Tag'ов**, в той же транзакции, что и запись (`EXISTS` по `engineering.devices` с `project_id`), сентинел `tag.ErrSupplierOfAnotherProject`; страховка на уровне схемы — составной FK `(device_id, project_id)` (правка миграции части 1).
- **Удаление с ссылками — отказ, а не каскад**: `DeviceRepository.Delete` отказывает, пока Device поставляет Tag (`ErrDeviceInUse`), `EnvironmentRepository.Delete` — пока у Environment есть пары (`ErrEnvironmentInUse`); политик и новых событий нет, ADR 0008 не задействован. Авто-отвязывание — только вместе с API привязки (задача 68).

## Проверка

- `make build` и `make test` проходят.
- Интеграционные тесты репозиториев (testcontainers-go) — на новые таблицы: CRUD, CAS по версии, область проекта; удаление Device, поставляющей Tag, отклоняется (`ErrDeviceInUse`), без ссылок — удаляет; Tag с Device чужого проекта отклоняется (`ErrSupplierOfAnotherProject`).
- Тесты `tagstate`: запись Tag'а чужой Device отклоняется; запись Tag'а без поставщика — по-прежнему разрешена (поставщика нет, писать может человек).

## Вне объёма

- Миграции — часть 1; домен — часть 2; REST, токены, симулятор — части 4–5.

## Comments

- 2026-11-02 (реализация): репозитории `Device`, `Environment`, `EnvironmentDevice` (CAS, outbox, область проекта по `project_id`, у пары ключ `(environment_id, device_id)`); области проекта получили `Devices()`, `Environments()`, `EnvironmentDevices()`; `DeviceService` и `EnvironmentService` с DTO в `appdto`; мост в Runtime переносит `device_id` (`appdto.TagDefinition` → `runtime.TagDefinition` → `tagstate.Definition` → `runtime.tags`); `tagstate.Repository.Write` принимает `tag.Supplier` и проверяет поставщика в том же UPDATE, отказывая `tag.ErrTagNotSuppliedByDevice`. Поставщик — объект-значение `tag.Supplier` (`NoSupplier`/`SuppliedByDevice`), проверка «Device того же проекта» — в транзакции записи Tag'а (`tag.ErrSupplierOfAnotherProject`), составной FK `(device_id, project_id)` добавлен правкой миграции части 1. Удаление Device/Environment с ссылками — отказ (`ErrDeviceInUse`/`ErrEnvironmentInUse`); очистка удалённого проекта теперь убирает пары, Device и Environment перед строкой проекта. Тесты: интеграционные репозиториев (CRUD, CAS, область проекта, отказы удаления, поставщик чужого проекта), `tagstate` (чужой поставщик и Tag без поставщика), сервисы Device/Environment. `make build`, `make test`, `make bruno_test` проходят.
- 2026-11-03 (code review): Runtime-копии Project'а — производные данные: `runtime.devices`/`runtime.environments` ссылаются на `projects` с `ON DELETE CASCADE`, `runtime.environment_devices` — на обе копии с тем же (иначе после Deploy строка проекта не удалялась бы), добавлен индекс пары по `device_id` в обеих схемах. Списки Device и Environment читаются одним запросом с проверкой живого проекта (`FindAllInLiveProject`), как Tag и Scene. `PutEnvironmentDevice` проверяет Version до разбора `params` (устаревшая запись — 409), FK-гонка поставщика Tag'а отдаёт `ErrSupplierOfAnotherProject`. Добавлены тест очистки проекта с Draft-Device/Environment/парой и тест каскадов runtime-копий; `AGENTS.md` и комментарии контракта событий обновлены. **Миграции `20261101000000` правятся на месте** (составной FK, каскады, индексы): на БД, где старая версия уже применена, нужен `make full_restart` (или `make db_down` + `make db_up`), иначе goose их не переиграет.
