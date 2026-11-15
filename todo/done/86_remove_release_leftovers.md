# Остатки удалённого Release в коде и мёртвый `UpsertTagDefinitions`
Status: ready-for-agent
Priority: 1

Найдено при сверке backlog с кодом 2026-11-15, после цепочки 85. Поведение не меняется: это мёртвый метод порта, устаревшие комментарии и имена, которые остались от Release и от «позднего upsert» определений Tag'ов.

**Область — только Release.** Идентификаторы `Commit` переименовывает задача 88 (решение 2026-11-15: термин уходит целиком); здесь их не трогаем, чтобы задачи не пересекались. `Commit` в прозе — задача 87. Работа идёт в feature-ветке.

## Мёртвый `UpsertTagDefinitions`

- `runtimeapp.TagStateService.UpsertTagDefinitions` (`application/runtime/tag_state_service.go`, интерфейс и реализация) вызывается только тестами: `application/project_application_service_test.go` (имитация позднего upsert для проверки очистки Project) и тестами `tag_state_service_test.go`.
- В продакшене определения Tag'ов попадают в `runtime.tags` только при Deploy (`infrastructure/postgres/runtime/deployment_repository_postgres.go`); `CreateTag` и job `sync_runtime_tag_definitions` из задачи 52 больше не существуют.
- **Решение: удалить.** Убрать метод из `TagStateService`, его реализацию, тесты `TestUpsertTagDefinitions_*` и фейк `stateUpsertingAfterRemoval`; тест очистки Project переписать без имитации позднего upsert — такого пути в коде нет. Если задаче 52 понадобится проверка надгробия `runtime.removed_projects`, она добавит свой метод и тест.

## Устаревшие комментарии

- `application/project_cleanup_policy.go`, комментарий `Run` («So does it when a Tag state is upserted after the states were removed (a creation's call, a bridge job running late)») описывает путь, которого нет: убрать абзац, оставив «the Project stays until its content is gone».
- `application/library_application_service.go`, `DeleteLibraryByID`: «with its releases», «Its Draft and releases are removed later» — у Library больше нет Release (85_7): писать «its Draft and saved states».
- `application/deploy_application_service.go`: «Release takes no part in it» — заменить на формулировку про saved state («A Deploy takes a saved state, not the Draft»).
- `interface/ui/project/buttons.go`: «a Release's Deploy, a Commit's …» — из строки убрать Release.
- `interface/ui/project/project.go`, `interface/ui/project/commits.go`: комментарии «The window knows no Release», «the mode has no Release», «No Release is joined» — переформулировать без Release (например, «a Deployment from a file has no saved state»).
- `infrastructure/postgres/repositories/draft_repository_postgres.go`: «A Library's Discard puts the released names back… the released ones are inserted» — «released» здесь про прошлый Release, а не про saved state; писать «the names of the saved state».
- `domain/library/library.go`: «with an empty Draft and no release» — «with an empty Draft».
- `domain/draft/repository.go`: «what a successful Release of before task 85_7 left behind» — переформулировать через saved state/`clean_version`.
- `growctl/manifest.go`: «a saved state of the Library, not a Release» — переписать без Release: «a saved state of the Library».
- `domain/event/event_type.go`: номера 23, 39–41 с комментарием о том, что они были событиями Release, — **оставить**: это объяснение, почему номера не переиспользуются.
- `infrastructure/postgres/runtime/journal_codec.go`: `EntryTypeRevision` — **оставить**: это чтение старых Journal-записей (формат данных), а не термин модели.

## Устаревшие имена

- `domain/draft/commit_content.go`: `releasedWidgetTypeDocument` → `widgetTypeCopyDocument`, комментарий «the field names repeat the release tables» → «the field names are the stored form of a saved state». **Имена JSON-полей не менять**: это формат сохранённых документов.
- `domain/draft/session.go`: параметр `released` в `ReplaceWidgetTypes` → `copies`.
- `domain/library/widget_type.go`: параметр `released` в `RecreateWidgetType` → `copy`, локальную переменную тоже.
- Пройтись по `grep -rniE "release|revision|editlock"` в `internal/server/{domain,application,interface}` без тестов и решить каждый оставшийся случай; `Release()` у таймеров/`onMessage`, `releaseTouch`, `releasePointerCapture` и «after commit» про транзакцию БД — не про Release домена, их не трогать.

## Проверка

- `make build` и `make test` проходят; контракт и API не меняются, SchemaVersion не растёт, Bruno не меняется.
- `grep -rni release` по не-тестовому коду находит только `Release()` таймеров/потоков, `releaseTouch`, `releasePointerCapture` и намеренные объяснения в `event_type.go`/`journal_codec.go`.
- `grep -rn "UpsertTagDefinitions"` не находит ничего.

## Связанное

- Задача 85_7 — удаление Release; задача 52 — сетевые ноды и надгробие Project: нужный ей метод она добавит сама.
- `GLOSSARY.md` — saved state единственный термин, Commit — прежнее имя.
