# Library Release из Commit; закрепления и защита от удаления Library

Status: ready-for-agent
Priority: 1
Blocked by: 72_2, 71

Слой C. Термины — `GLOSSARY.md` (Library, Library Release, Library Pin, Commit, Release). Часть 5 из `72_save_release_deploy.md`.

## Задача

Library Release делается из Commit, а не из Draft. Существующие Library Release остаются рабочими как `legacy`. Закрепление Project'а по-прежнему указывает на номер Library Release; Project видит WidgetType только из закреплённой Release. Удаление Library больше не может оборвать Release, названный Commit'ом, Project Release или работающей Deployment (задача 71).

## Домен и хранение

- Library Commit уже создаётся в 72_2: содержимое — `[]library.ReleasedWidgetType` (снимок всех WidgetType Draft).
- `library.LibraryRelease` получает `CommitNumber *int`, `Author`, `Origin` (`commit`/`legacy`), `Note`, `Number`, `CreatedAt`, `WidgetTypes`. Содержимое неизменяемо; заметка редактируется отдельной кнопкой (`WithNote`, событие `library_release_note_updated`).
- `engineering.library_releases` расширяется:
  - `commit_number integer null`, `author text not null default 'system'`, `origin text not null default 'legacy' check (origin in ('commit','legacy'))`;
  - `commit_number` nullable: старые Release не имеют Commit (Commit есть только у Release, созданной после 72_2/72_5);
  - unique `(library_id, commit_number) where commit_number is not null and origin = 'commit'`.
- `engineering.released_widget_types` не меняется: Release по-прежнему копирует туда WidgetTypes (это проекция для `FindPinnedWidgetType`, палитры Project и `released_widget_types`-тестов). Запись идёт в той же транзакции, что вставка `library_releases`.
- `CreateRelease` из Commit:
  - Commit существует и принадлежит Library; Draft и его Version не читаются;
  - один Commit — одна Release: `ErrCommitAlreadyReleased` (409);
  - номер — следующий в Library (unique + повтор `ErrReleaseNumberTaken` или advisory lock — как в 72_3);
  - копирует WidgetTypes Commit'а в `released_widget_types`;
  - событие `library_released` (существующее; для Project Release — `release_created`, 72_3).
- `LibraryService.Release` перестаёт проверять Draft Version и `DraftDiffers`: Commit уже снимок. `ErrNothingToRelease` больше не нужен; Save без изменений отвечает `nothing_to_save` (72_2).
- «Есть несохранённые изменения» = Version Draft > `draft_version` последнего Commit. «Есть неопубликованные изменения» = номер последнего Commit больше `commit_number` последней Release. Признак в списке Library (`ReleaseStateOf`/`DraftDiffers`) пересчитать на эту пару; `DraftDiffers` оставить только если он нужен для отображения, без SQL-копии.

## Discard Library

- Library **Discard** сбрасывает Draft к **последнему Commit'у** (последнему Save) — отменить несохранённое. Если Commit'ов нет — 409 `nothing_to_discard`, а не сброс к Release.
- **Revert to latest Release** — отдельное действие: сброс Draft к последней опубликованной Library Release (прежняя семантика Discard). Новый маршрут `POST …/libraries/{id}/revert-to-release`; нет Release — 409.
- Discard и Revert — одна транзакция над Draft (ADR 0016), чистят стек undo/redo, не трогают Commit/Release.
- Project Discard тоже к последнему Commit'у; Project Revert to deployed — к работающей Deployment (72_4). Семантика обоих режимов одинаковая: Discard = отменить несохранённое, Revert = вернуться к опубликованному.

## API

Под Library:

- `POST …/libraries/{id}/releases` меняет тело с `{version, note}` на `{commit, note?, author}`:
  - 201 `ReleaseResponse`; 409 `commit_already_released`; 404 Commit/Library; 400.
  - `version` больше не принимается; старый клиент получит 400 (пока MAJOR = 0 это допустимо; Bruno/growctl обновляются здесь и в 72_8).
- `GET …/libraries/{id}/releases` → 200 `{releases: [...]}` с `commit_number`, `title` (из Commit или null), `author`, `origin`, `note`, `created_at`, `content_counts`.
- `GET …/libraries/{id}/releases/{n}` → 200 с содержимым WidgetTypes (как сегодня).
- `PATCH …/libraries/{id}/releases/{n}` `{note}` → 200 `ReleaseResponse`; содержимое не меняется, заметка — метаданные; событие `library_release_note_updated`.
- `GET …/projects/{id}/library-pins` и `GET …/projects/{id}/library-pins/{library_id}/impact`: в ответ добавить провенанс Release (номер Commit'а/заголовок/автор), чтобы Project видел, что за Release он закрепил. Сами WidgetTypes — как сегодня.
- `GET …/widget-types` (палитра Project) не меняется: WidgetTypes из закреплённых Release.
- `POST …/libraries/{id}/revert-to-release` → 200 `{version}`: сброс Draft к последней Release; 409 без Release. `POST …/libraries/{id}/discard` меняет смысл на «к последнему Commit» (409 `nothing_to_discard` без Commit'ов).
- `contract/api/v0` — MINOR, `make schemas`; `apiclient` обновить.

## Защита Library и Release (задача 71)

Каждый Commit Project'а хранит пины закреплённых Library Release, а Project Release копирует их. Поэтому одной проверки `library_pins` живых Project'ов недостаточно.

- Таблицы пинов уже пишутся в 72_2 (`engineering.commit_library_pins`) и 72_3 (`engineering.project_release_library_pins`); здесь их не дублировать.
- `NewLibraryCleanupPolicy` (реализация — задача 71, гибрид):
  - **блокируют** удаление живой `library_pins` Project'а и работающая Deployment, называющая Release этой Library (через `project_release_library_pins` или Runtime-порт);
  - **не блокируют** старые Commit/Project Release: их содержимое самодостаточно (копии WidgetTypes), история рендерится и после удаления Library. UI предупреждает, что в истории останется ссылка на удалённую Library.
- Работающая Deployment, созданная из файла (`release_number = nil`), может называть Release удалённой Library: задача 71 добавляет Runtime-порт `DeploymentNamesLibraryRelease` или производную таблицу пинов Deployment. Если Release исчезла, восстановление пинов (Discard к Commit'у или Revert к старой Deployment) пропускает пин с предупреждением и не ретраится вечно.
- Следствие: Library с одной только старой историей **можно** удалить; Library, которую держит живой Draft или работающая Deployment, — нельзя (409 с причиной).
- Отдельную Release по-прежнему нельзя удалить отдельно: `library_releases`/`released_widget_types` уходят только с Library.

## UI

Полный Library UI — 72_7. Здесь минимально:

- В истории сохранений Library у Commit'а — **Сделать Release** (заметка, автор).
- Список Release показывает провенанс Commit'а и `legacy` для старых; заметка редактируется отдельной кнопкой.
- В Project-пинах видно, какая Release закреплена (номер, заметка, автор, Commit).
- Discard Library — к последнему Commit'у, с подтверждением; отдельно Revert to latest Release — к последней Release.

## Тесты

- Домен: Release из Commit, один Commit — одна Release, чужой Commit, `legacy`, `Origin`, `WithNote`/`library_release_note_updated`.
- Репозиторий (testcontainers): миграция старых `library_releases` в `legacy`; `released_widget_types` заполняется из Commit; номер; unique commit; CASCADE; `FindPinnedWidgetType` продолжает работать; проверка `commit_library_pins`/`project_release_library_pins` при удалении Library (71).
- Application/REST: новый `POST …/releases`; list/find; `PATCH` заметки; pins с провенансом; 409/404/400.
- Cleanup (гибрид): живой pin и работающая Deployment блокируют удаление; Library с одной только старой историей Commit/Release удаляется; Discard к старой Deployment после удаления пропускает исчезнувшую Release с предупреждением.
- Discard: к последнему Commit'у; без Commit'ов — 409. Revert to latest Release: к последней Release; без Release — 409.
- UI (`uitest`): Release из Commit'а, провенанс в пинах, legacy-строка, Discard.
- Bruno: обновить library release flow, pins, discard; `requestBodies`; `seq`.
- `make build`, `make test`, `make bruno_test`.

## Документы

- ADR 0007 (поправка: Library Release из Commit; Commit/Release пинят Library), ADR 0012 (закрепление и защита), ADR 0016/0017 (Discard к Commit'у, стек), ADR 0021.
- `GLOSSARY.md`: Library Release, Library Pin, Commit, legacy.
- `AGENTS.md`, `CHANGELOG.md`.

## Вне объёма

- Retention Commit'ов и удаление истории.
- Публичность Library и права — задача 50.
- Полный Library UI — 72_7.

## Comments

- 2026-11-07: выделено из 72. Хранение Library Release оставлено в существующих таблицах, чтобы не ломать `released_widget_types` и закрепления; Project Release — новая таблица (72_3). Задача 71 стала критичной и входит в блокировку.
