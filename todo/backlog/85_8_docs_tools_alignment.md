# Документы, схемы и инструменты после перехода на лейблы и saved state

Status: ready-for-agent
Priority: 1
Blocked by: 85_1_mode_screen_persistence, 85_2_label_aggregate, 85_3_label_assignments, 85_4_deploy_saved_state, 85_5_deploy_window, 85_6_import_library_state, 85_7_remove_releases

Часть 8 из `85_labels_instead_of_releases.md`. Слой документов и инструментов
вокруг сервера. Кода поведения здесь нет, кроме правок текстов, справки и
коллекций.

## ADR

- Новый `docs/adr/0025-labels-are-an-aggregate-apart-at-organization-level.md`:
  - Label — агрегат Engineering node, логически принадлежит Organization
    (`organization_id`), API серверный, пока Organization одна;
  - назначение — отдельная связь many-to-many, не DraftChange, не Version, не
    часть ProjectFile/Commit/Deployment;
  - удаление лейбла каскадно снимает назначения; события;
  - почему не поле Commit и не часть Draft: лейбл живёт дольше и меняется
    независимо, как настройка, но виден всем;
  - при появлении выбора Organization меняется только путь/выборка.
- Поправки:
  - **0007 Libraries** — Project использует не Release, а сохранённое
    состояние Library (Import); копии WidgetTypes по-прежнему в ProjectFile;
  - **0012 Library Pin** — impact считается по Commit'у; защита Library от
    удаления — живые импорты и работающая Deployment;
  - **0014/0021 Commit/Release/Deployment** — Release удалён; Deploy делает
    Commit; `Deployment.commit_number`; порядок Save → Deploy; Library:
    Save → Import; один Commit больше не «публикуется»;
  - **0019 Journal / 0023 Checkpoint** — запись `deployment` и Checkpoint
    несут `commit_number`; старые строки переписаны миграцией 85_4.

## GLOSSARY.md

- Добавить:
  - **Label** — именованная метка Organization, назначаемая сохранённым
    состояниям; не content.
  - **Label assignment** — связь лейбла и saved state; без Version и undo.
  - **Import** — выбор сохранённого состояния Library, которое использует
    Project; внутренний механизм — Library Pin.
- Обновить:
  - **Saved state / Commit** — то, что деплоится и импортируется.
  - **Deploy** — делает сохранённое состояние Project'а работающим.
  - **Deployment** — помнит `commit_number`, nil для file/старых.
  - **Library Pin** — теперь сохранённое состояние Library, не Release.
- Удалить (или пометить историческими, если на них ссылаются старые ADR):
  **Release**, **Project Release**, **Library Release**, **Legacy Release**,
  **Deploy Release**, **Revert to latest Release**. В текущем глоссарии
  оставить только актуальные термины; историю — в ADR.

## AGENTS.md

- Таблица домена: `release/` удалить; `label/` добавить (Label +
  Assignment); `library/` — без LibraryRelease.
- Application: `ReleaseService` → `DeployService`; CommitService без
  `released`; LibraryService без Release/RevertToLatestRelease;
  LibraryPinService — Commit.
- Infrastructure: миграции release удалены, добавлены labels/commit_labels,
  `deployments.commit_number`; Draft snapshots и ProjectFile — `commit`.
- REST: маршруты `/labels`, `…/commits/{n}/labels/{id}`,
  `…/commits/{n}/deploy`, `…/library-pins` с `commit`; удалённые — 410.
- UI: окно Labels; History с лейблами и без Release; окно Deploy по saved
  states с фильтром; окно Import; состояния Library; сохранение экрана
  выбора при смене вкладок.
- growctl: `save` → `deploy --commit`; `release`/`releases` нет; лейблы в
  `commits`.
- `make run_simulator`/`run_simulator_dashboard`: сид требует Save и Deploy
  сохранённого состояния (не Release); обновить описание и `tests/device_simulator`.
- Ubiquitous language: Label, Import, Saved state, Deployment; Release — нет.
- Правило про codegraph/ntfy не трогать.

## CHANGELOG.md

- Новая версия (0.0.98 или следующая по текущей схеме):
  - добавлены Label и назначения, окно Labels;
  - Deploy сохранённого состояния, `commit_number` в Deployment/Journal/
    Checkpoint/API;
  - Import Library по сохранённому состоянию, `commit` в ProjectFile;
  - удалены Release (Project/Library), их маршруты (410), события, поля
    (`released`, `latest_release`, `has_unpublished_changes`,
    `release_number`);
  - ломающие изменения API/record/project контрактов и MINOR-бампы;
  - миграция: legacy Library Release → синтетический Commit; потеря
    undo-истории DraftChange с пинами; необратимый Down.
- Ссылки на задачи 85_1–85_8.

## Схемы и контракты

- `contract/api/v0` — MINOR после всех частей; `contract/project/v0` — MINOR
  (`LibraryPin.Commit`); `contract/record/v0` — MINOR (`CommitNumber`).
- `make schemas`; проверить `schemas/<contract>/<MAJOR.MINOR>.json`,
  `frozen.sha256`, что старые версии не перезаписаны; `make test` проверяет
  соответствие типов и схем.
- `contract/api/v0/schema_test.go`, `contract/record/v0/schema_test.go` —
  новые типы в списках.

## Bruno

- Синхронизировать `tests/api/bruno_collections/growscada_server`:
  - лейблы: create/list/rename/delete, 409, 404;
  - назначение: put/delete на Project и Library Commit, `?label=`, 400;
  - Deploy: Save → Deploy Commit; running/409; file deploy без Release;
  - Import: Save Library → Import её saved state (с фильтром по лейблу) →
    switch/remove; impact;
  - удалённые Release-маршруты: по одному запросу на маршрут с 410;
  - убрать старые release-запросы, которые больше не отражают модель;
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
