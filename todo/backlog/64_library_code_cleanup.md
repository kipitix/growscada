# Чистка кода после задачи 33
Status: needs-triage

Появилось при ревью задачи 33 (находки №10–13). Это не ошибки в поведении (кроме недетерминированного сообщения growctl), а места, где код легко разойдётся при следующих изменениях.

- **Порядок проверки обязательных полей в growctl** (`internal/growctl/manifest.go`, `validateWidgetType`, и `spec.libraryPins` в `validateProject`). Обязательные поля проверяются обходом map, порядок которого в Go случаен. Если в документе нет ни `html`, ни `width`, growctl сообщает то одно, то другое, и тесты на текст ошибки нестабильны. Проверять в фиксированном порядке (срез полей) и, лучше, сообщать обо всех пропущенных полях сразу.
- **`library.WithNumber`** (`domain/library/library_release.go`) используется только в тесте репозитория. По комментарию она нужна для повтора при гонке за номер, но `LibraryService.Release` собирает релиз заново, а сама функция обходит проверку `DraftDiffers`. Удалить её; в тесте собрать релиз нужного номера через `CreateRelease` поверх последнего.
- **Дубль CAS в репозитории закреплений** (`postgres/repositories/library_pin_repository_postgres.go`). `Save` и `Delete` заново реализуют шаг версии (Initial→Committed, Next) и транзакцию из `aggregateStore.save`/`delete`, хотя у `aggregateStore` есть хук `lock` как раз для такой блокировки. Перевести закрепления на `aggregateStore`, передав `lockForChange` хуком, чтобы изменения CAS и классификации конфликтов доходили и до них.
- **Разбор версии клиента в шести местах** (`application/`: `libraryVersion` в `LibraryService`, inline в `LibraryPinService`, `WidgetTypeService`, `ProjectService`, `SceneService`). Правило одно: «отрицательная версия — это ошибка ввода клиента (400), а не конфликт». Свести к обобщённой `clientVersion[T](raw int) (version.Version[T], error)` с одной формулировкой ошибки.
- `make build`, `make test`, `make bruno_test` проходят без изменений в API.
