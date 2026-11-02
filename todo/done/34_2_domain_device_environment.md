# Домен: Device, Environment, EnvironmentDevice
Status: ready-for-agent
Blocked by: 34_1

Часть 2 задачи 34. Агрегаты и правила — без persistence и без API.

Источник истины — `todo/backlog/34_device_gateway.md` (часть A), раздел «Сущности» — это **спецификация**, а не отдельная работа: она уже согласована, её делать не нужно.

## Что сделать

### Новый пакет `internal/server/domain/device`

Образец — `internal/server/domain/librarypin` (агрегат, идентифицируемый парой, иммутабельный, свой Version, свои события) и `internal/server/domain/library`.

- **Device** — агрегат: `id`, `projectID`, `name` (уникально в проекте — ADR 0003), `comment`, `protocol` (enum-строка: `simulation` на старте), `config` (JSON под протокол), `version`. Методы: `CreateDevice`, `ReconstituteDevice`, `Update` (смена имени/комментария/протокола/конфига), `Delete`. Свой Version, `CheckVersion` — как у остальных агрегатов. События: `DeviceCreatedEvent`, `DeviceUpdatedEvent`, `DeviceDeletedEvent` — все `project.ContentEvent` (живут в черновике проекта → едут в проектный SSE-поток).
- **Environment** — агрегат: `id`, `projectID`, `name` (уникально в проекте), `comment`, `version`. Методы: `Create`, `Reconstitute`, `Rename`, `Delete`. События: `EnvironmentCreatedEvent`, `EnvironmentUpdatedEvent`, `EnvironmentDeletedEvent` — `project.ContentEvent`.
- **EnvironmentDevice** — агрегат, идентифицируемый **парой** `(environmentID, deviceID)`: `id`, `environmentID`, `deviceID`, `params` (JSON под протокол), `version`. Методы: `Create`, `Reconstitute`, `UpdateParams`, `Delete`. События: `EnvironmentDeviceCreatedEvent`, `EnvironmentDeviceUpdatedEvent`, `EnvironmentDeviceDeletedEvent` — `project.ContentEvent`.
- **Значения** — value objects по образцу `tag.TagName` / `library.LibraryName`: `DeviceName`, `EnvironmentName`, `DeviceProtocol`. Пустое имя — ошибка; непустой `protocol` обязателен. Имена — с проверкой формата, как у `TagName`.
- **Ошибки** — sentinel'ы по образцу `library`/`scene`: `ErrDeviceNotFound`, `ErrDeviceConflict`, `ErrDeviceNameTaken`, `ErrEnvironmentNotFound`, `ErrEnvironmentConflict`, `ErrEnvironmentNameTaken`, `ErrEnvironmentDeviceNotFound`, `ErrEnvironmentDeviceConflict`.
- **Репозитории** — интерфейсы по образцу `librarypin.LibraryPinRepository`: `DeviceRepository`, `EnvironmentRepository`, `EnvironmentDeviceRepository` (с областями проекта, как `ProjectScopes`).

### Связь с Tag

- `internal/server/domain/tag` — `Tag` получает `deviceID` (nullable) в определении: `CreateTag`/`ReconstituteTag` — с поставщиком; метод `Supplier()` (или `DeviceID()`) — доступ. Домен не знает про `domain/device` напрямую: ID — `id.ID[device.Device]`, а связь хранится в tag'е (как сейчас хранится `projectID`).
- Валидация: Tag без поставщика — норма; Tag с поставщиком — норма; **поставщик не может измениться без версии** (смена поставщика — обычная мутация Tag'а, ожидающая версию).

### Роли

- `internal/server/domain/device` добавить в `engineeringRole` в `internal/server/roles_test.go` (список пакетов роли задан явно — без этого тест ролей его не увидит).

## Проверка

- `make build` и `make test` проходят.
- Юнит-тесты домена (`internal/server/domain/device/`): создание, переименование, смена конфига, удаление; уникальность имён (в домене — как у `library`); `CheckVersion` отдаёт конфликт при чужой версии.
- `roles_test.go` проходит.

## Вне объёма

- Миграции — часть 1. Persistence, application, REST — части 3–4.

## Comments

- 2026-11-01 (реализация): события Device/Environment/EnvironmentDevice заведены сразу во всех местах, которых требуют экзаустивные тесты: `event.EventType*`, `apiv0` (`EventTypes()` и поля `environment_id`/`device_id`, MINOR 0.6 → 0.7 и `make schemas`), `restapi.apiEventTypes`, outbox-кодек, журнал событий UI (`eventTypeInfoByType`). Без этого уже в этой части падали бы `TestCodec_SamplesCoverEveryDomainEventType`, `TestNewAPIEvent_TranslatesEveryEventType`, `TestAPIEventTypes_UsesEveryContractEventType` и `TestEventTypeInfo_CoversTheContract`; части 4 остаются маршруты, обработчики и Bruno.
- `domain/device` отнесён к `sharedDomain`, а не к `engineeringRole`: `domain/tag` (общий для ролей) называет поставщика как `id.ID[device.Device]`, и транзитивный импорт сделал бы Runtime-ноду зависящей от engineering-пакета (`TestRoles_DoNotImportEachOther` упал бы). Если нужен именно `engineeringRole` — придётся отказаться от `id.ID[device.Device]` в `tag` (см. ревью).
- Мутации поставщика у Tag'а нет: у Tag'а вообще нет update-метода и своего события определения (`tag_updated` занят состоянием), а привязка Tag'а к Device в API ещё не спроектирована (задача 66). Поставщик задаётся при создании и восстановлении (`tag.NoSupplier()` — «поставщика нет»); проверка «не меняется без версии» появится вместе с самой мутацией.

- 2026-11-02 (code review): домен поправлен по итогам ревью — валидация `protocol`/имени в конструкторах и мутаторах, нулевые `DeviceConfig`/`EnvironmentDeviceParams` значат `{}`, мёртвый `ReloadTargets.Devices` убран, контракты репозиториев больше не обещают изменений чужих агрегатов. Решениями гриллинга `Rename` у `Environment` заменён на `Update` (имя и комментарий), а у `EnvironmentDevice` убран собственный ID: пару, как и Library Pin, называют Environment и Device (миграции обеих цепочек поправлены). Остальное — `tag.Supplier`, проверки поставщика, типизированный конфиг `simulation` — в задачах 34_3, 68 и 69.
