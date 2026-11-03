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

## Comments

- 2026-11-03 (реализация): контракт **0.8** — DTO Device/Environment/EnvironmentDevice/DeviceToken/DeviceConfig и `JSONObject` (JSON-объект, сохраняемый как есть) в `contract/api/v0`; новые типы в `Roots:`, `make schemas`. Маршруты: CRUD устройства и окружений, пары `(Environment, Device)`, токены (`POST`/`GET`/`DELETE …/token`, `POST …/token/rotate`), `GET /device/config`, SSE `GET /device/events`; у каждого свой `resource` в `errors.go`, `routes()` и `requestBodies`. Токены — на Runtime-узле: `domain/devicetoken` (генерация `gsc_dev_…`, SHA-256, отпечаток, разбор префикса), `application/runtime.DeviceTokenService` (один токен на Device, выпуск/ротация/отзыв, кэш в памяти со сбросом при изменении), `postgres/runtime` (хеш и отпечаток). `PATCH …/tags/{id}/value` с device-токеном передаёт `device_id` писателя в `SetTagValue`: чужой Tag — 403, неизвестный/отозванный токен — 401, токен другого проекта — 403; без `Authorization` пишут люди. `DeviceConfigService` отдаёт конфигурацию из Draft (до Deploy) — токен спящей Device получает 403. Ошибки: 403 (`tag.ErrTagNotSuppliedByDevice`), 409 занятости (`device.ErrDeviceInUse`/`ErrEnvironmentInUse`), 404/409 токена; `NewUnauthorized`/`NewForbidden`, `Authorization` в CORS. Bruno — запрос на каждый маршрут и seed-фикстура (Device `sim`, Environment `bench`, пара, Tag `device.speed`, токен; Device `sim-2` без токена). `make build`, `make test`, `make bruno_test` проходят. UI и growctl — вне объёма, как и договаривались.
- 2026-11-03 (решение по схеме): у `runtime.device_tokens` появилась колонка `project_id` (новая миграция `20261103000000`, FK на `projects` с CASCADE). Токен называет проект без чтения `runtime.devices` (копии пока пусты до Deploy) и переживает Deploy/Rollback; хеш ищется по-прежнему по уникальному индексу.
- 2026-11-03 (решение по `/device/events`): поток авторизуется токеном и несёт **события конфигурации** проекта (`device_*`, `environment_*`, `environment_device_*`, события проекта), а не `tag_updated` — Gateway не должен получать поток значений. Отзыв/ротация токена отдельного события в контракте не получает: Gateway узнаёт о них по 401 на следующем запросе (и переподключается к SSE — это делает часть 5). Новых типов событий не заводили: девять `device_*`/`environment*` уже есть с части 2.
- 2026-11-03 (решение по валидации токена): выпуск/отзыв/ротация проверяют, что Device есть в проекте, через Engineering-сервис (маршрут называет проект и Device); сам сервис токенов проекта не знает, кроме `project_id` в записи токена. На сервере без Runtime-узла управление токенами отвечает 404 (`Services.DeviceTokens == nil`).
