# REST: устройства, окружения, токены и API для Gateway
Status: ready-for-agent
Blocked by: 34_3

Часть 4 задачи 34. Контракт, обработчики, аутентификация токенами, Bruno.

Источник истины — `todo/backlog/34_device_gateway.md` (часть A), разделы «Управление Device, Environment и токенами», «API для Gateway», «Авторизация» — это **спецификация**, а не отдельная работа: она уже согласована, её делать не нужно.

## Что сделать

### Контракт `contract/api/v0`

- DTO: `DeviceResponse`, `CreateDeviceRequest`, `UpdateDeviceRequest` (с `version`), `EnvironmentResponse`, `CreateEnvironmentRequest`, `UpdateEnvironmentRequest`, `EnvironmentDeviceResponse`, `UpdateEnvironmentDeviceRequest`, ответы списков, `DeviceTokenResponse` (секрет — только при выпуске/ротации), `DeviceConfigResponse` (для Gateway'а).
- Поднять `SchemaVersion()` (MAJOR=0 → только MINOR), выполнить `make schemas`. Контракт уже поднят до **0.7** в части 2 (типы событий Device/Environment/пары), поэтому часть 4 поднимает до **0.8** — не забыть и не пытаться переиспользовать 0.7.
- Добавить новые типы в `Roots:` в `contract/api/v0/schema_test.go` — иначе они не попадут в сгенерированную схему.

### Маршруты (`restapi`)

По образцу существующих ресурсов; у каждого — свой `resource` в `errors.go` (с `name` и `pathParam`), свой файл обработчика, строки в `routes()` и в `requestBodies` теста `TestBrunoCollection`.

- `GET/POST /projects/{id}/devices`, `GET/PUT/DELETE /projects/{id}/devices/{device_id}`.
- `GET/POST /projects/{id}/environments`, `GET/PUT/DELETE /projects/{id}/environments/{environment_id}`.
- `GET /projects/{id}/environments/{environment_id}/devices`, `PUT/DELETE …/devices/{device_id}` — пара идентифицируется двумя параметрами пути.
- **Токены**: `POST …/devices/{device_id}/token` (201, тело с самим токеном — показывается **один раз**), `DELETE …/token` (отзыв), `POST …/token/rotate` (новый токен, старый отзывается), `GET …/token` (метаданные без секрета).
- **Для Gateway'а**: `GET /api/v0/device/config?environment=<id>` — `{device, environment, connection, tags}`; `GET /api/v0/device/events` — SSE, аутентифицированный токеном.
- Ошибки в `errorMappings`: 404 (нет ресурса), 409 (имя занято, версия устарела, токен уже выпущен), 403 (писать нельзя).

### Аутентификация и авторизация

- Разбор `Authorization: Bearer gsc_dev_…` — в сервисе токенов (`application/runtime`, Runtime-нода): токен → `deviceID`; **хеш** (sha-256) сверяется с `runtime.device_tokens`; результат кэшируется в памяти (токен меняется только при отзыве/ротации — тогда кэш сбрасывается).
- **Правило префикса**: bearer без префикса `gsc_dev_` — **не** device-токен (пользовательская авторизация, задача 50) ⇒ 401.
- `SetTagValue` получает `deviceID` писателя (часть 3) ⇒ запись Tag'а чужой Device — **403**; неизвестный/отозванный токен — **401**; токен есть, Device нет в ревизии — **403**.
- **Управление токенами** — на Runtime-узле. На сервере только с ролью Engineering — **404** (управления нет: токен выдаётся на Runtime-узле).
- `Authorization` не должен попадать в CORS-preflight без нужды: заголовок `Authorization` — простой, preflight не нужен; существующий `corsMiddleware` проверить на `Access-Control-Allow-Headers`.

### События

Новые типы (по четыре правки — см. `todo/backlog/34_device_gateway.md`): `device_created`, `device_updated`, `device_deleted`, `environment_created`, `environment_updated`, `environment_deleted`, `environment_device_created`, `environment_device_updated`, `environment_device_deleted`. Они — `project.ContentEvent` ⇒ едут в проектный SSE-поток.

### Bruno

- Коллекция `tests/api/bruno_collections/growscada_server/`: запрос на каждый новый маршрут, с проверками статуса, заголовка `GrowSCADA-Schema-Version`, `application/problem+json` на ошибке и ключевых полей на чтении; порядок `seq` — чтения, затем изменения, удаления в конце.
- Коллекция `device_simulator/` — если меняется поведение симулятора (часть 5).

## Проверка

- `make build`, `make test`, `make bruno_test` проходят.
- `TestBrunoCollection` (`restapi`) проходит: каждый маршрут покрыт запросом, каждое тело декодируется строго.

## Вне объёма

- UI — отдельная задача. growctl-манифест — отдельная задача (свой контракт, `kind`, `requireAPIVersion`).
