# ProjectFile, Deploy, Revision, Discard
Status: ready-for-agent
Blocked by: 32, 33, 34

Слой B. Термины — `GLOSSARY.md` (ProjectFile, Deploy, Revision, Discard).

- Формат ProjectFile (контракт `contract/project/v0`, ADR 0005): весь Draft проекта — Scenes с Widgets, определения Tag, Devices — плюс содержимое используемых WidgetType из закреплённых Library Release (задача 33) и `schema_version`.
- Deploy: Engineering node сериализует Draft в ProjectFile и передаёт Runtime node; тот атомарно создаёт Revision (`runtime.revisions(project_id, n, deployed_at, config jsonb)`), присваивает номер и переключает Operation.
- WidgetType больше не действует мгновенно — только через Release (задача 33) и Deploy.
- Перед Deploy Engineer видит, какие runtime-сущности пострадают (исчезли Scene/WidgetType); такие Widgets удаляются. Конфликт имён Tag с runtime-тегом → Deploy отклоняется.
- Discard: Draft сбрасывается к текущей Revision, полученной через API Runtime node.
- Engineering node хранит копию последнего отправленного ProjectFile.
- Запись Deploy в Journal — в задаче 42.
- REST, Bruno, тесты, `CHANGELOG.md`.

**Это спецификация, не тикет.** Она уже согласована и **делать её не нужно** — работа разбита на пять частей, по одной на слой; каждая самодостаточна и проверяема.

| Часть | Файл | Что | Blocked by |
|---|---|---|---|
| 1 | `35_1_projectfile_contract.md` | Формат `contract/project/v0`: ProjectFile целиком, `SchemaVersion` 0.2, схемы | 33 |
| 2 | `35_2_revision_storage.md` | Схема: `runtime.revisions`, `engineering.project_files`, репозитории и транзакция создания ревизии | 35_1 |
| 3 | `35_3_runtime_deploy.md` | Runtime-нода: Deploy в порту, текущая Revision, событие `revision_created`, правила состояний Tag, конфиг Gateway из копий | 35_2 |
| 4 | `35_4_engineering_deploy_discard.md` | Engineering: сборка ProjectFile, Deploy/preview, Discard-policy, снятие моста Draft→Runtime | 35_3 |
| 5 | `35_5_rest_ui_docs.md` | REST, apiclient, PWA (Project и Operation), Bruno, ADR 0013 и поправки, CHANGELOG | 35_4 |

Части идут по порядку; каждая ссылается на эту спецификацию как на источник истины.

## Comments

- 2026-10-04: бывший тикет 03 задачи 36. Решения: Q1 (Draft + Deploy, а не живое редактирование), Q8 (Deploy публикует весь Draft атомарно → Revision), Q10 (Draft автосохраняется на сервере, кнопки «Сохранить» нет; явные действия — Deploy и Discard), Q15 (runtime-Widget переживает Deploy; удаляется, только если исчезли его Scene или WidgetType, с предупреждением перед Deploy), Q16 (Deploy с конфликтом имён отклоняется), Q20 (Revision — неизменяемый jsonb-документ). Зависимость от Journal снята: запись Deploy в Journal добавит задача 42.
- 2026-10-27 (grilling задачи 33): в ProjectFile копируются `ReleasedWidgetType` из закреплённых Library Pins (только используемые Widgets); до этой задачи Operation читает их через `GET …/widget-types` проекта.
- 2026-11-03 (grilling, Q1–Q14). Решения:

  **Operation и источники (Q1, Q3, Q11).** Operation читает **только текущую Revision**; правки Draft (Scene, Widget, WidgetType, пины) не видны до Deploy, WidgetType берётся из вшитых копий; до первого Deploy — пустое состояние «проект не развёрнут». Отдача: `GET /api/v0/projects/{id}/revision` → `{number, deployed_at, content: projectv0.ProjectFile}` (404, если Deploy не было) и отдельно `GET …/tag-states` — снимок `id → value/quality/seq` (значения живут только на Runtime-ноде), дальше SSE. Новое событие `revision_created` (ContentEvent проекта; публикует Runtime-нода после коммита, как `tag_updated`, без outbox) — единственный сигнал, по которому Operation перезагружает Revision; content-события Draft и `library_*` его больше не трогают. Project-режим на `revision_created` обновляет бейдж.

  **Формат ProjectFile (Q2, Q14).** `contract/project/v0`: Header `{schema_version, project_id}` (имени нет — переименование Deploy не требует) → Tag-определения (id, имя, тип, поставщик) → Devices (protocol, config) → Environments и пары (params) → Scenes с Widgets (геометрия, labels, port_bindings, type_id) → копии только используемых WidgetType из закреплённых Release + список пинов (library_id, номер release) как провенанс. Без состояний Tag, токенов, версий агрегатов и истории. Environments входят, хотя Deploy их «не выбирает»: копии `runtime.environments`/`environment_devices` заполняются из Revision, Gateway резолвит окружение по имени.

  **Runtime (Q4, Q5, Q8).** Мост Draft→Runtime убирается целиком: upsert в `CreateTag`, удаление состояния в `DeleteTagByID`, policy `sync_runtime_tag_definitions`. Deploy: новый Tag — initial, оставшийся — состояние **сохраняется**, исчезнувший — уходит с копией, смена TagType — сброс в initial; запись значения в Tag вне работающей Revision → 404. `runtime.revisions.config` (jsonb) — истина; `runtime.tags/devices/environments/environment_devices` — производные копии, пересобираются **диффом** в одной транзакции (не DELETE-всё: каскад убил бы `tag_states`). Транзакция Runtime: advisory lock по `project_id` → `n = max+1` (unique `(project_id, n)` — страховка) → insert revision → дифф копий → событие; ответ `{number, deployed_at}`. Engineering после ответа пишет `engineering.project_files`; сбой этой записи — 500 с честным текстом, Revision при этом активна и повторный Deploy не будет отклонён как «без изменений», потому что сравнение идёт с Revision через порт. Конфиг Gateway и проверка поставщика читаются из копий, Draft не участвует.

  **Копия у Engineering и preview (Q6, Q9).** `engineering.project_files(project_id PK, revision_n, deployed_at, config jsonb)` — копия вторична, удаляется с Project; «есть неразвёрнутые изменения» = канонизированное (стабильный порядок списков и ключей) сравнение байт свежесобранного ProjectFile с сохранённым. Никаких dirty-флагов. Идентичный Draft отклоняется 409 `nothing_to_deploy` (как `CreateRelease` у Library). `GET …/deploy/preview` → `{current_revision, unchanged, dropped_widgets: []}` уже сейчас; списки заполнит задача 48 (runtime-сущности, конфликт имён Tag) — форма API и диалог подтверждения фиксируются и проверяются UI-тестом.

  **Discard (Q7, Q10).** `POST …/discard`: нет Revision → 409; иначе `StartDiscard` на Project и сразу ответ, policy `NewDiscardProjectPolicy` восстанавливает Draft к текущей Revision (читается через `RuntimeNode`) в транзакциях по одному агрегату, с обычным `Save` (Version+1, штатные события), порядок: пины → Devices → Environments → пары → Tags → Scenes с Widgets (пины первыми — восстановление Scene проходит `checkPinned`); лишнее удаляется, недостающее создаётся с теми же ID; пометка снимается последней (`FinishDiscard`). Пины пишутся штатными методами агрегата, но **без `ImpactOf`** и без ожидаемой версии — сирот нет по построению. Пока пометка стоит — записи в Draft → 409. Runtime (состояния, токены) Discard не трогает.

  **Следствие по ролям (аудит DDD).** `DeviceConfigService` переезжает в `application/runtime`: конфигурация работающей Revision — данные Runtime-ноды, а не чтение Draft через порт Engineering→Runtime.

  **UI (Q12).** Project-режим: рядом с переключателем — бейдж «Revision N · deployed at T» (или «не развёрнуто»), индикатор неразвёрнутых изменений, кнопки Deploy (диалог с preview) и Discard (подтверждение). Operation: пустое состояние либо рендер Revision. Списка ревизий и дифф-экрана нет — задача 36.

  **Сид, Bruno, тесты, доки (Q13).** Сид Revision не создаёт. Bruno-коллекция сервера получает раздел `deploy` (preview → deploy → revision → tag-states) до первого `PATCH …/value`; коллекция симулятора деплоит проект перед запуском драйвера. Интеграционные тесты, пишущие значения, деплоят в setup (или в хелпере `servertest`). Документация: новый ADR 0013 (Deploy/Revision/Discard; jsonb — истина; мост убран), поправки в ADR 0007 и 0011, `CHANGELOG.md`, строка в README/`AGENTS.md` о том, что ручной прогон симулятора требует Deploy.

  **За рамками:** список ревизий и Rollback — задача 36; DraftChanges undo/redo — 38; runtime-сущности, конфликт имён Tag, непустой `dropped_widgets` — 48; сетевой `RuntimeNode` и разделение БД — 52; EditLock — 37.
- 2026-11-03 (разбиение): задача разбита на пять частей по слоям — `35_1_projectfile_contract`, `35_2_revision_storage`, `35_3_runtime_deploy`, `35_4_engineering_deploy_discard`, `35_5_rest_ui_docs`; каждая самодостаточна, проверяема и ссылается на эту спецификацию. Работу, которая по решению Q4/Q13 не входит в часть (список ревизий, runtime-сущности, EditLock, сетевой порт), не добавлять сюда.
