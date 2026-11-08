# Project UI: Save, история сохранений, Release и Deploy по Release

Status: ready-for-agent
Priority: 1
Blocked by: 72_4

Слой E. Термины — `GLOSSARY.md` (Commit, Release, Deploy, Author). Часть 6 из `72_save_release_deploy.md`.

## Задача

Project-режим переходит на редакторскую модель: Save, история сохранений, Release и выбор любой Release для Deploy. Строка EditLock, Check in/out и read-only без замка уже удалены в 72_1. Старый Deploy Draft и Rollback из UI убираются.

## Шапка Project

- **Save** — основная кнопка. Выключена, когда Version Draft равна `draft_version` последнего Commit (нечего сохранять), с подсказкой «Нет изменений с последнего сохранения».
- Индикатор **есть несохранённые изменения** (Version Draft > `draft_version` последнего Commit) — точка/подпись рядом с Save, не блокирует правку.
- **История сохранений** (Commit'ы) — диалог.
- **Releases** — диалог; на кнопке может быть счётчик/бейдж работающей Release.
- **Deploy** — открывает список Releases (если Releases нет — подсказка «Сначала сохраните и опубликуйте Release»).
- **Project file…** — остаётся: Download Draft, Open a file…, Deploy a file…; без EditLock, открытие файла создаёт Draft, который затем можно Save.
- **Discard changes** — сброс Draft к последнему Commit'у (отменить несохранённое), подтверждение; нет Commit'ов — 409 `nothing_to_discard` с toast.
- **Revert to deployed** — сброс Draft к работающей Deployment (прежняя семантика Discard), подтверждение; нет Deployment — 409. После — перечитать Draft/стек и preview; кнопка отделена от Discard, чтобы не путать «отменить несохранённое» и «вернуться к production».
- Имя автора: `author:name` в LocalStorage; первый Save/Release/Deploy спрашивает имя (диалог), дальше подставляет. Показывать «Автор: <name>» или маленькую кнопку смены; это не замок и не блокирует других.
- Undo/Redo и `Ctrl+Z`/`Ctrl+Shift+Z` остаются; они работают по Draft и не зависят от Save. `Ctrl+S`/`Cmd+S` — Save (если есть изменения; иначе ничего).

## Save

- Диалог Save: заголовок (обязателен, по умолчанию «Сохранение DD.MM.YYYY HH:MM»), автор (если ещё не задан); кнопка Save.
- Запрос `POST …/projects/{id}/commits` `{version, title, author}`.
- Успех: toast, история сохранений перечитывается, индикатор несохранённых изменений гаснет. Version Draft не меняется; `draftVersions` не сбрасывать.
- 409 `nothing_to_save` — toast «Нет изменений»; 409 `draft_not_saveable` — toast/диалог с причиной (Draft нельзя собрать); 409 `draft_conflict` — перечитать Draft, стек и историю, toast.
- Save не очищает undo/redo и не меняет Release/Deploy.

## История сохранений

- Строка: номер, заголовок, автор, время, признак «опубликовано» (есть Release), счётчики содержимого (Scenes/Widgets/Tags/Devices/Environments).
- Действия: **Переименовать** (отдельная кнопка/иконка, `PATCH …/commits/{n}` `{title}`; ошибки — toast), **Скачать ProjectFile** (`GET …/commits/{n}/projectfile`), **Сделать Release** (если ещё не опубликовано).
- «Сделать Release»: диалог с заметкой (необязательна) и автором; `POST …/projects/{id}/releases` `{commit, note, author}`; после успеха — история помечает Commit опубликованным, список Release перечитывается.
- Список обновляется на `commit_created`, `commit_title_updated`, `release_created` и `EventResync`; для чужого Project — `eventlog.ForProject` (как остальные события).

## Releases и Deploy

- Строка Release: номер, заголовок Commit'а, заметка, автор, время, счётчики; кнопка **Deploy**; скачать ProjectFile Release; **Изменить заметку** (`PATCH …/releases/{n}`, содержимое не меняется). Список `GET …/releases` читается независимо от Runtime; маркер **Работает** ставится, если `Deployment.release_number` из `GET …/deployment` совпал с номером Release. Если чтение Deployment упало — маркер «не удалось проверить», список остаётся.
- Deploy: подтверждение «Сделать Release N работающей? Operation переключится на неё»; `POST …/projects/{id}/releases/{n}/deploy`; успех — toast, перечитать список Release и `GET …/deployment`; событие `deployment_created` обновляет другие вкладки.
- 409 `nothing_to_deploy` — toast «Эта Release уже работает»; 404 — перечитать список.
- Если работает Deployment без Release (`release_number = nil`): строка «Работает Deployment N (не Release)»; список Release без пометки; Deploy любой Release заменяет её.
- **История деплоев**: read-only список Deployment (`GET …/deployments`) с номером, временем, причиной, ссылкой на Release; скачивание документа (`GET …/deployments/{n}/projectfile`). Кнопки Rollback нет — Rollback заменён Deploy соответствующей Release. Если Release у старой Deployment нет, документ всё равно доступен.
- Старый диалог Deploy Draft (preview `current_deployment`/`unchanged`/`dropped_widgets`) удалить. `GET …/deploy/preview` больше не вызывается.

## Удалить из UI

- `Check out`/`Check in`/`Take over`, строка замка, read-only без замка, `editlock:*`, `GiveBackDraft`, `waitMoves`-перед-Check-in (ожидание серии стрелок перед записью остаётся, но без Check in).
- Кнопку **Rollback** и её подтверждение (Rollback остаётся в API/History).
- Бейдж «Deployment N · deployed at T» заменить на «Release N · работает» (плюс «Deployment N» в истории деплоев).
- Диалог Deploy Draft и его preview.

## Технические правила

- Все чтения/записи — через `uiutil.Call`/`Do`/`Load`/`Versions`; одна Version Draft на Project (`draftVersions`), версия Release/Commit — свои номера, не Version Draft.
- 409 `draft_conflict` — общий обработчик: перечитать Draft, стек, историю сохранений; не показывать «замок занят».
- Долгие операции (Save собирает ProjectFile) — индикатор на кнопке, повторный клик не шлёт второй запрос.
- События Project-потока: `commit_*`, `release_*`, `deployment_created`, `tag_*`, `scene_*` и т.д.; чужие события отбрасываются `eventlog.ForProject`.
- После Deploy Operation переключается сама по `deployment_created`; Project не перезагружает Operation.

## Тесты

- `uitest` (go-app TestEngine + `servertest.StartAPI`):
  - Save: диалог, создание Commit'а, индикатор, история, повторный Save без изменений;
  - переименование заголовка;
  - Release из Commit'а, повторная публикация — toast;
  - Deploy Release, пометка «Работает» переезжает, повторный Deploy — toast;
  - Deployment без Release — строка;
  - 409 `draft_conflict` — перечитывание;
  - Project file: download/open/deploy без замка;
  - Discard changes — к последнему Commit'у; без Commit'ов — toast; Revert to deployed — к работающей Deployment, после него preview `unchanged`.
- Ручная проверка через chrome-devtools: два таба, Save/Release/Deploy, консоль без ошибок.
- `make build`, `make test`, `make bruno_test`.

## Документы

- `AGENTS.md`: описание Project-режима (Save, история, Releases, Deploy, без EditLock).
- `CHANGELOG.md`.

## Вне объёма

- Library UI — 72_7.
- growctl/Bruno — 72_8 (здесь только запросы, которые использует UI).
- Права и User — 49/50.

## Comments

- 2026-11-07: выделено из 72. История деплоев остаётся read-only; Rollback как отдельная кнопка уходит, его роль выполняет Deploy любой Release.
