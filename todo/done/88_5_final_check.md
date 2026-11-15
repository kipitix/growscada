# 88_5 — финальная проверка переименования
Status: ready-for-agent
Priority: 1
Blocked by: 88_4

Часть цепочки `88_saved_state_rename.md`.

## Объём

- Поиск остатков: `grep -rniE "\bcommits?\b"` по `internal/`, `cmd/`, `contract/`, `tests/`, `schemas/`, `Makefile`, `GLOSSARY.md`, `AGENTS.md`, `docs/architecture`; каждый остаток — либо транзакция/git, либо история формата (`EntryTypeRevision`, retired event types, имена миграций), либо 410-заглушка.
- `grep -rniE "release|revision|editlock|pins"` — проверить, что задача 86 и 87 закрыты и остались только история/заглушки.
- Полный прогон: `make build`, `make test`, `make schemas` (схемы совпадают), `make bruno_test`.
- Браузер (`chrome-devtools`): Save, History (лейблы, Load), Deploy saved state, Import libraries, Deployments, Operation — консоль без ошибок.
- Проверить старый клиент: `GET /api/v0/projects/{id}/commits` → 410 с новым путём; старый ProjectFile с `library_imports[].commit` → 400.
- Проверить миграцию на данных: имя сохранённого состояния, импорт Library и работающая Deployment после переименования таблиц/колонок.

## Проверка

- Всё выше зелёное; отчёт — что показали браузер и Bruno.

## Связанное

- Спецификация — `88_saved_state_rename.md`; задачи 86, 87.

## Comments

- 2026-11-15 (остатки `Commit`): в коде термина нет. `grep -rniE
  "\bcommits?\b"` по `internal/`, `cmd/`, `contract/`, `tests/`,
  `schemas/`, `Makefile`, `GLOSSARY.md`, `AGENTS.md`, `docs/architecture`
  находит только: транзакцию БД или git (`tx.Commit`, `NotifyOnCommit`,
  `racing commits`, «after commit»), историю формата (`EntryTypeRevision`,
  `retiredEventTypeNames` с `commit_created`/`commit_title_updated`,
  `LegacySavedState` и `LegacyNumber` в кодеке, `snapshot.go` и
  `draftchange/codec.go`), имена старых миграций, замороженные схемы
  ≤ 0.42 и заглушки 410 (`removed_routes.go` с `removedSavedStatesPath`,
  его тесты и Bruno `*_commit_*_gone.yml`); в `GLOSSARY.md` — запись
  «Commit» как прежнее имя, в `AGENTS.md` — правила про git. `commitEdit`
  и `commitSceneEdit` оставлены осознанно (акт применения правки, 88_3).
- 2026-11-15 (`release|revision|editlock|pins`): остались только история и
  заглушки — `EntryTypeRevision` с чтением старого `revision` в
  `contract/record/v0`, retired event types 23/39–41, имена миграций,
  `removedDeploymentPath`/`removedDeploymentsPath` с 410 (Bruno
  `get_revision_*_gone.yml`) и комментарий `libraryPins` об истории
  манифеста. Задачи 86 и 87 закрыты, живых имён нет.
- 2026-11-15 (полный прогон): `make build` зелёный; `make test` зелёный,
  включая `TestMigrationSavedStates_RenamesCommitsAndColumns`,
  `TestMigrationSavedStateDocuments_RenamesTheSavedStateKeys`,
  `TestMigrationDeploymentCommitNumber_ReadsTheSavedStatesBack` и
  `TestMigrationLibraryImports_TurnsPinsIntoImports`; `make schemas` не
  меняет ни одного файла (api 0.43, record 0.7, project 0.4, manifest 0.5
  уже опубликованы, старые заморожены); `make bruno_test` — 235/235 и 6/6.
- 2026-11-15 (старый клиент): `GET|POST …/commits`, `GET|PATCH
  …/commits/{n}`, `POST …/commits/{n}/load`, `POST …/commits/{n}/deploy`,
  `GET …/commits/{n}/projectfile` и `PUT|DELETE …/commits/{n}/labels/{id}`
  отвечают 410 `application/problem+json` с «Commit is called saved
  state: use …/saved-states»; ProjectFile со старым
  `library_imports[].commit` — 400 `file_issues`
  (`unknown field "commit"`).
- 2026-11-15 (миграция на данных): в сиде `engineering.saved_states`,
  `saved_state_labels`, `saved_state_library_imports` и
  `runtime.deployments.saved_state_number`; `GET …/library-imports`
  отдаёт `saved_state_number`, `GET …/deployments` —
  `saved_state_number`, а Deployment, сделанный из saved state #4,
  работает. Down проверен тестами выше.
- 2026-11-15 (браузер, PWA): Save («Saved state 4 saved», кнопка затем
  «Nothing to save since the last save»); History проекта (#4…#1, метка
  `ready_for_import`, «Load» → «Saved state 1 loaded»); Deploy saved
  state (подтверждение «Make saved state 4 the running one?», «Saved
  state 4 deployed», бейдж «Saved state #4 · running»); Deployments
  («Saved state #4 · current»); Import libraries («latest saved state
  #1», «Imported: saved state #1»); History библиотеки; Operation — живые
  значения развёрнутого #4. Консоль без ошибок приложения: только старая
  внешняя иконка go-app из manifest (404, CORB) и a11y-подсказки.
- 2026-11-15 (ревью): первый проход по `commits?` пропустил тестовые
  имена — `libraries_test.go` (`commit`/`commits`, «POST commits», «GET
  commits», `hasImport(…, commit int)`), `project_helpers_test.go` (поле
  `testServices.commits`, «save commit»), `widgets_test.go` (`commit`,
  «as a Commit»), `events_test.go` (`commitNumber`) и
  `TestRuntimeNode_DeployCommit_…`. Это код, а не история формата:
  переименованы в saved state, тест `inprocess` называется
  `TestRuntimeNode_DeploySavedState_…`; `version.Committed`/`IsCommitted`
  оставлены — там commit о записи в базу, а не о сохранённом состоянии.
- 2026-11-15 (наблюдения, не правки): вкладка History в верхнем меню —
  давний плейсхолдер `ui/history/history.go`, история открывается кнопкой
  с панели saved states, к 88 не относится; тосты называют номер без `#`
  («Saved state 4 saved»), панель и диалоги — с `#`, это не остаток
  термина и в 88_5 не менялось.
