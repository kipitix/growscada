# Saved state вместо Commit: переименование целиком (спецификация цепочки)
Status: ready-for-agent
Priority: 1

Решение 2026-11-15: термин **Commit** уходит отовсюду — из прозы, кода, БД, REST API, событий, CLI и UI. Единственное имя понятия — **saved state** (сохранённое состояние). Задача 84 оставляла `Commit` в идентификаторах; это решение отменяется.

Это спецификация цепочки; части — `88_1`…`88_5` (строка `Parts:` ниже, конвейер её пропускает). Каждая часть оставляет `make build` и `make test` зелёными и коммитится отдельно.

Parts: 88_1, 88_2, 88_3, 88_4, 88_5

## Терминология: таблица соответствия

| Было | Стало |
|---|---|
| `draft.Commit`, `CommitContent`, `CommitTitle`, `CommitNumber`, `NewCommit` | `draft.SavedState`, `SavedStateContent`, `SavedStateTitle`, `SavedStateNumber`, `NewSavedState` |
| `CommitFilter`, `CommitRepository`, `CommitService`, `FindLastCommit`, `FindCommits` | `SavedStateFilter`, `SavedStateRepository`, `SavedStateService`, `FindLastSavedState`, `FindSavedStates` |
| `appdto.Commit`, `CommitContentCounts` | `appdto.SavedState`, `SavedStateContentCounts` |
| `/commits`, `CommitPath`, `CommitLoadPath`, `CommitDeployPath`, `CommitLabelsPath` | `/saved-states`, `SavedStatePath`, `SavedStateLoadPath`, `SavedStateDeployPath`, `SavedStateLabelsPath` |
| `CommitResponse`, `GetCommitsResponse`, `CommitContentCounts` | `SavedStateResponse`, `GetSavedStatesResponse`, `SavedStateContentCounts` |
| `commit_number` (API, record, ProjectFile), `CommitNumber` | `saved_state_number`, `SavedStateNumber` |
| `Event.Commit` (номер saved state) | `Event.SavedState` |
| `commit_created`, `commit_title_updated`, `commit_not_found` | `saved_state_created`, `saved_state_title_updated`, `saved_state_not_found` |
| `library_imports.commit`, `PutLibraryImportRequest.commit` | `library_imports.saved_state`, `…saved_state` |
| таблицы `engineering.commits`, `commit_labels`, `commit_library_imports`; колонки `commit_id`, `commit_number`, `last_commit_number`, `last_commit_draft_version` | `saved_states`, `saved_state_labels`, `saved_state_library_imports`; `saved_state_id`, `saved_state_number`, `last_saved_state_number`, `last_saved_state_draft_version` |
| CLI `commits`, `--commit N`, колонка `COMMIT` | `saved-states`, `--saved-state N`, колонка `SAVED STATE` |
| UI-код `commits.go`, `CommitBar`, `data-commit-*` | `saved_states.go`, `SavedStateBar`, `data-saved-state-*` |

## Не переименовывать

- «commit» в смысле транзакции БД или git (`after commit`, `racing commits`, `git commit`) — это не домен.
- Историю формата: `EntryTypeRevision` (`contract/record/v0`), чтение старых Journal-записей, комментарии о retired event type 23/39–41, имена миграций (`20261108000000_commits`, `20261114000002_deployment_commit_number`) — миграции не переписываются.
- ADR и старые записи `CHANGELOG.md` — история; в новых документах термин только saved state.

## Решения, принятые для реализации

- **API:** пути `/saved-states`, поля `saved_state_number` и `saved_state`, события `saved_state_*`, ошибка `saved_state_not_found`, DTO `SavedStateResponse`/`GetSavedStatesResponse`/`SavedStateContentCounts`, `PutLibraryImportRequest.SavedState`, `Event.SavedState`.
- **Совместимость:** старые маршруты `/commits…` (все методы и вложенные `load`/`deploy`/`projectfile`/`labels`) отвечают 410 с указанием нового пути; удаляются вместе с `/api/v1` при 1.0. Старый ProjectFile (`library_imports[].commit`) — 400 с `file_issues`. Старые события больше не публикуются.
- **Версии контрактов:** `contract/api/v0` MINOR 0.42 → 0.43, `contract/record/v0` 0.6 → 0.7, `contract/project/v0` 0.3 → 0.4; `contract/manifest/v0` остаётся 0.5 (ключ `spec.libraryImports` не меняется). `make schemas`, старые схемы замораживаются.
- **БД:** переименование таблиц/колонок одной миграцией в каждой цепочке (`engineering`, `runtime`); JSONB-документы (`commits.content`, `deployments.config`, Journal, Checkpoint) переписываются во второй миграции. Старые Journal/Checkpoint-записи читаются кодеком (`commit_number` → `saved_state_number`), чтобы не терять историю.
- **CLI:** команда `saved-states`, флаг `--saved-state`; алиаса `commits` нет — growctl наш, старые скрипты обновляются. Колонка `SAVED STATE`.
- **ADR 0026** «Saved state is the name of a Commit»: решение, отмена решения 84, совместимость и порядок; поправка к ADR 0021.
- **Никаких функциональных изменений:** только имена. Поведение, тесты и схемы проверяются после каждой части.

## Как работать

- Go-символы переименовывать `gopls go_rename_symbol` (или `go_search`), затем `gofmt`/`goimports`; строковые имена (JSON-теги, пути, события, SQL, флаги) — отдельным grep-проходом, потому что rename их не видит.
- После каждой части: `go test ./internal/...` (быстрее), затем `make build` и `make test`.
- Старые имена искать `grep -rn -iE "\bcommits?\b|commit_number|commit_"` по слою и разбирать каждый случай: идентификатор, история формата или «commit» про транзакцию.
- Один коммит на часть; ветка — feature, не `path`.

## Порядок

1. `88_1` — домен, приложение, репозитории, БД (переименование таблиц/колонок), seed. API/JSON пока прежние.
2. `88_2` — контракты API/record/project, REST, события, `apiclient`, механические правки вызывающих (growctl/UI, чтобы сборка была зелёной), JSONB-миграция, заглушки 410, Bruno.
3. `88_3` — пользовательская поверхность growctl (команда/флаг/колонка/help) и внутренние имена UI (файлы, поля, `data-*`), тесты; Bruno уже обновлён в 88_2.
4. `88_4` — ADR 0026, `GLOSSARY.md`, `AGENTS.md`, `docs/architecture`, `CHANGELOG.md`.
5. `88_5` — финальная проверка: поиск остатков, полный прогон, браузер, старый клиент.

## Приёмка цепочки

- `grep -rniE "\bcommits?\b"` по не-тестовому коду находит только транзакции/git, историю формата и заглушки 410.
- REST, события, CLI, seed и Bruno называют saved state; старый клиент получает 410, а не молчаливый 404.
- `make build`, `make test`, `make bruno_test` проходят; PWA-экраны (Save, History, Deploy, Import) работают в браузере.

## Риски

- Это переименование ~1200 идентификаторов (оценка задачи 84) плюс миграция БД, контракты, Bruno, seed и тексты; делать отдельной цепочкой, не смешивая с функциональными изменениями.
- Ошибка в миграции теряет связь `library_imports`/`deployments` с сохранёнными состояниями; миграция и её Down проверяются тестом на миграциях, как у 85_6.
- 88_2 — самая большая часть: контракт, REST, `apiclient` и все вызовы обязаны измениться в одном коммите, иначе сборка красная.
