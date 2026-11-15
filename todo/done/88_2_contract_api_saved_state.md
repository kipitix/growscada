# 88_2 — контракты, REST и события: saved_state
Status: ready-for-agent
Priority: 1
Blocked by: 88_1

Вторая часть цепочки `88_saved_state_rename.md`; таблица соответствия — там. Самая большая часть: контракт, REST, `apiclient` и все вызовы меняются одним коммитом, иначе сборка красная.

## Объём

- `contract/api/v0`: `CommitPath`/`CommitLoadPath`/`CommitDeployPath`/`CommitLabelsPath` → `SavedState…`; пути `/commits` → `/saved-states`; `CommitResponse` → `SavedStateResponse`, `GetCommitsResponse` → `GetSavedStatesResponse`, `CommitContentCounts` → `SavedStateContentCounts`, `PutLibraryImportRequest.Commit` → `SavedState`; `Event.Commit` → `Event.SavedState` (номер saved state); `commit_number` → `saved_state_number`; `commit_not_found` → `saved_state_not_found`; константы событий `EventCommitCreated`/`EventCommitTitleUpdated` → `EventSavedStateCreated`/`EventSavedStateTitleUpdated`; строки `commit_created`/`commit_title_updated` → `saved_state_created`/`saved_state_title_updated`. Номера event type не меняются.
- `contract/record/v0`: `Entry.CommitNumber` → `SavedStateNumber` (`saved_state_number`); кодек читает старое `commit_number` (история формата), новые записи пишут новое имя; SchemaVersion 0.6 → 0.7.
- `contract/project/v0`: `LibraryImport.Commit` → `SavedState` (`saved_state`); строгий декод старого `commit` — 400 `file_issues`; SchemaVersion 0.3 → 0.4.
- `contract/manifest/v0`: только комментарий; SchemaVersion остаётся 0.5.
- REST (`interface/restapi`): хендлеры, `routes()`, `apiEventTypes`, samples; старые маршруты `/commits…` — 410 с указанием `/saved-states…` (образец `removed_routes.go`), удаляются с API 1.0. Затронуты все методы и вложенные `load`, `deploy`, `projectfile`, `labels`.
- События: `eventlog`, `eventbus`, outbox-кодек, `ui/player` и `savedstates` — новые имена.
- `apiclient`: типы, методы, пути; e2e-тесты. Механически обновить вызовы в `growctl` и UI, чтобы `go build` был зелёным (пользовательские команды/флаги growctl остаются на 88_3).
- JSONB-миграции: `engineering/20261116000001_saved_state_documents.sql` и `runtime/20261116000001_saved_state_documents.sql` — `engineering.commits.content` (`library_imports[].commit` → `saved_state`), `runtime.deployments.config`, `runtime.journal` и `runtime.checkpoints` (`commit_number` → `saved_state_number`). Старые Journal/Checkpoint-записи, если миграция их не переписала, читаются кодеком.
- `make schemas`: API 0.42 → 0.43, record 0.6 → 0.7, project 0.3 → 0.4; старые схемы замораживаются. Примечание в схеме: после 1.0 такое удаление — MAJOR.
- Bruno: переименовать запросы и папки `…/commits…`, обновить `requestBodies`, seq, заголовки; добавить 410-запросы старых путей. Статическая проверка `TestBrunoCollection` — часть `make test`.

## Проверка

- `make build`, `make test` (REST, `apiclient` e2e, миграции, Bruno static).
- Старый маршрут отвечает 410 `application/problem+json`; старый ProjectFile с `library_imports[].commit` — 400 `file_issues`; новый проходит.
- `make bruno_test` зелёный.
- `grep -rn "commit"` по `contract/` находит только историю формата (`EntryTypeRevision`, старое имя в кодеке, комментарии о retired event types).

## Связанное

- Спецификация — `88_saved_state_rename.md`; задача 62 — образец 410-заглушек.
