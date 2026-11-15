# Saved state вместо Commit: переименование целиком (спецификация цепочки)
Status: needs-triage
Priority: 3

Решение 2026-11-15: термин **Commit** уходит отовсюду — из прозы, кода, БД, REST API, событий, CLI и UI. Единственное имя понятия — **saved state** (сохранённое состояние). Задача 84 оставляла `Commit` в идентификаторах; это решение отменяется.

Это спецификация цепочки; части — `88_1`…`88_5` (строка `Parts:` ниже, конвейер её пропускает). Каждая часть оставляет `make build` и `make test` зелёными.

Parts: 88_1, 88_2, 88_3, 88_4, 88_5

## Терминология: таблица соответствия

| Было | Стало |
|---|---|
| `draft.Commit`, `CommitContent`, `CommitTitle`, `CommitNumber`, `NewCommit` | `draft.SavedState`, `SavedStateContent`, `SavedStateTitle`, `SavedStateNumber`, `NewSavedState` |
| `CommitRepository`, `CommitService`, `FindLastCommit`, `FindCommits` | `SavedStateRepository`, `SavedStateService`, `FindLastSavedState`, `FindSavedStates` |
| `appdto.Commit`, `CommitContentCounts` | `appdto.SavedState`, `SavedStateContentCounts` |
| `/commits`, `CommitPath`, `CommitLoadPath`, `CommitDeployPath`, `CommitLabelsPath` | `/saved-states`, `SavedStatePath`, `SavedStateLoadPath`, `SavedStateDeployPath`, `SavedStateLabelsPath` |
| `commit_number`, `CommitNumber` в контрактах (API, record, ProjectFile) | `saved_state_number`, `SavedStateNumber` |
| `commit_created`, `commit_title_updated`, `commit_not_found` | `saved_state_created`, `saved_state_title_updated`, `saved_state_not_found` |
| `library_imports.commit`, `PutLibraryImportRequest.commit` | `library_imports.saved_state`, `…saved_state` |
| таблицы `engineering.commits`, `commit_labels`, `commit_library_imports`; колонки `commit_id`, `commit_number`, `last_commit_number`, `last_commit_draft_version` | `saved_states`, `saved_state_labels`, `saved_state_library_imports`; `saved_state_id`, `saved_state_number`, `last_saved_state_number`, `last_saved_state_draft_version` |
| CLI `commits`, `--commit N`, колонка `COMMIT` | `saved-states`, `--saved-state N`, колонка `SAVED STATE` |
| UI-код `commits.go`, `CommitBar`, `data-commit-*` | `savedstates`-имена (`saved_states.go`, `SavedStateBar`, `data-saved-state-*`) |

## Не переименовывать

- «commit» в смысле транзакции БД или git (`after commit`, `racing commits`, `git commit`) — это не домен.
- Историю формата: `EntryTypeRevision` (`contract/record/v0`), чтение старых Journal-записей, комментарии о retired event type 23/39–41, имена миграций (`20261108000000_commits`, `20261114000002_deployment_commit_number`) — миграции не переписываются.
- ADR и старые записи `CHANGELOG.md` — история; в новых документах термин только saved state.

## Порядок

1. `88_1` — домен, приложение, репозитории, БД (миграция переименования), seed.
2. `88_2` — контракты API/record/project, REST, события, `apiclient`, заглушки старых маршрутов.
3. `88_3` — growctl (команды, флаги, вывод), UI-код, Bruno.
4. `88_4` — ADR, `GLOSSARY.md`, `AGENTS.md`, `docs/architecture`, `CHANGELOG.md`.
5. `88_5` — финальная проверка: поиск остатков, полный прогон, браузер.

## Контракт и совместимость

- Пути и поля API меняются: `contract/api/v0` поднимает MINOR (пока MAJOR = 0), `make schemas`; `contract/record/v0`, `contract/project/v0`, `contract/manifest/v0` — по своим правилам. В схеме-примечании зафиксировать, что после 1.0 такие удаления потребуют MAJOR.
- Старые маршруты `/commits…` отвечают 410 с указанием `/saved-states…` (образец — `removed_routes.go`), удаляются вместе с `/api/v1` при 1.0.
- Чтение старых данных: миграция переименовывает таблицы/колонки; Journal/Checkpoint/ProjectFile прежних форматов читаются кодеком (`commit_number` → `saved_state_number`) — старые записи не переписываются, кроме одной миграции, если без неё не обойтись.
- Внешние ProjectFile старого формата (`library_imports[].commit`) — 400 с `file_issues`; новый формат называет `saved_state`.

## Приёмка цепочки

- `grep -rniE "\bcommits?\b"` по не-тестовому коду находит только транзакции/git, историю формата и заглушки 410.
- REST, события, CLI, seed и Bruno называют saved state; старый клиент получает 410, а не молчаливый 404.
- `make build`, `make test`, `make bruno_test` проходят; PWA-экраны (Save, History, Deploy, Import) работают в браузере.

## Риски

- Это переименование ~1200 идентификаторов (оценка задачи 84) плюс миграция БД, контракты, Bruno, seed и тексты; делать отдельной цепочкой, не смешивая с функциональными изменениями.
- Ошибка в миграции теряет связь `library_imports`/`deployments` с сохранёнными состояниями; миграция и её Down проверяются тестом на миграциях, как у 85_6.
- Ветка `path` не трогается: работа идёт в feature-ветке; каждая часть завершается зелёными `make build`/`make test`.
