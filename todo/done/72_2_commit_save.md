# Commit: Save как снимок Draft с заголовком и автором

Status: ready-for-agent
Priority: 1
Blocked by: 72_1

Слой C. Термины — `GLOSSARY.md` (Draft, DraftChange, Commit, Author). Часть 2 из `72_save_release_deploy.md`.

Имя сущности — **Commit** (решение 2026-11-07). Работающая конфигурация Runtime node — **Deployment** (72_9).

## Задача

Каждый Save создаёт неизменяемый снимок всего Draft — Commit — с номером, заголовком, автором, временем и Version Draft. Commit принадлежит агрегату Draft (как DraftChange), а не является отдельным агрегатом. Список Commit'ов виден; заголовок меняется отдельной кнопкой. Save не является единственной точкой сохранения: правки Draft по-прежнему пишутся сразу; Save фиксирует именованную точку истории.

## Домен

- `domain/draft.Commit` — сущность Draft:
  - `ID`, `Number` (1, 2, 3… в пределах Draft), `Title` (VO: непустой, обрезанный, максимум ~200 символов), `Author` (VO: непустое объявленное имя, максимум ~100), `CreatedAt`, `DraftVersion` (Version Draft, которую зафиксировал Save), `Content`.
  - Содержимое неизменяемо; `WithTitle` возвращает новый Commit для отдельной операции переименования. События: `commit_created`, `commit_title_updated`.
  - `Number` и `ID` уникальны в Draft; номер выдаёт репозиторий под блокировкой строки Draft.
- `domain/draft.Draft` получает `LastCommitNumber` и `LastCommitDraftVersion` (или Commit читается отдельно). Save без изменений (`LastCommitDraftVersion == Version`) — `ErrNothingToSave`.
- Содержимое Commit:
  - Project — `projectv0.ProjectFile`, собранный тем же каноническим сборщиком, что Deploy: Tags, Devices, Environments, пары, Scenes с Widgets, копии WidgetType закреплённых Release, пины. Сборщик вынести в функцию над `draft.Content` + пинами, чтобы её вызывали и Deploy, и Save внутри транзакции Draft.
  - Library — снимок всех WidgetType Draft в виде `[]library.ReleasedWidgetType` (тот же value object, что в Library Release), сериализованный внутренним кодеком с версией формата (как outbox/DraftChange, ADR 0005).
- Commit не пишет DraftChange: это не правка содержимого. Стек undo/redo не трогается. Version Draft не поднимается.

## Репозиторий и транзакция

- Таблица `engineering.commits`:
  - `id uuid pk`, `draft_id uuid not null references engineering.drafts(id) on delete cascade`,
  - `number int not null`, `title text not null`, `author text not null`, `created_at timestamptz not null`,
  - `draft_version int not null`, `content jsonb not null`,
  - `unique (draft_id, number)`, индекс `(draft_id, number desc)`.
- В `engineering.drafts` добавить `last_commit_number int not null default 0`, `last_commit_draft_version int not null default 0`.
- `engineering.commit_library_pins (commit_id uuid references engineering.commits(id) on delete cascade, library_id uuid, release_number int, primary key (commit_id, library_id))`: пины ProjectFile Commit'а, пишутся в транзакции Save. Нужны задаче 71 (защита Library от удаления); для Library Commit'ов таблица пуста (у Library нет пинов).
- Новый метод `draft.Repository.SaveCommit(ctx, scope, expected version.Version[draft.Draft], title, author) (draft.Commit, error)`:
  1. Собрать содержимое Draft **по Version `expected`** существующим сборщиком (ProjectFile для Project, WidgetTypes для Library). Сборка не в транзакции и может быть тяжёлой; это нормально. Тот же помощник «прочитать Version → собрать → проверить Version» использует Deploy в 72_1.
  2. Открыть транзакцию, `SELECT … FOR UPDATE` строки Draft (как `Update`), проверить, что Project/Library жива.
  3. Сверка `expected` с текущей `version`; расхождение — `draft.ConflictError` (409). Если во время сборки была запись, она уже закоммичена (мы ждём её на блокировке строки), Version выросла — Save отклоняется, клиент повторяет. Так содержимое Commit'а всегда соответствует Version.
  4. Если `last_commit_draft_version == version` — `ErrNothingToSave` (409).
  5. Отказ сборщика (несобираемый Draft) — `ErrDraftNotSaveable{Reason}` (409).
  6. `number = last_commit_number + 1`; вставка Commit и строк `commit_library_pins`; обновление `last_commit_number`, `last_commit_draft_version = version`.
  7. Коммит; outbox с `commit_created`.
- Альтернатива — сборка внутри транзакции через `draftLoader` — не нужна: блокировка строки Draft + сверка Version даёт тот же результат без переписывания сборщика. Если понадобится строгая изоляция чтения, вернуться к ней отдельной задачей.
- Переименование: `draft.Repository.RenameCommit(ctx, scope, number, title) (draft.Commit, error)` — без Version Draft (заголовок не содержимое), но с проверкой, что Commit принадлежит области; событие `commit_title_updated`. Кнопка работает и после чужих правок Draft; содержимое от этого не меняется.
- `FindCommits(ctx, scope)`, `FindCommit(ctx, scope, number)`; Project/Library не жив — `project.ErrProjectNotFound`/`library.ErrLibraryNotFound`.
- Save не блокирует другие записи Draft дольше сборки документа; сборка внутри транзакции может быть тяжёлой. Принять для первой версии; при необходимости вынести в read-only снимок с ретраем (не усложнять сейчас).

## Application

- `CommitService` (Project и Library, либо один с `draft.Scope`):
  - `Save(ctx, scope, rawVersion int, title, author string) (appdto.Commit, error)`;
  - `List(ctx, scope) ([]appdto.Commit, error)`;
  - `Find(ctx, scope, number int) (appdto.Commit, error)`;
  - `Rename(ctx, scope, number int, title string) (appdto.Commit, error)`.
- Ошибки: `ErrNothingToSave` (409 `nothing_to_save`), `ErrDraftNotSaveable` (409 `draft_not_saveable` с `reason`), `draft.ConflictError` (409 `draft_conflict`), неверный ввод (400) — пустой заголовок/автор, номер < 1.
- `Author` пока приходит в теле запроса; User (задача 49) заменит его. Пустое имя — 400; UI подставляет сохранённое.
- `appdto.Commit`: `number`, `title`, `author`, `created_at`, `draft_version`, `released` (bool, false до 72_3), `content_counts` (для Project: scenes/widgets/tags/devices/environments; для Library: widget_types). Полное содержимое — отдельным маршрутом/файлом.

## REST (`contract/api/v0`, MINOR, `make schemas`)

Под Project и Library:

- `POST …/commits` `{version, title, author}` → 201 `CommitResponse`; 409 `nothing_to_save`, `draft_not_saveable`, `draft_conflict`; 400.
- `GET …/commits` → 200 `{commits: [CommitResponse]}` (новые первыми, без содержимого).
- `GET …/commits/{n}` → 200 `CommitResponse` + `content_counts`; 404.
- `PATCH …/commits/{n}` `{title}` → 200 `CommitResponse`; 400/404.
- `GET …/projects/{id}/commits/{n}/projectfile` → 200 `application/json` attachment (как `GET …/revisions/{n}/projectfile`), 404.
- Для Library содержимое Commit доступно через `GET …/commits/{n}` (WidgetTypes) или отдельным `GET …/commits/{n}/widget-types`; решить в реализации, но не грузить содержимое в список.
- События `commit_created`, `commit_title_updated`: Project — в поток Project, Library — в серверный поток; `ReloadTargetsFor` обновляет список Commit'ов, но не содержимое Draft.
- `apiclient`: `SaveCommit`, `Commits`, `Commit`, `RenameCommit`, `CommitProjectFile`.

## UI (Project и Library, минимально в этой части)

- В шапке — кнопка **Save** и индикатор «есть несохранённые изменения» (Version Draft > `draft_version` последнего Commit); при отсутствии изменений Save выключается или отвечает toast по 409 `nothing_to_save`.
- Первый Save спрашивает имя автора (`author:name` в LocalStorage) и заголовок (диалог; по умолчанию «Сохранение DD.MM.YYYY HH:MM»). Дальше имя подставляется.
- Диалог **История сохранений**: номер, заголовок, автор, время; у каждой строки — кнопка переименования заголовка; содержимое — по кнопке (72_3 добавит «Сделать Release»).
- Индикатор и список перечитываются на `commit_created`/`commit_title_updated` и на `EventResync`; после Save — сразу.
- Save не должен требовать, чтобы Draft был задеплоен; он работает и до первого Deploy.
- Убрать упоминания «Check in/out» из подсказок (72_1 уже убрал механику).

## Тесты

- Домен: Commit номера/заголовок/автор; `WithTitle`; ошибки пустого заголовка/автора; Save без изменений.
- Репозиторий (testcontainers): Save создаёт commit 1, затем 2; `last_commit_*`; Save без изменений — `ErrNothingToSave`; две конкурентные Save на одной Version — один Commit; Draft, изменённый между Save, даёт 409 на устаревшей Version; переименование; CASCADE при удалении Draft; содержимое Project — канонический ProjectFile, содержимое Library — WidgetTypes.
- Application/REST: маршруты, 409/400/404, события; Project и Library.
- UI (`uitest`): Save создаёт строку в истории, индикатор гаснет, переименование меняет заголовок, две вкладки — 409 и перечитывание.
- Bruno: save/list/find/rename, `requestBodies`; статическая проверка.
- `make build`, `make test`, `make bruno_test`.

## Документы

- ADR 0021 (начало): Commit — сущность Draft, Save — снимок; окончательно оформить в 72_8.
- `GLOSSARY.md`: Commit, Author, Save; обновить Draft (история Commit'ов).
- `AGENTS.md`, `CHANGELOG.md`.

## Вне объёма

- Release из Commit — 72_3 (Project) и 72_5 (Library).
- Deploy — 72_4.
- Retention/обрезка Commit'ов — отдельная задача позже.
- User/права — 49/50.

## Comments

- 2026-11-07: выделено из 72. Имя снимка — Commit (решение 2026-11-07); Runtime-сущность переименована в Deployment (72_9).
