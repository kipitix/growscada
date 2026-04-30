# growscada [CHANGELOG](https://keepachangelog.com/en/1.1.0/)

## [0.0.3] - 2026-04-30

### Added

- Юнит-тесты для доменного агрегата `tag` (`internal/domain/tag/`) — 9 файлов, 63 теста:
  - `tag_id_test.go` — генерация UUID, парсинг, паника для невалидных строк, roundtrip
  - `tag_name_test.go` — создание имени, `String()`
  - `tag_kind_test.go` — парсинг строк, `String()`, `IsValid()`
  - `tag_quality_test.go` — парсинг строк, `String()`, `IsValid()`
  - `tag_value_boolean_test.go` — конструктор из `string`/`bool`/`int`, ошибки, `Value()`, `String()`
  - `tag_value_integer_test.go` — конструктор из `string`/`bool`/`int`, ошибки парсинга, `Value()`, `String()`
  - `tag_value_string_test.go` — конструктор из `string`/`bool`/`int`, конвертации, `Value()`, `String()`
  - `tag_value_test.go` — фабрика `NewTagValue`: диспетчеризация по `TagKind`, неизвестный kind, несовместимый тип
  - `tag_test.go` — создание агрегата, все геттеры, `IncrementVersion`, `UpdateValue`
- `.github/workflows/go.yaml` — GitHub Actions CI: запуск тестов и сборка при push/PR в ветку `path`

### Changed

- `.gitverse/workflows/go.yaml` — исправлены три проблемы: триггер `workflow_dispatch` заменён на `push`/`pull_request` для ветки `path`; версия Go зафиксирована через `go-version-file: go.mod` вместо явного `1.23`; путь сборки `cmd/server/main.go` (несуществующий) исправлен на `./cmd/combined/`

## [0.0.2] - 2026-04-28

### Added

- `FindTagByID` — реализован метод получения тега по UUID в сервисном слое (`TagService`)
- `FindByID` — реализован метод в репозитории PostgreSQL: SQL-запрос по `id`, маппинг строки в доменные value objects, возврат `ErrTagNotFound` при отсутствии записи
- `ErrTagNotFound` — добавлена сентинель-ошибка в доменный пакет `tag`
- Bruno-коллекция: добавлен запрос `GET /api/v1/tags/{{TAG_ID}}`
- `emergencyExit` — вспомогательная функция для аварийного завершения с корректным shutdown всех компонентов

### Changed

- `TagService.FindTagByID` — тип параметра изменён с `int` на `tag.TagID`
- `gracedown` обновлён до версии `v0.0.0-20260423215449-38425123f880`; переход с `WaitForSignal` на `WaitForSignalAndShutdown`, коды выхода вынесены в пакет `gracedown`
- Порядок инициализации в `main.go`: инфраструктурные компоненты (БД) поднимаются раньше интерфейсных (HTTP-серверы); регистрация shutdown-хука для БД перенесена до `Ping()`, чтобы соединение закрывалось при аварийном завершении
- `sendJSONResponse` — сериализация тела теперь выполняется до отправки заголовков, что позволяет вернуть `500` при ошибке маршалинга
- `GetTags`, `GetTagsByID` — упрощены: ручной `json.Marshal` + `w.Write` заменены на `sendJSONResponse`
- `PostTags` — исправлен отсутствующий `return` после ошибки `CreateTag`; без него параллельно уходили ответы `500` и `201`
- `GetTagsByID` зарегистрирован в роутере: `GET /api/v1/tags/{id}`

## [0.0.1] - 2026-04-22

- Basic tag functionality
