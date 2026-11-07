# Миграция, growctl, Bruno и документы после Save/Release/Deploy

Status: ready-for-agent
Priority: 1
Blocked by: 72_1, 72_2, 72_3, 72_4, 72_5, 72_6, 72_7, 72_9

Слой C/E. Термины — `GLOSSARY.md` (Commit, Release, Deploy, Author). Часть 8 (закрывающая) из `72_save_release_deploy.md`.

## Задача

Свести модель Save → Commit → Release → Deploy в миграциях, инструментах и документации; убрать остатки EditLock; проверить обратную совместимость и полный прогон.

## Миграции и совместимость

- Порядок миграций (каждая часть добавила свою, здесь проверить цепочки и откаты):
  - `engineering`: `commits` + поля `drafts.last_commit_*` (72_2); `project_releases` (72_3); `library_releases.commit_number/author/origin` (72_5); `commit_library_pins`/`project_release_library_pins` (72_5).
  - `runtime`: `revisions` → `deployments` (72_9), `deployments.release_number` (72_4).
  - `public`: изменений нет.
- Существующие данные:
  - Library Release до задачи — `origin='legacy'`, `commit_number=NULL`, `author='system'`; содержимое `released_widget_types` не меняется.
  - Project без Commit'ов — `last_commit_number=0`, `last_commit_draft_version=0`; первый Save создаёт Commit 1.
  - Работающая Deployment до задачи — `release_number=NULL`; UI показывает «Работает Deployment N (не Release)». Backfill не делаем: текущий Draft может отличаться от Deployment, и синтетический Commit был бы ложью. Инженер может Save → Release → Deploy, когда захочет.
  - Deployment из файла — `release_number=NULL`; если файл деплоится после 72_4, создаётся Release с `origin='file'`.
- Проверить `make db_up`, `make full_restart`, три goose-цепочки, сид; миграционные тесты `migrations_test.go`; удаление Project/Library CASCADE; `released_widget_types` FK не сломан.
- Старые клиенты: удалённые edit-lock маршруты и `POST …/deploy` отвечают 404/410; `POST …/libraries/{id}/releases` с `version` — 400. Пока MAJOR = 0 это допустимо (ADR 0005), но отметить в CHANGELOG и обновить growctl/симулятор/Bruno. `devicelink` не затронут.

## growctl

- Убрать Check out/Check in и заголовок клиента из всех запросов; мутации шлют только Version.
- Новые команды (имена уточнить в реализации, синтаксис — по стилю `growctl`):
  - `growctl save <project|library> --title <title> [--author <name>]` → `POST …/commits`;
  - `growctl release <project|library> --commit <n> [--note <text>] [--author <name>]` → `POST …/releases`;
  - `growctl deploy <project> --release <n> [--author <name>]` → `POST …/releases/{n}/deploy`;
  - `growctl commits <project|library>` и `growctl releases <project|library>` — списки (для человека/скриптов).
- `growctl apply` по-прежнему правит Draft; чтобы изменения стали Release/Deploy, нужны `save` → `release` → `deploy`. Описать workflow в README/help.
- Манифесты `contract/manifest/v0` не получают Commit/Release как kind: это императивные акты, не декларативное состояние. Если понадобится декларативный `release`, bump manifest MINOR и `make schemas` — решить в реализации, по умолчанию не добавлять.
- Автор: флаг `--author`, иначе имя пользователя ОС/`GrowSCADA_AUTHOR`, иначе ошибка (пустой автор недопустим). Обновить e2e `growctl_e2e_test.go`.

## Bruno

- Удалить setup/teardown-запросы EditLock и `X-GrowSCADA-Client` из всех запросов.
- Project flow: создать Project → править Draft → `POST …/commits` (Save) → `POST …/releases` (Release) → `POST …/releases/{n}/deploy` → `GET …/deployment` → `PATCH …/value`.
- Library flow: править WidgetTypes → Save → Release → Project pin → Project Save → Release → Deploy.
- Обновить `post_deploy` → `post_release_deploy`; добавить 409 повторного Deploy, 404, 410 старого `POST …/deploy`; обновить `post_library_release` (тело `{commit, note, author}`), `get_library_releases`, `get_project_releases`, `post_commit`, `get_commits`, `patch_commit_title`.
- `requestBodies` в тесте `TestBrunoCollection`: каждый новый маршрут с телом/параметрами; `invalid-body` для отвергаемых; `manual` для SSE.
- `seq`: чтения → Commit → Release → Deploy → правки → удаления; ID/номера из ответов через `bru.setVar`; environment placeholders.
- `make bruno_test` на чистой БД; коллекция симулятора деплоит Release перед запуском драйвера.

## Документы

- **ADR 0021** «Save создаёт Commit; Release — публикация Commit'а; Deploy активирует Project Release»: уровни, неизменяемость, один Commit — одна Release, копирование содержимого, `release_number` в Deployment, отсутствие Backfill, Discard к последнему Commit'у, отдельные Revert к Deployment/Release.
- **ADR 0022** «Draft правится без EditLock; конкурентность — Version, автор — объявленное имя»: отменяет ADR 0015, поправляет ADR 0016, фиксирует общий undo и ограничения до User/прав.
- Поправки: ADR 0007 (Library Release из Commit; Commit/Release пинят Library), 0012 (закрепление/защита), 0013 (Deploy Release, не Draft; Deployment вместо Revision), 0014 (Rollback сохраняет Release), 0016 (нет EditLock в Draft), 0017 (Save/Release/Deploy не чистят стек; только Discard), 0018 (Deploy file создаёт Release), 0019 (Journal `deployment` + `release_number`), 0020 (Operation читает Deployment с номером Release).
- **GLOSSARY.md**: добавить Commit, Author, Project Release, Deployment; Revision — прежнее имя Deployment (историческое); обновить Release, Deploy, Draft, Discard; удалить EditLock, Check out, Check in, Client или переопределить Client как автора.
- **AGENTS.md**: заменить все разделы про EditLock/Check in/out на Edit/Save; описать Commit/Release/Deploy, маршруты, Project/Library UI, growctl, Bruno; обновить `Makefile`-описания, если workflow симулятора требует Deploy Release.
- **CHANGELOG.md**: крупный раздел; удалённые маршруты/события, новые маршруты, изменения формы Library Release, миграции, обратная совместимость.
- `make schemas`: `api` (Commit/Release/Deploy, удалён EditLock), `record` (`release_number`); проверить `schemas/frozen.sha256` и `contract/*/SchemaVersion`.
- `codegraph sync` после слияния (правило 10 AGENTS.md), если индекс есть.

## Приёмка

- `make build`, `make test`, `make bruno_test` проходят на чистой БД.
- `make db_up`, `make full_restart` — миграции применяются в правильном порядке; сид работает; старые Library Release видны как `legacy`.
- Ручной обход PWA (правило 9 AGENTS.md): Project Save → Release → Deploy → Operation видит новую конфигурацию; Library Save → Release → Project закрепляет и видит WidgetType; Discard; два таба; консоль без ошибок.
- В коде/доках не осталось `editlock`, `edit_lock`, `CheckOut`, `CheckIn`, `TakeOver`, `X-GrowSCADA-Client` (кроме исторических ADR/CHANGELOG).
- Нет висячих `POST …/deploy`, `deploy/preview`, `DraftDiffers`-признаков, старых тел Library Release в Bruno/growctl.
- Старые Deployment без Release отображаются как «Deployment N (не Release)», а не ломают список.

## Риски и заметки

- **Объём хранения**: каждый Save хранит полный ProjectFile/WidgetTypes. Save — явный, не каждое действие; при росте объёма — retention отдельной задачей. Не обрезать Commit'ы, на которые ссылаются Release (Release копирует содержимое, так что можно).
- **Library с историей** (гибрид 71): живой пин и работающая Deployment блокируют удаление; старая история — нет. Восстановление пинов из старого Commit/Deployment пропускает исчезнувшую Release с предупреждением; UI показывает, что в истории не хватает Release.
- **Общий undo** без EditLock: два инженера могут отменять чужие DraftChange. До задач 49/50 это принято; не пытаться чинить здесь.
- **Старые клиенты**: growctl/симулятор/Bruno обновлены; сторонние клиенты получат 400/404/410. Пока MAJOR = 0 — допустимо, зафиксировать в CHANGELOG.
- **Backfill не делаем**: синтетические Commit/Release из текущей Deployment исказили бы историю; UI честно показывает legacy/не-Release.

## Comments

- 2026-11-07: закрывающая часть 72. Сводит миграции, инструменты, Bruno и документы; проверяет полный сценарий Save → Release → Deploy.
