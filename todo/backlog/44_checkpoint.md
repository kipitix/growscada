# Checkpoint
Status: ready-for-agent
Priority: 3
Blocked by: 35, 42, 72_4
Parts: 44_1_checkpoint_storage, 44_2_checkpoint_operation_at

Слой D. Термин — `GLOSSARY.md` (Checkpoint); архитектура — ADR 0002, ADR 0019, ADR 0020, ADR 0023.

Источник истины — этот файл. Работы — `44_1_checkpoint_storage` и `44_2_checkpoint_operation_at`, в этом порядке: первая даёт запись и её писателей, вторая — read-model `?at=` для History; у второй `Closes parent: yes`.

## Модель

**Checkpoint** — неизменяемая запись операционного состояния Project на позицию Journal:

- `seq` — позиция, которую описывает снимок: состояние *после* записи на этой позиции;
- ссылка на работающую Deployment: `number`, `cause`, `rolled_back_from?`, `release_number?` — тот же payload, что у `deployment`-записи Journal, без документа;
- состояния всех тегов Project: `tag_id`, `value`, `quality`, `seq` (позиция последнего изменения тега).

Checkpoint — **не агрегат и не запись Journal**: у него нет поведения, событий и Version, он не входит в поток и не читается Engineering-нодой. Это запись read-стороны Runtime-ноды рядом с Journal, как строка outbox рядом с агрегатом (ADR 0008, ADR 0019, ADR 0023).

**Что не входит.** Документ Deployment не дублируется: он неизменяем и адресуется номером (ADR 0014, ADR 0019). Runtime-сущности задачи 48 появятся в записи отдельным MINOR, когда будут; Checkpoint, написанный до 48, читается без них — это верное состояние на момент до их появления. `written_at` тега не хранится: запись о value, Quality и позиции.

**Когда пишется.**

- При каждом появлении Deployment — Deploy Release, Deploy файла (cause `file`), Rollback — в той же транзакции, что вставка Deployment, запись Journal и перестройка runtime-копий. Позиция = `seq` записи `deployment`, вставка после `refreshCopies`, чтобы в снимок попали начальные состояния новых тегов. Неудавшаяся Deployment не оставляет Checkpoint.
- Периодически: фоновый сервис Runtime-ноды раз в минуту ищет Project'ы, у которых текущая позиция ушла не меньше чем на `CheckpointEveryEntries = 10 000` от последнего Checkpoint, и снимает по одному снимку в одной транзакции `REPEATABLE READ`: `MAX(seq)`, текущая Deployment и все `runtime.tag_states` из одного снимка; вставка `ON CONFLICT (project_id, seq) DO NOTHING`. Ошибка логируется и повторяется на следующем проходе; outbox/policy не участвуют.

**Чтение для History.** `GET …/operation?at=<position>` — тот же `OperationStateResponse`, что и текущий read-model: ближайший Checkpoint ≤ position, его Deployment с документом (форма `GET …/deployments/{n}`) и его состояния тегов; `position` в ответе — `seq` Checkpoint'а. History дочитывает `GET …/journal?from=<seq>` до нужного момента. До первого Checkpoint — пустое состояние (`position: 0`, `deployment: null`, `tag_states: {}`); position больше текущей позиции — зажим до текущей; отрицательный/нечисловой — 400 `invalid_input`; Project не жив — 404 `project_not_found`. Без `at` маршрут не меняется и читает текущее состояние: Checkpoint отстаёт на порог и задержал бы первую картину Operation. Сервер не собирает состояние на T — записи применяет клиент (ADR 0020).

**Хранение.** Все Checkpoint'ы хранятся и уходят вместе с Project (как Journal, Tag states, Deployments). Корректность от них не зависит: без снимка History проигрывает с 0. Retention/партиционирование отложены (ADR 0002, ADR 0019); PlaybackFile (46) берёт снимки нужных периодов.

## Контракт

- `contract/record/v0` (0.4 → 0.5): `Checkpoint` (встраивает `Header`; `seq int64`, `project_id uuid.UUID`, `timestamp` RFC 3339 — когда снят; `deployment *Deployment` — переиспользуется payload Journal; `tag_states []CheckpointTagState`), `CheckpointTagState{tag_id, value, quality, seq}`. `make schemas`.
- `contract/api/v0` (0.30 → 0.31): необязательный `at` у `GET …/operation` (`OperationQueryAt`); новых типов нет. `make schemas`.
- `apiclient.ProjectClient.OperationAt(ctx, position)` — для History (45); текущий `Operation` не меняется.

## За рамками

- History UI (45), PlaybackFile (46), runtime-сущности (48), retention/партиционирование.
- Серверный Player и «состояние на T» одним ответом: записи до T применяет клиент.
- Замена текущего read-model Operation (`GET …/operation` без `at`) на Checkpoint.
- Права доступа к Checkpoint (50).

## Comments

- 2026-10-04: бывший тикет 09 задачи 36. Решение Q19: Checkpoint на каждый Deploy и периодически.
- 2026-11-07: Deploy теперь активирует Release (72_4); Checkpoint фиксирует Deployment и номер Release. Заблокирован 72_4.
- 2026-11-09 (grilling; ADR 0023). Решения:
  - **Природа.** Checkpoint — отдельная неизменяемая запись рядом с Journal, не тип записи Journal и не агрегат; термин уточнён в `GLOSSARY.md` (запись read-стороны; `_Avoid_`: snapshot, backup, savepoint, cursor, offset).
  - **Состав.** Ссылка на Deployment (`number`/`cause`/`rolled_back_from`/`release_number`) + все `tag_states` с `seq`; документ Deployment не дублируется; runtime-сущности — задача 48 отдельным MINOR.
  - **Когда.** Любая Deployment (Release/файл/Rollback) — в своей транзакции, на `seq` записи `deployment`, после `refreshCopies`; периодически — по числу записей Journal (`CheckpointEveryEntries = 10 000`), скан раз в минуту, одна `REPEATABLE READ`-транзакция на Project, `ON CONFLICT DO NOTHING`.
  - **Чтение.** `GET …/operation?at=<position>` с тем же `OperationStateResponse`; до первого снимка — пустое состояние на позиции 0; будущий `at` зажимается; нечисловой/отрицательный — 400; неживой Project — 404. Без `at` — как сейчас.
  - **Хранение.** Все снимки, каскад с Project; retention отложен.
  - **Разбиение.** `44_1_checkpoint_storage` (запись, контракт, кодек/репозиторий, анкоры, фоновый сервис) и `44_2_checkpoint_operation_at` (`?at=`, `apiclient`, Bruno); последняя закрывает родителя.
- 2026-11-09 (аудит DDD по правилу 8). Расхождений нет:
  - **Агрегаты и транзакции.** Checkpoint — не агрегат: инвариантов, событий и Version нет, запись производная. Анкор пишется в транзакции Deployment (той же, что Journal-запись и `refreshCopies`); периодический снимок — одна `REPEATABLE READ`-транзакция чтения состояния и вставки производной строки, без изменения агрегатов. Outbox/policy не нужны: ADR 0008 требует их для согласования агрегатов, а не для read-модели.
  - **Роли и слои.** Только Runtime-нода (`application/runtime`, `postgres/runtime`, `contract/record/v0`); порт `application.RuntimeNode` не меняется, Engineering-нода Checkpoint не читает; `roles_test` остаётся зелёным. REST-слой Runtime-ноды добавляет `?at=`; `apiclient` — только контракт; UI History — задача 45.
  - **Термины.** Checkpoint уточнён в `GLOSSARY.md`; Journal position и Starting state не меняются.
  - **ADR.** Решение записано в ADR 0023; ADR 0019 (позиции) и ADR 0020 (стартовое состояние) не противоречат.
