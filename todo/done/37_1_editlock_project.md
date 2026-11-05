# EditLock: Draft проекта

Status: ready-for-agent
Blocked by: 31, 33
Parent: 37_editlock
Part: 1/2

Часть 1 задачи 37. Источник истины — `todo/backlog/37_editlock.md` (решения грилинга 2026-11-05, ADR 0015). Здесь — вся механика EditLock и её первая область: Draft проекта. Draft Library остаётся без замка до 37_2.

**За рамками:** Draft Library и её Release/Discard, события и UI Library — 37_2. DraftChange и undo/redo — 38. User как держатель — 49. Права доступа — 50. Разнесение узлов по сети — 52.

## Миграция и домен

- Миграция `engineering.20261105000000_edit_locks`: таблица `engineering.edit_locks` — `project_id uuid NULL REFERENCES projects(id) ON DELETE CASCADE`, `library_id uuid NULL REFERENCES engineering.libraries(id) ON DELETE CASCADE`, `holder_key uuid NULL`, `holder_name text NULL`, `since timestamptz NULL`, `last_seen timestamptz NULL`, `version bigint NOT NULL`; CHECK «ровно одна область», CHECK «держатель целиком или никак», частичные уникальные индексы по каждой области. Строка остаётся при свободном замке: `version` растёт монотонно, `last_seen` её не поднимает.
- `domain/editlock`: `Scope` (Project или Library), `Holder` (ключ + имя), агрегат `EditLock` (держатель, `since`, `version`) с `Acquire`, `Release`, `TakeOver(expectedVersion)`, `Expire`, `Seen`; ошибки `ErrEditLockHeld`, `ErrEditLockFree`, `ErrEditLockNotHeld`, `ErrEditLockVersionStale`, `ErrEditLockNameEmpty`, `ErrNoClient`; события `edit_lock_acquired`, `edit_lock_released`, `edit_lock_taken_over`, `edit_lock_expired` (в задаче — только для области Project; в `event.AllEventTypes` все четыре). Событие несёт `Scope()`; поток проекта выбирается по `Scope().ProjectID()` (у Library — `uuid.Nil`, серверный поток). `domain/editlock` входит в `engineeringRole` в `roles_test.go`.
- `EditLockRepository` (домен) и его postgres-реализация: `Find(scope)`, `Create`, `Save` (CAS по `version`), `FindAllHeld`. События — через outbox, как у прочих агрегатов инженерного узла (ADR 0008).
- Проверка в записи — отдельный страж в `postgres/repositories` (читает `engineering.edit_locks` `FOR SHARE` в транзакции записи и сверяет ключ из context); `aggregateStore` Project-области вызывает его перед изменением. Политики и очистка ставят в context `editlock.System` и проверку не проходят; запрос без ключа — `ErrNoClient` (400).

## Сервис и живость

- `application.EditLockService` (инженерный узел): `State`, `CheckOut`, `CheckIn`, `TakeOver`, `Attach(key)`, `Detach(key)`; presence — map ключ → число SSE-соединений в памяти узла.
- Reaper: горутина раз в 30 с (`ReapOnce` для тестов) — присутствующему держателю обновляет `last_seen`, отсутствующего с `last_seen` старше `editLockTTL = 5 мин` освобождает с `edit_lock_expired`. Запускается в `main.go` (и в `servertest`), останавливается по context.
- Check out свободного — 201; занятого — 409 с держателем; Check in держателем — 200, свободного — 200 (идемпотентно), чужого — 409; перехват занятого с `expected_version` — 200, устаревшая — 409, свободного — 409. Check out при идущем Discard проекта — 409 `project_discarding`.
- Deploy и Discard проверяют замок: Deploy — перед сборкой ProjectFile (она идёт несколькими запросами, одной транзакции нет, CAS не нужен), Discard — в транзакции записи пометки (отдельным проверяющим методом репозитория Project); Release — в 37_2.

## REST и контракт

- `contract/api/v0` MINOR (`make schemas`): константа заголовка `X-GrowSCADA-Client` и параметра `?client=`; `EditLockResponse` (`held`, держатель с именем и `since`, `version`), `CheckOutEditLockRequest`, `TakeOverEditLockRequest`; поле `edit_lock` в `Problem` — на 409 без замка; четыре имени событий и поле состояния замка в `Event`.
- Маршруты проекта: `GET …/projects/{id}/edit-lock` (200 и при свободном), `POST …/edit-lock` (Check out), `DELETE …/edit-lock` (Check in), `POST …/edit-lock/takeover`. CORS: заголовок клиента в `Access-Control-Allow-Headers`. Заголовок читается middleware и кладётся в context каждого запроса (пустой — «клиента нет»).
- SSE: `?client=` у `/events` и `/projects/{id}/events`; обработчик при открытии делает `Attach`, при закрытии — `Detach`. Параметр необязателен (Device, History и прочие потоки без него).
- `apiclient`: `WithClientKey(key)` (заголовок на каждом запросе), `StateEditLock`, `CheckOutEditLock`, `CheckInEditLock`, `TakeOverEditLock`; 409 уже даёт `ErrConflict` и Problem.

## UI (Project-режим)

- Ключ вкладки — UUID в `sessionStorage` (`editlock:client`), имя — в `LocalStorage` (`editlock:name`); создаются в `root`, уходят в `apiclient` и в `eventlog.Bar` (параметр `client`).
- В полосе Project — строка замка: `Check out` (при первом разе спрашивает имя), `Editing as <name> — Check in`, `<name> is editing since HH:MM — Take over`. Состояние грузится на `EventProjectResync` и по событиям `edit_lock_*`.
- Без замка Project-режим только для чтения: создание/переименование/удаление Scene, добавление/перемещение/размер/поворот/удаление Widget и его привязки и подписи, создание/удаление Tag, pin/switch/unpin Library, Deploy и Discard — выключены с подсказкой. Диалог Revisions и Rollback остаются доступны.
- Перехват — диалог подтверждения с текущим держателем и его временем, затем POST с `version`, затем перезагрузка содержимого. Потеря замка (перехват, истечение, Check in из другой вкладки) — toast и read-only.
- Переключение проекта в `projects.Switcher` (и уход из Project-режима с выбранным проектом) сначала шлёт Check in прежнему; отказ/сеть — toast, дальше таймаут.
- UI-тесты: свободный/занятый/свой замок в строке, read-only выключает правки и Deploy/Discard, Check out и Check in, перехват с подтверждением, toast о потере, Check in при переключении проекта.

## Инструменты

- growctl: мутирующие команды проекта берут Check out с ключом на запуск и именем (`--client-name`, по умолчанию — имя пользователя ОС), в конце Check in; занятый замок — ошибка с именем держателя, `--take-over` — форс. `apply` без изменений замок не берёт.
- Bruno: setup-запрос Check out проекта Main, заголовок клиента у всех мутирующих запросов, teardown — Check in; статическая проверка коллекции (`routes`, `requestBodies`) и порядок `seq` обновлены.
- Серверные и UI-тесты, которые пишут в Draft проекта, берут замок в setup (хелпер `servertest`); тесты без записи — нет.

## Документы и проверка

- ADR 0015, `GLOSSARY.md` (Client, Check out, Check in, EditLock), `AGENTS.md` (маршруты, строка замка, read-only), `CHANGELOG.md`.
- `make build`, `make test`, `make bruno_test`; ручная проверка PWA: Check out, правка, Check in, второй клиент видит «редактирует», перехват, read-only после перехвата.

## Comments

- 2026-11-05: часть задачи 37 после грилинга. Разрез по областям выбран вместо «сервер, потом UI»: после этой части приложение работает (Project под замком, Library — пока без), после 37_2 — целиком.
