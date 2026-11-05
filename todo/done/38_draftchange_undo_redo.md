# DraftChange и undo/redo

Status: ready-for-agent
Parts: 38_1_draft_aggregate_project, 38_2_draft_library, 38_3_undo_redo_rest, 38_4_undo_redo_ui
Blocked by: 35, 37

Слой C. Термины — `GLOSSARY.md` (Draft, DraftChange, Version, EditLock). Решения — ADR 0016, ADR 0017.

**Это спецификация, не тикет.** Работа разбита на четыре части, по одной на область и слой; каждая самодостаточна и проверяема. Источник истины — этот файл.

## Модель

### Агрегат Draft

- Draft — единственный агрегат области: Draft проекта или Draft Library. Всё содержимое области — его сущности, а не отдельные корни: Scene с Widgets, определение Tag, Device, Environment, пара Environment Device, Library Pin, WidgetType. Project и Library (имя, удаление, Release) остаются отдельными агрегатами: это жизненный цикл, а не правка Draft.
- `engineering.drafts` — одна строка на область: собственный `id`, две nullable FK (`project_id → projects`, `library_id → engineering.libraries`, CHECK «ровно одна», обе `ON DELETE CASCADE`, по уникальному индексу), `version`, поля замка (`holder_key`, `holder_name`, `held_since`, `last_seen`, `lock_version`), `stack_position`, `created_at`, `updated_at`. Существующие Project и Library получают строку миграцией; новый — при первом обращении.
- Содержимое остаётся в своих таблицах (`engineering.scenes`, `widgets`, `tags`, `devices`, `environments`, `environment_devices`, `library_pins`, `widget_types`): Draft их композирует, но не переписывает в один документ. Запись открывает одну транзакцию над Draft, загружает только затронутые сущности, проверяет замок и `version`, пишет затронутые строки, DraftChange и outbox и коммитит. Граница агрегата — транзакция, а не полный граф в памяти.
- Домен — `domain/draft.Draft` со `Scope` (Project или Library), `Version`, состоянием замка, стеком и операциями над содержимым; сущности остаются в своих пакетах (`scene`, `tag`, `device`, `library`, `librarypin`) и не импортируют `draft`. Загрузку сущностей в Draft делает транзакционный загрузчик репозитория; домен SQL не знает.
- Одна `version` на всё содержимое Draft: она заменяет версии Scene, Tag, Device, Environment, пары, Pin и WidgetType. Поле `version` в DTO содержимого `api/v0` несёт именно её — одно и то же число во всех сущностях одного ответа. Project и Library сохраняют свои версии (переименование и удаление), EditLock — свой `lock_version`, растущий только при смене держателя, чтобы Take over оставался возможен во время правок держателя.
- EditLock переезжает в строку Draft: `engineering.edit_locks` удаляется, проверка в транзакции записи читает строку Draft, presence и реапер не меняются (ADR 0015). Взятие, отдача и перехват замка пишут ту же строку, но `version` содержимого не поднимают.
- Переходные блокировки уходят: Scene↔Library Pin и WidgetType↔Library становятся инвариантами внутри транзакции Draft (смена Pin снимает сломанные привязки в той же DraftChange, запись Widget проверяет закреплённый релиз в той же транзакции). Через агрегаты остаётся только удаление Library против Pin проекта — Library вне Draft любого проекта.

### DraftChange

- Любая успешная запись содержимого Draft — ровно одна DraftChange, включая многосущностные эффекты одного намерения: удаление Tag снимает его привязки, смена закреплённого Library Release снимает сломанные привязки, удаление Scene уносит её Widgets. Создание, изменение и удаление Tag, Scene, Widget, Device, Environment, пары, Pin и WidgetType — всё это DraftChange. Не DraftChange: переименование и удаление Project/Library, Deploy, Release, Discard, взятие/отдача/перехват замка и сами undo/redo.
- `engineering.draft_changes`: `draft_id → drafts ON DELETE CASCADE`, `seq`, `description`, `before jsonb`, `after jsonb`, `created_at`, PK `(draft_id, seq)`. `before`/`after` — полное состояние каждой затронутой сущности с её видом и ID (у созданной или удалённой сущности одна сторона пуста); кодировка внутренняя, как у outbox (ADR 0005), со своей версией формата. `description` — короткая строка, сохраняемая в момент правки («Delete tag pressure»), её UI показывает как следующий шаг undo/redo.
- Undo пишет `before` изменения на позиции стека, redo — `after`; оба поднимают `version` Draft и публикуют обратные события (undo удаления Tag — `tag_created` и `widget_updated` для Scene, чьи привязки вернулись). Undo/redo не записываются как DraftChange.
- Стек живёт на Draft: `stack_position`; новая правка после undo удаляет redo-хвост и дописывает изменение на позицию. Глубина — 100 изменений, более старые строки удаляются и не откатываются. Check in, Check out и перехват стек не трогают: следующий держатель откатывает правки предыдущего.
- Стек чистится публикацией: после успешного Deploy (Revision создана Runtime node) и после успешного Release (строки релиза закоммичены) — отдельной транзакцией, потому что Deploy уходит на Runtime node, а Library Release — другой агрегат (ADR 0007, ADR 0008). Сбой очистки — 500 при уже созданной Revision/Release и оставшемся стеке. Discard чистит стек в транзакции сброса Draft. Удаление Draft уносит строки по FK.

### События

- Прямая правка пишет прежние события (`tag_created`, `widget_updated`, `library_pin_updated`, …) — по ним UI перезагружает списки, а SSE-потоки Project и Library работают как раньше.
- Undo/redo пишут обратные события с восстановленным состоянием; отдельного `draft_changed` нет.
- `NewUnbindDeletedTagPolicy` и `NewWidgetReconciliationPolicy` удаляются: их работу делает транзакция DraftChange. `NewProjectCleanupPolicy` и `NewLibraryCleanupPolicy` остаются — удалённый Project/Library вне своего Draft. `DiscardProject`/`DiscardLibrary` и события `project_discard_started`/`library_discard_started` удаляются вместе с пометками.

### Discard

- Project Discard читает текущую Revision через порт Runtime node, затем одной транзакцией заменяет всё содержимое Draft из неё, чистит стек и коммитит; Library Discard делает то же из последнего Release. Пометки `projects.discarding` и `libraries.discarding_to`, политики Discard и ответ 202 уходят; Discard синхронный, 200. Гейт — EditLock; `project_discarding`/`library_discarding` больше не существуют.
- Revision, прочитанная до транзакции, — та, к которой сбрасывается Draft; конкурентный Rollback между чтением и записью не отменяет сброс (Rollback меняет Revision на Runtime node, Draft не читает).

### REST

- `version` в теле update и в теле/параметре create и delete — Version Draft; устаревшая — 409. Create-тела получают `version`, DELETE — параметр `version`; update-тела сохраняют поле, меняя смысл. Deploy и Discard тел не меняют; Release получает `version` — Version Draft, которую видел клиент, — и проверяет её в транзакции релиза. Все три гейтятся EditLock.
- `GET …/draft-changes` → `{version, can_undo, can_redo, undo_description?, redo_description?}`; `POST …/draft-changes/undo` и `…/redo` с телом `{version}` → то же состояние после операции. Оба под путём Project и Library, гейтятся EditLock; устаревшая версия или пустой стек — 409.
- `POST …/discard` → 200 с состоянием Draft вместо 202 `{discarding: true}`; `Project` и `Library` теряют поле `discarding`/`discarding_to`.
- Контракт `api/v0` — MINOR: `make schemas`; Bruno-коллекции обновляются в каждой части.

### UI

- В шапке Project и Library — кнопки Undo/Redo, `Ctrl+Z`/`Ctrl+Shift+Z`, подсказка с описанием следующего шага; без замка и при пустом стеке кнопки выключены.
- Состояние стека читается `GET …/draft-changes` при загрузке, на `EventResync` и после каждой правки; undo/redo обновляют содержимое по событиям и версию по ответу.
- UI держит одну Version Draft на Draft вместо версий Scene/WidgetType; `lock_version` замка остаётся отдельным.

## Разбиение

| Часть | Файл | Что | Blocked by |
|---|---|---|---|
| 1 | `38_1_draft_aggregate_project.md` | Схема `drafts`/`draft_changes`, домен Draft и DraftChange, слияние EditLock, миграция всего содержимого Draft проекта, одна Version, удаление политик, синхронный Discard, REST/Bruno | 35, 37 |
| 2 | `38_2_draft_library.md` | Draft Library: WidgetTypes, Release, Discard, версия и политики Library, REST/Bruno | 38_1 |
| 3 | `38_3_undo_redo_rest.md` | Сервис undo/redo, REST-маршруты `draft-changes`, контракт, Bruno, growctl | 38_2 |
| 4 | `38_4_undo_redo_ui.md` | UI: кнопки, `Ctrl+Z`/`Ctrl+Shift+Z`, описание шага, одна Version Draft | 38_3 |

Ветка `38_draftchange_undo_redo` начинает с 38_1. Части идут по порядку; каждая ссылается на этот файл как на источник истины. Документы (`ADR 0016`, `ADR 0017`, `GLOSSARY.md`, `AGENTS.md`, `CHANGELOG.md`) обновляются по частям: 38_1 — агрегат, схема, Project; 38_2 — Library; 38_3 — маршруты; 38_4 — UI.

## Comments

- 2026-10-04: бывший тикет 05 задачи 36. Решения: Q11 (стек undo/redo — на сервере, у Draft), Q12 (стек живёт до Deploy/Discard; Check in EditLock его не трогает; глубина ограничена), Q13 (шаг undo = одна DraftChange). Расширено: тот же механизм у Draft Library.
- 2026-10-06 (DDD-ревью задачи 20, ADR 0008): атомарная DraftChange над несколькими агрегатами требует Draft как агрегата (единственный писатель под EditLock). Тогда Scene, WidgetType и определения Tag — сущности внутри Draft, а не корни; пересмотреть механизмы задачи 20, рассчитанные на конкуренцию корней: CAS по каждой Scene, `retryOnRace`, политики по Scene в отдельных транзакциях. Правило «одна транзакция — один агрегат» остаётся, меняется размер агрегата.
- 2026-10-27 (grilling задачи 33, ADR 0012): с Draft-агрегатом снимаются переходные блокировки строк — Scene↔Library Pin (сироты) и WidgetType↔Library (Discard только для чтения); Discard Library выполняется одной транзакцией над Draft, без политики. Смена Library Pin — одна DraftChange.
- 2026-10-27 (код-ревью задачи 33): Release проверяет Version Library в своей транзакции (под блокировкой строки Library), но эта Version не покрывает Draft: запись WidgetType её не поднимает, поэтому Release может снять снимок Draft, которого клиент не видел (правка или удаление WidgetType между чтением клиента и Release). С Draft-агрегатом Release должен ожидать Version Draft.
- 2026-11-05 (grilling, Q1–Q16). Решения:
  - **Агрегат (Q1, Q12, Q15).** Draft — единственный агрегат области с корневой строкой `engineering.drafts`; содержимое — его сущности в существующих таблицах; один `domain/draft.Draft` со `Scope` для Project и Library; выборочная загрузка сущностей внутри транзакции; граница живёт в доменном Draft, репозиторий открывает транзакцию и пишет затронутые строки, DraftChange и outbox.
  - **Хранение изменения (Q2).** DraftChange — строка с полными before/after снимками затронутых сущностей (не дифф и не обратные команды), указатель позиции в Draft, redo-хвост обрезается новой правкой.
  - **Версия (Q3, Q10, Q16).** Одна Version Draft — токен всех записей; `version` в DTO содержимого несёт её; версии сущностей уходят; create и delete тоже несут Version Draft (поле в теле create, параметр у DELETE).
  - **События (Q4).** Прямые правки — прежние события, undo/redo — обратные; `UnbindDeletedTag` и `WidgetReconciliation` удаляются, cleanup остаются.
  - **EditLock (Q5).** Свёрнут в строку Draft; отдельный `lock_version` для Take over, чтобы перехват был возможен во время правок держателя.
  - **Стек (Q6, Q14).** Чистится после успешного Deploy и Release (отдельной транзакцией) и в транзакции Discard; глубина 100; Check in/out не трогает; сбой очистки — 500 при уже созданной Revision/Release.
  - **Discard (Q7).** Синхронный, одной транзакцией над Draft; пометки, политики Discard и 202 уходят.
  - **Гранулярность (Q8).** Каждая успешная запись содержимого — одна DraftChange с сохранённой строкой-описанием; rename/delete Project и Library — не DraftChange.
  - **REST (Q9).** `GET …/draft-changes` состояния + `POST …/undo|redo` с `{version}`; оба под путём Project и Library, гейт — EditLock.
  - **Разбиение (Q13).** Четыре части: Draft проекта, Draft Library, undo/redo REST, UI.
