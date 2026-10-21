# Рефакторинг живой перезагрузки UI
Status: ready-for-agent

Появилось в ревью задачи 14: живая перезагрузка UI по SSE (`internal/server/interface/ui/project/live_reload.go`, `internal/server/interface/ui/library/library.go`) работает, но код дублируется. Поведение не меняется, это чистый рефакторинг.

- ~~Общий загрузчик списков.~~ Сделано в задаче 23: `uiutil.Load` и `uiutil.LoadFor` рядом с `uiutil.Reloader`; через них грузят списки Library, Project и Operation, `uiutil.FetchJSON` удалён. В задаче 24 через `uiutil.Load` перечитываются и отдельные Scene (`Project.loadStaleScenes`).
- ~~Одна таблица «тип события → что перезагружать».~~ Сделано в ревью задачи 16: `eventlog.ReloadTargetsFor` в `eventlog/reload.go` — общая для Library, Project и Operation; тест сверяет её с `eventTypeInfoByType`. Строковых литералов типов событий в UI нет с задачи 24: константы `apiv0.Event…`.
- Разделить `project/live_reload.go` (~380 строк) по причинам для изменений; сам файл удаляется:
  - `project/server_events.go` — `handleServerEvents` (маршрутизация событий). Отдельной чистой функции «что перечитать в Project» не заводить: хватает `ReloadTargets.WidgetsOf`, ветвление проверяют интеграционные тесты. `live_reload_ops_test.go` → `server_events_test.go`.
  - `project/load.go` — загрузчики списков: `loadWidgetTypes`, `loadScenes`/`setScenes`, `loadWidgets`/`setWidgets`, `loadTags`/`setTags`, `isInteracting`.
  - `project/scene_versions.go` — версии и состояния Scene: `sceneMark`, `markScenes`, `observeSceneVersion`, `forgetSceneVersion`, `loadScene`, `loadStaleScenes`, `setScene`.
  - `project/edit_fields.go` — слияние правок редакторов: `widgetEditFields` и новая парная `sceneEditFields` (`*Of`, чистые `merge*`, обёртки `sync*`/`set*`); `syncSceneEditingFields` переезжает сюда из `scene_ops.go`.
- Library — по той же схеме, что Project:
  - `library/server_events.go` — обработчик событий, вынесенный из `OnMount`.
  - `library/load.go` — `loadList`, `setWidgetTypes`.
  - `library/edit_fields.go` — `editFields{name, html, script, scriptLang, ports}`, чистые `editFieldsOf` и `mergeEditFields`; `selectItem` и `mergeSelected` — обёртки над ними. Порты по-прежнему сливаются целым списком.
- Тесты чистых функций слияния: `project/edit_fields_test.go` (виджет — перенос из `live_reload_test.go`, плюс Scene) и `library/edit_fields_test.go`. Маршрутизация уже покрыта `eventlog/reload_test.go`.
- Проверить в браузере (chrome-devtools, после `make full_restart` + `make run`):
  1. Теги через `growctl apply` и WidgetType через API появляются в Library и Project без перезагрузки страницы.
  2. Изменения из второй вкладки (переименование Scene, сдвиг Widget) доходят до первой.
  3. Несохранённая правка (имя в Library, posX в Project) остаётся, а поле, изменённое в другом месте, обновляется.
  4. С `make run_simulator_dashboard` Project не перечитывает `GET /api/v0/tags` на каждое `tag_updated`.

## Comments

- 2026-10-05: запись через сервер и замена UI-копий DTO на типы `contract/api/v0` — задача 23; SceneID в событиях Widget и константы типов событий — задача 24.
- 2026-10-07 (проверка UI задачи 20): Project перечитывает весь список тегов (`GET /api/v0/tags`) на каждое событие `tag_*`, включая `tag_updated` (`eventlog.ReloadTargetsFor`, `eventlog/reload.go:27` → `Project.loadTags`). С запущенным симулятором (`make run_simulator_dashboard`) — больше 200 запросов за минуту на открытой вкладке Project. `uiutil.Reloader` склеивает их (одна загрузка в полёте и одна отложенная), поэтому частота ограничена задержкой ответа, а не числом событий. Список тегов в Project показывает имя и тип, а панель свойств выбранного тега — ещё Value и Quality, поэтому `tag_updated` игнорировать нельзя. Предложение: на `tag_updated` применять состояние тега из события (оно его несёт), как в Operation, а весь список перечитывать только на `tag_created`/`tag_deleted`. Поведение меняется — учесть при рефакторинге таблицы «тип события → что перезагружать».
- 2026-10-13 (grilling задачи 23): задача 23 переводит все загрузки списков на `apiclient` и типы `apiv0`, удаляет `uiutil.FetchJSON` и вводит `uiutil.Call(ctx, call, onOK, onErr)` (Async → вызов → Dispatch через `compoCtx` → toast `FromError`). Общий загрузчик списков строить поверх `uiutil.Call` и `uiutil.Reloader`. SSE в eventlog разбирается в `apiv0.Event`, `ServerEvent.Tag` — `*apiv0.TagResponse`.
- 2026-10-15 (grilling задачи 24): применение `tag_updated` из события в Project (предложение от 2026-10-07) перенесено в задачу 24, вместе с общим пакетом состояния тегов для Project и Operation. Там же: `ReloadTargetsFor` принимает `apiv0.Event` (а не строку типа) и получает поле `Scene uuid.UUID`; `eventlog.ServerEvent` удаляется — режимы получают `apiv0.Event`, разбор SSE переезжает в `apiclient.ParseEvent`; строковые литералы типов событий заменяются константами `apiv0`.
- 2026-10-16 (ревью задачи 24): `live_reload.go` вырос — в нём теперь и версии Scene (`observeSceneVersion`, `sceneStates`, `markScenes`) и перечитывание устаревших Scene (`loadScene`/`loadStaleScenes`); при разделении файла это отдельная причина для изменений. Правило «перечитать виджеты выбранной Scene» и применение тегов из события вынесены в `eventlog` (`ReloadTargets.WidgetsOf`, `ApplyToTags`) — общие для Project и Operation.
- 2026-10-21 (grilling): объём сужен до оставшегося — разделение `project/live_reload.go`, та же схема для Library (`server_events.go`, `load.go`, `edit_fields.go`), чистые функции слияния правок с тестами, проверка в браузере вместе с симулятором. Поведение не меняется; новых терминов и ADR нет.
