# EditLock убрать: Edit и Save без Check in/out

Status: ready-for-agent
Priority: 1

Слой C. Термины — `GLOSSARY.md` (Draft, EditLock, Check out, Check in, Client). Решение — ADR 0022 (пишется здесь), отменяет ADR 0015 и правит ADR 0016.

Часть 1 из `72_save_release_deploy.md`. Начинает ветку `72_save_release_deploy`.

## Задача

Убрать замок редактирования целиком. Draft правится свободно; единственный токен конкурентности — Version Draft. Check out, Check in, Take over и read-only без замка исчезают из сервера, API, UI, growctl и Bruno. Это первый шаг к редакторской модели Edit/Save: сначала убираем неочевидный замок, затем (72_2) добавляем явный Save.

## Что убрать

### Домен и приложение

- Пакет `internal/server/domain/editlock` удаляется. Его `Scope` (Project или Library) переезжает в `domain/draft` (или в нейтральный `domain/scope`, если так проще избежать циклов импортов) и больше не несёт версии замка. Обновить все ~80 файлов, которые импортируют `editlock`.
- `editlock.EditLock`, `Holder`, `Snapshot`, события `edit_lock_*`, `EditLockService`, `EditLockGate`, `RunEditLockReaper`, presence (`Attach`/`Detach`), `editlock.System` — удалить.
- Из `domain/draft.Draft` убрать `Lock()`, `WithLock`, `PendingEvents` (если он остаётся только ради событий замка) и поля замка. Draft остаётся неизменяемым агрегатом с `Version`, `StackPosition`, undo/redo.
- Из `draftRepositoryPostgres` убрать `gateDraft`, проверку держателя, `last_seen` и `lock_version`; `Update` проверяет только `expected.Number() == row.version` и отдаёт `draft.ConflictError` при расхождении.
- Deploy, Release, Discard больше не проверяют замок. Deploy/Release проверяют только свои инварианты; Discard — Version/состояние Draft.
- Политики (`System`) больше не нуждаются в отдельной идентичности; если `event`/context ещё несёт «системного» клиента, убрать.

### Схема БД

- Миграция `engineering`: из `engineering.drafts` удалить `holder_key`, `holder_name`, `held_since`, `last_seen`, `lock_version` (и `engineering.edit_locks`, если она ещё где-то осталась; в текущей схеме замок уже в `drafts`).
- Проверить `runtime` и `public` на ссылки на замок (их быть не должно).
- Сид не создаёт замков.

### REST и контракт

- Удалить `contract/api/v0/editlock.go`, DTO замка, `Problem.edit_lock`, `Problem.Holder`, события `edit_lock_*` из `EventTypes`, `restapi.apiEventTypes`, `samples`. Четыре маршрута на Project и Library (`GET/POST/DELETE …/edit-lock`, `POST …/edit-lock/takeover`) отвечают **410 Gone** в формате Problem Details с подсказкой «EditLock убран: правьте и сохраняйте»; удалить их при выходе API 1.0 (как `/api/v1`).
- Убрать middleware `X-GrowSCADA-Client` и `?client=` из SSE; `eventHub` больше не считает presence. Параметр `?client=` у `/events` и `/projects/{id}/events` принимается и игнорируется, чтобы старые вкладки не падали; заголовок `X-GrowSCADA-Client` игнорируется.
- Пока MAJOR = 0 поднять MINOR `api/v0`, выполнить `make schemas` (правило 4 AGENTS.md).
- `apiclient`: удалить `WithClientKey`, методы замка, `CheckOut`/`CheckIn`/`TakeOver`; `Client.Project` больше не носит ключ клиента.
- 400 «без клиента» и 409 `edit_lock` исчезают; записи без Version по-прежнему 400 там, где Version обязательна.

### UI

- Из шапки Project и Library убрать строку замка (`Check out`, `Editing as … — Check in`, `… is editing since … — Take over`).
- Убрать read-only режим без замка: кнопки правки, Deploy/Release/Discard доступны всегда (их гейт — Version и серверные инварианты).
- Убрать `editlock:client` (sessionStorage), `editlock:name` (LocalStorage), `?client=` у SSE и `X-GrowSCADA-Client`. Имя автора понадобится в 72_2 — тогда завести `author:name`; в этой части его можно не вводить.
- Убрать обработку событий `edit_lock_*`, `EventResync`-перечитывание замка, toast «потерян замок».
- `GiveBackDraft`/`waitMoves` в Project: убрать отправку Check in; логика «дождаться серии стрелок перед записью» остаётся, потому что она про Version, а не про замок.
- При 409 `draft_conflict` UI перечитывает содержимое и стек (это уже есть); добавить понятный toast «Кто-то изменил Draft, обновите» вместо «замок занят».

### growctl и Bruno

- `growctl`: убрать Check out/Check in вокруг мутирующего прогона, флаги `--force`/имя держателя, заголовок клиента. Мутации просто шлют Version.
- Bruno: удалить setup/teardown-запросы замка (`get_edit_lock_*`, `post_edit_lock_*`, `delete_edit_lock_*`) и `X-GrowSCADA-Client` из всех запросов. Статическая проверка `TestBrunoCollection` должна снова пройти без непокрытых маршрутов.
- Устройства/симулятор: заголовок клиента не использовали — проверить, что ничего не сломалось.

### Документы

- ADR 0022 «Draft правится без EditLock; конкурентность — Version, автор — объявленное имя» (пишется здесь); поправка в ADR 0016 (замок больше не поле Draft).
- ADR 0015 помечается как заменённый ADR 0022 (не удалять: история решений).
- `GLOSSARY.md`: удалить EditLock, Check out, Check in, Client (или переопределить Client как автора); добавить Author.
- `AGENTS.md`: убрать строки про EditLock, Check in/out, `X-GrowSCADA-Client`, reaper, read-only; описать конкурентность по Version.
- `CHANGELOG.md`.

## Конкурентность после удаления

- Два инженера правят один Draft: каждая запись несёт Version, которую видела вкладка. Первая запись поднимает Version, вторая получает 409 `draft_conflict` и перечитывает Draft и стек. Никаких 400 «нет клиента» и 409 «замок занят».
- Undo/redo остаются общими: без замка второй инженер может отменить чужую DraftChange. Это осознанно до задач 49/50; записать в ADR 0022 и в UI-подсказку не обязательно.
- Deploy/Release/Discard больше не сериализуются замком. Deploy пока читает Draft (до 72_4) и защищается **проверкой Version вокруг сборки**:
  1. прочитать Version Draft V;
  2. собрать ProjectFile (сборка вне транзакции, как в 72_2 Save);
  3. в короткой транзакции заблокировать строку Draft и проверить, что Version всё ещё V; иначе 409 `draft_conflict`, клиент повторяет;
  4. только после успешной проверки отправить документ на Runtime.
  Так Deploy отправляет согласованный снимок Draft на Version V; правка после проверки на него уже не влияет. Тот же помощник переиспользует 72_2 для Save.
- Release и Discard не читают Draft так, как Deploy: Release в 72_3 читает неизменяемый Commit, Discard — снимок Draft в одной транзакции; отдельная защита им не нужна.

## Приёмка

- В коде и документации не осталось `editlock`, `edit_lock`, `CheckOut`, `CheckIn`, `TakeOver`, `X-GrowSCADA-Client` (кроме исторических ADR/CHANGELOG).
- Project и Library правятся в двух вкладках: вторая получает 409 `draft_conflict` и перечитывает состояние; read-only режима нет.
- Deploy, Release, Discard работают без замка; их прежние проверки замка удалены.
- Deploy, запущенный одновременно с правкой Draft, не отправляет смешанный ProjectFile: при изменении Version во время сборки — 409 `draft_conflict`, повтор даёт согласованный документ.
- `make build`, `make test`, `make bruno_test` проходят; Bruno не содержит запросов замка; UI не показывает строку замка и не шлёт `?client=`.
- `make schemas` обновлён; схема `api` без edit-lock; старые маршруты `/edit-lock` отвечают 410 и покрыты Bruno.

## Comments

- 2026-11-07: выделено из `72_save_release_deploy.md`. Первая часть: не зависит от имени снимка (решение — Commit) и сразу убирает неочевидный Check in/out. Автор появится в 72_2.
- 2026-11-07 (решение): Deploy до 72_4 защищается проверкой Version вокруг сборки — чтение V, сборка, проверка V в короткой транзакции, иначе 409; смешанный ProjectFile не отправляется. Помощник переиспользуется в 72_2.
