# Player и перевод Operation на Journal
Status: ready-for-agent
Blocked by: 43_1
Parent: 43_player_operation
Part: 2/2
Closes parent: yes

Часть 2 задачи 43. Источник истины — `todo/backlog/43_player_operation.md` (грилинг 2026-11-07, ADR 0020). Снимок `GET …/operation` уже сделан в 43_1. Здесь — клиентский Player и перевод режима Operation на Journal.

## Что сделать

### `ui/player` — операционная модель

- Новый пакет `internal/server/interface/ui/player` (UI-слой, roles не затрагивает).
- Модель Player:
  - документ работающей Revision (`RevisionResponse` / `projectv0.ProjectFile`), номер Revision;
  - состояния тегов по ID (`value`, `quality`, `seq`, `written_at`), включая теги, которых нет в текущей конфигурации;
  - позиция Journal;
  - состояние соединения (`Connected`).
- Маппинг `ProjectFile` → рендер-формы `livescene.View` (`apiv0.SceneResponse`, `WidgetResponse`, `ReleasedWidgetTypeResponse`, `TagResponse`) переезжает из `operation/load.go`/`matrix.go` в `player`: сцены с виджетами, WidgetTypes по ID, теги по ID только текущей конфигурации. Тег без состояния — zero-значение его типа с Quality `bad`, `seq` 0 (после смены Revision новые теги не приходят из снимка).
- Применение записей `contract/record/v0.Entry`:
  - `tag_state`: если `entry.TagState != nil` и `seq` больше известного — обновить состояние; запись для тега вне конфигурации сохранить по ID, но не рисовать;
  - `revision`: если номер больше текущего — `GET …/revisions/{n}`, заменить конфигурацию, состояния по ID сохранить; старые/повторные записи игнорировать;
  - неизвестный `type` — игнорировать (читатель нового формата не ломается);
  - `schema_version` записи сверить по MAJOR (чужой MAJOR — запись не применять).
- Источник записей — интерфейс (`Start`/`Entries`/`Connected`/`Close` или эквивалент): модель не знает, живой поток или перемотка. `Operation` даёт живой источник; `History` (45) позже — свой.
- Применение — сразу; перерисовка запрашивается не чаще короткого интервала (пачка записей → один Update). Порядок применения сохраняется.

### Живой источник

- `apiclient`:
  - `ProjectClient.Operation(ctx)` уже есть из 43_1;
  - построение URL `GET …/journal/events?from=<position>` и разбор `data:` в `recordv0.Entry` (терпимо к неизвестным полям; `id:` не нужен — позиция берётся из записи и снимка).
- Старт/ресинк: `GET …/operation` → модель, затем SSE с `?from=position`. Полный resync на **каждое явное открытие** (монтирование, смена Project, реконнект после CLOSED); нативный retry EventSource с тем же `from` — точный догон из таблицы, без обязательного нового снимка; если после `onopen` позиция снимка больше стартовой позиции потока — переоткрыть поток с новой позиции.
- Ошибки:
  - `GET …/operation` 404 — Project удалён → `uiutil.ActionProjectGone` (как списки проекта);
  - сбой загрузки документа Revision при `revision`-записи — считать потерей связи: старая конфигурация на экране, `Connected=false`, полный resync с backoff;
  - обрыв SSE — `Connected=false`, переоткрытие как у Bar (нативный retry, затем явный через задержку).
- Без периодического resync.

### Operation

- `operation.Operation` больше не читает `GET …/revision`, `GET …/tag-states` и не обрабатывает `eventlog.ActionServerEvent`/`EventProjectResync`; `livescene.View` получает модель из Player, `Connected` — из Player.
- `SetProject` создаёт/перезапускает Player и поток; `OnDismount` закрывает; выбор сцены, вкладки, LocalStorage и тема остаются в Operation.
- Project без Revision (`revision == null`) — пустое состояние «Project is not deployed»; первый Deploy приходит `revision`-записью и переключает экран без перезагрузки.
- Смена Project/Deploy/Rollback: выбранная сцена сохраняется по ID, если есть, иначе первая; пустая Revision — «No scenes to show».
- `Bar` и `/events` не меняются; Operation их не читает (остальные режимы — читают).

### Тесты и проверка

- Юнит-тесты `player`: `tag_state` по `seq` (меньший/равный игнорируется), запись тега вне конфигурации, `revision` по номеру (повтор/старый игнорируется, состояния сохраняются, новые теги zero/bad, удалённые не рисуются), неизвестный тип, чужой MAJOR, маппинг ProjectFile → рендер-формы, смена сцены, батчинг обновлений.
- UI-тест Operation через `servertest` (`ui/uitest`): проект без Revision → пустое состояние; Deploy → сцены и значения; `PATCH …/value` двигает значение без перезагрузки; второй Deploy/Rollback меняет конфигурацию; консоль без ошибок.
- Ручная проверка в браузере (правило 9): Operation показывает живые значения, переживает Deploy и переключение Project; баннер при остановленном сервере.
- `make bruno_test` (маршрут уже покрыт в 43_1).

### Документация

- `AGENTS.md`: раздел Operation/UI — Player, источник (`/operation` + `/journal/events`), resync, отказ от `tag_updated`/`revision_created` в Operation.
- `CHANGELOG.md`.
