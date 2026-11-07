# Player и режим Operation
Status: ready-for-agent
Blocked by: 35, 42
Parts: 43_1_operation_read_model, 43_2_player_operation

Слой D. Термины — `GLOSSARY.md` (Player, Operation, History, Starting state); архитектура — ADR 0014, ADR 0019, ADR 0020.

Источник истины — этот файл. Работа разбита на две части: `43_1_operation_read_model` (Runtime-нода: модель чтения состояния Operation) и `43_2_player_operation` (клиентский Player и перевод Operation на Journal). Части идут по порядку, вторая блокирована первой и закрывает родителя.

## Задача

- Player на клиенте: начальное состояние (Revision + runtime-сущности + значения тегов на позицию Journal) + применение записей Journal.
- Operation — Player, закреплённый на «сейчас» (записи по SSE); рендер Scenes текущей Revision с живыми значениями.
- Реализация Player не должна знать, живой поток или перемотка — это нужно для задачи 45 (History).
- Временный Operation поверх текущих таблиц — задача 16: заменить источник данных на Revision + Journal, переиспользовать её компонент «Scene с живыми значениями».
- Рекомендация (CQRS): начальное состояние Player отдавать отдельной моделью чтения, а не через агрегаты Draft. Один запрос — Scenes текущей Revision с Widgets, их WidgetTypes и значениями Tag на позицию Journal, в форме, нужной Player. Сейчас Operation собирает это из четырёх запросов (`/widget-types`, `/scenes`, `/scenes/{id}/widgets`, `/tags`). Revision (jsonb) и Journal — уже модели чтения Runtime node; поверх них нужен query-слой (задача 58, ступень 1), а не репозитории агрегатов. Проекции с eventual consistency (ступень 2) не нужны, пока чтение не станет узким местом.

## Решения (грилинг 2026-11-07)

**Модель чтения (Q1, Q6, Q7, Q13).** Runtime-нода отдаёт `GET /api/v0/projects/{id}/operation` → `{position, revision, tag_states}`: текущая позиция Journal, документ работающей Revision (`RevisionResponse` с `content: projectv0.ProjectFile`, `null` до первого Deploy) и состояния всех её тегов (ненаписанный тег — zero-значение его TagType с Quality `bad`, `seq` 0). Живой Project всегда отвечает 200, удалённый — 404. Модель чтения **не** собирает рендер-формы: CSS-матрица и содержимое WidgetType — геометрия Scene, домен Engineering-ноды (ADR 0011), и Runtime-ноде его импортировать нельзя; маппинг `ProjectFile` → `livescene` остаётся в UI.

**Порядок снимка (Q1).** Внутри ответа позиция читается первой, затем Revision, затем состояния тегов. Deploy, закоммиченный между чтениями, имеет позицию выше снимка — поток его отдаст; если читать позицию последней, Deploy мог бы навсегда пропасть из Operation. Записи, уже вошедшие в состояния, придут из потока повторно — это безвредно: запись несёт полное состояние, побеждает больший `seq` тега (и больший номер Revision). Отдельная транзакция/REPEATABLE READ не нужна.

**Player (Q2, Q8, Q9).** Player — клиентский механизм в `ui/player`: владеет операционной моделью (конфигурация Revision, состояния тегов по ID, позиция Journal, состояние своего потока) и применяет записи. Источник записей — интерфейс: `Operation` даёт живой источник (снимок + SSE), `History` (45) позже — перемотку; сам Player не знает, живой поток или нет. Operation остаётся тонким режимом: выбор сцены, вкладки, LocalStorage и рендер через `livescene.View`. `Connected`/баннер берутся из собственного потока Player, а не из Bar. Записи применяются сразу, но перерисовка запрашивается не чаще короткого интервала — пачка изменений даёт один рендер.

**Живой поток (Q3).** Operation подписывается на `GET …/journal/events` с позиции снимка; `tag_state` применяется по `seq`, `revision` — по номеру (загрузка `GET …/revisions/{n}`, смена конфигурации, состояния по ID сохраняются; новые теги — zero/bad, удалённые больше не рисуются). Неизвестные типы записей игнорируются. `/events` не меняется: `tag_updated` и `revision_created` остаются для Project/Library и Device Gateway, Operation их больше не читает. Судьба этих событий на `/events` — отдельная задача.

**Старт и разрыв (Q4, Q10, Q11).** При каждом открытии (монтирование, реконнект, смена Project) Player заново берёт снимок `GET …/operation` и подключает поток с его `position`. Сбой загрузки документа Revision при `revision`-записи считается потерей связи: старая конфигурация остаётся на экране, показывается disconnected, полный resync повторяется. Периодического resync нет: редкий пропуск живой доставки (ADR 0019) лечится следующей записью того же тега или переподключением.

**Контракты и объём.** `contract/api/v0` — MINOR 0.22 → 0.23 (`OperationPath`, `OperationStateResponse`); `contract/record/v0` не меняется; миграций нет. Runtime-сущностей (задача 48) ещё нет: снимок и записи несут конфигурацию Revision и состояния тегов, их типы и поля добавятся в 48. За рамками: History (45), Checkpoint (44), PlaybackFile (46), проекции CQRS (58), удаление `tag_updated`/`revision_created` из `/events`, перевод панели тегов Project-режима на Journal.

## Части

| Часть | Файл | Что | Blocked by |
|---|---|---|---|
| 1 | `43_1_operation_read_model.md` | Runtime-нода: `GET …/operation`, контракт, сервис, apiclient, Bruno, тесты | — |
| 2 | `43_2_player_operation.md` | `ui/player` + перевод Operation на `/journal/events`, UI-тесты, доки | 43_1 |

## Comments

- 2026-10-04: бывший тикет 08 задачи 36. Решение Q18: Operation и History — один Player.
- 2026-10-11 (grilling задачи 21): добавлена рекомендация по CQRS — начальное состояние Player через query-слой задачи 58.
- 2026-11-07 (grilling, Q1–Q13). Решения:

  **Модель чтения (Q1, Q6, Q7, Q13).** Начальное состояние — один запрос `GET …/operation` Runtime-ноды: `{position, revision, tag_states}`. `revision` — `RevisionResponse` с ProjectFile (`null` до первого Deploy), `tag_states` — все теги Revision, ненаписанные как zero/bad/seq 0. Живой Project — всегда 200, удалённый — 404. Конфигурация отдаётся как `projectv0.ProjectFile`, а не рендер-формами: матрица и WidgetType — домен Scene Engineering-ноды, Runtime-ноде его импортировать нельзя (ADR 0011). Порядок чтения внутри снимка — позиция, Revision, состояния: иначе Deploy, закоммиченный между чтениями, мог бы пропасть; повторы из потока безвредны (full-state, больший `seq`/номер побеждает).

  **Player и Operation (Q2).** Player в `ui/player` владеет операционной моделью (конфигурация, состояния тегов по ID, позиция, состояние потока) и применяет записи; источник записей — интерфейс, поэтому History (45) переиспользует Player с перемоткой. Operation — тонкий режим: выбор сцены, вкладки, LocalStorage, рендер `livescene.View`.

  **Поток (Q3).** Operation переходит на `GET …/journal/events`; `tag_state` — по `seq`, `revision` — по номеру через `GET …/revisions/{n}`. `/events` и его `tag_updated`/`revision_created` остаются как есть для остальных режимов и Gateway.

  **Старт, разрыв, сбои (Q4, Q8–Q11).** Полный resync на каждое открытие (монтирование, реконнект, смена Project), затем поток с позиции снимка; периодического resync нет. `Connected` — от собственного потока Player. Сбой загрузки документа Revision — disconnected + полный resync. Записи применяются сразу, перерисовка — пачкой (не чаще короткого интервала). Неизвестные типы записей игнорируются; состояния тегов, которых нет в конфигурации, хранятся по ID, но не рисуются.

  **Разбиение (Q12).** Задача делится на `43_1_operation_read_model` (серверная модель чтения) и `43_2_player_operation` (Player и Operation); вторая закрывает родителя. Контракт API — MINOR 0.22 → 0.23, миграций и изменений `record/v0` нет. Архитектура — ADR 0020; термины Operation, History, Starting state — в `GLOSSARY.md`.

  **За рамками:** History (45), Checkpoint (44), PlaybackFile (46), проекции CQRS (58), удаление `tag_updated`/`revision_created` из `/events`, панель тегов Project-режима на Journal.

- 2026-11-07 (аудит DDD). Расхождений нет: модель чтения — Runtime-ноды (`application/runtime`), Engineering-нода её не читает и порт `RuntimeNode` не расширяется; импортов `domain/scene`, `domain/library` и чужой схемы нет; запись, транзакции, политики и события не добавляются, поэтому ADR 0008 и ADR 0011 соблюдены. Revision остаётся истиной (ADR 0014), позиции — Journal (ADR 0019), контракт API поднимается на MINOR (ADR 0005). Термины Operation, History и Starting state добавлены в `GLOSSARY.md`, Player уточнён.
