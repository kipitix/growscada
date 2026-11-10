# 404 выбранной Library не сообщает Root: вкладка остаётся на пустом редакторе

Status: ready-for-agent
Priority: 1

Появилось при ревью задачи 73 (находка Bugs 4). У Library нет канала,
которым компонент сообщает Root, что выбранная сущность исчезла:
`ui/library/load.go` `libraryGone` сбрасывает выбор внутри компонента
(`selectLibrary(ctx, uuid.Nil)`), но `Root.showList` остаётся `false`, и
вкладка может остаться на пустом редакторе вместо страницы выбора.

У Project для этого есть `uiutil.ActionProjectGone` (его шлют `LoadOfProject`
на 404 списка контента и `player` на 404 `GET …/operation`); `Root.OnMount`
ловит его и переводит режим на страницу. У Library аналога нет.

## Решения грилинга (2026-11-10)

1. **Канал — новый `uiutil.ActionLibraryGone`** по образцу `ActionProjectGone`:
   действие с ID библиотеки; `Root` обрабатывает его рядом с проектным и делает
   `selectEntity(ModeLibrary, uuid.Nil, "")` — это уже включает
   `showList[ModeLibrary] = true`, `SetLibrary(nil)` и `Update`. Общий
   `ActionEntityGone` с режимом в значении отвергнут: он трогает работающий путь
   Project'а и заставляет свитчер Operation/History игнорировать библиотеку.
   Колбэк `OnGone` отвергнут — расходится с тем, как сделан Project.
2. **404 любого чтения выбранной Library значит «исчезла»**: три текущих сайта
   `LoadForGone` (`loadList`, `loadDraftState`, `loadReleases`) плюс
   `loadCommits` и `loadDraftChanges`, которые сейчас показывают тост. Это
   чистые чтения, и 404 у них бывает только когда Library не жива. Команды
   (`PATCH …/commits/{n}`, `…/releases/{n}`, create/delete WidgetType) свои 404
   сохраняют: там это `commit_not_found`/`release_not_found`/
   `widget_type_not_found`, и режим на список из-за них не уходит.
3. **При переходе страница перечитывает список.** `selectEntity` сам список не
   перечитывает, поэтому в Root появляется общий хелпер `entityGone(mode, id)`:
   проверка «это всё ещё выбранная сущность» (как у проектного обработчика),
   `selectEntity(mode, uuid.Nil, "")`, `picker.Reload`, `Update`. Им пользуются
   оба обработчика — Project получает ту же правку, и удалённая строка не висит
   в списке.
4. **Стухший ключ `library:selectedLibrary` закрывается тем же механизмом:**
   редактор монтируется, первое чтение получает 404, действие переводит Root на
   страницу. Отдельного кода не нужно; ключ чистит `selectLibrary(nil)` через
   `uiutil.StoreID`.

## Что меняется

- `internal/server/interface/ui/uiutil/call.go`: константа
  `ActionLibraryGone = "uiutil.library-gone"` с комментарием (значение — ID
  библиотеки), рядом с `ActionProjectGone`.
- `internal/server/interface/ui/library/load.go`: `libraryGone` шлёт действие
  вместо локального сброса; `loadCommits` и `loadDraftChanges` переводятся на
  `LoadForGone` (у `loadDraftChanges` сохраняется `mark`, взятый до запроса, —
  защита `applyDraftChanges` от отставшего ответа не меняется).
- `internal/server/interface/ui/root/root.go`: обработчик `ActionLibraryGone`
  рядом с проектным и общий хелпер `entityGone(mode, id)`; комментарий
  `renderBackBar`/`OnMount` о том, что исчезнувшая сущность возвращает на
  страницу.
- Тесты: `ui/root/selection_test.go` — удалённая через API библиотека при
  следующем чтении оставляет режим на странице (и список её не содержит);
  проектный путь и «← Список · <name>» работают как раньше.
  `ui/library` — 404 любого из пяти чтений уходит в `libraryGone`, а не в тост.
- REST и контракт не меняются; Bruno не трогаем.

## Приёмка

1. Удаление выбранной Library в другой вкладке (событие потеряно) или стухший
   `library:selectedLibrary` при открытии вкладки оставляют пользователя на
   странице выбора, а не на пустом редакторе; удалённой строки в списке нет.
2. `← Список · <имя>` и выбор другой Library после этого работают.
3. Тест: 404 Draft'а, Commit'ов, состояния Draft'а или Releases выбранной
   Library переводит Root на список; 404 команды (`commit_not_found` и т.п.)
   оставляет редактор и показывает свою ошибку.
4. Project ведёт себя как раньше, но его страница тоже перечитывает список.
5. `make build`, `make test` зелёные; проверка в браузере (правило 9).

## Связанное

- Задача 73 — страницы выбора и слим-строка возврата.
- `uiutil.ActionProjectGone` — существующий аналог для Project.
- Задача 74, 77, 78 — соседние находки ревью 73.

## Comments

- 2026-11-10: заведено по итогам ревью задачи 73; требовала выбора между общим
  action'ом и колбэком, поэтому `needs-triage`.
- 2026-11-10: грилинг — `ActionLibraryGone` (Q76.1); 404 любого чтения, не
  команды (Q76.2); общий хелпер в Root с перечитыванием списка у обоих режимов
  (Q76.3). Аудит по DDD: расхождений нет — это UI-механика, домен, слои и роли
  не затрагиваются, контракт не меняется.
