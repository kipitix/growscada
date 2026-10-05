# Симулятор устройства нижнего уровня

`Device` (агрегат) в реализации не требуется — симулятор пишет значения тегов через уже существующий `PATCH /tags/{id}/value`, без токенной авторизации на этом этапе. Когда агрегат `Device` и авторизация появятся (задача 34), симулятор станет первым клиентом, который начнёт использовать токены.

- Имитируется не протокол (Modbus/OPC-UA/MQTT), а сам факт поставки данных нижним уровнем: значения генерируются по паттернам.
- Один процесс = одно виртуальное устройство; несколько устройств — несколько экземпляров процесса.
- Теги сам не создаёт и не удаляет — это ответственность основного приложения / `growctl`.

## Раскладка пакетов (первый коммит ветки)

Чисто механический перенос, без изменения поведения: `internal/` раскладывается по приложениям.

```
internal/
  server/        domain/ application/ infrastructure/ interface/  (сервер + WASM-UI, как было)
  apiclient/     Go-клиент REST API сервера (вынесен из growctl), на server/.../restdto
  growctl/       CLI (на apiclient)
  devicelink/    контракт поставки данных Device → сервер
  devicesim/     симулятор
cmd/
  combined_server/ growctl/ device_simulator/
```

- `restdto` остаётся в `internal/server/interface/restapi/restdto`, `apiclient` импортирует его.
- Обновить пути в AGENTS.md, docker-compose (миграции, сиды), тестах с `runtime.Caller`.

## Контракт поставки данных (`internal/devicelink`)

- Референс для будущих протокольных адаптеров: они пишут значения тегов тем же способом. Семантика фиксируется в ADR 0004.
- Интерфейс `devicelink.Link`: `Resolve(ctx, name)` → тег (ID, тип, версия); `Write(ctx, tag, value, quality)`. Тип и качество — собственные перечисления `devicelink.TagType` / `devicelink.Quality` (строки REST API, без зависимости от `server/domain`).
- Реализация `devicelink.RESTLink` на `apiclient`:
  - теги только по имени (ADR 0003), `GET /api/v1/tags?name=`;
  - сама ведёт `version`, `PATCH /api/v1/tags/{id}/value`;
  - на `409` перечитывает тег (`GET /tags/{id}`) и повторяет запись один раз; повторная неудача — warn, ждать следующего тика;
  - `404` в работе — warn, тег выбывает из генерации.

## Паттерны

- `constant` (`value`), `ramp` (`start`, `rate` ед/с, опц. `max` → снова `start`), `sine` (`offset`, `amplitude`, `period`), `random_walk` (`start`, `step` — ± макс. за тик, `min`, `max`), `step` (`values` циклически, `hold`).
- Чистые функции времени с момента установки паттерна (random_walk — шаг на тик); числа float64, для integer округляются при записи. Округление касается только генерируемых значений (`ramp`, `sine`, `random_walk`): литералы `constant`/`step` для integer-тега должны быть целыми (`42.5` или `7.0` — ошибка конфига / `400`), дробное там скорее опечатка.
- Матрица совместимости: integer — все пять; boolean — `constant`, `step`; string — `constant`, `step`. Несовместимая пара — ошибка конфига при старте и `400` в control-API.
- Интервал на тег (`interval`, по умолчанию `defaultInterval`); запись каждый тик, даже если значение не изменилось.

## Конфиг

YAML, `KnownFields` (неизвестные поля — ошибка). Флаги `--config`, `--server` (env `DEVSIM_SERVER`) перекрывают конфиг.

```yaml
server: http://localhost:9090
control: :9191
autostart: true
defaultInterval: 1s
tags:
  - tag: sim.pump1.speed
    interval: 500ms        # опц.
    quality: good          # опц., default good
    pattern:
      kind: sine
      offset: 50
      amplitude: 20
      period: 10s
```

## Старт

- Control-API поднимается сразу, до подключения к серверу. Сетевые ошибки / 5xx при резолве — повтор с паузой без ограничения по времени, как у настоящего устройства. Пока симулятор подключается, `GET /api/v1/device` отдаёт `"state": "connecting"`, start/stop принимаются, а запросы к тегам (pattern, quality) — `503`: тип тега ещё неизвестен.
- Тег не найден или тип несовместим с паттерном — ошибка и выход с кодом ≠ 0, ничего не записав.

## Control-API

- `POST /api/v1/device/start`, `POST /api/v1/device/stop` — пауза: процесс и API живы, время паттернов продолжается с места остановки; идемпотентны. `stop` отвечает, когда записи, уже ушедшие на сервер, завершены: после ответа устройство молчит. `autostart` (default true).
- `GET /api/v1/device` — состояние устройства и тегов.
- `POST /api/v1/device/tags/{name}/pattern` — тело = JSON объекта `pattern`; смена на лету.
- `POST /api/v1/device/tags/{name}/quality` — `{"quality":"bad"}`, липкое переопределение, сразу пишется на сервер (если running); `DELETE /api/v1/device/tags/{name}/quality` — вернуть качество из конфига.
- Ошибки — RFC 7807 Problem Details.

## Сборка

- `cmd/device_simulator/`, таргет `make build_simulator` → `bin/growscada_device_simulator/`. `make build` объединяет `build_server`, `build_growctl` и `build_simulator`.
- Пример конфига `tests/device_simulator/example.yaml` под теги `tests/manifests/example_tags.yaml`.

## Тесты

Всё — обычные Go-тесты в `make test` (нужен только Docker для testcontainers).

- unit: паттерны, конфиг, устройство (start/stop, качество, смена паттерна, control-API через `httptest`) с фейковым `devicelink.Link`.
- интеграционный: `RESTLink` против настоящего restapi-роутера (`httptest`) + Postgres (testcontainers): резолв, версии, 409 → повтор.
- e2e: симулятор целиком (конфиг + control-API) против того же стенда; короткие интервалы и ожидание до дедлайна, без фиксированных `sleep`.
- Bruno — только ручные запросы к control-API: отдельная коллекция `tests/api/bruno_collections/device_simulator/`.

## CI

- GitHub workflow: `make build` собирает все бинарники (сервер, wasm, growctl, симулятор). GitVerse использует ту же конфигурацию GitHub.

## Сделано сверх плана

- Флаг `--control` (адрес control-API) — рядом с `--config` / `--server`, чтобы запускать несколько симуляторов.
- `tests/device_simulator/seed_dashboard.yaml` и таргеты `make run_simulator` / `make run_simulator_dashboard` — демо на seed-сцене «Main Dashboard».
- Коллекция `tests/api/bruno_collections/growscada/` переименована в `growscada_server/`, симметрично `device_simulator/`.
- `internal/server/servertest` — общий стенд REST API + Postgres (testcontainers, ленивый старт) для тестов devicelink, devicesim и growctl.
- `apiclient.FindTagByName` сверяет имя и на клиенте: сервер без фильтра `?name=` вернул бы все теги.
- В статусе control-API у тега есть `overridden` (качество переопределено) и `lost` (тег удалён на сервере); каждый успешный вызов control-API отвечает статусом.
