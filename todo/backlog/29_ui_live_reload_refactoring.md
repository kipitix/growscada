# Рефакторинг живой перезагрузки UI
Status: needs-triage

Появилось в ревью задачи 14: живая перезагрузка UI по SSE (`internal/server/interface/ui/project/live_reload.go`, `internal/server/interface/ui/library/library.go`) работает, но код дублируется. Поведение не меняется, это чистый рефакторинг.

- Общий загрузчик списков. `Project.loadWidgetTypes`, `loadScenes`, `loadWidgets`, `loadTags` и `Library.loadList` повторяют одну обвязку: `Reloader.Start()` → `uiutil.FetchJSON` → `ctx.Dispatch` → toast при ошибке → `Done()` и повторная загрузка. Вынести её в generic-хелпер рядом с `uiutil.Reloader`.
- ~~Одна таблица «тип события → что перезагружать».~~ Сделано в ревью задачи 16: `eventlog.ReloadTargetsFor` в `eventlog/reload.go` — общая для Library, Project и Operation; тест сверяет её с `eventTypeInfoByType`. Строковые литералы типов событий пока остаются в двух местах (`reload.go` и `event_types.go`).
- Разделить `live_reload.go` (~350 строк): маршрутизация событий, загрузчики четырёх списков и слияние правок редакторов (`mergeEditFields`, `mergeSceneEditingFields`) — разные причины для изменений.
- Покрыть тестами то, что выносится в чистые функции: маршрутизацию событий и слияние полей. Сейчас покрытие UI-пакетов около 1%.
- Проверить в браузере: изменения из `growctl` и из другой вкладки доходят до Library и Project без перезагрузки страницы, несохранённая правка в редакторе не затирается.

## Comments

- 2026-10-05: запись через сервер и замена UI-копий DTO на типы `contract/api/v0` — задача 23; SceneID в событиях Widget и константы типов событий — задача 24.
- 2026-10-07 (проверка UI задачи 20): Project перечитывает весь список тегов (`GET /api/v0/tags`) на каждое событие `tag_*`, включая `tag_updated` (`eventlog.ReloadTargetsFor`, `eventlog/reload.go:27` → `Project.loadTags`). С запущенным симулятором (`make run_simulator_dashboard`) — больше 200 запросов за минуту на открытой вкладке Project. `uiutil.Reloader` склеивает их (одна загрузка в полёте и одна отложенная), поэтому частота ограничена задержкой ответа, а не числом событий. Список тегов в Project показывает имя и тип, а панель свойств выбранного тега — ещё Value и Quality, поэтому `tag_updated` игнорировать нельзя. Предложение: на `tag_updated` применять состояние тега из события (оно его несёт), как в Operation, а весь список перечитывать только на `tag_created`/`tag_deleted`. Поведение меняется — учесть при рефакторинге таблицы «тип события → что перезагружать».
