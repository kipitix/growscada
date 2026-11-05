# Undo/redo: сервис и REST

Status: ready-for-agent
Blocked by: 38_2
Parent: 38_draftchange_undo_redo
Part: 3/4

Часть 3 задачи 38. Источник истины — `todo/backlog/38_draftchange_undo_redo.md`, решение — ADR 0017. Лог DraftChange пишется с 38_1 (Project) и 38_2 (Library); здесь — применение undo/redo и его API.

**За рамками:** кнопки, горячие клавиши и подсказки UI (38_4).

## Домен

- `Draft.Undo` применяет `before` изменения на `stack_position`, `Draft.Redo` — `after` изменения на `stack_position+1`; оба поднимают `version`, пишут затронутые строки, публикуют обратные события (undo удаления Tag — `tag_created` и `widget_updated` для затронутых Scene; undo создания Scene — `scene_deleted` и `widget_deleted` по Widgets; undo смены Pin — `library_pin_updated` и `widget_updated`; и так далее по каждому виду) и двигают позицию. Сами undo/redo в лог не пишутся.
- Снимки самодостаточны: undo восстанавливает полное состояние строк, а не пересчитывает операцию. LIFO гарантирует, что более поздние изменения отменены раньше; при глубине 100 более старые строки удалены и не откатываются — undo останавливается на самой старой оставшейся.
- Новая правка после undo удаляет redo-хвост (строки с `seq > stack_position`) и дописывает изменение; это уже делает репозиторий 38_1, здесь проверяется тестом.
- `CanUndo`/`CanRedo` и описания: `undo_description` — описание изменения на `stack_position`, `redo_description` — на `stack_position+1`; пустой стек — пустые описания.
- Ошибки: `draft.ErrNothingToUndo`, `draft.ErrNothingToRedo` (409), `draft.ErrDraftConflict` (устаревшая Version, 409).

## Приложение

- `DraftChangesService` (`application/draft_changes_service.go`): `State(ctx, scope)`, `Undo(ctx, scope, version)`, `Redo(ctx, scope, version)` для Project и Library. Открывает живой Project/Library (`ProjectContent`/`LibraryContent`), идёт через Draft-репозиторий; undo/redo гейтятся EditLock и Version Draft. Ответ — `appdto.DraftChanges{Version, CanUndo, CanRedo, UndoDescription, RedoDescription}`.
- `main.go`: сервис в `restapi.Services`; apiclient получает методы.

## REST и контракт

- Под путём Project и Library:
  - `GET …/draft-changes` → 200 `{version, can_undo, can_redo, undo_description?, redo_description?}`; Project не найден/удалён — 404; Library — 404.
  - `POST …/draft-changes/undo` и `…/redo` с телом `{version}` → 200 то же состояние после операции; пустой стек — 409 `nothing_to_undo`/`nothing_to_redo`; устаревшая версия — 409 conflict; без замка — 409 с держателем; без `X-GrowSCADA-Client` — 400.
- `contract/api/v0`: `DraftChangesResponse`, `DraftChangeCommandRequest` (поле `version`), константы маршрутов/ошибок; `SchemaVersion` — MINOR, `make schemas`.
- `apiclient`: `DraftChanges`, `UndoDraftChange`, `RedoDraftChange` (у `ProjectClient` и `LibraryClient`).
- `growctl`: проверить, что мутирующий прогон работает с Version Draft (правки create/delete/update уже в 38_1/38_2); undo/redo в CLI не добавляется, если не понадобится — в задаче только тест, что планировщик не ждёт версий сущностей.
- Bruno: запросы `GET …/draft-changes`, undo, redo, пустой стек, устаревшая версия, без замка; `seq` — чтения, потом изменения; `make bruno_test` в конце задачи.
- `AGENTS.md`: маршруты `draft-changes`, сервис, семантика undo/redo; `CHANGELOG.md`.

## Тесты

- Домен: undo/redo каждой операции, обратные события, позиция, redo-хвост, глубина, пустой стек, снимки удалённых сущностей.
- Репозиторий: применение снимков в одной транзакции, CAS по `drafts.version`, обратные события в outbox, позиция, удаление redo-хвоста.
- Сервис: State/Undo/Redo для Project и Library, гейт EditLock, 404, 409.
- REST: оба маршрута для обеих областей, тела, коды, Problem Details, Bruno.
