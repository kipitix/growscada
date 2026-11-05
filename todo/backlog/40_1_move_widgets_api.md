# Групповой перенос виджетов: домен, application и REST

Status: ready-for-agent
Blocked by: 38
Parent: 40_multi_select_move_objects
Part: 1/2

Часть 1 задачи 40. Источник истины — `todo/backlog/40_multi_select_move_objects.md`; здесь серверная часть. Решения — ADR 0010, ADR 0016, ADR 0017.

**За рамками:** выделение, рамка, перетаскивание и стрелки в PWA (40_2).

## Домен (`internal/server/domain/scene`)

- Новое значение `WidgetMove`: ID виджета и новая `Position`. (Имя и форма — на усмотрение реализации; важно, что перенос несёт только ID и Position.)
- `Scene.MoveWidgets(moves []WidgetMove) (Scene, []Widget, error)`:
  - для каждого элемента находит виджет по ID; нет — `ErrWidgetNotFound` (существующая ошибка), повтор ID — `ErrWidgetMovedTwice`;
  - строит новый Widget с новой `Position` и всеми прежними полями: имя, размер, origin, поворот, WidgetType, метки, PortBindings;
  - `z` берётся из сохранённой позиции, присланные `x`/`y` — новые;
  - записывает по `NewWidgetUpdatedEvent(w.ID(), s.id, s.projectID, w.TypeID())` на каждый сдвинутый виджет, в порядке `moves`;
  - виджет, чья позиция уже равна запрошенной, не меняется и события не даёт;
  - если не сдвинулся ни один — `ErrNothingToMove`, ничего не записано;
  - возвращает новую Scene и список фактически сдвинутых виджетов (нужен сессии для снимков after).
- `WidgetType`, `TagTypes` и PortBindings не читаются и не проверяются: перенос меняет только геометрию, сломанный биндинг не может отказать (ADR 0010).
- `UpdateWidget` и остальные операции Scene не меняются.

## Draft (`internal/server/domain/draft`)

- `Session.MoveWidgets(ctx, sceneID, moves)`:
  - загружает Scene один раз;
  - для каждого запрошенного ID снимает before (`SnapshotOfWidget(sceneID, stored)`);
  - вызывает `Scene.MoveWidgets`;
  - для каждого фактически сдвинутого пишет after (`SnapshotOfWidget(sceneID, moved)`);
  - `recordEvents(changed.PendingEvents()...)`;
  - ошибка домена возвращается как есть — репозиторий откатывает транзакцию.
- Снимки и события — те же, что у `UpdateWidget`; кодирование и хранение DraftChange не меняются (ADR 0017).

## Application (`internal/server/application`)

- `SceneService.MoveWidgets(ctx, projectID, sceneID uuid.UUID, version int, moves []appdto.WidgetMove) (int, error)` в `scene_application_service.go`; `appdto.WidgetMove{WidgetID uuid.UUID; X, Y float64}`.
- Порядок как у `UpdateWidget`: `ProjectContent.open` (live Project и Organization), `draftVersion`, разбор входа, `ProjectContent.Drafts().Update` с описанием `"Move N widgets"` (N — `len(moves)`), в замыкании — `Session.MoveWidgets`.
- Валидация входа: пустой `moves` — `invalidInput`; повтор `widget_id` — `invalidInput`; невалидный UUID ловит REST-декодер. Ошибку `scene.ErrWidgetMovedTwice` из домена сервис тоже оборачивает в `invalidInput`.
- Ответ — `result.Version().Number()`; ошибки маппятся как у `UpdateWidget` (`widgetSaveError`/`widgetChangeError`), плюс `scene.ErrNothingToMove`.

## REST (`internal/server/interface/restapi`)

- Обработчик `PatchWidgets` в `widgets.go`; маршрут в `router.go`: `PATCH <project path>/scenes/{scene_id}/widgets`.
- Контракт `contract/api/v0/widget.go`:
  - `MoveWidgetsRequest{Version int; Moves []WidgetMoveRequest}`;
  - `WidgetMoveRequest{WidgetID uuid.UUID; X float64; Y float64}`;
  - `MoveWidgetsResponse{Version int}`.
  - `SchemaVersion` `api/v0` — MINOR, 0.19 → 0.20; `make schemas` (схема формата `project/v0` не меняется).
- `errors.go`: `{scene.ErrNothingToMove, 409, sceneResource, "Nothing to move: the widgets already hold these positions"}`. 400 — через `invalidInput`, 404 — существующая строка `scene.ErrWidgetNotFound`, 409 Draft/EditLock — существующие.
- Тело декодируется `decodeRequest` (незнакомое поле — 400).

## Bruno (`tests/api/bruno_collections/growscada_server`)

Существующие версии в коллекции не трогаем: запросы переноса добавляются после `post_projectfile_open_invalid` (seq 144), где Draft ещё существует, но Scene уже удалена. Блок самодостаточный:

- `post_scene_for_move.yml` (seq 145) — создать Scene, `vars:postresponse` сохраняет `MOVE_SCENE_ID`; тело — с текущей Version (25).
- `post_widgets_for_move_1.yml` (seq 146) и `post_widgets_for_move_2.yml` (seq 147) — создать два виджета, сохранить `MOVE_WIDGET_ID_1`/`MOVE_WIDGET_ID_2`; версии 26 и 27.
- `patch_widgets_move.yml` (seq 148) — PATCH обе позиции, 200 `{version: 29}`, `GrowSCADA-Schema-Version` определён.
- `patch_widgets_move_nothing.yml` (seq 149) — те же позиции повторно, 409 `nothing_to_move`.
- `patch_widgets_move_stale.yml` (seq 150) — устаревшая Version, 409 `draft_conflict`.
- `patch_widgets_move_invalid.yml` (seq 151) — пустой `moves`, 400.
- `patch_widgets_move_missing.yml` (seq 152) — случайный ID виджета, 404.

Если `vars:postresponse` в коллекции окажется неудобен, допустим другой самодостаточный способ получить ID; важно не сдвигать версии существующих запросов. Новый маршрут добавляется в `requestBodies` статической проверки (`bruno_collection_internal_test.go`) типом тела.

## Тесты

- Домен: двигаются только `x`/`y` и только у указанных виджетов; поля виджета сохраняются; по событию на сдвинутый; `z` не меняется; повтор ID — `ErrWidgetMovedTwice`; неизвестный ID — `ErrWidgetNotFound`; все позиции совпадают — `ErrNothingToMove` и Scene не изменилась.
- Application: одна DraftChange на N виджетов с описанием и N снимками; Version +1; пустой список и повтор — invalid input; чужая Scene/Project — 404; без EditLock — 409; `nothing_to_move` — 409 и Version не выросла.
- Репозиторий: одна строка `draft_changes`, before/after каждого виджета, undo возвращает все позиции одной операцией и публикует `widget_updated` по каждому.
- REST: 200 и `{version}`; 400 (пусто, повтор, незнакомое поле); 404 (виджет не из Scene); 409 (`nothing_to_move`, устаревшая Version, без EditLock); заголовок схемы.
- Статическая проверка Bruno (`make test`) и прогон коллекции (`make bruno_test`).

## Документы

- `AGENTS.md`: маршрут в разделе REST API, `Scene.MoveWidgets`/`Session.MoveWidgets` в описании домена и application, Bruno-блок.
- `CHANGELOG.md`: запись о части 1 (домен, сервис, маршрут, контракт, Bruno).
