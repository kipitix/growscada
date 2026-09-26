# Рефакторинг живой перезагрузки UI
Status: needs-triage

Появилось в ревью задачи 14: живая перезагрузка UI по SSE (`internal/interface/ui/project/live_reload.go`, `internal/interface/ui/library/library.go`) работает, но код дублируется. Поведение не меняется, это чистый рефакторинг.

- Общий загрузчик списков. `Project.loadWidgetTypes`, `loadScenes`, `loadWidgets`, `loadTags` и `Library.loadList` повторяют одну обвязку: `Reloader.Start()` → `uiutil.FetchJSON` → `ctx.Dispatch` → toast при ошибке → `Done()` и повторная загрузка. Вынести её в generic-хелпер рядом с `uiutil.Reloader`.
- Одна таблица «тип события → что перезагружать». Сейчас строковые литералы `"widget_type_created"` и т. п. перечислены в `reloadTargetsFor` (`live_reload.go`), в `isWidgetTypeEvent` (`library.go`) и в `eventTypeInfoByType` (`eventlog/event_types.go`). Нужен единый источник, например агрегат события рядом с `eventTypeInfoByType`. Категории журнала событий для этого напрямую не годятся: `tag_updated` там отнесён к `categoryValue`, а не к `categoryTag`.
- Разделить `live_reload.go` (~350 строк): маршрутизация событий, загрузчики четырёх списков и слияние правок редакторов (`mergeEditFields`, `mergeSceneEditingFields`) — разные причины для изменений.
- Покрыть тестами то, что выносится в чистые функции: маршрутизацию событий и слияние полей. Сейчас покрытие UI-пакетов около 1%.
- Проверить в браузере: изменения из `growctl` и из другой вкладки доходят до Library и Project без перезагрузки страницы, несохранённая правка в редакторе не затирается.
