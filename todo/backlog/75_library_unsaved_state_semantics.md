# Состояния Library на странице выбора недостижимы: `has_unsaved_changes` истинен для несохранённого Draft

Status: needs-triage
Priority: 3

Появилось при ревью задачи 73 (находка Spec 3). На странице выбора Library
(`ui/picker`) состояние строки выбирается в порядке «Unsaved changes» →
`unpublished commit` → `published` → `legacy` → `never saved`
(`ui/picker/sources.go`). Но `FindAllLibraries` считает
`has_unsaved_changes` истинным для Draft'а, у которого ещё нет строки
(`LibraryContent.Version` возвращает 1, `ReleaseStateOf` сравнивает с 0), и
состояние Library, которую ни разу не сохраняли, — всегда «Unsaved changes».

## Задача

Выровнять семантику `has_unsaved_changes` у Library с исправленной в задаче 73
семантикой Project: Draft без строки — это отсутствие правок (у Project
`draftState` возвращает нулевое состояние без строки Draft'а), а не «грязный»
Draft. После этого на странице выбора достижимы все объявленные состояния:
`unsaved changes`, `unpublished commit`, `published`, `legacy`, `never saved`.

## Что проверить

- `application/library_content.go` `Version` — возвращать 0/«нет Draft'а» на
  отсутствие строки либо считать состояние в одном месте, как
  `application/project_content.go` `draftState`.
- `domain/library.ReleaseStateOf` и `appdto.NewLibrary` — не терять
  `LatestRelease`, `LatestCommit` при новом расчёте.
- `ui/picker/sources.go` — порядок состояний и подписи; `ui/library` —
  индикатор в шапке (те же переходы, что у Project).
- Не сломать `LibraryService.CreateLibrary`, который сейчас отдаёт
  `ReleaseState{UnsavedChanges: true}` для только что созданной библиотеки:
  согласовать с новым правилом (созданная библиотека — «never saved», а не
  «изменена»).

## Приёмка

1. Только что созданная Library на странице выбора показывает `never saved`.
2. После первой правки WidgetType — `unsaved changes`; после Save —
   `unpublished commit`; после Release — `published`; у Library со старым
   Release без Commit — `legacy`.
3. У Project поведение не изменилось.
4. Тесты на серверный расчёт состояний и UI-тест бейджа; `make build`,
   `make test`, `make bruno_test` зелёные.

## Связанное

- Задача 72_7 — исходная семантика `has_unsaved_changes` у Library.
- Задача 73 — исправление Project и страница выбора.
- Задача 74 — общий механизм «чистой» версии (после него этот расчёт может
  переехать туда).

## Comments

- 2026-11-10: заведено по итогам ревью задачи 73. Меняет поведение, заданное
  задачей 72_7, поэтому `needs-triage`, а не `ready-for-agent`.
