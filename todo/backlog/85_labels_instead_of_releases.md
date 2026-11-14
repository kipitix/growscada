# Лейблы, импорт библиотек и деплой сохранённого состояния вместо релизов

Status: ready-for-agent
Priority: 1
Parts: 85_1_mode_screen_persistence, 85_2_label_aggregate, 85_3_label_assignments, 85_4_deploy_saved_state, 85_5_deploy_window, 85_6_import_library_state, 85_7_remove_releases, 85_8_docs_tools_alignment

Слой C (Draft, Commit, публикация) и слой D (UI). Термины — `GLOSSARY.md`
(Saved state, Commit, Library Pin, Deploy, Deployment); меняются решения ADR
0007, 0012 и 0021, добавляется ADR 0025 о лейблах.

## Постановка (как понято)

1. Новая сущность **лейбл** (Label) — именованная метка, которую можно
   назначать на **любое сохранённое состояние** (Commit/saved state) как
   Project'а, так и Library.
2. Отдельное окно для добавления, переименования и удаления лейблов.
3. Импорт Library в Project: вместо текущего «pin library» — отдельное окно
   импорта, в котором сохранённые состояния Library фильтруются по лейблу.
4. Новое окно Deploy: сохранённые состояния Project'а фильтруются по лейблу
   перед выбором того, что деплоить.
5. Из модели убирается **Release** — и Project Release, и Library Release.
   Deploy делает сохранённое состояние, импорт закрепляет сохранённое
   состояние Library.
6. Переключение вкладок Library ↔ Project не должно сбрасывать экран выбора
   Library: если человек ушёл с page выбора, вернувшись, он остаётся на page,
   а не попадает в редактор.
7. Лейблы хранятся на уровне Organization. Полноценного агрегата Organization
   с поведением пока нет, поэтому API остаётся серверным (один Organization на
   сервер), но `organization_id` пишется с первого дня — перенос при появлении
   мультитенантности не потребует менять модель.

## Целевая модель

| Понятие | Что это | Хранение |
|---|---|---|
| **Saved state / Commit** | неизменяемый номерной снимок Draft'а, созданный Save | `engineering.commits` (уже есть) |
| **Label** | имя-метка Organization, необязательное для saved state | `engineering.labels` + `organization_id` |
| **Label assignment** | связь «лейбл — saved state», many-to-many | `engineering.commit_labels` |
| **Import** | Project использует сохранённое состояние Library; в домене и API это по-прежнему Library Pin, в UI — Import | `engineering.library_pins` с номером Commit'а |
| **Deploy** | делает сохранённое состояние Project'а работающей Deployment | `runtime.deployments.commit_number` |
| **Deployment** | неизменяемая запись Runtime node о запуске | `runtime.deployments` (уже есть) |
| ~~Release~~ | удаляется: Project Release, Library Release, их таблицы, маршруты, события и UI | — |

## Решения

- **D1. Лейбл — отдельный агрегат, а не часть Draft.** Назначение лейбла не
  пишет DraftChange, не поднимает Version Draft, не участвует в undo/redo и не
  попадает в ProjectFile, Commit или Deployment. Это view-метаданные
  сохранённого состояния, как заметка Release была метаданными, но общие для
  всех инженеров.
- **D2. Один saved state — много лейблов, один лейбл — много saved state.**
  Фильтр в окнах — по одному выбранному лейблу («All» + каждый лейбл).
- **D3. Назначение — отдельная связь со своим репозиторием и событиями**
  (`label_assigned`, `label_unassigned`), одна транзакция на назначение.
  Удаление лейбла каскадно снимает его назначения и шлёт `label_deleted`.
- **D4. Лейблы — уровень Organization.** `organization_id` ссылается на
  `organizations(id)`; API `GET/POST/PATCH/DELETE /api/v0/labels` серверный,
  потому что Organization на сервере один. В ADR 0025 и в комментарии к
  таблице зафиксировать: при появлении выбора Organization добавить её в путь
  и в выборку, модель и таблицы не меняются.
- **D5. Deploy делает Commit.** `POST …/commits/{n}/deploy`; Deployment
  хранит `commit_number` (nullable: file-deploy и старые Deployment), Journal,
  Checkpoint, событие `deployment_created` и UI говорят `commit_number`.
  `POST …/releases/{n}/deploy` временно отвечает как обёртка (Release → его
  Commit), пока UI не переедет, затем 410.
- **D6. Импорт = закрепление Commit'а Library.** Пин хранит номер/ID
  сохранённого состояния Library; WidgetTypes читаются из содержимого этого
  Commit'а, копии по-прежнему попадают в ProjectFile. `projectv0.LibraryPin`
  получает `commit` вместо `release`; старый `release` декодируется только для
  чтения старых файлов и канонизируется в `commit`.
- **D7. Legacy-данные не теряются.** Каждый Library Release старого образца
  (origin `legacy`, без Commit) в миграции становится синтетическим Commit'ом
  той же Library: title `Legacy release N`, author `system`, время Release,
  содержимое — его `released_widget_types`. Release-backed Commit'ы
  отображаются на свои Commit'ы. Затем пины и сохранённые документы
  переписываются с release на commit. Стеки DraftChange, чьи снапшоты
  ссылаются на исчезнувшие Release, чистятся (undo-история — эфемерная);
  Commit'ы, Deployment'ы и Checkpoint'ы переписываются, а не теряются.
- **D8. Удаление Release — ломающее изменение API, но MAJOR пока 0.** Каждый
  контракт поднимает MINOR (`api/v0`, `record/v0`, `project/v0`), `make
  schemas`; удалённые маршруты отвечают `410 Gone`, как старый
  `POST …/deploy`, и это проверяется Bruno.
- **D9. Терминология.** UI: **saved state**, **label**, **import**, **deploy**;
  слово Release из UI исчезает. Домен может сохранить `librarypin` как
  механизм импорта (как `Commit` и «saved state» в задаче 84), но UI и
  документы говорят Import.

## Разбиение

| Часть | Файл | Что | Blocked by |
|---|---|---|---|
| 1 | `85_1_mode_screen_persistence.md` | Не сбрасывать экран выбора Library/Project при смене вкладок | — |
| 2 | `85_2_label_aggregate.md` | Label: домен, таблица с `organization_id`, CRUD API, глобальное окно, события | — |
| 3 | `85_3_label_assignments.md` | Назначение лейблов saved state, чипы в History, `?label=` в списке Commit'ов | 85_2 |
| 4 | `85_4_deploy_saved_state.md` | Deploy Commit'а: `commit_number` в Runtime/Journal/Checkpoint/API, обёртка старых Release-маршрутов, миграция | — |
| 5 | `85_5_deploy_window.md` | Новое окно Deploy по saved states с фильтром по лейблу; убрать Make Release и Release из History/панели Project | 85_3, 85_4 |
| 6 | `85_6_import_library_state.md` | Pin по Commit'у, `commit` в ProjectFile, миграция legacy Release → Commit, окно Import с фильтром по лейблу | 85_3, 85_4 |
| 7 | `85_7_remove_releases.md` | Удалить остатки Release (Project и Library): домен, сервисы, таблицы, маршруты 410, события, состояния Library, growctl | 85_5, 85_6 |
| 8 | `85_8_docs_tools_alignment.md` | ADR 0025 + поправки, GLOSSARY, AGENTS, CHANGELOG, схемы, Bruno, growctl, seed/simulator | 85_1–85_7 |

Порядок прогона: 85_1 → 85_2 → 85_3 → 85_4 → 85_5 → 85_6 → 85_7 → 85_8.
85_1 независима и делается первой, потому что это отдельный дефект; 85_2/85_3
дают лейблы, без которых новые окна нечем фильтровать; 85_4 меняет носитель
Deploy; 85_5 и 85_6 переключают UI на saved state; 85_7 удаляет то, что
перестало использоваться; 85_8 закрывает документы и инструменты.

## Влияние на заведённые задачи

- **45 History** — вместо `release_number` показывать `commit_number` и
  подпись «saved state N»; Release в прошлом больше нет.
- **46 PlaybackFile** — запись `deployment` несёт `commit_number`; старые
  PlaybackFile с `release_number` не читаются без пересборки (вне объёма).
- **48 Adopt**, **51 Runtime embedded Deployment**, **52 Network nodes** —
  разговор о «Release» заменить на «Commit/saved state».
- **49 User**, **50 Access rights** — автор назначения лейбла и права на
  лейблы/импорт/деплой; до них автор — объявленное имя.
- **58 CQRS** — списки Commit'ов с лейблами и фильтром — read-модель.
- **71 Release pinned by history** — защита Library от удаления остаётся, но
  проверяет живые импорты (Commit'ы) и работающую Deployment.
- **72_3/72_4/72_5/84** — их Release-механика отменяется этой задачей; файлы
  остаются в `todo/done` как история.

## Риски

- **Совместимость документов.** ProjectFile с `"release"` в пинах обязан
  читаться после переименования в `"commit"`; иначе старые Commit'ы,
  Deployment'ы и скачанные файлы перестанут открываться. Решение — legacy-поле
  при декодировании + миграция сохранённых JSONB (D6, D8).
- **Миграция данных.** Legacy Library Release без Commit'а нельзя просто
  удалить: он может быть единственной копией содержимого. Синтетический Commit
  (D7) — обязательная часть 85_6, а не «после».
- **Два окна и один экран History.** 85_5 и 85_6 меняют общий
  `ui/savedstates`; делать их одновременно нельзя, иначе конфликт в одном
  файле. 85_5 идёт первой.
- **Совместимость старых клиентов.** Удаление `released`, `latest_release`,
  `has_unpublished_changes`, `release_number` ломает старый UI; баннер версии
  и MINOR-бампы обязательны.

## Вне объёма

- Полноценный агрегат Organization, выбор Organization в UI, права на лейблы.
- Цвета/иконки лейблов, несколько лейблов в одном фильтре, поиск по имени.
- Обрезка Commit'ов (retention), как и раньше.
- Переименование домена `librarypin` → `libraryimport` (сделано только в UI и
  документах; механическая замена пакета — отдельная задача при желании).
- Пересборка внешних PlaybackFile со старым `release_number`.

## Comments

- 2026-11-14: постановка пользователя. Лейблы на saved state, окно лейблов,
  импорт Library в Project с фильтром по лейблу, окно Deploy с фильтром по
  лейблу, удаление Release, сохранение экрана выбора при смене вкладок.
  Лейблы — уровень Organization, пока серверные. Разбито на 8 частей, все
  Priority: 1.
- 2026-11-14: уточнение по коду — агрегат Organization уже существует как
  identity marker (`domain/organization`, одна строка от миграций), поэтому
  `organization_id` пишется сразу, а серверным остаётся только API.
