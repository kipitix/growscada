# Project Release из Commit и список Release

Status: ready-for-agent
Priority: 1
Blocked by: 72_2

Слой C. Термины — `GLOSSARY.md` (Commit, Release, Project). Часть 3 из `72_save_release_deploy.md`.

## Задача

Project Release — публикация Commit'а: неизменяемый номер, заметка, автор, время, провенанс Commit'а и копия его `ProjectFile`. Release не удаляется; содержимое неизменяемо, заметка редактируется отдельной кнопкой. Список всех Release проекта виден в Project-режиме; в 72_4/72_6 он получит пометку работающей Release (через `GET …/deployment`) и кнопку Deploy.

## Домен

- `domain/release.Release` — отдельный агрегат области:
  - `ID`, `Scope` (Project или Library; Library реализуется в 72_5), `Number` (1, 2, 3… в области), `CommitNumber`, `CommitTitle` (снимок на момент Release), `Note` (необязательная), `Author`, `CreatedAt`, `Origin` (`commit`, позже `file`/`legacy`), `Content`.
  - Содержимое неизменяемо; `Note` задаётся при создании и **редактируется отдельной кнопкой** (`WithNote`, событие `release_note_updated`). Заголовок Release — снимок заголовка Commit'а на момент публикации; отдельно не меняется.
  - `CreateRelease(scope, commit, note, author, number, at)` — чистый конструктор с проверкой, что Commit принадлежит той же области. Событие `release_created`.
- Один Commit — не больше одной Release. `ErrCommitAlreadyReleased`.
- Release копирует содержимое Commit (для Project — `projectv0.ProjectFile`), поэтому не зависит от существования Commit'а, Draft'а или Library. Это важно для удаления Library и для будущей обрезки Commit'ов.
- `Origin` — enum без `Unknown` (ADR 0001): `commit` сейчас; `file` появится в 72_4 для Deploy file; `legacy` — для старых Library Release в 72_5/72_8.

## Хранение

- Таблица `engineering.project_releases` (Library Release остаётся в `engineering.library_releases`, 72_5):
  - `id uuid pk`, `project_id uuid not null references projects(id) on delete cascade`,
  - `number int not null`, `commit_number int null`, `commit_title text null`, `note text null`, `author text not null`, `created_at timestamptz not null`,
  - `origin text not null check (origin in ('commit','file'))` (значение `file` появится в 72_4),
  - `content jsonb not null`,
  - `unique (project_id, number)`, `unique (project_id, commit_number) where origin = 'commit'`.
- `engineering.project_release_library_pins (release_id uuid references engineering.project_releases(id) on delete cascade, library_id uuid, release_number int, primary key (release_id, library_id))`: пины содержимого Release, пишутся в транзакции создания. Нужны задаче 71; Release из файла (72_4) заполняет их из ProjectFile.
- Общая доменная модель `release.Release` (или `ProjectRelease` + общие VO `Number`, `Note`, `Author`, `Origin`) и общий `appdto.Release`; Library Release в 72_5 разделяет VO и форму ответа.
- Номер Release выдаётся `max(number)+1` в Project. Гонка — unique + повтор (`ErrReleaseNumberTaken`, как у текущего `LibraryService.Release`), либо advisory lock по Project. Выбрать одно и покрыть тестом.
- `commit_number`/`commit_title` обязательны для `origin='commit'`; для `origin='file'` (72_4) — nullable/заголовок файла.

## Application

- `ReleaseService` (Project; 72_5 расширит на Library):
  - `Create(ctx, projectID, commitNumber int, note, author string) (appdto.Release, error)`;
  - `List(ctx, projectID) ([]appdto.Release, error)` — новые первыми, без содержимого;
  - `Find(ctx, projectID, number int) (appdto.Release, error)`;
  - `UpdateNote(ctx, projectID, number int, note string) (appdto.Release, error)` — заметка не является содержимым, Version Draft не требуется; событие `release_note_updated`.
  - `ProjectFile(ctx, projectID, number int) (ProjectFileDownload, error)` — содержимое Release как файл (имя `<project>-release-<n>.json`).
- Проверки: Project жив и принадлежит Organization; Commit существует и принадлежит Project; Commit ещё не опубликован; заметка/автор — допустимые строки; номер > 0.
- Ошибки: `ErrCommitAlreadyReleased` (409 `commit_already_released`), `ErrReleaseNotFound` (404), `ErrNothingToRelease` больше не нужен (нет сравнения с Draft: Commit уже снимок). Неверный ввод — 400.
- `appdto.Release`: `number`, `commit_number`, `title` (из `commit_title`), `note`, `author`, `created_at`, `origin`, `content_counts`. Поля `running` нет: список Release — данные Engineering node, работающую Release UI определяет отдельным чтением `GET …/deployment` (72_4).
- `appdto.Commit.released` (из 72_2) становится true, когда у Commit'а есть Release; список Commit'ов перечитывается на `release_created`.
- `Create` не требует Version Draft и не читает Draft: Commit неизменяем. Это снимает текущую проверку Version Library в транзакции Release.

## REST (`contract/api/v0`, MINOR, `make schemas`)

Под Project (в 72_5 — под Library):

- `POST …/releases` `{commit, note?, author}` → 201 `ReleaseResponse`; 409 `commit_already_released`; 404 Commit/Project; 400.
- `GET …/releases` → 200 `{releases: [ReleaseResponse]}` (новые первыми; работает без Runtime node).
- `GET …/releases/{n}` → 200 `ReleaseResponse` + `content_counts`; 404.
- `PATCH …/releases/{n}` `{note}` → 200 `ReleaseResponse`; 400/404. Содержимое Release не меняется; заметка — метаданные, последняя запись побеждает.
- `GET …/projects/{id}/releases/{n}/projectfile` → 200 `application/json` attachment; 404.
- Событие `release_created` и `release_note_updated` — в поток Project (для Library — `library_released`/`library_release_note_updated`, 72_5); `ReloadTargetsFor` обновляет список Release.
- `apiclient`: `CreateRelease`, `Releases`, `Release`, `ReleaseProjectFile`.
- Старый маршрут `POST …/releases` у Library и его DTO пока не трогаются (72_5); у Project его раньше не было.

## UI (минимально; полный Project UI — 72_6)

- В диалоге истории сохранений у каждого Commit'а — кнопка **Сделать Release** (диалог: заметка, автор; Commit уже выбран). После успеха — toast и список Release.
- Диалог **Releases**: номер, заголовок Commit'а, заметка, автор, время; пока без Deploy и running-маркера (появятся в 72_4/72_6). У каждой строки — кнопка **Изменить заметку** (`PATCH …/releases/{n}`); содержимое не меняется.
- Кнопка доступна без Deploy и без Version; она не меняет Draft и не чистит историю.
- Если Commit уже опубликован — кнопка выключена с подсказкой или 409 с toast.

## Тесты

- Домен: `CreateRelease` из Commit, один Commit — одна Release, чужой Commit, пустая заметка (допустима), `WithNote` (событие `release_note_updated`), неверный автор, `Origin`.
- Репозиторий (testcontainers): номер 1/2, unique `(project_id, number)`, unique commit, CASCADE при удалении Project, содержимое ProjectFile сохраняется и читается, `commit_title` — снимок (переименование Commit после Release не меняет Release).
- Application/REST: create/list/find/projectfile, `PATCH` заметки, 409/404/400, события `release_created`/`release_note_updated`.
- UI (`uitest`): «Сделать Release» из истории сохранений, список Release, повторная публикация, редактирование заметки.
- Bruno: create/list/find/release projectfile; `requestBodies`; `seq` (Commit → Release).
- `make build`, `make test`, `make bruno_test`.

## Документы

- ADR 0021: Release — отдельный агрегат, копия Commit'а, один Commit — одна Release.
- `GLOSSARY.md`: Release (Project), Commit, Origin.
- `AGENTS.md`, `CHANGELOG.md`.

## Вне объёма

- Deploy Release и running-маркер — 72_4.
- Library Release из Commit и миграция старых Library Release — 72_5.
- Удаление Release и редактирование содержимого — не делается: Release неизменяем. Заметка — исключение (метаданные).

## Comments

- 2026-11-07: выделено из 72. Release копирует Commit, а не ссылается на него, чтобы пережить обрезку Commit'ов и удаление Library.
