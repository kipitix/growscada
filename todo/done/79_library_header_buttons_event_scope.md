# Кнопка History в режиме Library открывает панель Releases

Status: needs-triage

Найдено при ревью задачи 77 и подтверждено на базовом коммите (`d42d92d`),
то есть дефект пре-существующий, рефакторинг кнопок его не внёс.

## Что происходит

В редакторе Library кнопки шапки перепутаны: клик по **History** открывает
панель **Releases** (`data-releases-dialog`), клик по **Releases** — тоже
Releases. Панель истории Commit'ов (`data-commits-dialog`) так не открыть.
Симптом плавающий: он появляется и исчезает вместе со спаном индикатора
`Saved` / `Unsaved changes` рядом с кнопкой **Save**.

## Причина

`internal/server/interface/ui/library/commits.go` собирает шапку
(`renderCommitBar`, строки 174–193) как список детей, у которых число
элементов меняется: спан `data-commit-unsaved`/`data-commit-saved`
(`:177-189`) добавляется и убирается, а `History` и `Releases` идут после него
(`:190-193`). go-app сопоставляет детей позиционно, а обработчик считает
«тем же», если совпал указатель кода (`pkg/app/event.go`, `eventHandler.Equal`:
`reflect.ValueOf(h.goHandler).Pointer()`). Все кнопки шапки — тонкие обёртки
`barButton` (`render_libraries.go:260`), чей обработчик — один и тот же литерал
`func(app.Context, app.Event) { onClick() }`, поэтому при сдвиге детей
элемент переиспользуется со старым обработчиком.

Тот же дефект потенциально есть у соседних кнопок шапки Library, которые тоже
не передают `app.EventScope`: `Discard`/`Revert` (`render_libraries.go:115-118`)
и `Undo`/`Redo` (`draft_changes.go:128-129`). В режиме Project этого нет:
`deployButton` (`project/deploy.go:336-349`) ставит
`app.EventScope(label)`.

`renderReleasesDialog` уже знает про это свойство go-app и разводит строки
списка через `app.EventScope` (`render_libraries.go:139-153`); шапке этого
разведения не хватает.

## Что меняется

- `internal/server/interface/ui/library/commits.go`: `History` и `Releases`
  получают `app.EventScope` (например `"history"` и `"releases"`), как строки
  списков уже делают.
- `internal/server/interface/ui/library/render_libraries.go`:
  `Discard` и `Revert` — тоже со своим `EventScope`.
- `internal/server/interface/ui/library/draft_changes.go`: `Undo` и `Redo` —
  тоже.
- Если разведение по `EventScope` где-то не сработает (например два вызова
  одной обёртки с одинаковым значением), брать уникальный обработчик на
  кнопку; значение `EventScope` должно быть стабильным между рендерами, иначе
  go-app будет пересоздавать элемент.
- Проверить, нет ли того же в диалогах с условными детьми (шапка Save-диалога
  и панель Releases), и развести найденное там же.

## Приёмка

1. В редакторе Library клик по **History** открывает историю Commit'ов, по
   **Releases** — Releases, при обоих состояниях индикатора (`Saved`,
   `Unsaved changes`) и после их переключения на лету (Save, Discard, первая
   правка).
2. `Discard`, `Revert`, `Undo`, `Redo` делают каждый своё после появления и
   исчезновения индикатора.
3. Регрессия закреплена тестом, который насколько возможно ловит перепутанные
   обработчики (например, тест рендера проверяет, что у каждой кнопки шапки
   свой `EventScope`, либо UI-тест открывает панели в обоих состояниях
   индикатора); если в `TestEngine` это невыразимо — записать в задаче, чем
   именно проверено и почему тест невозможен.
4. `make build`, `make test` зелёные; браузерная проверка шапки Library
   (правило 9). REST и контракт не меняются — Bruno не трогаем.

## Связанное

- Задача 77 — ревью, на котором дефект найден (рефакторинг `uiutil.Button`).
- Задача 73 — страница выбора и слим-шапка редактора.
- Задача 80 — соседняя уборка кнопок (палитра вне `uiutil.Button`).

## Comments

- 2026-11-10: заведено из итогового отчёта `/kx_auto` по прогону задач 74–78.
  Воспроизведено в браузере на `path` до правок задачи 77 и после них.
- 2026-11-10: сделано. Кнопки шапки Library разведены по `app.EventScope`
  стабильными значениями: `history`, `releases` (`commits.go`), `discard`,
  `revert` (`render_libraries.go`), `undo`, `redo` (`draft_changes.go`).
  Дополнительно индикатор `Saved`/`Unsaved changes` стал постоянным вторым
  ребёнком шапки: когда сказать нечего, он скрыт `display:none`. Позиции
  History и Releases теперь не сдвигаются в принципе, а не только за счёт
  `EventScope`. Панель Releases и строки Commit'ов уже разводили обработчики
  по `EventScope` — проверено, там менять нечего.
- 2026-11-10: регрессия закреплена `TestCommitBar_IndicatorKeepsTheButtonPositions`
  (`ui/library/commits_test.go`): в обоих состояниях (unsaved, скрытый
  индикатор) шапка — четыре ребёнка, `Save`, индикатор, `History`, `Releases`
  на своих местах. `app.TestEngine` умеет только `Load`/`ConsumeAll` и не
  диспатчит DOM-клики, поэтому сами клики в тесте невыразимы — они проверены
  в браузере. Проверено в PWA в обоих состояниях индикатора (первая правка,
  Save, Discard, Revert): History открывает историю Commit'ов, Releases —
  Releases, Discard/Revert/Undo/Redo делают каждый своё, в том числе после
  переключения индикатора на лету.
- 2026-11-10: проверки — `make build`, `make test`, `make bruno_test`
  (коллекции прошли), ручной обход PWA (шапка Library в обоих состояниях,
  строки Commit'ов и Releases). REST, контракт и схемы не менялись.
