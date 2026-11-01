# Device, Device Gateway, Device Token, Environment
Status: ready-for-agent
Blocked by: 32

Слой B. Термины — `CONTEXT.md` (Device, Environment, Environment Device, Device Gateway, Device Token); обновить ADR 0004, пересмотреть ADR 0011 (структура runtime-схемы).

**Это спецификация, не тикет.** Она уже согласована и **делать её не нужно** — работа разбита на пять частей, по одной на слой, каждая самодостаточна и проверяема:

| Часть | Файл | Что | Blocked by |
|---|---|---|---|
| 1 | `34_1_schema_devices_and_tag_states.md` | Схема: миграции обеих цепочек, переезд состояния Tag'а в `runtime.tag_states` | 32 |
| 2 | `34_2_domain_device_environment.md` | Домен: агрегаты Device, Environment, EnvironmentDevice, поставщик у Tag'а | 34_1 |
| 3 | `34_3_persistence_and_application.md` | Репозитории, сервисы, мост Engineering↔Runtime, проверка поставщика | 34_2 |
| 4 | `34_4_rest_devices_tokens_gateway_api.md` | REST: устройства, окружения, токены, API для Gateway, Bruno | 34_3 |
| 5 | `34_5_simulator_as_device_gateway.md` | `devicesim` и `cmd/device_simulator`: токен, окружение, коды выхода | 34_4 |

Части идут по порядку; каждая ссылается на эту спецификацию как на источник истины.

**Задача — только часть A** (устройства, токены, окружения, запись в Runtime): согласована, готова к реализации. Отладка мнемосхем в Project-режиме на живых данных — **задача 65**, не решается здесь.

## Часть A. Согласованная модель

### Сущности (Engineering-узел, черновик проекта)

- **Device** — агрегат Engineering-ноды: `engineering.devices(id, project_id, name, comment, protocol, config jsonb, version)`. Имя уникально в проекте (ADR 0003), свой Version (мутации ждут ожидаемую версию), свои события `device_created` / `device_updated` / `device_deleted` (ContentEvent → проектный SSE-поток). `protocol` — enum-строка (`simulation` на старте), `config` — JSON под протокол (для `simulation` валидируется типизированно). Описательная метаинформация (комментарий), например пометка «это имитатор»; на Quality не влияет (задача 13).
- **Environment** — агрегат Engineering-ноды: `engineering.environments(id, project_id, name, comment, version)`. Имя уникально в проекте. Окружение — **набор параметров связи** для Device'ов проекта (адрес, порт, учётные данные); оно **не** выбирается Deploy'ом. Симулятор — просто одно из окружений.
- **Environment Device** — агрегат Engineering-ноды: `engineering.environment_devices(id, environment_id, device_id, params jsonb, version)`. Идентифицируется парой `(Environment, Device)`; несёт **параметры связи** (JSON под протокол).
- **Куда что стоит**: что это (протокол, как извлекать) — в `Device`; где это (адрес, учётные данные) — в `Environment Device`.

### Связь с Tag и правило одного поставщика

- `engineering.tags.device_id UUID NULL` + FK на `engineering.devices`. У Tag не больше одного поставщика значения: один Device (позже — одна Computation, задача 56) или никто. Два Device, пишущих один Tag, — ошибка валидации Draft (и отказ Deploy, задача 35).
- **Копия в Runtime**: `runtime.tags.device_id` — часть копии определения, рядом с `type`. Runtime-нода не читает `engineering.*` (ADR 0011), поэтому связь обязана быть в её схеме; проверка записи попадает в существующий один UPDATE.

### Структура runtime-схемы (пересмотр ADR 0011)

- Runtime **копирует** таблицы Engineering; runtime-специфичные данные лежат в **отдельных таблицах**, ссылающихся на копии по ID:
  - `runtime.tags` — копия определения (`name`, `type`, `version`, `device_id`); `runtime.tag_states(tag_id → runtime.tags.id, value, quality, seq)` — состояние; `runtime.tag_state_seq` — своя последовательность. **Определение и состояние больше не в одной таблице** (сейчас — в `runtime.tags`).
  - `runtime.devices` — копия `engineering.devices`; `runtime.environment_devices` — копия `engineering.environment_devices` **без токенов**.
  - `runtime.device_tokens` — боевые токены.
- **Следствие**: мост «`UpsertTagDefinitions` по событию `tag_created`/`tag_deleted`» (ADR 0011, п. 7) заменяется Deploy'ом — задача 35; до неё копия наполняется мостом, как сейчас, но **структура таблиц — уже новая**: мост должен удалять и копию определения, и состояние.

### Device Token

- **Один токен на Device**. Выпускается на Runtime-узле, хранится **только хешем** (sha-256), показывается **один раз** при выпуске. Отзыв и ротация — только явно. В UI видны «осиротевшие» токены (Device, которой больше нет).
- Передаётся как `Authorization: Bearer gsc_dev_…` (префикс отличает device-токен от будущего пользовательского bearer'а, задача 50). Токен определяет Device, а значит **проект** (CONTEXT.md).
- **В ревизию и ProjectFile токен не едет** — только его отпечаток.
- Токен **переживает Deploy и Rollback**. Спит токен, чей Device **нет в текущей ревизии**: запросы отклоняются (403 — Gateway ждёт), но токен не отзывается.

### Запись: только Runtime

- **Устройство пишет только в Runtime** — `devicelink.Link` не меняется, ADR 0004 цела. Черновикового контура для устройств **нет**.
- **Runtime** (`runtime.tag_states`): настоящее состояние, пишет **только поставщик** Tag'а (его Device). Человеку без прав на Tag с поставщиком — 403.
- `GET /projects/{id}/tags` отдаёт боевые состояния (сегодня — так и есть).

### Авторизация (на уровне application)

- `application/runtime` (Runtime-нода) получает сервис токенов: токен → `device_id`; проверка кэшируется в памяти (токен меняется только при отзыве/ротации).
- `SetTagValue` получает `device_id` писателя и проверяет его против `runtime.tags.device_id` **в том же UPDATE**, что и живость проекта и тип Tag'а. Решение принимает application; REST-слой только разбирает заголовок.
- Ошибки: **401** — токен неизвестен/отозван (Gateway умирает); **403** — токен есть, но писать нельзя (чужой Tag, или Device нет в ревизии); **404** — Tag/проект не найден.

### API для Gateway

- `GET /api/v0/device/config` — конфигурация Gateway'а: `{device: {id, name, protocol, config}, environment: {id, name}, connection: {params}, tags: [{id, name, type}]}`. Токен определяет Device, окружение — **параметр запроса** (`?environment=<id>`): Gateway знает, к какому подключению он относится.
- `GET /api/v0/device/events` — SSE **для Gateway**, аутентифицированный токеном (сервер определяет проект по токену). События: конфигурация Device изменилась (с payload), токен отозван/ротирован. Gateway переспрашивает конфигурацию по таймеру как fallback.
- До задачи 35 конфигурация отдаётся из Draft (текущих таблиц), после — из текущей Revision.

### Управление Device, Environment и токенами (REST, для инженера)

Содержимое проекта — под `apiv0.ProjectPath(id)` (ADR 0006); новые ресурсы — по образцу существующих (у каждого свой `resource` в `restapi/errors.go` с `name` и `pathParam`, свой файл обработчика, строки в `routes()` и в `requestBodies` теста `TestBrunoCollection`).

- `GET /projects/{id}/devices`, `GET …/devices/{device_id}`, `POST …/devices`, `PUT …/devices/{device_id}`, `DELETE …/devices/{device_id}` — CRUD Device. `PUT` несёт `version` (ожидаемая версия, ADR 0011/оптимистическая блокировка), как `UpdateProjectRequest`.
- `GET /projects/{id}/environments`, `GET …/environments/{environment_id}`, `POST …/environments`, `PUT …/environments/{environment_id}`, `DELETE …/environments/{environment_id}` — CRUD Environment.
- `GET /projects/{id}/environments/{environment_id}/devices` — список Environment Device; `PUT …/environments/{environment_id}/devices/{device_id}` — параметры связи пары; `DELETE …` — удаление пары. Пара идентифицируется **двумя** параметрами пути (`environment_id`, `device_id`), как Library Pin идентифицируется библиотекой в проекте.
- **Токены**: `POST /projects/{id}/devices/{device_id}/token` — выпуск: ответ **201** с **самим токеном** (`{"token": "gsc_dev_…"}`), он показывается **один раз** и больше недоступен. `DELETE …/token` — отзыв. `POST …/token/rotate` — ротация (новый токен, старый отзывается). `GET …/token` — метаданные без секрета: `{"issued_at", "fingerprint", "device_id"}`.
  - Валидация: выпуск/отзыв/ротация — на **Runtime-узле** (там `runtime.device_tokens`). На сервере с обеими ролями это тот же процесс. На сервере только с ролью Engineering — **404** (управления токенами нет: токен выдаётся на Runtime-узле).
  - Ошибки: выпуск, когда токен уже есть и не отозван — **409**; отзыв несуществующего — **404**; ротация — всегда 201 с новым токеном.

### Device Gateway

- `devicesim` — первый Device Gateway. **Один Gateway обслуживает одну Device** (в одном окружении). Паттерны — параметры Device (протокол `simulation`).
- Конфиг симулятора: токен, окружение, что писать. Режима «черновик» нет — симулятор настоящий Gateway.
- **Коды выхода** (`cmd/device_simulator`): `401`/`403` → `ExitNoPerm` (77); прочие `4xx` **и** ошибки TLS/схемы URL → `ExitProtocol` (76); тег не найден / несовместимый паттерн → `ExitConfig` (78).

### Политика и Deploy

- Два Device, пишущих один Tag, — ошибка валидации Draft; отказ Deploy — задача 35.
- Deploy переносит в ревизию: определения Tag'ов с `device_id`, Device'ы, окружения с параметрами. **Без токенов.** Окружение Deploy **не выбирает**.
- Rollback (задача 36) токены не затрагивает: токен Device, вернувшейся в ревизию, снова работает.

### Прочее

- REST, Bruno (обе коллекции), тесты, `CHANGELOG.md`.
- **growctl-манифест Device и Environment — вне объёма этой задачи** (задача **67**): манифест требует новый `kind` в `contract/manifest/v0` (сейчас 0.4), свою `requireAPIVersion`-версию «Since» и bump SchemaVersion + `make schemas` — это работа над отдельным контрактом. Здесь — только REST, чтобы growctl мог быть написан потом.
- **UI — вне объёма этой задачи** (задача **66**): создание Device/Environment, редактирование параметров связи, выпуск/отзыв токена в PWA. Здесь — только API.

### Порядок работы и точки сборки

- **Домен**: `internal/server/domain/device` — агрегаты Device, Environment, EnvironmentDevice (по образцу `domain/librarypin`: идентифицируется парой, иммутабельный, свой Version, свои события). Добавить пакет в `engineeringRole` в `internal/server/roles_test.go` — иначе тест ролей не увидит его (список пакетов роли там задан явно).
- **Схема**: миграции — цепочка `engineering/` (новые таблицы, `engineering.tags.device_id`) и цепочка `runtime/` (`runtime.tag_states`, `runtime.device_tokens`, копии `runtime.devices`/`runtime.environment_devices`, `runtime.tags` без `value`/`quality` и с `device_id`). Goose: `goose -dir … create <name> sql` (команда из AGENTS.md).
- **Сборка сервисов** — три места, все три надо поправить:
  - `cmd/combined_server/main.go` — `NewTagService` (стр. ~189), `restapi.NewRouter(restapi.Services{…})` (стр. ~191), новый сервис токенов и `runtimeapp.NewTagStateService` (передача сервиса токенов в конструктор).
  - `internal/server/servertest/servertest.go` — то же (стр. ~133 и ~157): тесты REST идут против настоящего API.
  - `internal/server/interface/restapi/router.go` — `Services` (новое поле) и `NewRouter`.
- **Контракт**: `contract/api/v0` — DTO Device/Environment/EnvironmentDevice/Token и ответов `/device/config`; добавить в `Roots:` в `contract/api/v0/schema_test.go`; поднять SchemaVersion (новые поля и типы — MINOR, пока MAJOR=0) и выполнить `make schemas`.
- **События**: новый тип события = четыре правки: `internal/server/domain/event/event_type.go` (константа + `eventTypeNames`), `contract/api/v0/event.go` (константа + `EventTypes()`), `internal/server/interface/restapi/api_event.go` (`apiEventTypes`), и образец в `samples` в `restapi/api_event_internal_test.go` — иначе `TestNewAPIEvent_TranslatesEveryEventType` и `TestAPIEventTypes_UsesEveryContractEventType` упадут.
- **Симулятор**: `cmd/device_simulator` и `internal/devicesim` — конфиг получает токен и окружение вместо имени проекта; `devicelink.NewRESTLink` — токен вместо `projectName` (проект определяется токеном); коды выхода 77/76/78.

### Критерии приёмки

- `make build` и `make test` проходят (правило 3 AGENTS.md).
- `make bruno_test` проходит: у каждого нового маршрута есть запрос в коллекции `growscada_server` с проверками статуса, заголовка `GrowSCADA-Schema-Version`, `application/problem+json` на ошибке и ключевых полей на чтении; у каждого маршрута с телом — запись в `requestBodies` (правило 5).
- Ручная проверка в браузере не требуется — **UI вне объёма**; но `make db_up` + `make run` поднимаются, сервер стартует без ошибок (правило 7).
- Проверено вручную: устройство с токеном пишет значение Tag'а; запись Tag'а чужой Device отклоняется (403); неизвестный токен — 401; `GET /api/v0/device/config` отдаёт конфигурацию; симулятор завершается с кодом 77 на отозванный токен и с 76 на прочие 4xx.
- `CHANGELOG.md` дополнен.

### Что меняется в коде (по слоям)

- **runtime-схема**: `runtime.tags` теряет `value`/`quality` (копия определения + `device_id`); появляются `runtime.tag_states(tag_id → runtime.tags.id, value, quality, seq)` и `runtime.device_tokens(device_id → runtime.devices.id, token_hash, ...)`. Копии `runtime.devices`, `runtime.environment_devices`.
- **engineering-схема**: `engineering.devices`, `engineering.environments`, `engineering.environment_devices`; `engineering.tags.device_id`.
- **Репозиторий Runtime-ноды** (`postgres/runtime/tag_state_repository_postgres.go`): сегодня `runtime.tags` — одна строка на Tag (определение + состояние), и `Write` — это `UPDATE runtime.tags SET value…`. После разделения: `Write` — `UPDATE runtime.tag_states … FROM runtime.tags WHERE … AND t.device_id = $device_id` (проверка поставщика в том же запросе), `FindInLiveProject`/`FindByIDs` — `JOIN` двух таблиц, `UpsertDefinitions` — вставка копии определения **и** начального состояния в одной транзакции, `Delete`/`DeleteAllOfProject` — удаляют и копию, и состояние.
- **Домен `tagstate`**: `Definition` получает `DeviceID` (поставщик — часть определения); `Repository` получает параметр поставщика в `Write`.
- **`application/runtime`**: сервис токенов (токен → `device_id`, кэш в памяти); `SetTagValueInput` получает `DeviceID` писателя.
- **`application` (Engineering)**: `TagService` — новые операции с Device; `RuntimeNode`-порт — `TagDefinition` получает `DeviceID`.
- **REST**: `restapi` — сервис токенов в `Services`, middleware/разбор `Authorization`, новые маршруты для Device/Environment и `/device/config`, `/device/events`; `contract/api/v0` — DTO и SchemaVersion (bump + `make schemas`); Bruno — обе коллекции.
- **Роли** (`roles_test.go`): новый `domain/device` — в `engineeringRole`; код Runtime-ноды не должен содержать строк `engineering.…`, код Engineering-ноды — строк `runtime.…`.

## Часть B вынесена отдельно

Отладка мнемосхем в Project-режиме на живых данных — **задача 65** (`todo/backlog/65_debug_mnemonics_in_project_mode.md`), статус `needs-info`: инженер поработает в приложении и сформулирует её. Здесь она не решается.

## Comments

- 2026-10-04: бывшая задача 23. По итогам grilling: Device — конфигурация в проекте, а не активный процесс; исполнитель — Device Gateway; авторизация — Device Token на Device (вариант «токен на Gateway» и «конфиг локально у Gateway» отклонены). Регистрация runtime-тегов со стороны Device (бывший тикет 12 задачи 36) — в задаче 48.
- 2026-10-30..2026-11-01 (grilling): Device — отдельный агрегат Engineering-ноды; связь с Tag — `engineering.tags.device_id`; введена сущность **Environment** (набор параметров связи; симулятор — просто одно из окружений); **Runtime копирует таблицы Engineering, состояние — в отдельных таблицах** (`runtime.tag_states`), определение и состояние разделены; токен — `Authorization: Bearer gsc_dev_…`, хранится хешем, **один на Device**, выдаётся на Runtime-узле, **в ревизию не едет**; устройство пишет **только в Runtime** (`devicelink` не меняется, ADR 0004 цела); авторизация на уровне application — проверка «Tag принадлежит этой Device» в том же UPDATE; коды выхода симулятора разведены (77/76/78).
- 2026-11-01 (grilling, вторая половина): пересмотрено после уточнения требований — токен **не** на паре (Environment, Device), а на Device; **черновикового контура для устройств нет**; Deploy **не** выбирает окружение; подписка Runtime→Engineering через порт **не нужна** (поток идёт прямо в UI по SSE); ограничение «Engineering не может существовать без Runtime» **не вводится**. Выявлено: `tag_updated` уже доходит до Project-режима, `GET /tags` уже отдаёт боевые состояния — отладка на живых данных в Project-режиме технически возможна, не хватает отображения и хранения закреплённых значений.
- 2026-11-01 (grilling, финал): часть B вынесена в задачу 65 (статус `needs-info` — спецификацию формулирует инженер после работы в приложении). Дописано в часть A: REST для Device/Environment/EnvironmentDevice и **управление токенами** (выпуск/отзыв/ротация, токен показывается один раз); порядок работы и точки сборки (`main.go`, `servertest.go`, `restapi/router.go`, `roles_test.go`, `contract/api/v0`, четыре правки на новый тип события, goose-миграции обеих цепочек); критерии приёмки.
- 2026-11-01 (гриллинг, разбиение): задача разбита на пять частей по слоям — `34_1_schema_devices_and_tag_states`, `34_2_domain_device_environment`, `34_3_persistence_and_application`, `34_4_rest_devices_tokens_gateway_api`, `34_5_simulator_as_device_gateway`; каждая самодостаточна и ссылается на эту спецификацию. Границы объёма вынесены в отдельные задачи: **UI — задача 66**, **growctl-манифест — задача 67**.
