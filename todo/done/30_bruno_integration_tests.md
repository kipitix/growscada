# Интеграционные тесты приложения через bruno
Status: ready-for-agent

Цель — проверять, что Bruno-коллекции (`tests/api/bruno_collections/`) не отстали от API. Основными тестами остаются Go-тесты. Проверок две: статическая сверка коллекций с кодом (`make test`) и прогон коллекций против собранного приложения в контейнерах (`make bruno_test`).

## Статическая сверка коллекций (Go, `make test`)

- Маршруты `restapi/router.go` и `devicesim/control.go` собрать в неэкспортируемую таблицу `routes() []route{pattern, handler}`; `ServeMux` регистрирует их циклом. Паттерны и поведение не меняются.
- Новый пакет `internal/brunotest` (только для тестов): читает `*.yml` коллекции (кроме `opencollection.yml` и `environments/`), подставляет `{{VAR}}` из `environments/localhost.yml`, возвращает `[]Request{File, Method, URL, Body, Tags}`.
- Internal-тест в `restapi` для коллекции `growscada_server` и в `devicesim` для коллекции `device_simulator` проверяет:
  - каждый запрос коллекции попадает в маршрут таблицы (`serveMux.Handler(req)` возвращает pattern); не попал — ошибка с именем файла;
  - каждый маршрут таблицы покрыт хотя бы одним запросом; не покрыт — ошибка с паттерном;
  - тело запроса декодируется `decodeStrict` (тем же, что `decodeRequest`) в тип из map `requestBodies` (pattern → тип, лежит в тесте). У маршрута POST/PUT/PATCH нет записи в map — ошибка «добавь тип тела»; у маршрутов без тела (`POST /control/device/start`) в map явно стоит `nil`.
- Запрос с тегом Bruno `invalid-body` (`post_tags_newer_client`, `post_tags_invalid_quality` и т.п.) пропускает только проверку тела; маршрут он покрывать обязан.
- Тест проходит без Docker и поэтому идёт и на GitVerse.

## Прогон коллекций (`bru run`)

- В каждом запросе обеих коллекций — декларативные assert'ы: ожидаемый `res.status`; наличие заголовка `GrowSCADA-Schema-Version` (для сервера); у ошибок `content-type` `application/problem+json`; у чтений — одно-два ключевых поля (`id`, `name`). Полную форму тел проверяют Go-тесты и схемы.
- Данные: каждый прогон идёт на свежей БД с миграциями и seed (как `make db_up`), ID берутся из `environments/localhost.yml`, как сейчас. Порядок задаётся `seq`: изменения после чтений, удаления в конце; при необходимости переупорядочить `seq`.
- `get_events` (SSE не завершается) — тег `manual`, прогон идёт с `--exclude-tags=manual`. Маршрут покрыт статической сверкой.
- Симулятор в прогоне работает с `tests/device_simulator/seed_dashboard.yaml`; в `device_simulator/environments/localhost.yml` `TAG_NAME` меняется на seed-тег `max_connections`. Окружение одно — `localhost`: `bru` ходит с хоста на проброшенные порты, отдельный ci env не нужен.

## Сборка в контейнере

- Multi-stage `Dockerfile`: стадия golang собирает `app.wasm` и бинарник сервера (как `make build_server`); финальная стадия (distroless/alpine) содержит `growscada_combined_server` и `web/`. Отдельный target того же Dockerfile — симулятор.
- `tests/api/docker-compose.yaml`: db (postgres:18), migrations и seed (goose-контейнеры, как в `tools/debug_db`), server (`DATABASE_URL` на хост `db`), simulator (`DEVSIM_SERVER=http://server:9090`, `--config seed_dashboard.yaml`). Без named volume — БД всегда свежая. Порты 9090 и 9191 проброшены на хост; healthcheck'и, чтобы работал `--wait`.
- `make bruno_test`: `docker compose up --build --wait` → `npx @usebruno/cli@<зафиксированная версия> run` обеих коллекций (`--env localhost --exclude-tags=manual`) → `docker compose down -v`, в том числе при падении (код выхода — от `bru`).

## Конфигурация сервера

- DSN: поле `serverArgs` с env `DATABASE_URL`, по умолчанию нынешний `postgres://growscada:growscada@localhost:5432/growscada?sslmode=disable`.
- Адрес API для UI: поле `serverArgs` с env `PUBLIC_API_URL`, передаётся в `app.Handler.Env` (ключ `API_URL`). В WASM константу `apiServerURL` заменяет `apiURL()`: `app.Getenv("API_URL")`, а если пусто — `scheme://hostname страницы:9090`. Тогда один образ работает с любым адресом без пересборки, а `make run` локально — как раньше.
- Порты (8080 PWA, 9090 API) и CORS `*` остаются. Перевод на один origin в эту задачу не входит.

## CI

- GitHub (`.github/workflows/`): отдельный job — `make bruno_test` (Docker и Node есть на раннере).
- GitVerse (`.gitverse/workflows/go.yaml`): прогона нет — на раннерах нет Docker; статическая сверка идёт в составе `go test`. Причину объяснить комментарием в workflow.

## Документация

- `AGENTS.md`: в таблицу Makefile добавить `make bruno_test`; в разделе «API Collections» вместо «Automated tests are Go tests only» описать обе проверки и теги `manual` / `invalid-body`; в правило 5 дописать: у нового запроса должны быть assert'ы, порядок `seq` соблюдается (удаления в конце), новому маршруту с телом нужна запись в `requestBodies`.
- ADR не нужен (решения легко обратимы); `CONTEXT.md` не меняется (новых доменных терминов нет).
