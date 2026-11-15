# 88_1 — домен, приложение и БД: SavedState вместо Commit
Status: needs-triage
Priority: 3

Часть цепочки `88_saved_state_rename.md`; таблица соответствия — там.

## Объём

- `domain/draft`: `commit.go` → `saved_state.go`, `commit_content.go` → `saved_state_content.go`, `commit_event.go`, `commit_value_objects.go`; типы `Commit` → `SavedState`, `CommitContent` → `SavedStateContent`, `CommitTitle` → `SavedStateTitle`, `CommitNumber` → `SavedStateNumber`, `CommitFilter` → `SavedStateFilter`, `CommitEvent` → `SavedStateEvent`, `NewCommit`/`CreateCommit` → `…SavedState`.
- `domain/draft/repository.go` и `session.go`: `SaveCommit` → `SaveSavedState`, `FindCommits` → `FindSavedStates`, `FindLastCommit` → `FindLastSavedState`, `CommitFilter`, `cleanCommit` → `cleanSavedState`, `lastCommitNumber` → `lastSavedStateNumber`; `loader.LibraryCommit` → `LibrarySavedState`.
- `domain/libraryimport`: `Commit`/`NewCommit`/`CommitNumber` → `SavedState`/`NewSavedState`/`SavedStateNumber`; `library_import.go`, `impact.go`, `library_import_event.go`.
- `application`: `CommitService`/`NewCommitService` → `SavedStateService`/`NewSavedStateService`, `commit_application_service.go` → `saved_state_application_service.go`; `appdto.Commit`/`CommitContentCounts` → `SavedState`/`SavedStateContentCounts`; все обёртки ошибок и тексты.
- `infrastructure/postgres/repositories`: SQL и имена функций/структур (`commit_repository_postgres.go` → `saved_state_repository_postgres.go`); `outbox` — типы событий сохранённого состояния.
- Миграции (новая пара engineering/runtime, не переписывать старые): `engineering.commits` → `saved_states`, `commit_labels` → `saved_state_labels`, `commit_library_imports` → `saved_state_library_imports`; колонки `commit_id` → `saved_state_id`, `commit_number` → `saved_state_number`, `last_commit_number`/`last_commit_draft_version` → `last_saved_state_*`; `runtime.deployments.commit_number` → `saved_state_number`; Down возвращает имена.
- JSONB документов здесь **не** трогаем: их поля (`library_imports[].commit`, `commit_number` в Journal/Checkpoint) меняются в 88_2 вместе с контрактами.
- `test_data/` seed: имена таблиц/колонок и вставки.
- JSON-теги и REST пока **не** меняются (это 88_2): домен уже SavedState, API ещё `commit` — сборка и тесты зелёные.

## Проверка

- `make build`, `make test` (включая `migrations_test` и репозиторные тесты на testcontainers).
- Миграция проверяется на данных: импорт Library, работающая Deployment и Draft ссылаются на переименованные строки, Down возвращает схему.
- В домене не осталось идентификаторов `Commit` (кроме истории формата и «commit» про транзакцию).

## Связанное

- Спецификация — `88_saved_state_rename.md`.
