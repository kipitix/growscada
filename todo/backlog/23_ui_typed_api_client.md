# Типизированный клиент сервера для PWA UI
Status: ready-for-agent

Появилось в архитектурном ревью 2026-10-05. UI ходит в REST API напрямую из каждой операции и держит свои копии JSON-структур. Пересекается с задачей 29: там общий загрузчик списков, здесь — все вызовы REST, типы и ошибки.

## Что не так сейчас

- Прямые `http.*` вызовы: `ui/project/scene_ops.go` (8), `widget_ops.go` (5), `tags_ops.go` (3), `ui/library/widget_type_ops.go` (7); `uiutil.FetchJSON` умеет только GET (11 вызовов: `project/live_reload.go`, `library/library.go`, `operation/load.go`). Блок `ctx.Dispatch(… toast.NetworkError(err))` повторён 21 раз.
- `widget_ops.go:createWidget` — около 75 строк: склейка URL, `json.Marshal`, `http.Post`, проверка статуса, декодирование, 10 присваиваний полей редактора.
- Учёт Version Scene (`currentSceneVersion`, `applyNewSceneVersion`) вручную протянут через каждую операцию; версия лежит и в `scenes[i].Version`, и в `widgets[j].SceneVersion`.
- `ui/project/dto.go` (19 структур), `ui/livescene/dto.go` (8), `ui/library/dto.go`, `ui/operation/dto.go` и `eventlog.wireMessage` вручную копируют `contract/api/v0`. Новое поле Widget приходится менять примерно в 10 местах.
- `internal/apiclient` уже разбирает Problem Details, но умеет только Tags, отдаёт свой `apiclient.Tag`, а из ошибки остаётся только склеенная строка — Title/Detail для toast из неё не достать. UI его не использует.
- `apiclient` ставит `GrowSCADA-Schema-Version` на каждый запрос, UI (`uiutil.SendSchemaVersion`) — только на запросы с телом. Версию сервера из заголовка ответа не читает ни один клиент.
- Сервер описывает Problem Details своим `restapi.ProblemDetails`, а не типом контракта.
- Операции UI не покрыты тестами, покрытие UI-пакетов около 1%.

## Решения (grilling 2026-10-13)

### Контракт и apiclient

- `apiclient` расширяется на Scenes, Widgets и WidgetTypes и возвращает типы `apiv0` как есть. `apiclient.Tag` заменяется на `apiv0.TagResponse` (затрагивает growctl, devicelink). Методы создания возвращают ответ сервера (`CreateTag` → `apiv0.CreateTagResponse`: UI нужен ID).
- `apiv0.Problem` — пять полей RFC 9457 (`Type`, `Title`, `Status`, `Detail`, `Instance`), без Extensions. restapi отвечает им; `restapi.ProblemDetails` с ручным Marshal/Unmarshal удаляется. JSON на проводе не меняется. Полевые расширения — задача 59. SchemaVersion API 0.1 → 0.2, `make schemas`.
- `apiclient.StatusError` несёт `apiv0.Problem` и Method; `Error()` и `errors.Is(ErrNotFound/ErrConflict)` не меняются.
- `contract` остаётся пакетом форм — без кодеков. Весь клиентский разбор в `apiclient.do`, на stdlib:
  - тело запроса — `json.Marshal(apiv0.…Request)`;
  - успешный ответ — терпимый `Decode` в `apiv0.…Response` (читатель по ADR 0005);
  - ответ с ошибкой — терпимый `Decode` в `apiv0.Problem`; тело не JSON (HTML прокси) → `Title` = `StatusText`, `Detail` = начало тела.
- Строгий `decodeStrict` остаётся в restapi; в общий пакет — когда появится второй получатель (Runtime node, задача 35).
- Заголовок `GrowSCADA-Schema-Version` в запросе — только при теле: сервер читает его лишь в `decodeRequest`, а на GET он вызывал бы CORS preflight. `uiutil.SendSchemaVersion` удаляется.
- Версия сервера из ответа: опция `apiclient.OnServerVersion(func(contract.SchemaVersion))`, `do` вызывает её на каждом ответе (включая ошибки), если заголовок разобрался. Клиент состояния не хранит.
  - UI: root подписывается (через Dispatch); при любом расхождении с версией UI — незакрываемая плашка «Сервер говорит на API X, интерфейс — Y. Перезагрузите страницу» с кнопкой; исчезает, когда версии совпали. Работа не блокируется.
  - growctl: сервер старше — одно предупреждение в stderr за запуск; сервер новее — тишина.
  - devicelink/devicesim: сервер старше — запись в лог раз на смену версии сервера; сервер новее — тишина.
- `apiclient` собирается под `GOOS=js GOARCH=wasm` (проверено); новых зависимостей в `app.wasm` нет.

### UI

- Все вызовы REST из UI (Project, Library, Operation, включая GET) идут через один `*apiclient.Client`, созданный в `main`/`root` из `apiServerURL` и переданный компонентам полем. `uiutil.FetchJSON` и прямой `net/http` к REST из UI удаляются. Обвязка Reloader для списков остаётся как есть — её обобщение в задаче 29.
- UI-копии DTO удаляются: Project, Library, Operation и `livescene` работают на типах `apiv0` напрямую. `livescene.Quality` приводится на месте использования.
- SSE: eventlog один раз терпимо разбирает `data:` в `apiv0.Event`; `ServerEvent.Tag` становится `*apiv0.TagResponse`. `eventlog.wireMessage` и `operation.decodeTagUpdate` удаляются.
- ID в состоянии UI — `uuid.UUID`, «не выбрано» — `uuid.Nil`. Строки парсятся только на границах: LocalStorage, SSE, атрибуты DOM/drag.
- Version Scene — одно место в Project (map sceneID → известная версия), обновляется из любого ответа (список Scenes, список Widgets, результат записи) по максимуму; записи берут версию только оттуда. `applyNewSceneVersion` и чтение `widgets[j].SceneVersion` для записи уходят.
- `toast.Problem` остаётся моделью показа; строится одной `toast.FromError(err)`: `errors.As` → `*apiclient.StatusError` → поля `apiv0.Problem` + Method, иначе Network Error. `toast.FromHTTPError` и json-теги `toast.Problem` удаляются, `toast.ClientError` остаётся.
- Обвязка операций — generic `uiutil.Call(ctx, call func(context.Context) (T, error), onOK func(app.Context, T), onErr func(app.Context, error))`: Async → вызов → Dispatch через `compoCtx` компонента; `onErr == nil` → toast `FromError`. Project передаёт свой `onErr` для Scene и Widget (409 → `showRejectedSceneEdit`).

### Тесты

- Без шва и in-memory адаптера: фейк дублировал бы правила сервера (версии Scene, 409, валидация, reconcile). UI держит `*apiclient.Client` напрямую, тесты Project и Library идут против настоящего API на `servertest.StartAPI`; его очистку расширить на scenes, widgets, widget_types.
- Компоненты в тестах гоняются через `app.NewTestEngine()` (go-app v10: `Load`, `ConsumeAll` выполняет Async/Dispatch). **Первый шаг — спайк**: заводится ли TestEngine на host с OnMount Project/Library (SSE, LocalStorage). Если нет — остановиться и вернуться к вопросу (запасной вариант: чистый слой операций без `app.Context`).

## Что сделать

1. Спайк TestEngine + `servertest` для Project и Library.
2. `apiv0.Problem` в контракте, restapi на нём, SchemaVersion 0.2, `make schemas`.
3. `apiclient`: Scenes, Widgets, WidgetTypes; `apiv0.TagResponse` вместо `apiclient.Tag`; `CreateTag` возвращает ответ; `StatusError` с `apiv0.Problem`; фолбэк для не-JSON ошибки; заголовок только при теле; `OnServerVersion`. Поправить growctl, devicelink, devicesim.
4. growctl и devicelink: предупреждения о более старом сервере.
5. UI: клиент из `main`/`root`; `uiutil.Call`, `toast.FromError`; перевести Project, Library, Operation, eventlog на клиент и `apiv0`; ID → `uuid.UUID`; единый учёт Version Scene; плашка версии в root. Удалить `uiutil.FetchJSON`, `uiutil.SendSchemaVersion`, `toast.FromHTTPError`, UI-копии DTO, `wireMessage`, `decodeTagUpdate`.

## Приёмка

- Поведение UI не меняется. В браузере: создание, изменение, удаление Scene, Widget, Tag и WidgetType работает, ошибки показываются toast'ом с title/detail сервера, 409 на Scene — toast «Scene changed» и перечитывание.
- В UI нет прямых `net/http`-вызовов к REST API и UI-копий типов `contract/api/v0`.
- Тесты на TestEngine + `servertest` покрывают все операции `*_ops` Project и Library (включая сохранение после drag, bind/unbind порта, rename) и сценарии ошибок: 409 (второй клиент правит Scene между чтением и записью), 400 с detail сервера, сетевая ошибка (клиент на закрытый порт). Тест плашки версии.
- Unit-тесты: учёт Version Scene (max), `toast.FromError`, новые методы `apiclient`, терпимое чтение ответа с лишним полем, разбор JSON-Problem и фолбэк для HTML, `OnServerVersion` (заголовок есть / нет / битый), предупреждения growctl и devicelink.
- Bruno-коллекции не меняются (форма запросов и ответов та же).
- `make build` и `make test` проходят.

## Comments

- 2026-10-13 (grilling): in-memory адаптер и интерфейс-шов из первоначального плана отменены в пользу `servertest`; добавлены `apiv0.Problem`, чтение версии сервера из ответа и перевод чтений (включая Operation и SSE) на клиент.
