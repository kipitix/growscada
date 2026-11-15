# 88_1 — домен, приложение и БД: SavedState вместо Commit
Status: ready-for-agent
Priority: 1

Первая часть цепочки `88_saved_state_rename.md`; таблица соответствия — там. API, JSON-теги и события пока не меняются: это 88_2.

## Объём

- `domain/draft`: `commit.go` → `saved_state.go`, `commit_content.go` → `saved_state_content.go`, `commit_event.go`, `commit_value_objects.go`; типы `Commit` → `SavedState`, `CommitContent` → `SavedStateContent`, `CommitTitle` → `SavedStateTitle`, `CommitNumber` → `SavedStateNumber`, `CommitFilter` → `SavedStateFilter`, `CommitEvent` → `SavedStateEvent`, `NewCommit`/`CreateCommit` → `NewSavedState`/`CreateSavedState`.
- `domain/draft/repository.go`, `session.go`, `loader.go`: `SaveCommit` → `SaveSavedState`, `FindCommits` → `FindSavedStates`, `FindLastCommit` → `FindLastSavedState`, `CommitFilter`, `cleanCommit` → `cleanSavedState`, `lastCommitNumber` → `lastSavedStateNumber`, `loader.LibraryCommit` → `LibrarySavedState`.
- `domain/libraryimport`: `Commit`/`NewCommit`/`CommitNumber` → `SavedState`/`NewSavedState`/`SavedStateNumber`; `library_import.go`, `impact.go`, `library_import_event.go`.
- `application`: `CommitService`/`NewCommitService` → `SavedStateService`/`NewSavedStateService`, `commit_application_service.go` → `saved_state_application_service.go`; `appdto.Commit`/`CommitContentCounts` → `SavedState`/`SavedStateContentCounts`; обёртки ошибок и тексты.
- `interface/restapi`: только компиляционные ссылки на переименованные `appdto`- и application-типы; пути, DTO и JSON-поля не трогать.
- `cmd/combined_server/main.go`: проводка `application.NewSavedStateService` (и переименованные типы в `Services`).
- `infrastructure/postgres/repositories`: SQL, имена функций/структур (`commit_repository_postgres.go` → `saved_state_repository_postgres.go`); `outbox` — типы событий сохранённого состояния.
- Миграции: `engineering/20261116000000_saved_states.sql` и `runtime/20261116000000_saved_states.sql`.
  - `engineering.commits` → `saved_states`, `commit_labels` → `saved_state_labels`, `commit_library_imports` → `saved_state_library_imports`;
  - колонки `commit_id` → `saved_state_id`, `commit_number` → `saved_state_number`, `last_commit_number`/`last_commit_draft_version` → `last_saved_state_*`;
  - `runtime.deployments.commit_number` → `saved_state_number`;
  - имена constraints/indexes, на которые ссылается код, переименовать; остальные можно оставить (они не видны API), но отметить в комментарии миграции;
  - Down возвращает старые имена и данные.
- JSONB-документы здесь **не** трогаем: их поля (`library_imports[].commit`, `commit_number` в Journal/Checkpoint) меняются в 88_2 вместе с контрактами.
- `test_data/` seed: имена таблиц/колонок и вставки; значения те же.

## Проверка

- `make build`, `make test`, включая `migrations_test` и репозиторные тесты на testcontainers.
- Миграция проверяется на данных: импорт Library, работающая Deployment и Draft ссылаются на переименованные строки; Down возвращает схему.
- `grep -rn -E "\bCommit(s)?\b"` по `domain`, `application`, `infrastructure` находит только «commit» про транзакцию/историю формата.

## Связанное

- Спецификация — `88_saved_state_rename.md`.
