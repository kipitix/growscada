# growscada [CHANGELOG](https://keepachangelog.com/en/1.1.0/)

## [0.0.47] - 2026-10-25

### Added

- **Organization и Project: `project_id` везде** (задача 31, ADR 0006)
  - Домен: агрегат `project.Project` (имя уникально среди живых проектов Organization; `Rename` с ожидаемой Version, `Delete` только помечает), события `project_created`/`project_updated`/`project_deleted`; `organization.Organization` — пока только ключ. `Tag` и `Scene` несут неизменяемый `ProjectID`, их события и события Widget — тоже (`project.ContentEvent`). `scene.TagTypes` читаются для Project: Widget привязывается только к Tag своего проекта, чужой — «tag not found» (400 на запись, удаление привязки политикой)
  - Миграция `20261025000000_organizations_and_projects`: таблицы `organizations` (стартовая Organization) и `projects` (частичный `UNIQUE (organization_id, name) WHERE deleted_at IS NULL`); `project_id` в `tags`, `scenes`, `widgets` (FK Widget → `(scene_id, project_id)`), FK без CASCADE; `UNIQUE (project_id, name)` у тегов вместо `uq_tags_name`; всё существующее — в проект «Main»; строки outbox и ключи ждущих заданий политик получают проект
  - Шов `application.ProjectScopes`: `ForProject(p)` → `Tags()`, `Scenes()`, каждый запрос с `project_id`, запись агрегата чужого проекта — ошибка. `ProjectContent` проверяет, что проект жив и принадлежит Organization сервера, прежде чем открыть его содержимое
  - Политика `remove_deleted_project`: по `project_deleted` (а также `tag_created`/`scene_created` — гонка с созданием) удаляет Scenes, затем Tags, каждое своей транзакцией со своими событиями, строку проекта — последней. `unbind_deleted_tag` и `reconcile_widgets_with_widget_type` работают через репозитории проекта события; обновлённый WidgetType сверяет Scenes всех проектов (`FindProjectsUsingWidgetType`)
  - REST API **0.4**: `GET/POST /api/v0/projects` (`?name=`), `GET/PUT/DELETE /api/v0/projects/{project_id}`; Tags, Scenes, Widgets — под `/api/v0/projects/{project_id}/…`; несуществующий или удалённый проект — 404 проекта. SSE: `/api/v0/events` — проекты, WidgetTypes, подключения, с `?project=<id>` — плюс содержимое этого проекта (не найденный проект ничего не добавляет); `/api/v0/projects/{id}/events` — только содержимое проекта. `apiclient`: методы проектов и `Client.Project(id)` → `ProjectClient`
  - growctl (манифесты **0.2**): `--project`, контекст `growctl config use-project`/`current-project` (файл `GROWCTL_CONFIG`, по умолчанию `growctl/config.yaml` в каталоге настроек пользователя), необязательное `metadata.project` (контекст — только значение по умолчанию, как namespace в kubectl: проект манифеста главнее; расхождение с `--project` или разные проекты в одной команде — ошибка), `kind: Project` (apply создаёт до тегов, `--prune` проекты не трогает, а манифест из одних проектов не удаляет теги, `delete -f` удаляет), `get projects [-o yaml]`; `get tags -o yaml` пишет `metadata.project`; `kind: Project` и `metadata.project` в документе `growscada/v0.1` — ошибка с просьбой поднять `apiVersion`; apply выполняет шаги тегов в проекте, против которого строился план
  - devicelink и симулятор: обязательное `project:` в конфигурации, проект ищется по имени при старте; нет такого — `ExitConfig` (78)
  - UI: переключатель проекта в шапке (список, создание, переименование, удаление с подтверждением), выбор в LocalStorage, удалённый выбранный проект — выбирается первый; Project и Operation показывают выбранный проект, Library от него не зависит; Bar держит одно SSE-соединение `/events?project=<выбранный>` и переоткрывает его при смене проекта (`EventResync`, затем `EventProjectResync`)
  - Seed: содержимое — в «Main», плюс пустой проект «Line 2»; тест накатывает миграции и seed на чистую БД
  - Тесты: агрегат Project, изоляция репозиториев и сервисов, миграция существующих данных, outbox, политика удаления (включая гонку с созданием), сверка по всем проектам, REST и маршрутизация SSE по проектам, growctl (проекты, контекст, расхождение `metadata.project`), devicelink/симулятор с проектом, переключатель и смена проекта в UI
  - Bruno: запросы проектов (`get projects`, `… by name`, `… by id`, `post projects`, `… duplicate name`, `put project`, `delete project`, `get project events` — manual), пути содержимого под `projects/{{PROJECT_ID}}`, новый порядок `seq`

### Changed

- ADR 0004: пути devicelink под проектом; AGENTS.md: слой проектов, маршрутизация SSE, переключатель проекта

## [0.0.46] - 2026-10-23

### Added

- **Bruno-коллекции проверяются автоматически** (задача 30). Основными тестами остаются Go-тесты; две проверки не дают коллекциям отстать от API
  - **Статическая сверка** (`TestBrunoCollection` в `restapi` и `devicesim`, часть `make test`, без Docker — идёт и на GitVerse): новый пакет `internal/brunotest` читает запросы коллекции с подставленными переменными окружения `localhost` и обслуживает их ServeMux из паттернов маршрутов. Каждый запрос должен попасть в обработчик маршрута (ответ ServeMux редиректом 301 на неочищенный путь или путь без завершающего слэша — не попадание), каждый маршрут — быть покрыт запросом, каждое тело — строго декодироваться в тип из `requestBodies` теста. У маршрута POST/PUT/PATCH без записи в `requestBodies` и у записи без маршрута — ошибка. Неизвестная переменная в URL — ошибка, в теле остаётся как есть (как в Bruno); динамические `{{$…}}` не подставляются. Коллекция ищется от `go.mod` вверх, так что сверка работает и под `-trimpath`
  - Маршруты `restapi/router.go` и `devicesim/control.go` собраны в таблицы `routes()`, ServeMux регистрирует их циклом; паттерны не менялись
  - **Прогон** — `make bruno_test` (только GitHub CI, отдельный job): `Dockerfile` (target `server` — сервер и `web/app.wasm`, target `simulator`; у каждого своя стадия сборки, кэш сборки Go — cache mount) и `tests/api/docker-compose.yaml` (свежая БД без volume, миграции и seed, сервер, симулятор с `seed_dashboard.yaml`, healthcheck'и для `up --wait`); обе коллекции идут через `npx @usebruno/cli@4.2.1` с `--env localhost --exclude-tags=manual`, контейнеры удаляются при любом исходе, код выхода — от `bru`
  - В каждом запросе обеих коллекций — assert'ы: статус, заголовок `GrowSCADA-Schema-Version` (сервер), `application/problem+json` у ошибок, одно-два ключевых поля у чтений. Порядок `seq`: чтения, изменения, удаления в конце; версии в телах следуют этому порядку на seed-данных (Scene 1 → 4, Tag 1 → 2, WidgetType 1 → 2)
  - Теги Bruno: `manual` — вне прогона (`get_events`: SSE-поток не завершается), `invalid-body` — тело, которое сервер должен отвергнуть, без проверки тела (`post_tags_newer_client`). `post_tags_invalid_quality` проверяется полностью: тело корректно, отказ приходит из домена, assert на `quality` в `detail`
  - `TAG_NAME` коллекции `device_simulator` — seed-тег `max_connections`
- Конфигурация сервера: `DATABASE_URL` (по умолчанию прежний DSN) и `PUBLIC_API_URL` — адрес REST API для браузера, передаётся PWA как `API_URL`. Без него UI ищет API на порту 9090 хоста, с которого загружена страница (раньше — всегда `http://localhost:9090`), так что один образ работает за любым адресом. Завершающий слэш обрезается, IPv6-хост берётся в скобки
- `AGENTS.md`: `make bruno_test`, обе проверки и теги в «API Collections», правило 5 дополнено (assert'ы, порядок `seq`, `requestBodies`)

### Changed

- Ошибки REST API приходят с `Content-Type: application/problem+json` (RFC 7807) вместо `application/json`; тела те же, поэтому SchemaVersion `contract/api/v0` не поднимался. Ошибки отправляет `sendProblem(w, problem)`, статус берётся из `Problem.Status`; `sendJSONResponse` — только для успешных ответов
- Тесты `restapi` поднимают Postgres при первом обращении к БД, так что тесты без БД (сверка коллекции) идут без Docker. Если подготовка БД не удалась, контейнер останавливается до паники

### Fixed

- Healthcheck Postgres в `tests/api/docker-compose.yaml` и `tools/debug_db/docker-compose.yaml` — `pg_isready -h 127.0.0.1`: через Unix-сокет он мог отрапортовать готовность временного сервера init образа, ещё не слушающего TCP, и миграции падали с `connection refused`. В `debug_db` имена базы и пользователя берутся из `.env`
- Bruno: документация `get_events` описывает 503 как `application/problem+json`

## [0.0.45] - 2026-10-22

### Changed

- **go-app v10.1.11 → v11.0.8**: импорты `github.com/maxence-charriere/go-app/v11`. Код UI не менялся: в v11 сеттеры атрибутов (`Style`, `ID`, `Href`, `Title`, …) и `Page.Set*` принимают готовую строку, форматирующие варианты вынесены в `…f` (`Stylef`, `IDf`, …), а проект формат в них не передавал

## [0.0.44] - 2026-10-21

### Changed

- **Рефакторинг живой перезагрузки UI** (задача 29), поведение не меняется
  - `project/live_reload.go` разделён по причинам для изменений: `server_events.go` (`handleServerEvents`), `load.go` (загрузчики списков WidgetType, Scene, Widget, Tag), `scene_versions.go` (версии и состояния Scene, перечитывание устаревших Scene), `edit_fields.go` (слияние правок редакторов). `live_reload_ops_test.go` → `server_events_test.go`
  - `project/edit_fields.go`: `widgetEditFields` и новая парная `sceneEditFields` — `*EditFieldsOf`, чистые `merge*EditFields`, обёртки `set*EditFields`/`sync*`/`merge*EditingFields`; `syncSceneEditingFields` переехал из `scene_ops.go`
  - Library по той же схеме: `server_events.go` (обработчик событий вынесен из `OnMount`), `load.go` (`loadList`, `setWidgetTypes`), `edit_fields.go` (`editFields`, чистые `editFieldsOf`/`mergeEditFields`; `selectItem` и `mergeSelected` — обёртки, порты сливаются целым списком)
  - Тесты чистых функций слияния: `project/edit_fields_test.go` (Widget и Scene), `library/edit_fields_test.go`

## [0.0.43] - 2026-10-21

### Added

- **Контракт скрипта WidgetType `widgetscript` и точное значение входа `text`** (задача 28, ADR 0005)
  - Новый версионируемый контракт `contract/widgetscript/v0` (package `widgetscriptv0`), SchemaVersion **0.1**, схема `schemas/widgetscript/0.1.json`: типы `Inputs` и `Input` переехали из `uiutil`. `Input.Value` — тип `Value` (`IntegerValue`/`BooleanValue`/`StringValue`, нулевой — ключа `value` нет, `omitzero`); его `JSONSchema` перечисляет JS-типы (`integer`, `boolean`, `string`), так что смена типа, которым передаётся TagType, меняет схему. Скрипту версия не передаётся
  - `inputs.<port>` = `{ value, text }`: `text` — значение Tag строкой в канонической форме сервера для всех TagType (`"9007199254740993"`, `"true"`, сама строка). У порта без значения (нет PortBinding, Tag удалён) `text` нет — виджет отличает «значения нет» от настоящих `0`/`false`/`""`. `value` не изменился
  - Превью Library и Project строят вход по type hint порта: допустимый ввод даёт `value` и `text` в канонической форме (`+007` → `7`/`"7"`), пустой или недопустимый (`abc` в integer, `yes` в boolean) — порт без значения; в порт «any» непустой ввод идёт строкой как есть
  - `uiutil`: `NewInput(text, type)`, `NoValueInput(typeHint)`, `PreviewInputs`; `livescene.WidgetInputs` строит входы через них
  - `docs/widget_type_contract.md`: версия и схема контракта, поле `text`, предел точности Integer ±(2^53 − 1) и пример с `BigInt(inputs.<port>.text)`, расхождения превью с Operation. Doc-комментарий пакета `contract` и таблица пакетов в `AGENTS.md` дополнены
  - Тесты: границы безопасного диапазона и `math.MaxInt64`/`math.MinInt64` (`text` точен, `value` как раньше), недопустимый ввод и порт без привязки в превью, `NewInput` принимает те же тексты и даёт ту же форму, что `tag.TagType.NewTagValue` домена, `TestSchemaIsCommitted` для `widgetscript`

### Fixed

- **Integer-значения больше 2^53 теряли точность в виджете**: скрипт получал только JS `number`, точного значения не было. Теперь оно в `inputs.<port>.text`
- `jsonschema`: именованная map или slice в поле — `$ref` на запись в `$defs`, как у корня (раньше корневая map получала запись отдельной веткой, а в поле — встраивалась). Проверка опубликованной схемы (`make schemas`) работает и в репозитории, открытом через симлинк
- Bruno: `patch tag value` передаёт `version` (без него сервер отвечает 409) и описывает его в `docs`

## [0.0.42] - 2026-10-20

### Added

- **Холст Project показывает фон Scene** (задача 27)
  - `uiutil.SceneBackground(html)`: iframe фона Scene без скриптов с `pointer-events: none`; его используют и холст Project (`renderSceneCanvas`), и Operation (`livescene.View`, `renderBackground` удалён)
  - Слои холста Project снизу вверх: однотонный `var(--surface)`, фон Scene (если `background_html` не пуст), точечная сетка редактора (отдельный слой с `pointer-events: none`), виджеты. Drop, клик по пустому месту, drag/resize/rotate по-прежнему обрабатывает холст
  - Сохранённый (blur) `background_html` сразу виден на холсте; правки из другой вкладки или `growctl` приходят живой перезагрузкой
  - Тесты: разметка хелпера, фон под сеткой на холсте Project, холст без фона, сохранённый фон на холсте, фон в `livescene.View`

### Fixed

- **Фон Scene выполнял скрипты**: go-app выбрасывает атрибут с пустым значением, и `sandbox=""` не попадал в DOM. Фон теперь в `sandbox="allow-same-origin"` (`uiutil.SceneBackgroundSandbox`) — без `allow-scripts` ничего не выполняется

## [0.0.41] - 2026-10-19

### Fixed

- **Остановка сервера не ждёт SSE-соединения** (задача 26)
  - `restapi`: `eventHub.Close` закрывает каналы всех подключённых клиентов (отписка от EventBus — вне мьютекса hub'а), так что `GetEvents` сразу завершается, а клиент получает конец потока; повторный вызов ничего не делает. Раньше с открытой вкладкой UI `apiServer.Shutdown` ждал SSE-обработчики до таймаута 15 с и процесс выходил с кодом 1
  - `GetEvents` после `Close` отвечает 503 Problem Details «server is shutting down»; при превышении `MAX_SSE_CLIENTS` — по-прежнему «too many concurrent event stream connections» (`addClient` возвращает причину отказа). Контракт API не менялся
  - Хуки «Event Hub» и «API HTTP Server» остаются параллельными: на их порядок не полагаемся — `Shutdown` ждёт SSE-обработчики, пока hub дочищает outbox, и освобождается, когда hub закрывает потоки. `client_disconnected` при закрытии hub'ом публикуется как обычно и никому не доставляется
  - Тесты: открытый поток завершается после `Close` без отмены контекста; подключение после `Close` получает 503; `Close` дважды параллельно с отключением клиентов (`-race`); настоящий `http.Server` — `Close` и `Shutdown` параллельно, `Shutdown` возвращает `nil` до таймаута
  - `.mcp.json`: chrome-devtools запускается с `--headless` (работает и без `DISPLAY`, например по SSH)

## [0.0.40] - 2026-10-19

### Changed

- **Привязка тега к порту проверяет тип тега; сломанные привязки снимаются политиками** (задача 25, ADR 0010)
  - `scene`: `AddWidget`/`UpdateWidget` принимают `scene.TagTypes` — типы привязанных тегов значениями (Scene сама теги не читает) — и проверяют `PortTypeHint.Accepts`: несовпадение типа — `ErrPortTypeMismatch`, тег, которого нет, — `ErrBoundTagNotFound`. `SceneService` (получает `tag.TagRepository`) загружает типы тегов и отвечает на обе ошибки `port_bindings: invalid input` (400), как на `ErrPortNotDeclared`
  - `Scene.ReconcileWith(wt, tagTypes)` снимает у виджетов типа и привязки к тегам, которые hint больше не принимает, и к несуществующим тегам. Политика `reconcile_widgets_with_widget_type` получает `tag.TagRepository` и читает теги после Scene: смена `type_hint` у WidgetType снимает несовместимые привязки, а job на `widget_created`/`widget_updated` — привязку к тегу, удалённому параллельно с записью виджета
  - Новая политика `unbind_deleted_tag`: `tag_deleted` → job `tag:<id>`; находит Scene через `SceneRepository.FindByTagID` (поиск по JSON `port_bindings`) и в каждой отдельной транзакции снимает привязки к тегу (`Scene.UnbindTag`, событие `widget_updated` ByReconciliation, новых jobs не порождает)
  - `UpdateWidget` проверяет только привязки, которых у сохранённого виджета (того же типа) не было: привязка, сломавшаяся после создания, не мешает перемещать и править виджет — её снимает job события `widget_updated`
  - `unbind_deleted_tag` снимает привязки, только если тег на момент запуска действительно не существует
  - `TagRepository.FindTypesByIDs`: типы привязанных тегов читаются одним запросом (`WHERE id = ANY($1)`), а не по запросу на тег; общий для политик шаг «перечитать Scene, изменить, сохранить» вынесен в `changeScene`/`changeEachScene`
  - Миграции: GIN-индекс `idx_widgets_port_bindings` для поиска Scene по тегу; job `reconcile_widgets_with_widget_type` на каждый используемый WidgetType — снимает уже сохранённые сломанные привязки при первом запуске
  - UI (Project → Properties): привязка к удалённому тегу или к тегу неподходящего типа показывается выбранным пунктом «⚠ …», и выбор «— unbound —» её снимает
  - Bruno: `post widgets` и `put widget by id` привязывают порт `state` к boolean-тегу `BOOLEAN_TAG_ID`
  - Удаление тега и смена портов или `type_hint` у WidgetType по-прежнему не отклоняются из-за привязок. Контракты не менялись

## [0.0.39] - 2026-10-15

### Changed

- **События Widget несут SceneID, имена событий — в контракте** (задача 24, ADR 0009)
  - `contract/api/v0`: `Event.SceneID` (`scene_id`, `omitempty`) — у `widget_created`/`widget_updated`/`widget_deleted`; имена всех 15 типов событий — константы `apiv0.Event…` и `apiv0.EventTypes()`. SchemaVersion API 0.2 → 0.3, схема `schemas/api/0.3.json`
  - `restapi`: доменное событие переводится в `apiv0.Event` в `newAPIEvent` (`api_event.go`) по явной таблице `event.EventType → apiv0`-имя, а не через `EventType.String()`; `eventSourceID` и `eventTag` удалены. Тест требует образец на каждый тип из `event.AllEventTypes()`, использование каждой константы контракта и только имена из `apiv0.EventTypes()`; тест в `contract/api/v0` сверяет `EventTypes()` с блоком констант `Event…` (по исходнику)
  - `apiclient.ParseEvent`: разбор сообщения SSE в `apiv0.Event` за один проход; неподходящий `tag` отбрасывается, остальные поля сохраняются. Отбрасывается и `tag` не того тега, о котором событие (без ID или с чужим ID)
  - UI: режимы получают `apiv0.Event` (`eventlog.ServerEvent` и `parseEvent` удалены), строк типов событий в UI больше нет; `system_ready` — в журнале («Server ready»)
  - `eventlog.ReloadTargetsFor(apiv0.Event)`, поле `ReloadTargets.Scene`: событие Widget невыбранной Scene перечитывает только её (`GET /scenes/{id}`), выбранной — только её виджеты (они несут версию Scene; если виджетов нет, перечитывается Scene), а не весь список Scene; без `scene_id` — прежний фолбэк. Operation список Scene на событие Widget не перечитывает
  - `tag_updated` с телом тега ничего не перечитывает: Project (список и панель свойств тега), как и Operation, применяет тег сам. Общее состояние тегов — пакет `ui/tagstate` (побеждает большая Version, версии забываются на resync)
  - Project: перечитанная Scene заменяет запись в списке, только если её состояние новее (версия свойств — `sceneStates`; после отказа в сохранении она забывается): ответ, запрошенный до сохранения Scene, больше не откатывает правку. Версию выбранной Scene дают только её виджеты: более новая версия из ответа Scene или списка перечитывает виджеты (после drag), а не подставляется под показанные — иначе drag затирал чужое изменение виджета вместо 409. Версия Scene, которой ещё нет в списке, тоже учитывается. `uiutil.Versions`: `Advance`, `Forgotten`
  - `tagstate`: `Mark`, `Merge(mark, list)` — список тегов, запрошенный до resync, отбрасывается и не закрепляет версии старого сервера. Какие теги есть, решает только список: `tag_updated` тега, которого в списке нет (удалён или ещё не перечитан), его не показывает — состояние ждёт следующего списка
  - Project: список тегов, отправленный до создания тега, не снимает выделение с только что созданного
  - `eventlog.ReloadTargets.WidgetsOf(selected)` и `eventlog.ApplyToTags(tags, ev)` (тег из `tag_updated`, забывание версий на resync) — одно правило для Operation и Project вместо двух копий. `tagstate` хранит позицию тега в списке: `tag_updated` копирует только список, а не пересобирает map; `ByID()` удалён, `livescene.SceneTags` ищет теги через `Find`. Project: выбор Scene — метод `selectScene` (вкладка и тесты)
  - Project перечитывает Scene по событиям Widget по одному запросу за раз: Scene, отмеченные, пока он в полёте, ждут его, и если их несколько, вместо `GET /scenes/{id}` на каждую читается список `GET /scenes` (тем же Reloader, что и прочие загрузки списка). Изменение WidgetType, согласующее 50 Scene, — не больше двух запросов, а не 50 параллельных. Своя склейка Start/Done/rerun в `loadScene` заменена на `uiutil.Load`
  - Bruno: `get_events.yml` — пример `widget_*` со `scene_id`, `system_ready`

## [0.0.38] - 2025-10-14

### Fixed

- **CI на GitVerse: тесты с Postgres падали с `rootless Docker not found`.** Общие раннеры GitVerse не дают job доступа к Docker (монтирование `/var/run/docker.sock` раннер отбрасывает: «is not a valid volume»), и testcontainers не может поднять Postgres. Для GitVerse — отдельный `.gitverse/workflows/go.yaml` (в настройках репозитория: «Use configuration from .gitverse/workflows»): тесты гоняются без пакетов, чьим тестам нужен testcontainers (список вычисляется через `go list -test`, выводится в лог), затем `make build`. `.github/workflows/go.yaml` гоняет все тесты на GitHub: `make test` вместо `go test ./... -v`. Исходники не менялись

## [0.0.37] - 2026-10-14

### Changed

- **PWA UI ходит в REST API через один типизированный клиент** (задача 23). Поведение UI то же; Bruno-коллекции не менялись (запросы и ответы те же)
  - `contract/api/v0`: `apiv0.Problem` — тело любого ответа с ошибкой, пять полей RFC 9457 (`type`, `title`, `status`, `detail`, `instance`). restapi отвечает им; `restapi.ProblemDetails` с ручным Marshal/Unmarshal и Extensions удалён. JSON на проводе прежний. SchemaVersion API 0.1 → 0.2, схема `schemas/api/0.2.json`
  - `apiclient`: Scenes, Widgets, WidgetTypes; методы возвращают типы `apiv0` как есть (`apiclient.Tag` заменён на `apiv0.TagResponse`), создание — ответ сервера (`CreateTag` → `apiv0.CreateTagResponse`), удаление — последнее состояние ресурса. `StatusError` несёт `Method` и `apiv0.Problem` (терпимое чтение); ответ с ошибкой не в JSON (например, HTML прокси) даёт `Title` по статусу и начало тела в `Detail`. `errors.Is(ErrNotFound/ErrConflict)` как раньше. Заголовок `GrowSCADA-Schema-Version` — только у запросов с телом. Опция `apiclient.OnServerVersion` сообщает версию сервера из каждого ответа (и с ошибкой); клиент собирается под `GOOS=js GOARCH=wasm`
  - growctl: сервер с более старым API — одно предупреждение в stderr за запуск. Device simulator (`devicelink.WarnOlderServer`): запись в лог раз на смену версии сервера. Более новый сервер — без сообщений
  - UI: все вызовы (Project, Library, Operation, включая чтение списков) — через `*apiclient.Client` из Root и `uiutil.Call`; ошибки — `toast.FromError`. Удалены `uiutil.FetchJSON`, `uiutil.SendSchemaVersion`, `toast.FromHTTPError`, UI-копии DTO (`project/dto.go`, `library/dto.go`, `operation/dto.go`, `livescene/dto.go`, пакет `uidto`), `eventlog.wireMessage`, `operation.decodeTagUpdate`. SSE разбирается один раз в `apiv0.Event`, `ServerEvent.Tag` — `*apiv0.TagResponse`
  - UI: ID в состоянии — `uuid.UUID`, строки разбираются только на границах (LocalStorage, SSE, DOM/drag)
  - Project: версия Scene хранится в одном месте (`sceneVersions`) и обновляется по максимуму из любого ответа (список Scenes, Widgets, результат записи); `applyNewSceneVersion` и чтение `SceneVersion` Widget для записи убраны
  - Root: незакрываемая плашка «The server speaks API X, this interface Y. Reload the page.» с кнопкой Reload, пока версия сервера отличается от версии UI
  - Тесты: операции Project и Library гоняются в go-app `TestEngine` (`ui/uitest`) против настоящего API (`servertest.StartAPI`, очистка расширена на scenes, widgets, widget_types), включая 409, 400 с detail сервера и сетевую ошибку

## [0.0.36] - 2026-10-13

### Changed

- **Перевод ошибок в Problem Details — один модуль `restapi/errors.go`** (задача 22). URL не менялись, коды ответов — только у отрицательной версии (см. ниже); тела ответов те же Problem Details, поэтому SchemaVersion `contract/api/v0` не поднимался
  - Таблица `{sentinel, статус, ресурс, detail для 409}` и `sendError(w, r, err)`: сначала `application.ErrInvalidInput` → 400 с текстом ошибки, затем строки таблицы, иначе 500 (полная ошибка в лог, наружу общий текст). Доменные sentinel-ошибки импортирует только этот файл, лестниц `errors.Is` в обработчиках больше нет; `sendServiceError` убран
  - 404 — только для ресурса из пути: ID берётся из параметра, закреплённого за ресурсом (`{tag_id}`, `{widget_type_id}`, `{scene_id}`, `{widget_id}`). Шаблоны в `router.go` собираются из того же `resource` (`wildcard()`), так что имя параметра задано в одном месте. Если параметра нет — это ошибка сервиса, 500. Ссылка из тела запроса на несуществующее остаётся 400
  - Not-found из репозиториев и из `Scene` несёт ID (`id.NotFoundError`, `errors.Is` по-прежнему совпадает с sentinel). Если этот ID не тот, что в пути, ответ — 500, а не 404 про ресурс из пути
  - `pathUUID` разбирает ID из пути и сам отдаёт 400 вида «scene_id: 'abc' is not a valid UUID» — в snake_case, как поля API
  - Тест читает исходники `domain/` и требует, чтобы каждый sentinel был либо в таблице, либо в списке намеренно не отображённых с объяснением
  - `title` у 409 — «Conflict: Scene» (имена ресурсов из глоссария: Tag, WidgetType, Scene, Widget); `detail` — фиксированная фраза: «Scene was changed concurrently; reload it and retry» (так же для WidgetType и Tag), «WidgetType is used by Widgets», «Tag name is already taken»
  - Удаление (и снятие Widget со Scene), проигравшее гонку записи 3 раза подряд, — по-прежнему 409, но с `detail` «Scene kept changing while the request ran; retry it later»: версии в запросе нет, перезагружать нечего. `retryOnRace` помечает такой исход `application.ErrRaceLost`
  - Строка таблицы со статусом, который `sendError` не умеет отдавать, даёт 500, а не пустой 200
  - Отрицательная `version` в `PUT /scenes/{sceneId}` и `PUT /widget-types/{widgetTypeId}` — 400 «version: …» вместо 409 «changed concurrently»: такой версии не бывает, повтор с перезагрузкой не помог бы. Так же уже работал `scene_version` у Widget

### Fixed

- **`detail` у 404 не раскрывает внутренний текст.** Все вызовы `NewNotFound` передавали текст ошибки вместо типа ресурса, и клиент видел, например, «error on find scene by id in repository: scene not found with ID '…' not found». Теперь — «Scene with ID '…' not found»

## [0.0.35] - 2026-10-11

### Changed

- **Согласование Widget с изменённым WidgetType — политика по событию** (задача 21, ADR 0008). REST API и контракт `contract/api/v0` не менялись
  - `UpdateWidgetType` сохраняет только WidgetType; синхронный цикл по Scene (`removeOrphanedPortBindings`, `reconcileScene`) убран из сервиса, `NewWidgetTypeService` больше не принимает `SceneRepository`. Привязки Widget к удалённым портам снимает политика вскоре после commit
  - **Задания политик**: таблица `policy_jobs (policy, job_key, generation, attempts, next_attempt_at, last_error)`. Диспетчер outbox не выполняет политики: в транзакции, удаляющей доставленные строки, он ставит задание каждой политике, которой нужно событие (`Policy.JobKey`), и будит раннер. Задание с тем же ключом сливается со стоящим (`generation + 1`), поэтому событие, пришедшее во время выполнения, приводит к повторному прогону. Новое задание созревает сразу, а слившееся событие не сокращает паузу задания, которое падает: повтор всё равно прочитает текущее состояние
  - `outbox.PolicyRunner`: одна горутина, задания по очереди; успех — `DELETE … WHERE generation = g`, а если за время прогона слилось событие — задание созревает сразу со сброшенными `attempts` и `last_error`; сбой — `slog`, `last_error` и пауза 1 с → ×2 → 5 мин, повторы бесконечны. Паника политики и превышение таймаута попытки (`WithAttemptTimeout`, по умолчанию 5 мин) считаются сбоем, так что зависшая или падающая политика не роняет сервер и не задерживает остальные задания. `Run` будится диспетчером и таймером ближайшего задания, `RunDueOnce` — для тестов. При остановке текущее задание прерывается и выполняется после старта. Реестр политик собирается в `main.go`; раннер передаётся диспетчеру через `Dispatcher.SetPolicyRunner`, потому что репозитории политик будят уже созданный диспетчер
  - `application.Policy` (`Name`, `JobKey`, `Run`) и `NewWidgetReconciliationPolicy`: слушает `widget_type_updated` (ключ `widget_type:<id>` — все Scene с Widget этого типа), `widget_created` и `widget_updated` (ключ `widget_type:<id>/scene:<id>` — только Scene этого Widget, так что правка Widget не обходит весь проект); собственные `widget_updated` политики (`WidgetUpdatedEvent.ByReconciliation`, только снимают привязки; хранится во внутреннем кодеке outbox, в SSE не передаётся) заданий не ставят. WidgetType читается при выполнении, после Scene, и перечитывается вместе с ней при гонке записи, чтобы не снять привязку к порту, добавленному тем временем (удалён — задание выполнено) и согласует каждую Scene в своей транзакции (`retryOnRace`, удалённая Scene пропускается). Сбой одной Scene не останавливает остальные (`errors.Join`), повтор проходит уже согласованные вхолостую
  - `WidgetCreatedEvent`/`WidgetUpdatedEvent` несут обязательный WidgetType (`scene.WidgetTypedEvent.TypeID`; не `WidgetTypeID`, иначе событие Widget стало бы и `library.WidgetTypeEvent`); кодек outbox хранит `widget_type_id`. Строку, записанную без него, кодек читает в свой приватный `scene.WidgetEvent` без WidgetType: она доставляется в SSE, но задания не ставит. Домен о старом формате не знает
  - **UI Project**: `409` на правке Scene или Widget — тост «Scene changed … Repeat your edit», Scenes и Widgets перечитываются сразу, не дожидаясь SSE; правка не переприменяется

### Fixed

- Project: значения Simulate Inputs стирались при переходе на другую вкладку (например, в Library, чтобы поменять WidgetType) и обратно: Root при каждом открытии вкладки создаёт новый `Project`, а значения жили в компоненте. Теперь они хранятся в состоянии приложения go-app (`project:simInputs`, без `Persist`): переживают смену вкладок, но не перезагрузку страницы
- Library: значения Input Data превью так же стирались при смене вкладки, а ещё при выборе другого WidgetType. Теперь они хранятся по WidgetType в состоянии приложения (`library:inputValues`) и переживают и то и другое
- Ошибка посреди согласования (не конфликт) отвечала клиенту `500` после сохранённого WidgetType, а несогласованные Scene навсегда оставались с привязками к исчезнувшим портам. Теперь задание повторяется до успеха
- Widget, добавленный или изменённый параллельно с удалением порта (WidgetType прочитан до удаления), навсегда сохранял привязку к удалённому порту. Теперь его событие ставит задание политики, и привязка снимается
- Project: после переключения Scene Widget новой Scene мог показывать документ Widget прежней (например, значение из Simulate Inputs несвязанному Widget того же типа). go-app переиспользует компонент iframe, а `OnUpdate` искал элемент по новому id раньше, чем тот попадал в DOM. `uiutil.IframeSrcdoc` (поле компонента iframe, `Mount`/`Update`) теперь работает с самим элементом компонента (`JSValue()`) и не перезагружает iframe, если документ не изменился (превью Project и Library). Последний заданный документ хранится в компоненте, так что `OnUpdate` не копирует `srcdoc` из DOM через границу WASM/JS

## [0.0.34] - 2026-10-07

### Changed

- **Агрегат и его события в одной транзакции — outbox** (задача 20, ADR 0008). Поведение REST API не изменилось, кроме удаления при гонке записи (см. Fixed)
  - **Домен**: события переехали в пакеты агрегатов (`tag`, `scene`, `library`); `domain/event` — только база (`Event`, `Base`, `EventType`, `EventTimestamp`, `Recorder`) и не импортирует агрегаты. Агрегаты копят pending-события (`PendingEvents()`): `CreateTag`/`CreateWidgetType`/`CreateScene` записывают Created, `Reconstitute…` (для репозиториев) — ничего; `Tag.SetValue` — `tag_updated` с состоянием, которое сохранит `Save`; `Scene.Update`/`AddWidget`/`UpdateWidget`/`RemoveWidget` — свои события, `ReconcileWith` — `widget_updated` на каждый изменённый Widget; `Delete()` — Deleted (Scene — ещё `widget_deleted` на каждый Widget). События Widget несут SceneID (в контракт он пока не выходит — задача 24), но `SceneEvent` не удовлетворяют (метод-маркер): `type switch` не зависит от порядка `case`
  - `WidgetType` неизменяемый, как Scene: `Update(expected, …)` сам сверяет ожидаемую Version (`ErrWidgetTypeConflict`), а неверное определение отдаёт как `ErrInvalidWidgetType`; `CheckVersion`, `Delete()`. Сверка Version для `Tag.SetValue` пока остаётся в сервисе (задача 32)
  - `EventBus` и события Client/System переехали из домена в `interface/eventbus` (вместе с заглушкой MQTT): это события доставки
  - **Outbox**: таблица `outbox (seq, tx_id, type, payload jsonb, created_at)`, внутренний кодек событий (не контракт), общий шаблон `Save`/`Delete` Postgres-репозиториев (`aggregateStore`): CAS по загруженной Version, классификация NotFound/Conflict и запись событий — одна транзакция. Пишущие транзакции не ждут друг друга: `tx_id` (id транзакции, `pg_current_xact_id()`) задаёт порядок доставки
  - `DeleteByID` репозиториев заменён на `Delete(ctx, agg)` с CAS; у Tag — без CAS: после создания у тега меняется только значение, и удаление от него не зависит
  - **Диспетчер** `outbox.Dispatcher`: сигнал `Notify` после commit (`repositories.NotifyOnCommit`) + опрос раз в секунду; строки по `(tx_id, seq)` → `EventBus.Publish` (SSE) → один DELETE на пачку доставленных, at-least-once; строка ждёт, пока не закончатся все более старые транзакции (`pg_snapshot_xmin`), поэтому изменение, сделанное после commit другого, доставляется после него. DELETE доставленной строки не прерывается остановкой диспетчера. `DrainOnce` для тестов. Запускается в `main.go`, при остановке outbox дочищается до закрытия Event Hub, пока SSE-клиенты ещё подключены
  - **Сервисы** больше не держат `EventBus` и не публикуют событий: разбор входа → загрузка → метод агрегата → `Save`/`Delete`. Согласование Widget в `UpdateWidgetType` пока синхронное, но события каждой изменённой Scene теперь попадают в outbox (политика — задача 21)

### Fixed

- Событие могло уйти до commit или потеряться, а сервис мог его забыть (`UpdateWidgetType` не публиковал события согласованных Scene). Теперь откат ⇒ ничего не доставлено, commit ⇒ доставлено
- Удаление Scene, пока в неё добавляют Widget, не публиковало `widget_deleted` для добавленного Widget
- Удаление (WidgetType, Scene, Widget) при гонке записи перечитывает и повторяет (`retryOnRace`); проиграв гонку 3 раза подряд, отвечает `409`. Раньше удаление Widget при гонке сразу отвечало `409`. Удаление Tag гонки не знает и `409` не отвечает: тег удаляется, даже если Device пишет в него значения

## [0.0.33] - 2026-10-05

### Changed

- **Изменения Widget — через агрегат Scene** (задача 19). REST API и контракт `contract/api/v0` не менялись
  - `scene.Scene` неизменяемая: `AddWidget(expected, w, wt)`, `UpdateWidget(expected, w, wt)`, `RemoveWidget(widgetID)`, `Update(expected, name, size, background)` и `ReconcileWith(wt)` возвращают новую Scene, исходная не меняется; `FindWidget(widgetID)` ищет Widget сцены. Агрегат сам ловит конфликт правки (ожидаемая Version ≠ текущей → `ErrSceneConflict`) и проверяет, что `TypeID` Widget совпадает с переданным WidgetType и PortBinding ссылаются только на его InputPort (`ErrWidgetTypeMismatch`, `ErrPortNotDeclared`; Widget с занятым ID — `ErrWidgetAlreadyExists`)
  - `SceneRepository` сокращён до `NextID`, `NextWidgetID`, `FindByID`, `FindAll`, `FindByWidgetTypeID` (Scene целиком), `Save`, `DeleteByID`. `Save` пишет Scene с её Widget в одной транзакции: CAS по строке `scenes` (Version повышается один раз на сохранение), затем INSERT новых / UPDATE изменённых / DELETE исчезнувших Widget. Чтение — одна read-only транзакция `REPEATABLE READ`, Scene и её Widget всегда из одного момента; `DeleteByID` блокирует строку сцены перед чтением Widget
  - `SceneService`: каждое изменение — загрузка Scene (нет → `404`) и WidgetType (нет → `400`) → метод агрегата → `Save`; Version в сервисе не сверяется
  - `removeOrphanedPortBindings` сервиса WidgetType: `FindByWidgetTypeID` → `ReconcileWith` → `Save` изменившихся сцен (без атомарности и событий — задача 21)
  - **Пакеты домена**: Widget и его value objects (`WidgetName`, `Position`, `Origin`, `Rotation`, `TransformationMatrix`, `PortBinding`, `ErrWidgetNotFound`) переехали в `domain/scene`, размер Widget стал `scene.WidgetSize`. `domain/widget` переименован в `domain/library` (WidgetType, InputPort, PortTypeHint, шаблон, скрипт); размер по умолчанию WidgetType остаётся `library.Size`

### Fixed

- Удаление Widget поднимало Version сцены без CAS (`UPDATE scenes SET version = version + 1`) и не замечало правку сцены между чтением и записью. Теперь удаление идёт через `Save`: при гонке записи — `409`, параллельная правка не затирается
- `FindByID` читал Scene и её Widget двумя запросами вне транзакции и мог увидеть их в разные моменты

### Removed

- `SceneRepository.AddWidget`, `UpdateWidget`, `DeleteWidget`, `FindWidgetsBySceneID`, `FindWidgetByID`, `FindWidgetsByTypeID`, тип `scene.WidgetInScene`; в Postgres-репозитории `bumpSceneVersion`; в `SceneService` — `validateWidgetType` и `widgetSaveError`

## [0.0.32] - 2026-10-05

### Added

- задачи 19–24 по итогам архитектурного ревью 2026-10-05 (только backlog, код не менялся): изменения Widget через агрегат Scene (19), единица работы — состояние, Journal и события в одной транзакции (20), атомарное согласование Widget при изменении WidgetType (21), перевод ошибок в Problem Details одним модулем restapi (22), типизированный клиент сервера для PWA UI (23), SceneID в событиях Widget и типы событий в контракте (24)

### Changed

- **Перенумерация backlog**: задачи 19–51 сдвинуты на 6 и стали 25–57, следующий свободный номер — 58. Ссылки «задача N» и `Blocked by:` в `todo/backlog/`, `todo/done/` и ADR 0003, 0004, 0006 пересчитаны; исторические упоминания («бывшая задача N», тикеты расформированного пакета 36) и прошлые записи этого файла не менялись

## [0.0.31] - 2026-10-04

### Changed

- **Невалидный ввод в REST API → `400 Bad Request`** (задача 18): ошибки разбора и валидации значений из запроса (имя, `TagType`, `Quality`, значение тега, в т. ч. несовместимое с типом, `ScriptLanguage`, `PortTypeHint`, имя порта, размеры, origin, версия сцены, matcher имени) раньше уходили в `500`, теперь — `400` с причиной в `detail`
  - сцены: `422 Validation Error` заменён на `400`
  - ссылка из тела на несуществующий WidgetType и порт, не объявленный на WidgetType, — `400`; тип виджета теперь проверяется всегда, а не только при port bindings. Если в том же запросе нет сцены или виджета из URL, ответ — `404`: несуществующий тип проверяется внешним ключом уже после них
  - `404` остаётся только для ресурса из URL, `409` — для конфликта версии или имени
  - devicelink и device simulator больше не ретраят такой ввод: `IsTransient` считает временными только `5xx`
- **WidgetType виджета всегда существует**: `DELETE /api/v0/widget-types/{id}` для типа, который используют Widgets, отвечает `409 Conflict` и ничего не удаляет (раньше тип удалялся, а у его Widgets очищались port bindings). Гарантирует внешний ключ `fk_widgets_type_id` (`ON DELETE RESTRICT`); репозитории отдают `widget.ErrWidgetTypeInUse` и `widget.ErrWidgetTypeNotFound` (распознаются оба SQLSTATE блокировки удаления: `23503` до PostgreSQL 18 и `23001` restrict_violation с 18 — тесты идут на 16, dev-БД на 18)
  - миграция `20261004000000_widgets_type_id_foreign_key` **удаляет Widgets с несуществующим типом**, оставшиеся от прежних удалений, и поднимает `version` их сцен, чтобы клиент со старой версией не записал поверх изменившейся сцены
  - `CONTEXT.md`: правило в термине WidgetType; Bruno: `delete_widget_type_by_id` описывает `409`, новый пример `post_tags_invalid_quality` (`400`)
- **UI больше не подменяет ввод**: свойства сцены (ширина/высота ≤ 0 → 1920/1080) и геометрия виджета (размер ≤ 0 → 10, origin обрезался до 0–1, нечисловое поле → 0) молча сохраняли не то, что ввёл пользователь, а в поле оставалось введённое. Теперь число уходит как есть: сервер отвечает `400`, toast показывает причину, поле возвращается к сохранённому значению; нечисловое или пустое поле, а также `NaN`/`Inf` — toast «Client Error» с именем поля, запрос не отправляется (`project/number_fields.go`)
- application-слой помечает ввод вызывающего общим `application.ErrInvalidInput`; ошибки восстановления сохранённых данных маркер не получают и остаются `500`. Хендлеры маппят ошибки сервисов общим `sendServiceError`
- application-слой: у Create и Update сцены, WidgetType и виджета один вход (`appdto.SceneInput`, `WidgetTypeInput`, `WidgetInput`) — только поля ресурса; ID и ожидаемая версия (в т. ч. `sceneVersion` виджета) передаются аргументами. Поля разбирает одна функция на ресурс (`parseScene`, `parseWidgetType`, `parseWidget`), ошибка называет поле из JSON-контракта: `cannot update widget: size: widget width must be positive, got 0`

### Fixed

- Project: клик по виджету на холсте без перетаскивания (и перетаскивание, вернувшееся в исходную точку) отправлял PUT с теми же значениями и поднимал версию сцены — у других клиентов это давало лишние перезагрузки и ложные конфликты версий. Теперь виджет сохраняется, только если его геометрия изменилась (`dragChangedWidget`)

### Removed

- `scene.ErrSceneValidation`, `widget.ErrWidgetInvalidInput` (заменены `application.ErrInvalidInput`), `restapi.NewValidationError` и `restapi.TypeValidation`
- `appdto.CreateSceneInput`/`UpdateSceneInput`, `CreateWidgetTypeInput`/`UpdateWidgetTypeInput`, `CreateWidgetInput`/`UpdateWidgetInput` (заменены едиными входами)

## [0.0.30] - 2026-10-04

### Added

- **Модель многоарендности и библиотек** (grilling-сессия 2026-10-04, только документация, код не менялся):
  - ADR 0006 «Every operational entity is keyed by Project, every Project by Organization»: сервер хостит много проектов в обеих ролях узла, `project_id` вводится во все таблицы до появления Journal и Revision, пока одна стартовая Organization из seed
  - ADR 0007 «Libraries are shared by pinned release and copied into the Revision on Deploy»: Library принадлежит Organization и может быть публичной, у неё свой Draft (EditLock, undo/redo) и Release → неизменяемый Library Release; проект закрепляет по одному релизу от нескольких Library, при Deploy используемое содержимое копируется в Revision
  - `CONTEXT.md`: термины Organization, Project, Library, Library Release, Release, Device Gateway, Device Token
- задачи 25 (Organization и Project), 27 (Library и Library Release), 28 (Device, Device Gateway, Device Token) переписаны по решениям сессии; задача 45 уточнена: «Runtime node со вшитой Revision»

### Changed

- **Перенумерация backlog**: номер задачи — её место в очереди, следующий свободный — 52:
  - 18–24 — мелкие независимые задачи (бывшие 34, 35, 38, 39, 40, 37, 33)
  - A, 25–26 — Organization и Project; разделение хранилищ engineering/runtime, Origin
  - B, 27–30 — Library Release; Device; ProjectFile, Deploy, Revision, Discard; Rollback
  - C, 31–35 — EditLock; DraftChange и undo/redo; ProjectFile в UI; групповое выделение; направляющие
  - D, 36–41 — Journal; Player и Operation; Checkpoint; History; PlaybackFile; наведение мыши
  - E–F, 42–44 — runtime-сущности и Adopt; User; права доступа
  - 45–51 — Runtime node со вшитой Revision; разнесение узлов по сети; Command; Alert; звуки; Computation; Theme
- пакет `36_project_undo_redo_history` расформирован: его тикеты стали обычными задачами со строками `Status:` и `Blocked by:`, решения Q1–Q30 из `map.md` разнесены по `## Comments` соответствующих задач. Отменены Q7 (Library — часть Draft проекта) и Q21 (один неявный проект)
- ProjectFile в UI (33) стоит после DraftChange (32): открытие файла — одна DraftChange
- Deploy (29) и Rollback (30) больше не зависят от Journal: их записи в Journal добавляет задача 36
- задача 36 (Journal) поглотила бывшую 24 «Запись и воспроизведение событий»; задача 42 (Adopt) зависит от DraftChange (32); EditLock (31) и DraftChange (32) работают и для Draft Library; User (43) — член Organization, а не владелец проектов
- ссылки «задача N» в `todo/backlog/`, `todo/done/` и в этом файле пересчитаны на новые номера
- `CONTEXT.md`: Device — конфигурация оборудования в проекте, а не процесс; Draft, EditLock и Discard — у проекта и у Library; имя Tag уникально внутри Project; у Tag не больше одного поставщика значения
- ADR 0003: имя Tag уникально внутри Project; ADR 0004: поправка — Device как конфигурация, Device Gateway исполняет её по Device Token, токен переживает Deploy и Rollback и отзывается только явно

### Removed

- `todo/backlog/36_project_undo_redo_history/` (`map.md` и 13 тикетов) и `todo/backlog/24_event_record_playback.md` — содержимое перенесено в задачи 26, 29–33, 36–40, 42, 46

## [0.0.29] - 2026-10-04

### Added

- **Версионирование контрактов** (задача 17, ADR 0005, термин SchemaVersion в `CONTEXT.md`): у каждого из четырёх контрактов — серверный API, формат проекта, формат операционной записи, манифесты growctl — своя SchemaVersion `MAJOR.MINOR`, у всех стартовая `0.1`. Пока MAJOR = 0, совместимость не обещается
- публичный пакет `contract/`: `SchemaVersion`, по подпакету на контракт и MAJOR (`contract/api/v0`, `contract/project/v0`, `contract/record/v0`, `contract/manifest/v0`). Форматы проекта и записи пока заготовки — только версия и корневое поле `schema_version`
- JSON-схемы генерируются из Go-типов в `schemas/<контракт>/<MAJOR.MINOR>.json` (`make schemas`) и коммитятся. `make test` падает, если типы изменились, а версию не подняли, и если новая MINOR (при MAJOR ≥ 1) не только добавляет необязательные поля
- заголовок `GrowSCADA-Schema-Version`: сервер ставит его в каждый ответ, включая поток событий и CORS preflight (разрешён и виден браузеру); `apiclient` отправляет его в каждом запросе, UI — только в запросах с телом: сервер читает заголовок лишь при разборе тела, а на GET нестандартный заголовок стоил бы CORS-preflight. Сервер отдаёт `Access-Control-Max-Age: 7200`, чтобы браузер не повторял preflight перед каждым POST/PUT/PATCH/DELETE
- опубликованная схема контракта не меняется: старые версии замораживаются хешем в `schemas/<контракт>/frozen.sha256` (тесты падают, если файл изменился), а `make schemas` отказывается переписывать версию, которая уже есть в ветке `path`, и просит поднять SchemaVersion. Без git или без ветки `path` — предупреждение вместо проверки. Пока MAJOR = 0, `make schemas` при новой MINOR печатает NOTE о том, что после 1.0 потребовало бы MAJOR, а тест совместимости MINOR прогоняется на парах `0.x` и только логирует
- Bruno: запрос «post tags newer client»
- ADR 0005 «Contracts carry a MAJOR.MINOR SchemaVersion; readers skip, receivers reject»; в `AGENTS.md` — правило «изменил тип в `contract/` — подними SchemaVersion и запусти `make schemas`», цель `make schemas` и раскладка пакетов `contract/` и `schemas/`

### Changed

- **Ломающее изменение API**: префикс `/api/v1` → `/api/v0` (MAJOR в пути). Запрос на `/api/v1/...` получает `410 Gone` с подсказкой перейти на `/api/v0` (до 1.0, ADR 0005); Bruno: «get pre-versioning path»
- **Ломающее изменение control API симулятора**: `/api/v1/device/...` → `/control/device/...`. У control API нет контракта (ADR 0005), и цифра в пути только выдавала его за версию 1; Bruno-коллекция `device_simulator` обновлена
- **Ломающее изменение манифестов**: `apiVersion: growscada/v0.1` вместо `growscada/v1`; `growscada/v0` читается как `v0.0`. На `growscada/v1` growctl отвечает подсказкой заменить её на `growscada/v0.1` (до 1.0). Манифест более новой MINOR growctl отклоняет с просьбой обновить growctl, а не с ошибкой «unknown field»
- сервер разбирает тела запросов строго: `400` на незнакомое поле, на повторный ключ (в том числе отличающийся только регистром — `encoding/json` молча оставлял последний) и на данные после JSON-значения; раньше всё это молча отбрасывалось. Если клиент в заголовке указал версию новее серверной и ошибка — незнакомое поле, в ответе написано «обновите сервер»
- UI отправляет тела запросов типами `contract/api/v0` вместо собственных копий, так что расхождение с сервером ловит компилятор; `uidto.InputPortDTO` — алиас `apiv0.InputPort`. Битый ID в состоянии UI показывается как «Client Error», запрос не отправляется
- задача 17 переписана по итогам grilling-сессии 2026-10-03 и перенесена в `todo/done/`; пути в ADR 0004 и Bruno-коллекции `growscada_server` — `/api/v0`
- `SchemaVersion` контрактов (`apiv0`, `manifestv0`, `projectv0`, `recordv0`) — функция `SchemaVersion()` вместо экспортированной переменной: пакет `contract` публичный, и любой импортёр мог её изменить
- JSON-типы REST API перенесены из `internal/server/interface/restapi/restdto` в `contract/api/v0` (`InputPortDTO` → `InputPort`, `PortBindingDTO` → `PortBinding`). Сообщение потока событий — тип `apiv0.Event`, преобразования в `appdto` — в `restapi/dto_mapping.go`

### Fixed

- генератор JSON-схем паниковал на поле-указателе на тип с методом `JSONSchema` по значению, а `*uuid.UUID` и `*time.Time` получали схему без `format`
- генератор JSON-схем встраивал поля не так, как кодировщики: для yaml встраивал анонимное поле без `,inline` (yaml.v3 вкладывает его под ключом), для json не встраивал встроенную структуру с тегом без имени (`json:",omitempty"`), пропускал встроенную неэкспортированную структуру с именем в теге и пропускал поле с тегом `json:"-,"` (это поле с именем `-`). Закоммиченные схемы не изменились
- генератор JSON-схем объявлял обязательным поле с `json:",omitzero"`, которое encoding/json опускает, и описывал поле с `json:",string"` его Go-типом, хотя в JSON оно строка: теперь `string` с `pattern` для целых и `enum` для bool

## [0.0.28] - 2026-09-29

### Added

- **Режим Operation** (задача 16) — Operator наблюдает Scene с живыми значениями тегов, без управления:
  - вкладки со всеми Scene (только переключение), выбранная запоминается в LocalStorage (`operation:sceneID`); холст 1:1: помещающаяся Scene стоит по центру без полос прокрутки, не помещающаяся прокручивается от левого верхнего угла; под виджетами — `background_html` Scene в iframe без скриптов
  - виджет получает значения привязанных тегов; iframe загружается один раз, новые значения приходят через `postMessage` (без мерцания и сброса анимаций), после handshake `ready`; iframe пересоздаётся только при смене кода WidgetType
  - поверх виджета — маркер худшей Quality его портов: `Uncertain` — жёлтая рамка, `Bad` — красная рамка с затемнением; порт без привязки или с удалённым тегом считается `Bad`
  - изменения конфигурации (Scene, Widget, WidgetType, создание/удаление тегов) подхватываются на лету; значения тегов применяются прямо из `tag_updated`, состояние тегов сливается по Version
  - при разрыве SSE — плашка «No connection to the server» и маркеры `Uncertain`; после переподключения конфигурация и теги загружаются заново
  - компонент `livescene.View` рисует Scene по готовому состоянию и ничего не загружает (для будущего Player)
- `docs/widget_type_contract.md` — контракт скрипта WidgetType

### Changed

- **Ломающее изменение контракта WidgetType**: `render(inputs)` получает каждый порт объектом — `inputs.<port>.value` вместо `inputs.<port>`; поля входа в будущем только добавляются. `render` может вызываться многократно и должен быть идемпотентным. Seed-виджеты переписаны («String Ticker» больше не перезапускает бегущую строку при том же тексте)
- SSE-сообщение `tag_updated` несёт полное состояние тега в поле `tag` (форма `GET /api/v1/tags/{id}`); доменное `TagUpdatedEvent` содержит Tag
- `eventlog.Bar` публикует состояние связи `eventlog.StateDisconnected` и передаёт состояние тега в `ServerEvent.Tag`

## [0.0.27] - 2026-09-28

### Fixed

- Вкладки сцен (и списки типов виджетов, виджетов сцены) периодически меняли порядок: как и у тегов, запросы шли без `ORDER BY`, и после `UPDATE` строка уезжала в конец. Теперь `SceneRepository.FindAll`, `FindWidgetsBySceneID`, `FindWidgetsByTypeID` и `WidgetTypeRepository.FindAll` возвращают в порядке создания (`ORDER BY pk_id`): при переименовании сцена остаётся на своём месте
- Project: после двойного клика по вкладке сцены (переименование) другие сцены показывались пустыми до перезагрузки страницы. Результат асинхронного запроса диспатчился через контекст элемента, породившего событие; go-app молча отбрасывает `Dispatch`, если этот элемент уже размонтирован (вкладка превратилась в поле ввода), и `Reloader` виджетов навсегда оставался в состоянии загрузки. Теперь все асинхронные операции Project и Library диспатчат через контекст компонента (`compoCtx`, захватывается в `OnMount`); заодно больше не теряется новая версия сцены после переименования
- Список тегов (Project → Tags, `GET /api/v1/tags`, в том числе с `?name_pattern=`/`?name_regex=`) перемешивался при обновлении значений тегов: Postgres отдавал строки без `ORDER BY` в порядке хранения, а `UPDATE` перемещает строку. Теперь `TagRepository.FindAll` сортирует по имени

### Added

- **Симулятор устройства нижнего уровня** (задача 15) — `cmd/device_simulator/`, логика в `internal/devicesim/`; собирается в `make build` (или отдельно `make build_simulator`) → `bin/growscada_device_simulator/`, `make run_simulator` — с примером `tests/device_simulator/example.yaml`, `make run_simulator_dashboard` — с `tests/device_simulator/seed_dashboard.yaml`, который управляет сидовыми тегами сцены «Main Dashboard» (результат виден в Operation сразу после `make db_up`):
  - один процесс — одно виртуальное устройство; YAML-конфиг (`server`, `control`, `autostart`, `defaultInterval`, `tags`), неизвестные поля — ошибка; `--config`, `--server` / `DEVSIM_SERVER`, `--control`; завершение через `gracedown`, как у сервера: по SIGINT/SIGTERM сначала останавливается control-API, затем устройство (текущие записи дописываются); коды выхода sysexits — 0 при штатной остановке, 66 — нет файла конфига, 78 — ошибка конфига, отсутствующий тег или несовместимый паттерн, 69 — не удалось занять порт control-API
  - теги только по имени (ADR 0003), сам их не создаёт и не удаляет; control-API поднимается сразу, а сервер симулятор ждёт без ограничения по времени, как настоящее устройство (пока ждёт: `GET /api/v1/device` → `"state":"connecting"`, запросы к тегам → 503); отсутствующий тег или несовместимый с типом паттерн — ошибка без единой записи
  - паттерны `constant`, `ramp`, `sine`, `random_walk`, `step`; integer — все, boolean и string — `constant` и `step`; генерируемые числа округляются для integer, а литералы `constant`/`step` должны быть целыми; интервал на тег, запись каждый тик
  - control-API (`:9191`): `POST /api/v1/device/start|stop` (пауза, время паттернов не идёт; `stop` отвечает, когда записи в полёте завершены), `GET /api/v1/device` (у тегов — `overridden`, `lost`), `POST /api/v1/device/tags/{name}/pattern` (смена на лету), `POST|DELETE /api/v1/device/tags/{name}/quality` (липкое переопределение качества и сброс); ошибки — Problem Details
  - тесты: unit (паттерны, конфиг, устройство и control-API с фейковым линком), интеграционные и сквозные против настоящего REST API и Postgres в testcontainers, включая конфликт версий (409)
  - Bruno: коллекция `tests/api/bruno_collections/device_simulator/` для ручных запросов к control-API
- `internal/devicelink` — контракт поставки значений тегов от Device к серверу (ADR 0004): резолв по имени, учёт версии, при 409 — перечитать и повторить один раз, 404 — тег выбывает; свои перечисления `TagType` и `Quality` (без нулевого-«unknown», ADR 0001), без зависимости от `server/domain`
- `internal/server/servertest` — общий тестовый стенд: настоящий REST API поверх Postgres в testcontainers с ленивым стартом контейнера; на нём e2e-тесты devicelink, devicesim и growctl (unit-тесты growctl больше не требуют Docker)
- `make build` разбит на `build_server` (WASM + сервер), `build_growctl` и `build_simulator` и собирает все три; `make run` собирает только сервер
- GitHub CI собирает все бинарники через `make build`

### Changed

- Bruno: коллекция сервера переименована `tests/api/bruno_collections/growscada/` → `growscada_server/` (в Bruno — «growscada server»), коллекция симулятора называется «growscada device simulator»; в Bruno это две отдельные коллекции, каждая открывается своей папкой
- `internal/` разложен по приложениям: слои сервера (`domain`, `application`, `infrastructure`, `interface`) перенесены в `internal/server/`; поведение не менялось
- REST-клиент вынесен из `growctl` в общий пакет `internal/apiclient` (добавлены `GetTag`, `SetTagValue`, `ErrConflict`); `growctl` использует его
- AGENTS.md: правило зависимостей инструментов вне `server/` по ролям — Engineer-инструменты (`growctl`) могут импортировать value objects из `server/domain`, сторона Device (`devicelink`, `devicesim`) — только контракт REST

## [0.0.26] - 2026-09-27

### Added

- **`growctl`** (задача 14) — CLI для декларативного управления тегами через YAML-манифесты в стиле kubectl (`apiVersion: growscada/v1`, `kind: Tag`, `metadata.name`, `spec.type`/`initialValue`/`initialQuality`):
  - `cmd/growctl/`, логика в `internal/growctl/` (разбор манифестов, чистый плановщик, REST-клиент, команды на `spf13/cobra`); собирается в `make build` → `bin/growctl/`
  - работает только через REST API: `--server` / `GROWCTL_SERVER`, по умолчанию `http://localhost:9090`
  - `apply -f <файл|каталог|->` создаёт недостающие теги; `initialValue`/`initialQuality` применяются только при создании; `--prune` удаляет теги сервера, которых нет в манифесте
  - `diff -f` показывает план, только изменения: `+ tag/<имя> (<type>)` — создание, `- tag/<имя>` — prune (код выхода 0 — нет изменений, 1 — есть, 2 — ошибка); `delete -f` удаляет теги манифеста по имени (отсутствующий тег — строка `tag/<имя> not found`, код выхода 0)
  - `-f` можно повторять; одно имя тега, объявленное в манифестах дважды, — ошибка; `get tag` — синоним `get tags`
  - план строится до первого изменения: невалидный манифест или смена `type` у существующего тега — ошибка без изменений; сбой REST-запроса останавливает выполнение, повторный `apply` доводит дело до конца
  - тесты: unit-тесты манифеста и плановщика, сквозные тесты через `httptest` с настоящим REST-роутером и Postgres в testcontainers
  - `tests/manifests/example_tags.yaml` — пример манифеста
  - шаблоны имён `--pattern` (`*`, `?`) / `--regex` (Go RE2): `apply`/`diff --prune --pattern` удаляет только лишние теги внутри шаблона; `delete --pattern|--regex` удаляет подходящие теги с подтверждением (`--yes` — без); `get tags [--pattern|--regex] [-o table|yaml]` — список тегов, `-o yaml` — манифест для `apply -f`. Клиент фильтрует ответ сервера и сам, поэтому сервер без поддержки фильтров не расширит выборку. Ошибки использования — код выхода 2, в том числе `apply`/`diff` без `-f` и неизвестная подкоманда (`growctl foo`, `growctl get foo`)
  - ошибка HTTP-ответа называет метод и полный URL запроса (`GET http://…/api/v1/tags: 404 Not Found`), поэтому неверный `--server` сразу виден
  - клиент сверяет имя в ответе `GET /tags?name=`, поэтому корректно работает и с сервером, который фильтр ещё не поддерживает
- `GET /api/v1/tags?name=<имя>` — фильтр по имени тега; ответ той же формы `{"tags":[...]}` с 0 или 1 элементом; Bruno: `get_tags_by_name.yml`
- `TagRepository.FindByName`, `TagService.FindTagByName`
- Поиск тегов по шаблону и регулярному выражению — `GET /api/v1/tags?name_pattern=<шаблон>` (`*` — любая последовательность, `?` — один символ, совпадение со всем именем) и `GET /api/v1/tags?name_regex=<regex>` (Go RE2, совпадение в любом месте имени; `^`/`$` для привязки). Ответ — `{"tags":[...]}` с любым числом тегов; невалидное выражение и более одного из `name`/`name_pattern`/`name_regex` — 400. `?name=` остаётся строгим поиском. Домен: value object `tag.TagNameMatcher` (`NewTagNamePattern`, `NewTagNameRegex`), ошибка `tag.ErrInvalidTagNameMatcher`; сервис: `FindTagsByNamePattern`, `FindTagsByNameRegex`. Bruno: `get_tags_by_name_pattern.yml`, `get_tags_by_name_regex.yml`. Имена параметров — константы `restdto.TagsQueryName`/`TagsQueryNamePattern`/`TagsQueryNameRegex`, общие для сервера и `growctl`
- UI обновляется по SSE-событиям сервера — изменения из `growctl`, другой вкладки или по REST видны без перезагрузки страницы:
  - `eventlog.Bar` (единственное SSE-соединение приложения) рассылает каждое событие go-app действием `eventlog.ActionServerEvent`
  - Library перечитывает список типов виджетов по `widget_type_*`; Project — палитру типов (`widget_type_*`), сцены (`scene_*`), виджеты выбранной сцены и сцены (`widget_*`: изменение виджета повышает версию сцены без события сцены) и теги (`tag_*`) — `internal/interface/ui/project/live_reload.go`
  - редакторы сливают свежие данные по полям (`uiutil.MergeField`): поле, которое пользователь не трогал, следует за сервером, несохранённая правка сохраняется; удалённый извне объект снимает выделение
  - перезагрузки склеиваются (`uiutil.Reloader`): пока идёт запрос, новые события ставят в очередь не более одной повторной загрузки; перезагрузка виджетов откладывается до конца перетаскивания/поворота/ресайза; версия сцены в UI никогда не откатывается назад
  - `uiutil.FetchJSON` — общий GET+JSON с ошибкой в виде toast

### Fixed

- Library: textarea редактора не очищалась, когда текст становился пустым (удаление типа, тип с пустым скриптом) — go-app не сбрасывает свойство `value`; добавлен `textareaValueSync`
- Library: список не мигает «Loading...» при перезагрузках, плейсхолдер только при первой загрузке
- Bruno `put_scene_by_id.yml`: добавлено обязательное поле `version` (без него запрос всегда получал 409)

### Changed

- **BREAKING: имя тега уникально** (`docs/adr/0003-tag-name-is-unique-natural-key.md`, `CONTEXT.md`):
  - миграция `20260926000000_unique_tag_names.sql` — ограничение `uq_tags_name` вместо индекса `idx_tags_name` (упадёт, если в БД уже есть дубликаты имён)
  - новая доменная ошибка `tag.ErrTagNameTaken`; репозиторий отображает в неё нарушение `uq_tags_name`
  - `POST /api/v1/tags` с занятым именем отвечает `409 Conflict` (Problem Details); Bruno: `post_tags_duplicate_name.yml` (имя `is_active` из сида)
- `make run`: chromium запускается с `--disable-background-networking`
- `AGENTS.md`: описаны сборка `growctl` в `make build` и `cmd/growctl/` в структуре слоёв; в таблицу целей добавлена существующая цель `make bench`

## [0.0.25] - 2026-09-24

### Added

- `internal/domain/event/event_type_test.go` — `TestAllEventTypes_ListsEveryDeclaredEventType`: разбирает `event_type.go` и сверяет число объявленных значений `EventType` с `len(AllEventTypes())`; ловит значение, не добавленное в таблицу имён (такие события молча не доходили бы до SSE-клиентов, т. к. хаб подписывается на `AllEventTypes()`)
- `tests/api/bruno_collections/growscada/post_widget_types.yml` — пример порта с `"type_hint": ""` («любой тип»)
- `docs/agents/` (`issue-tracker.md`, `triage-labels.md`, `domain.md`) и раздел «Agent skills» в `AGENTS.md` — конфигурация agent skills: задачи ведутся в `todo/`, стандартные triage-метки, single-context `CONTEXT.md` + `docs/adr/`

### Removed

- **BREAKING: качество тега `simulated`** — Quality описывает надёжность значения, а не его происхождение; имитатор (задача 15) поставляет обычные `good`/`bad`/`uncertain`, а пометка «это имитатор» станет метаинформацией `Device` (задача 28):
  - `internal/domain/tag/tag_quality.go` — удалён `TagQualitySimulated`
  - `internal/infrastructure/postgres/migrations/20260923000000_remove_simulated_tag_quality.sql` — существующие теги с `simulated` переводятся в `good`, `chk_tags_quality` пересоздаётся без `simulated` (`Down` возвращает ограничение, данные не трогает)
  - `POST /api/v1/tags` и `PATCH /api/v1/tags/{id}/value` с `"quality": "simulated"` теперь отклоняются (пока 500, как и любая ошибка валидации домена — маппинг в 400 вынесен в задачу 18)
  - тестовые данные: тег `is_cached` получил качество `uncertain`
- **Заглушки `Unknown` в доменных перечислениях** (`docs/adr/0001-no-unknown-enum-sentinels.md`) — удалены `TagQualityUnknown`, `TagTypeUnknown`, `ScriptLanguageUnknown`, `EventTypeUnknown`; нулевое значение перечисления невалидно (`String()` → `"invalid"`, `IsValid()` → `false`), разбор `"unknown"`, `""` и любой нестандартной строки возвращает ошибку

### Changed

- `internal/domain/tag/tag.go` — `NewTag` отклоняет невалидные тип и качество, `SetValue` — невалидное качество (раньше нулевое значение останавливал только CHECK в БД)
- `internal/domain/widget/widget_type.go` — `NewWidgetType` отклоняет невалидный `ScriptLanguage`
- `internal/domain/event/event_type.go` — `EventType` из `int` + `iota` стал struct value object с приватным полем, как остальные перечисления; снаружи пакета нельзя получить произвольное значение приведением
- **BREAKING: `InputPort` type hint** — новый value object `widget.PortTypeHint` (`internal/domain/widget/port_type_hint.go`): либо «любой тип» (`AnyTypeHint()`), либо конкретный `TagType` (`TypeHintFor`); методы `IsAny`, `TagType`, `Accepts`; «любой тип» в REST и в JSON-колонке `input_ports` — это `""`:
  - в ответах API «любой тип» теперь `"type_hint": ""` вместо `"unknown"`
  - запрос с `"type_hint": "unknown"` отклоняется (пока 500, см. задачу 18)
  - UI (`uidto.TypeHintLabel`, `project/render_properties.go`) больше не обрабатывает `"unknown"`
- Глоссарий (`CONTEXT.md`, `AGENTS.md`): Quality — `Bad`/`Uncertain`/`Good`, TagType без `Unknown`, новый термин Device
- `internal/domain/event/event_type.go` — имена типов событий заданы одной таблицей `eventTypeNames`, из неё выводятся `AllEventTypes()`, `NewEventType()` и `String()` (вместо двух параллельных `switch` на 15 веток); добавлен `IsValid()`, как у остальных перечислений. Строковые имена и порядок `AllEventTypes()` не изменились. Round-trip тест теперь покрывает все значения `AllEventTypes()`, добавлена проверка уникальности значений и имён
- `docs/adr/0001-no-unknown-enum-sentinels.md` — оговорено исключение: нулевое значение `PortTypeHint{}` валидно и означает «любой тип» (это отдельный value object, а не перечисление)
- Форматирование `gofmt` в `internal/domain/tag/tag.go`, `internal/domain/widget/{input_port,port_binding}.go`, `internal/interface/ui/library/*.go`, `internal/interface/ui/project/{preview,project}.go` — без изменения поведения

## [0.0.24] - 2026-09-18

### Added

- **Протокол доменных событий клиент-сервер на базе SSE** — сервер транслирует все события `EventBus` подключённым клиентам через `GET /api/v1/events`, клиент отображает их в новой панели лога в статус-баре:
  - `internal/interface/restapi/event_hub.go` — `eventHub`: подписывается на `event.AllEventTypes()` ровно один раз при создании и раздаёт каждое опубликованное событие всем текущим SSE-клиентам (`broadcast`); у каждого клиента канал `messages` с буфером `eventClientBufferSize = 64`; при переполнении буфера клиент считается медленным и отключается (`closed = true`, канал закрывается), не блокируя `Publish` для остальной части приложения; `addClient`/`removeClient` учитывают лимит `maxClients`
  - `internal/interface/restapi/events.go` — `EventsHandlers.GetEvents`: хендлер `GET /api/v1/events`; проверяет лимит подключений до апгрейда до SSE (при превышении — обычный JSON 503, а не уже отправленный поток); пишет SSE-комментарий `: connected\n\n` сразу после заголовков; публикует `ClientConnectedEvent`/`ClientDisconnectedEvent` (через `defer`) при подключении/отключении; история не воспроизводится — только события, случившиеся после подключения
  - `internal/domain/client` (новый пакет) — `Client` — маркерный интерфейс-идентичность для событий жизненного цикла SSE-подключения; поведение агрегата пока не определено
  - `internal/domain/event/client_event.go`, `client_connected_event.go`, `client_disconnected_event.go` — `ClientEvent` (база с `ClientID() id.ID[client.Client]`), `ClientConnectedEvent`, `ClientDisconnectedEvent`; `EventType` расширен значениями `EventTypeClientConnected`/`EventTypeClientDisconnected`
  - `internal/domain/event/event_type.go` — `AllEventTypes()`: возвращает все известные `EventType` кроме `EventTypeUnknown`; используется хабом для универсальной подписки без ручного перечисления типов при добавлении новых
  - `internal/domain/event/event_bus.go` — `EventBus.Subscribe` теперь возвращает `Subscription` (непрозрачный хендл `{eventType, id}`), добавлен метод `Unsubscribe(Subscription)`; внутреннее хранилище обработчиков сменилось с `map[EventType][]EventHandler` на `map[EventType]map[uint64]EventHandler`, чтобы обработчики (не сравнимые как `func`) можно было адресно удалять; `internal/infrastructure/mqtt/event_bus_mqtt.go` обновлён под новую сигнатуру (декоратор проксирует `Subscribe`/`Unsubscribe` во внутреннюю шину)
  - `internal/interface/restapi/problems.go` — `NewServiceUnavailableError()`: RFC 7807 503 для случая превышения лимита одновременных SSE-подключений
  - `internal/interface/restapi/router.go` — `NewRouter` принимает `event.EventBus` и `maxSSEClients`; регистрирует `GET /api/v1/events`; `APIRouter.Close()` отписывает хаб от шины при graceful shutdown
  - `cmd/combined_server/main.go` — конфигурация через `github.com/alexflint/go-arg`: `serverArgs.MaxSSEClients` (env `MAX_SSE_CLIENTS`, по умолчанию 100); хаб зарегистрирован в `gracedownManager` как отдельный интерфейс `"Event Hub"` с таймаутом остановки 15s
  - `internal/interface/ui/eventlog` (новый пакет) — компонент **`Bar`**: статус-бар (текущее время, обновляется раз в секунду через `ctx.After`, + кнопка «Events») и раскрываемая панель лога событий под ним:
    - Подключение к `/api/v1/events` через нативный `EventSource` держится всё время жизни компонента независимо от того, открыта ли панель; браузер сам переподключается при разрыве, история не восстанавливается ни на клиенте, ни на сервере
    - Буфер `entries` ограничен `maxEntries = 500`; старые записи вытесняются независимо от активных фильтров (фильтры влияют только на отображение)
    - Панель — фиксированная высота на `visibleRows = 10` строк с внутренним скроллом, не меняется в зависимости от количества записей
    - Фильтры по категориям (`event_types.go`): `Tag`, `Widget`, `WidgetType`, `Scene`, `Value`, `Client`, `Other`; каждый тип события из `eventTypeInfoByType` маппится на категорию и человекочитаемый шаблон описания; по умолчанию скрыта только категория `Value` (поштучные обновления значений тегов иначе засоряют лог); состояние фильтров персистируется в `localStorage` (`eventlog:filters`)
    - `internal/interface/ui/root/root.go` — `eventlog.NewBar(r.apiServerURL)` смонтирован в `Root.Render()`
  - `tests/api/bruno_collections/growscada/get_events.yml` — запрос для ручной проверки SSE-потока
  - `internal/domain/event/event_test.go` — тесты на `Subscribe`/`Unsubscribe` (адресное снятие обработчика без затрагивания остальных подписчиков того же типа)
  - `internal/interface/restapi/events_test.go` — интеграционные тесты SSE-хендлера: подключение/формат сообщений, лимит `maxSSEClients` → 503, публикация `client_connected`/`client_disconnected`, отключение медленного клиента
  - `internal/interface/ui/eventlog/bar_test.go`, `event_types_test.go` — юнит-тесты фильтров по умолчанию и маппинга типов событий на категории/описания

### Changed

- `go.mod` — новая прямая зависимость `github.com/alexflint/go-arg` (+ indirect `github.com/alexflint/go-scalar`); `github.com/pressly/goose/v3`, `github.com/testcontainers/testcontainers-go`, `github.com/testcontainers/testcontainers-go/modules/postgres` переведены из indirect в прямые зависимости
- `Makefile` — добавлена цель `bench`: `go test -run=^$ -bench=. -benchmem ./...`

## [0.0.23] - 2026-09-14

### Changed

- **`Widget` стал entity внутри агрегата `Scene`** вместо самостоятельного aggregate root — устраняет несогласованность между доменной моделью и слоем событий при удалении сцены (виджеты удалялись каскадом в БД, но `WidgetDeletedEvent` не публиковался, ломая аудит в History). Единая версия `Scene.Version()` теперь служит границей optimistic concurrency и для сцены, и для всех её виджетов.
  - `internal/domain/widget/widget.go`: `Widget` лишился `SceneID()` и `Version()`; `NewWidget()` больше не принимает `sceneID`/`version`
  - `internal/domain/widget/widget_repository.go` удалён; ошибки `ErrWidgetNotFound`/`ErrWidgetInvalidInput` перенесены в новый `internal/domain/widget/errors.go` (`ErrWidgetConflict` убран — конфликт версии теперь выражается через `scene.ErrSceneConflict`)
  - `internal/domain/scene/scene.go`: `Scene` получил `Widgets() []widget.Widget`; `NewScene()` принимает список виджетов
  - `internal/domain/scene/scene_repository.go`: `SceneRepository` — единственная точка доступа к виджетам (`NextWidgetID`, `FindWidgetsBySceneID`, `FindWidgetByID`, `AddWidget`, `UpdateWidget`, `DeleteWidget`, `FindWidgetsByTypeID`); `AddWidget`/`UpdateWidget` проверяют версию сцены и атомарно её инкрементируют, `DeleteWidget` инкрементирует без проверки версии (удаление не рискует затереть чужие изменения)
  - `internal/infrastructure/postgres/repositories/widget_repository_postgres.go` удалён, логика объединена в `scene_repository_postgres.go`; общие функции сканирования/сериализации виджета вынесены в новый `widget_scan.go`. Точечные операции (`AddWidget`/`UpdateWidget`/`DeleteWidget`) делают целевой `UPDATE`/`INSERT`/`DELETE` по одной строке `widgets` + инкремент `scenes.version` в одной транзакции, не перечитывая все виджеты сцены. `DeleteByID` сцены читает её виджеты до каскадного удаления (для последующей публикации событий) в той же транзакции, что и сам `DELETE`
  - `internal/application/scene_application_service.go`: `SceneService` получил методы работы с виджетами (`FindWidgetsBySceneID`, `FindWidgetByID`, `CreateWidget`, `UpdateWidget`, `DeleteWidgetByID`), включая валидацию port bindings против `WidgetType` (перенесена из `WidgetService`). `DeleteSceneByID` публикует `WidgetDeletedEvent` для каждого виджета сцены перед `SceneDeletedEvent`
  - `internal/application/widget_application_service.go` удалён — `WidgetService` упразднён
  - `internal/application/widget_type_application_service.go`: `removeOrphanedPortBindings` теперь использует `SceneRepository.FindWidgetsByTypeID` (кросс-сценовый запрос, единственное легитимное исключение из правила "виджет только через сцену") вместо `WidgetRepository`
  - `internal/application/appdto/widget.go`: `Widget`/`CreateWidgetInput`/`UpdateWidgetInput` получили `SceneVersion` вместо `Version`; `SceneID` убран из `Create/UpdateWidgetInput` (передаётся отдельным параметром, приходит из URL)
  - REST API: `/api/v1/widgets/*` заменён на вложенный `/api/v1/scenes/{sceneId}/widgets` и `/api/v1/scenes/{sceneId}/widgets/{widgetId}` (breaking change); `internal/interface/restapi/widgets.go`, `router.go`, `restdto/widget.go` обновлены; тело запроса/ответа виджета несёт `scene_version` вместо `version`, `scene_id` в теле запроса убран (уже есть в пути)
  - `cmd/combined_server/main.go`: убрана отдельная wiring для `widgetRepository`/`widgetService`
  - UI (`internal/interface/ui/project/`): `dto.go`, `widget_ops.go` обновлены под вложенные URL и версионирование через сцену (`currentSceneVersion()`/`applyNewSceneVersion()` вместо версии на виджет); `scene_ops.go`/`render_scene.go` — удаление сцены теперь подтверждается диалогом с числом виджетов (`confirmDeleteScene()`)
  - `internal/infrastructure/postgres/migrations/20260601000002_drop_widgets_version.sql` — колонка `widgets.version` удалена как более не используемая (единственная версия — `scenes.version`); каскад `ON DELETE CASCADE` на `widgets.scene_id` не тронут
  - `internal/infrastructure/postgres/test_data/20990101000000_insert_test_data.sql`: `INSERT INTO widgets` больше не указывает колонку `version` — без этого seed падал с ошибкой (колонки не существует) и 3 примера виджетов (`status-circle-main`, `speedometer-max-connections`, `string-ticker-status`) не создавались при `make db_up`
  - `tests/api/bruno_collections/growscada/`: `post_widgets.yml`, `put_widget_by_id.yml`, `delete_widget_by_id.yml`, `get_widget_by_id.yml` переведены на вложенные URL и `scene_version`; `get_widgets.yml` (глобальный список виджетов) удалён — эндпоинта больше нет

### Removed

- Глобальные REST-эндпоинты `GET/POST /api/v1/widgets`, `GET/PUT/DELETE /api/v1/widgets/{id}` — виджет больше не адресуется независимо от сцены

## [0.0.22] - 2026-06-14

### Added

- `internal/interface/ui/uiutil/srcdoc.go` — новый пакет с общими утилитами для sandboxed-iframe превью:
  - **`BuildSrcdoc()`** — объединённая версия `buildSrcdoc()` из `library` и `project`; дополнительно инжектирует `<style>html,body{background:<bgColor>}</style>` в `<head>` iframe, чтобы фон совпадал с темой приложения; защищает от преждевременного закрытия `<style>` через `styleCloseRE.ReplaceAllString`
  - **`SetIframeSrcdoc()`** — общий хелпер для прямого DOM-присвоения `srcdoc` по `id`
  - **`IframeBgColor()`** — читает CSS-переменную `--bg` из `document.documentElement` через `getComputedStyle`; fallback `"#ffffff"`
  - **`IframeTextMuted()`** — читает `--text-muted`; fallback `"#888888"`
  - **`IsValidJSIdentifier()`** — проверяет, что строка соответствует `^[a-zA-Z_$][a-zA-Z0-9_$]*$`; используется для валидации имён портов перед добавлением
  - **`PortValueToJS()`** — перенесена из `library` и `project` (ранее приватная); логика без изменений

### Changed

- `internal/interface/ui/library/render_editor.go`:
  - Приватные `buildSrcdoc()` и `portValueToJS()` удалены — заменены вызовами `uiutil.BuildSrcdoc()` и `uiutil.PortValueToJS()`
  - `previewFrame.setSrcdoc()` удалена — `OnMount`/`OnUpdate` теперь вызывают `uiutil.SetIframeSrcdoc()`
  - iframe получает `allowtransparency="true"` и `background: transparent` для сквозной прозрачности
  - Валидация имени порта расширена: добавлена проверка `uiutil.IsValidJSIdentifier(name)` перед добавлением порта
  - Превью Library теперь передаёт `uiutil.IframeBgColor()` в `BuildSrcdoc` — фон iframe синхронизирован с темой

- `internal/interface/ui/project/preview.go`:
  - Приватные `buildSrcdoc()`, `portValueToJS()`, `setSrcdoc()` удалены из файла — заменены `uiutil`-аналогами
  - `widgetPreviewFrame` и `widgetThumbnailFrame` получают `allowtransparency="true"` и `background: transparent`

- `internal/interface/ui/project/render_scene.go`:
  - `renderSceneCanvas()` вычисляет `bg` и `textMuted` один раз через `uiutil.IframeBgColor()` / `uiutil.IframeTextMuted()` и передаёт их в `renderWidget()`
  - Подпись `renderWidget()` расширена параметрами `bg, textMuted string`; fallback-текст ненайденного виджета использует `textMuted` вместо захардкоженного `#888`

- `internal/interface/ui/project/render_widget_types.go`:
  - Миниатюры в панели типов строятся через `uiutil.BuildSrcdoc()` с явным `bg`

- `internal/interface/ui/project/render_properties.go`:
  - `buildSrcdoc()` в обработчике `OnChange` заменена на `uiutil.BuildSrcdoc()` с `uiutil.IframeBgColor()`

- `internal/interface/ui/library/library.go`:
  - Добавлено поле `ThemeMode string` (экспортированное — go-app отслеживает изменения полей компонента для планирования ре-рендера)
  - `OnMount` подписывается на состояние `"theme"` через `ctx.ObserveState`

- `internal/interface/ui/project/project.go`:
  - Аналогично `library`: добавлено `ThemeMode string` и `ctx.ObserveState("theme", &p.ThemeMode)`

- `internal/interface/ui/root/root.go`:
  - `setTheme()` теперь публикует тему в `ctx.SetState("theme", mode)`; то же происходит при `OnMount` для восстановления сохранённой темы — компоненты `Library` и `Project` получают уведомление и перерисовываются с актуальным фоном

## [0.0.21] - 2026-06-08

### Added

- `internal/interface/ui/project/preview.go` — три новых компонента и два вспомогательных функции:
  - **`widgetPreviewFrame`** — sandboxed-iframe для отображения виджета на холсте сцены; `pointer-events:none` пропускает мышиные события к прозрачному drag-оверлею; `OnMount`/`OnUpdate` устанавливают `srcdoc` через прямое DOM-присвоение (обход ограничения браузера на перезагрузку `<iframe srcdoc>` через `setAttribute`)
  - **`widgetThumbnailFrame`** — sandboxed-iframe для миниатюры в панели типов; рендерит виджет в нативном размере и уменьшает через CSS `transform: scale(N)` с `transform-origin: 0 0`; контейнер получает `overflow:hidden` с уже масштабированными размерами
  - **`simInputField`** — управляемое поле ввода для панели симуляции; `OnMount`/`OnUpdate` устанавливают DOM `value` через `getElementById` + `ctx.Defer` (без этого go-app vdom вызывает `removeAttribute` для пустой строки, оставляя поле с устаревшим текстом при переключении между виджетами)
  - **`buildSrcdoc()`** — формирует HTML-документ для `<iframe srcdoc>`: вставляет `htmlTemplate` в `<body>`, оборачивает `script` в `<script>`, генерирует JS-вызов `render(inputs)` на основе входных портов; добавляет `Content-Security-Policy: connect-src 'none'` для блокировки `fetch()`/XHR из iframe; экранирует `</script>` в пользовательском JS, чтобы содержимое поля не могло прервать `<script>` блок
  - **`portValueToJS()`** — конвертирует строковое значение из поля ввода в JS-литерал с учётом `typeHint` (`integer`, `boolean`, `string`); использует `strconv.ParseInt`, `json.Marshal` — инъекция невозможна

- `internal/interface/ui/project/render_properties.go` — панель «Simulate Inputs»:
  - Секция появляется для виджетов с входными портами; показывает поле `simInputField` на каждый порт с меткой типа
  - `OnChange` напрямую обновляет `srcdoc` у iframe `w-preview-<id>` через DOM без ожидания ре-рендера родительского компонента (в go-app v10 обработчики на дочернем компоненте не планируют ре-рендер родителя)
  - `capturedWT widgetTypeItem` захватывается один раз в начале `renderWidgetProperties` и используется в замыкании `OnChange`
  - Тестовые значения хранятся в `p.simInputs[widgetID][portName]` — не сохраняются на сервер

- `internal/interface/ui/project/project.go`:
  - Поле `simInputs map[string]map[string]string` — in-memory хранилище тестовых значений для симуляции
  - Метод `widgetTypeByID(id string) (widgetTypeItem, bool)` — линейный поиск по `p.widgetTypes`

### Changed

- `internal/interface/ui/project/render_scene.go` — виджеты на холсте теперь отображаются как sandboxed-iframe вместо текстовой метки:
  - Содержимое виджета: `widgetPreviewFrame` (`pointer-events:none`) + прозрачный `div`-оверлей для drag/click + рамка выделения (`pointer-events:none`, показывается при `isSelected`)
  - При ненайденном `WidgetType` (тип ещё не загружен или удалён) показывается имя виджета как текстовый fallback через экранированный HTML
  - Добавлен импорт пакета `"html"` для `html.EscapeString`

- `internal/interface/ui/project/render_widget_types.go` — панель типов виджетов теперь показывает миниатюры `widgetThumbnailFrame` вместо строк «имя + размеры»:
  - Масштаб миниатюры рассчитывается так, чтобы вписать виджет в ширину панели; при превышении `maxThumbH = 120` px пересчитывается scale и `thumbW` с повторной проверкой минимальной ширины 50 px
  - При отсутствии `HtmlTemplate` показывается пунктирный прямоугольник с текстом «no HTML»

- `internal/interface/ui/project/dto.go` — структура `widgetTypeItem` дополнена полями `HtmlTemplate string` и `Script string` для передачи шаблона и скрипта в компоненты предпросмотра

## [0.0.20] - 2026-06-07

### Added

- `internal/interface/ui/library/render_editor.go` — компонент **`inputDataField`**: тонкая обёртка над полем ввода в панели «Input Data»:
  - Решает проблему go-app vdom: при пустом значении фреймворк вызывает `removeAttribute`, а не присваивает `input.value = ""`, из-за чего браузер не очищает поле при переключении WidgetType
  - `OnMount` и `OnUpdate` присваивают свойство `value` напрямую через `getElementById` + `ctx.Defer`; `OnUpdate` пропускает DOM-запись если `Val` не изменился (`lastVal` guard)
  - Поле `Lib *Library` заменено на `OnChange func(string)` — компонент самодостаточен и не держит обратную ссылку на родителя

- `internal/interface/ui/library/render_editor.go` — компонент **`previewFrame`**: тонкая обёртка над sandboxed-iframe предпросмотра:
  - Решает ограничение браузера: обновление атрибута `srcdoc` через `setAttribute` не перезагружает iframe; работает только прямое присвоение JS-свойства `iframe.srcdoc`
  - `OnMount` и `OnUpdate` вызывают `setSrcdoc()`, которая делает `getElementById(p.ID).Set("srcdoc", p.Srcdoc)`
  - Поле `ID string` вынесено в структуру — исключает хардкод и позволяет иметь несколько экземпляров

- `internal/infrastructure/postgres/test_data` — новый тег `status_message` (`6ba7b819-...`): строковый тег для демонстрации виджета String Ticker

### Changed

- `internal/infrastructure/postgres/test_data/20990101000000_insert_test_data.sql` — примеры WidgetType заменены на наглядные JavaScript-виджеты:
  - **Boolean Circle** (`0001`) — SVG-круг, зеленеет при `inputs.state = true`
  - **Integer Speedometer** (`0002`) — SVG-спидометр 0–100: дуга и стрелка, цвет меняется по порогам (зелёный / оранжевый / красный)
  - **String Ticker** (`0003`) — бегущая строка с marquee-анимацией при переполнении контейнера
  - Удалены старые примеры: Pressure Gauge, Temperature Indicator, Boolean Lamp (CSS/div), Flow Meter (Python), Level Sensor (Lua)
  - DOWN-миграция расширена: теперь удаляет и старые ID (`0003`, `0004`, `0005-old`) — безопасно при их отсутствии

- `internal/interface/ui/library/render_editor.go` — `buildSrcdoc` теперь вызывает `render(inputs)` вместо `update(inputs)` (исправлено несоответствие с именем функции во всех примерах)

- `tests/api/bruno_collections` — приведено в соответствие с актуальными тестовыми данными:
  - `environments/localhost.yml` — `WIDGET_TYPE_ID` исправлен: `0005-...-0001` → `0001-...-0001` (Boolean Circle)
  - `post_widget_types.yml`, `put_widget_type_by_id.yml` — скрипты переименованы: `function update` → `function render`
  - `post_widgets.yml`, `put_widget_by_id.yml` — порт `pressure`/`temperature` → `state`; убрана несуществующая переменная `{{TAG_ID_2}}`

## [0.0.19] - 2026-06-04

### Added

- `internal/interface/ui/toast` — новый пакет **toast** для централизованного отображения ошибок (`toast.go`):
  - `Container` — фиксированный overlay, монтируется один раз в `root.go`; слушает глобальный экшн `toast.add` через `ctx.Handle`
  - `Problem` — структура RFC 9457: `Title`, `Status`, `Detail`, `Instance`, `Method` (HTTP-метод из `resp.Request`, не сериализуется)
  - `FromHTTPError(resp)` — читает и закрывает тело ответа, парсит как RFC 9457; при ошибке парсинга формирует синтетическую запись из `http.StatusText` и сырого тела
  - `NetworkError(err)` — обёртка для транспортных ошибок (connection refused, timeout и т.п.)
  - Анимация на трёх фазах (`entering → visible → exiting`) через вложенные `ctx.After`; CSS-кейфреймы инжектируются в `<head>` через `injectToastCSS()` один раз при монтировании `Root`
  - Защита от регрессии фазы: `setPhase` игнорирует любые переходы, если таблица уже в фазе `exiting`
  - Отображение: HTTP-метод (моноширинный), статус-код + заголовок, время создания (правый верхний угол), детальное сообщение, instance URI
  - Цвета статусов и фон таблички управляются CSS-переменными темы (`--toast-bg`, `--toast-shadow`, `--toast-err-*`, `--toast-warn-*`, `--toast-info-*`, `--toast-muted-*`) — корректно адаптируются при переключении светлой/тёмной/авто темы

- `internal/interface/ui/root` — добавлены переменные `--toast-*` в `lightVars()` и `darkVars()` (`root.go`); светлая тема: белый фон `rgba(252,252,252,0.98)`, тёмные цвета текста; тёмная тема: прежний полупрозрачный тёмный фон

### Changed

- `internal/interface/ui/library` (`library.go`, `render_list.go`, `widget_type_ops.go`) — поле `fetchErr string` и его inline-рендеринг удалены; все HTTP-ошибки (`http.Get`, `http.Post`, `http.DefaultClient.Do`) теперь диспатчат `toast.ActionAdd`
- `internal/interface/ui/project` (`project.go`, `scene_ops.go`, `tags_ops.go`, `tags_render.go`, `widget_ops.go`) — поля `fetchErr string` и `tagFetchErr string` удалены; все HTTP-ошибки переведены на `toast.ActionAdd`
- `internal/interface/ui/project/scene_ops.go` — при сетевой ошибке или ответе `>= 400` в `saveSceneProperties` и `commitSceneEdit` вызывается `p.loadScenes(ctx)` для отката оптимистичного обновления `p.scenes[i]`
- `internal/interface/ui/project/widget_ops.go` — при сетевой ошибке или ответе `>= 400` в `putWidget` вызывается `p.loadWidgets(ctx)` для отката оптимистичного обновления `p.widgets[idx]`; порог ошибки выровнен с остальными хэндлерами (`>= 400` вместо `< 200 || >= 300`)
- `internal/interface/ui/library/widget_type_ops.go` — порог ошибки в `applyChanges` выровнен: `>= 400` вместо `< 200 || >= 300`
- `Makefile` — `make run` открывает Chromium с флагом `--start-maximized`; `make full_restart` корректно останавливает Docker Compose и удаляет том перед пересозданием БД

## [0.0.18] - 2026-06-02

### Fixed

- `internal/interface/ui/project` — исправлено смещение виджета при изменении размера через хендл SE после поворота (`project.go`, `render_scene.go`):
  - **Причина**: CSS-матрица виджета содержит трансляцию `e = posX + ox·(1−cosA) + oy·sinA`, `f = posY − ox·sinA + oy·(1−cosA)`, где `ox = originX·W`, `oy = originY·H`. При изменении ширины/высоты `ox` и `oy` менялись, сдвигая левый-верхний угол виджета на экране — даже если `posX`/`posY` оставались прежними
  - При mousedown на хендле SE теперь фиксируется канвас-позиция левого-верхнего угла (`resizeStartE`, `resizeStartF`) и текущий `origin` (`resizeOriginX`, `resizeOriginY`)
  - В каждом mousemove после вычисления нового размера `posX`/`posY` пересчитываются так, чтобы `(e, f)` оставалась неизменной: `newPosX = E − newOx·(1−cosA) − sinA·newOy`, `newPosY = F + sinA·newOx − newOy·(1−cosA)`
  - Панель свойств обновляет поля X/Y синхронно с изменением размера

## [0.0.17] - 2026-06-01

### Added

- `internal/interface/ui/project` — панель **Properties** при отсутствии выбранного виджета теперь отображает свойства текущей сцены вместо текста «Select a widget» (`render_properties.go`, `scene_ops.go`, `project.go`):
  - Поля Name (text input), Size W × H (number spinbutton), Background HTML (textarea) — редактируемые, сохраняются через PUT `/api/v1/scenes/:id` при потере фокуса или нажатии Enter
  - Добавлены поля `editingScenePropsName`, `editingScenePropsWidth`, `editingScenePropsHeight`, `editingScenePropsBG` в структуру `Project`
  - `syncSceneEditingFields()` — синхронизирует поля панели с выбранной сценой; вызывается при загрузке, переключении вкладки сцены и переименовании через inline-редактор
  - `saveSceneProperties(ctx)` — отправляет PUT с оптимистичным обновлением in-memory + обновлением `Version` из ответа

### Fixed

- `internal/interface/ui/project` — исправлен сброс выбора виджета: один клик по холсту теперь снимает фокус (`project.go`, `render_scene.go`, `widget_ops.go`):
  - **Причина**: `OnMouseDown` на виджете сразу выставлял `draggingWidgetID`, из-за чего `finalizeAllDrags` при `mouseup` ставил `dragJustEnded = true` даже без реального перемещения — первый клик по сцене проглатывался
  - Добавлен флаг `dragDidMove bool`: выставляется в `OnMouseMove` только при наличии активного drag-операции; `finalizeAllDrags` теперь устанавливает `dragJustEnded = dragDidMove` (и сбрасывает `dragDidMove = false`) — подавление срабатывает только после реального перетаскивания, а не при обычном клике
  - Добавлен `OnMouseDown` на холст, сбрасывающий `dragJustEnded`: устраняет случай, когда после drag браузер не генерирует `click` (mousedown и mouseup на разных элементах) — флаг оставался `true` и следующий намеренный клик по сцене тоже проглатывался

## [0.0.16] - 2026-06-01

### Fixed

- `internal/interface/ui/project` — исправлен баг с перемещением точки **Origin** у повёрнутого виджета (`project.go`, `render_scene.go`):
  - При ненулевом угле поворота перетаскивание хендла Origin перемещало весь виджет вместо точки привязки. Три последовательно устранённые ошибки:
    1. Экранная дельта мыши не преобразовывалась в локальное пространство виджета; добавлено обратное вращение: `d_ox = cosA·dx + sinA·dy`, `d_oy = −sinA·dx + cosA·dy` с сохранением `originDragRotDeg` при начале drag
    2. Изменение `(ox, oy)` смещало компоненты трансляции CSS-матрицы (`e = posX + ox·(1−cosA) + oy·sinA`), визуально сдвигая виджет; добавлена компенсация позиции: `newPosX = startPosX − dOx·(1−cosA) − dOy·sinA`, `newPosY = startPosY − dOy·(1−cosA) + dOx·sinA`; добавлены поля `originStartPosX`, `originStartPosY`
    3. Два исправления объединены: дельта мыши сначала поворачивается в локальные оси виджета, затем кламп-скорректированные локальные дельты подставляются в формулу компенсации позиции — хендл точно следует за курсором при любом угле поворота, виджет остаётся на месте
- `internal/interface/ui/project` — исправлен сброс выбора (selection) виджета при отпускании хендлов вращения и Origin над холстом (`render_scene.go`, `widget_ops.go`, `project.go`):
  - После завершения drag браузер генерировал отдельное событие `click`, которое поднималось к canvas (минуя контейнер виджета) и вызывало `clearWidgetSelection`
  - Добавлен `OnClick` → `stopPropagation` на контейнер виджета: перехватывает клики по хендлам, когда мышь остаётся в пределах виджета
  - Добавлен флаг `dragJustEnded bool`: `finalizeAllDrags` выставляет его при завершении любого drag; canvas `OnClick` при виде флага сбрасывает его и пропускает очистку selection — покрывает случай отпускания мыши над canvas вне контейнера виджета

## [0.0.15] - 2026-05-29

### Added

- `internal/domain/widget` — **InputPort** и **PortBinding** Value Objects:
  - `InputPortName` — валидируется как JS-идентификатор (`^[a-zA-Z_$][a-zA-Z0-9_$]*$`); `NewInputPortNameFromStorage` пропускает regex для доверенных данных из БД
  - `InputPort` — именованный типизированный слот ввода на WidgetType; поля `Name`, `Description`, `TypeHint` (`tag.TagType`); `TypeHint == TagTypeUnknown` означает «любой тип тега»
  - `PortBinding` — связь порта (по имени) с конкретным экземпляром `Tag` (через `id.ID[tag.Tag]`)
  - `NewWidgetType` возвращает `(WidgetType, error)` и проверяет уникальность имён портов; при дубликате — `ErrDuplicateInputPortName`
  - `WidgetType.InputPorts()` — возвращает защитную копию среза
  - `Widget.PortBindings()` заменяет `Widget.TagIDs()` — виджет теперь хранит именованные привязки портов вместо плоского списка UUID тегов
- `internal/application` — `InputPortDTO`, `PortBindingDTO` добавлены в `appdto/widget.go` и `appdto/widget_type.go`; application-сервисы `WidgetService` и `WidgetTypeService` маппят порты и привязки через `NewInputPort` / `NewPortBinding`
- `internal/infrastructure/postgres/migrations`:
  - `20260601000000_add_input_ports_to_widget_types.sql` — колонка `input_ports JSONB NOT NULL DEFAULT '[]'` в `widget_types`; элемент: `{"name":"…","description":"…","type_hint":"…"}`
  - `20260601000001_replace_tag_ids_with_port_bindings_in_widgets.sql` — добавляет `port_bindings JSONB`, мигрирует существующие `tag_ids` в синтетические имена `port_0`, `port_1`, …, затем удаляет колонку `tag_ids`
- `internal/infrastructure/postgres/repositories` — репозитории `widget_repository_postgres` и `widget_type_repository_postgres` сериализуют/десериализуют `InputPort[]` и `PortBinding[]` как JSONB; `widget_type_repository_postgres_test.go` — новый набор интеграционных тестов
- `internal/interface/restapi/restdto` — `InputPortDTO` и `PortBindingDTO` добавлены в REST-DTO; поле `tag_ids []uuid` заменено на `port_bindings []PortBindingDTO` в `WidgetRequest/Response`; `input_ports []InputPortDTO` добавлено в `WidgetTypeRequest/Response`
- `internal/interface/ui/library` — редактор типов виджетов расширен колонкой **Input Ports**:
  - список существующих портов с кнопкой удаления (✕)
  - форма добавления: поля Name (JS identifier), Description (опционально), Type Hint (any / string / boolean / integer)
  - порты сохраняются через PUT вместе с остальными полями типа виджета
- `internal/interface/ui/project` — панель свойств виджета заменяет тег-список секцией **Port Bindings**:
  - для каждого входного порта WidgetType отображается `<select>` с тегами, отфильтрованными по `type_hint`; текущая привязка подсвечивается; можно сменить тег или снять привязку (`— unbound —`)
  - изменения сохраняются через PUT виджета
- `internal/interface/ui/uidto` — новый пакет `uidto` с `InputPortDTO`, общим для пакетов `library` и `project`, устраняет дублирование DTO
- `internal/interface/ui/root` — система тем оформления:
  - CSS custom properties (`var(--bg)`, `var(--accent)`, `var(--error)`, …) инжектируются через `<style id="gs-theme-vars">` в `<head>` при каждой смене темы
  - Три режима: `light`, `dark`, `auto` (авто использует `@media (prefers-color-scheme: dark)`)
  - Кнопка переключения темы: одиночный cycling-значок `◑` (auto) → `☀` (light) → `☾` (dark); режим сохраняется в `localStorage` (`root:theme`)
  - CSS-сброс `html, body { margin: 0; overflow: hidden }` устраняет постоянно видимый вертикальный scrollbar
- `internal/interface/ui/project` — изменяемые ширины панелей в Scenes:
  - Левая панель Widget Types и правая панель Properties разделены 5px-разделителями с `cursor: col-resize`
  - Drag-изменение ширины: левая [100, 480] px, правая [160, 600] px; ширины персистируются в `localStorage` (`project:widgetTypeWidth`, `project:propertiesWidth`)
- Диалоги подтверждения (`app.Window().Call("confirm", …).Bool()`) перед всеми деструктивными операциями в Library и Project
- Персистентность состояния UI в `localStorage`: активная вкладка навигации (`root:mode`), активная сцена (`project:sceneID`), активная под-вкладка Scenes/Tags (`project:subTab`), выбранный тип виджета в Library (`library:selectedID`), выбранный виджет на холсте
- Юнит-тесты `internal/domain/widget/input_port_test.go` — `TestNewInputPortName_*`, `TestNewInputPort_*`, `TestNewWidgetType_DuplicateInputPortName`, `TestNewWidgetType_UniqueInputPorts_OK`, `TestNewWidgetType_InputPortsCopied`

### Changed

- `Widget.TagIDs() []id.ID[tag.Tag]` → `Widget.PortBindings() []PortBinding` во всём стеке (домен → application → инфраструктура → REST → UI)
- `NewWidgetType` сигнатура: добавлен параметр `someInputPorts []InputPort`; возврат изменён с `WidgetType` на `(WidgetType, error)`
- `tag.NewTagType` принимает `"unknown"` и `""` как синонимы `TagTypeUnknown` без ошибки — упрощает маппинг `type_hint` из JSON
- `internal/interface/restapi/restdto/widget_type.go` — `WidgetTypeResponse.InputPorts` ранее отсутствовало и добавлено в Put-запрос; имя поля `input_ports` унифицировано с JSONB-схемой БД
- Все UI-компоненты Library и Project переведены с жёстко заданных hex-цветов на CSS custom properties; кнопки Delete стилизованы красным (`var(--error-*)`), Create — приглушённым акцентом (`var(--accent-*)`)

### Fixed

- Виджет не пропадал с холста после удаления: `loadWidgets` внутри `ctx.Dispatch` запускал `ctx.Async` в запрещённом контексте; заменено прямой фильтрацией среза `p.widgets` (оптимистичное обновление UI) — `widget_ops.go`
- Перетаскивание точки Origin перемещало виджет: удалена компенсация `Position.X/Y`; теперь изменяются только `Origin.X/Y` — `render_scene.go`, `project.go`
- `WidgetTypeService.UpdateWidgetType` при пустом срезе `InputPorts` в запросе обнулял порты вместо сохранения существующих; исправлено явной передачей портов из request — `widget_type_application_service.go`
- `WidgetService.UpdateWidget` аналогично игнорировал `PortBindings` при обновлении позиции/размера — `widget_application_service.go`, `widget_ops.go`
- UI Library: форма добавления порта не очищалась после добавления и не валидировала пустое имя — `render_editor.go`

## [0.0.14] - 2026-05-28

### Added

- `internal/interface/ui/project` — вкладка **Tags** в Project-компоненте:
  - `projectSubTab` — тип-перечисление (`scenes` / `tags`); `activeSubTab` хранит активную вкладку; `renderSubTabs()` рендерит таб-бар; `subTab()` — helper для отдельного таба с подсветкой активного состояния
  - `tags_render.go` — рендеринг тегов: `renderTagsPanel` (две колонки), `renderTagListColumn` (список с кнопками Create/Delete), `renderTagList` (кликабельные строки с именем и типом), `renderTagPropertiesPanel` (панель свойств выбранного тега: Name, Type, Value, Quality), `renderTagCreateForm` (форма с полями Name и Type, кнопки Create/Cancel)
  - `tags_ops.go` — сетевые операции: `createTag` (POST `/api/v1/tags`), `deleteTag` (DELETE `/api/v1/tags/{id}`); вспомогательная `defaultTagValue` возвращает начальное значение по типу тега
- `Makefile` — цель `full_restart`: `db_down` → `db_up` → `run`

### Changed

- `dto.go` — `tagItem` расширен полями `Type`, `Value`, `Quality`, `Version`; добавлены `createTagRequest` и `createTagResponse`
- `docs/ubiquitous_language/ubiquitous_language.md` — термин `Alarm` переименован в `Alert` во всех вхождениях

## [0.0.13] - 2026-05-27

### Fixed

- Оптимистичная блокировка виджетов: `UpdateWidget` с `version=0` в теле запроса тихо уходил в INSERT вместо UPDATE, вызывая ошибку PK-ограничения (HTTP 500 вместо 409). Исправлено: application-сервис выполняет `FindByID` + сравнение версий до `Save`; репозиторий добавил ветку `IsCommitted()` и fallback-ошибку для неопределённых состояний версии
- Оптимистичная блокировка сцен: `UpdateSceneInput` не содержал поле `Version`, что делало OCC структурно невозможным. Исправлено: добавлено поле `Version int` в `UpdateSceneInput`, `UpdateSceneRequest` и `UpdateSceneResponse`; `UpdateScene` проверяет `found.Version().Number() != input.Version` → `ErrSceneConflict`
- Оптимистичная блокировка типов виджетов: `UpdateWidgetType` игнорировал версию клиента — конкурентные правки перезаписывали друг друга без 409. Исправлено аналогичной проверкой версии в application-сервисе
- Nil-UUID валидация: `CreateWidget`/`UpdateWidget` с отсутствующим `scene_id` или `type_id` в JSON тихо сохраняли виджет с произвольным UUID. Исправлено явными проверками `== uuid.Nil` → `ErrWidgetInvalidInput` в application-сервисе
- `PostScenes` возвращал HTTP 500 при ошибках валидации домена; теперь возвращает 422 Unprocessable Entity при `ErrSceneValidation`
- `PostWidgets` возвращал HTTP 500 при невалидном вводе; теперь возвращает 400 Bad Request при `ErrWidgetInvalidInput`

### Changed

- `classifyUpdateConflict` — вынесена из четырёх репозиториев в единую пакетную generic-функцию `classifyUpdateConflict[T]` в `update_conflict_classification.go`; введён тип `tableName` с константами `tableNameTags`, `tableNameScenes`, `tableNameWidgets`, `tableNameWidgetTypes`, исключающий передачу произвольных строк
- `renderSceneCanvas` — удалён избыточный клиентский фильтр `w.SceneID != p.selectedSceneID`; `FindBySceneID` уже ограничивает выборку на уровне БД
- `finalizeAllDrags` — четыре одинаковых if-блока заменены циклом по `[]*string` с указателями на поля структуры
- Тесты: удалена функция-заглушка `mustTagIDFromUUID` (возвращала аргумент без изменений); удалена мёртвая переменная `_ = i` в range-цикле `widget_application_service_test.go`

## [0.0.12] - 2026-05-27

### Added

- `internal/domain/scene` — новый пакет домена для сцен:
  - `Scene` — агрегат (интерфейс + `sceneImpl`): `ID`, `Name`, `Size`, `BackgroundHTML`, `Version`
  - `SceneName` — Value Object; валидация: пустая строка возвращает ошибку
  - `SceneSize` — Value Object; ширина и высота в пикселях; `NewSceneSize` возвращает ошибку при неположительных значениях
  - `BackgroundHTML` — Value Object-обёртка над строкой статического HTML-фона сцены
  - `SceneRepository` — интерфейс с методами `NextID`, `Save`, `FindByID`, `FindAll`, `DeleteByID`; sentinel-ошибки `ErrSceneNotFound`, `ErrSceneConflict`, `ErrSceneValidation`
- `internal/domain/widget` — расширен агрегатом экземпляра виджета:
  - `Widget` — агрегат (интерфейс + `widgetImpl`): экземпляр `WidgetType`, размещённый на `Scene`; поля: `ID`, `Name`, `Position`, `Size`, `Origin`, `Rotation`, `TransformationMatrix`, `TypeID`, `SceneID`, `Labels`, `TagIDs`, `Version`
  - `WidgetName` — Value Object; валидация: пустая строка возвращает ошибку
  - `Position` — Value Object; координаты на холсте (`x`, `y` float64, `z` int для z-index)
  - `Size` — Value Object; ширина и высота в пикселях; `NewSize` возвращает ошибку при неположительных значениях
  - `Origin` — Value Object; нормализованная точка привязки трансформации (`0.0`–`1.0` по каждой оси)
  - `Rotation` — Value Object; угол поворота в градусах (float64)
  - `TransformationMatrix` — Value Object; 2D аффинная матрица CSS `matrix(a,b,c,d,e,f)`, вычисляется из `Position`, `Origin`, `Rotation`, `Size` с учётом точки привязки; метод `CSS()` возвращает строку для `transform:`
  - `WidgetRepository` — интерфейс с методами `NextID`, `Save`, `FindByID`, `FindAll`, `FindBySceneID`, `DeleteByID`; sentinel-ошибки `ErrWidgetNotFound`, `ErrWidgetConflict`, `ErrWidgetInvalidInput`
- `internal/domain/id` — новый пакет обобщённого Value Object `ID[T]`:
  - `ID[T]` — UUID-обёртка с типовым параметром агрегата; исключает перепутывание ID разных агрегатов на этапе компиляции
  - `NewID[T]`, `IDWithUUID[T]`, `IDOption[T]`; при создании без опций генерирует новый UUID автоматически
- `internal/domain/event` — события для жизненного цикла `Widget` и `Scene`:
  - `WidgetCreatedEvent`, `WidgetUpdatedEvent`, `WidgetDeletedEvent`
  - `SceneCreatedEvent`, `SceneUpdatedEvent`, `SceneDeletedEvent`
  - `EventType` расширен 6 новыми значениями
- `internal/application/widget_application_service.go` — `WidgetService` с методами `FindAllWidgets`, `FindWidgetsBySceneID`, `FindWidgetByID`, `CreateWidget`, `UpdateWidget`, `DeleteWidgetByID`; валидация обязательных полей `scene_id` и `type_id` на уровне сервиса; публикует события через `EventBus`
- `internal/application/scene_application_service.go` — `SceneService` с методами `FindAllScenes`, `FindSceneByID`, `CreateScene`, `UpdateScene`, `DeleteSceneByID`; публикует события через `EventBus`
- `internal/application/appdto/widget.go` — DTO `Widget`, `CreateWidgetInput`, `UpdateWidgetInput`; фабрики `NewWidget`, `NewWidgetList`
- `internal/application/appdto/scene.go` — DTO `Scene`, `CreateSceneInput`, `UpdateSceneInput`; фабрики `NewScene`, `NewSceneList`
- `internal/infrastructure/postgres/repositories/widget_repository_postgres.go` — PostgreSQL-реализация `WidgetRepository`; INSERT при `version == Initial`, UPDATE с оптимистичной блокировкой; `classifyUpdateConflict` определяет `ErrWidgetNotFound` vs `ErrWidgetConflict`
- `internal/infrastructure/postgres/repositories/scene_repository_postgres.go` — PostgreSQL-реализация `SceneRepository`; аналогичная стратегия оптимистичной блокировки
- Миграции:
  - `20260516000000_create_widgets.sql` — DDL таблицы `widgets`
  - `20260516000001_create_scenes.sql` — DDL таблицы `scenes`
  - `20260516000002_add_scene_id_to_widgets.sql` — добавлен FK `scene_id` в `widgets`
  - `20260518000001_add_size_to_widgets.sql` — добавлены колонки `width`, `height` в `widgets`
  - `20260526000000_add_transform_to_widgets.sql` — добавлены `origin_x`, `origin_y`, `rotation_degrees` в `widgets`
  - `20260527000000_widget_scene_id_not_null.sql` — `scene_id` переведён в `NOT NULL`
- `internal/interface/restapi/widgets.go` — REST-хэндлеры CRUD для экземпляров виджетов:
  - `GET /api/v1/widgets` — список всех виджетов
  - `GET /api/v1/widgets/{id}` — виджет по ID; 404 при отсутствии
  - `POST /api/v1/widgets` — создание; 201 с объектом; 400 при невалидном вводе
  - `PUT /api/v1/widgets/{id}` — обновление; 404 при отсутствии, 409 при конфликте версий
  - `DELETE /api/v1/widgets/{id}` — удаление; 200 с телом удалённого объекта, 404 при отсутствии
  - `GET /api/v1/scenes/{id}/widgets` — виджеты по сцене (вложенный ресурс)
- `internal/interface/restapi/scenes.go` — REST-хэндлеры CRUD для сцен:
  - `GET /api/v1/scenes` — список всех сцен
  - `GET /api/v1/scenes/{id}` — сцена по ID; 404 при отсутствии
  - `POST /api/v1/scenes` — создание; 201; 422 при ошибке валидации домена
  - `PUT /api/v1/scenes/{id}` — обновление; 404, 409 (конфликт версий), 422 (валидация)
  - `DELETE /api/v1/scenes/{id}` — удаление; 200 с телом удалённой сцены
- `internal/interface/restapi/restdto/widget.go` — HTTP-DTO `WidgetResponse`, `GetWidgetsResponse`, `CreateWidgetRequest/Response`, `UpdateWidgetRequest/Response`
- `internal/interface/restapi/restdto/scene.go` — HTTP-DTO `SceneResponse`, `GetScenesResponse`, `CreateSceneRequest/Response`, `UpdateSceneRequest/Response`
- Bruno-коллекция — новые запросы: `get_scenes`, `get_scene_by_id`, `post_scenes`, `put_scene_by_id`, `delete_scene_by_id`, `get_widgets`, `get_widget_by_id`, `get_widgets_by_scene_id`, `post_widgets`, `put_widget_by_id`, `delete_widget_by_id`
- Юнит-тесты домена `widget` — `widget_test.go`, `widget_name_test.go`, `widget_position_test.go`, `widget_size_test.go`, `widget_origin_test.go`, `widget_rotation_test.go`, `widget_transform_matrix_test.go`
- Юнит-тесты домена `scene` — `scene_test.go`, `scene_name_test.go`, `scene_size_test.go`, `scene_background_html_test.go`
- Юнит-тесты `internal/domain/id/id_test.go` — генерация нового UUID, задание через `IDWithUUID`, `String`, тип-параметр как изолятор
- Интеграционные тесты `internal/application/widget_application_service_test.go` — полный CRUD, валидация обязательных полей, конфликт версий, публикация событий
- Интеграционные тесты `internal/infrastructure/postgres/repositories/widget_repository_postgres_test.go`
- Интеграционные тесты `internal/interface/restapi/widgets_test.go` и `scenes_test.go` — все хэндлеры включая 409 Conflict и 422 Unprocessable Entity

### Changed

- `internal/domain/id` — `TagID`, `WidgetTypeID` и все прочие агрегатные идентификаторы переведены на обобщённый `id.ID[T]` из нового пакета `internal/domain/id`; per-aggregate `TagID`, `WidgetTypeID` удалены как отдельные типы
- `internal/domain/version` — `Version` стал обобщённым `Version[T]`; конструкторы `Initial[T]()` и `Committed[T]()` получили тип-параметр агрегата; исключает применение версии одного агрегата к другому
- `internal/interface/ui/library` — `Library` разбита на отдельные файлы: `library.go` (структура и монтирование), `dto.go`, `render_list.go`, `render_editor.go`, `widget_type_ops.go` (сетевые операции)
- `internal/interface/ui/project` — монолитный `project.go` разбит на: `project.go` (корневая структура), `dto.go`, `matrix.go`, `render_scene.go`, `render_properties.go`, `render_widget_types.go`, `scene_ops.go`, `widget_ops.go`
- Bruno-коллекция — все имена файлов с пробелами переименованы в snake_case (например, `get tag by id.yml` → `get_tag_by_id.yml`)

## [0.0.11] - 2026-05-16

### Added

- `internal/domain/widget` — новый пакет домена для типов виджетов:
  - `WidgetType` — агрегат (интерфейс + `widgetTypeImpl`); immutable: поля не мутируются напрямую, обновление через создание нового экземпляра в репозитории
 - `WidgetTypeID` — Value Object на основе UUID; функциональные опции `WidgetTypeIDWithUUID`; `ParseWidgetTypeID`, `MustParseWidgetTypeID`
  - `WidgetTypeName` — Value Object; валидация: пустая строка возвращает ошибку
  - `HtmlTemplate`, `Script` — Value Object-обёртки над строками; валидация зарезервирована (TODO)
  - `ScriptLanguage` — enum-VO (`javascript`, `python`, `lua`); сериализуется строкой; `NewScriptLanguage` возвращает ошибку для неизвестных значений
  - `WidgetTypeRepository` — интерфейс репозитория с методами `NextID`, `Save`, `FindByID`, `FindAll`, `DeleteByID`; sentinel-ошибки `ErrWidgetTypeNotFound` и `ErrWidgetTypeConflict` (optimistic locking)
- `internal/domain/version` — новый пакет Value Object `Version` для оптимистичной блокировки: константы `Initial` (0) и `Committed` (1); методы `Next()`, `IsCommitted()`, `Number()`; конструктор с валидацией на отрицательные числа
- `internal/domain/event` — события для жизненного цикла `WidgetType`: `WidgetTypeCreatedEvent`, `WidgetTypeUpdatedEvent`, `WidgetTypeDeletedEvent`; `EventType` расширен тремя новыми значениями
- `internal/application/widget_type_application_service.go` — `WidgetTypeService` с методами `FindAllWidgetTypes`, `FindWidgetTypeByID`, `CreateWidgetType`, `UpdateWidgetType`, `DeleteWidgetTypeByID`; публикует события через `EventBus` после каждой успешной мутации
- `internal/application/appdto/widget_type.go` — DTO `WidgetType`, `CreateWidgetTypeInput`, `UpdateWidgetTypeInput`; фабрики `NewWidgetType`, `NewWidgetTypeList`
- `internal/infrastructure/postgres/repositories/widget_type_repository_postgres.go` — PostgreSQL-реализация `WidgetTypeRepository`; INSERT при `version == Initial`, UPDATE с `WHERE id = $X AND version = $Y` при `IsCommitted()`; при `rowsAffected == 0` метод `classifyUpdateConflict` делает `SELECT EXISTS` и возвращает `ErrWidgetTypeNotFound` или `ErrWidgetTypeConflict`
- `internal/infrastructure/postgres/migrations/20260515000000_create_widget_types.sql` — DDL таблицы `widget_types`
- `internal/infrastructure/postgres/test_data/20260515000001_insert_test_data.sql` — тестовые данные для `widget_types`
- `internal/interface/restapi/widget_types.go` — REST-хэндлеры CRUD:
  - `GET /api/v1/widget-types` — список всех типов виджетов
  - `GET /api/v1/widget-types/{id}` — тип виджета по ID; 404 при отсутствии
  - `POST /api/v1/widget-types` — создание; 201 с ID
  - `PUT /api/v1/widget-types/{id}` — обновление; 404 при отсутствии, 409 при конфликте версий
  - `DELETE /api/v1/widget-types/{id}` — удаление; 200 с телом удалённого объекта, 404 при отсутствии
- `internal/interface/restapi/restdto/widget_type.go` — HTTP-DTO `WidgetTypeResponse`, `GetWidgetTypesResponse`, `CreateWidgetTypeRequest/Response`, `UpdateWidgetTypeRequest/Response`
- `internal/interface/ui/library/library.go` — UI-компонент `Library` для управления типами виджетов: список, создание, редактирование, удаление через REST API
- Bruno-коллекция: запросы `GET /widget-types`, `GET /widget-types/{{ID}}`, `POST /widget-types`, `PUT /widget-types/{{ID}}`, `DELETE /widget-types/{{ID}}`
- Юнит-тесты домена `widget` — `widget_type_test.go`, `widget_type_id_test.go`, `widget_type_name_test.go`, `widget_type_script_language_test.go`
- Юнит-тесты `internal/domain/version/version_test.go` — `New` с валидными значениями, отрицательное → ошибка, константы, `Next`, `IsCommitted`, `String`
- Интеграционные тесты `internal/application/widget_type_application_service_test.go` — CRUD, not-found, конфликт версий, публикация событий
- Интеграционные тесты `internal/interface/restapi/widget_types_test.go` — все CRUD-хэндлеры включая 409 Conflict

### Changed

- `internal/domain/version` — версия тега перенесена из `int` в `version.Version`; `Tag.Version()` теперь возвращает `version.Version`; `tag.TagVersionInitial`/`TagVersionCommitted` удалены
- `Tag.IncrementVersion()` — удалён из интерфейса и реализации; инкремент версии выполняется в репозитории через `version.Next()`
- `TagRepository.Save` — сигнатура изменена с `error` на `(Tag, error)`; репозиторий возвращает сохранённый агрегат с обновлённой версией
- `TagID`, `TagName`, `TagQuality`, `TagType` — переведены с type alias / primitive (`type Foo int`, `type Foo string`) на `struct { field T }`; исключает нечаянные приведения типов
- `TagQuality.IsValid()`, `TagType.IsValid()` — удалены; валидность гарантируется конструктором (`NewTagQuality`, `NewTagType` возвращают ошибку для неизвестных строк)
- `restapi/tags.go`, `restapi/widget_types.go` — 500-ошибки переведены на `sendInternalError(w, r, err)`: внутренние детали (SQL, стек) логируются, но не попадают в тело HTTP-ответа
- Тесты `tag_quality_test.go`, `tag_type_test.go` — граничный кейс `default` в `String()` восстановлен через `TagQuality{quality: 99}` / `TagType{tagType: 99}` вместо ранее использовавшихся raw-приведений типов

## [0.0.10] - 2026-05-12

### Added

- `internal/application/app_dto` — новый пакет с DTO уровня прикладного сервиса: `Tag`, `CreateTagInput`, `UpdateTagInput`; поля не имеют JSON-тегов, сериализация делегирована интерфейсному слою; фабричные функции `NewTag` и `NewTagList` конвертируют доменные агрегаты в DTO
- `internal/interface/restapi/rest_dto` — новый пакет с HTTP-DTO: `TagResponse`, `GetTagsResponse`, `CreateTagRequest`, `CreateTagResponse`, `UpdateTagRequest`, `UpdateTagResponse`; конвертеры `NewTagResponse`, `NewGetTagsResponse`, `NewCreateTagResponse`, `NewUpdateTagResponse`, `NewCreateTagInput`, `NewUpdateTagInput` изолируют маппинг между слоями

### Removed

- `internal/application/dto` — удалён единый DTO-пакет, смешивавший JSON-сериализацию и application-слой; его ответственность разделена между `app_dto` и `rest_dto`

### Changed

- `TagService` — все методы переведены на `app_dto`: `FindAllTags` возвращает `[]app_dto.Tag` вместо `dto.FindAllTagsResponse`; `FindTagByID` возвращает `app_dto.Tag` вместо `dto.Tag`; `CreateTag` принимает `app_dto.CreateTagInput` вместо `dto.CreateTagRequest` и возвращает полный `app_dto.Tag` вместо `dto.CreateTagResponse{ID}`; `DeleteTagByID` возвращает плоский `app_dto.Tag` вместо `dto.DeleteTagResponse{Tag: dto.Tag}`; `SetTagValueByID` принимает `app_dto.UpdateTagInput` вместо `dto.UpdateTagRequest` и возвращает `app_dto.Tag` вместо `dto.UpdateTagResponse{Version}`
- `restapi/tags.go` — хендлеры переведены на `rest_dto`: каждый хендлер конвертирует результат сервиса через соответствующую фабрику `rest_dto.New*`; удалены промежуточные переменные для ответов с ошибками; `UpdateTagRequest` больше не содержит поле `ID` — идентификатор передаётся только через path-параметр и подставляется в `NewUpdateTagInput`
- Тесты `tag_application_service_test.go` и `tags_test.go` — обновлены импорты и типы: `dto.CreateTagRequest` → `app_dto.CreateTagInput`, `dto.UpdateTagRequest` → `app_dto.UpdateTagInput`; тип возврата `createTagViaService` изменён с `dto.CreateTagResponse` на `app_dto.Tag`; декодирование HTTP-ответов использует `rest_dto.*` вместо `dto.*`
- Все doc-комментарии в `app_dto` и `rest_dto` переведены на английский язык

## [0.0.9] - 2026-05-08

### Added

- `EventBus` — реализация интерфейса `event.EventBus` в `internal/domain/event/event_bus.go`: хранилище подписчиков `map[EventType][]EventHandler`, потокобезопасность через `sync.RWMutex`; при `Publish` список обработчиков копируется под `RLock` и вызывается без блокировки, что исключает дедлок при вызове `Subscribe` из обработчика
- `MQTTEventBus` — декоратор над `event.EventBus` в `internal/infrastructure/mqtt/event_bus_mqtt.go`; при `Publish` вызывает внутреннюю шину и публикует событие в MQTT-брокер с топиком `events/<event_type>`; `MQTTClient` — порт-интерфейс с методом `Publish(topic string, payload []byte) error`, позволяющий подключить любую MQTT-библиотеку без изменения кода декоратора
- `NewTagService` — добавлен параметр `anEventBus event.EventBus`; `tagServiceImpl` хранит ссылку на шину и публикует события после успешных мутирующих операций: `TagCreatedEvent` в `CreateTag`, `TagDeletedEvent` в `DeleteTagByID`, `TagUpdatedEvent` в `SetTagValueByID`
- Юнит-тесты пакета `event` (`internal/domain/event/event_test.go`) — 33 теста:
  - `EventTimestamp`: создание без опций (метка близка к `now`), с фиксированным временем (`EventTimestampWithTime`), `String`, `ParseEventTimestamp` (валидная строка RFC3339 / невалидная → ошибка), `MustParseEventTimestamp` (валидная / паника)
  - `EventType`: `NewEventType` для всех 4 строк, для неизвестной строки → ошибка, round-trip `String` → `NewEventType` для каждого значения
  - `EventBus`: вызов подписчика при совпадении типа, изоляция по типу (другие подписчики не вызываются), несколько подписчиков для одного типа вызываются все, `Publish` без подписчиков не паникует
  - `SystemReadyEvent`, `TagCreatedEvent`, `TagUpdatedEvent`, `TagDeletedEvent`: тип, `TagID`, метка времени (не нулевая), `String` (не пустая), опция `WithTimestamp`
- Интеграционные тесты прикладного сервиса — 6 новых тестов на публикацию событий через `bus.Subscribe`:
  - `TestCreateTag_Success_PublishesTagCreatedEvent` — проверка типа события и `TagID`
  - `TestCreateTag_InvalidRequest_NoEventPublished` — при ошибке до `Save` событие не публикуется
  - `TestDeleteTagByID_Success_PublishesTagDeletedEvent` — проверка типа события и `TagID`
  - `TestDeleteTagByID_NotFound_NoEventPublished`
  - `TestSetTagValueByID_Success_PublishesTagUpdatedEvent` — проверка типа события и `TagID`
  - `TestSetTagValueByID_InvalidRequest_NoEventPublished`

### Fixed

- `SetTagValueByID` публиковал `TagDeletedEvent` вместо `TagUpdatedEvent` — исправлено

### Changed

- `NewTagService` — сигнатура расширена вторым параметром `anEventBus event.EventBus`; тесты в `tag_application_service_test.go` и `restapi/tags_test.go` обновлены: теперь передают `event.NewEventBus()`; вспомогательная функция `newServiceWithBus()` заменила `newServiceWithSpy()` — шина берётся из пакета `event`, подписка через `bus.Subscribe`
- `TagType` — добавлено нулевое значение `TagTypeUnknown TagType = iota` первым в блоке констант; `String()` получил явный `case TagTypeUnknown`; `NewTagType` возвращает `TagTypeUnknown` вместо `-1` при неизвестной строке; `IsValid()` исключает `TagTypeUnknown`
- `TagQuality` — добавлено нулевое значение `TagQualityUnknown TagQuality = iota` первым в блоке констант; аналогичные правки в `String()`, `NewTagQuality` и `IsValid()`
- `EventType` — добавлено нулевое значение `EventTypeUnknown EventType = iota` первым в блоке констант; `String()` получил явный `case EventTypeUnknown`; `NewEventType` возвращает `EventTypeUnknown` вместо `0` при неизвестной строке
- `Mode` — добавлена константа `ModeUnknown Mode = ""` первой в блоке констант (документирует нулевое значение типа `string`)
- Тесты `tag_type_test.go` — `TagType(-1)` в String-кейсе заменён на `TagTypeUnknown`; out-of-range значение `TagType(99)` покрывает `default`; `TagTypeUnknown` добавлен в список невалидных в `IsValid`-тесте
- Тесты `tag_quality_test.go` — аналогичные правки: `TagQuality(-1)` → `TagQualityUnknown`, добавлен `TagQuality(99)` для `default`, `TagQualityUnknown` в список невалидных
- Тесты `event_test.go` — `TestNewEventType_UnknownString_ReturnsError` расширен проверкой возвращаемого `EventTypeUnknown`; добавлен тест `TestEventType_String_Unknown`

## [0.0.8] - 2026-05-03

### Changed

- `TagKind` переименован в `TagType` во всём проекте: тип, константы (`TagTypeString`, `TagTypeBoolean`, `TagTypeInteger`), конструктор `NewTagType`
- `tag_kind.go` → `tag_type.go`, `tag_kind_test.go` → `tag_type_test.go`
- Метод `Tag.Kind() TagKind` переименован в `Tag.Type() TagType` в доменном интерфейсе и реализации
- Колонка `kind` переименована в `type` в схеме БД: обновлены DDL, индексы (`idx_tags_type`, `idx_tags_name_type`) и constraint (`chk_tags_type`)
- Все SQL-запросы в репозитории обновлены: `kind` → `type` в INSERT, UPDATE, SELECT, RETURNING
- DTO: поле `Kind string json:"kind"` переименовано в `Type string json:"type"` в `Tag` и `CreateTagRequest`
- Тесты: имена функций (`InvalidKind` → `InvalidType`, `ByKind` → `ByType`, `ForKind` → `ForType`, `UnknownKind` → `UnknownType`), переменные (`kind` → `tagType`, `newKind` → `newType`) и строки в логах (`"Kind:"` → `"Type:"`) обновлены во всех пакетах

## [0.0.7] - 2026-05-03

### Changed

- `cmd/combined_server/main.go`: все вызовы `fmt.Printf`/`fmt.Println` заменены на `slog.Error`/`slog.Info` со структурированными key-value аргументами
- `slogAdapter` — добавлен адаптер, реализующий интерфейс `gracedown.Logger` (`Println(v ...any)`); внутренние сообщения `gracedown` теперь выводятся через `slog.Info`
- `gracedown.NewManager` — передаётся `WithLogger(slogAdapter{})`, `gracedown` больше не использует `fmt.Println` напрямую

## [0.0.6] - 2026-05-03

### Added

- `DeleteTagByID` — метод удаления тега по ID в `TagService`; возвращает удалённый тег в `DeleteTagResponse`
- `SetTagValueByID` — метод обновления значения и качества тега в `TagService`; возвращает новую версию в `UpdateTagResponse`
- `DeleteByID` — метод удаления тега в `TagRepository`; возвращает `(Tag, error)`; реализация использует `DELETE ... RETURNING id, name, kind, value, quality, version`, что исключает лишний `SELECT`
- REST-хэндлер `DeleteTagByID` (`DELETE /api/v1/tags/{id}`) — удаление тега; 200 с телом удалённого тега, 404 при отсутствии, 400 при невалидном UUID
- REST-хэндлер `PatchTagValue` (`PATCH /api/v1/tags/{id}/value`) — обновление значения и качества; 200 с новой версией, 404 при отсутствии, 400 при невалидном UUID или теле
- Маршруты `DELETE /api/v1/tags/{id}` и `PATCH /api/v1/tags/{id}/value` зарегистрированы в роутере
- Bruno-коллекция: запросы `DELETE /api/v1/tags/{{TAG_ID}}` и `PATCH /api/v1/tags/{{TAG_ID}}/value`
- Интеграционные тесты репозитория — 4 теста для `DeleteByID`:
  - `TestDeleteByID_ExistingTag_ReturnsDeletedTag` — проверка всех полей возвращённого тега
  - `TestDeleteByID_ExistingTag_TagIsRemovedFromDB` — тег отсутствует в БД после удаления
  - `TestDeleteByID_ExistingTag_OtherTagsAreUnaffected` — прочие теги не затрагиваются
  - `TestDeleteByID_NotFound_ReturnsErrTagNotFound` — возвращает `ErrTagNotFound`
- Интеграционные тесты прикладного сервиса — 8 тестов:
  - `TestDeleteTag_ExistingTag_ReturnsDeletedTag`, `TestDeleteTag_ExistingTag_TagIsRemovedFromDB`, `TestDeleteTag_NotFound_ReturnsWrappedErrTagNotFound`
  - `TestSetTagValueByID_ValidUpdate_ReturnsIncrementedVersion`, `TestSetTagValueByID_ValidUpdate_ValueAndQualityAreUpdated`, `TestSetTagValueByID_NotFound_ReturnsWrappedErrTagNotFound`, `TestSetTagValueByID_InvalidQuality_ReturnsError`, `TestSetTagValueByID_InvalidValueForKind_ReturnsError`
- Интеграционные тесты HTTP-хэндлеров — 9 тестов:
  - `TestDeleteTagByID_ExistingTag_Returns200WithDeletedTag`, `TestDeleteTagByID_ExistingTag_TagIsRemovedFromDB`, `TestDeleteTagByID_NotFound_Returns404WithProblemDetails`, `TestDeleteTagByID_InvalidUUID_Returns400WithProblemDetails`
  - `TestPatchTagValue_ValidUpdate_Returns200WithVersion`, `TestPatchTagValue_ValidUpdate_ValueAndQualityAreUpdated`, `TestPatchTagValue_NotFound_Returns404WithProblemDetails`, `TestPatchTagValue_InvalidUUID_Returns400WithProblemDetails`, `TestPatchTagValue_InvalidJSON_Returns400WithProblemDetails`

### Changed

- `Tag.UpdateValue` переименован в `Tag.SetValue` в доменном интерфейсе и реализации; тест-функции переименованы соответственно (`TestTag_SetValue_*`)
- `TagService.DeleteTag` переименован в `TagService.DeleteTagByID`
- `TagRepository.Delete` переименован в `TagRepository.DeleteByID`; сигнатура изменена с `error` на `(Tag, error)` — сервис делает один вызов репозитория вместо двух (`FindByID` + `Delete`)
- `Save` в PostgreSQL-репозитории: добавлена явная обработка ошибки `RowsAffected()` для INSERT и UPDATE (для postgres всегда `nil`)
- `.github/workflows/go.yaml`: путь сборки исправлен с `./cmd/combined/` на `./cmd/combined_server/` — директория была переименована

## [0.0.5] - 2026-05-01

### Added

- Интеграционные тесты прикладного сервиса `tag` (`internal/application/tag_application_service_test.go`) — 10 тестов:
  - `TestCreateTag_ValidIntegerTag_ReturnsID` — создание тега типа `integer`, проверка ненулевого UUID в ответе
  - `TestCreateTag_ValidStringTag_ReturnsID` — создание тега типа `string`
  - `TestCreateTag_ValidBooleanTag_ReturnsID` — создание тега типа `boolean`
  - `TestCreateTag_InvalidKind_ReturnsError` — невалидный kind возвращает ошибку
  - `TestCreateTag_InvalidQuality_ReturnsError` — невалидное quality возвращает ошибку
  - `TestCreateTag_InvalidValueForKind_ReturnsError` — значение несовместимо с типом (строка вместо числа) возвращает ошибку
  - `TestFindAllTags_EmptyDB_ReturnsEmptyList` — пустая БД возвращает пустой список
  - `TestFindAllTags_MultipleTags_ReturnsAll` — возвращает все сохранённые теги
  - `TestFindTagByID_ExistingTag_ReturnsTag` — возвращает тег с корректными полями в DTO
  - `TestFindTagByID_NotFound_ReturnsWrappedErrTagNotFound` — возвращает ошибку с обёрнутым `ErrTagNotFound`
- Интеграционные тесты HTTP-хэндлеров `tag` (`internal/interface/restapi/tags_test.go`) — 8 тестов; запросы проходят через `router.ServeMux().ServeHTTP()`, что корректно заполняет path-параметры:
  - `TestGetTags_EmptyDB_Returns200WithEmptyList` — пустая БД, статус 200, пустой массив тегов
  - `TestGetTags_WithTags_Returns200WithAll` — теги в БД, статус 200, все теги в ответе
  - `TestGetTagsByID_ExistingTag_Returns200WithTag` — существующий тег, статус 200, корректные поля
  - `TestGetTagsByID_NotFound_Returns404WithProblemDetails` — несуществующий UUID, статус 404, тело в формате RFC 9457
  - `TestGetTagsByID_InvalidUUID_Returns400WithProblemDetails` — невалидная строка в `{id}`, статус 400, тело RFC 9457
  - `TestPostTags_ValidBody_Returns201WithID` — корректный JSON, статус 201, ненулевой UUID в ответе
  - `TestPostTags_InvalidJSON_Returns400WithProblemDetails` — невалидный JSON, статус 400, тело RFC 9457
  - `TestPostTags_InvalidKind_Returns500WithProblemDetails` — невалидный kind, статус 500, тело RFC 9457
- Юнит-тесты `ProblemDetails` (`internal/interface/restapi/problems_test.go`) — 11 тестов:
  - `TestProblemDetails_MarshalJSON_*` — 4 теста: стандартные поля попадают в JSON, пустой `detail` опускается, extensions сериализуются, extension не перебивает стандартное поле
  - `TestProblemDetails_UnmarshalJSON_*` — 2 теста: стандартные поля парсятся, неизвестные поля попадают в `Extensions`
  - Фабрики: `TestNewBadRequest_*`, `TestNewNotFound_*`, `TestNewConflict_*`, `TestNewValidationError_*`, `TestNewInternalError_*` — проверка статуса, типа, полей и extensions

## [0.0.4] - 2026-04-30

### Added

- Интеграционные тесты для инфраструктурного репозитория `tag` (`internal/infrastructure/postgres/repositories/tag_repository_postgres_test.go`) — 8 тестов:
  - `TestNextID_ReturnsUniqueIDs` — генерирует уникальные идентификаторы
  - `TestSave_NewTag_InsertsSuccessfully` — INSERT новой записи без ошибок
  - `TestSave_DuplicateID_ReturnsError` — повторный INSERT с тем же ID возвращает ошибку
  - `TestSave_ExistingTag_UpdatesSuccessfully` — UPDATE существующей записи, проверка новых значений
  - `TestSave_StaleVersion_ReturnsError` — оптимистичная блокировка: UPDATE с несовпадающей версией возвращает ошибку
  - `TestFindByID_ExistingTag_ReturnsTag` — возвращает тег с корректными полями
  - `TestFindByID_NotFound_ReturnsErrTagNotFound` — возвращает `ErrTagNotFound` при отсутствии записи
  - `TestFindAll_EmptyDB_ReturnsEmptySlice` — возвращает пустой срез при пустой таблице
  - `TestFindAll_MultipleTags_ReturnsAll` — возвращает все сохранённые теги
- Зависимость `github.com/pressly/goose/v3 v3.27.1` — применение SQL-миграций в тестовом окружении
- Тесты используют `testcontainers-go/modules/postgres`: контейнер поднимается в `TestMain`, миграции из `internal/infrastructure/postgres/migrations/` применяются через goose

## [0.0.3] - 2026-04-30

### Added

- Юнит-тесты для доменного агрегата `tag` (`internal/domain/tag/`) — 9 файлов, 63 теста:
  - `tag_id_test.go` — генерация UUID, парсинг, паника для невалидных строк, roundtrip
  - `tag_name_test.go` — создание имени, `String()`
  - `tag_kind_test.go` — парсинг строк, `String()`, `IsValid()`
  - `tag_quality_test.go` — парсинг строк, `String()`, `IsValid()`
  - `tag_value_boolean_test.go` — конструктор из `string`/`bool`/`int`, ошибки, `Value()`, `String()`
  - `tag_value_integer_test.go` — конструктор из `string`/`bool`/`int`, ошибки парсинга, `Value()`, `String()`
  - `tag_value_string_test.go` — конструктор из `string`/`bool`/`int`, конвертации, `Value()`, `String()`
  - `tag_value_test.go` — фабрика `NewTagValue`: диспетчеризация по `TagKind`, неизвестный kind, несовместимый тип
  - `tag_test.go` — создание агрегата, все геттеры, `IncrementVersion`, `UpdateValue`
- `.github/workflows/go.yaml` — GitHub Actions CI: запуск тестов и сборка при push/PR в ветку `path`

### Changed

- `.gitverse/workflows/go.yaml` — исправлены три проблемы: триггер `workflow_dispatch` заменён на `push`/`pull_request` для ветки `path`; версия Go зафиксирована через `go-version-file: go.mod` вместо явного `1.23`; путь сборки `cmd/server/main.go` (несуществующий) исправлен на `./cmd/combined/`

## [0.0.2] - 2026-04-28

### Added

- `FindTagByID` — реализован метод получения тега по UUID в сервисном слое (`TagService`)
- `FindByID` — реализован метод в репозитории PostgreSQL: SQL-запрос по `id`, маппинг строки в доменные value objects, возврат `ErrTagNotFound` при отсутствии записи
- `ErrTagNotFound` — добавлена сентинель-ошибка в доменный пакет `tag`
- Bruno-коллекция: добавлен запрос `GET /api/v1/tags/{{TAG_ID}}`
- `emergencyExit` — вспомогательная функция для аварийного завершения с корректным shutdown всех компонентов

### Changed

- `TagService.FindTagByID` — тип параметра изменён с `int` на `tag.TagID`
- `gracedown` обновлён до версии `v0.0.0-20260423215449-38425123f880`; переход с `WaitForSignal` на `WaitForSignalAndShutdown`, коды выхода вынесены в пакет `gracedown`
- Порядок инициализации в `main.go`: инфраструктурные компоненты (БД) поднимаются раньше интерфейсных (HTTP-серверы); регистрация shutdown-хука для БД перенесена до `Ping()`, чтобы соединение закрывалось при аварийном завершении
- `sendJSONResponse` — сериализация тела теперь выполняется до отправки заголовков, что позволяет вернуть `500` при ошибке маршалинга
- `GetTags`, `GetTagsByID` — упрощены: ручной `json.Marshal` + `w.Write` заменены на `sendJSONResponse`
- `PostTags` — исправлен отсутствующий `return` после ошибки `CreateTag`; без него параллельно уходили ответы `500` и `201`
- `GetTagsByID` зарегистрирован в роутере: `GET /api/v1/tags/{id}`

## [0.0.1] - 2026-04-22

- Basic tag functionality
