# EditLock: Draft Library

Status: ready-for-agent
Blocked by: 37_1
Parent: 37_editlock
Part: 2/2
Closes parent: yes

Часть 2 задачи 37. Источник истины — `todo/backlog/37_editlock.md` (ADR 0015). Механика (агрегат, таблица `engineering.edit_locks`, presence, reaper, события, контракт, apiclient) готова в 37_1; здесь — вторая область: Draft Library.

**За рамками:** поведение Draft проекта (37_1), DraftChange и undo/redo (38), User (49), права доступа и публичность Library (50).

## Сервер

- Включить Library-область в сервис и страж: `Scope` Library, строка `engineering.edit_locks` по `library_id` (колонка уже есть с 37_1; новой миграции не нужно).
- Маршруты: `GET …/libraries/{id}/edit-lock`, `POST …/edit-lock`, `DELETE …/edit-lock`, `POST …/edit-lock/takeover` — та же семантика, что у проекта (201/200/409, `expected_version` у перехвата, 409 `library_discarding`, пока идёт Discard).
- Гейт: записи WidgetType Draft Library и Release и Discard библиотеки; проверка WidgetType — в транзакции записи (репозиторий области Library), Release — в `LibraryReleaseRepository.Save` (там же, где блокировка строки Library и `ErrDraftChanged`), Discard — в транзакции записи пометки (проверяющий метод репозитория Library). Pin/switch/unpin Library Pin со стороны проекта — это Draft проекта, замок Library не берут; чтение релизов и `ImpactOf` — тоже.
- События `edit_lock_*` области Library идут в серверный поток (`Scope().ProjectID()` = `uuid.Nil`); `newAPIEvent` отдаёт состояние замка и ID области. Библиотечный `DELETE …/libraries/{id}` и переименование остаются вне замка.
- CORS, контракт и схемы — уже с 37_1; новых типов нет, только маршруты Library (MINOR-версия контракта не меняется сверх 37_1).

## UI (Library-режим)

- В шапке выбранной Library — та же строка замка, что в Project: `Check out` (имя из общего `LocalStorage`), `Editing as <name> — Check in`, `<name> is editing since HH:MM — Take over`.
- Без замка Library только для чтения: создание, правка, удаление WidgetType, Release (с заметкой) и Discard (с подтверждением) выключены с подсказкой; список Library, переименование и удаление Library остаются доступны.
- Потеря замка — toast и read-only. Переключение выбранной Library сначала шлёт Check in прежней; список релизов и просмотр содержимого — всегда доступны.
- Состояние грузится на `EventResync`/`EventProjectResync` (Library-события приходят в серверном потоке, который Bar держит всегда) и по `edit_lock_*`.
- UI-тесты: свободный/занятый/свой замок, read-only для WidgetType/Release/Discard, Check out/in, перехват, toast о потере, Check in при переключении Library.

## Инструменты и тесты

- growctl: мутирующие команды Library (`apply` манифеста Library/WidgetType, `release`, `discard`) берут Check out и Check in так же, как проектные; занятый замок — ошибка с держателем, `--take-over` — форс.
- Bruno: setup/teardown для Library, заголовок клиента у мутирующих запросов раздела библиотек; статическая проверка и порядок `seq`.
- Серверные и UI-тесты, пишущие в Draft Library, берут замок в setup; тесты, пишущие в Project, — как в 37_1.
- `CHANGELOG.md` и `AGENTS.md` (Library-маршруты, строка замка и read-only Library-режима).

## Проверка

- `make build`, `make test`, `make bruno_test`; ручная проверка PWA: Library — Check out, правка WidgetType и Release, Check in; второй клиент видит держателя и получает read-only; перехват; закрытая вкладка отпускает замок по таймауту.

## Comments

- 2026-11-05: часть задачи 37 после грилинга; разрез по областям (Project в 37_1, Library здесь), чтобы после каждой части приложение оставалось рабочим.
