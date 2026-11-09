# Бейдж «Unsaved changes» не исчезает после Discard к последнему Commit

Status: needs-triage
Priority: 3

Появилось при ревью задачи 73 (находки Bugs 3 / Spec 2). Приёмочный п. 5
задачи 73 требует «после Save/Discard бейдж исчезает», а решение Q2 той же
задачи — считать `has_unsaved_changes` по `Version` Draft и
`last_commit_draft_version` последнего Commit. Два требования несовместимы:
`Discard` (`draft.Repository.Replace`) поднимает Version Draft, поэтому после
сброса к последнему Commit'у флаг остаётся истинным.

## Задача

Дать Draft'у понятие «чистой» версии, чтобы read-model «незакоммиченные
изменения» отвечал на вопрос «содержимое отличается от последнего Commit'а»,
а не «Version выросла», и применить его к Project и Library.

## Что затронуто

- **Сервер:** `has_unsaved_changes` в `ProjectResponse`/`LibraryResponse`
  (`appdto.NewProject`/`appdto.NewLibrary`, `application/project_content.go`
  `DraftVersion`, `application/library_content.go` `Version`,
  `domain/library.ReleaseStateOf`).
- **UI:** индикатор в шапке редактора Project и Library, бейдж строки на
  странице выбора (`ui/picker`, `ui/library/render_libraries.go`).
- **Команды Draft:** Save, `Discard` (к последнему Commit), `Revert`
  (к работающему Deployment / к последнему Release), Undo/Redo и любая запись
  содержимого.

## Варианты решения (выбрать один)

1. **Отметка чистой версии.** Новый столбец `engineering.drafts.clean_version`
   (nullable): `SaveCommit` ставит его в Version, которую зафиксировал Commit;
   `Discard` к последнему Commit — в Version после `Replace`; `Revert` — только
   когда целевой документ совпадает с содержимым последнего Commit'а, иначе
   чистой версии нет. Флаг = `Version != clean_version`. Дешёвое чтение, но
   нужно аккуратно с Revert'ом и библиотечным `RevertToLatestRelease`.
2. **Сравнение содержимого.** Считать флаг сравнением содержимого Draft'а с
   содержимым последнего Commit'а тем же кодом, что Save/ProjectFileBuilder
   (`draft.CommitContent`, canonical form). Точнее по смыслу, но дороже:
   список сущностей читает и собирает каждый Draft.
3. **Отдельный read-model на дисквалификацию.** Хранить `dirty` в Draft'е и
   сбрасывать его только на Save/Discard/эквивалентный Revert; каждая запись
   DraftChange поднимает `dirty`. Принципиально то же, что вариант 1, но без
   сравнения версий.

## Приёмка

1. Новый проект/библиотека без единой записи — бейджа нет; после записи
   содержимого — бейдж есть; после Save — исчез; после Discard к последнему
   Commit — исчез; после Revert к Deployment/Release последнего Commit'а — исчез.
2. После Discard к Commit'у и последующей новой правки бейдж появляется снова.
3. Флаг одинаково верен в `GET /projects`, `GET /libraries` и в обоих
   редакторах; одна и та же логика у Project и Library.
4. Тесты: unit на серверный расчёт (все переходы выше) и UI-тест бейджа.
5. `make build`, `make test`, `make bruno_test` зелёные.

## Связанное

- Задача 73 — ввод `has_unsaved_changes` у Project и страницы выбора.
- Задача 72_7 — индикатор и `has_unsaved_changes` у Library.
- ADR 0016 (Draft — один агрегат), ADR 0021 (Commit/Release), ADR 0022.

## Comments

- 2026-11-10: заведено по итогам ревью задачи 73. Решение требует выбора
  механизма, поэтому `needs-triage`.
