# События Widget несут SceneID, имена событий — в контракте
Status: ready-for-agent

Появилось в архитектурном ревью 2026-10-05. Сейчас форма живых событий заставляет UI перечитывать больше, чем изменилось. Корректная остановка SSE-соединений — отдельная задача 26. Решения приняты на grilling 2026-10-15; перевод доменных событий в контракт — ADR 0009.

## Что не так сейчас

- В событиях Widget на проводе нет ID Scene (в домене он есть: `scene.WidgetEvent.SceneID()`). `eventlog.ReloadTargetsFor` на любое событие Widget перечитывает список Scene (`GET /scenes`) и виджеты выбранной Scene.
- `restapi/event_hub.go` пишет на провод `EventType.String()`: доменные имена (они же формат outbox) неявно служат именами API. `eventSourceID` и `eventTag` — отдельные type switch.
- Строки типов событий заданы в UI в двух местах: `ui/eventlog/reload.go` и `event_types.go`, плюс литерал `"tag_updated"` в Operation (отмечено и в задаче 29).
- Project перечитывает весь `GET /tags` на каждый `tag_updated` (перенесено из задачи 29).
- Клиент разбирает SSE в `eventlog.parseEvent` с фолбэком на «конверт», где поля перечислены вручную, и рассылает режимам UI-копию события `ServerEvent` с `ID string`.

## Контракт (`contract/api/v0`)

- `Event` получает поле `SceneID string json:"scene_id,omitempty"`; его несут только `widget_created`, `widget_updated`, `widget_deleted`.
- Имена всех 15 типов событий (включая `system_ready`) — нетипизированные string-константы (`EventTagCreated = "tag_created"`, …) и `EventTypes() []string`. `Event.Type` остаётся `string`, схема его не меняется.
- MINOR-bump SchemaVersion (0.2 → 0.3), `make schemas`.

## Сервер (`restapi`)

- Явная таблица перевода `event.EventType → apiv0`-имя (ADR 0009); домен `contract/` не импортирует.
- `eventSourceID` и `eventTag` сливаются в один переводчик `newAPIEvent(e event.Event) apiv0.Event` (отдельный файл): имя — из таблицы, ID/SceneID/Tag — switch по семействам событий (`TagEvent`, `WidgetEvent`, …). `system_ready` — без ID.
- Тест: образец на каждый тип из `event.AllEventTypes()` (тип без образца роняет тест), сверка всех полей результата; каждая константа `apiv0.EventTypes()` использована в таблице.

## Разбор на клиенте

- `apiclient.ParseEvent(data string) (apiv0.Event, bool)`: один проход в типы контракта — структура со встроенным `apiv0.Event` и `tag json.RawMessage`; ошибка разбора `tag` обнуляет только `Tag`, остальные поля сохраняются без перечисления вручную. Невалидный JSON, скаляр не той формы, пустой `type` → `false`. `TestParseEvent_ReadsTolerantly` переезжает в apiclient и дополняется случаями со `scene_id`.
- `eventlog.Bar.handleMessage`: `ParseEvent` → запись журнала из того же `apiv0.Event` (id как пришёл) → обнуление `Tag`, если его ID нулевой или не совпадает с `id` события → `ActionServerEvent` со значением `apiv0.Event`. Resync — `apiv0.Event{Type: EventResync}`. `ServerEvent`, `parseEvent` и конверт удаляются.
- Режимы работают с `apiv0.Event` и константами `apiv0`, своих строк типов нет. `system_ready` добавляется в `eventTypeInfoByType` (`categoryOther`, без перечитывания).
- `EventResync` — синтетический тип UI; тест проверяет, что его нет среди `apiv0.EventTypes()`.

## Маршрутизация и реакция режимов

- `eventlog.ReloadTargetsFor(ev apiv0.Event)`; `ReloadTargets` получает поле `Scene uuid.UUID` — одна Scene, чьё состояние и Widgets устарели. Это единственное место, где разбирается `scene_id` (`uiutil.ParseID`). `widget_*` → `{Scene: X}`; без `scene_id` или с неразборчивым — фолбэк `{Scenes, Widgets}`.
- `tag_updated` с телом тега → пустые `ReloadTargets`, режимы применяют тег сами; без тела → `{Tags}`. Тест «каждый тип что-то перечитывает» получает для этого случая исключение.
- **Project** на `{Scene: X}`: `GET /scenes/X` (`apiclient.GetScene`, свой Reloader с ключом по Scene), ответ вливается в `p.scenes` (Observe версии в `sceneVersions`, merge редактора свойств); виджеты перечитываются, только если X выбрана. 404 и X, которой нет в `p.scenes`, молча игнорируются: состав списка меняют только `scene_*` и resync.
- **Operation** на `{Scene: X}`: виджеты, только если X выбрана; список Scene не перечитывается.
- **`tag_updated`**: `applyTagUpdate`, `mergeTags`, `forgetVersions` переезжают из `operation/state.go` в общий ui-пакет (рабочее имя `ui/tagstate`, map + упорядоченный срез для показа) с правилом «побеждает большая Version». Project применяет тег из события к списку и панели свойств, Operation — как сейчас; оба сливают загруженный список по тому же правилу и забывают версии на resync.

## Bruno

- `tests/api/bruno_collections/growscada_server/get_events.yml`: в docs пример `widget_*` со `scene_id`.

## Приёмка

- Юнит-тесты: `apiclient.ParseEvent`, `newAPIEvent` (полнота по `AllEventTypes`), `ReloadTargetsFor` (Scene, фолбэк, `tag_updated`), `tagstate`, проверка тела тега в Bar.
- Тест сверяет константы контракта с `eventTypeInfoByType`.
- uitest Project с подсчётом запросов через `http.Client` в `mountWith`:
  - событие Widget в невыбранной Scene → только `GET /scenes/X`;
  - в выбранной → `GET /scenes/X` и её виджеты;
  - после удаления последнего Widget невыбранной Scene правка её свойств проходит без 409;
  - `tag_updated` не вызывает `GET /tags`.
- В браузере по сетевым запросам: изменение Widget в одной Scene не перечитывает остальные Scene; с `make run_simulator_dashboard` Project не перечитывает теги на каждое значение.
- `make build` и `make test` проходят.
