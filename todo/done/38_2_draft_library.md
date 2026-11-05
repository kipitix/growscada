# Draft Library на Draft-агрегате

Status: ready-for-agent
Blocked by: 38_1
Parent: 38_draftchange_undo_redo
Part: 2/4

Часть 2 задачи 38. Источник истины — `todo/backlog/38_draftchange_undo_redo.md`, решения — ADR 0016 и ADR 0017. Фундамент (схема `drafts`/`draft_changes`, домен Draft, DraftChange, слияние EditLock, Draft проекта) готов в 38_1; здесь — вторая область.

**За рамками:** Draft проекта (38_1), маршруты undo/redo (38_3), UI undo/redo (38_4).

## Схема

- `engineering/20261105000002_library_draft.sql`:
  - удалить `engineering.widget_types.version`;
  - удалить `libraries.discarding_to`.
- Строки `engineering.drafts` для Library уже созданы/бэкфиллятся в 38_1; здесь меняется только содержимое.

## Домен

- `Draft` Library (`Scope` = Library) получает операции `CreateWidgetType`, `UpdateWidgetType`, `DeleteWidgetType` и `RecreateWidgetType` (для Discard); `WidgetType` теряет `Version()`/`CheckVersion`/`expected`, версию проверяет Draft. WidgetType остаётся в `domain/library`, Draft его композирует.
- Инвариант «WidgetType принадлежит Draft этой Library» проверяется в транзакции Draft; Scene на WidgetType не ссылается напрямую (только через закреплённый Release, ADR 0007), поэтому удаление WidgetType ничего в Scenes не меняет.
- `LibraryRelease` остаётся отдельным агрегатом (ADR 0007): Release читает Draft, проверяет Version Draft в своей транзакции (вместо `libraries.version`), пишет релизные строки и коммитит; стек чистится отдельной транзакцией после успеха (ADR 0017), сбой очистки — 500 при уже созданном релизе.
- `Library` теряет `DiscardingTo`/`StartDiscard`/`FinishDiscard`; `LibraryContent.openNotDiscarding`, `library.ErrLibraryDiscarding`, `NewDiscardLibraryPolicy`, событие `library_discard_started` и поле `discarding_to` удаляются.
- Discard Library: читает последний Release, одной транзакцией Draft сбрасывает WidgetTypes (recreate/restore/remove), чистит стек и коммитит; ответ 200. Политика Discard и пометка не нужны — гейт EditLock.

## Приложение и REST

- `WidgetTypeService` сохраняет интерфейс, каждая запись принимает Version Draft и идёт через Draft-репозиторий; чтения (`FindAllWidgetTypes`, `FindWidgetTypeByID`, …) остаются.
- `LibraryService.Release` принимает `version` (Version Draft, которую видел клиент) в `CreateReleaseRequest` и проверяет её в транзакции релиза; `LibraryService.Discard` синхронный, 200; `Library` теряет `discarding_to`.
- `POST …/libraries/{id}/discard` отвечает 200 состоянием Draft; Problem `library_discarding` уходит.
- `api/v0` — MINOR: `make schemas`; apiclient (create/delete WidgetType с версией, Release с версией, Discard), Bruno (WidgetTypes, Release, Discard, убрать `discarding_to`), `CHANGELOG.md`.
- UI Library-режима — минимальная адаптация под контракт: убрать `discarding_to` и ответ 202 у Discard; кнопки undo — в 38_4.
- `AGENTS.md`: Draft Library на Draft-агрегате, Release и Discard, удалённые политики.

## Тесты

- Домен: операции Library Draft, конфликт Version Draft, Discard-операции над WidgetTypes.
- Репозиторий: CAS по `drafts.version` для Library, строки `draft_changes`, гейт замка Library, отсутствие записи при отказе.
- Сервисы: Release проверяет Version Draft и чистит стек после успеха, Discard одной транзакцией, каждая запись — одна DraftChange.
- REST: create/delete WidgetType с версией, Release с версией, Discard 200, отсутствие `discarding_to`, гейт замка.
- Bruno: обновлённые тела и порядок; `make bruno_test` в конце задачи.
