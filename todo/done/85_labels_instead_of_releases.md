# Лейблы, импорт библиотек и деплой сохранённого состояния вместо релизов

Status: ready-for-agent
Priority: 1
Parts: 85_1_mode_screen_persistence, 85_2_label_aggregate, 85_3_label_assignments, 85_4_deploy_saved_state, 85_5_deploy_window, 85_6_import_library_state, 85_7_remove_releases, 85_8_docs_tools_alignment

Слой C (Draft, Commit, публикация) и слой D (UI). Термины — `GLOSSARY.md`
(Saved state, Commit, Label, Library Import, Deploy, Deployment); меняются
решения ADR 0007, 0012, 0014, 0018, 0019, 0021 и 0023, добавляется ADR 0025 о
лейблах.

## Постановка (как понято и уточнено грилингом)

1. Новая сущность **Label** — именованная метка с необязательным цветом,
   которую можно назначать на **любое сохранённое состояние** (Commit/saved
   state) Project'а и Library.
2. Отдельное окно для добавления, переименования, перекраски и удаления
   лейблов.
3. Импорт Library в Project: вместо «pin library» — окно **Import**, в котором
   сохранённые состояния Library фильтруются по лейблу.
4. Новое окно **Deploy**: сохранённые состояния Project'а фильтруются по
   лейблу перед выбором того, что деплоить.
5. Из модели убирается **Release** — и Project Release, и Library Release.
   Deploy делает сохранённое состояние, Import закрепляет сохранённое
   состояние Library.
6. Переключение вкладок Library ↔ Project не сбрасывает экран выбора; выбранный
   экран (страница или редактор) сохраняется и после перезагрузки браузера.
7. Лейблы хранятся на уровне Organization: `organization_id` пишется с первого
   дня, API пока серверный, потому что Organization на сервере одна.
8. Два стартовых лейбла — `ready_for_import` и `ready_for_deploy` — создаются
   миграцией как обычные, редактируемые и удаляемые метки. Импорт и деплой от
   них не зависят.

## Решения грилинга

### Лейблы

- **Label — один агрегат, назначения — его сущности.** Label владеет
  множеством назначений (label — saved state). Создание, переименование,
  перекраска, assign, unassign и удаление — по одной транзакции на Label, как
  требует ADR 0008. Назначения не грузятся для переименования, но моделью
  принадлежат Label.
- **У сохранения может быть несколько лейблов; фильтр — по одному.** В окнах
  Import и Deploy выбирается `All labels` или один лейбл; мультифильтра нет.
- **Назначение — в History.** У строки сохранения чипы и «+ label»; окно
  Labels управляет только именами и цветами. Назначение не пишет DraftChange,
  не поднимает Version Draft, не попадает в undo/redo и не входит в
  ProjectFile, Commit или Deployment.
- **Удаление лейбла каскадно снимает его назначения**, с предупреждением в UI;
  отдельного запрета «лейбл используется» нет.
- **Удаление Project/Library каскадно снимает назначения его Commit'ов через
  БД.** Отдельные `label_unassigned` не шлются: клиенты перечитывают историю по
  `project_deleted`/`library_deleted`. Это задокументированное исключение из
  ADR 0008 в ADR 0025.
- **Label: имя + необязательный цвет.** Имя — trim, 1–100 рун, без переводов
  строк; уникально в Organization без учёта регистра (`Prod` = `prod` →
  409). Цвет — `#RRGGBB` или пусто (нейтральный цвет темы); чип всегда
  показывает имя, цвет не является единственным различием. События:
  `label_created`, `label_updated` (имя и/или цвет), `label_deleted`,
  `label_assigned`, `label_unassigned`.
- **События лейблов идут в серверный поток.** Назначение Project'а называет
  scope и номер Commit'а в payload, но маршрутизируется как серверное событие:
  так его видят и History Project'а (Bar с `?project=` получает и серверный
  поток), и глобальное окно Labels в любом режиме.
- **Окно Labels — глобальное, в шапке**, рядом с переключателем темы; доступно
  во всех режимах.
- **Стартовые лейблы.** Миграция 85_2 вставляет `ready_for_import` и
  `ready_for_deploy` в единственную Organization с нейтральным цветом,
  `ON CONFLICT DO NOTHING`. Они обычные: переименовываются, перекрашиваются и
  удаляются; миграция их не восстанавливает.
- **growctl:** минимальные `labels` (`list`, `create`, `rename`, `delete`,
  `color`) и `label assign/unassign --commit N --label <name|id>`; лейблы
  показываются в `commits`. Полного манифеста лейблов нет.

### Release, Deploy и Import

- **Deploy делает Commit.** `POST …/commits/{n}/deploy`; Deployment хранит
  `commit_number` (nullable: file-deploy и старые Deployment). Journal,
  Checkpoint, `deployment_created` и UI говорят `commit_number`. Deploy file
  больше не создаёт Release; причина `file`, `commit_number = nil`.
- **Import = сохранённое состояние Library.** Проект импортирует Commit
  Library; WidgetTypes читаются из его содержимого, копии по-прежнему попадают
  в ProjectFile. Пины переименовываются полностью: `librarypin` →
  `libraryimport`, таблица `library_imports`, маршруты
  `/projects/{id}/library-imports`, DTO `LibraryImport*`, события
  `library_import_*`, `ProjectFile.library_imports` с полем `commit`.
- **Release удаляется полностью.** Нет Project Release и Library Release, их
  домена, таблиц, сервисов, событий, полей и UI. Старые маршруты `/releases`
  **удаляются** (404), без заглушек 410.
- **Legacy Library Release без Commit: удалить вместе с импортами.**
  Синтетических Commit'ов нет. Пины на legacy Release снимаются; копии их
  WidgetTypes и Widget'ы этих типов удаляются из Draft'ов. Это осознанная
  потеря данных, отражённая в CHANGELOG.
- **Старые документы переписываются.** Commit'ы, Deployment'ы и **работающая
  Deployment** очищаются от пинов на удалённые legacy Release, копий их
  WidgetTypes и Widget'ов этих типов. Operation сразу перестаёт показывать эти
  Widget'ы. Это одноразовое исключение из неизменяемости Commit/Deployment,
  зафиксированное в ADR 0021.
- **Все стеки undo очищаются.** Снапшоты DraftChange ссылаются на старую
  нумерацию Release; после миграции они неприменимы. Контент, Commit'ы и
  Deployment'ы это не затрагивает.
- **Внешний ProjectFile** с пином на удалённый legacy Release при Open
  отклоняется 400 с `file_issues`; пины на commit-backed Release нормализуются
  в commit и открываются. Старые поля `library_pins`/`release` читаются как
  legacy и канонизируются в `library_imports`/`commit`.
- **growctl:** `save` остаётся, `release`/`releases` удаляются, `deploy
  --commit`; манифест `spec.libraryPins` → `spec.libraryImports` с `commit`.
- **Контракты:** `api/v0`, `project/v0`, `record/v0` — MINOR-бампы, `make
  schemas`; `project/v0` получает `library_imports`/`commit`, `record/v0` —
  `commit_number`.

### Режимы

- `Root.SetMode` не пересчитывает экран уже открытой моды. `showList` хранится
  в LocalStorage (`library:screen`, `project:screen`) и восстанавливается при
  загрузке; отсутствие выбранной сущности или её удаление всегда показывают
  страницу выбора.

## Целевая модель

| Понятие | Что это | Хранение |
|---|---|---|
| **Saved state / Commit** | неизменяемый номерной снимок Draft'а, созданный Save | `engineering.commits` (уже есть) |
| **Label** | имя + необязательный цвет, принадлежит Organization; владеет назначениями | `engineering.labels` + `organization_id` |
| **Label assignment** | сущность Label: метка на одном saved state | `engineering.commit_labels` |
| **Library Import** | Project использует сохранённое состояние Library | `engineering.library_imports` с Commit'ом |
| **Deploy** | делает сохранённое состояние Project'а работающей Deployment | `runtime.deployments.commit_number` |
| **Deployment** | неизменяемая запись Runtime node о запуске | `runtime.deployments` (уже есть) |
| ~~Release~~ | удалён: Project Release, Library Release, их таблицы, маршруты, события и UI | — |

## Разбиение

| Часть | Файл | Что | Blocked by |
|---|---|---|---|
| 1 | `85_1_mode_screen_persistence.md` | Экран выбора Library/Project сохраняется при смене вкладок и перезагрузке | — |
| 2 | `85_2_label_aggregate.md` | Label: имя + цвет, `organization_id`, seed двух лейблов, CRUD API, глобальное окно, события | — |
| 3 | `85_3_label_assignments.md` | Назначения как сущности Label, чипы в History, `?label=`, growctl assign | 85_2 |
| 4 | `85_4_deploy_saved_state.md` | Deploy Commit'а; `commit_number` в Runtime/Journal/Checkpoint/API; миграция | — |
| 5 | `85_5_deploy_window.md` | Новое окно Deploy по saved states с фильтром по лейблу; Release из Project UI | 85_3, 85_4 |
| 6 | `85_6_import_library_state.md` | Полное переименование pin → import, Commit в ProjectFile, удаление legacy Release, перепись документов, окно Import | 85_3, 85_4 |
| 7 | `85_7_remove_releases.md` | Удалить остатки Release, маршруты 404, состояния Library, growctl manifest | 85_5, 85_6 |
| 8 | `85_8_docs_tools_alignment.md` | ADR 0025 и поправки, GLOSSARY, AGENTS, CHANGELOG, схемы, Bruno, seed | 85_1–85_7 |

Порядок: 85_1 → 85_2 → 85_3 → 85_4 → 85_5 → 85_6 → 85_7 → 85_8.
Каждая часть оставляет `make build`, `make test` и (где менялся REST/UI)
`make bruno_test` зелёными.

## Влияние на заведённые задачи

- **45 History** — вместо `release_number` показывать `commit_number` и
  «saved state N»; Release в прошлом больше нет.
- **46 PlaybackFile** — запись `deployment` несёт `commit_number`; старые
  PlaybackFile с `release_number` не пересобираются (вне объёма).
- **48 Adopt**, **51 Runtime embedded Deployment**, **52 Network nodes** —
  «Release» заменить на «Commit/saved state».
- **49 User**, **50 Access rights** — автор назначения лейбла и права на
  лейблы/импорт/деплой; до них автор — объявленное имя.
- **58 CQRS** — списки Commit'ов с лейблами и фильтром — read-модель.
- **71 Release pinned by history** — защита Library от удаления остаётся, но
  проверяет живые импорты и работающую Deployment.
- **72_3/72_4/72_5/84** — их Release-механика отменяется; файлы остаются в
  `todo/done` как история.

## Риски

- **Потеря данных legacy Release.** Принята осознанно: legacy Release без
  Commit'а удаляется с пинами и Widget'ами, а старые документы переписываются.
  CHANGELOG обязан перечислить это до обновления сервера.
- **Неизменяемость истории.** Одноразовая перепись Commit/Deployment —
  исключение; ADR 0021 фиксирует его и причину.
- **Миграция JSONB.** Нужно переписать `commits.content`, `deployments.config`,
  Checkpoint/Journal payload'ы, ProjectFile-пины и таблицы импортов; покрыть
  тестом на контейнере со всеми тремя видами данных.
- **Два окна и общий History.** 85_5 и 85_6 меняют `ui/savedstates`; 85_5 идёт
  первой, чтобы не конфликтовать.
- **Полное переименование.** `librarypin` → `libraryimport` затрагивает
  домен, таблицы, снапшоты, события, API, ProjectFile и growctl; делать
  механически, отдельными коммитами внутри части.

## Вне объёма

- Полноценный агрегат Organization, выбор Organization в UI, права на лейблы.
- Мультифильтр по лейблам, поиск по имени, иконки лейблов.
- Обрезка Commit'ов (retention).
- Пересборка внешних PlaybackFile со старым `release_number`.

## Comments

- 2026-11-14: постановка пользователя: лейблы на saved state, окно лейблов,
  импорт Library с фильтром, окно Deploy с фильтром, удаление Release,
  сохранение экрана выбора. Разбито на 8 частей, все Priority: 1.
- 2026-11-14 (грилинг): лейблов несколько на сохранение; фильтр по одному;
  назначение в History; окно Labels глобальное; имя + необязательный цвет;
  уникальность без учёта регистра; `organization_id` с первого дня; seed
  `ready_for_import`/`ready_for_deploy`; удаление лейбла каскадное; назначения
  — сущности Label; каскад при удалении Project/Library — исключение ADR 0008.
- 2026-11-14 (грилинг): Deploy делает Commit; Import — Commit; полное
  переименование pin → import; Release удаляется, маршруты 404; legacy Release
  удаляется с пинами и Widget'ами без синтетических Commit'ов; старые
  Commit/Deployment/работающая Deployment переписываются; все стеки undo
  очищаются; внешние файлы с удалённым пином отклоняются; growctl — минимальные
  команды лейблов и `deploy --commit`.
- 2026-11-14 (грилинг): экран выбора сохраняется и после перезагрузки
  (LocalStorage); аудит DDD чист, исключения зафиксированы в ADR 0025/0021.
