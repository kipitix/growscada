# Документы, схемы и инструменты после перехода на лейблы и saved state

Status: ready-for-agent
Priority: 1
Parent: 85_labels_instead_of_releases
Part: 8/8
Closes parent: yes
Blocked by: 85_1_mode_screen_persistence, 85_2_label_aggregate, 85_3_label_assignments, 85_4_deploy_saved_state, 85_5_deploy_window, 85_6_import_library_state, 85_7_remove_releases

Часть 8 из `85_labels_instead_of_releases.md`. Слой документов и инструментов
вокруг сервера. Кода поведения здесь нет, кроме правок текстов, справки и
коллекций; ADR 0025 и поправки пишутся при грилинге (2026-11-14), здесь — их
сверка с кодом.

## ADR

- `docs/adr/0025-labels-are-an-aggregate-apart-at-organization-level.md`
  (написана при грилинге; здесь — сверка):
  - Label — агрегат Engineering node с именем и необязательным цветом,
    логически принадлежит Organization (`organization_id`), API серверный,
    пока Organization одна;
  - назначения — сущности Label, не отдельный агрегат; many-to-many,
    не DraftChange, не Version, не часть ProjectFile/Commit/Deployment;
  - удаление лейбла каскадно снимает назначения; удаление Project/Library
    каскадно снимает назначения их Commit'ов через БД — исключение ADR 0008;
  - события Label идут в серверный поток;
  - два стартовых лейбла `ready_for_import`/`ready_for_deploy` — обычные
    метки, не гейты;
  - при появлении выбора Organization меняется только путь/выборка.
- Поправки:
  - **0007 Libraries** — Project использует не Release, а сохранённое
    состояние Library (Import); копии WidgetTypes по-прежнему в ProjectFile;
    полное переименование pin → import; legacy Release удаляется с импортами и
    Widget'ами, без синтетических Commit'ов;
  - **0012 Library Import** — impact считается по Commit'у; защита Library от
    удаления — живые импорты и работающая Deployment;
  - **0014/0021 Commit/Release/Deployment** — Release удалён; Deploy делает
    Commit; `Deployment.commit_number`; порядок Save → Deploy; Library:
    Save → Import; один Commit больше не «публикуется»; одноразовая перепись
    Commit/Deployment (включая работающую) — исключение из неизменяемости;
  - **0018 Project files** — file deploy не создаёт Release; старые файлы с
    `library_pins`/`release` отклоняются `file_issues`;
  - **0019 Journal / 0023 Checkpoint** — запись `deployment` и Checkpoint
    несут `commit_number`; старые строки переписаны миграцией 85_4.

## GLOSSARY.md

- Добавить:
  - **Label** — имя + необязательный цвет, принадлежит Organization,
    назначается сохранённым состояниям; владеет назначениями; не content.
  - **Label assignment** — сущность Label: метка на одном saved state; без
    Version и undo.
  - **Import / Library Import** — выбор сохранённого состояния Library,
    которое использует Project; заменил Library Pin.
- Обновить:
  - **Saved state / Commit** — то, что деплоится и импортируется.
  - **Deploy** — делает сохранённое состояние Project'а работающим.
  - **Deployment** — помнит `commit_number`, nil для file/старых.
  - **Library Pin** → **Library Import**; механизм и таблица переименованы.
- Удалить (или пометить историческими, если на них ссылаются старые ADR):
  **Release**, **Project Release**, **Library Release**, **Legacy Release**,
  **Deploy Release**, **Revert to latest Release**. В текущем глоссарии
  оставить только актуальные термины; историю — в ADR.

## AGENTS.md

- Таблица домена: `release/` удалить; `label/` добавить (Label +
  Assignment); `library/` — без LibraryRelease.
- Application: `ReleaseService` → `DeployService`; CommitService без
  `released`; LibraryService без Release/RevertToLatestRelease;
  LibraryImportService — Commit; LabelService — имя/цвет/назначения.
- Infrastructure: миграции release удалены, добавлены labels/commit_labels,
  `library_imports`, `deployments.commit_number`; Draft snapshots и
  ProjectFile — `library_imports`/`commit`.
- REST: маршруты `/labels`, `…/commits/{n}/labels/{id}`,
  `…/commits/{n}/deploy`, `…/library-imports` с `commit`; удалённые —
  404 без заглушек.
- UI: окно Labels (имя + цвет); History с лейблами и без Release; окно Deploy
  по saved states с фильтром; окно Import; состояния Library; сохранение
  экрана выбора при смене вкладок и перезагрузке.
- growctl: `save` → `deploy --commit`; `release`/`releases` нет; `labels`
  CRUD и `label assign/unassign`; манифест `spec.libraryImports`.
- `make run_simulator`/`run_simulator_dashboard`: сид требует Save и Deploy
  сохранённого состояния (не Release); обновить описание и `tests/device_simulator`.
- Ubiquitous language: Label, Import, Saved state, Deployment; Release — нет.
- Правило про codegraph/ntfy не трогать.

## CHANGELOG.md

- Новая версия (0.0.98 или следующая по текущей схеме):
  - добавлены Label (имя + необязательный цвет) и назначения, окно Labels,
    стартовые лейблы `ready_for_import`/`ready_for_deploy`;
  - Deploy сохранённого состояния, `commit_number` в Deployment/Journal/
    Checkpoint/API;
  - Import Library по сохранённому состоянию, `library_imports`/`commit` в
    ProjectFile, полное переименование pin → import;
  - удалены Release (Project/Library), их маршруты (404, без заглушек),
    события, поля (`released`, `latest_release`, `has_unpublished_changes`,
    `release_number`);
  - ломающие изменения API/record/project контрактов и MINOR-бампы;
  - миграция: legacy Library Release **удаляется** вместе с импортами и
    Widget'ами (без синтетических Commit'ов); старые Commit'ы, Deployment'ы и
    работающая Deployment переписываются; **все** стеки DraftChange
    очищаются; Draft'ы с изменённым контентом получают Version+1 и
    `clean_version = 0`; внешние старые ProjectFile отклоняются; необратимый
    Down.
- Ссылки на задачи 85_1–85_8.

## Схемы и контракты

- `contract/api/v0` — MINOR после всех частей; `contract/project/v0` — MINOR
  (`LibraryImport.Commit`, `library_imports`); `contract/record/v0` — MINOR
  (`CommitNumber`).
- `make schemas`; проверить `schemas/<contract>/<MAJOR.MINOR>.json`,
  `frozen.sha256`, что старые версии не перезаписаны; `make test` проверяет
  соответствие типов и схем.
- `contract/api/v0/schema_test.go`, `contract/record/v0/schema_test.go` —
  новые типы в списках.

## Bruno

- Синхронизировать `tests/api/bruno_collections/growscada_server`:
  - лейблы: create/list/update (имя и цвет)/delete, 409, 404; seed-лейблы;
  - назначение: put/delete на Project и Library Commit, `?label=`, 400;
  - Deploy: Save → Deploy Commit; running/409; file deploy без Release;
  - Import: Save Library → Import её saved state (с фильтром по лейблу) →
    switch/remove; impact;
  - старые release-запросы удалить из коллекции (маршрутов больше нет);
  - убрать запросы, которые больше не отражают модель;
  - `seq` — чтения, затем изменения, удаления последними; ID/версии — из
    ответов;
  - `requestBodies` в статическом тесте — для всех новых тел;
  - `localhost.yml` — переменные новых ID, если нужны.
- `tests/api/bruno_collections/device_simulator` — только если сид/теги
  изменились (не должны).
- `make bruno_test`.

## Прочие документы

- `README`/`docs/` — grep `Release`, `release_number`, `pin`, `publish`;
  заменить на saved state/import/deploy, где это текущая модель.
- `todo/backlog/45_history.md`, `46_playbackfile.md` — убрать
  `release_number`, писать `commit_number` / saved state.
- `todo/PRIORITY.md` — после закрытия цепочки обновить (это делает
  `/kx_auto`/финальный прогон, не отдельная правка здесь).
- Проверить, что в UI-строках не осталось «Release», «published», «pin».

## Проверки

- `make build`, `make test`, `make bruno_test`.
- Ручной проход PWA: Labels (CRUD) → Library Save → Label на saved state →
  Import с фильтром → Project Save → Label → Deploy с фильтром → Operation
  работает; History без Release; переключение вкладок сохраняет экран выбора.
- `grep -ri "release"` по пользовательским строкам и AGENTS/GLOSSARY — только
  исторические упоминания в ADR/CHANGELOG.

## Вне объёма

- Новые ADR о правах на лейблы (задачи 49/50).
- Ретенция Commit'ов.

## Comments

- 2026-11-14: выделено из 85. Документы и коллекции — в конце, когда форма
  API стабилизировалась.
- 2026-11-15: сверка ADR 0025 и поправок к 0007/0012/0014/0018/0019/0021/0023
  с кодом — расхождений нет (`engineering.labels` с `organization_id`,
  `library_imports` с `commit_id`/`commit_number`, `commit_number` в
  `runtime.deployments`, записи Journal и Checkpoint, record 0.6); новых
  поправок не потребовалось.
- 2026-11-15: `GLOSSARY.md` уже был приведён частями 85_2–85_7; убрана одна
  устаревшая фраза про Release в «Deploy a file». `AGENTS.md` переписан
  целиком по слоям: таблица домена (`label/`, `libraryimport/`, без
  `release/` и `LibraryRelease`), application (Label, Deploy Commit,
  Library без публикаций), REST (лейблы и назначения, `commits/{n}/deploy`,
  `library-imports`, без release-маршрутов), UI (три состояния Library,
  чипы в History, окна Deploy/Import/Labels, сохранённый экран выбора),
  growctl, сид и Bruno.
- 2026-11-15: `contract/api/v0` поднят 0.41 → 0.42: в `schema_test.go` не
  были перечислены `GetLabelsResponse`, `CreateLabelRequest` и
  `UpdateLabelRequest`, поэтому опубликованная схема не описывала API
  лейблов. Типы добавлены, `make schemas` создал `0.42.json`, `0.41`
  заморожена.
- 2026-11-15: Bruno уже синхронизирован частями 85_2–85_7 (лейблы,
  назначения, Deploy Commit, импорты; release-запросов нет). Запрос на
  удалённые маршруты не добавлялся: статический тест требует, чтобы каждый
  запрос попадал в `routes()`, а удалённых маршрутов там нет.
- 2026-11-15: `CHANGELOG.md` — сводная запись 0.0.100 со ссылками на
  85_1–85_8 и перечнем миграций/потерь данных. `todo/backlog/45` и `46`
  переведены на `commit_number`/saved state; в 47–52, 57, 58, 61, 63, 64
  убраны или помечены устаревшие Release-упоминания. `todo/PRIORITY.md`
  оставлен финальному прогону `/kx_auto`.
- 2026-11-15: ревью ветки. В `schema_test.go` не хватало
  `ProjectOrderResponse` и `SetProjectOrderRequest`, поэтому схема 0.42 не
  описывала тела `/settings/project-order` — типы добавлены, `0.42.json`
  перегенерирована (изменение только аддитивное, версия ещё не на `path`);
  удалён неиспользуемый `MissingImportResponse`; строка `settings/` и список
  сервисов в `AGENTS.md` и описание сида в `CHANGELOG.md` приведены к коду
  (сид сохраняет Draft'ы Library и импортирует их, но Project'ы не сохраняет
  и Deployment не создаёт).
