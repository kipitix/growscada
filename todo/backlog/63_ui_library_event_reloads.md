# Лишние перезагрузки UI на события библиотек
Status: needs-triage
Priority: 3

Появилось при ревью задачи 33 (находки №8 и №9). На события библиотек и их WidgetTypes режимы Library и Project перезагружают больше, чем им нужно.

## Что осталось после 72_7 и цепочки 85

- **Режим Library** (`ui/library/server_events.go`). Выбранная библиотека уже перезагружает историю только на `commit_*` (`targets.Commits && mine`) и Draft — только на `widget_type_*` своей библиотеки (`targets.WidgetTypes && mine`); `setWidgetTypes` больше не тянет содержимое сохранённых состояний. Но `targets.Libraries` выставляется на каждое `widget_type_*` (`eventlog.ReloadTargetsFor`), и режим перезагружает `GET /libraries` — весь список библиотек сервера — на каждое изменение WidgetType любой библиотеки, даже когда список в редакторе не показан.
- **Режим Project** (`ui/project/server_events.go`). На `widget_type_*` он делает и `loadLibraries` (`GET /libraries`), и `loadWidgetTypes` (`GET …/library-imports/widget-types`). Импортированные копии WidgetType меняются только при импорте/переключении (`library_import_*`), а имена и последние saved states библиотек — нет; обе загрузки лишние.
- **`GET /libraries`** (`LibraryService.describeAll`) считает последний saved state и `has_unsaved_changes` для каждой библиотеки: `FindLastCommit` + `Draft.Read` на библиотеку, то есть N+1 запрос. Прежней дорогой части (`DraftDiffers`, полное чтение последнего Release) больше нет, но список остаётся не бесплатным и перечитывается чаще, чем меняется.
- **Лейблы**: `label_*` даёт `{Labels: true, Commits: true}`; режим Project перечитывает историю на назначение лейбла любому saved state любого проекта и библиотеки. Чипы лейблов — да, но список saved states проекта не меняется.

## Требования

- Режимы перезагружают только то, что событие могло изменить: Library — `GET /libraries` только на `library_*`, Project — только на `library_*` и `library_import_*`; `widget_type_*` не трогает ни тот, ни другой список (Library читает свой Draft, Project — импорты).
- Лейбл: чипы и меню перечитываются всегда, список saved states — только для своего проекта/библиотеки (событие несёт scope/ID saved state).
- Если `has_unsaved_changes` в `GET /libraries` останется дорогим, решить: убрать его из списка или отдавать только для выбранной библиотеки. Он и сейчас считается доменной функцией Draft'а (`HasUnsavedChanges` по `clean_version`) — не заменять её SQL-копией.
- Тесты UI (`uitest`): Apply в редакторе не перезагружает список библиотек; событие `widget_type_*` другой библиотеки не трогает выбранную; `label_*` чужого проекта не перечитывает историю.

## Comments

- 2026-11-07: после 72_7 меняются источники: история — saved states, публикации — Library Release из saved state (тогда Release ещё существовал), признак «неопубликовано» — по номеру saved state/Release, а не `DraftDiffers`. Выполнять после 72_7.
- 2026-11-15: Release больше нет (задача 85_7); «релизы» в находках — это saved states и их история, а `DraftDiffers` тоже ушёл. Первая находка (история тянет полное содержимое) закрыта: `GetCommitsResponse` без содержимого, история читается только на `commit_*` своей библиотеки. Остались лишние `GET /libraries` на `widget_type_*` в обоих режимах, N+1 в `GET /libraries` и лишние перечитывания истории на лейблы — их и надо закрыть.
