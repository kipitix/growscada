# Чистка кода после задачи 33
Status: needs-triage
Priority: 3

Появилось при ревью задачи 33 (находки №10–13). Это не ошибки в поведении (кроме недетерминированного сообщения growctl), а места, где код легко разойдётся при следующих изменениях.

## Что осталось

- **Порядок проверки обязательных полей в growctl** (`internal/growctl/manifest.go`): `validateWidgetType` обходит map при проверке `html`/`script`/`width`/`height`, а `validateProject` — `spec.libraryImports`; порядок в Go случаен. Если в документе нет двух полей, growctl сообщает то одно, то другое, и тесты на текст ошибки нестабильны. Проверять в фиксированном порядке (срез полей) и, лучше, сообщать обо всех пропущенных сразу.
- **Разбор версии клиента в четырёх местах** (`application/`): `libraryVersion` в `LibraryService`, inline в `TagService` (`draftVersion`), `ProjectService` и `runtime/tag_state_service.go`. Правило одно: «отрицательная версия — это ошибка ввода клиента (400), а не конфликт». Свести к обобщённой `clientVersion[T](raw int) (version.Version[T], error)` с одной формулировкой ошибки.
- `make build`, `make test`, `make bruno_test` проходят без изменений в API.

## Закрыто до этой задачи

- `library.WithNumber` и `library_release.go` удалены задачей 85_7; `DraftDiffers` и Release-путь тоже.
- CAS в репозитории закреплений: `library_pin_repository_postgres.go` больше нет — импорты стали сущностями Draft'а (`library_import_repository_postgres.go` только читает), поэтому дубль CAS исчез вместе с пинами.
- `spec.libraryPins` переименован в `spec.libraryImports` (85_6), проверка обязательных полей у него осталась.
- N+1 в `LibraryService.describeAll` (`FindLastCommit` + чтение Draft'а на каждую библиотеку) передан в задачу 63: это поведение `GET /libraries` и событий, а не чистка growctl.

## Comments

- 2026-11-07: `WithNumber`, `DraftDiffers` и Release-путь меняются в 72_2/72_5 (Release из saved state). Выполнять после 72_5, иначе чистка разойдётся с новой моделью.
- 2026-11-15: сверено с кодом. Задачи 72_5 и 85_7 закрыты: два пункта из четырёх отпали (см. «Закрыто»), разбор версии сократился с шести мест до четырёх, дубль CAS ушёл вместе с пинами. Остались порядок полей в growctl и разбор версии; N+1 по решению от 2026-11-15 живёт в 63.

