# 88_2 — контракты, REST и события: saved_state
Status: needs-triage
Priority: 3
Blocked by: 88_1

Часть цепочки `88_saved_state_rename.md`; таблица соответствия — там.

## Объём

- `contract/api/v0`: `CommitPath`/`CommitLoadPath`/`CommitDeployPath`/`CommitLabelsPath` → `SavedState…`; пути `/commits` → `/saved-states`; DTO `CommitResponse`, `GetCommitsResponse`, `CommitContentCounts`, `PutLibraryImportRequest.Commit` → `SavedState`; поле `Event.Commit` → `Event.SavedState`; `commit_number` → `saved_state_number`; ошибка `commit_not_found` → `saved_state_not_found`.
- `contract/record/v0`: `Entry.CommitNumber` → `SavedStateNumber` (`saved_state_number`); кодек читает старое `commit_number` (история формата), новые записи пишут новое имя; SchemaVersion MINOR.
- `contract/project/v0`: `LibraryImport.Commit` → `SavedState` (`saved_state`); строгий декод старого `commit` — 400 `file_issues`; SchemaVersion MINOR.
- `contract/manifest/v0`: проверить `spec.libraryImports` — ключ не меняется (значение — номер); bump только если появится поле.
- REST (`interface/restapi`): хендлеры, `routes()`, `apiEventTypes`, samples; старые маршруты `/commits…` — 410 с указанием `/saved-states…` (образец `removed_routes.go`), удалить с API 1.0.
- События: `commit_created`/`commit_title_updated` → `saved_state_created`/`saved_state_title_updated`; номера типов 23/39–41 не переиспользовать; `eventlog.ReloadTargetsFor` и UI-маппинг.
- `apiclient`: типы, методы, пути, e2e-тесты; `devicelink`/`devicesim` не затрагиваются (они не знают сохранённых состояний).
- `make schemas`, заморозка старых схем, примечание: после 1.0 такое удаление — MAJOR.
- Миграция JSONB документов: `commits.content` (ProjectFile с `library_imports[].commit`), `runtime.deployments.config`, Journal и Checkpoint — переписать на `saved_state` или читать оба имени кодеком; старые записи не терять.
- Bruno: переименовать запросы `…/commits…`, `requestBodies`, seq; проверить 410-запросы старых путей.

## Проверка

- `make build`, `make test`; статическая сверка Bruno; старый маршрут отвечает 410 `application/problem+json`.
- Старые ProjectFile (`library_imports[].commit`) отклоняются 400, новые проходят; тест миграции форматов.
- `apiclient` e2e против настоящего API.

## Связанное

- Спецификация — `88_saved_state_rename.md`; задача 62 — образец 410-заглушек.
