# growscada [CHANGELOG](https://keepachangelog.com/en/1.1.0/)

## [0.0.10] - 2026-05-12

### Added

- `internal/application/appdto` — новый пакет с DTO уровня прикладного сервиса: `Tag`, `CreateTagInput`, `UpdateTagInput`; поля не имеют JSON-тегов, сериализация делегирована интерфейсному слою; фабричные функции `NewTag` и `NewTagList` конвертируют доменные агрегаты в DTO
- `internal/interface/restapi/restdto` — новый пакет с HTTP-DTO: `TagResponse`, `GetTagsResponse`, `CreateTagRequest`, `CreateTagResponse`, `UpdateTagRequest`, `UpdateTagResponse`; конвертеры `NewTagResponse`, `NewGetTagsResponse`, `NewCreateTagResponse`, `NewUpdateTagResponse`, `NewCreateTagInput`, `NewUpdateTagInput` изолируют маппинг между слоями

### Removed

- `internal/application/dto` — удалён единый DTO-пакет, смешивавший JSON-сериализацию и application-слой; его ответственность разделена между `appdto` и `restdto`

### Changed

- `TagService` — все методы переведены на `appdto`: `FindAllTags` возвращает `[]appdto.Tag` вместо `dto.FindAllTagsResponse`; `FindTagByID` возвращает `appdto.Tag` вместо `dto.Tag`; `CreateTag` принимает `appdto.CreateTagInput` вместо `dto.CreateTagRequest` и возвращает полный `appdto.Tag` вместо `dto.CreateTagResponse{ID}`; `DeleteTagByID` возвращает плоский `appdto.Tag` вместо `dto.DeleteTagResponse{Tag: dto.Tag}`; `SetTagValueByID` принимает `appdto.UpdateTagInput` вместо `dto.UpdateTagRequest` и возвращает `appdto.Tag` вместо `dto.UpdateTagResponse{Version}`
- `restapi/tags.go` — хендлеры переведены на `restdto`: каждый хендлер конвертирует результат сервиса через соответствующую фабрику `restdto.New*`; удалены промежуточные переменные для ответов с ошибками; `UpdateTagRequest` больше не содержит поле `ID` — идентификатор передаётся только через path-параметр и подставляется в `NewUpdateTagInput`
- Тесты `tag_application_service_test.go` и `tags_test.go` — обновлены импорты и типы: `dto.CreateTagRequest` → `appdto.CreateTagInput`, `dto.UpdateTagRequest` → `appdto.UpdateTagInput`; тип возврата `createTagViaService` изменён с `dto.CreateTagResponse` на `appdto.Tag`; декодирование HTTP-ответов использует `restdto.*` вместо `dto.*`
- Все doc-комментарии в `appdto` и `restdto` переведены на английский язык

## [0.0.9] - 2026-05-08

### Added

- `EventBus` — реализация интерфейса `event.EventBus` в `internal/domain/event/event_bus.go`: хранилище подписчиков `map[EventType][]EventHandler`, потокобезопасность через `sync.RWMutex`; при `Publish` список обработчиков копируется под `RLock` и вызывается без блокировки, что исключает дедлок при вызове `Subscribe` из обработчика
- `MQTTEventBus` — декоратор над `event.EventBus` в `internal/infrastructure/mqtt/event_bus_mqtt.go`; при `Publish` вызывает внутреннюю шину и публикует событие в MQTT-брокер с топиком `events/<event_type>`; `MQTTClient` — порт-интерфейс с методом `Publish(topic string, payload []byte) error`, позволяющий подключить любую MQTT-библиотеку без изменения кода декоратора
- `NewTagService` — добавлен параметр `anEventBus event.EventBus`; `tagServiceImpl` хранит ссылку на шину и публикует события после успешных мутирующих операций: `TagCreatedEvent` в `CreateTag`, `TagDeletedEvent` в `DeleteTagByID`, `TagUpdatedEvent` в `SetTagValueByID`
- Юнит-тесты пакета `event` (`internal/domain/event/event_test.go`) — 33 теста:
  - `EventTimestamp`: создание без опций (метка близка к `now`), с фиксированным временем (`EventTimestampWithTime`), `String`, `ParseEventTimestamp` (валидная строка RFC3339 / невалидная → ошибка), `MustParseEventTimestamp` (валидная / паника)
  - `EventType`: `NewEventType` для всех 4 строк, для неизвестной строки → ошибка, round-trip `String` → `NewEventType` для каждого значения
  - `EventBus`: вызов подписчика при совпадении типа, изоляция по типу (другие подписчики не вызываются), несколько подписчиков для одного типа вызываются все, `Publish` без подписчиков не паникует
  - `SystemReadyEvent`, `TagCreatedEvent`, `TagUpdatedEvent`, `TagDeletedEvent`: тип, `TagID`, метка времени (не нулевая), `String` (не пустая), опция `WithTimestamp`
- Интеграционные тесты прикладного сервиса — 6 новых тестов на публикацию событий через `bus.Subscribe`:
  - `TestCreateTag_Success_PublishesTagCreatedEvent` — проверка типа события и `TagID`
  - `TestCreateTag_InvalidRequest_NoEventPublished` — при ошибке до `Save` событие не публикуется
  - `TestDeleteTagByID_Success_PublishesTagDeletedEvent` — проверка типа события и `TagID`
  - `TestDeleteTagByID_NotFound_NoEventPublished`
  - `TestSetTagValueByID_Success_PublishesTagUpdatedEvent` — проверка типа события и `TagID`
  - `TestSetTagValueByID_InvalidRequest_NoEventPublished`

### Fixed

- `SetTagValueByID` публиковал `TagDeletedEvent` вместо `TagUpdatedEvent` — исправлено

### Changed

- `NewTagService` — сигнатура расширена вторым параметром `anEventBus event.EventBus`; тесты в `tag_application_service_test.go` и `restapi/tags_test.go` обновлены: теперь передают `event.NewEventBus()`; вспомогательная функция `newServiceWithBus()` заменила `newServiceWithSpy()` — шина берётся из пакета `event`, подписка через `bus.Subscribe`
- `TagType` — добавлено нулевое значение `TagTypeUnknown TagType = iota` первым в блоке констант; `String()` получил явный `case TagTypeUnknown`; `NewTagType` возвращает `TagTypeUnknown` вместо `-1` при неизвестной строке; `IsValid()` исключает `TagTypeUnknown`
- `TagQuality` — добавлено нулевое значение `TagQualityUnknown TagQuality = iota` первым в блоке констант; аналогичные правки в `String()`, `NewTagQuality` и `IsValid()`
- `EventType` — добавлено нулевое значение `EventTypeUnknown EventType = iota` первым в блоке констант; `String()` получил явный `case EventTypeUnknown`; `NewEventType` возвращает `EventTypeUnknown` вместо `0` при неизвестной строке
- `Mode` — добавлена константа `ModeUnknown Mode = ""` первой в блоке констант (документирует нулевое значение типа `string`)
- Тесты `tag_type_test.go` — `TagType(-1)` в String-кейсе заменён на `TagTypeUnknown`; out-of-range значение `TagType(99)` покрывает `default`; `TagTypeUnknown` добавлен в список невалидных в `IsValid`-тесте
- Тесты `tag_quality_test.go` — аналогичные правки: `TagQuality(-1)` → `TagQualityUnknown`, добавлен `TagQuality(99)` для `default`, `TagQualityUnknown` в список невалидных
- Тесты `event_test.go` — `TestNewEventType_UnknownString_ReturnsError` расширен проверкой возвращаемого `EventTypeUnknown`; добавлен тест `TestEventType_String_Unknown`

## [0.0.8] - 2026-05-03

### Changed

- `TagKind` переименован в `TagType` во всём проекте: тип, константы (`TagTypeString`, `TagTypeBoolean`, `TagTypeInteger`), конструктор `NewTagType`
- `tag_kind.go` → `tag_type.go`, `tag_kind_test.go` → `tag_type_test.go`
- Метод `Tag.Kind() TagKind` переименован в `Tag.Type() TagType` в доменном интерфейсе и реализации
- Колонка `kind` переименована в `type` в схеме БД: обновлены DDL, индексы (`idx_tags_type`, `idx_tags_name_type`) и constraint (`chk_tags_type`)
- Все SQL-запросы в репозитории обновлены: `kind` → `type` в INSERT, UPDATE, SELECT, RETURNING
- DTO: поле `Kind string json:"kind"` переименовано в `Type string json:"type"` в `Tag` и `CreateTagRequest`
- Тесты: имена функций (`InvalidKind` → `InvalidType`, `ByKind` → `ByType`, `ForKind` → `ForType`, `UnknownKind` → `UnknownType`), переменные (`kind` → `tagType`, `newKind` → `newType`) и строки в логах (`"Kind:"` → `"Type:"`) обновлены во всех пакетах

## [0.0.7] - 2026-05-03

### Changed

- `cmd/combined_server/main.go`: все вызовы `fmt.Printf`/`fmt.Println` заменены на `slog.Error`/`slog.Info` со структурированными key-value аргументами
- `slogAdapter` — добавлен адаптер, реализующий интерфейс `gracedown.Logger` (`Println(v ...any)`); внутренние сообщения `gracedown` теперь выводятся через `slog.Info`
- `gracedown.NewManager` — передаётся `WithLogger(slogAdapter{})`, `gracedown` больше не использует `fmt.Println` напрямую

## [0.0.6] - 2026-05-03

### Added

- `DeleteTagByID` — метод удаления тега по ID в `TagService`; возвращает удалённый тег в `DeleteTagResponse`
- `SetTagValueByID` — метод обновления значения и качества тега в `TagService`; возвращает новую версию в `UpdateTagResponse`
- `DeleteByID` — метод удаления тега в `TagRepository`; возвращает `(Tag, error)`; реализация использует `DELETE ... RETURNING id, name, kind, value, quality, version`, что исключает лишний `SELECT`
- REST-хэндлер `DeleteTagByID` (`DELETE /api/v1/tags/{id}`) — удаление тега; 200 с телом удалённого тега, 404 при отсутствии, 400 при невалидном UUID
- REST-хэндлер `PatchTagValue` (`PATCH /api/v1/tags/{id}/value`) — обновление значения и качества; 200 с новой версией, 404 при отсутствии, 400 при невалидном UUID или теле
- Маршруты `DELETE /api/v1/tags/{id}` и `PATCH /api/v1/tags/{id}/value` зарегистрированы в роутере
- Bruno-коллекция: запросы `DELETE /api/v1/tags/{{TAG_ID}}` и `PATCH /api/v1/tags/{{TAG_ID}}/value`
- Интеграционные тесты репозитория — 4 теста для `DeleteByID`:
  - `TestDeleteByID_ExistingTag_ReturnsDeletedTag` — проверка всех полей возвращённого тега
  - `TestDeleteByID_ExistingTag_TagIsRemovedFromDB` — тег отсутствует в БД после удаления
  - `TestDeleteByID_ExistingTag_OtherTagsAreUnaffected` — прочие теги не затрагиваются
  - `TestDeleteByID_NotFound_ReturnsErrTagNotFound` — возвращает `ErrTagNotFound`
- Интеграционные тесты прикладного сервиса — 8 тестов:
  - `TestDeleteTag_ExistingTag_ReturnsDeletedTag`, `TestDeleteTag_ExistingTag_TagIsRemovedFromDB`, `TestDeleteTag_NotFound_ReturnsWrappedErrTagNotFound`
  - `TestSetTagValueByID_ValidUpdate_ReturnsIncrementedVersion`, `TestSetTagValueByID_ValidUpdate_ValueAndQualityAreUpdated`, `TestSetTagValueByID_NotFound_ReturnsWrappedErrTagNotFound`, `TestSetTagValueByID_InvalidQuality_ReturnsError`, `TestSetTagValueByID_InvalidValueForKind_ReturnsError`
- Интеграционные тесты HTTP-хэндлеров — 9 тестов:
  - `TestDeleteTagByID_ExistingTag_Returns200WithDeletedTag`, `TestDeleteTagByID_ExistingTag_TagIsRemovedFromDB`, `TestDeleteTagByID_NotFound_Returns404WithProblemDetails`, `TestDeleteTagByID_InvalidUUID_Returns400WithProblemDetails`
  - `TestPatchTagValue_ValidUpdate_Returns200WithVersion`, `TestPatchTagValue_ValidUpdate_ValueAndQualityAreUpdated`, `TestPatchTagValue_NotFound_Returns404WithProblemDetails`, `TestPatchTagValue_InvalidUUID_Returns400WithProblemDetails`, `TestPatchTagValue_InvalidJSON_Returns400WithProblemDetails`

### Changed

- `Tag.UpdateValue` переименован в `Tag.SetValue` в доменном интерфейсе и реализации; тест-функции переименованы соответственно (`TestTag_SetValue_*`)
- `TagService.DeleteTag` переименован в `TagService.DeleteTagByID`
- `TagRepository.Delete` переименован в `TagRepository.DeleteByID`; сигнатура изменена с `error` на `(Tag, error)` — сервис делает один вызов репозитория вместо двух (`FindByID` + `Delete`)
- `Save` в PostgreSQL-репозитории: добавлена явная обработка ошибки `RowsAffected()` для INSERT и UPDATE (для postgres всегда `nil`)
- `.github/workflows/go.yaml`: путь сборки исправлен с `./cmd/combined/` на `./cmd/combined_server/` — директория была переименована

## [0.0.5] - 2026-05-01

### Added

- Интеграционные тесты прикладного сервиса `tag` (`internal/application/tag_application_service_test.go`) — 10 тестов:
  - `TestCreateTag_ValidIntegerTag_ReturnsID` — создание тега типа `integer`, проверка ненулевого UUID в ответе
  - `TestCreateTag_ValidStringTag_ReturnsID` — создание тега типа `string`
  - `TestCreateTag_ValidBooleanTag_ReturnsID` — создание тега типа `boolean`
  - `TestCreateTag_InvalidKind_ReturnsError` — невалидный kind возвращает ошибку
  - `TestCreateTag_InvalidQuality_ReturnsError` — невалидное quality возвращает ошибку
  - `TestCreateTag_InvalidValueForKind_ReturnsError` — значение несовместимо с типом (строка вместо числа) возвращает ошибку
  - `TestFindAllTags_EmptyDB_ReturnsEmptyList` — пустая БД возвращает пустой список
  - `TestFindAllTags_MultipleTags_ReturnsAll` — возвращает все сохранённые теги
  - `TestFindTagByID_ExistingTag_ReturnsTag` — возвращает тег с корректными полями в DTO
  - `TestFindTagByID_NotFound_ReturnsWrappedErrTagNotFound` — возвращает ошибку с обёрнутым `ErrTagNotFound`
- Интеграционные тесты HTTP-хэндлеров `tag` (`internal/interface/restapi/tags_test.go`) — 8 тестов; запросы проходят через `router.ServeMux().ServeHTTP()`, что корректно заполняет path-параметры:
  - `TestGetTags_EmptyDB_Returns200WithEmptyList` — пустая БД, статус 200, пустой массив тегов
  - `TestGetTags_WithTags_Returns200WithAll` — теги в БД, статус 200, все теги в ответе
  - `TestGetTagsByID_ExistingTag_Returns200WithTag` — существующий тег, статус 200, корректные поля
  - `TestGetTagsByID_NotFound_Returns404WithProblemDetails` — несуществующий UUID, статус 404, тело в формате RFC 9457
  - `TestGetTagsByID_InvalidUUID_Returns400WithProblemDetails` — невалидная строка в `{id}`, статус 400, тело RFC 9457
  - `TestPostTags_ValidBody_Returns201WithID` — корректный JSON, статус 201, ненулевой UUID в ответе
  - `TestPostTags_InvalidJSON_Returns400WithProblemDetails` — невалидный JSON, статус 400, тело RFC 9457
  - `TestPostTags_InvalidKind_Returns500WithProblemDetails` — невалидный kind, статус 500, тело RFC 9457
- Юнит-тесты `ProblemDetails` (`internal/interface/restapi/problems_test.go`) — 11 тестов:
  - `TestProblemDetails_MarshalJSON_*` — 4 теста: стандартные поля попадают в JSON, пустой `detail` опускается, extensions сериализуются, extension не перебивает стандартное поле
  - `TestProblemDetails_UnmarshalJSON_*` — 2 теста: стандартные поля парсятся, неизвестные поля попадают в `Extensions`
  - Фабрики: `TestNewBadRequest_*`, `TestNewNotFound_*`, `TestNewConflict_*`, `TestNewValidationError_*`, `TestNewInternalError_*` — проверка статуса, типа, полей и extensions

## [0.0.4] - 2026-04-30

### Added

- Интеграционные тесты для инфраструктурного репозитория `tag` (`internal/infrastructure/postgres/repositories/tag_repository_postgres_test.go`) — 8 тестов:
  - `TestNextID_ReturnsUniqueIDs` — генерирует уникальные идентификаторы
  - `TestSave_NewTag_InsertsSuccessfully` — INSERT новой записи без ошибок
  - `TestSave_DuplicateID_ReturnsError` — повторный INSERT с тем же ID возвращает ошибку
  - `TestSave_ExistingTag_UpdatesSuccessfully` — UPDATE существующей записи, проверка новых значений
  - `TestSave_StaleVersion_ReturnsError` — оптимистичная блокировка: UPDATE с несовпадающей версией возвращает ошибку
  - `TestFindByID_ExistingTag_ReturnsTag` — возвращает тег с корректными полями
  - `TestFindByID_NotFound_ReturnsErrTagNotFound` — возвращает `ErrTagNotFound` при отсутствии записи
  - `TestFindAll_EmptyDB_ReturnsEmptySlice` — возвращает пустой срез при пустой таблице
  - `TestFindAll_MultipleTags_ReturnsAll` — возвращает все сохранённые теги
- Зависимость `github.com/pressly/goose/v3 v3.27.1` — применение SQL-миграций в тестовом окружении
- Тесты используют `testcontainers-go/modules/postgres`: контейнер поднимается в `TestMain`, миграции из `internal/infrastructure/postgres/migrations/` применяются через goose

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
