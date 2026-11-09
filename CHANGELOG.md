# growscada [CHANGELOG](https://keepachangelog.com/en/1.1.0/)

## [0.0.86] - 2026-11-09

### Added

- **Checkpoint: неизменяемый снимок операционного состояния на позицию Journal — запись и её писатели** (задача 44_1; ADR 0023)
  - `contract/record/v0` **0.5**: `Checkpoint` (встраивает `Header`; `seq`, `project_id`, `timestamp`, `deployment`, `tag_states`) и `CheckpointTagState` (`tag_id`, `value`, `quality`, `seq`); Deployment — тот же payload, что у `deployment`-записи Journal, документ не копируется; `make schemas`
  - Миграция `runtime/20261111000000_checkpoints`: `runtime.checkpoints` с ключом `(project_id, seq)`, `taken_at`, `schema_version`, jsonb-`payload` `{deployment, tag_states}`, каскад с Project и индекс `(project_id, seq DESC)`
  - `postgres/runtime/checkpoint_codec.go` — кодек с `schema_version` в строке; `NewCheckpointRepositoryPostgres` — `Nearest` (последний Checkpoint ≤ позиции; неживой Project — `project.ErrProjectNotFound`), `DueProjects` (порог считается записями Journal, только живые Project'ы) и `Snapshot` (одна `REPEATABLE READ`-транзакция, вставка `ON CONFLICT DO NOTHING`)
  - Deploy Release, Deploy файла и Rollback пишут Checkpoint в своей транзакции — после `refreshCopies`, на `seq` записи `deployment`; `insertDeployment` теперь возвращает `(number, seq)`, а неудавшаяся Deployment снимка не оставляет
  - `application/runtime.CheckpointService`: `Run` (тикер `CheckpointScanEvery = time.Minute`) и `RunOnce` для детерминированных тестов; `CheckpointEveryEntries = 10 000`; ошибка снимка логируется и повторяется следующим проходом, outbox и policy не участвуют. Запуск в `cmd/combined_server/main.go` рядом с dispatcher и policy runner; `servertest` тикер не поднимает
  - Тесты: кодек (round-trip, отказ чужой `schema_version`, пустой `tag_states`), интеграционные (`postgres/runtime`: три пути Deployment пишут Checkpoint на позицию записи `deployment`, состояния снимка совпадают с `runtime.tag_states`, `Nearest` до/на/после, `DueProjects` до и после порога, повторный `Snapshot` не дублирует, удаление Project удаляет Checkpoint каскадом), `CheckpointService.RunOnce` (снимок только созревшего Project, повторный проход ничего не добавляет, ошибка одного Project не останавливает другие)
  - `?at=` у `GET …/operation`, `apiclient` и Bruno — часть 2 (44_2); текущий read-model Operation и REST не менялись
  - Проверки: `make build`, `make test`; `make bruno_test` не требуется — REST и UI не менялись

## [0.0.85] - 2026-11-09

### Changed

- **Модель Save → Commit → Release → Deploy сведена и закрыта; EditLock убран из инструментов, Bruno и документов** (задача 72_8, закрывающая часть 72; ADR 0021/0022)
  - Уровни: Draft правится сразу DraftChange'ами под Version своего содержимого (ADR 0016/0017/0022); **Save** фиксирует весь Draft как неизменяемый нумерованный **Commit** (Project'а или Library); **Release** публикует один Commit — **Project Release** (72_3) или **Library Release** (72_5) — неизменяемой нумерованной копией содержимого, заголовка на момент публикации, заметки, автора и времени; **Deploy** делает Project Release работающей **Deployment** (72_4). Один Commit публикуется не более одной Release
  - **Discard** сбрасывает Draft к последнему Commit'у, **Revert to deployed** — к работающей Deployment Project'а, **Revert to latest Release** — к последней Release Library: каждая одной транзакцией, каждая чистит стек. **Save, Release и Deploy стек не чистят** и Draft не меняют (поправка ADR 0017); Save не пишет DraftChange и не поднимает Version
  - Backfill не делался: работавшая до модели Deployment остаётся с `release_number = NULL` и показывается «Deployment N (not a Release)», синтетических Commit/Release нет. Library Release до Commit'ов — `origin = 'legacy'`, `commit_number = NULL`, `author = 'system'`; `released_widget_types` не меняется. Инженер сам делает Save → Release → Deploy, когда хочет запустить текущий Draft
  - **Миграции**: `engineering/20261108000000_commits` (+`drafts.last_commit_*`, `commit_library_pins`), `20261109000000_project_releases` (+`project_release_library_pins`), `20261109000001_release_commit_provenance`, `20261110000000_library_release_commit`; `runtime/20261107000000_deployments` (`revisions` → `deployments`, `n` → `number`, Journal `revision` → `deployment`), `20261110000000_deployment_release_number`; `public` не менялся. Цепочки и откаты последних миграций проверены на живой БД (`goose down`/`up`), полный `make db_up` применяет три цепочки и сид; legacy-Release и старые Deployment после отката/наката читаются как положено
  - **REST/контракт**: `api/v0` **0.30**, `record/v0` **0.4** (`release_number`), `manifest/v0` без изменений. Добавлены `…/commits` (Save/список/просмотр/переименование/ProjectFile), `…/releases` (Release/список/просмотр/заметка/ProjectFile), `POST …/releases/{n}/deploy`, `GET …/deployment`, `GET …/tag-states`; `POST …/deploy` отвечает **410 Gone**, `GET …/deploy/preview` удалён, маршруты `/edit-lock` на Project и Library (по четыре) — **410 Gone**, старые `/revision(s)…` — **410 Gone** (72_9). Library Release принимает `{commit, note?, author}` и возвращает `commit_number`/`title`/`author`/`origin`/`content_counts`; `PATCH …/libraries/{id}/releases/{n}` меняет заметку без Version
  - **События**: `commit_created`, `commit_title_updated`, `release_created`, `release_note_updated`, `deployment_created` (с `release_number`); `edit_lock_*` удалены (72_1), `revision_created` переименован в `deployment_created` (72_9). Journal-запись `deployment` несёт `release_number`, старые записи `revision` читаются и нормализуются
  - **growctl**: `apply`/`diff`/`delete` по-прежнему правят только Draft; дальше `save <project|library> --title [--author]`, `release <project|library> --commit <n> [--note] [--author]`, `deploy <project> --release <n>`, списки `commits` и `releases` (Project или Library по имени; `--library` при совпадении). Старый `release <library>` с неявным сохранением убран. Автор — `--author`, иначе `GROWSCADA_AUTHOR`, иначе имя пользователя ОС; пустой автор — ошибка. Check out/Check in и заголовок клиента отсутствуют, мутации шлют только Version; справка `growctl` и e2e описывают порядок
  - **Bruno**: потоки Project (create → правка → Commit → Release → Deploy → `GET …/deployment` → `PATCH …/value`) и Library (WidgetTypes → Commit → Release → пин → Commit/Release/Deploy проекта) собраны на `bru.setVar`; есть 409 повторного Save/Release/Deploy, 404, 410 старых маршрутов; setup/teardown EditLock и `X-GrowSCADA-Client` отсутствуют, `requestBodies`/`invalid-body`/`manual` актуальны; коллекция симулятора деплоит Release перед драйвером
  - **Документы**: ADR 0021 (полная модель: уровни, неизменяемость, один Commit — одна Release, копирование, `release_number`, отсутствие Backfill, Discard к Commit'у, отдельные Revert) и ADR 0022 (автор — объявленное имя, Deploy/Release не читают Draft, стек чистит только сброс); поправки/уточнения ADR 0007, 0012, 0013, 0014, 0016, 0017, 0018, 0019, 0020; `GLOSSARY.md` — Commit, Save, Release, Project Release, Deployment, Author, legacy Release, Deploy; убраны EditLock/Check out/Check in и дубль Client; `AGENTS.md` — growctl, Bruno, Makefile/симулятор, словарь; `make schemas` — api 0.30, record 0.4, `frozen.sha256`
  - **Обратная совместимость**: пока MAJOR = 0 (ADR 0005) старый клиент получит 400/404/410 на изменённые маршруты — это зафиксировано здесь; сторонние клиенты должны перейти на Commit/Release/Deploy. `devicelink` не менялся
  - **Проверки**: `make build`, `make test`, `make bruno_test` на чистой БД; ручной обход PWA (Project Save → Release → Deploy → Operation, Library Save → Release → пин, Discard, две вкладки)

## [0.0.84] - 2026-11-09

### Changed

- **Library UI: редакторская модель Save → Commit → Release доведена до описания задачи 72_7** (task 72_7; ADR 0016/0017/0021)
  - Шапка Library: **Save** — диалог с заголовком (по умолчанию «Save DD.MM.YYYY HH:MM») и автором; успех — toast «Commit N saved», кнопка показывает «Saving…» и выключена без изменений с подсказкой «Nothing to save since the last save»; `Ctrl+S`/`Cmd+S` открывает диалог (и перехватывает браузерное «Сохранить страницу»), при отсутствии изменений не делает ничего; 409 `draft_conflict` перечитывает Draft, стек, список WidgetTypes и историю. Автор хранится в `author:name` и показывается строкой «Author: <name>» с кнопкой смены — это не замок
  - Индикатор несохранённых изменений: Discard этой вкладки и Revert к Release последнего Commit'а считаются «чистыми», хотя Version Draft вырос; в списке Library у каждой строки — состояние «unsaved changes», «unpublished commit» или «published» (легаси-Release — `legacy`)
  - **История**: у каждого Commit'а — просмотр его WidgetTypes только для чтения (`GET …/commits/{n}`, снимок Commit'а, не Draft); **Make Release** открывает диалог (заметка необязательна, автор из LocalStorage) вместо браузерного prompt, успех — toast «Release N published», повторная публикация того же Commit'а — toast 409; у опубликованного Commit'а кнопка выключена
  - **Releases**: строка списка показывает номер, Commit (или `legacy`), заметку, автора, время и число WidgetTypes; выбор строки открывает копии только для чтения, заметка правится отдельной кнопкой; Discard — к последнему Commit'у, Revert — к последней Release, обе с подтверждением и перечитыванием Draft/стека/истории
  - События: `commit_*`, `library_released` и `library_release_note_updated` другой Library не перезагружают выбранную; `widget_type_*` перечитывают Draft и признак неопубликованных изменений, но не Releases с содержимым; `EventResync` перечитывает шапку, историю и Releases
  - Документы: `AGENTS.md` (Library-режим), CHANGELOG
  - Тесты `uitest`: `Ctrl+S` с изменениями и без, toast Save и конфликт с перечитыванием, Discard к Commit'у и индикатор, Revert к последней Release, публикация и повторная публикация (toast 409), просмотр Commit'а только для чтения, три состояния списка, счётчики строки Release, события чужой Library и `widget_type_updated` без перечитывания Releases

## [0.0.83] - 2026-11-08

### Changed

- **Project UI: редакторская модель Save → Release → Deploy доведена до описания задачи 72_6** (task 72_6; ADR 0021/0022)
  - Шапка Project: **Save** — основная кнопка, выключена без изменений с подсказкой «Nothing to save since the last save», во время запроса показывает «Saving…» и второй клик ничего не шлёт; успех — toast «Commit N saved», индикатор несохранённых изменений гаснет; 409 `draft_conflict` перечитывает Draft, стек и историю. `Ctrl+S`/`Cmd+S` открывает диалог Save (и перехватывает браузерное «Сохранить страницу»), при отсутствии изменений не делает ничего; из поля ввода тоже работает
  - Автор: имя хранится в `author:name`, показывается в шапке («Author: <name>») с кнопкой смены (браузерный prompt); диалоги Save и Release подставляют его, первый диалог спрашивает. Это не замок и никого не блокирует
  - История: у опубликованного Commit'а — пометка «released»; Save/Release/Deploy по-прежнему перечитывают список, `commit_*`/`release_*`/resync — тоже
  - Релиз/Deploy: у кнопки **Releases** — бейдж номера работающей Release; шапка показывает «Release N · running» (или «Deployment N · not a Release», «Not deployed», «Deployment unknown»); пустой список Release подсказывает сохранить Draft и опубликовать Release; 409 `nothing_to_deploy` — отдельный toast «This Release is running already»; 404 перечитывает список
  - **Discard** переименован в **Discard changes**, **Revert** — в **Revert to deployed**: обе операции после успеха перечитывают содержимое Draft, Version, стек, историю и работающую Deployment (события делают то же, но сброс не ждёт их); Discard, сделанный этой вкладкой, считается «чистым» для индикатора несохранённых изменений, хотя Version Draft вырос, а Revert — когда работающая Deployment пришла из Release последнего Commit'а
  - Документы: `AGENTS.md` (Project-режим), CHANGELOG
  - Тесты `uitest`: toast Save и «нечего сохранять», `Ctrl+S` с изменениями и без, строка автора, пометка опубликованного Commit'а, пустой список Release, Discard changes к последнему Commit'у и 409 без Commit'ов, Revert to deployed к работающей Deployment и 409 без неё, новые подписи кнопок; `uiutil.ShortcutKey` покрывает `Ctrl+S`

## [0.0.82] - 2026-11-08

### Added

- **Library Release из Commit; Revert to latest Release; провенанс в пинах** (задача 72_5; ADR 0007/0012/0016/0017/0021)
  - Домен `library`: `LibraryRelease` получает `CommitNumber() *int` (nil для legacy-Release), `Author`, `Origin` (`commit`/`legacy`); `CreateRelease(lib, number, commitNumber, note, author, at, types)` публикует Commit и пишет `library_released`; `ReconstituteRelease` проверяет провенанс; `WithNote` меняет только заметку и пишет `library_release_note_updated` (новый `EventType` и имя в контракте); `ErrCommitAlreadyReleased`; `ErrNothingToRelease`, `DraftDiffers` и `WithNumber` удалены; `ReleaseStateOf` считает `UnsavedChanges` (Version Draft выше зафиксированной последним Commit) и `UnpublishedChanges` (номер последнего Commit выше опубликованного последней Release)
  - БД (`engineering/20261110000000_library_release_commit`): `library_releases.commit_number`/`author`/`origin` (default `system`/`legacy`), check `origin IN ('commit','legacy')` и провенанса, частичный unique `(library_id, commit_number) where origin='commit'`; `released_widget_types` не меняется
  - Репозиторий: `Save` без Version Draft, проверяет живую Library, пишет провенанс и копии, различает `ErrReleaseNumberTaken` и `ErrCommitAlreadyReleased`, `UpdateNote` (с событием), `ReleasedCommits`; `session.ReplaceWidgetTypes` принимает копии WidgetTypes
  - Application: `LibraryService.Release` публикует Commit (читы `FindCommit`, копии, номер + retry, без Draft Version и без очистки стека); `FindReleases`/`FindRelease` несут текущий заголовок Commit'а и `content_counts`; `UpdateReleaseNote`; `Discard` — к последнему Commit (409 `nothing_to_discard`), новый `RevertToLatestRelease` — к последней Release (409 `NoRelease`); `appdto.Library` — `LatestCommit`, `HasUnsavedChanges`, `HasUnpublishedChanges`; `LibraryPin` несёт провенанс Release. `CommitService` помечает опубликованные Commit'ы Library через `ReleasedCommits`
  - REST (контракт `api/v0` **0.30**, MINOR): `POST …/libraries/{id}/releases` `{commit, note?, author}` → 201 (409 `commit_already_released`, 404 Commit/Library, 400); `GET …/libraries/{id}/releases` с `commit_number`, `title`, `author`, `origin`, `note`, `content_counts`; `PATCH …/libraries/{id}/releases/{n}` `{note}` → 200 (без Version); `POST …/libraries/{id}/revert-to-release` → 200 `{version}`; `POST …/discard` — к последнему Commit; `LibraryPinResponse` — `commit_number`, `title`, `author`, `origin`, `note`; Library DTO и пины опубликованы в схеме 0.30 (`Roots` теста схемы); `apiclient` (`Release` без Version, `UpdateReleaseNote`, `RevertToRelease`), `growctl release` сохраняет Commit и публикует его, `libraries` печатает commit/saved/unpublished
  - UI (Library): у Commit'а в History — **Make Release** (заметка, автор из LocalStorage; у опубликованного — «Released»), список Release показывает Commit/`legacy`, автора и заметку с редактированием заметки, статус — «unpublished commits», Discard — к Commit, **Revert** — к последней Release; UI (Project): у пина — провенанс Release (Commit, заголовок, автор, заметка/`legacy`); `eventlog` знает `library_release_note_updated` и перечитывает Release/Commit'ы на `library_released`
  - Документы: ADR 0007/0012/0016/0017/0021 (поправки task 72_5), `GLOSSARY.md` (Library Release, Legacy Release, Discard, Revert to latest Release, Release), `AGENTS.md`, CHANGELOG
  - Bruno: Commit перед Release, провенанс в ответе и пинах, 409 повторной публикации Commit'а, `PATCH` заметки, `revert-to-release`, legacy `origin` в сиде, Commit для cleanup-сценария; `requestBodies` пополнены
  - Тесты: домен (Release из Commit, провенанс, `WithNote`, состояния), репозиторий (провенанс, unique commit, legacy, `released_widget_types`, `ReleasedCommits`), application (публикация Commit'а, 404/409, заметка, Discard к Commit, Revert к Release, пины с провенансом), REST, uitest (кнопка Release, legacy-строка, провенанс пина), apiclient

## [0.0.81] - 2026-11-08

### Added

- **Deploy Project Release, работающая Deployment и Rollback** (задача 72_4; ADR 0013/0014/0018/0019/0020/0021)
  - Runtime node: миграция `runtime/20261110000000_deployment_release_number` — `runtime.deployments.release_number` (nullable, `> 0`, индекс `(project_id, release_number)`; старые Deployment и Deployment из файла — `NULL`). `runtime.Deployment`/`DeploymentSummary` получают `ReleaseNumber *int`; `CreateDeployment` принимает `releaseNumber *int`; `DeployRelease` — `cause=deploy`, `release_number=n`, `DeployFile` — `cause=file`, `nil`; повторный Deploy той же работающей Release — `runtime.ErrNothingToDeploy` (409) под тем же advisory lock, что нумерация, поэтому два конкурентных Deploy одной Release дают одну Deployment. `Rollback` копирует `release_number` целевой Deployment (файловая остаётся `nil`). `FindCurrent`/`FindAll`/`FindByNumber` отдают номер Release. `deployment_created` получает `release_number`; запись `deployment` в `contract/record/v0` — тоже (MINOR, старые записи декодируются как `nil`)
  - Application: `ReleaseService.Deploy` (Release живого Project — `RuntimeNode.DeployRelease`, 409 `nothing_to_deploy`, 404 `ErrReleaseNotFound`; `List` не ходит в Runtime), `ReleaseService.CreateFromFile` (валидирует файл тем же валидатором, публикует Release с origin `file` без Commit, заголовок `?title=`/«File», автор `?author=`/`file`, затем деплоит; неудавшийся Deploy оставляет Release для повтора). Старый `DeployService`/`Preview`/`DeployState` удалены. `DiscardService.Discard` теперь сбрасывает Draft к последнему Commit (409 `nothing_to_discard` без Commit), новый `Revert` — к работающей Deployment (409); обе операции — одна транзакция, чистят стек, не трогают Commit/Release/Deployment
  - REST (контракт `api/v0` **0.29**, MINOR): `POST …/projects/{id}/releases/{n}/deploy` → 201 `{number, deployed_at}` (409/404); `POST …/deploy` отвечает 410 Gone с подсказкой Save → Release → Deploy; `GET …/deploy/preview` удалён; `POST …/projectfile/deploy` использует `CreateFromFile` (`?author=&title=`); `DeploymentResponse`/`DeploymentSummaryResponse` получают `release_number`; `Event` — `release_number` у `deployment_created`. `apiclient`: `DeployRelease`, `DeploymentState.ReleaseNumber`, `RevertToDeployed`; `Deploy`/`DeployPreview` удалены. `make schemas` (api 0.29, record 0.4)
  - UI (Project): кнопка Deploy в шапке открывает список Release; у неработающей Release — Deploy с подтверждением, у работающей — пометка Running; Deployment без Release показывает «Running Deployment N (not a Release)»; после успеха toast и перечитывание Release + `GET …/deployment`, событие `deployment_created` делает то же в других вкладках; старый диалог Deploy Draft и кнопка Rollback в Project убраны (Rollback остаётся в API/History), Discard сбрасывает к последнему Commit, добавлена Revert
  - Документы: GLOSSARY (Deploy, Deploy Release, Revert to deployed, Discard, Deployment, Rollback, Deploy a file), ADR 0013/0014/0018/0019/0020 (поправки task 72_4), AGENTS.md, CHANGELOG
  - Bruno: поток Save → Release → Deploy вместо Deploy Draft; проверки 409 повторного Deploy, 404, 410 старого маршрута; Revert вместо Discard для работающей Deployment с чужими пинами; файловый Deploy создаёт Release; `requestBodies` пополнены; Docker Compose стенда публикует Commit/Release и деплоит Release перед симулятором
  - Тесты: Runtime (миграция и `release_number`, повторный/конкурентный Deploy, файловая Deployment без Release, Rollback сохраняет Release, Journal), Engineering (Deploy Release 409/404, `List` без Runtime, Release из файла, Discard к Commit, Revert, 410), REST/UI (список Release, Running, Deployment без Release, toast, событие `deployment_created`), Operation (Player переживает новое поле)

## [0.0.80] - 2026-11-08

### Added

- **Удаление Library с историей: работающая Deployment блокирует, Discard деградирует** (задача 71; ADR 0007/0012/0021)
  - Runtime node: `DeploymentRepository.DeploymentNamesLibrary` / `DeploymentService.DeploymentNamesLibrary` — «называет ли работающая Deployment любого живого Project'а Release этой Library». Читает `runtime.deployments` (последняя Deployment каждого живого Project'а) и `library_pins` её документа через `jsonb_array_elements`; таблица пинов Deployment не заводится. Порт `RuntimeNode.DeploymentNamesLibrary`, реализация `inprocess`
  - Application: `LibraryService.DeleteLibraryByID` спрашивает Runtime node и отвечает `409 library` с `ErrLibraryInRunningDeployment`, если Release библиотеки названа работающей Deployment; старые Commit/Project Release и старые Deployment удаление не блокируют (их документы самодостаточны). Живой `library_pins` блокирует как раньше (`ErrLibraryPinned`). `NewLibraryCleanupPolicy` не перепроверяет Deployment: помеченная Library уже невидима API, а Deployment из файла, появившаяся позже, деградирует в предупреждение при Discard
  - Application: `DiscardService.Discard` вместо отказа `deployment_not_discardable` пропускает пины работающей Deployment, чьей Library или Release нет на сервере: пин, WidgetTypes этой Library и Widgets из них не восстанавливаются, остальное содержимое — как есть; ответ несёт `warnings` (`library_id`, `release`, `dropped_widgets`), Draft снова доступен для записи. Общий помощник `missingLibraryReleases`/`dropMissingLibraryReleases` (`application/library_pins_of_deployment.go`)
  - `DeployService.Preview` добавляет `missing_releases`: пины работающей Deployment, которых нет на этом сервере (для Project-режима — предупреждение «в истории не хватает Release»)
  - REST (контракт `api/v0` **0.28**, MINOR): `DiscardProjectResponse` становится структурой `{version, warnings?}`, новый `DiscardWarningResponse`; `DeployPreviewResponse` получает `missing_releases` (`{library_id, release}`); `DELETE /libraries/{id}` отвечает 409 и для Library, названной работающей Deployment. `make schemas`
  - UI (Project): в шапке и в диалоге Deploy — предупреждение о нехватке Library Release у работающей Deployment; после Discard — toast со списком пропущенных Release и числом удалённых Widgets. UI (Library): подтверждение удаления предупреждает, что Commit/Release прошлого сохранят ссылку на удалённую Library, а работающая Deployment удаление запрещает
  - Документы: ADR 0007 (поправка: работающая Deployment держит Library), ADR 0012 (поправка: проверка при удалении, политика не ретраится), ADR 0021 (пины Commit/Release не блокируют удаление), `GLOSSARY.md` (Library Release, Library Pin, Deployment), `AGENTS.md`, CHANGELOG
  - Bruno: `DELETE /libraries/{id}` при живом пине — 409, при работающей Deployment — 409, только при старой истории — 200; отдельный сценарий: Library, названная работающей Deployment, → 409; после нового Deploy без пина → 200; Deployment из файла с чужим пином → `POST …/discard` 200 с `warnings`, Draft доступен для записи
  - Тесты: репозиторий (`DeploymentNamesLibrary` — работающая/старая Deployment, удалённый Project), application (удаление при работающей Deployment — 409, после нового Deploy — 200; Discard с чужим пином — warnings и записываемый Draft), REST (preview `missing_releases`, Discard 200 с warnings и удалёнными Widgets), UI-предупреждения

## [0.0.79] - 2026-11-08

### Added

- **Project Release: публикация Commit'а и список Release** (задача 72_3; ADR 0021)
  - Домен: новый пакет `domain/release` — отдельный агрегат `Release`: `ID`, `Scope` (Project, Library — в 72_5), `Number` (VO, 1, 2, 3… в области), `CommitNumber`, `CommitTitle` (снимок заголовка Commit'а), `Note` (VO, необязательная, ≤2000 символов, редактируется отдельно), `Author`, `CreatedAt`, `Origin` (`commit`; `file` — 72_4) и `Content` — копия `draft.CommitContent`. `CreateRelease(scope, commit, number, note, author, at)` — чистый конструктор: Commit должен быть той же области, заголовок и содержимое копируются, Release не зависит от Commit'а/Draft'а/Library; `release_created`. `WithNote` возвращает новый Release с `release_note_updated`; содержимое неизменяемо. Новые `release.ErrReleaseNotFound` и `release.ErrCommitAlreadyReleased` (один Commit — не больше одной Release); `release_created`/`release_note_updated` в `event.EventType`
  - Application: `ReleaseService` (`Create`, `List`, `Find`, `UpdateNote`, `ProjectFile`) для Project'а. Проверяет живого Project'а и Organization, заметку и автора, читает Commit из Draft'а; Version Draft'а не читается и не требуется. `commit_created`-история помечает опубликованный Commit (`appdto.Commit.Released` по `ReleasedCommits`), `List`/`Find` отдают `appdto.Release` с `content_counts`
  - БД (`engineering/20261109000000_project_releases`): `engineering.project_releases` (`id`, `project_id` → `projects` CASCADE, `number`, `commit_number`, `commit_title`, `note`, `author`, `created_at`, `origin` с check `commit|file` и check провенанса (`origin='commit'` требует `commit_number` и `commit_title`), `content` jsonb; `unique(project_id, number)`, частичный `unique(project_id, commit_number) where origin='commit'`, индекс) и `engineering.project_release_library_pins` (пины ProjectFile'а Release, для задачи 71). Номер — `max(number)+1` под `pg_advisory_xact_lock` по Project (unique — страховка); один Commit — одна Release
  - REST (контракт `api/v0` **0.27**, MINOR): `POST …/releases` `{commit, note?, author}` → 201 `ReleaseResponse` (409 `commit_already_released`, 404 Project/Commit, 400); `GET …/releases` → `{releases}` новыми первыми без содержимого; `GET …/releases/{n}` → `ReleaseResponse` + `content_counts` (404); `PATCH …/releases/{n}` `{note}` → 200 без Version (404); `GET …/projects/{id}/releases/{n}/projectfile` — вложение `<project>-release-<n>.json`. События `release_created`/`release_note_updated` с полем `release` — в поток Project'а; `apiclient`: `CreateRelease`, `Releases`, `Release`, `UpdateReleaseNote`, `ReleaseProjectFile`
  - UI (Project): у каждого Commit'а в **History** — кнопка **Make Release** (диалог: заметка, автор из LocalStorage; у опубликованного Commit'а выключена с подсказкой), toast после успеха; кнопка **Releases** и диалог со списком Release (номер, заголовок Commit'а, заметка, автор, время, счётчики), редактированием заметки на месте и выгрузкой ProjectFile'а; `eventlog.ReloadTargets.Releases` перечитывает список и историю на `release_created`/`release_note_updated` и на resync
  - Документы: ADR 0021 (Project Release — отдельный агрегат, копия Commit'а, один Commit — одна Release), `GLOSSARY.md` (Release, Project Release, Release Origin, Commit), AGENTS.md, CHANGELOG
  - Bruno: create/list/find/download/rename заметки, `409 commit_already_released`, `404` для отсутствующего Commit/Release; `requestBodies` пополнены
  - Тесты: домен (копия Commit'а, чужой Commit, пустая/длинная заметка, `WithNote`, `Origin`), репозиторий (нумерация 1/2, один Commit — одна Release и конкурентная публикация, снимок заголовка, пины, CASCADE проекта), application/REST (create/list/find/projectfile, PATCH заметки, 400/404/409, `released` в истории и после переименования, доставка `release_created`/`release_note_updated` через outbox), UI (`uitest`: публикация из истории, повторная публикация, список, правка заметки, диалоги и шорткаты)

## [0.0.78] - 2026-11-08

### Added

- **Commit: Save фиксирует снимок Draft'а** (задача 72_2; ADR 0021)
  - Домен: `domain/draft.Commit` — сущность Draft'а (не отдельный агрегат): `ID`, `Number` (1, 2, 3… в Draft'е), `Title` (VO: обрезанный, непустой, ≤200 символов), `Author` (VO: обрезанный, непустой, ≤100), `CreatedAt`, `DraftVersion`, `Content`; неизменяем, `WithTitle` возвращает новый Commit с событием `commit_title_updated`; `NewCommit` пишет `commit_created`. `CommitContent` — либо канонический `projectv0.ProjectFile`, либо копии `library.ReleasedWidgetType`, кодируется внутренним форматом с версией (как outbox/DraftChange). `Draft`/`DraftChange` не меняются: Save не пишет DraftChange и не поднимает Version. Новые `draft.ErrNothingToSave`, `draft.ErrCommitNotFound`; `commit_created`/`commit_title_updated` в `event.EventType`
  - Application: `CommitService` (`Save`, `List`, `Find`, `Rename`, `ProjectFile`) для Project'а и Library через `draft.Scope`. Save читает Version, собирает снимок тем же `ProjectFileBuilder`, что Deploy (Library — `WidgetTypes`), затем репозиторий под блокировкой строки Draft'а сверяет Version (`409 draft_conflict`), отказывает неизменённому Draft'у (`409 nothing_to_save`), выдаёт номер `last_commit_number+1` и пишет Commit с пинами; отказ сборщика — `409 draft_not_saveable` с причиной. `appdto.Commit` с `content_counts` и `released` (пока всегда false)
  - БД (`engineering/20261108000000_commits`): `engineering.commits` (`id`, `draft_id` → `engineering.drafts` CASCADE, `number`, `title`, `author`, `created_at`, `draft_version`, `content` jsonb; `unique(draft_id, number)`, индекс `(draft_id, number desc)`), `engineering.commit_library_pins` (пины ProjectFile'а Commit'а, для задачи 71), `engineering.drafts.last_commit_number`/`last_commit_draft_version`
  - REST (контракт `api/v0` **0.26**, MINOR): `POST …/commits` `{version,title,author}` → 201 `CommitResponse`; `GET …/commits` → `{commits}` новыми первыми без содержимого; `GET …/commits/{n}` → `CommitResponse` (у Library — с `widget_types`); `PATCH …/commits/{n}` `{title}`; `GET …/projects/{id}/commits/{n}/projectfile` — вложение с документом. События `commit_created`/`commit_title_updated` (поле `commit`), Project — в поток Project'а, Library — в серверный; `apiclient`: `SaveCommit`, `Commits`, `Commit`, `RenameCommit`, `CommitProjectFile`
  - UI (Project и Library): кнопка **Save** (выключена без изменений), индикатор «Unsaved changes»/«Saved» по Version Draft'а против `draft_version` последнего Commit'а, диалог Save (заголовок по умолчанию, автор из `author:name` в LocalStorage), диалог **History** (номер, заголовок, автор, время, счётчики) с переименованием заголовка на месте и выгрузкой ProjectFile'а Commit'а; `eventlog.ReloadTargets.Commits` перечитывает историю на `commit_created`/`commit_title_updated` и на resync
  - Документы: ADR 0021 (начало; окончательно — 72_8), GLOSSARY (Commit, Save, Draft), AGENTS.md, CHANGELOG
  - Bruno: save/list/find/download/rename для Project'а и Library, `409 nothing_to_save`, `400` без автора; `requestBodies` пополнены
  - Тесты: домен (заголовок/автор, `WithTitle`, кодирование содержимого), репозиторий (нумерация, `last_commit_*`, `nothing_to_save`, конфликт и две конкурентные Save, переименование, CASCADE, содержимое Project/Library), application/REST (маршруты, 400/404/409), UI (`uitest`: Save, индикатор, история, переименование, конфликт)

## [0.0.77] - 2026-11-08

### Changed

- **Runtime-сущность Revision переименована в Deployment** (задача 72_9)
  - Домен и приложение: `application/runtime.Revision` → `Deployment`, `RevisionSummary` → `DeploymentSummary`, `RevisionCause` → `DeploymentCause` (значения `deploy`/`rollback`/`file` те же), `RevisionRepository` → `DeploymentRepository`, `RevisionService` → `DeploymentService`, `ErrRevisionNotFound` → `ErrDeploymentNotFound`; файлы `revision.go`/`revision_event.go`/`revision_service.go` → `deployment.go`/`deployment_event.go`/`deployment_service.go`; `NewRevisionCreatedEvent` → `NewDeploymentCreatedEvent`. `appdto.Revision` → `appdto.Deployment`, `RevisionSummary` → `DeploymentSummary`. Порт `RuntimeNode`: `FindCurrentRevision`/`FindRevision` → `FindCurrentDeployment`/`FindDeployment`; `inprocess` и `servertest` переименованы
  - БД (`runtime/20261107000000_deployments`): `runtime.revisions` → `runtime.deployments`, колонка `n` → `number`, constraint'ы и индексы (`uq_runtime_deployments_project_number`, `idx_runtime_deployments_rolled_back_from`, `fk_/ck_runtime_deployments_*`, `deployments_pkey`, `deployments_pk_id_seq`); `cause`, `rolled_back_from`, `deployed_at`, `config` не менялись. Записи `runtime.journal` с `type='revision'` переведены в `type='deployment'`, payload (`number`, `cause`, `rolled_back_from`) тот же; новые записи — `deployment`. Старые строки читаются без перезаписи, Down возвращает прежние имена
  - Контракт `api/v0` **0.25** (MINOR): `RevisionResponse` → `DeploymentResponse`, `RevisionSummaryResponse` → `DeploymentSummaryResponse`, `ListRevisionsResponse` → `ListDeploymentsResponse` (`deployments`), `RevisionCause` → `DeploymentCause`; в `OperationStateResponse` поле `revision` → `deployment`, в `DeployPreviewResponse` `current_revision` → `current_deployment`; событие `revision_created` → `deployment_created`, поле события `revision` → `deployment`. Маршруты `GET …/revision`, `GET …/revisions`, `GET …/revisions/{n}`, `GET …/revisions/{n}/projectfile`, `POST …/revisions/{n}/rollback` заменены на `…/deployment` и `…/deployments…`; старые отвечают **410 Gone** в формате Problem Details с подсказкой на новые и удаляются при выходе API 1.0
  - Контракт `record/v0` **0.3** (MINOR): `Revision` → `Deployment`, тип записи Journal `revision` → `deployment`; payload-поле `deployment`. Кодек и `apiclient.ParseEntry` читают старый тип `revision` и старое поле `revision` (для старых Journal и PlaybackFile) и нормализуют их в `deployment`; пишется только новое имя. `make schemas`
  - `apiclient`: `RevisionState` → `DeploymentState`, `ProjectClient.Revision` → `Deployment`, `Revisions` → `Deployments`, `RevisionByNumber` → `DeploymentByNumber`, `RevisionProjectFile` → `DeploymentProjectFile`, `Rollback` → `RollbackDeployment`; `ParseEntry` принимает обе формы
  - UI: `ui/player.Model.Revision()`/`SetRevision` → `Deployment()`/`SetDeployment`, `Result.Revision` → `Deployment`; `GET …/operation` читает `deployment`; Player применяет записи `deployment` и читает документ `GET …/deployments/{n}`. Project-режим: «Deployments» вместо «Revisions», `GET …/deployment`, список `GET …/deployments`, кнопки и подписи; событие `deployment_created` перезагружает бейдж и список. Operation подписан на `deployment_created`; History/бэкенд читает записи `deployment`
  - Документы: поправки в ADR 0013, 0014, 0019, 0020; `GLOSSARY.md` — **Deployment** и историческое **Revision**; AGENTS.md, CHANGELOG
  - Bruno: запросы переименованы на `/deployments…`, добавлены пять запросов на старые маршруты, ожидающих 410
  - Тесты: все runtime/application/restapi/UI/apiclient-тесты переименованы; добавлена миграционная проверка чтения старых `runtime.revisions` и Journal-записи `revision` и возврата имён Down'ом

## [0.0.76] - 2026-11-08

### Changed

- **EditLock убран: правка и сохранение без Check in/out** (задача 72_1; ADR 0022, поправка в ADR 0016; ADR 0015 помечен заменённым)
  - Удалён пакет `internal/server/domain/editlock` и всё, что он вёл: агрегат `EditLock`, `Holder`, `Snapshot`, события `edit_lock_*`, `application.EditLockService`, `EditLockGate`, `RunEditLockReaper`, presence (`Attach`/`Detach`), `editlock.System` и идентичность клиента в контексте. `Scope` (Draft Project'а или Library) переехал в `domain/draft` (`draft.Scope`, `ProjectScope`, `LibraryScope`) и больше не несёт версии замка. `domain/draft.Draft` потерял `Lock()`, `WithLock`, `PendingEvents` и поля замка
  - Конкурентность — только Version Draft: `draftRepositoryPostgres.Update` блокирует строку Draft и сравнивает заявленную Version, расхождение — `draft.ConflictError` (409 `draft_conflict` с текущей Version в `Problem.draft`); 400 «нет клиента» и 409 «замок занят» исчезли. Undo/redo остаются общими: второй инженер может отменить чужой DraftChange (осознанно до задач 49/50)
  - Deploy защищён проверкой Version вокруг сборки: читает Version V, собирает ProjectFile, затем в короткой транзакции проверяет, что Version всё ещё V (`draft.Repository.CheckVersion`), иначе 409 `draft_conflict`; только после проверки документ уходит на Runtime, поэтому смешанный ProjectFile не отправляется. Тот же помощник переиспользует 72_2
  - Миграция `engineering/20261106000000_remove_edit_lock`: из `engineering.drafts` удалены `holder_key`, `holder_name`, `held_since`, `last_seen`, `lock_version` (таблица `engineering.edit_locks` была удалена ещё в `20261105000001_drafts`). Сид замков не создаёт
  - REST и контракт: удалён `contract/api/v0/editlock.go`, DTO замка, `Problem.edit_lock`, события `edit_lock_*` из `EventTypes`, `apiEventTypes` и samples; MINOR `api/v0` поднят до 0.24, `make schemas` обновлён. Четыре маршрута на Project и Library (`GET/POST/DELETE …/edit-lock`, `POST …/edit-lock/takeover`) отвечают **410 Gone** в формате Problem Details с подсказкой; удаляются при выходе API 1.0. Middleware `X-GrowSCADA-Client` удалён, `?client=` у SSE принимается и игнорируется, `eventHub` больше не считает presence
  - `apiclient`: удалены `WithClientKey`, `ClientKey` и методы замка; `growctl`: нет Check out/Check in вокруг мутирующего прогона, флагов `--client-name`/`--take-over` и заголовка клиента — мутации просто шлют Version
  - UI: из шапок Project и Library убрана строка замка, read-only режим без замка, `editlock:client` (sessionStorage), `editlock:name` (LocalStorage), `?client=` у SSE, обработка `edit_lock_*` и toast «потерян замок»; кнопки правки, Deploy/Release/Discard доступны всегда. `GiveBackDraft`/Check in удалены, логика «дождаться серии стрелок перед записью» сохранена (она про Version); 409 `draft_conflict` перечитывает содержимое и стек
  - Bruno: удалены setup/teardown-запросы замка и `X-GrowSCADA-Client` во всех запросах; маршруты `/edit-lock` покрыты запросами, ожидающими 410
  - Документы: ADR 0022, поправка в ADR 0016, ADR 0015 помечен заменённым; GLOSSARY (EditLock/Check out/Check in удалены, Client переопределён, добавлен Author); AGENTS.md; CHANGELOG
  - Тесты: удалены тесты замка (домен, приложение, репозитории, REST, UI, growctl, apiclient e2e); тесты DraftChanges/Deploy/движения виджетов проверяют Version и конфликт; добавлены проверки `CheckVersion` и 410-маршрутов через Bruno

## [0.0.75] - 2026-11-07

### Added

- **Player и перевод Operation на Journal** (задача 43, часть 2; ADR 0020)
  - Новый пакет `internal/server/interface/ui/player` — операционная модель UI: документ работающей Revision и её номер, состояния тегов по ID (`value`, `quality`, `seq`, `written_at`, включая теги вне текущей конфигурации), позиция Journal и `Connected`; `Model.Apply` применяет записи `contract/record/v0` сразу и по порядку: `tag_state` — только при `seq` больше известного (запись тега вне конфигурации хранится, но не рисуется), `revision` — по номеру больше текущего (Payload несёт только номер; документ читается `GET …/revisions/{n}` отдельно), неизвестный `type` и чужой MAJOR игнорируются; позиция Journal двигается каждой прочитанной записью, даже неприменённой — это курсор
  - Маппинг `ProjectFile` → рендер-формы `livescene` переехал из `operation/load.go`/`matrix.go` в `player/model.go`: сцены с виджетами (матрица из геометрии через `uiutil.MatrixCSS`), WidgetTypes по ID и теги только текущей конфигурации; тег без состояния — zero-значение его типа с Quality `bad` и `seq` 0. `Model.MergeSnapshot` сливает стартовое состояние с уже применёнными записями по большему `seq`/номеру Revision и большей позиции, поэтому снимок, прочитанный во время потока, ничего не откатывает
  - Источник записей — `player.Source` (`Open`/`Close`, отчёты через `player.Events{Entry, Opened, Lost}`): модель не знает, живой поток это или перемотка; `player.LiveSource` — браузерный EventSource над `GET …/journal/events?from=<position>`. `player.Player` читает `GET …/operation`, открывает поток с его позиции и перерисовывает пачкой (`updateBatcher`, не чаще 50 мс: пачка записей — один `Update`); полный resync — на каждое явное открытие (монтирование, смена Project, реконнект после CLOSED), нативный retry EventSource догоняет из таблицы с тем же `from`, а если снимок ушёл вперёд при открытии — поток переоткрывается с новой позиции. Сбой чтения документа Revision — потеря связи: старая конфигурация на экране, `Connected=false`, resync с backoff (2 с, удвоение до 30 с); `GET …/operation` 404 — `uiutil.ActionProjectGone`. Периодического resync нет
  - `apiclient`: `ProjectClient.JournalEventsURL(from)` — URL живого потока Journal, `ParseEntry(data)` — терпимый разбор `data:` в `recordv0.Entry` (неизвестные поля и неизвестный `type` сохраняются, мусор и запись без `seq`/`type` отбрасываются); `ProjectClient.Operation` из 43_1 не менялся
  - Operation больше не читает `GET …/revision` и `GET …/tag-states` и не обрабатывает `eventlog.ActionServerEvent`/`EventProjectResync`: `SetProject` создаёт/перезапускает Player и его поток, `OnDismount` закрывает; выбор сцены, вкладки, LocalStorage и тема остаются в режиме. Project без Revision — «Project is not deployed», первый Deploy приходит `revision`-записью и переключает экран без перезагрузки, Revision без сцен — «No scenes to show»; выбранная сцена сохраняется по ID, если есть в новой конфигурации, иначе первая
  - `eventlog.ReloadTargets` без поля `Revision` (его читала только Operation), `revision_created` в таблице маршрутизации теперь только `Deploy`; поведение Bar и `/events` не менялось
  - AGENTS.md: Player, `/operation` + `/journal/events`, resync и батчинг, отказ Operation от `/revision`, `/tag-states` и событий Bar; CHANGELOG
  - Тесты: `player` — `tag_state` по `seq` (меньший/равный игнорируется), запись тега вне конфигурации, `revision` по номеру (повтор/старый игнорируется, состояния сохраняются, новые теги zero/bad, удалённые не рисуются), неизвестный тип, чужой MAJOR, отсутствие payload, слияние снимка, маппинг ProjectFile (сцены, матрица, WidgetTypes, порты, теги), смена сцены, батчинг (одна перерисовка на пачку), переоткрытие потока при сдвиге позиции, потеря связи и `Close`; `apiclient` — URL потока и терпимый разбор записи; UI Operation через `servertest`/`uitest` — проект без Revision, первый Deploy из Journal, значение из `tag_state` без перезагрузки, второй Deploy и Rollback меняют конфигурацию, и Operation не делает запросов `/revision`/`/tag-states`

## [0.0.74] - 2026-11-07

### Added

- **Модель чтения Operation на Runtime-ноде** (задача 43, часть 1; ADR 0020)
  - Контракт `api/v0` **0.23** (MINOR): `OperationPath = "/operation"` и `OperationStateResponse{position, revision, tag_states}` — позиция Journal проекта, работающая Revision (`RevisionResponse` с `content: ProjectFile`, `null` до первого Deploy, без `omitempty`) и состояния всех её тегов по ID (`TagStateResponse`, `{}` без Revision, не `null`); `make schemas`, прежняя версия ушла в `frozen.sha256`
  - `application/runtime.OperationStateService` (файл `operation_state_service.go`, в порт `RuntimeNode` не входит): `Get(ctx, projectID)` отвечает `OperationState{Position, Revision, Deployed, TagStates}` и читает в одном порядке — `JournalService.Position`, `RevisionService.FindCurrent`, `TagStateService.FindTagStates` по ID тегов документа; так Deploy, закоммиченный между чтениями, получает позицию выше снимка и приходит из потока, а чтение позиции последней могло бы скрыть его навсегда (ADR 0020)
  - Ненаписанный тег добивает `tag.NewTagType(definition.Type).InitialState()` — zero-значение типа, Quality `bad`, `seq` 0, `WrittenAt` nil; неизвестный тип в документе — ошибка; живой Project без Revision — `Deployed == false` с позицией и без состояний, не ошибка; удалённый/чужой Project — ошибка с `project.ErrProjectNotFound`. Рендер-формы не собираются: конфигурация — `projectv0.ProjectFile` как есть, импортов `domain/scene` и engineering-схем нет (ADR 0011); новых репозиториев, миграций и записей outbox нет
  - REST Runtime-ноды: `GET /api/v0/projects/{project_id}/operation` — 200 `{position, revision, tag_states}`, заголовок схемы как у всех ответов; живой Project без Revision — 200 с `revision: null`, удалённый — 404 `project_not_found`; проводка `restapi.Services.Operation`, `main.go`, `servertest`
  - `apiclient.ProjectClient.Operation(ctx)` — читает снимок, 404 остаётся `apiclient.ErrNotFound`, отсутствие Revision — успешный ответ с `revision == nil`
  - Bruno: `get_operation` (200, заголовок схемы, `position > 0`, `revision.number`, тег с записанным состоянием и тег с zero-значением и `quality: bad`) в разделе deploy — после Deploy, до первых записей значений; `requestBodies` не нужен (GET)
  - AGENTS.md: маршрут и модель чтения Operation (порядок чтений, ненаписанные теги, отсутствие Revision) в разделах Runtime и REST; CHANGELOG
  - Тесты: сервис (порядок чтений, zero/bad/`seq` 0 для string/integer/boolean, отсутствие Revision, развёрнутый пустой Project, неизвестный тип, `project.ErrProjectNotFound`), REST (200 до и после Deploy, начальное состояние тега, сдвиг позиции записью значения, 404 удалённого Project), apiclient (снимок до/после Deploy и после записи, 404)

## [0.0.73] - 2026-11-06

### Added

- **Journal: чтение и живой поток** (задача 42, часть 2; ADR 0019)
  - `application/runtime.JournalService` — сторона чтения Journal на Runtime-ноде, в порт `RuntimeNode` не входит: `Read(ctx, projectID, from, limit)` отдаёт записи проекта после исключающей `from` по возрастанию `seq` (не больше `limit`), `from < 0` и `limit` вне `(0, JournalMaxPageSize]` — `ErrInvalidInput`; `Position` — текущая позиция проекта (`COALESCE(MAX(seq), 0)`); константы `JournalPageSize = 100`, `JournalMaxPageSize = 1000`
  - `postgres/runtime.NewJournalRepositoryPostgres` (`JournalRepository`: `FindAfter`, `Position`) над `runtime.journal`: `WHERE project_id = $1 AND seq > $2 ORDER BY seq LIMIT $3`, строки декодирует `journal_codec.go`; живость проекта проверяется до чтения (как у `FindAll` Revisions), поэтому удалённый проект — `project.ErrProjectNotFound`, а не пустая страница; записи чужого проекта не возвращаются
  - Контракт `api/v0` **0.22** (MINOR): `JournalResponse{entries []recordv0.Entry}` (встраивание record/v0, как `RevisionResponse.Content`), `JournalPath`, `JournalEventsPath`, `JournalQueryFrom`, `JournalQueryLimit`; `make schemas`
  - REST Runtime-ноды: `GET /api/v0/projects/{project_id}/journal?from=<seq>&limit=<n>` — 200 `{entries}`, `from` по умолчанию 0 и исключающая, `limit` по умолчанию 100 и не больше 1000, нечисловое или вне диапазона — 400, проект не жив — 404 `project_not_found`; `GET …/journal/events` — SSE: `Content-Type: text/event-stream`, `: connected`, затем `id: <seq>` и `data: <Entry>` на запись; старт — `?from=` (важнее), иначе `Last-Event-ID`, иначе `Position` (только новое), плохая позиция — 400 и мёртвый проект — 404 ещё до открытия потока
  - Поток подписывается на `tag_updated` и `revision_created` **до** чтения позиции, читает таблицу страницами до пустой и ждёт пробуждения (payload не используется); страховочный тикер 5 с, сигналы схлопываются в канал ёмкостью 1; доставка at-most-once, а переподключение с `Last-Event-ID` догоняет точно, потому что читает таблицу (ADR 0019); свой обработчик, не `eventHub`: `maxSSEClients` на него не распространяется, `?client=` не нужен и EditLock не трогается; `/events` не менялся — судьба `tag_updated`/`revision_created` там за задачей 43
  - `restapi.Services.Journal`; `main.go` и `servertest.StartAPI` проводят сервис; `APIRouter.Close` закрывает открытые Journal-потоки, чтобы shutdown не ждал их
  - `apiclient.ProjectClient.Journal(ctx, from, limit)` — чтение диапазона, `ErrNotFound` на 404
  - Bruno: `get_journal` (200, заголовок схемы, ровно одна запись — `tag_state` с `tag_state.tag_id` записанного тега и `seq`, равный позиции из ответа на запись), `get_journal_line2` (журнал проекта Line 2 — одна запись `revision`, позиция уходит в `JOURNAL_LAST_SEQ`), `get_journal_from_last` (после последней позиции — пустой список), `get_journal_invalid_limit` (`limit=0` — 400), `get_journal_unknown_project` (404), `get_journal_events` (manual: поток не заканчивается); плейсхолдеры `TAG_WRITE_SEQ`, `TAG_WRITE_BEFORE`, `JOURNAL_LAST_SEQ` в окружении
  - AGENTS.md: чтение и поток Journal (маршруты, `Last-Event-ID`/`from`, at-most-once и точный догон, `/events` без изменений); CHANGELOG
  - Тесты: сервис (передача диапазона, `ErrInvalidInput` на `from`/`limit`, `project.ErrProjectNotFound`), репозиторий (порядок `seq`, исключающая `from`, усечение `limit`, чужой проект, удалённый проект, пустой Journal), REST (200/400/404 чтения, SSE с `id:` и декодируемым `data`, `from` важнее заголовка, старт с позиции — только новое, событие чужого проекта не будит, отключение снимает подписку, `Close` завершает поток), apiclient (чтение, пустая страница после последней позиции, 404)

## [0.0.72] - 2026-11-06

### Added

- **Journal: хранение и запись** (задача 42, часть 1; ADR 0019)
  - Контракт `record/v0` **0.2** (MINOR): `Entry` встраивает `Header` и несёт `seq`, `timestamp` (RFC 3339), `project_id`, `type` и payload `tag_state`/`revision` (оба `omitempty`); `EntryType` (`tag_state`, `revision`) с `EntryTypes()`; payload-типы `TagState{tag_id, value, quality}` и `Revision{number, cause, rolled_back_from?}`; `RevisionCause` (`deploy`, `rollback`, `file`) — своё перечисление, запись не импортирует контракт API; `aggregate_id` нет — сущность названа в payload; `make schemas`, прежняя версия ушла в `frozen.sha256`
  - Миграция `runtime/20261106000000_journal`: `runtime.journal` (`seq BIGINT` из явной `runtime.journal_seq`, `project_id` с `ON DELETE CASCADE`, `ts`, `type` с CHECK, `payload JSONB`, `schema_version`), индекс `(project_id, seq)`, append-only; `setval('runtime.journal_seq', MAX(tag_states.seq)+1, false)` до `DROP SEQUENCE runtime.tag_state_seq`, чтобы новое изменение было выше любого выданного `seq`; `runtime.tag_states.last_write_at` (NULL — начальное состояние); пустая `runtime.journal_cursors(subscriber, last_seq)`; Down возвращает `tag_state_seq` (значение выше сохранённых `seq`) и убирает таблицы и колонку
  - Кодек `postgres/runtime/journal_codec.go`: `recordv0.Entry` ↔ строка `runtime.journal` (payload по `type`, `schema_version` строкой, время в `ts`), конструкторы `newTagStateEntry`/`newRevisionEntry`, неизвестный `type` при чтении — ошибка, чужая версия схемы и запись без payload своего типа — отказ; нужен и чтению 42_2
  - Запись `tag_state`: `tagstate.Repository.Write` возвращает третий результат `changed bool`; одна транзакция — `SELECT … FOR UPDATE OF s` с проверкой Project, TagType и поставщика, сравнение value/Quality, при изменении `INSERT runtime.journal … RETURNING seq` и `UPDATE runtime.tag_states SET value, quality, seq, last_write_at`; повтор пишет только `last_write_at`, `seq` не двигается; строка тега блокируется, поэтому конкурентные писатели получают позиции в порядке блокировки; `tagstate.TagState.WrittenAt()` (`nil` у Initial) участвует в `Reconstitute`
  - Запись `revision`: `CreateRevision` (Deploy, DeployFile с cause `file`) и `Rollback` вставляют `revision`-запись (`{number, cause, rolled_back_from?}`, без документа) в транзакции, нумерующей Revision, `ts = deployed_at`; откат транзакции записи не оставляет; `UpsertDefinitions`/`Delete`/`refreshCopies`/`RemoveProjectTagStates` Journal не трогают
  - `TagStateService.SetTagValue` публикует `tag_updated` только при изменении; ответ PATCH несёт `seq` состояния (новый или прежний) и `written_at`; `FindTagStates` заполняет `WrittenAt`
  - Контракт `api/v0` **0.21** (MINOR): `WrittenAt *string` (`written_at`, `omitempty`, RFC 3339) в `TagResponse`, `TagStateResponse` и `UpdateTagResponse`; `newTagResponse`/`newTagStateResponse`/`newUpdateTagResponse`/`newGetTagStatesResponse` переносят его; apiclient и PWA типы берут из `apiv0` — правок нет
  - Seed `test_data`: состояния берут `seq` из `runtime.journal_seq` и пишут `last_write_at` (фикстура — не история, записей Journal в ней нет)
  - AGENTS.md: `seq` — позиция Journal, `last_write_at`, записи `tag_state`/`revision` в тех же транзакциях, миграция `journal`; CHANGELOG
  - Тесты: кодек (round-trip обоих типов, неизвестный `type`, чужая версия схемы, payload без своего типа), репозиторий (изменение — ровно одна запись с `seq` состояния и payload; повтор — ни записи, ни сдвига `seq`, двигается `last_write_at`; смена Quality — изменение; чужой поставщик — ни записи, ни изменения; конкурентные писатели — по записи на изменение в порядке блокировки; Deploy/DeployFile/Rollback — запись `revision` с временем Revision; отказ транзакции — ни записи; каскад Project уносит Journal, курсоры остаются), миграция (`setval` выше сохранённых `seq`, `last_write_at` NULL, `tag_state_seq` удалена, курсоры пусты, Down возвращает последовательность), сервис (`tag_updated` только при изменении, прежний `seq` у повтора, `written_at` до DTO), REST (`written_at` в ответах, повтор без сдвига `seq`), Bruno (`written_at` в `get_tags`, `get_tag_by_id`, `patch_tag_value`)

## [0.0.71] - 2026-11-06

### Added

- **Направляющие и притяжение при редактировании сцен, двухцветная сетка** (задача 41)
  - Сетка редактора не зависит от темы: точка — светлое ядро радиусом 1 px в тёмном кольце до 1.75 px (два `radial-gradient` в одном `background-image`), так что на любом фоне хотя бы одна её часть контрастирует; `--dot-color` убран из обеих тем, шаг 24 px и слой между фоном Scene и виджетами прежние, в Operation сетки по-прежнему нет
  - Притяжение при перетаскивании тела виджета — одиночного и группы: axis-aligned bounding box выборки (у группы — общий, объединение `widgetBounds` всех выбранных) жёстко притягивается к краям и центрам других виджетов текущей Scene и самой Scene, порог 6 px, по каждой оси независимо, побеждает ближайшая цель, при равенстве — цель Scene; свободная ось сохраняет сырую дельту, позиции остаются `float64`; выбранные виджеты целями не являются
  - Пока ось притянута, поверх виджетов и под рамкой выделения рисуется одна линия на ось — 1 px `var(--accent)` во всю ширину/высоту холста, `pointer-events: none`; линии исчезают с отпусканием мыши
  - Alt/Option во время перетаскивания отключает притяжение и прячет линии, пересчитывая позиции от старта переноса; отпускание Alt возвращает притяжение без ожидания следующего `mousemove` (до первого движения мыши Alt ничего не двигает — mousedown остаётся кликом, PATCH не уходит)
  - Ресайз, поворот, перенос origin, стрелки, панель свойств и создание виджета из палитры идут без направляющих; итог переноса — тот же один `PATCH …/scenes/{id}/widgets`, новых DraftChange, событий и полей API нет
  - Геометрия — чистые функции рядом с `widgetBounds` (`ui/project/marquee.go`): сбор целей, ближайшая цель оси, смещение выборки и координаты линий; размер холста и его умолчание 1920×1080 вынесены в один `sceneSize()`, общий для холста и притяжения
  - AGENTS.md: сетка не зависит от темы, направляющие и притяжение 6 px при перемещении одного виджета и группы (общий AABB), Alt отключает притяжение
  - Тесты: юнит — край к краю и к центру другого виджета, к краям и центру Scene, за пределами 6 px, повёрнутый виджет по AABB, группа по общему AABB, Alt отключает притяжение, Alt до первого движения ничего не меняет, равенство расстояний выбирает Scene, переприменение переноса по Alt; UI — слой направляющих в дереве холста во время переноса, его отсутствие после mouseup и притянутая позиция на сервере; сетка без `--dot-color` и с двумя градиентами

## [0.0.70] - 2026-11-06

### Added

- **Выделение и групповой перенос в PWA** (задача 40, часть 2; ADR 0010, ADR 0016, ADR 0017)
  - Состояние выделения Project-режима — упорядоченный `selectedWidgetIDs`, ведущий (primary) — последний: клик заменяет выделение, Shift/Ctrl/Cmd+клик добавляет или убирает без перетаскивания, Shift+рамка добавляет, `Ctrl/Cmd+A` выделяет все виджеты Scene, `Esc` снимает выделение и отменяет начатую рамку; в LocalStorage по-прежнему только ведущий ID, после перезагрузки выделен один виджет; смена Scene/Project снимает выделение, а перечитывание списка виджетов отбрасывает исчезнувшие ID и передаёт ведущего последнему оставшемуся
  - Рамка с пустого места холста: прямоугольник в координатах холста (полупрозрачная заливка и пунктир `var(--accent)`, `pointer-events: none`), попадание по пересечению axis-aligned bounding box, посчитанного из матрицы трансформации (поворот учитывается); протяжка меньше 3 px по обеим осям — клик; рамка работает и в read-only
  - Групповой перенос мышью: нажатие на невыделенном виджете заменяет выделение им и начинает перенос только его, на выделенном — переносит всю группу на одну дельту; завершение всегда шлёт один `PATCH …/widgets` со всеми сдвинутыми позициями (и для одного виджета), возврат на старт запроса не шлёт
  - Стрелки: 1 px, Shift — 10 px; движение локальное, PATCH уходит по keyup стрелки, через 500 мс без нажатий или по blur окна; любая другая запись Draft (удаление, панель, undo/redo, Deploy, Check in, смена Scene/Project, начало переноса) сначала завершает серию и ждёт её ответа, поэтому две записи не спорят за одну Version; в read-only стрелки ничего не делают и не показывают тост
  - Панель свойств: ровно один выбранный — прежние редакторы, удаление и ручки якоря/поворота/масштаба; несколько — «N widgets selected» без редакторов и полей Scene; ни одного — свойства Scene
  - Один перенос — одна DraftChange и N `widget_updated`; существующий `uiutil.Reloader` склеивает перечитывание
  - AGENTS.md: выделение, рамка, групповой перенос, стрелки, панель свойств и read-only в разделе PWA UI, Project-режим
  - Тесты: выделение (клик, toggle, Shift/Ctrl, Ctrl+A, Esc, снятие, ведущий при удалении, чистка после `setWidgets`), рамка (пересечение, вхождение, поворот, Shift-добавление, порог клика), перенос (группа одной дельтой и одним PATCH, одиночный PATCH, возврат на старт без запроса, read-only), стрелки (1/10 px, один PATCH на серию, сброс перед другой записью), панель свойств; прежние drag-тесты переведены на PATCH

## [0.0.69] - 2026-11-06

### Added

- **Групповой перенос виджетов: домен, application и REST** (задача 40, часть 1; ADR 0010, ADR 0016, ADR 0017)
  - Домен: `scene.WidgetMove` (ID виджета и новые x/y) и `Scene.MoveWidgets(moves)`: перенос меняет только `Position.X/Y`, `z` и все прочие поля (имя, размер, origin, поворот, WidgetType, метки, PortBindings) сохраняются; `WidgetType` и `TagTypes` не читаются и привязки не перепроверяются — сломанный биндинг не отказывает в переносе (ADR 0010); на каждый фактически сдвинутый виджет — `WidgetUpdatedEvent` в порядке `moves`, виджет на своей позиции пропускается без события; повтор ID — `ErrWidgetMovedTwice`, неизвестный ID — `ErrWidgetNotFound`, ни одного сдвига — `ErrNothingToMove`; возвращаются новая Scene и список сдвинутых виджетов
  - Draft: `Session.MoveWidgets` загружает Scene один раз, вызывает домен и пишет before/after снимок каждого сдвинутого виджета (несдвинутые в изменение не попадают), события — `PendingEvents` результата; undo возвращает все позиции группы одной операцией
  - Application: `SceneService.MoveWidgets(ctx, projectID, sceneID, version, moves)` и `appdto.WidgetMove{WidgetID, X, Y}` — один `DraftChange` «Move N widgets», Version +1; пустой список и повтор `widget_id` (и `scene.ErrWidgetMovedTwice` из домена) — invalid input (400), чужой виджет/Scene — 404, `nothing_to_move` — 409; гейт EditLock, `X-GrowSCADA-Client` и `draft_conflict` — общие для записи Draft
  - REST: `PATCH /api/v0/projects/{project_id}/scenes/{scene_id}/widgets` с `{version, moves: [{widget_id, x, y}]}`, ответ 200 `{version}`; 400 (пусто, повтор, незнакомое поле), 404 для виджета, названного телом запроса (не путём — `notFoundInPath` берёт ID из ошибки, когда в пути параметра нет), 409 `nothing_to_move` с деталью «Nothing to move: the widgets already hold these positions»
  - Контракт `api/v0` **0.20** (MINOR): `MoveWidgetsRequest`, `WidgetMoveRequest`, `MoveWidgetsResponse`; `make schemas`; `project/v0` без изменений, новых событий нет
  - Bruno: самодостаточный блок после `post_projectfile_open_invalid` — открытие файла стирает и пины Library, поэтому блок возвращает пин, создаёт Scene и два виджета заново (ID берутся из ответов через `bru.setVar` в `after-response`, имена объявлены в окружении `localhost` плейсхолдерами для статической проверки), затем перенос группы (200 `{version: 30}`), повтор позиций (`nothing_to_move`), устаревшая Version (`draft_conflict`), пустой `moves` (400) и виджет не из Scene (404); новый маршрут в `requestBodies`
  - AGENTS.md: `Scene.MoveWidgets`/`WidgetMove`, `SceneService.MoveWidgets`, маршрут и его отказы, соглашение о ID из ответа в Bruno
  - Тесты: домен (перенос только указанных, сохранение полей и `z`, порядок событий, пропуск несдвинутых, `ErrNothingToMove`, повтор и неизвестный ID), application (одна DraftChange с N снимками и описанием, Version +1, пусто/повтор, чужая Scene и чужой виджет, stale Version, `nothing_to_move` без записи, гейт замка), репозиторий (одна строка `draft_changes`, before/after каждого виджета, undo всех позиций и `widget_updated` по каждому), REST (200 и `{version}`, 400, 404 по ID из тела, 409 `nothing_to_move`/`draft_conflict`/`edit_lock`, заголовок схемы)

## [0.0.68] - 2026-11-05

### Added

- **ProjectFile в UI: скачивание, открытие и Deploy из файла** (задача 39; ADR 0018)
  - Маршруты под Project: `GET …/projectfile` — Draft как ProjectFile, собранный тем же `ProjectFileBuilder`, что и Deploy (неконсистентный Draft — 409 `draft_not_deployable` с причиной в `detail`; EditLock не нужен), `GET …/revisions/{n}/projectfile` — ровно сохранённый документ Revision (404 для номера, которого нет), `POST …/projectfile?version=V` — открытие файла в Draft (200 `{version}`, 409 `nothing_to_open`), `POST …/projectfile/deploy` — Revision из файла без Draft и EditLock (201 `{number, deployed_at}`, 409 `nothing_to_deploy`); скачивания — `application/json` + `Content-Disposition: attachment; filename="<project>-draft.json"` / `"<project>-revision-<n>.json"`, имя Project санитизируется до безопасных символов, при пустом результате — ID, `GrowSCADA-Schema-Version` остаётся заголовком API
  - Одна валидация для открытия и Deploy файла (`application/project_file_validator.go`): сначала `schema_version` (тот же MAJOR, MINOR не новее серверного — новый файл отвечает «файл новее сервера»), затем строгий decode (незнакомое поле — 400), совпадение `header.project_id` с Project из пути, доменные инварианты и ссылочная целостность (имена/типы/протоколы/размеры/языки скриптов; уникальные ID, включая ID Scene, и естественные ключи; `supplier_device_id` — Device из файла; `PortBinding` — Tag из файла, порт объявлен WidgetType и его `TypeHint` принимает TagType; пара — Environment и Device из файла; `Widget.TypeID` назван в `widget_types`; каждый виджет ровно в одной Scene) и пересборка ProjectFile из будущего содержимого с каноническим равенством; все проблемы сразу — 400 с `Problem.file_issues: [{path, message}]`
  - Открытие: `draft.Session.Open` пишет только фактически различающиеся сущности (сравнение по ID и состоянию) с обычными событиями сущностей — один DraftChange «Open a project file», Version +1, redo-хвост отброшен, стек не очищается, undo восстанавливает прежний Draft; сравнение «файл равен Draft» пропускается, если Draft не собирается, — валидный файл чинит неконсистентный Draft; пины сверяются с live Library и её Release и запираются `FOR SHARE` в транзакции Draft; Tag states и Device Tokens не трогаются
  - Deploy из файла: `RuntimeNode.DeployFile` → `RevisionService.DeployFile` → `CreateRevision` с cause `file` — одна транзакция нумерует Revision, пишет документ, пересобирает производные копии тем же diff, публикует `revision_created`; Draft и стек не трогаются, EditLock не нужен, локальные Libraries не проверяются (документ самодостаточен)
  - Discard Revision, называющей отсутствующие локально Library Releases, — 409 `revision_not_discardable` с `file_issues` по пинам, Draft не меняется; локальные пины запираются `FOR SHARE` в транзакции Discard
  - Контракт `api/v0` **0.19** (MINOR): `Problem.file_issues` + `FileIssue{path, message}`, `RevisionCauseFile = "file"`; `project/v0` без изменений; `make schemas`; миграция `runtime/20261105000001_revision_cause_file` расширяет CHECK'и `cause` до `deploy | rollback | file` и `rolled_back_from` (NULL для `deploy` и `file`)
  - apiclient: `DraftProjectFile`/`RevisionProjectFile` (сырые байты + имя из `Content-Disposition`), `OpenProjectFile(version, data)`, `DeployProjectFile`; `file_issues` приходят в `Problem` ошибки
  - Bruno: скачивание Draft и Revision, отсутствующая Revision, открытие (успех, равный Draft, устаревшая Version, невалидный файл), Deploy файла (успех, равный Revision, файл с чужими пинами), Discard чужой Revision; `requestBodies` статической проверки
  - UI Project-режима: одна кнопка `Project file…` с диалогом — `Download Draft` (Blob + `<a download>`), `Open a file…` (выбор `.json`, подтверждение с именем, схемой, project_id и счётчиками Tags/Scenes/Widgets/Devices/Environments, предупреждение «заменяет весь Draft, можно отменить», без EditLock недоступно с подсказкой), `Deploy a file…` (подтверждение с текущей Revision, доступно без замка); ошибки `file_issues` — списком «путь: сообщение», остальные — тост; после открытия перечитываются содержимое и стек, после Deploy файла — preview и список Revisions; в списке Revisions у каждой строки `Download`, причина `file` — «Deployed from file»
  - AGENTS.md: маршруты, `ProjectFileService` и валидатор, `Session.Open`, `DeployFile`, причина `file`, миграция, диалог UI и подсказки
  - Тесты: валидатор (новая/чужая версия схемы, чужой project_id, незнакомое поле, дубликаты ID и имён, включая две Scene с одним ID, висячие ссылки, порт не объявлен и несовместимый TagType, лишняя копия WidgetType, отсутствующий локальный Release, Draft не равен файлу, локальные проверки только при открытии), репозиторий (одна транзакция, один DraftChange, Version +1, undo, отброшенный redo-хвост), REST (четыре маршрута, заголовки, 409/400 и `file_issues`), apiclient, UI (диалог, подтверждение, список ошибок, открытие и Deploy файла, read-only, скачивание, причина `file`)

## [0.0.67] - 2026-11-05

### Added

- **Undo/redo в PWA** (задача 38, часть 4; ADR 0016, ADR 0017)
  - Project: кнопки `Undo`/`Redo` в шапке рядом со строкой EditLock; подсказка — `undo_description`/`redo_description` из `GET …/draft-changes` (`Undo: Delete tag pressure`), пустой стек — `Nothing to undo`/`Nothing to redo`, без замка — та же подсказка read-only, кнопки выключены; `Ctrl+Z`/`Cmd+Z` и `Ctrl+Shift+Z`/`Cmd+Shift+Z` — тот же POST, в поле ввода, при открытом диалоге режима и при скрытом режиме не перехватываются; клавиша читается и по `event.code` (`KeyZ`), так что работает и на нелатинской раскладке
  - Library: те же кнопки и клавиши в колонке библиотек, выключены и без замка, и при показе релиза; Release и Discard перечитывают состояние стека
  - Состояние стека (`loadDraftChanges`) читается при загрузке режима, на `EventResync`/`EventProjectResync`, после каждого события содержимого (`eventlog.ReloadTargets.DraftChanged`: правка Draft, Deploy, Release), после undo/redo и после Deploy/Discard/Release; ответ не подменяет более новое состояние (сравнение с известной Version); undo/redo шлют `{version}` из последнего ответа стека
  - Отказы: 409 устаревшей Version (`Problem.draft`) — забыть Version и перечитать стек и содержимое, как 409 правки; 409 пустого стека — toast и перечитывание состояния; потеря замка — read-only без потери стека
  - Одна Version Draft на Draft в обоих режимах: Project свёл `sceneVersions` и `sceneStates` к одной `draftVersions` (номер Scene и её Widgets, Tag, Pin — один и тот же; он же говорит, новее ли перечитанное состояние), Library переименовал свою карту в `draftVersions` с помощниками `draftVersion`/`markDraft`/`observeDraftVersion`/`forgetDraftVersion`; `lock_version` EditLock остаётся отдельным
  - `uiutil`: `KeyAction` (`KeyNone`/`KeyUndo`/`KeyRedo`), `UndoRedoKey`, `KeyEventAction` и `KeyEventTyping` — разбор keydown (Ctrl/Cmd+Z, Shift, поле ввода, раскладка по `code`)
  - AGENTS.md: кнопки и клавиши, чтение состояния стека, одна Version Draft
  - Тесты: `uiutil` (разбор клавиш), Project (пустой стек и подсказки, описание шага, undo кнопкой и redo клавишей, 409 устаревшей Version и пустого стека, read-only, событие содержимого, переключение проекта, одна Version в записях), Library (то же для WidgetType, Release показан read-only, Release чистит стек, переключение библиотеки)

## [0.0.66] - 2026-11-05

### Added

- **Undo/redo: сервис и REST** (задача 38, часть 3; ADR 0016, ADR 0017)
  - Домен: `Draft.Undo`/`Draft.Redo` применяют `before` изменения на `stack_position` и `after` на `stack_position+1`, поднимают Version и двигают позицию; `draft.Session.Undo`/`Redo` пишут снимки (операция не переигрывается) и публикуют обратные события — `tag_created` для восстановленного Tag, `widget_updated` для Scene, чьи привязки вернулись, `scene_deleted`/`widget_deleted` для отмены создания, `library_pin_updated` для отката смены Pin и так далее по каждому виду; сами undo/redo в лог не пишутся; `draft.StackState` — два изменения, которые клиент откатит и повторит; ошибки `draft.ErrNothingToUndo`, `draft.ErrNothingToRedo`
  - Репозиторий `draftRepositoryPostgres`: `Stack` (состояние без записи, пустой Draft — версия 1) и `Undo`/`Redo` — одна транзакция над Draft (блокировка строки, гейт EditLock, CAS по `drafts.version`, чтение изменения на позиции, запись снимков, обратные события в outbox, подъём Version и сдвиг позиции, без новой DraftChange); запрос без клиента — 400 до чтения стека, пустой стек — 409, устаревшая Version — 409
  - `application.DraftChangesService` (`State`, `Undo`, `Redo`) для Project и Library по `editlock.Scope`, с проверкой живого Project/Library (`ProjectContent`/`LibraryContent`); `appdto.DraftChanges{Version, CanUndo, CanRedo, UndoDescription, RedoDescription}`; сервис в `restapi.Services` (`main.go`, `servertest`)
  - REST: `GET …/draft-changes`, `POST …/draft-changes/undo` и `…/redo` под путём Project и Library; 200 с состоянием после операции, 409 `nothing_to_undo`/`nothing_to_redo` (Problem называет Library для Library), 409 устаревшей Version с `Problem.draft`, 409 без замка с `Problem.edit_lock`, 400 без `X-GrowSCADA-Client`, 404 неживой области; `api/v0` **0.18** (MINOR): `DraftChangesResponse`, `DraftChangeCommandRequest`, константы `DraftChangesPath`/`DraftChangesUndoPath`/`DraftChangesRedoPath`, `make schemas`
  - apiclient: `DraftChanges`, `UndoDraftChange`, `RedoDraftChange` у `ProjectClient` и `LibraryClient` (Version 0 — текущая Version Draft)
  - growctl: тест, что планировщик не читает версии сущностей — Version DTO теперь Version Draft (ADR 0016)
  - Bruno: чтение стека Project и Library, undo/redo обеих областей, восстановление Environment после undo и его отсутствие после redo, пустой стек, устаревшая Version, запрос без клиента, undo без замка (409 `edit_lock`), пустой стек Library
  - Тесты: домен (позиция и Version, отказ при чужом изменении, снимки каждой операции, обратные события каждого вида), репозиторий (восстановление Tag с привязками и события, redo и обрезка redo-хвоста новой правкой, пустой стек и гейт, Library WidgetType, глубина 100), сервис (Project, Library, 404, гейт, устаревшая Version, пустой стек), REST (оба маршрута обеих областей, коды и Problem Details, заголовки), apiclient, growctl

## [0.0.65] - 2026-11-05

### Added

- **Draft Library на Draft-агрегате** (задача 38, часть 2; ADR 0016, ADR 0017)
  - Схема `engineering/20261105000002_library_draft`: удалены `engineering.widget_types.version` и `libraries.discarding_to`, сняты задания политики `discard_library` и события `library_discard_started`; строка Draft'а Library уже бэкфиллена в 38_1
  - Домен: операции Draft библиотеки `CreateWidgetType`, `UpdateWidgetType`, `DeleteWidgetType` и `ReplaceWidgetTypes` (Discard) со снимками `draft.KindWidgetType`; `WidgetType` теряет `Version()`/`CheckVersion`/`expected` (версию проверяет Draft), `Library` теряет `DiscardingTo`/`StartDiscard`/`FinishDiscard`/`CheckEditable`; `LibraryRelease` проверяет Version Draft, событие `library_discard_started` и ошибки `ErrLibraryDiscarding`/`ErrDraftChanged` удалены
  - Репозитории: `draftRepositoryPostgres` пишет WidgetType'ы Library (CAS по `drafts.version`, DraftChange, гейт EditLock, `Replace` очищает строки перед вставкой релиза); `WidgetTypeRepository` стал только читающим (`NextID`, `FindByID`, `FindAll`), `ForDiscard` удалён; `LibraryReleaseRepository.Save` принимает Version Draft, блокирует строку Draft и проверяет её; `LibraryRepository` больше не хранит пометку Discard
  - Приватность: `WidgetTypeService` принимает Version Draft во всех записях, `LibraryService.Release` — Version Draft и чистит стек после успеха, `LibraryService.Discard` синхронный (`ReplaceWidgetTypes`) и отвечает `{version}`, `GET …/libraries/{id}/draft` отдаёт Version Draft; `NewDiscardLibraryPolicy` удалён
  - REST: create/delete WidgetType с версией (DELETE `?version=`), Release с версией Draft, Discard 200 `{version}`, поле `discarding_to` убрано; `api/v0` **0.17** (MINOR), `make schemas`
  - apiclient/growctl: `LibraryClient.DraftState`, автоматическое заполнение Version Draft в create/update/delete/Release, Discard возвращает состояние Draft; growctl читает Draft Version для Release и печатает `discarded: version N`
  - UI Library-режима: одна Version Draft на библиотеку (`uiutil.Versions` по ID библиотеки), Discard 200, удалены `discarding_to` и ожидание конца политики; кнопки undo — в 38_4
  - Bruno: `GET …/draft`, версии Draft в телах WidgetType/Release, `?version=` у DELETE WidgetType, Discard 200
  - AGENTS.md: Draft Library, Release и Discard, удалённые политика и пометка
  - Тесты: домен Library (Release/Discard-операции, ResetTo/Recreate), репозиторий Draft Library (одна DraftChange на запись, CAS, `Replace` с перестановкой имён и очисткой стека), сервисы (Version Draft, Discard одной транзакцией, публикация событий), REST (версии, Discard 200, `GET …/draft`), UI, growctl, apiclient, seed-данные

## [0.0.64] - 2026-11-05

### Added

- **Draft: один агрегат на область, DraftChange и Draft проекта** (задача 38, часть 1; ADR 0016, ADR 0017)
  - Схема: `public/20261105000001_drop_project_discard` убирает `projects.discarding`; `engineering/20261105000001_drafts` создаёт `engineering.drafts` (одна строка на Project или Library, две nullable FK с `ON DELETE CASCADE`, CHECK «ровно одна область», `version`, `lock_version`, поля держателя замка, `stack_position`) и `engineering.draft_changes` (`draft_id → drafts ON DELETE CASCADE`, `seq`, `description`, `before`/`after` `jsonb`), наполняет `drafts` существующими Project и Library, удаляет `engineering.edit_locks` и колонки `version` у Tag, Scene, Device, Environment, пары и Library Pin
  - Домен `domain/draft`: `Draft` (`Scope`, одна `Version` содержимого, встроенный EditLock с отдельным `lock_version`, `StackPosition`, `Replace`, `Purge`), `draft.Session` — одна запись в транзакции Draft с загрузчиком только нужных сущностей (`CreateTag`, `DeleteTag` со снятием привязок, Scenes, Widgets с проверкой закреплённого релиза, Devices, Environments, пары, `PutLibraryPin`/`DeleteLibraryPin` с `ImpactOf` и чисткой сломанных привязок в том же изменении), `draft.Snapshot` — полное состояние строки до/после; внутренний кодек `postgres/draftchange` со своей версией формата
  - Репозиторий `draftRepositoryPostgres`: одна транзакция на запись (строка Draft `FOR UPDATE`, гейт EditLock, CAS по `drafts.version`, выборочная загрузка, запись затронутых строк и outbox, одна строка `draft_changes`, подъём Version и позиции стека, обрезка redo-хвоста и глубины 100); `Replace` (Discard), `Purge` (удалённый Project), `ClearStack` (после Deploy/Release); он же — хранилище EditLock (`Find`, `Create`, `Save` по `lock_version`, `FindAllHeld`, `Touch`)
  - Записи содержимого Draft проекта переведены на Draft: `TagService`, `SceneService`, `DeviceService`, `EnvironmentService`, `LibraryPinService` принимают Version Draft (create-тела и DELETE `?version=`), устаревшая — 409 `draft.ErrDraftConflict`; DTO содержимого несут одну и ту же Version Draft; `GET …/projects/{id}/draft` отдаёт её состояние
  - Discard проекта синхронный: читает текущую Revision и одной транзакцией Draft заменяет содержимое, чистит стек и отвечает 200 `{version}`; `StartDiscard`/`FinishDiscard`, `project_discarding`, `project_discard_started`, `SaveDiscarding`, `NewDiscardProjectPolicy` и политика удаления привязок удалены; `NewUnbindDeletedTagPolicy` и `NewWidgetReconciliationPolicy` удалены — их работу делает DraftChange; cleanup проекта пишет `Purge` как `editlock.System`
  - Deploy после успешной Revision чистит стек отдельной транзакцией; `ProjectFileBuilder` не изменился
  - Контракт `api/v0` **0.16** (MINOR): `version` в create-телах Tag/Scene/Device/Environment/Widget, `scene_version` виджета переименован в `version`, `?version=` у DELETE, `DiscardProjectResponse` — `{version}`, `EventProjectDiscardStarted` удалён, `GET …/draft`, 409 устаревшей Version несёт текущую в `Problem.draft`; `make schemas`
  - Bruno: `GET …/draft`, версии в телах create/update по порядку `seq`, `?version=` у DELETE, Discard 200 `{version}`; apiclient заполняет Version из `GET …/draft`, когда клиент её не указал, growctl и UI читают её явно
  - UI Project-режима: одна Version Draft вместо версий Scene (`uiutil.Versions`: наблюдается из ответов по отметке запроса, забывается на 409 и ресинке), Discard 200 (его Version и версия удаления последней Scene учитываются); кнопки undo/redo — часть 38_4
  - Тесты: домен Draft и кодек, репозиторий Draft (CAS, `draft_changes`, гейт замка, сохранение виджетов при правке Scene, CASCADE, Purge), сервисы (Version Draft, Discard одной транзакцией, Deploy чистит стек, cleanup как System), REST (version в create/delete, 409, Discard 200, `GET …/draft`, отсутствие `discarding`), seed-данные и миграции

## [0.0.63] - 2026-11-05

### Added

- **EditLock: Draft библиотеки правит один клиент** (задача 37, часть 2; ADR 0015)
  - `EditLockService` работает с `editlock.Scope` обеих областей: `State`, `CheckOut`, `CheckIn`, `TakeOver`, `CheckHeld` берут область Draft'а, проверяют живой Project (`ProjectContent`) или Library (`LibraryContent`, новая обёртка над `LibraryRepository` и организацией) и отказывают `project_discarding` / `library_discarding` при идущем Discard. `EditLockGate` Deploy тоже принимает область
  - Гейт записей Library-области в транзакции записи: `aggregateStore` библиотечной области (WidgetTypes Draft'а) читает строку замка по `library_id`; Release — в `LibraryReleaseRepository.Save` рядом с блокировкой строки Library и `ErrDraftChanged`; Discard — в транзакции записи пометки (`LibraryRepository.Save`), снятие пометки политикой пишет как `System`. Гейт общий (`checkEditLock`), Project и Library различаются колонкой
  - REST API: маршруты `GET`/`POST`/`DELETE …/libraries/{id}/edit-lock` и `POST …/edit-lock/takeover` — та же семантика, что у проекта (201/200/409 с держателем, 400 без клиента, 404 удалённой Library, 409 `library_discarding` при Check out во время Discard); `apiclient.LibraryClient` получил `StateEditLock`, `CheckOutEditLock`, `CheckInEditLock`, `TakeOverEditLock`
  - События `edit_lock_*` области Library идут в серверный поток: `newAPIEvent` называет Library в `id` и `library_id`, `eventlog` не считает их содержимым проекта (иначе тег чужого проекта и потеря события на устаревшем соединении), `ReloadTargetsFor(EventResync)` перечитывает замок
  - PWA, Library-режим: та же строка замка в шапке (имя — из общего `editlock:name`, ключ вкладки — из клиента `root`); без замка WidgetTypes, Release и Discard только для чтения (кнопки видны и выключены с подсказкой), список/создание/переименование/удаление Library и просмотр релизов остаются доступны; потеря замка — toast и read-only; переключение Library сначала шлёт Check in прежней; состояние читается на `EventResync` и по `edit_lock_*` своей библиотеки
  - growctl: мутирующий прогон берёт замок и у Library — `apply` (WidgetTypes), `release`, `discard`, `delete -f` (WidgetTypes); занятый замок — ошибка с именем держателя, `--take-over` — форс. Общий `lockDraft` над Project и Library
  - Bruno: setup/teardown замка библиотеки, заголовок клиента у её мутирующих запросов, запросы свободного состояния, 409 с держателем, перехвата и возврата замка
  - Тесты: сервис Library-области (взятие/отдача/перехват, `CheckHeld`, 404, Check out при Discard), репозиторий (гейт WidgetType, Release, пометки Discard; запись держателя поднимает `last_seen`), REST (четыре маршрута, 409 с держателем, 400 без ключа, 404, гейт записи, Check out при Discard), UI (свободен/мой/чужой, read-only WidgetType/Release/Discard, Check out с именем, Check in, перехват, toast потери, Check in при переключении Library), growctl (занятый замок и `--take-over` для apply и release), `newAPIEvent` Library-замка
  - Документация: `AGENTS.md` описывает маршруты Library-замка, гейт Library-области и read-only Library-режима

## [0.0.62] - 2026-11-05

### Added

- **EditLock: Draft проекта правит один клиент** (задача 37, часть 1; ADR 0015)
  - Миграция `engineering.20261105000000_edit_locks`: `engineering.edit_locks` — одна строка на Draft, две nullable FK (`project_id → projects`, `library_id → engineering.libraries`, обе `ON DELETE CASCADE`), CHECK «ровно одна область», CHECK «держатель целиком или никак», частичные уникальные индексы по каждой области и индекс по `last_seen` для реапера. Строка остаётся при свободном замке: `version` растёт монотонно (взятие, отдача, перехват, истечение), `last_seen` версию не поднимает
  - `domain/editlock`: `Scope` (Project или Library), `Key` (UUID вкладки), `Holder` (ключ + добровольно указанное имя), агрегат `EditLock` (`Acquire`, `Release`, `TakeOver(expectedVersion)`, `Expire`, `Seen`), ошибки `ErrEditLockHeld`, `ErrEditLockFree`, `ErrEditLockNotHeld`, `ErrEditLockVersionStale`, `ErrEditLockNameEmpty`, `ErrEditLockExists`, `ErrNoClient`; события `edit_lock_acquired`/`_released`/`_taken_over`/`_expired` несут `Scope()` и новое состояние и идут через outbox; `domain/editlock` — пакет инженерной ноды
  - `EditLockRepository` и его postgres-реализация: `Find`, `Create`, `Save` (CAS по `version`), `FindAllHeld`, `Touch` (только `last_seen`, без версии и событий)
  - Гейт в транзакции записи: `aggregateStore` каждой области проекта читает строку замка `FOR SHARE` и сверяет ключ из context; чужой/свободный — `ErrEditLockNotHeld` с держателем, запрос без клиента — `ErrNoClient` (400), контекст без идентичности отказывает; политики и очистка пишут как `editlock.System` и проверку не проходят. Запись держателя поднимает `last_seen`, поэтому клиент без SSE (growctl, Bruno) живёт записями
  - `application.EditLockService` (`State`, `CheckOut`, `CheckIn`, `TakeOver`, `CheckHeld`, `Attach`, `Detach`, `ReapOnce`) с presence ключ → число SSE-соединений; реапер `RunEditLockReaper` раз в 30 с обновляет `last_seen` присутствующим и освобождает отсутствующих после `EditLockTTL = 5 мин` с `edit_lock_expired`; запущен в `main.go` и в `servertest`, останавливается по context
  - Deploy проверяет замок перед сборкой ProjectFile; Discard — в транзакции пометки (`ProjectRepository.SaveDiscarding`); Check out при идущем Discard — 409 `project_discarding`
  - REST API **0.14** (MINOR): заголовок `X-GrowSCADA-Client` (и в `Access-Control-Allow-Headers`), параметр `?client=` SSE-потоков, `EditLockResponse`, `EditLockHolderResponse`, `CheckOutEditLockRequest`, `TakeOverEditLockRequest`, поле `edit_lock` в `Problem` и в `Event`; маршруты `GET`/`POST`/`DELETE …/projects/{id}/edit-lock` и `POST …/edit-lock/takeover`; заголовок читает middleware и кладёт идентичность в context каждого запроса; SSE-обработчик при открытии делает `Attach`, при закрытии — `Detach`
  - `apiclient.WithClientKey` (заголовок на каждом запросе), `StateEditLock`, `CheckOutEditLock` (201 или 200), `CheckInEditLock`, `TakeOverEditLock`
  - PWA, Project-режим: ключ вкладки — UUID в `sessionStorage` (`editlock:client`), имя — в `LocalStorage` (`editlock:name`), создаются в `root` и уходят в клиент и в `eventlog.Bar` (`?client=`); строка замка в шапке — `Check out` (спрашивает имя один раз), `Editing as <name> — Check in`, `<name> is editing since HH:MM — Take over`; без замка Draft и Deploy/Discard только для чтения с подсказкой; перехват — подтверждение с держателем и его временем, затем POST с `version` и перезагрузка содержимого; потеря замка — toast и read-only; переключение проекта и уход из Project-режима сначала шлют Check in прежнему
  - growctl: `--client-name` (по умолчанию имя пользователя ОС) и `--take-over`; мутирующие команды проекта берут Check out с ключом на запуск и Check in в конце, занятый замок — ошибка с именем держателя, `apply` без изменений замок не берёт
  - Bruno: setup — Check out проекта Main заголовком клиента, teardown — Check in, заголовок у всех мутирующих запросов, новые запросы маршрутов замка (свободный, взятие, повтор, 409 с держателем, перехват, Check in), порядок `seq` сдвинут
  - Тесты: домен (взятие, отказ чужому, идемпотентность, перехват с устаревшей версией, истечение, `Seen` без версии), репозиторий (CAS, `FindAllHeld`, `Touch`, CASCADE строки проекта, гейт записи), сервис (Check out/in/перехват, `CheckHeld`, реапер с presence и TTL, Deploy/Discard без замка), REST (четыре маршрута, 409 с держателем, 400 без ключа и с кривым заголовком, 404, гейт записи), UI (строка свободен/мой/чужой, read-only, Check out с именем, Check in, перехват с подтверждением, toast потери, Check in при переключении и уходе из режима), growctl (замок берётся и отдаётся, занятый — ошибка, `--take-over`, неизменный apply замок не берёт)
  - Документация: `AGENTS.md` описывает маршруты замка, таблицу, гейт, сервис, реапер и read-only Project-режим

## [0.0.61] - 2026-11-04

### Added

- **Rollback и список Revisions на Runtime-ноде** (задача 36; ADR 0014, поправка к ADR 0013)
  - Миграция `runtime.20261104000000_revision_cause`: `runtime.revisions` получает `cause` (`deploy` | `rollback`, NOT NULL без default) и `rolled_back_from` — самоссылочный FK `(project_id, rolled_back_from) → runtime.revisions(project_id, n)` с `ON DELETE CASCADE`; CHECK'и не дают неизвестную причину и откат без скопированной Revision
  - `application/runtime.Revision` несёт `Cause` и `RolledBackFrom`; `RevisionRepository` — `FindAll` (сводки новыми первыми, без документов), `FindByNumber` (полный документ, `ErrRevisionNotFound`), `Rollback`: одна транзакция с тем же advisory-lock и блокировкой строки проекта, что Deploy, сравнение `jsonb` целевого документа с работающим (`ErrNothingToRollback`), запись `max(n)+1` с причиной `rollback` и копией документа без изменений, пересборка производных копий тем же диффом (состояния неизменённых Tag'ов и все Device Tokens живут, исчезнувшие состояния уходят, новые/со сменённым TagType — initial). Удалённый проект — `project.ErrProjectNotFound`, идущий Discard — `project.ErrProjectDiscarding`
  - `RevisionService.List`/`Find`/`Rollback`; после коммита откат публикует тот же `revision_created` (без outbox и без новых полей)
  - REST API **0.13**: `GET …/revisions` (200 `{revisions: [{number, deployed_at, cause, rolled_back_from?}]}`, новыми первыми, `[]` без Revision, 404 удалённого проекта), `GET …/revisions/{n}` (форма работающей Revision плюс причина и `content`; 404, 400 на нечисловой номер), `POST …/revisions/{n}/rollback` (201 со сводкой, 404, 409 `nothing_to_rollback` / `project_discarding`). Контракт `api/v0` — `RevisionCause`, поля причины в `RevisionSummaryResponse` и `RevisionResponse`, `ListRevisionsResponse`; схемы перегенерированы
  - `apiclient.ProjectClient`: `Revisions`, `RevisionByNumber`, `Rollback`
  - PWA, Project-режим: кнопка `Revisions` в deploy-полосе (всегда доступна), диалог — список новыми первыми с причиной («Deployed» / «Rolled back from #N»), текущая Revision помечена и не откатывается; выбор строки подгружает документ и показывает счётчики Scenes/Widgets/Tags/Devices/Environments, `Roll back` подтверждается, успех закрывает диалог и перезагружает preview и список, отказы — toast; список перезагружается на `revision_created` и resync
  - Тесты: нумерация и `cause`/`rolled_back_from`, сохранение состояний и токенов, сброс состояния исчезнувшего Tag и смена типа, отказ при равном содержимом, 404 чужого/несуществующего номера, отказ при Discard, сериализация параллельных Rollback/Deploy; сервис публикует событие только после коммита; REST — три маршрута, `200 []`, 404, 400, 409; UI — пустой список, список после Deploy, откат и бейдж, 409 как toast, перезагрузка на `revision_created`
  - Bruno: `GET …/revisions`, `GET …/revisions/{n}` (с документом), 404 и 400, `POST …/rollback` — 409 на работающую Revision и успешный откат после второго Deploy; Draft проекта возвращается в исходное состояние, номера `seq` последующих запросов сдвинуты

### Removed

- **Копия ProjectFile у Engineering-ноды** (ADR 0014, поправка к ADR 0013): миграция `engineering.20261104000000_drop_project_files`, `ProjectFileCopies`, `ProjectFileRepositoryPostgres`, `appdto.DeployState`, `DeployService.State`, `ErrRevisionCopyNotSaved` и его 500-маппинг. Повторный Deploy неизменённого Draft — всегда 409 `nothing_to_deploy`, «починки копии» больше нет; Project-режим берёт Revision и признак неразвёрнутых изменений из `deploy/preview`

## [0.0.60] - 2026-11-04

### Added

- **REST, PWA и документация Deploy/Revision/Discard** (задача 35, часть 5; ADR 0007, ADR 0008, ADR 0011, ADR 0013)
  - REST API 0.12: `GET /projects/{id}/revision` (текущая Revision с ProjectFile; 404 до первого Deploy), `GET …/tag-states` (снимок состояний Tag'ов Revision по ID), `GET …/deploy/preview` (`current_revision`, `unchanged`, `dropped_widgets`), `POST …/deploy` (201 `{number, deployed_at}`, 409 `nothing_to_deploy`, 500 при несохранённой копии у Engineering) и `POST …/discard` (202 `{discarding: true}`, 409 без Revision). Revision и состояния Tag — Runtime-ноды, Deploy и Discard — Engineering-ноды; `revision_created` уже в потоке событий проекта
  - `apiclient.ProjectClient`: `Revision` (404 — «не развёрнут», не ошибка), `TagStates`, `DeployPreview`, `Deploy`, `Discard`
  - PWA, Project-режим: полоса над сценами — бейдж «Revision N · deployed T» или «Not deployed», индикатор «Unreleased changes», кнопка Deploy (диалог по свежему preview: текущая Revision, предупреждение о выпадающих Widgets, подтверждение; отказ сервера — toast) и Discard (подтверждение; 409 без Revision — toast). Состояние развёртывания перезагружается на content-события Draft и `revision_created`
  - PWA, Operation: читает только `GET …/revision` и `GET …/tag-states`, показывает сцены, Widget и WidgetType'ы из Revision, значения Tag — из снимка и `tag_updated`; пустое состояние «Project is not deployed»; Draft- и `library_*`-события больше не перезагружают вид, `revision_created` и resync — перезагружают
  - UI-тесты: Project показывает бейдж, диалог preview и Revision 1 после Deploy, «Nothing to deploy» для неизменённого Draft, «Unreleased changes» после правки; Operation показывает пустое состояние, сцены и значения Revision, игнорирует сцену Draft и перезагружается на `revision_created`
  - Bruno: раздел `deploy` (preview до Deploy → deploy → revision → tag-states → preview неизменённого → повторный 409 → discard) **до** первой записи значения Tag, на пустом проекте Line 2; `docker-compose.yaml` коллекций делает Deploy проекта Main перед стартом симулятора — без Revision его Device и Tags не существуют на Runtime-ноде
  - Документация: ADR 0013 «Deploy делает неизменяемую Revision; Discard сбрасывает Draft»; поправки в ADR 0007 (пины в ProjectFile как провенанс), ADR 0008 (Discard — policy) и ADR 0011 (мост удалён, копии — только из Revision); `AGENTS.md` описывает маршруты, Project-полосу, Operation по Revision и Deploy перед симулятором

### Changed

- Генератор JSON-схем различает одноимённые типы из разных пакетов: второй получает имя с пакетом (`project.v0.PortBinding`), а не отказ — Revision в контракте API несёт ProjectFile
- `restapi.Services` получил `Deploy`, `Discard` и `Revisions`; `main.go` и `servertest` собирают те же сервисы

## [0.0.59] - 2026-11-04

### Added

- **Engineering-нода: сборка ProjectFile, Deploy, preview и Discard** (задача 35, часть 4; ADR 0006, ADR 0008, ADR 0011, ADR 0012)
  - `application.ProjectFileBuilder` — один сборщик над `ProjectScope` и пинами: Tags с поставщиком, Devices, Environments и пары, Scenes с Widgets, копии **только используемых** WidgetType из закреплённых Release и список пинов как провенанс. Документ канонизируется (списки по ID, ключи JSON отсортированы), поэтому один Draft даёт байт-в-байт одинаковый ProjectFile. Висячие ссылки — ошибка сборки, а не пропуск: Widget типа, которого нет в закреплённом Release, Tag с поставщиком не из Devices проекта, PortBinding на Tag, которого в Draft нет
  - `application.DeployService`: `Deploy` сравнивает собранный документ с Revision, которую выполняет Runtime-нода (совпал — 409 `ErrNothingToDeploy`), иначе `RuntimeNode.Deploy` → копия в `engineering.project_files`; сбой записи копии — `ErrRevisionCopyNotSaved` при **активной** Revision, а повторный Deploy того же Draft не отклоняется, а дописывает копию. `Preview` (`current_revision`, `unchanged`, `dropped_widgets` — пуст до задачи 48) и `State` (`deployed`, `differs`) отдают Project-режиму то, что он показывает
  - `application.DiscardService`, `domain/project.StartDiscard`/`FinishDiscard`/`IsDiscarding` и `ProjectDiscardStartedEvent`: `Discard` помечает проект (409 `ErrNoRevisionToDiscard`, если Revision нет) и сразу отвечает; `NewDiscardProjectPolicy` (ключ `project:<id>`) возвращает Draft к работающей Revision — сначала убирает то, чего она не называет, в обратном порядке ссылок (Scenes, пины, Tags, пары, Environments, Devices), затем восстанавливает в прямом (пины → Devices → Environments → пары → Tags → Scenes), по одному агрегату на транзакцию, восстанавливая недостающее под теми же ID; пины — штатными методами без `ImpactOf` и без ожидаемой версии, пометка снимается последней. Состояния Tag и Device Tokens — Runtime-ноды, Discard их не трогает
  - Пока пометка стоит, Draft read-only: каждая запись его агрегатов проходит `aggregateStore.lockProject` и отвечает 409 `ErrProjectDiscarding`, Deploy отказывает и на Engineering-ноде (`ProjectContent.openNotDiscarding`), и в транзакции `CreateRevision`, которая берёт строку проекта `FOR SHARE`
  - `projects.discarding` — миграция `20261104000000_project_discard` (схема public); `project_discard_started` в контракте — **REST API 0.11** (MINOR, пока MAJOR = 0), схема `schemas/api/0.11.json`; `appdto.Project` несёт пометку
  - Тесты: Deploy создаёт Revision и копию; повторный Deploy без правок — 409; правка Widget — `differs` и Revision N+1; сбой копии — Revision активна, повтор дописывает копию; Discard возвращает удалённые Scene/Widget/Tag/Device/пин с теми же ID и удаляет добавленные, включая Device и Environment, которые держат Tag и пара, не трогая состояния Tag; запись в Draft и Deploy во время Discard — 409; Discard без Revision — 409

### Changed

- **Мост Draft→Runtime умер** (ADR 0011, поправка 2026-11-04): `CreateTag` больше не upsert'ит определение после коммита, `DeleteTagByID` не удаляет состояние, policy `sync_runtime_tag_definitions` и её регистрация удалены, а `UpsertTagDefinitions`/`RemoveTagStates` ушли из порта `RuntimeNode`. Копии Runtime-ноды — принадлежность работающей Revision: Tag, созданный в Draft, получает значение только после Deploy (404), удалённый теряет состояние со следующей Revision
- `AGENTS.md` описывает сборщик, Deploy, preview, Discard и пометку; `servertest` деплоит через `DeployService` и больше не пишет копии Runtime-ноды в обход Draft

## [0.0.58] - 2026-11-04

### Added

- **Runtime-нода: Deploy, текущая Revision, конфиг Gateway** (задача 35, часть 3; ADR 0011, ADR 0004)
  - Порт `application.RuntimeNode` получил `Deploy(ctx, projectID, file) (appdto.Revision, error)` и `FindCurrentRevision(ctx, projectID) (appdto.Revision, bool, error)`: номер и время ревизии назначает Runtime-нода, документ чужого проекта — `runtime.ErrInvalidInput`; прежние методы и их поведение сохранены
  - `application/runtime.RevisionService`: `Deploy` пишет ревизию через `RevisionRepository.CreateRevision` (вся транзакция — в репозитории) и **после коммита** публикует `revision_created` через `eventbus.AsyncPublisher` — без outbox, at-most-once, как `tag_updated`; `FindCurrent` отдаёт текущую ревизию. `inprocess.NewRuntimeNode` и `main.go` прокидывают сервис
  - Событие `revision_created` — ContentEvent проекта (`{id: project_id, revision, deployed_at}`), попадает только в поток своего проекта; **REST API 0.10** (MINOR, пока MAJOR = 0): поля `revision` и `deployed_at` в `Event`, схема `schemas/api/0.10.json`
  - `DeviceConfigService` переехал в `application/runtime` и собирает конфигурацию Device Gateway из работающей Revision — `runtime.devices`, `runtime.environments`, `runtime.environment_devices`, `runtime.tags` — в одной REPEATABLE READ транзакции, Draft не участвует: Device вне Revision (спящий токен) — 403, нет Environment или пары — 404, токен не удаляется и не меняется; Project, помеченный удалённым, конфигурацию больше не отдаёт (`ErrProjectNotFound`)
  - Сид: копии Gateway-фикстур (`sim`/`bench`, `gateway`/`lab`) в `runtime.devices`/`environments`/`environment_devices` — без них `GET /device/config` на чистой БД отвечал 403, а Bruno и симулятор не проходили
  - Тесты: две ревизии подряд с растущим `deployed_at` и `UNIQUE` при параллельных Deploy; `revision_created` публикуется после коммита и не публикуется при ошибке; гонка моста с Deploy — копия вне Revision не выживает; конфиг отвечает по копиям и не видит Device, которого нет в Revision, но чей токен жив

### Changed

- **Мост Draft→Runtime молчит у проекта с Revision** (ADR 0011, поправка 2026-11-04): `UpsertDefinitions` и `Delete` в `postgres/runtime` берут тот же advisory-lock, что `CreateRevision`, и в той же транзакции проверяют наличие Revision, поэтому копии принадлежат работающей Revision. Tag, созданный во время Deploy, попадает в **следующую** ревизию, а запись его значения до неё — 404; до первой Revision мост по-прежнему заполняет копии
- `appdto.DeviceConfig` удалён: конфигурация Gateway — DTO Runtime-ноды (`application/runtime.DeviceConfig`)
- Симулятор и его e2e-тесты деплоят проект через `servertest.DeployProject`; `AGENTS.md` и ADR 0011 описывают мост и конфиг Gateway по новому поведению

## [0.0.57] - 2026-11-04

### Added

- **Хранение Revision: `runtime.revisions` и `engineering.project_files`** (задача 35, часть 2; ADR 0011, ADR 0005)
  - `runtime.revisions(pk_id, project_id, n, deployed_at, config jsonb)`: неизменяемый номерной снимок ProjectFile — истина о том, что проект выполняет; `UNIQUE (project_id, n)` и нумерует, и страхует гонку, строка уходит с Project каскадом, отдельного индекса под чтение текущей ревизии нет. `engineering.project_files(project_id PK, revision_n, deployed_at, config jsonb)` — вторичная копия последнего отправленного ProjectFile: бейдж «Revision N · deployed at T» и признак неразвёрнутых изменений
  - `postgres/runtime.NewRevisionRepositoryPostgres`: `CreateRevision` — одна транзакция (advisory lock по проекту → `n = max+1` → вставка документа → пересборка копий). Копии `runtime.tags`, `runtime.devices`, `runtime.environments`, `runtime.environment_devices` пересобираются **диффом** (upsert по ID, удаление исчезнувших), а не сносом: `tag_states` и `device_tokens` переживают Deploy. Новый Tag получает initial, смена TagType — сброс в initial, неизменённый сохраняет записанное значение, копия без состояния получает его назад; `FindCurrent` отдаёт текущую ревизию. `postgres.NewProjectFileRepositoryPostgres` — upsert копии у Engineering, где запись более старой ревизии ничего не меняет: два одновременных Deploy не откатывают бейдж назад
  - Миграция `20261103000002_deferrable_environment_names`: уникальность имени Environment в проекте стала `DEFERRABLE`, транзакция ревизии откладывает проверку до коммита — Deploy, в котором два Environment меняются именами, больше не падает на дубле, а документ с настоящим дублем по-прежнему отклоняется
  - `appdto.Revision` и `application/runtime.Revision` — по одному на каждую роль (ADR 0011): номер, время и документ; `application/runtime.RevisionRepository` — порт хранения
  - Тесты (testcontainers): нумерация 1, 2, 3 и восемь параллельных Deploy без дублей; `UNIQUE` как страховка; сохранение значений Tag и токенов; каскад исчезнувших копий; сброс при смене TagType; лечение пропавшего состояния; обновление существующих копий Device и Environment; поставщик Tag'а; пустые `config`/`params` записываются как `{}`, а не NULL; пара чужого проекта отклоняется, не трогая его `params`; обмен именами и настоящий дубль; удаление Project уносит ревизии и копию ProjectFile
  - Проверка ролей признаёт контракты общими (ADR 0005): пакеты роли вправе называть `contract/project/v0`, и `TestRoles_DoNotImportEachOther` снова зелёный

## [0.0.56] - 2026-11-03

### Added

- **Контракт ProjectFile: формат `project/v0`** (задача 35, часть 1; ADR 0005, ADR 0007)
  - `contract/project/v0`: `Header {schema_version, project_id}` (имени проекта нет — переименование не требует Deploy) и `ProjectFile` — полный снимок Draft: определения Tag'ов с поставщиком, Devices с конфигурацией под протокол, Environments и пары `(Environment, Device)` с параметрами связи, Scenes с Widgets (геометрия, labels, port_bindings, type_id), копии только используемых WidgetType из закреплённых Release и Library Pins как провенанс. Без состояний Tag, Device Tokens, версий агрегатов, трансформ-матрицы и истории
  - `JSONObject` — JSON-объект, перевозимый как есть (`config`, `params`): нулевое значение — `{}`, не-объект — ошибка чтения. Копия `apiv0.JSONObject`, чтобы контракты оставались независимыми (ADR 0005)
  - **Формат проекта 0.2** (MINOR, пока MAJOR = 0; обязательный `project_id` делает документы 0.1 нечитаемыми новым типом — ADR 0005 разрешает до 1.0): схема `schemas/project/0.2.json`, `0.1.json` заморожен в `frozen.sha256`
  - Тесты: типы и закоммиченная схема совпадают; `JSONObject` отвергает не-объекты и пишет `{}` для нулевого значения

## [0.0.55] - 2026-11-03

### Added

- **Симулятор как Device Gateway: токен, окружение, коды выхода** (задача 34, часть 5; ADR 0004, поправка 2026-11-03)
  - **REST API 0.9** (MINOR, пока MAJOR = 0): `DeviceIdentityResponse` и `GET /api/v0/device` — что означает токен Gateway'а: его Device и проект. Gateway читает это первым: проект входит в путь каждой записи, и назвать его может только токен. Схема `schemas/api/0.9.json`
  - `devicelink.NewRESTLink(client, token)` — вместо имени проекта токен: каждый запрос несёт `Authorization: Bearer gsc_dev_…`, проект берётся из `/api/v0/device` и кэшируется (при 404 переспрашивается), `Resolve` и `Write` идут прежними путями REST. `IsUnauthenticated` (401), `IsForbidden` (403) и `IsNoPermission` (оба) различают отказ сервера от временной ошибки
  - Конфиг симулятора: вместо `project:` — `token:` и `environment:` (имя или ID; имя резолвится среди окружений проекта); `server`, `control`, `autostart`, `defaultInterval`, `tags` прежние. `--token`/`DEVSIM_TOKEN` переопределяют токен ещё до проверки конфига, поэтому секрет может не лежать в файле вовсе
  - Симулятор **читает конфигурацию у сервера** (`GET /api/v0/device/config?environment=…`): Device, окружение, параметры связи и Tag'и, которые Device поставляет; локальный конфиг задаёт только, что и как писать. Tag, которого Device не поставляет, и несовместимый паттерн — ошибка конфигурации
  - **Перечитывание конфигурации**: подписка на `GET /api/v0/device/events` (SSE по токену) с переподключением и перечитыванием, плюс таймер как fallback; при изменении состава Tag'ов и их типов симулятор останавливает или возобновляет запись. В статусе control API — Device, окружение и протокол
  - Отказы сервера: 401 (токен не тот/отозван) завершает Gateway; 403 на запись (Tag не поставляется, Device вне конфигурации) останавливает этот Tag; 403 на чтение конфигурации (Device вне конфигурации) Gateway пережидает, а не завершается
  - **Коды выхода** `cmd/device_simulator` разведены: 401 → `ExitNoPerm` (77), прочие 4xx, ошибки TLS и схемы URL → `ExitProtocol` (76), тег не найден / несовместимый паттерн / окружение / ошибка конфига → `ExitConfig` (78), нет файла конфига → `ExitNoInput` (66); транзиентные ошибки по-прежнему повторяются
  - `apiclient`: `WithDeviceToken`, `DeviceIdentity`, `DeviceConfig`, `ListEnvironments`, `DeviceEvents` (разбор SSE, ошибка `ErrStreamClosed`); поток не ограничен таймаутом клиента
  - Seed-данные: Device `gateway` и окружение `lab` с fixture-токеном для dashboard-симулятора; Device `sim` (`bench`) остаётся fixture-устройством Bruno — у него своя судьба (Bruno отзывает его токен и удаляет его Tag), поэтому они разведены. Dashboard-теги поставляет `gateway`
  - Bruno: запрос `get_device_identity`, `get_device` симулятора проверяет Device и окружение; обе коллекции проходят
- Тесты: e2e `internal/devicesim` против настоящего API (значения по паттернам, control API, отсутствующий Tag → `ErrConfig`, неизвестный токен, отзыв токена останавливает Gateway), unit — перечитывание конфигурации по событию и по таймеру, 403 на запись останавливает Tag, 403 конфигурации пережидается, смена типа Tag'а не возобновляет запись старым генератором, окружение по имени и по ID (включая переименование); `apiclient` — токен, `/device`, `/device/config`, разбор SSE и 401; `restapi` — `/device`; `cmd/device_simulator` — таблица кодов выхода

### Changed

- `internal/devicesim.Device` получает конфигурацию через `Source` (реализация — `devicesim.Gateway` поверх REST API), а не ищет Tag'и сам; `devicelink.Link` (`Resolve` + `Write`) не менялся (ADR 0004). `make run_simulator` и `make run_simulator_dashboard` работают на seed-данных без `growctl apply`
- `PUT …/devices/{id}` и `PUT …/environments/{id}` разбирают `version` до чтения агрегата, как это уже делает `PUT …/environments/{id}/devices/{device_id}`: отрицательная версия — 400 клиента, а не 404 устройства или окружения, которого нет

### Fixed

- **Симулятор не гоняет личность Tag'а с писателем**: `write` берёт `devicelink.Tag` под замком `Device.mu` — при перечитывании конфигурации (новый ID или тип) писатель больше не читает поле, которое перезаписывает горутина `watch`, и запись уходит ровно той личности, для которой сгенерировано значение. Имя Tag'а (`simTag.name`) неизменно и читается без замка
- **Фатальная ошибка не подвешивает выключение Gateway'а**: `reportFatal` отдаёт ошибку в `Run`, но отпускает горутину, если ctx уже отменён; раньше второй отправитель в буфер `fatal` (размер 1) мог ждать читателя вечно, а `Run` — свою `wg.Wait()`
- **Перечитывание конфигурации не логирует под замком и не повторяет одно и то же предупреждение**: `applyConfiguration` собирает сообщения и пишет их после `Unlock`; «Tag не поставляется», «неизвестный тип» и «паттерн не подходит» предупреждают один раз, а не на каждом перечитывании
- **`PATCH …/tags/{id}/value` аутентифицирует раньше разбора тела**, как `GET /device/config` разбирает `environment`: запрос с неизвестным или чужим токеном получает 401/403, даже если тело он прислал негодное, а не 400 раньше проверки токена

## [0.0.54] - 2026-11-03

### Added

- **REST: устройства, окружения, токены и API для Gateway** (задача 34, часть 4; ADR 0004, поправки 2026-11-01; ADR 0011)
  - **REST API 0.8** (MINOR, пока MAJOR = 0): DTO `DeviceResponse`/`CreateDeviceRequest`/`UpdateDeviceRequest` (с `version`), `EnvironmentResponse`/`Create…`/`Update…`, `EnvironmentDeviceResponse`/`UpdateEnvironmentDeviceRequest`, `DeviceTokenResponse` (секрет — только при выпуске и ротации), `DeviceTokenMetadataResponse` (без секрета), `DeviceConfigResponse` для Gateway'а и `JSONObject` — JSON-объект протокольных конфигураций и параметров связи, сохраняемый как есть. Схема `schemas/api/0.8.json`
  - Маршруты: CRUD `…/devices` и `…/environments`, пары `GET …/environments/{id}/devices` + `PUT`/`DELETE …/devices/{device_id}` (пара идентифицируется двумя ID пути), токены `POST`/`GET`/`DELETE …/devices/{device_id}/token` и `POST …/token/rotate`, а также `GET /api/v0/device/config?environment=<id>` и SSE `GET /api/v0/device/events` для Gateway'а. У каждого ресурса свой `resource` в `errors.go`, свой файл обработчика, строки в `routes()` и `requestBodies`
  - **Device Token — на Runtime-узле** (`domain/devicetoken`, `application/runtime.DeviceTokenService`, `postgres/runtime`): один токен на Device, хранится только хешем (SHA-256) и отпечатком, показывается один раз; выпуск, ротация и отзыв; `Authorization: Bearer gsc_dev_…` разрешается в Device и проект, результат кэшируется в памяти и кэш сбрасывается при ротации/отзыве. Bearer без префикса — не device-токен (401, пользовательская авторизация — задача 50)
  - **Авторизация записи**: `PATCH …/tags/{id}/value` с device-токеном несёт `device_id` писателя в `SetTagValue`; чужой Tag — 403 (`tag.ErrTagNotSuppliedByDevice` в том же UPDATE), неизвестный/отозванный токен — 401, токен другого проекта — 403. Без заголовка пишут люди, как раньше; Tag с поставщиком человеку недоступен (403)
  - **API для Gateway**: `DeviceConfigService` собирает конфигурацию из Draft (до Deploy, задача 35): Device, Environment, параметры пары и Tag'и, которые Device поставляет. Токен спящей Device (её нет в текущей конфигурации) — 403; SSE отдаёт проектные события по токену
  - `runtime.device_tokens.project_id` (новая миграция): токен называет проект без обращения к Engineering-ноде и переживает Deploy; строка уходит вместе с проектом (CASCADE)
  - Отображения ошибок: 403 (`tag.ErrTagNotSuppliedByDevice`) и 409 «в использовании» (`device.ErrDeviceInUse`, `device.ErrEnvironmentInUse`), 404/409 токена (`devicetoken.ErrTokenNotFound`/`ErrTokenExists`); `problems.go` получил `NewUnauthorized`/`NewForbidden`, `corsMiddleware` разрешает `Authorization`
- **Bruno**: в коллекции `growscada_server` — запрос на каждый новый маршрут (чтения, затем изменения, удаления в конце), с проверками статуса, заголовка схемы, `application/problem+json` на ошибках и ключевых полей. Новые seed-данные: Device `sim`, Environment `bench`, пара с параметрами, Tag `device.speed` и fixture-токен; Device `sim-2` без токена — для выпуска, ротации и отзыва
- Тесты: `domain/devicetoken` (генерация, хеш, отпечаток, разбор префикса), сервис токенов (выпуск/кеш/ротация/отзыв, чужой проект), репозиторий токенов на Postgres (вставка, замена, отзыв, каскад с проектом), REST — CRUD Device/Environment/пар, жизненный цикл токена, `/device/config` (200/401/403/404), запись Tag'а Device/человеком/чужим токеном, SSE Gateway'а; сентинелы ошибок в `errors_internal_test.go`

### Fixed

- **Кэш токенов не может воскресить отозванный токен**: `Authenticate` запоминает поколение кэша до чтения из БД и не пишет результат, если за это время прошли ротация/отзыв/удаление проекта; иначе `put` после `clear()` возвращал бы старый токен в кэш
- **Удалённый проект забывает свои токены**: `NewProjectCleanupPolicy` после удаления строки проекта зовёт `RuntimeNode.RemoveProjectDeviceTokens`; `DeviceTokenService.RemoveProjectTokens` удаляет строки (`DeleteByProject`) и вычищает кэш проекта — иначе токен удалённого проекта продолжал бы аутентифицироваться в памяти и открывать SSE
- **SSE Gateway'а получает события своего проекта**: `eventStreams.accepts` доставляет Gateway'у `project_*` своего проекта (они идут в серверный поток), не отдавая чужих; раньше `project.ProjectEvent` в `gatewayConfigEvent` был недостижим
- **Осиротевший токен виден и отзывается**: чтение и отзыв (`GET`/`DELETE …/devices/{id}/token`) больше не требуют существования Device в Draft; выпуск и ротация по-прежнему проверяют его
- **401 несёт `WWW-Authenticate: Bearer`**, а `GET /device/config` аутентифицирует раньше разбора `environment` (401, а не 400)

## [0.0.53] - 2026-11-02

### Added

- **Persistence, application и проверка поставщика** (задача 34, часть 3; ADR 0004, поправка 2026-11-01; ADR 0011)
  - Postgres-репозитории `DeviceRepository`, `EnvironmentRepository`, `EnvironmentDeviceRepository` по образцу соседних: запись состояния и pending-событий одной транзакцией (ADR 0008), CAS по загруженной Version, все запросы ограничены проектом (ADR 0006). У пары нет своего ID: её ключ — `(environment_id, device_id)`, `Save` вставляет и обновляет по нему. Области проекта (`ProjectScopes`) получили `Devices()`, `Environments()` и `EnvironmentDevices()`
  - `DeviceService` и `EnvironmentService` (последний — CRUD окружений и привязка пары `(Environment, Device)` с параметрами; `version 0` в `PUT` добавляет пару) и DTO Device, Environment, EnvironmentDevice в `appdto`
  - `tag.Supplier` — объект-значение поставщика Tag'а: `tag.NoSupplier()` (пишут люди) и `tag.SuppliedByDevice(id.ID[device.Device])`; `Tag.Supplier()` заменил `DeviceID()` и сырой нулевой `id.ID`. Поставщик по-прежнему задаётся только `CreateTag`/`ReconstituteTag`; его смена с ожидаемой Version — вместе с API привязки Tag→Device (задача 68)
  - Поставщик из другого проекта отклоняется в транзакции записи Tag'а (`tag.ErrSupplierOfAnotherProject`); страховка в схеме — составной FK `engineering.tags(device_id, project_id) → engineering.devices(id, project_id)`
  - Удаление с ссылками — отказ, не каскад: `DeviceRepository.Delete` отдаёт `device.ErrDeviceInUse`, пока Tag поставляется Device или Device стоит в паре, `EnvironmentRepository.Delete` — `device.ErrEnvironmentInUse`, пока у окружения есть пары. Очистка удалённого проекта убирает пары, устройства и окружения перед его строкой
  - **Мост в Runtime**: `appdto.TagDefinition`, `runtime.TagDefinition`, `tagstate.Definition` и `postgres/runtime.UpsertDefinitions` переносят `device_id` в копию определения; `inprocess` пробрасывает его
  - **Запись только поставщиком**: `runtime.SetTagValueInput.DeviceID` — Device писателя (ноль — человек); `tagstate.Repository.Write` принимает `tag.Supplier` и проверяет его в том же UPDATE, что живость проекта и TagType (ADR 0011), отказывая `tag.ErrTagNotSuppliedByDevice` (Tag есть, писать нельзя; 403 появится с REST в части 4). Tag без поставщика по-прежнему пишут люди

### Changed

- **Runtime-копии удалённого проекта уходят вместе с его строкой** (ADR 0011): `runtime.devices` и `runtime.environments` ссылаются на `projects` с `ON DELETE CASCADE`, `runtime.environment_devices` — на обе копии с тем же; иначе после Deploy (задача 35) строка проекта не удалялась бы. В Draft ссылки по-прежнему отказывают, а не каскадят (ADR 0008)
- **Пары индексируются по Device**: `engineering.environment_devices` получил `idx_environment_devices_device_id`, `runtime.environment_devices` — одноимённый индекс. По `device_id` смотрит проверка занятости Device и FK, отказывающий его удаление; прежний уникальный ключ начинается с `environment_id` и этот поиск не покрывает
- **Списки Device и Environment читаются в живом проекте одним запросом** (`FindAllInLiveProject`, как у Tag и Scene): UI перезагружает их на события, а проект читается отдельно только при пустом списке, чтобы отличить отсутствующий от пустого. `PutEnvironmentDevice` сначала проверяет ожидаемую Version и только потом разбирает `params` — устаревшая запись даёт 409, как у Device и Environment, даже с невалидными параметрами; гонка «Device удалили между проверкой поставщика и записью Tag'а» отдаёт `tag.ErrSupplierOfAnotherProject`, а не общий 500
- `roles_test` уже относит `domain/device` к общему домену и проверяет, что общие пакеты не импортируют роль; поправка ADR 0011 об этом сделана в части 2

## [0.0.52] - 2026-11-01

### Added

- **Домен Device, Environment и EnvironmentDevice** (задача 34, часть 2; ADR 0004, поправка 2026-11-01). Persistence, application и REST — части 3–5
  - `internal/server/domain/device`: `Device` — имя (уникально в проекте, ADR 0003), комментарий, протокол, JSON-конфигурация под протокол и своя Version; `Update` меняет имя/комментарий/протокол/конфигурацию и ждёт ожидаемую Version, `Delete` пишет событие. `Environment` — имя (уникально в проекте), комментарий, `Update` (имя и комментарий) с ожидаемой Version, `Delete`. `EnvironmentDevice` — агрегат на пару (Environment, Device) с параметрами связи, `UpdateParams` с ожидаемой Version; своего идентификатора у пары нет — как у Library Pin, её называют Environment и Device. Все три иммутабельны: изменение возвращает новый агрегат и записывает событие, `CheckVersion` при чужой версии отдаёт `Err…Conflict`
  - Объекты-значения: `DeviceName`, `EnvironmentName` (пустое имя — ошибка), `DeviceProtocol` (закрытый набор, на старте только `simulation`; ноль невалиден, ADR 0001), `DeviceConfig` и `EnvironmentDeviceParams` — неизменяемый JSON-объект (вход и выход копируются, не-объект — ошибка, пусто — `{}`)
  - События `device_created`/`_updated`/`_deleted`, `environment_*`, `environment_device_*` — `project.ContentEvent`: называют проект и едут в его SSE-поток; у событий пары `ID` — это ID окружения (своего ID у пары нет), плюс `EnvironmentID` и `DeviceID`
  - Ошибки-сентинелы по образцу `library`/`scene` (`ErrDeviceNotFound`, `ErrDeviceConflict`, `ErrDeviceNameTaken`, `ErrEnvironment*`, `ErrEnvironmentDeviceNotFound`, `ErrEnvironmentDeviceConflict`, `ErrEnvironmentDeviceExists`) и интерфейсы `DeviceRepository`, `EnvironmentRepository`, `EnvironmentDeviceRepository` в области проекта (как `ProjectScopes`)
- **Поставщик у Tag'а**: `Tag` несёт `Supplier` — объект-значение поставщика (`tag.NoSupplier()` — пишут люди, `tag.SuppliedByDevice(id.ID[device.Device])`), часть определения; `CreateTag`/`ReconstituteTag` принимают поставщика. Репозиторий Tag'ов читает и пишет `engineering.tags.device_id`
- REST API **0.7**: типы событий `device_*`, `environment_*`, `environment_device_*` в `apiv0.EventTypes()` и поля `environment_id`/`device_id` у `Event` (MINOR, пока MAJOR = 0)

### Changed

- Outbox-кодек хранит и читает события Device, Environment и EnvironmentDevice (payload получает `environment_id`/`device_id` пары); журнал событий UI знает их типы и относит к содержимому проекта, но ничего не перезагружает: панели устройств нет (задача 66)
- `restapi` знает ошибки Device/Environment/EnvironmentDevice: 404 и 409 с ресурсами `Device`, `Environment`, `EnvironmentDevice` (маршруты — часть 4)
- `roles_test`: `domain/device` отнесён к общему домену, а не к `engineeringRole`: `domain/tag` (общий для ролей) называет ID поставщика как `id.ID[device.Device]`, и Runtime-нода компилирует `tag` вместе с ним

## [0.0.51] - 2026-11-01

### Changed

- **Схема Device, Environment и разделение состояния Tag'а** (задача 34, часть 1; ADR 0011, поправка 2026-11-01; ADR 0004, поправка 2026-11-01). Поведения в этой части нет — только миграции обеих цепочек и приведение репозитория Runtime-ноды к новой схеме; домен, application, REST и симулятор — части 2–5
  - `engineering/20261101000000_devices_and_environments`: таблицы `devices` (имя уникально в проекте, `protocol`, `config` JSONB, своя Version), `environments` (имя уникально в проекте), `environment_devices` (пара «окружение + устройство», `params` JSONB, своя Version, `created_at`/`updated_at` с триггером, как у соседних таблиц; ни с одной стороны нет CASCADE, поэтому Device или Environment удаляется только когда пар с ним не осталось); `engineering.tags.device_id` с FK на `devices` и индексом — поставщик значения Tag'а
  - `runtime/20261101000000_tag_states_and_devices`: `runtime.tags` становится копией **только определения** (остаются `id`, `project_id`, `name`, `type`, `version`; добавлен `device_id`; `value`/`quality`/`seq` убраны), состояние переезжает в `runtime.tag_states(tag_id PK, value, quality, seq)` — с FK на копию определения и `ON DELETE CASCADE`, `CHECK` по quality, `seq` через прежнюю `runtime.tag_state_seq`; существующие состояния переносятся без потери значений и `seq`. Копии `runtime.devices`, `runtime.environments` и `runtime.environment_devices` (без `version`); у копии окружения имя уникально в проекте — Gateway называет окружение по имени, и имя резолвится в пару внутри runtime-схемы. `runtime.device_tokens(device_id PK, token_hash, fingerprint, issued_at)` с уникальным индексом по хешу и **без FK** на копию Device: токен переживает Deploy и Rollback и спит, пока его Device нет в ревизии, поэтому удаление копии его не трогает. Откат возвращает состояния в `runtime.tags` вместе с `seq`
  - Репозиторий Runtime-ноды читает и пишет две таблицы: `FindInLiveProject`/`FindByIDs` — `JOIN` копии и состояния (проверка живости проекта и типа Tag'а осталась в тех же запросах), `Write` — `UPDATE runtime.tag_states … FROM runtime.tags`, `UpsertDefinitions` — вставка копии определения и начального состояния одной транзакцией (состояние уже написанного Tag'а не трогается), `Delete`/`DeleteAllOfProject` — удаление копии, состояние уходит с ней
  - Seed и тесты миграций приведены к новой схеме; новый тест миграций проверяет перенос состояний, каскад, уникальность имён и пар в Draft, копии Runtime и откат
- **Словарь домена — один `GLOSSARY.md`**. `CONTEXT.md` удалён: единственный словарь теперь `GLOSSARY.md` в формате скилла `domain-modeling` (термин, короткое определение без деталей реализации, `_Avoid_` с отвергнутыми синонимами, кластеры подзаголовками); ссылки на словарь обновлены в `AGENTS.md` (правило 9, «Agent skills»), `docs/agents/domain.md` (теперь ровно шаблон скилла: `GLOSSARY.md` / `GLOSSARY-MAP.md`), ADR 0004, `docs/widget_type_contract.md`, задачах `todo/` и комментарии миграции. Заодно уточнены термины: `Revision` больше не несёт Environment (Deploy его не выбирает, ADR 0004), а `Draft Tag state` описан как состояние вне определения Tag'а

## [0.0.50] - 2026-10-29

### Added

- Иконка загрузки PWA: круглая обрезка центра эмблемы (`cmd/combined_server/web/icon-192.png`, `icon-512.png`), центр обрезки совпадает с центром изображения, поэтому вращение go-app (`goapp-spin`) идёт вокруг эмблемы. Иконка задана в `app.Handler` (`Icon.Default`, `Large`, `Maskable`) и больше не тянется с GitHub; `make build_server` и Dockerfile копируют её в `web/`

### Changed

- Favicon (`Icon.SVG`) остался по умолчанию: go-app тянет его с GitHub

## [0.0.49] - 2026-10-27

### Added

- **Library, Library Release и Library Pin** (задача 33, ADR 0007, ADR 0012)
  - Домен `library`: агрегат `Library` (имя уникально среди живых библиотек Organization; `Rename` с ожидаемой Version, `Delete` только помечает, `StartDiscard`/`FinishDiscard` — пометка «сброс к релизу N», `CheckEditable`), события `library_created`/`library_updated`/`library_deleted`/`library_discard_started`. `WidgetType` несёт неизменяемый `LibraryID` (его события — тоже), имя уникально в библиотеке; `ResetTo` и `RecreateWidgetType` — изменения Discard. `LibraryRelease` — неизменяемый агрегат без Version: номер (последний + 1), время, заметка, копии WidgetTypes Draft (`ReleasedWidgetType`, объект-значение без Version); `CreateRelease` отказывает, если Draft не изменился (`ErrNothingToRelease`) или идёт Discard; чистая функция `DraftDiffers` — и проверка Release, и признак «есть невыпущенные изменения»; событие `library_released`
  - Домен `librarypin`: агрегат `LibraryPin` на пару (Project, Library) — номер релиза и Version, содержимое Draft проекта (`ContentEvent`: `library_pin_created`/`_updated`/`_deleted`); `Switch` с ожидаемой Version. Одна функция `ImpactOf` считает последствия смены или снятия: Widgets без WidgetType (`OrphansError`, отказ) и PortBindings, которые станут невалидны; ею пользуются и предпросмотр, и проверка
  - `Scene` проверяет Widgets по `ReleasedWidgetType` закреплённого релиза; `FindByWidgetTypeID` и `ProjectScopes.FindProjectsUsingWidgetType` удалены
  - Миграция `engineering/20261027000000_libraries`: таблицы `libraries`, `library_releases`, `released_widget_types`, `library_pins`; `widget_types.library_id` и `UNIQUE (library_id, name)`; FK `widgets.type_id → widget_types` снят (Widget ссылается на копию в релизе). Существующие WidgetTypes — в Library «Main» первой Organization (ID фиксирован), дубли имён — с суффиксом « (2)», Library Release 1, закреплён в каждом живом проекте; payload ждущих событий `widget_type_*` получает `library_id`, ждущие задания сверки одной Scene — новый ключ, задания «все сцены типа» — задания `project:<id>` каждого живого проекта с Widget этого типа
  - Блокировки строк (ADR 0012): запись WidgetType берёт строку Library `FOR SHARE` и проверяет пометку Discard (репозиторий Discard — наоборот, пишет только во время сброса); Release — тоже; закрепление берёт строку живой Library и живого Project `FOR SHARE`; смена/снятие закрепления — строку закрепления `FOR UPDATE` и проверяет Widgets проекта в той же транзакции; запись Scene, добавляющая Widget или меняющая его WidgetType, берёт строку закрепления `FOR SHARE` и проверяет тип в закреплённом релизе. Пометка удаления Library отказывает, пока её закрепляет живой проект
  - Сервисы: `LibraryService` (CRUD, Release с повтором при гонке за номер, история релизов, Discard), `WidgetTypeService` по библиотеке, `LibraryPinService` (список, добавление/смена через `PUT`, снятие, предпросмотр `impact`, WidgetTypes закреплённых релизов). Политики: `reconcile_widgets_with_pins` (по `library_pin_updated` — все Scenes проекта, по `widget_created`/`widget_updated` — Scene этого Widget, против закреплённых релизов; задание по `widget_type_updated` удалено), `discard_library` (удаляет новые WidgetTypes, восстанавливает изменённые, возвращает удалённые с теми же ID — обмен имён разрешается временным именем — и последней снимает пометку), `remove_deleted_library` (удаляет помеченную библиотеку с Draft и релизами, пока её держат закрепления удаляемых проектов — повторяет); `remove_deleted_project` удаляет и закрепления проекта
  - REST API **0.6**: `/libraries` (CRUD, `?name=`), `/libraries/{id}/widget-types` (Draft), `POST /libraries/{id}/releases` (заметка + Version), `GET /libraries/{id}/releases[/{n}]`, `POST /libraries/{id}/discard` (202); проектные `GET …/library-pins`, `PUT|DELETE …/library-pins/{library_id}`, `GET …/library-pins/{library_id}/impact?release=n`, `GET …/widget-types` (палитра). 409 смены закрепления перечисляет Widgets в `orphaned_widgets` Problem Details. События Library и Library Release — в серверный поток (`library_id` у `widget_type_*`, `release` у `library_released`), Library Pin — в поток проекта. `apiclient`: методы библиотек, `Client.Library(id)` → `LibraryClient`, закрепления и палитра в `ProjectClient`
  - growctl (манифесты **0.4**): виды `Library` и `WidgetType` (`metadata.library`; `html`, `script`, `width`, `height`, необязательные `scriptLanguage` и `inputPorts`) — apply создаёт и обновляет Draft, `--prune` удаляет WidgetTypes библиотек, чьи WidgetTypes объявлены; `spec.libraryPins` у `Project` (имя библиотеки → номер релиза) — apply добавляет или переключает закрепление, отказ перечисляет Widgets; `delete -f` удаляет WidgetTypes и библиотеки (с подтверждением, как проекты); команды `growctl release <library> [--note]`, `growctl discard <library>`, `get libraries`; пример `tests/manifests/example_library.yaml`
  - UI: режим Library — список библиотек (создать, переименовать двойным щелчком, удалить), Draft выбранной в прежнем редакторе, статус «Release N · unreleased changes», Release (заметка), Discard (подтверждение; «Discarding…», Draft только для чтения), история релизов с просмотром содержимого только для чтения; режим Project — панель Library Pins (закрепить последний релиз, сменить релиз, снять; перед сменой предпросмотр: мешающие Widgets блокируют, теряемые PortBindings требуют подтверждения), палитра — WidgetTypes закреплённых релизов, сгруппированные по библиотеке; Operation рисует Widgets по закреплённым релизам проекта
  - Seed: WidgetTypes — в Draft «Main», её релиз 1 с копиями; вторая библиотека «Extras» («Integer Bar», релиз 1); Main закрепляет обе, Line 2 — «Main»
  - Тесты: домен (Library, Release, `DraftDiffers`, `ResetTo`, `ImpactOf`, Switch), репозитории (блокировки и пометки, удаление при закреплениях, гонка за номер релиза, check при смене закрепления), сервисы и политики (Release, Discard с обменом имён, отказ смены с сиротами, предпросмотр, сверка только своего проекта), миграция и seed, REST, apiclient, growctl e2e, UI (Library, Project), Bruno

### Changed

- **Ломающее изменение API** (0.5 → 0.6; пока MAJOR = 0): `/api/v0/widget-types` удалён — WidgetTypes живут в Draft библиотеки, проект видит только закреплённые релизы; `WidgetTypeResponse` получил обязательное `library_id`; Widget может ссылаться только на WidgetType закреплённого релиза (иначе 400 `type_id`). Удаление WidgetType из Draft больше не отказывает из-за Widgets (`ErrWidgetTypeInUse` удалён) и не трогает Scenes
- `restapi.NewRouter` принимает `restapi.Services`
- Bruno: запросы WidgetTypes — под `/libraries/{id}`; новые запросы библиотек, релизов, Discard, закреплений, предпросмотра и палитры; порядок `seq` пересчитан

### Fixed

- По ревью задачи 33:
  - Миграция `20261027000000_libraries` не удаляет ждущие задания сверки «все сцены типа»: Release ничего не сверяет, и PortBindings к удалённому порту или сузившемуся type hint оставались бы в Scenes (ADR 0010). Теперь они становятся заданиями `project:<id>` проектов, где есть Widgets этого типа
  - Признак «unreleased changes» считает сервис доменной функцией `library.ReleaseStateOf` (через `DraftDiffers`), той же, что проверяет Release; SQL-копия сравнения (`FindReleaseStates`) удалена — UI больше не может предлагать Release, на который сервер отвечает 409
  - Запись Scene, ждавшая смену закрепления, больше не отвечает ложным 400 `type_id`, когда тип есть и в новом релизе: `checkPinned` сначала блокирует строку закрепления отдельным запросом, затем проверяет тип в релизе, на который она указывает после ожидания
  - `GET …/library-pins/{library_id}/impact` без `?release` для библиотеки, которую проект не закрепил, отвечает 404, как само снятие, а не 200 с пустым предпросмотром
  - `POST /libraries/{id}/releases` отвечает `created_at`, сохранённым в базе (микросекунды), как `GET` того же релиза
  - growctl не запрашивает `/libraries`, если в манифестах нет библиотек, WidgetTypes и `spec.libraryPins`: `apply`/`diff` одних тегов работают и против сервера без `/libraries`
  - Режим Library: 404 Draft или релизов удалённой библиотеки не показывает тост, а перечитывает список и выбирает другую библиотеку (`uiutil.LoadForGone`, через него теперь и `LoadOfProject`); после удаления последней выбор и LocalStorage сбрасываются
  - Остальные находки ревью — задачи 63 (лишние перезагрузки UI на события библиотек) и 64 (чистка кода)

## [0.0.48] - 2026-10-26

### Added

- **Разделение узлов и хранилищ: Tag definition / Tag state** (задача 32, ADR 0011)
  - Одна БД, три схемы, у каждой своя цепочка goose: `public` (серверная, прежняя цепочка: organizations, projects, outbox, policy_jobs; `goose_db_version`), `engineering` (widget_types, scenes, widgets, определения Tag; `engineering.goose_db_version`), `runtime` (Tag state с копией определений, последовательность `runtime.tag_state_seq`; `runtime.goose_db_version`). Миграции разложены по `migrations/public|engineering|runtime`; пакет `migrations` встраивает их и накатывает по порядку (`migrations.Up`; список цепочек неэкспортируемый, тест сверяет с ним оба docker compose), docker compose (`make db_up`, `make bruno_test`) — по сервису goose на цепочку (`db_migrations`, `db_migrations_engineering`, `db_migrations_runtime`, таблица — через `GOOSE_EXTRA_ARGS=-table=…`: entrypoint образа `goose-docker` игнорирует `command`, прежний `command` ничего не делал)
  - Миграция `20261026000000_engineering_and_runtime_schemas`: `ALTER TABLE … SET SCHEMA engineering` для четырёх таблиц, значение и Quality каждого тега — в `runtime.tags` вместе с копией определения (имя, тип, версия) и `seq` в порядке создания, колонки `value`/`quality` из `engineering.tags` удаляются, строки `tag_updated` из outbox — тоже; таблицы goose новых цепочек создаются с базовой версией. Откат возвращает таблицы в `public`, теги без состояния получают нулевое значение и `bad`
  - Домен: `tag.Tag` — неизменяемое определение (имя, TagType, Version; `Delete` возвращает новый Tag, события `tag_created`/`tag_deleted`); новый пакет `tagstate` — `TagState` (значение, Quality, `Seq`, копия `Definition`; `Initial` — из `TagType.InitialState` (нулевое значение и `Bad`; им же Engineering показывает тег без состояния); `Write` без ожидаемой Version), `TagUpdatedEvent` (это `tag.TagEvent`, в outbox не попадает), интерфейс `Repository`; `TagType.ZeroValue`
  - Runtime node: `application/runtime.TagStateService` (`SetTagValue` — quality разбирается до запроса к базе, запись последним-побеждает, вне отмены запроса, `seq` присваивается в UPDATE, после записи сам публикует `tag_updated` в EventBus через очередь `eventbus.AsyncPublisher` — рассылку SSE ведёт одна горутина, запись устройства ждёт только при полной очереди (1024), at-most-once; `FindTagStates`, идемпотентный `UpsertTagDefinitions` (состояние другого проекта или типа — ошибка, а не тихий пропуск), `RemoveTagStates`, `RemoveProjectTagStates`), репозиторий `postgres/runtime` над `runtime.tags`, читающий из `public` только projects
  - Engineering node обращается к Runtime только через порт `application.RuntimeNode`; in-process реализация — пакет `internal/server/inprocess` (общий для `main.go` и `servertest`). `TagService` читает определение и состояние через порт (определение без состояния показывает начальное, как и любое, если Runtime не ответил: чтение и удаление тегов от него не зависят); создание сразу после commit, вне отмены запроса, отдаёт определение Runtime (тег принимает значения сразу после 201; если не вышло — ошибка, хотя тег создан), затем перечитывает Draft и удаляет состояние тега, удалённого за это время, — поздний upsert не оставляет сироту; удаление сразу удаляет состояние; политика `sync_runtime_tag_definitions` по `tag_created`/`tag_deleted` (ключ `project:<id>/tag:<id>`) сводит копию к Draft — upsert, если тег есть в Draft, иначе удаление состояния. `remove_deleted_project` удаляет и состояния тегов проекта через порт, строку проекта — последней (поздний upsert удерживает строку, следующий прогон убирает его)
  - Тест границ ролей `internal/server/roles_test.go`: пакеты Engineering и Runtime не импортируют друг друга (Runtime — только общий домен), строки кода роли не называют таблицы схемы другой роли
  - REST API **0.5**: `GET /tags`, `/tags/{id}` — прежняя форма, `version` — версия определения, новое обязательное поле `seq` (0 у состояния, которое ещё не писали), оно же в `tag_updated`; `PATCH /tags/{id}/value` — без `version`, без 409, ответ `{"seq": …}`; `POST /tags` — только `name` и `type`. `apiclient.SetTagValue` без версии, возвращает `seq`
  - Манифесты growctl **0.3**: `spec.initialValue`/`spec.initialQuality` убраны; документ любой версии с ними — ошибка с объяснением, а не «unknown field»; `get tags -o yaml` их не пишет; `tests/manifests/example_tags.yaml` — `growscada/v0.3`
  - `devicelink` не отслеживает Version и не повторяет запись после 409; e2e-тесты симулятора следят за записями по `seq`. UI (`tagstate`) отбрасывает старое состояние по `seq`, а не по Version (`ForgetSeqs` при ресинке)
  - Тесты: домен `tagstate`, сервис Runtime на репозитории в памяти, репозиторий Runtime (testcontainers: upsert, запись подряд и конкурентная — разные возрастающие `seq`, проверка проекта), миграция и её откат, seed, мост и очистка проекта (включая поздний upsert), in-process `RuntimeNode` (передача полей и ошибок), события SSE: несколько записей подряд — `tag_updated` с возрастающими `seq` (замечание 2026-10-07), REST и Bruno

### Changed

- **Ломающее изменение API** (0.4 → 0.5; пока MAJOR = 0, совместимость не обещается, ADR 0005): клиенты 0.4 — `devicelink`, симулятор, growctl — обновляются вместе с сервером. Сервер отвечает 400 `unknown field` на `PATCH …/value` с `version` и на `POST /tags` с `value`/`quality`. В `tag_updated` поле `version` теперь — версия определения и при записях не растёт: читатель, отсеивавший дубли по `version`, теряет все обновления после первого; новое состояние отличают по `seq`
- `make db_up` на существующем томе: цепочка `public` накатывается с `-allow-missing` (`GOOSE_EXTRA_ARGS`) — seed делит с ней `goose_db_version`, и его версия `20990101000000` выше любой миграции, так что новая миграция иначе считалась «пропущенной»; так же в ручных командах AGENTS.md и README миграций
- Bruno: `post tags`, `post tags duplicate name`, `post tags newer client` без `value`/`quality`; `post tags invalid quality` → `post tags invalid type`; `patch tag value` без `version`, assert на `seq`; новый `patch tag value invalid quality`; `get tag by id` проверяет `seq`; порядок `seq` сдвинут
- Seed: определения тегов в `engineering.tags` (все версии 1), состояния — в `runtime.tags`; таблицы — со схемой
- ADR 0004: поправка — запись без Version, без 409 и повторов; AGENTS.md: роли и схемы, `RuntimeNode`, мост, миграции по цепочкам

### Fixed

- По ревью задачи 32:
  - Tag, созданный с 201, всегда принимает значения: определение отдаётся Runtime после commit вне отмены запроса (таймаут 10 с); если не вышло — `POST /tags` отвечает ошибкой (тег создан, политика `sync_runtime_tag_definitions` доведёт его). Раньше ошибка только логировалась, `PATCH …/value` отвечал 404, и симулятор навсегда переставал писать тег
  - Поздний upsert после удаления тега не оставляет его состояние: после upsert создание перечитывает Draft и удаляет состояние тега, удалённого за это время
  - `make db_up` на существующем томе больше не падает на «missing migrations» (`-allow-missing` для цепочки `public`)
  - Запись состояния, отменённая клиентом после коммита UPDATE, всё равно публикует `tag_updated`: запись идёт вне отмены запроса
  - `GET /tags`, `?name=`, `DELETE /tags/{id}` не отвечают 500, когда Runtime не отвечает: теги показываются с начальным состоянием, в лог — предупреждение
  - `UpsertTagDefinitions` поверх состояния другого проекта или типа — ошибка (job повторяется и виден в логах), а не тихий успех
  - Рассылка `tag_updated` по SSE ушла с горячего пути `PATCH …/value` в отдельную горутину (`eventbus.AsyncPublisher`, очередь 1024; при полной очереди запись ждёт, событие не теряется)
  - `PATCH …/value` с неверной quality отвергается до запросов к базе (400 и для несуществующего тега или проекта)
  - Правило начального состояния — в одном месте, `tag.TagType.InitialState`; `tagstate.NewDefinition` (не использовалась) удалена; список цепочек миграций неэкспортируемый, тест `TestComposeFilesApplyEveryChain` сверяет с ним оба docker compose
  - Очистка удалённого проекта держится на FK `fk_runtime_tags_project_id` между схемами — перенесено в задачу 52 (надгробие проекта в Runtime)

## [0.0.47] - 2026-10-25

### Added

- **Organization и Project: `project_id` везде** (задача 31, ADR 0006)
  - Домен: агрегат `project.Project` (имя уникально среди живых проектов Organization; `Rename` с ожидаемой Version, `Delete` только помечает), события `project_created`/`project_updated`/`project_deleted`; `organization.Organization` — пока только ключ. `Tag` и `Scene` несут неизменяемый `ProjectID`, их события и события Widget — тоже (`project.ContentEvent`). `scene.TagTypes` читаются для Project: Widget привязывается только к Tag своего проекта, чужой — «tag not found» (400 на запись, удаление привязки политикой)
  - Миграция `20261025000000_organizations_and_projects`: таблицы `organizations` (стартовая Organization) и `projects` (частичный `UNIQUE (organization_id, name) WHERE deleted_at IS NULL`); `project_id` в `tags`, `scenes`, `widgets` (FK Widget → `(scene_id, project_id)`), FK без CASCADE; `UNIQUE (project_id, name)` у тегов вместо `uq_tags_name`; всё существующее — в проект «Main»; payload ждущих строк outbox и ключи ждущих заданий политик получают проект (в payload событий содержимого — `project_id`, отдельной колонки нет); откат удаляет содержимое проектов, помеченных удалёнными, но ещё не очищенных, их задания политик и задания `remove_deleted_project`, а также события `project_*` из outbox
  - Шов `application.ProjectScopes`: `ForProject(p)` → `Tags()`, `Scenes()`, каждый запрос с `project_id`, запись агрегата чужого проекта — ошибка. `ProjectContent` проверяет, что проект жив и принадлежит Organization сервера, прежде чем открыть его содержимое; запись значения тега проверяет это в том же запросе, что читает тег (`FindByIDInLiveProject`), и читает проект отдельно только если тег не найден
  - Политика `remove_deleted_project`: по `project_deleted` удаляет Scenes, затем Tags, каждое своей транзакцией со своими событиями, строку проекта — последней; создание Tag или Scene берёт строку живого проекта `FOR SHARE`, поэтому после пометки удаления ничего не создаётся. `unbind_deleted_tag` и `reconcile_widgets_with_widget_type` работают через репозитории проекта события; обновлённый WidgetType сверяет Scenes всех проектов (`FindProjectsUsingWidgetType`)
  - Помеченный удалённым, но ещё не очищенный проект: сверка виджетов с WidgetType (`FindProjectsUsingWidgetType`) его пропускает; `unbind_deleted_tag` в нём ничего не отвязывает (сцены удаляет очистка) — задача каждого тега, удалённого очисткой, стоит два запроса по ключу без GIN-поиска сцен; удаление WidgetType, который держат только его виджеты, — 409 с деталью «WidgetType is used only by Widgets of deleted Projects, still being removed; retry shortly» (`library.ErrWidgetTypeInUseByDeletedProjects`) вместо «WidgetType is used by Widgets»
  - Списки, которые UI перечитывает на каждое событие (теги, сцены, виджеты сцены), проверяют проект в своём запросе (`FindAllInLiveProject`, `FindByIDInLiveProject` сцены) и читают проект отдельно, только если ничего не нашли
  - `project.ContentEvent` и `project.ProjectEvent` не пересекаются: события тегов, сцен и виджетов встраивают `project.Content` (метод-маркер), поэтому маршрутизация SSE, payload outbox и ключ задания очистки не зависят от порядка проверок
  - `scene.TagTypes` другого проекта — ошибка `ErrTagTypesOfAnotherProject` (`AddWidget`, `UpdateWidget`, `ReconcileWith`, который теперь возвращает ошибку), а не «тега нет»: сверка не удалит привязки из-за перепутанного проекта; `loadTagTypes` берёт проект из той же `ProjectScope`, что и репозиторий тегов (`ProjectScope.ProjectID()`)
  - UI: при смене проекта Bar держит старое SSE-соединение (только серверные события), пока не откроется новое, и шлёт только `EventProjectResync` — список проектов и WidgetTypes не перечитываются, версии Library не сбрасываются; `EventResync` — только если серверные события не приходили (соединение ещё не открыто или потеряно)
  - UI: `OnDismount` у Bar, Project и Operation сбрасывает `mounted`: `SetProject` размонтированного компонента только запоминает проект (не идёт через контекст размонтированного компонента), следующий `OnMount` его загружает
  - REST API **0.4**: `GET/POST /api/v0/projects` (`?name=`), `GET/PUT/DELETE /api/v0/projects/{project_id}`; Tags, Scenes, Widgets — под `/api/v0/projects/{project_id}/…`; несуществующий или удалённый проект — 404 проекта. SSE: `/api/v0/events` — проекты, WidgetTypes, подключения, с `?project=<id>` — плюс содержимое этого проекта (не найденный проект ничего не добавляет); `/api/v0/projects/{id}/events` — только содержимое проекта. `apiclient`: методы проектов и `Client.Project(id)` → `ProjectClient`
  - growctl (манифесты **0.2**): `--project`, контекст `growctl config use-project`/`current-project` (файл `GROWCTL_CONFIG`, по умолчанию `growctl/config.yaml` в каталоге настроек пользователя), необязательное `metadata.project` (контекст — только значение по умолчанию, как namespace в kubectl: проект манифеста главнее, теги без `metadata.project` — в единственном объявленном в манифестах `kind: Project` (при нескольких — ошибка); расхождение с `--project` или разные проекты в одной команде — ошибка), `kind: Project` (apply создаёт до тегов, `--prune` проекты не трогает, а манифест из одних проектов не удаляет теги, `delete -f` удаляет существующие проекты со всем содержимым только после подтверждения: перечень и `[y/N]`, `--yes` — без вопроса, при `-f -` обязателен), `get projects [-o yaml]`; `get tags -o yaml` пишет `metadata.project`; `kind: Project` и `metadata.project` в документе `growscada/v0.1` — ошибка с просьбой поднять `apiVersion`; apply выполняет шаги тегов в проекте, против которого строился план
  - devicelink и симулятор: обязательное `project:` в конфигурации, проект ищется по имени при старте; нет такого — `ExitConfig` (78); после 404 запроса к проекту он ищется по имени заново (удалённый — `ErrProjectNotFound`, пересозданный под тем же именем находится по новому ID; теги удалённого проекта потеряны, как удалённые теги)
  - UI: переключатель проекта в шапке (список, создание, переименование, удаление с подтверждением), выбор в LocalStorage, удалённый выбранный проект — выбирается первый, сразу по `project_deleted` или по 404 списка сцен или тегов проекта (без тоста), не дожидаясь перезагрузки списка; Project и Operation показывают выбранный проект, Library от него не зависит; Bar держит одно SSE-соединение `/events?project=<выбранный>` и переоткрывает его при смене проекта (`EventResync`, затем `EventProjectResync`); содержимое нового проекта Project и Operation грузят один раз — по `EventProjectResync`
  - Seed: содержимое — в «Main», плюс пустой проект «Line 2»; тест накатывает миграции и seed на чистую БД
  - Тесты: агрегат Project, изоляция репозиториев и сервисов, миграция существующих данных, outbox, политика удаления (включая гонку с созданием), сверка по всем проектам, REST и маршрутизация SSE по проектам, growctl (проекты, контекст, расхождение `metadata.project`), devicelink/симулятор с проектом, переключатель и смена проекта в UI
  - Bruno: запросы проектов (`get projects`, `… by name`, `… by id`, `post projects`, `… duplicate name`, `put project`, `delete project`, `get project events` — manual), пути содержимого под `projects/{{PROJECT_ID}}`, новый порядок `seq`

### Changed

- ADR 0004: пути devicelink под проектом; AGENTS.md: слой проектов, маршрутизация SSE, переключатель проекта

## [0.0.46] - 2026-10-23

### Added

- **Bruno-коллекции проверяются автоматически** (задача 30). Основными тестами остаются Go-тесты; две проверки не дают коллекциям отстать от API
  - **Статическая сверка** (`TestBrunoCollection` в `restapi` и `devicesim`, часть `make test`, без Docker — идёт и на GitVerse): новый пакет `internal/brunotest` читает запросы коллекции с подставленными переменными окружения `localhost` и обслуживает их ServeMux из паттернов маршрутов. Каждый запрос должен попасть в обработчик маршрута (ответ ServeMux редиректом 301 на неочищенный путь или путь без завершающего слэша — не попадание), каждый маршрут — быть покрыт запросом, каждое тело — строго декодироваться в тип из `requestBodies` теста. У маршрута POST/PUT/PATCH без записи в `requestBodies` и у записи без маршрута — ошибка. Неизвестная переменная в URL — ошибка, в теле остаётся как есть (как в Bruno); динамические `{{$…}}` не подставляются. Коллекция ищется от `go.mod` вверх, так что сверка работает и под `-trimpath`
  - Маршруты `restapi/router.go` и `devicesim/control.go` собраны в таблицы `routes()`, ServeMux регистрирует их циклом; паттерны не менялись
  - **Прогон** — `make bruno_test` (только GitHub CI, отдельный job): `Dockerfile` (target `server` — сервер и `web/app.wasm`, target `simulator`; у каждого своя стадия сборки, кэш сборки Go — cache mount) и `tests/api/docker-compose.yaml` (свежая БД без volume, миграции и seed, сервер, симулятор с `seed_dashboard.yaml`, healthcheck'и для `up --wait`); обе коллекции идут через `npx @usebruno/cli@4.2.1` с `--env localhost --exclude-tags=manual`, контейнеры удаляются при любом исходе, код выхода — от `bru`
  - В каждом запросе обеих коллекций — assert'ы: статус, заголовок `GrowSCADA-Schema-Version` (сервер), `application/problem+json` у ошибок, одно-два ключевых поля у чтений. Порядок `seq`: чтения, изменения, удаления в конце; версии в телах следуют этому порядку на seed-данных (Scene 1 → 4, Tag 1 → 2, WidgetType 1 → 2)
  - Теги Bruno: `manual` — вне прогона (`get_events`: SSE-поток не завершается), `invalid-body` — тело, которое сервер должен отвергнуть, без проверки тела (`post_tags_newer_client`). `post_tags_invalid_quality` проверяется полностью: тело корректно, отказ приходит из домена, assert на `quality` в `detail`
  - `TAG_NAME` коллекции `device_simulator` — seed-тег `max_connections`
- Конфигурация сервера: `DATABASE_URL` (по умолчанию прежний DSN) и `PUBLIC_API_URL` — адрес REST API для браузера, передаётся PWA как `API_URL`. Без него UI ищет API на порту 9090 хоста, с которого загружена страница (раньше — всегда `http://localhost:9090`), так что один образ работает за любым адресом. Завершающий слэш обрезается, IPv6-хост берётся в скобки
- `AGENTS.md`: `make bruno_test`, обе проверки и теги в «API Collections», правило 5 дополнено (assert'ы, порядок `seq`, `requestBodies`)

### Changed

- Ошибки REST API приходят с `Content-Type: application/problem+json` (RFC 7807) вместо `application/json`; тела те же, поэтому SchemaVersion `contract/api/v0` не поднимался. Ошибки отправляет `sendProblem(w, problem)`, статус берётся из `Problem.Status`; `sendJSONResponse` — только для успешных ответов
- Тесты `restapi` поднимают Postgres при первом обращении к БД, так что тесты без БД (сверка коллекции) идут без Docker. Если подготовка БД не удалась, контейнер останавливается до паники

### Fixed

- Healthcheck Postgres в `tests/api/docker-compose.yaml` и `tools/debug_db/docker-compose.yaml` — `pg_isready -h 127.0.0.1`: через Unix-сокет он мог отрапортовать готовность временного сервера init образа, ещё не слушающего TCP, и миграции падали с `connection refused`. В `debug_db` имена базы и пользователя берутся из `.env`
- Bruno: документация `get_events` описывает 503 как `application/problem+json`

## [0.0.45] - 2026-10-22

### Changed

- **go-app v10.1.11 → v11.0.8**: импорты `github.com/maxence-charriere/go-app/v11`. Код UI не менялся: в v11 сеттеры атрибутов (`Style`, `ID`, `Href`, `Title`, …) и `Page.Set*` принимают готовую строку, форматирующие варианты вынесены в `…f` (`Stylef`, `IDf`, …), а проект формат в них не передавал

## [0.0.44] - 2026-10-21

### Changed

- **Рефакторинг живой перезагрузки UI** (задача 29), поведение не меняется
  - `project/live_reload.go` разделён по причинам для изменений: `server_events.go` (`handleServerEvents`), `load.go` (загрузчики списков WidgetType, Scene, Widget, Tag), `scene_versions.go` (версии и состояния Scene, перечитывание устаревших Scene), `edit_fields.go` (слияние правок редакторов). `live_reload_ops_test.go` → `server_events_test.go`
  - `project/edit_fields.go`: `widgetEditFields` и новая парная `sceneEditFields` — `*EditFieldsOf`, чистые `merge*EditFields`, обёртки `set*EditFields`/`sync*`/`merge*EditingFields`; `syncSceneEditingFields` переехал из `scene_ops.go`
  - Library по той же схеме: `server_events.go` (обработчик событий вынесен из `OnMount`), `load.go` (`loadList`, `setWidgetTypes`), `edit_fields.go` (`editFields`, чистые `editFieldsOf`/`mergeEditFields`; `selectItem` и `mergeSelected` — обёртки, порты сливаются целым списком)
  - Тесты чистых функций слияния: `project/edit_fields_test.go` (Widget и Scene), `library/edit_fields_test.go`

## [0.0.43] - 2026-10-21

### Added

- **Контракт скрипта WidgetType `widgetscript` и точное значение входа `text`** (задача 28, ADR 0005)
  - Новый версионируемый контракт `contract/widgetscript/v0` (package `widgetscriptv0`), SchemaVersion **0.1**, схема `schemas/widgetscript/0.1.json`: типы `Inputs` и `Input` переехали из `uiutil`. `Input.Value` — тип `Value` (`IntegerValue`/`BooleanValue`/`StringValue`, нулевой — ключа `value` нет, `omitzero`); его `JSONSchema` перечисляет JS-типы (`integer`, `boolean`, `string`), так что смена типа, которым передаётся TagType, меняет схему. Скрипту версия не передаётся
  - `inputs.<port>` = `{ value, text }`: `text` — значение Tag строкой в канонической форме сервера для всех TagType (`"9007199254740993"`, `"true"`, сама строка). У порта без значения (нет PortBinding, Tag удалён) `text` нет — виджет отличает «значения нет» от настоящих `0`/`false`/`""`. `value` не изменился
  - Превью Library и Project строят вход по type hint порта: допустимый ввод даёт `value` и `text` в канонической форме (`+007` → `7`/`"7"`), пустой или недопустимый (`abc` в integer, `yes` в boolean) — порт без значения; в порт «any» непустой ввод идёт строкой как есть
  - `uiutil`: `NewInput(text, type)`, `NoValueInput(typeHint)`, `PreviewInputs`; `livescene.WidgetInputs` строит входы через них
  - `docs/widget_type_contract.md`: версия и схема контракта, поле `text`, предел точности Integer ±(2^53 − 1) и пример с `BigInt(inputs.<port>.text)`, расхождения превью с Operation. Doc-комментарий пакета `contract` и таблица пакетов в `AGENTS.md` дополнены
  - Тесты: границы безопасного диапазона и `math.MaxInt64`/`math.MinInt64` (`text` точен, `value` как раньше), недопустимый ввод и порт без привязки в превью, `NewInput` принимает те же тексты и даёт ту же форму, что `tag.TagType.NewTagValue` домена, `TestSchemaIsCommitted` для `widgetscript`

### Fixed

- **Integer-значения больше 2^53 теряли точность в виджете**: скрипт получал только JS `number`, точного значения не было. Теперь оно в `inputs.<port>.text`
- `jsonschema`: именованная map или slice в поле — `$ref` на запись в `$defs`, как у корня (раньше корневая map получала запись отдельной веткой, а в поле — встраивалась). Проверка опубликованной схемы (`make schemas`) работает и в репозитории, открытом через симлинк
- Bruno: `patch tag value` передаёт `version` (без него сервер отвечает 409) и описывает его в `docs`

## [0.0.42] - 2026-10-20

### Added

- **Холст Project показывает фон Scene** (задача 27)
  - `uiutil.SceneBackground(html)`: iframe фона Scene без скриптов с `pointer-events: none`; его используют и холст Project (`renderSceneCanvas`), и Operation (`livescene.View`, `renderBackground` удалён)
  - Слои холста Project снизу вверх: однотонный `var(--surface)`, фон Scene (если `background_html` не пуст), точечная сетка редактора (отдельный слой с `pointer-events: none`), виджеты. Drop, клик по пустому месту, drag/resize/rotate по-прежнему обрабатывает холст
  - Сохранённый (blur) `background_html` сразу виден на холсте; правки из другой вкладки или `growctl` приходят живой перезагрузкой
  - Тесты: разметка хелпера, фон под сеткой на холсте Project, холст без фона, сохранённый фон на холсте, фон в `livescene.View`

### Fixed

- **Фон Scene выполнял скрипты**: go-app выбрасывает атрибут с пустым значением, и `sandbox=""` не попадал в DOM. Фон теперь в `sandbox="allow-same-origin"` (`uiutil.SceneBackgroundSandbox`) — без `allow-scripts` ничего не выполняется

## [0.0.41] - 2026-10-19

### Fixed

- **Остановка сервера не ждёт SSE-соединения** (задача 26)
  - `restapi`: `eventHub.Close` закрывает каналы всех подключённых клиентов (отписка от EventBus — вне мьютекса hub'а), так что `GetEvents` сразу завершается, а клиент получает конец потока; повторный вызов ничего не делает. Раньше с открытой вкладкой UI `apiServer.Shutdown` ждал SSE-обработчики до таймаута 15 с и процесс выходил с кодом 1
  - `GetEvents` после `Close` отвечает 503 Problem Details «server is shutting down»; при превышении `MAX_SSE_CLIENTS` — по-прежнему «too many concurrent event stream connections» (`addClient` возвращает причину отказа). Контракт API не менялся
  - Хуки «Event Hub» и «API HTTP Server» остаются параллельными: на их порядок не полагаемся — `Shutdown` ждёт SSE-обработчики, пока hub дочищает outbox, и освобождается, когда hub закрывает потоки. `client_disconnected` при закрытии hub'ом публикуется как обычно и никому не доставляется
  - Тесты: открытый поток завершается после `Close` без отмены контекста; подключение после `Close` получает 503; `Close` дважды параллельно с отключением клиентов (`-race`); настоящий `http.Server` — `Close` и `Shutdown` параллельно, `Shutdown` возвращает `nil` до таймаута
  - `.mcp.json`: chrome-devtools запускается с `--headless` (работает и без `DISPLAY`, например по SSH)

## [0.0.40] - 2026-10-19

### Changed

- **Привязка тега к порту проверяет тип тега; сломанные привязки снимаются политиками** (задача 25, ADR 0010)
  - `scene`: `AddWidget`/`UpdateWidget` принимают `scene.TagTypes` — типы привязанных тегов значениями (Scene сама теги не читает) — и проверяют `PortTypeHint.Accepts`: несовпадение типа — `ErrPortTypeMismatch`, тег, которого нет, — `ErrBoundTagNotFound`. `SceneService` (получает `tag.TagRepository`) загружает типы тегов и отвечает на обе ошибки `port_bindings: invalid input` (400), как на `ErrPortNotDeclared`
  - `Scene.ReconcileWith(wt, tagTypes)` снимает у виджетов типа и привязки к тегам, которые hint больше не принимает, и к несуществующим тегам. Политика `reconcile_widgets_with_widget_type` получает `tag.TagRepository` и читает теги после Scene: смена `type_hint` у WidgetType снимает несовместимые привязки, а job на `widget_created`/`widget_updated` — привязку к тегу, удалённому параллельно с записью виджета
  - Новая политика `unbind_deleted_tag`: `tag_deleted` → job `tag:<id>`; находит Scene через `SceneRepository.FindByTagID` (поиск по JSON `port_bindings`) и в каждой отдельной транзакции снимает привязки к тегу (`Scene.UnbindTag`, событие `widget_updated` ByReconciliation, новых jobs не порождает)
  - `UpdateWidget` проверяет только привязки, которых у сохранённого виджета (того же типа) не было: привязка, сломавшаяся после создания, не мешает перемещать и править виджет — её снимает job события `widget_updated`
  - `unbind_deleted_tag` снимает привязки, только если тег на момент запуска действительно не существует
  - `TagRepository.FindTypesByIDs`: типы привязанных тегов читаются одним запросом (`WHERE id = ANY($1)`), а не по запросу на тег; общий для политик шаг «перечитать Scene, изменить, сохранить» вынесен в `changeScene`/`changeEachScene`
  - Миграции: GIN-индекс `idx_widgets_port_bindings` для поиска Scene по тегу; job `reconcile_widgets_with_widget_type` на каждый используемый WidgetType — снимает уже сохранённые сломанные привязки при первом запуске
  - UI (Project → Properties): привязка к удалённому тегу или к тегу неподходящего типа показывается выбранным пунктом «⚠ …», и выбор «— unbound —» её снимает
  - Bruno: `post widgets` и `put widget by id` привязывают порт `state` к boolean-тегу `BOOLEAN_TAG_ID`
  - Удаление тега и смена портов или `type_hint` у WidgetType по-прежнему не отклоняются из-за привязок. Контракты не менялись

## [0.0.39] - 2026-10-15

### Changed

- **События Widget несут SceneID, имена событий — в контракте** (задача 24, ADR 0009)
  - `contract/api/v0`: `Event.SceneID` (`scene_id`, `omitempty`) — у `widget_created`/`widget_updated`/`widget_deleted`; имена всех 15 типов событий — константы `apiv0.Event…` и `apiv0.EventTypes()`. SchemaVersion API 0.2 → 0.3, схема `schemas/api/0.3.json`
  - `restapi`: доменное событие переводится в `apiv0.Event` в `newAPIEvent` (`api_event.go`) по явной таблице `event.EventType → apiv0`-имя, а не через `EventType.String()`; `eventSourceID` и `eventTag` удалены. Тест требует образец на каждый тип из `event.AllEventTypes()`, использование каждой константы контракта и только имена из `apiv0.EventTypes()`; тест в `contract/api/v0` сверяет `EventTypes()` с блоком констант `Event…` (по исходнику)
  - `apiclient.ParseEvent`: разбор сообщения SSE в `apiv0.Event` за один проход; неподходящий `tag` отбрасывается, остальные поля сохраняются. Отбрасывается и `tag` не того тега, о котором событие (без ID или с чужим ID)
  - UI: режимы получают `apiv0.Event` (`eventlog.ServerEvent` и `parseEvent` удалены), строк типов событий в UI больше нет; `system_ready` — в журнале («Server ready»)
  - `eventlog.ReloadTargetsFor(apiv0.Event)`, поле `ReloadTargets.Scene`: событие Widget невыбранной Scene перечитывает только её (`GET /scenes/{id}`), выбранной — только её виджеты (они несут версию Scene; если виджетов нет, перечитывается Scene), а не весь список Scene; без `scene_id` — прежний фолбэк. Operation список Scene на событие Widget не перечитывает
  - `tag_updated` с телом тега ничего не перечитывает: Project (список и панель свойств тега), как и Operation, применяет тег сам. Общее состояние тегов — пакет `ui/tagstate` (побеждает большая Version, версии забываются на resync)
  - Project: перечитанная Scene заменяет запись в списке, только если её состояние новее (версия свойств — `sceneStates`; после отказа в сохранении она забывается): ответ, запрошенный до сохранения Scene, больше не откатывает правку. Версию выбранной Scene дают только её виджеты: более новая версия из ответа Scene или списка перечитывает виджеты (после drag), а не подставляется под показанные — иначе drag затирал чужое изменение виджета вместо 409. Версия Scene, которой ещё нет в списке, тоже учитывается. `uiutil.Versions`: `Advance`, `Forgotten`
  - `tagstate`: `Mark`, `Merge(mark, list)` — список тегов, запрошенный до resync, отбрасывается и не закрепляет версии старого сервера. Какие теги есть, решает только список: `tag_updated` тега, которого в списке нет (удалён или ещё не перечитан), его не показывает — состояние ждёт следующего списка
  - Project: список тегов, отправленный до создания тега, не снимает выделение с только что созданного
  - `eventlog.ReloadTargets.WidgetsOf(selected)` и `eventlog.ApplyToTags(tags, ev)` (тег из `tag_updated`, забывание версий на resync) — одно правило для Operation и Project вместо двух копий. `tagstate` хранит позицию тега в списке: `tag_updated` копирует только список, а не пересобирает map; `ByID()` удалён, `livescene.SceneTags` ищет теги через `Find`. Project: выбор Scene — метод `selectScene` (вкладка и тесты)
  - Project перечитывает Scene по событиям Widget по одному запросу за раз: Scene, отмеченные, пока он в полёте, ждут его, и если их несколько, вместо `GET /scenes/{id}` на каждую читается список `GET /scenes` (тем же Reloader, что и прочие загрузки списка). Изменение WidgetType, согласующее 50 Scene, — не больше двух запросов, а не 50 параллельных. Своя склейка Start/Done/rerun в `loadScene` заменена на `uiutil.Load`
  - Bruno: `get_events.yml` — пример `widget_*` со `scene_id`, `system_ready`

## [0.0.38] - 2025-10-14

### Fixed

- **CI на GitVerse: тесты с Postgres падали с `rootless Docker not found`.** Общие раннеры GitVerse не дают job доступа к Docker (монтирование `/var/run/docker.sock` раннер отбрасывает: «is not a valid volume»), и testcontainers не может поднять Postgres. Для GitVerse — отдельный `.gitverse/workflows/go.yaml` (в настройках репозитория: «Use configuration from .gitverse/workflows»): тесты гоняются без пакетов, чьим тестам нужен testcontainers (список вычисляется через `go list -test`, выводится в лог), затем `make build`. `.github/workflows/go.yaml` гоняет все тесты на GitHub: `make test` вместо `go test ./... -v`. Исходники не менялись

## [0.0.37] - 2026-10-14

### Changed

- **PWA UI ходит в REST API через один типизированный клиент** (задача 23). Поведение UI то же; Bruno-коллекции не менялись (запросы и ответы те же)
  - `contract/api/v0`: `apiv0.Problem` — тело любого ответа с ошибкой, пять полей RFC 9457 (`type`, `title`, `status`, `detail`, `instance`). restapi отвечает им; `restapi.ProblemDetails` с ручным Marshal/Unmarshal и Extensions удалён. JSON на проводе прежний. SchemaVersion API 0.1 → 0.2, схема `schemas/api/0.2.json`
  - `apiclient`: Scenes, Widgets, WidgetTypes; методы возвращают типы `apiv0` как есть (`apiclient.Tag` заменён на `apiv0.TagResponse`), создание — ответ сервера (`CreateTag` → `apiv0.CreateTagResponse`), удаление — последнее состояние ресурса. `StatusError` несёт `Method` и `apiv0.Problem` (терпимое чтение); ответ с ошибкой не в JSON (например, HTML прокси) даёт `Title` по статусу и начало тела в `Detail`. `errors.Is(ErrNotFound/ErrConflict)` как раньше. Заголовок `GrowSCADA-Schema-Version` — только у запросов с телом. Опция `apiclient.OnServerVersion` сообщает версию сервера из каждого ответа (и с ошибкой); клиент собирается под `GOOS=js GOARCH=wasm`
  - growctl: сервер с более старым API — одно предупреждение в stderr за запуск. Device simulator (`devicelink.WarnOlderServer`): запись в лог раз на смену версии сервера. Более новый сервер — без сообщений
  - UI: все вызовы (Project, Library, Operation, включая чтение списков) — через `*apiclient.Client` из Root и `uiutil.Call`; ошибки — `toast.FromError`. Удалены `uiutil.FetchJSON`, `uiutil.SendSchemaVersion`, `toast.FromHTTPError`, UI-копии DTO (`project/dto.go`, `library/dto.go`, `operation/dto.go`, `livescene/dto.go`, пакет `uidto`), `eventlog.wireMessage`, `operation.decodeTagUpdate`. SSE разбирается один раз в `apiv0.Event`, `ServerEvent.Tag` — `*apiv0.TagResponse`
  - UI: ID в состоянии — `uuid.UUID`, строки разбираются только на границах (LocalStorage, SSE, DOM/drag)
  - Project: версия Scene хранится в одном месте (`sceneVersions`) и обновляется по максимуму из любого ответа (список Scenes, Widgets, результат записи); `applyNewSceneVersion` и чтение `SceneVersion` Widget для записи убраны
  - Root: незакрываемая плашка «The server speaks API X, this interface Y. Reload the page.» с кнопкой Reload, пока версия сервера отличается от версии UI
  - Тесты: операции Project и Library гоняются в go-app `TestEngine` (`ui/uitest`) против настоящего API (`servertest.StartAPI`, очистка расширена на scenes, widgets, widget_types), включая 409, 400 с detail сервера и сетевую ошибку

## [0.0.36] - 2026-10-13

### Changed

- **Перевод ошибок в Problem Details — один модуль `restapi/errors.go`** (задача 22). URL не менялись, коды ответов — только у отрицательной версии (см. ниже); тела ответов те же Problem Details, поэтому SchemaVersion `contract/api/v0` не поднимался
  - Таблица `{sentinel, статус, ресурс, detail для 409}` и `sendError(w, r, err)`: сначала `application.ErrInvalidInput` → 400 с текстом ошибки, затем строки таблицы, иначе 500 (полная ошибка в лог, наружу общий текст). Доменные sentinel-ошибки импортирует только этот файл, лестниц `errors.Is` в обработчиках больше нет; `sendServiceError` убран
  - 404 — только для ресурса из пути: ID берётся из параметра, закреплённого за ресурсом (`{tag_id}`, `{widget_type_id}`, `{scene_id}`, `{widget_id}`). Шаблоны в `router.go` собираются из того же `resource` (`wildcard()`), так что имя параметра задано в одном месте. Если параметра нет — это ошибка сервиса, 500. Ссылка из тела запроса на несуществующее остаётся 400
  - Not-found из репозиториев и из `Scene` несёт ID (`id.NotFoundError`, `errors.Is` по-прежнему совпадает с sentinel). Если этот ID не тот, что в пути, ответ — 500, а не 404 про ресурс из пути
  - `pathUUID` разбирает ID из пути и сам отдаёт 400 вида «scene_id: 'abc' is not a valid UUID» — в snake_case, как поля API
  - Тест читает исходники `domain/` и требует, чтобы каждый sentinel был либо в таблице, либо в списке намеренно не отображённых с объяснением
  - `title` у 409 — «Conflict: Scene» (имена ресурсов из глоссария: Tag, WidgetType, Scene, Widget); `detail` — фиксированная фраза: «Scene was changed concurrently; reload it and retry» (так же для WidgetType и Tag), «WidgetType is used by Widgets», «Tag name is already taken»
  - Удаление (и снятие Widget со Scene), проигравшее гонку записи 3 раза подряд, — по-прежнему 409, но с `detail` «Scene kept changing while the request ran; retry it later»: версии в запросе нет, перезагружать нечего. `retryOnRace` помечает такой исход `application.ErrRaceLost`
  - Строка таблицы со статусом, который `sendError` не умеет отдавать, даёт 500, а не пустой 200
  - Отрицательная `version` в `PUT /scenes/{sceneId}` и `PUT /widget-types/{widgetTypeId}` — 400 «version: …» вместо 409 «changed concurrently»: такой версии не бывает, повтор с перезагрузкой не помог бы. Так же уже работал `scene_version` у Widget

### Fixed

- **`detail` у 404 не раскрывает внутренний текст.** Все вызовы `NewNotFound` передавали текст ошибки вместо типа ресурса, и клиент видел, например, «error on find scene by id in repository: scene not found with ID '…' not found». Теперь — «Scene with ID '…' not found»

## [0.0.35] - 2026-10-11

### Changed

- **Согласование Widget с изменённым WidgetType — политика по событию** (задача 21, ADR 0008). REST API и контракт `contract/api/v0` не менялись
  - `UpdateWidgetType` сохраняет только WidgetType; синхронный цикл по Scene (`removeOrphanedPortBindings`, `reconcileScene`) убран из сервиса, `NewWidgetTypeService` больше не принимает `SceneRepository`. Привязки Widget к удалённым портам снимает политика вскоре после commit
  - **Задания политик**: таблица `policy_jobs (policy, job_key, generation, attempts, next_attempt_at, last_error)`. Диспетчер outbox не выполняет политики: в транзакции, удаляющей доставленные строки, он ставит задание каждой политике, которой нужно событие (`Policy.JobKey`), и будит раннер. Задание с тем же ключом сливается со стоящим (`generation + 1`), поэтому событие, пришедшее во время выполнения, приводит к повторному прогону. Новое задание созревает сразу, а слившееся событие не сокращает паузу задания, которое падает: повтор всё равно прочитает текущее состояние
  - `outbox.PolicyRunner`: одна горутина, задания по очереди; успех — `DELETE … WHERE generation = g`, а если за время прогона слилось событие — задание созревает сразу со сброшенными `attempts` и `last_error`; сбой — `slog`, `last_error` и пауза 1 с → ×2 → 5 мин, повторы бесконечны. Паника политики и превышение таймаута попытки (`WithAttemptTimeout`, по умолчанию 5 мин) считаются сбоем, так что зависшая или падающая политика не роняет сервер и не задерживает остальные задания. `Run` будится диспетчером и таймером ближайшего задания, `RunDueOnce` — для тестов. При остановке текущее задание прерывается и выполняется после старта. Реестр политик собирается в `main.go`; раннер передаётся диспетчеру через `Dispatcher.SetPolicyRunner`, потому что репозитории политик будят уже созданный диспетчер
  - `application.Policy` (`Name`, `JobKey`, `Run`) и `NewWidgetReconciliationPolicy`: слушает `widget_type_updated` (ключ `widget_type:<id>` — все Scene с Widget этого типа), `widget_created` и `widget_updated` (ключ `widget_type:<id>/scene:<id>` — только Scene этого Widget, так что правка Widget не обходит весь проект); собственные `widget_updated` политики (`WidgetUpdatedEvent.ByReconciliation`, только снимают привязки; хранится во внутреннем кодеке outbox, в SSE не передаётся) заданий не ставят. WidgetType читается при выполнении, после Scene, и перечитывается вместе с ней при гонке записи, чтобы не снять привязку к порту, добавленному тем временем (удалён — задание выполнено) и согласует каждую Scene в своей транзакции (`retryOnRace`, удалённая Scene пропускается). Сбой одной Scene не останавливает остальные (`errors.Join`), повтор проходит уже согласованные вхолостую
  - `WidgetCreatedEvent`/`WidgetUpdatedEvent` несут обязательный WidgetType (`scene.WidgetTypedEvent.TypeID`; не `WidgetTypeID`, иначе событие Widget стало бы и `library.WidgetTypeEvent`); кодек outbox хранит `widget_type_id`. Строку, записанную без него, кодек читает в свой приватный `scene.WidgetEvent` без WidgetType: она доставляется в SSE, но задания не ставит. Домен о старом формате не знает
  - **UI Project**: `409` на правке Scene или Widget — тост «Scene changed … Repeat your edit», Scenes и Widgets перечитываются сразу, не дожидаясь SSE; правка не переприменяется

### Fixed

- Project: значения Simulate Inputs стирались при переходе на другую вкладку (например, в Library, чтобы поменять WidgetType) и обратно: Root при каждом открытии вкладки создаёт новый `Project`, а значения жили в компоненте. Теперь они хранятся в состоянии приложения go-app (`project:simInputs`, без `Persist`): переживают смену вкладок, но не перезагрузку страницы
- Library: значения Input Data превью так же стирались при смене вкладки, а ещё при выборе другого WidgetType. Теперь они хранятся по WidgetType в состоянии приложения (`library:inputValues`) и переживают и то и другое
- Ошибка посреди согласования (не конфликт) отвечала клиенту `500` после сохранённого WidgetType, а несогласованные Scene навсегда оставались с привязками к исчезнувшим портам. Теперь задание повторяется до успеха
- Widget, добавленный или изменённый параллельно с удалением порта (WidgetType прочитан до удаления), навсегда сохранял привязку к удалённому порту. Теперь его событие ставит задание политики, и привязка снимается
- Project: после переключения Scene Widget новой Scene мог показывать документ Widget прежней (например, значение из Simulate Inputs несвязанному Widget того же типа). go-app переиспользует компонент iframe, а `OnUpdate` искал элемент по новому id раньше, чем тот попадал в DOM. `uiutil.IframeSrcdoc` (поле компонента iframe, `Mount`/`Update`) теперь работает с самим элементом компонента (`JSValue()`) и не перезагружает iframe, если документ не изменился (превью Project и Library). Последний заданный документ хранится в компоненте, так что `OnUpdate` не копирует `srcdoc` из DOM через границу WASM/JS

## [0.0.34] - 2026-10-07

### Changed

- **Агрегат и его события в одной транзакции — outbox** (задача 20, ADR 0008). Поведение REST API не изменилось, кроме удаления при гонке записи (см. Fixed)
  - **Домен**: события переехали в пакеты агрегатов (`tag`, `scene`, `library`); `domain/event` — только база (`Event`, `Base`, `EventType`, `EventTimestamp`, `Recorder`) и не импортирует агрегаты. Агрегаты копят pending-события (`PendingEvents()`): `CreateTag`/`CreateWidgetType`/`CreateScene` записывают Created, `Reconstitute…` (для репозиториев) — ничего; `Tag.SetValue` — `tag_updated` с состоянием, которое сохранит `Save`; `Scene.Update`/`AddWidget`/`UpdateWidget`/`RemoveWidget` — свои события, `ReconcileWith` — `widget_updated` на каждый изменённый Widget; `Delete()` — Deleted (Scene — ещё `widget_deleted` на каждый Widget). События Widget несут SceneID (в контракт он пока не выходит — задача 24), но `SceneEvent` не удовлетворяют (метод-маркер): `type switch` не зависит от порядка `case`
  - `WidgetType` неизменяемый, как Scene: `Update(expected, …)` сам сверяет ожидаемую Version (`ErrWidgetTypeConflict`), а неверное определение отдаёт как `ErrInvalidWidgetType`; `CheckVersion`, `Delete()`. Сверка Version для `Tag.SetValue` пока остаётся в сервисе (задача 32)
  - `EventBus` и события Client/System переехали из домена в `interface/eventbus` (вместе с заглушкой MQTT): это события доставки
  - **Outbox**: таблица `outbox (seq, tx_id, type, payload jsonb, created_at)`, внутренний кодек событий (не контракт), общий шаблон `Save`/`Delete` Postgres-репозиториев (`aggregateStore`): CAS по загруженной Version, классификация NotFound/Conflict и запись событий — одна транзакция. Пишущие транзакции не ждут друг друга: `tx_id` (id транзакции, `pg_current_xact_id()`) задаёт порядок доставки
  - `DeleteByID` репозиториев заменён на `Delete(ctx, agg)` с CAS; у Tag — без CAS: после создания у тега меняется только значение, и удаление от него не зависит
  - **Диспетчер** `outbox.Dispatcher`: сигнал `Notify` после commit (`repositories.NotifyOnCommit`) + опрос раз в секунду; строки по `(tx_id, seq)` → `EventBus.Publish` (SSE) → один DELETE на пачку доставленных, at-least-once; строка ждёт, пока не закончатся все более старые транзакции (`pg_snapshot_xmin`), поэтому изменение, сделанное после commit другого, доставляется после него. DELETE доставленной строки не прерывается остановкой диспетчера. `DrainOnce` для тестов. Запускается в `main.go`, при остановке outbox дочищается до закрытия Event Hub, пока SSE-клиенты ещё подключены
  - **Сервисы** больше не держат `EventBus` и не публикуют событий: разбор входа → загрузка → метод агрегата → `Save`/`Delete`. Согласование Widget в `UpdateWidgetType` пока синхронное, но события каждой изменённой Scene теперь попадают в outbox (политика — задача 21)

### Fixed

- Событие могло уйти до commit или потеряться, а сервис мог его забыть (`UpdateWidgetType` не публиковал события согласованных Scene). Теперь откат ⇒ ничего не доставлено, commit ⇒ доставлено
- Удаление Scene, пока в неё добавляют Widget, не публиковало `widget_deleted` для добавленного Widget
- Удаление (WidgetType, Scene, Widget) при гонке записи перечитывает и повторяет (`retryOnRace`); проиграв гонку 3 раза подряд, отвечает `409`. Раньше удаление Widget при гонке сразу отвечало `409`. Удаление Tag гонки не знает и `409` не отвечает: тег удаляется, даже если Device пишет в него значения

## [0.0.33] - 2026-10-05

### Changed

- **Изменения Widget — через агрегат Scene** (задача 19). REST API и контракт `contract/api/v0` не менялись
  - `scene.Scene` неизменяемая: `AddWidget(expected, w, wt)`, `UpdateWidget(expected, w, wt)`, `RemoveWidget(widgetID)`, `Update(expected, name, size, background)` и `ReconcileWith(wt)` возвращают новую Scene, исходная не меняется; `FindWidget(widgetID)` ищет Widget сцены. Агрегат сам ловит конфликт правки (ожидаемая Version ≠ текущей → `ErrSceneConflict`) и проверяет, что `TypeID` Widget совпадает с переданным WidgetType и PortBinding ссылаются только на его InputPort (`ErrWidgetTypeMismatch`, `ErrPortNotDeclared`; Widget с занятым ID — `ErrWidgetAlreadyExists`)
  - `SceneRepository` сокращён до `NextID`, `NextWidgetID`, `FindByID`, `FindAll`, `FindByWidgetTypeID` (Scene целиком), `Save`, `DeleteByID`. `Save` пишет Scene с её Widget в одной транзакции: CAS по строке `scenes` (Version повышается один раз на сохранение), затем INSERT новых / UPDATE изменённых / DELETE исчезнувших Widget. Чтение — одна read-only транзакция `REPEATABLE READ`, Scene и её Widget всегда из одного момента; `DeleteByID` блокирует строку сцены перед чтением Widget
  - `SceneService`: каждое изменение — загрузка Scene (нет → `404`) и WidgetType (нет → `400`) → метод агрегата → `Save`; Version в сервисе не сверяется
  - `removeOrphanedPortBindings` сервиса WidgetType: `FindByWidgetTypeID` → `ReconcileWith` → `Save` изменившихся сцен (без атомарности и событий — задача 21)
  - **Пакеты домена**: Widget и его value objects (`WidgetName`, `Position`, `Origin`, `Rotation`, `TransformationMatrix`, `PortBinding`, `ErrWidgetNotFound`) переехали в `domain/scene`, размер Widget стал `scene.WidgetSize`. `domain/widget` переименован в `domain/library` (WidgetType, InputPort, PortTypeHint, шаблон, скрипт); размер по умолчанию WidgetType остаётся `library.Size`

### Fixed

- Удаление Widget поднимало Version сцены без CAS (`UPDATE scenes SET version = version + 1`) и не замечало правку сцены между чтением и записью. Теперь удаление идёт через `Save`: при гонке записи — `409`, параллельная правка не затирается
- `FindByID` читал Scene и её Widget двумя запросами вне транзакции и мог увидеть их в разные моменты

### Removed

- `SceneRepository.AddWidget`, `UpdateWidget`, `DeleteWidget`, `FindWidgetsBySceneID`, `FindWidgetByID`, `FindWidgetsByTypeID`, тип `scene.WidgetInScene`; в Postgres-репозитории `bumpSceneVersion`; в `SceneService` — `validateWidgetType` и `widgetSaveError`

## [0.0.32] - 2026-10-05

### Added

- задачи 19–24 по итогам архитектурного ревью 2026-10-05 (только backlog, код не менялся): изменения Widget через агрегат Scene (19), единица работы — состояние, Journal и события в одной транзакции (20), атомарное согласование Widget при изменении WidgetType (21), перевод ошибок в Problem Details одним модулем restapi (22), типизированный клиент сервера для PWA UI (23), SceneID в событиях Widget и типы событий в контракте (24)

### Changed

- **Перенумерация backlog**: задачи 19–51 сдвинуты на 6 и стали 25–57, следующий свободный номер — 58. Ссылки «задача N» и `Blocked by:` в `todo/backlog/`, `todo/done/` и ADR 0003, 0004, 0006 пересчитаны; исторические упоминания («бывшая задача N», тикеты расформированного пакета 36) и прошлые записи этого файла не менялись

## [0.0.31] - 2026-10-04

### Changed

- **Невалидный ввод в REST API → `400 Bad Request`** (задача 18): ошибки разбора и валидации значений из запроса (имя, `TagType`, `Quality`, значение тега, в т. ч. несовместимое с типом, `ScriptLanguage`, `PortTypeHint`, имя порта, размеры, origin, версия сцены, matcher имени) раньше уходили в `500`, теперь — `400` с причиной в `detail`
  - сцены: `422 Validation Error` заменён на `400`
  - ссылка из тела на несуществующий WidgetType и порт, не объявленный на WidgetType, — `400`; тип виджета теперь проверяется всегда, а не только при port bindings. Если в том же запросе нет сцены или виджета из URL, ответ — `404`: несуществующий тип проверяется внешним ключом уже после них
  - `404` остаётся только для ресурса из URL, `409` — для конфликта версии или имени
  - devicelink и device simulator больше не ретраят такой ввод: `IsTransient` считает временными только `5xx`
- **WidgetType виджета всегда существует**: `DELETE /api/v0/widget-types/{id}` для типа, который используют Widgets, отвечает `409 Conflict` и ничего не удаляет (раньше тип удалялся, а у его Widgets очищались port bindings). Гарантирует внешний ключ `fk_widgets_type_id` (`ON DELETE RESTRICT`); репозитории отдают `widget.ErrWidgetTypeInUse` и `widget.ErrWidgetTypeNotFound` (распознаются оба SQLSTATE блокировки удаления: `23503` до PostgreSQL 18 и `23001` restrict_violation с 18 — тесты идут на 16, dev-БД на 18)
  - миграция `20261004000000_widgets_type_id_foreign_key` **удаляет Widgets с несуществующим типом**, оставшиеся от прежних удалений, и поднимает `version` их сцен, чтобы клиент со старой версией не записал поверх изменившейся сцены
  - `GLOSSARY.md`: правило в термине WidgetType; Bruno: `delete_widget_type_by_id` описывает `409`, новый пример `post_tags_invalid_quality` (`400`)
- **UI больше не подменяет ввод**: свойства сцены (ширина/высота ≤ 0 → 1920/1080) и геометрия виджета (размер ≤ 0 → 10, origin обрезался до 0–1, нечисловое поле → 0) молча сохраняли не то, что ввёл пользователь, а в поле оставалось введённое. Теперь число уходит как есть: сервер отвечает `400`, toast показывает причину, поле возвращается к сохранённому значению; нечисловое или пустое поле, а также `NaN`/`Inf` — toast «Client Error» с именем поля, запрос не отправляется (`project/number_fields.go`)
- application-слой помечает ввод вызывающего общим `application.ErrInvalidInput`; ошибки восстановления сохранённых данных маркер не получают и остаются `500`. Хендлеры маппят ошибки сервисов общим `sendServiceError`
- application-слой: у Create и Update сцены, WidgetType и виджета один вход (`appdto.SceneInput`, `WidgetTypeInput`, `WidgetInput`) — только поля ресурса; ID и ожидаемая версия (в т. ч. `sceneVersion` виджета) передаются аргументами. Поля разбирает одна функция на ресурс (`parseScene`, `parseWidgetType`, `parseWidget`), ошибка называет поле из JSON-контракта: `cannot update widget: size: widget width must be positive, got 0`

### Fixed

- Project: клик по виджету на холсте без перетаскивания (и перетаскивание, вернувшееся в исходную точку) отправлял PUT с теми же значениями и поднимал версию сцены — у других клиентов это давало лишние перезагрузки и ложные конфликты версий. Теперь виджет сохраняется, только если его геометрия изменилась (`dragChangedWidget`)

### Removed

- `scene.ErrSceneValidation`, `widget.ErrWidgetInvalidInput` (заменены `application.ErrInvalidInput`), `restapi.NewValidationError` и `restapi.TypeValidation`
- `appdto.CreateSceneInput`/`UpdateSceneInput`, `CreateWidgetTypeInput`/`UpdateWidgetTypeInput`, `CreateWidgetInput`/`UpdateWidgetInput` (заменены едиными входами)

## [0.0.30] - 2026-10-04

### Added

- **Модель многоарендности и библиотек** (grilling-сессия 2026-10-04, только документация, код не менялся):
  - ADR 0006 «Every operational entity is keyed by Project, every Project by Organization»: сервер хостит много проектов в обеих ролях узла, `project_id` вводится во все таблицы до появления Journal и Revision, пока одна стартовая Organization из seed
  - ADR 0007 «Libraries are shared by pinned release and copied into the Revision on Deploy»: Library принадлежит Organization и может быть публичной, у неё свой Draft (EditLock, undo/redo) и Release → неизменяемый Library Release; проект закрепляет по одному релизу от нескольких Library, при Deploy используемое содержимое копируется в Revision
  - `GLOSSARY.md`: термины Organization, Project, Library, Library Release, Release, Device Gateway, Device Token
- задачи 25 (Organization и Project), 27 (Library и Library Release), 28 (Device, Device Gateway, Device Token) переписаны по решениям сессии; задача 45 уточнена: «Runtime node со вшитой Revision»

### Changed

- **Перенумерация backlog**: номер задачи — её место в очереди, следующий свободный — 52:
  - 18–24 — мелкие независимые задачи (бывшие 34, 35, 38, 39, 40, 37, 33)
  - A, 25–26 — Organization и Project; разделение хранилищ engineering/runtime, Origin
  - B, 27–30 — Library Release; Device; ProjectFile, Deploy, Revision, Discard; Rollback
  - C, 31–35 — EditLock; DraftChange и undo/redo; ProjectFile в UI; групповое выделение; направляющие
  - D, 36–41 — Journal; Player и Operation; Checkpoint; History; PlaybackFile; наведение мыши
  - E–F, 42–44 — runtime-сущности и Adopt; User; права доступа
  - 45–51 — Runtime node со вшитой Revision; разнесение узлов по сети; Command; Alert; звуки; Computation; Theme
- пакет `36_project_undo_redo_history` расформирован: его тикеты стали обычными задачами со строками `Status:` и `Blocked by:`, решения Q1–Q30 из `map.md` разнесены по `## Comments` соответствующих задач. Отменены Q7 (Library — часть Draft проекта) и Q21 (один неявный проект)
- ProjectFile в UI (33) стоит после DraftChange (32): открытие файла — одна DraftChange
- Deploy (29) и Rollback (30) больше не зависят от Journal: их записи в Journal добавляет задача 36
- задача 36 (Journal) поглотила бывшую 24 «Запись и воспроизведение событий»; задача 42 (Adopt) зависит от DraftChange (32); EditLock (31) и DraftChange (32) работают и для Draft Library; User (43) — член Organization, а не владелец проектов
- ссылки «задача N» в `todo/backlog/`, `todo/done/` и в этом файле пересчитаны на новые номера
- `GLOSSARY.md`: Device — конфигурация оборудования в проекте, а не процесс; Draft, EditLock и Discard — у проекта и у Library; имя Tag уникально внутри Project; у Tag не больше одного поставщика значения
- ADR 0003: имя Tag уникально внутри Project; ADR 0004: поправка — Device как конфигурация, Device Gateway исполняет её по Device Token, токен переживает Deploy и Rollback и отзывается только явно

### Removed

- `todo/backlog/36_project_undo_redo_history/` (`map.md` и 13 тикетов) и `todo/backlog/24_event_record_playback.md` — содержимое перенесено в задачи 26, 29–33, 36–40, 42, 46

## [0.0.29] - 2026-10-04

### Added

- **Версионирование контрактов** (задача 17, ADR 0005, термин SchemaVersion в `GLOSSARY.md`): у каждого из четырёх контрактов — серверный API, формат проекта, формат операционной записи, манифесты growctl — своя SchemaVersion `MAJOR.MINOR`, у всех стартовая `0.1`. Пока MAJOR = 0, совместимость не обещается
- публичный пакет `contract/`: `SchemaVersion`, по подпакету на контракт и MAJOR (`contract/api/v0`, `contract/project/v0`, `contract/record/v0`, `contract/manifest/v0`). Форматы проекта и записи пока заготовки — только версия и корневое поле `schema_version`
- JSON-схемы генерируются из Go-типов в `schemas/<контракт>/<MAJOR.MINOR>.json` (`make schemas`) и коммитятся. `make test` падает, если типы изменились, а версию не подняли, и если новая MINOR (при MAJOR ≥ 1) не только добавляет необязательные поля
- заголовок `GrowSCADA-Schema-Version`: сервер ставит его в каждый ответ, включая поток событий и CORS preflight (разрешён и виден браузеру); `apiclient` отправляет его в каждом запросе, UI — только в запросах с телом: сервер читает заголовок лишь при разборе тела, а на GET нестандартный заголовок стоил бы CORS-preflight. Сервер отдаёт `Access-Control-Max-Age: 7200`, чтобы браузер не повторял preflight перед каждым POST/PUT/PATCH/DELETE
- опубликованная схема контракта не меняется: старые версии замораживаются хешем в `schemas/<контракт>/frozen.sha256` (тесты падают, если файл изменился), а `make schemas` отказывается переписывать версию, которая уже есть в ветке `path`, и просит поднять SchemaVersion. Без git или без ветки `path` — предупреждение вместо проверки. Пока MAJOR = 0, `make schemas` при новой MINOR печатает NOTE о том, что после 1.0 потребовало бы MAJOR, а тест совместимости MINOR прогоняется на парах `0.x` и только логирует
- Bruno: запрос «post tags newer client»
- ADR 0005 «Contracts carry a MAJOR.MINOR SchemaVersion; readers skip, receivers reject»; в `AGENTS.md` — правило «изменил тип в `contract/` — подними SchemaVersion и запусти `make schemas`», цель `make schemas` и раскладка пакетов `contract/` и `schemas/`

### Changed

- **Ломающее изменение API**: префикс `/api/v1` → `/api/v0` (MAJOR в пути). Запрос на `/api/v1/...` получает `410 Gone` с подсказкой перейти на `/api/v0` (до 1.0, ADR 0005); Bruno: «get pre-versioning path»
- **Ломающее изменение control API симулятора**: `/api/v1/device/...` → `/control/device/...`. У control API нет контракта (ADR 0005), и цифра в пути только выдавала его за версию 1; Bruno-коллекция `device_simulator` обновлена
- **Ломающее изменение манифестов**: `apiVersion: growscada/v0.1` вместо `growscada/v1`; `growscada/v0` читается как `v0.0`. На `growscada/v1` growctl отвечает подсказкой заменить её на `growscada/v0.1` (до 1.0). Манифест более новой MINOR growctl отклоняет с просьбой обновить growctl, а не с ошибкой «unknown field»
- сервер разбирает тела запросов строго: `400` на незнакомое поле, на повторный ключ (в том числе отличающийся только регистром — `encoding/json` молча оставлял последний) и на данные после JSON-значения; раньше всё это молча отбрасывалось. Если клиент в заголовке указал версию новее серверной и ошибка — незнакомое поле, в ответе написано «обновите сервер»
- UI отправляет тела запросов типами `contract/api/v0` вместо собственных копий, так что расхождение с сервером ловит компилятор; `uidto.InputPortDTO` — алиас `apiv0.InputPort`. Битый ID в состоянии UI показывается как «Client Error», запрос не отправляется
- задача 17 переписана по итогам grilling-сессии 2026-10-03 и перенесена в `todo/done/`; пути в ADR 0004 и Bruno-коллекции `growscada_server` — `/api/v0`
- `SchemaVersion` контрактов (`apiv0`, `manifestv0`, `projectv0`, `recordv0`) — функция `SchemaVersion()` вместо экспортированной переменной: пакет `contract` публичный, и любой импортёр мог её изменить
- JSON-типы REST API перенесены из `internal/server/interface/restapi/restdto` в `contract/api/v0` (`InputPortDTO` → `InputPort`, `PortBindingDTO` → `PortBinding`). Сообщение потока событий — тип `apiv0.Event`, преобразования в `appdto` — в `restapi/dto_mapping.go`

### Fixed

- генератор JSON-схем паниковал на поле-указателе на тип с методом `JSONSchema` по значению, а `*uuid.UUID` и `*time.Time` получали схему без `format`
- генератор JSON-схем встраивал поля не так, как кодировщики: для yaml встраивал анонимное поле без `,inline` (yaml.v3 вкладывает его под ключом), для json не встраивал встроенную структуру с тегом без имени (`json:",omitempty"`), пропускал встроенную неэкспортированную структуру с именем в теге и пропускал поле с тегом `json:"-,"` (это поле с именем `-`). Закоммиченные схемы не изменились
- генератор JSON-схем объявлял обязательным поле с `json:",omitzero"`, которое encoding/json опускает, и описывал поле с `json:",string"` его Go-типом, хотя в JSON оно строка: теперь `string` с `pattern` для целых и `enum` для bool

## [0.0.28] - 2026-09-29

### Added

- **Режим Operation** (задача 16) — Operator наблюдает Scene с живыми значениями тегов, без управления:
  - вкладки со всеми Scene (только переключение), выбранная запоминается в LocalStorage (`operation:sceneID`); холст 1:1: помещающаяся Scene стоит по центру без полос прокрутки, не помещающаяся прокручивается от левого верхнего угла; под виджетами — `background_html` Scene в iframe без скриптов
  - виджет получает значения привязанных тегов; iframe загружается один раз, новые значения приходят через `postMessage` (без мерцания и сброса анимаций), после handshake `ready`; iframe пересоздаётся только при смене кода WidgetType
  - поверх виджета — маркер худшей Quality его портов: `Uncertain` — жёлтая рамка, `Bad` — красная рамка с затемнением; порт без привязки или с удалённым тегом считается `Bad`
  - изменения конфигурации (Scene, Widget, WidgetType, создание/удаление тегов) подхватываются на лету; значения тегов применяются прямо из `tag_updated`, состояние тегов сливается по Version
  - при разрыве SSE — плашка «No connection to the server» и маркеры `Uncertain`; после переподключения конфигурация и теги загружаются заново
  - компонент `livescene.View` рисует Scene по готовому состоянию и ничего не загружает (для будущего Player)
- `docs/widget_type_contract.md` — контракт скрипта WidgetType

### Changed

- **Ломающее изменение контракта WidgetType**: `render(inputs)` получает каждый порт объектом — `inputs.<port>.value` вместо `inputs.<port>`; поля входа в будущем только добавляются. `render` может вызываться многократно и должен быть идемпотентным. Seed-виджеты переписаны («String Ticker» больше не перезапускает бегущую строку при том же тексте)
- SSE-сообщение `tag_updated` несёт полное состояние тега в поле `tag` (форма `GET /api/v1/tags/{id}`); доменное `TagUpdatedEvent` содержит Tag
- `eventlog.Bar` публикует состояние связи `eventlog.StateDisconnected` и передаёт состояние тега в `ServerEvent.Tag`

## [0.0.27] - 2026-09-28

### Fixed

- Вкладки сцен (и списки типов виджетов, виджетов сцены) периодически меняли порядок: как и у тегов, запросы шли без `ORDER BY`, и после `UPDATE` строка уезжала в конец. Теперь `SceneRepository.FindAll`, `FindWidgetsBySceneID`, `FindWidgetsByTypeID` и `WidgetTypeRepository.FindAll` возвращают в порядке создания (`ORDER BY pk_id`): при переименовании сцена остаётся на своём месте
- Project: после двойного клика по вкладке сцены (переименование) другие сцены показывались пустыми до перезагрузки страницы. Результат асинхронного запроса диспатчился через контекст элемента, породившего событие; go-app молча отбрасывает `Dispatch`, если этот элемент уже размонтирован (вкладка превратилась в поле ввода), и `Reloader` виджетов навсегда оставался в состоянии загрузки. Теперь все асинхронные операции Project и Library диспатчат через контекст компонента (`compoCtx`, захватывается в `OnMount`); заодно больше не теряется новая версия сцены после переименования
- Список тегов (Project → Tags, `GET /api/v1/tags`, в том числе с `?name_pattern=`/`?name_regex=`) перемешивался при обновлении значений тегов: Postgres отдавал строки без `ORDER BY` в порядке хранения, а `UPDATE` перемещает строку. Теперь `TagRepository.FindAll` сортирует по имени

### Added

- **Симулятор устройства нижнего уровня** (задача 15) — `cmd/device_simulator/`, логика в `internal/devicesim/`; собирается в `make build` (или отдельно `make build_simulator`) → `bin/growscada_device_simulator/`, `make run_simulator` — с примером `tests/device_simulator/example.yaml`, `make run_simulator_dashboard` — с `tests/device_simulator/seed_dashboard.yaml`, который управляет сидовыми тегами сцены «Main Dashboard» (результат виден в Operation сразу после `make db_up`):
  - один процесс — одно виртуальное устройство; YAML-конфиг (`server`, `control`, `autostart`, `defaultInterval`, `tags`), неизвестные поля — ошибка; `--config`, `--server` / `DEVSIM_SERVER`, `--control`; завершение через `gracedown`, как у сервера: по SIGINT/SIGTERM сначала останавливается control-API, затем устройство (текущие записи дописываются); коды выхода sysexits — 0 при штатной остановке, 66 — нет файла конфига, 78 — ошибка конфига, отсутствующий тег или несовместимый паттерн, 69 — не удалось занять порт control-API
  - теги только по имени (ADR 0003), сам их не создаёт и не удаляет; control-API поднимается сразу, а сервер симулятор ждёт без ограничения по времени, как настоящее устройство (пока ждёт: `GET /api/v1/device` → `"state":"connecting"`, запросы к тегам → 503); отсутствующий тег или несовместимый с типом паттерн — ошибка без единой записи
  - паттерны `constant`, `ramp`, `sine`, `random_walk`, `step`; integer — все, boolean и string — `constant` и `step`; генерируемые числа округляются для integer, а литералы `constant`/`step` должны быть целыми; интервал на тег, запись каждый тик
  - control-API (`:9191`): `POST /api/v1/device/start|stop` (пауза, время паттернов не идёт; `stop` отвечает, когда записи в полёте завершены), `GET /api/v1/device` (у тегов — `overridden`, `lost`), `POST /api/v1/device/tags/{name}/pattern` (смена на лету), `POST|DELETE /api/v1/device/tags/{name}/quality` (липкое переопределение качества и сброс); ошибки — Problem Details
  - тесты: unit (паттерны, конфиг, устройство и control-API с фейковым линком), интеграционные и сквозные против настоящего REST API и Postgres в testcontainers, включая конфликт версий (409)
  - Bruno: коллекция `tests/api/bruno_collections/device_simulator/` для ручных запросов к control-API
- `internal/devicelink` — контракт поставки значений тегов от Device к серверу (ADR 0004): резолв по имени, учёт версии, при 409 — перечитать и повторить один раз, 404 — тег выбывает; свои перечисления `TagType` и `Quality` (без нулевого-«unknown», ADR 0001), без зависимости от `server/domain`
- `internal/server/servertest` — общий тестовый стенд: настоящий REST API поверх Postgres в testcontainers с ленивым стартом контейнера; на нём e2e-тесты devicelink, devicesim и growctl (unit-тесты growctl больше не требуют Docker)
- `make build` разбит на `build_server` (WASM + сервер), `build_growctl` и `build_simulator` и собирает все три; `make run` собирает только сервер
- GitHub CI собирает все бинарники через `make build`

### Changed

- Bruno: коллекция сервера переименована `tests/api/bruno_collections/growscada/` → `growscada_server/` (в Bruno — «growscada server»), коллекция симулятора называется «growscada device simulator»; в Bruno это две отдельные коллекции, каждая открывается своей папкой
- `internal/` разложен по приложениям: слои сервера (`domain`, `application`, `infrastructure`, `interface`) перенесены в `internal/server/`; поведение не менялось
- REST-клиент вынесен из `growctl` в общий пакет `internal/apiclient` (добавлены `GetTag`, `SetTagValue`, `ErrConflict`); `growctl` использует его
- AGENTS.md: правило зависимостей инструментов вне `server/` по ролям — Engineer-инструменты (`growctl`) могут импортировать value objects из `server/domain`, сторона Device (`devicelink`, `devicesim`) — только контракт REST

## [0.0.26] - 2026-09-27

### Added

- **`growctl`** (задача 14) — CLI для декларативного управления тегами через YAML-манифесты в стиле kubectl (`apiVersion: growscada/v1`, `kind: Tag`, `metadata.name`, `spec.type`/`initialValue`/`initialQuality`):
  - `cmd/growctl/`, логика в `internal/growctl/` (разбор манифестов, чистый плановщик, REST-клиент, команды на `spf13/cobra`); собирается в `make build` → `bin/growctl/`
  - работает только через REST API: `--server` / `GROWCTL_SERVER`, по умолчанию `http://localhost:9090`
  - `apply -f <файл|каталог|->` создаёт недостающие теги; `initialValue`/`initialQuality` применяются только при создании; `--prune` удаляет теги сервера, которых нет в манифесте
  - `diff -f` показывает план, только изменения: `+ tag/<имя> (<type>)` — создание, `- tag/<имя>` — prune (код выхода 0 — нет изменений, 1 — есть, 2 — ошибка); `delete -f` удаляет теги манифеста по имени (отсутствующий тег — строка `tag/<имя> not found`, код выхода 0)
  - `-f` можно повторять; одно имя тега, объявленное в манифестах дважды, — ошибка; `get tag` — синоним `get tags`
  - план строится до первого изменения: невалидный манифест или смена `type` у существующего тега — ошибка без изменений; сбой REST-запроса останавливает выполнение, повторный `apply` доводит дело до конца
  - тесты: unit-тесты манифеста и плановщика, сквозные тесты через `httptest` с настоящим REST-роутером и Postgres в testcontainers
  - `tests/manifests/example_tags.yaml` — пример манифеста
  - шаблоны имён `--pattern` (`*`, `?`) / `--regex` (Go RE2): `apply`/`diff --prune --pattern` удаляет только лишние теги внутри шаблона; `delete --pattern|--regex` удаляет подходящие теги с подтверждением (`--yes` — без); `get tags [--pattern|--regex] [-o table|yaml]` — список тегов, `-o yaml` — манифест для `apply -f`. Клиент фильтрует ответ сервера и сам, поэтому сервер без поддержки фильтров не расширит выборку. Ошибки использования — код выхода 2, в том числе `apply`/`diff` без `-f` и неизвестная подкоманда (`growctl foo`, `growctl get foo`)
  - ошибка HTTP-ответа называет метод и полный URL запроса (`GET http://…/api/v1/tags: 404 Not Found`), поэтому неверный `--server` сразу виден
  - клиент сверяет имя в ответе `GET /tags?name=`, поэтому корректно работает и с сервером, который фильтр ещё не поддерживает
- `GET /api/v1/tags?name=<имя>` — фильтр по имени тега; ответ той же формы `{"tags":[...]}` с 0 или 1 элементом; Bruno: `get_tags_by_name.yml`
- `TagRepository.FindByName`, `TagService.FindTagByName`
- Поиск тегов по шаблону и регулярному выражению — `GET /api/v1/tags?name_pattern=<шаблон>` (`*` — любая последовательность, `?` — один символ, совпадение со всем именем) и `GET /api/v1/tags?name_regex=<regex>` (Go RE2, совпадение в любом месте имени; `^`/`$` для привязки). Ответ — `{"tags":[...]}` с любым числом тегов; невалидное выражение и более одного из `name`/`name_pattern`/`name_regex` — 400. `?name=` остаётся строгим поиском. Домен: value object `tag.TagNameMatcher` (`NewTagNamePattern`, `NewTagNameRegex`), ошибка `tag.ErrInvalidTagNameMatcher`; сервис: `FindTagsByNamePattern`, `FindTagsByNameRegex`. Bruno: `get_tags_by_name_pattern.yml`, `get_tags_by_name_regex.yml`. Имена параметров — константы `restdto.TagsQueryName`/`TagsQueryNamePattern`/`TagsQueryNameRegex`, общие для сервера и `growctl`
- UI обновляется по SSE-событиям сервера — изменения из `growctl`, другой вкладки или по REST видны без перезагрузки страницы:
  - `eventlog.Bar` (единственное SSE-соединение приложения) рассылает каждое событие go-app действием `eventlog.ActionServerEvent`
  - Library перечитывает список типов виджетов по `widget_type_*`; Project — палитру типов (`widget_type_*`), сцены (`scene_*`), виджеты выбранной сцены и сцены (`widget_*`: изменение виджета повышает версию сцены без события сцены) и теги (`tag_*`) — `internal/interface/ui/project/live_reload.go`
  - редакторы сливают свежие данные по полям (`uiutil.MergeField`): поле, которое пользователь не трогал, следует за сервером, несохранённая правка сохраняется; удалённый извне объект снимает выделение
  - перезагрузки склеиваются (`uiutil.Reloader`): пока идёт запрос, новые события ставят в очередь не более одной повторной загрузки; перезагрузка виджетов откладывается до конца перетаскивания/поворота/ресайза; версия сцены в UI никогда не откатывается назад
  - `uiutil.FetchJSON` — общий GET+JSON с ошибкой в виде toast

### Fixed

- Library: textarea редактора не очищалась, когда текст становился пустым (удаление типа, тип с пустым скриптом) — go-app не сбрасывает свойство `value`; добавлен `textareaValueSync`
- Library: список не мигает «Loading...» при перезагрузках, плейсхолдер только при первой загрузке
- Bruno `put_scene_by_id.yml`: добавлено обязательное поле `version` (без него запрос всегда получал 409)

### Changed

- **BREAKING: имя тега уникально** (`docs/adr/0003-tag-name-is-unique-natural-key.md`, `GLOSSARY.md`):
  - миграция `20260926000000_unique_tag_names.sql` — ограничение `uq_tags_name` вместо индекса `idx_tags_name` (упадёт, если в БД уже есть дубликаты имён)
  - новая доменная ошибка `tag.ErrTagNameTaken`; репозиторий отображает в неё нарушение `uq_tags_name`
  - `POST /api/v1/tags` с занятым именем отвечает `409 Conflict` (Problem Details); Bruno: `post_tags_duplicate_name.yml` (имя `is_active` из сида)
- `make run`: chromium запускается с `--disable-background-networking`
- `AGENTS.md`: описаны сборка `growctl` в `make build` и `cmd/growctl/` в структуре слоёв; в таблицу целей добавлена существующая цель `make bench`

## [0.0.25] - 2026-09-24

### Added

- `internal/domain/event/event_type_test.go` — `TestAllEventTypes_ListsEveryDeclaredEventType`: разбирает `event_type.go` и сверяет число объявленных значений `EventType` с `len(AllEventTypes())`; ловит значение, не добавленное в таблицу имён (такие события молча не доходили бы до SSE-клиентов, т. к. хаб подписывается на `AllEventTypes()`)
- `tests/api/bruno_collections/growscada/post_widget_types.yml` — пример порта с `"type_hint": ""` («любой тип»)
- `docs/agents/` (`issue-tracker.md`, `triage-labels.md`, `domain.md`) и раздел «Agent skills» в `AGENTS.md` — конфигурация agent skills: задачи ведутся в `todo/`, стандартные triage-метки, single-context `GLOSSARY.md` + `docs/adr/`

### Removed

- **BREAKING: качество тега `simulated`** — Quality описывает надёжность значения, а не его происхождение; имитатор (задача 15) поставляет обычные `good`/`bad`/`uncertain`, а пометка «это имитатор» станет метаинформацией `Device` (задача 28):
  - `internal/domain/tag/tag_quality.go` — удалён `TagQualitySimulated`
  - `internal/infrastructure/postgres/migrations/20260923000000_remove_simulated_tag_quality.sql` — существующие теги с `simulated` переводятся в `good`, `chk_tags_quality` пересоздаётся без `simulated` (`Down` возвращает ограничение, данные не трогает)
  - `POST /api/v1/tags` и `PATCH /api/v1/tags/{id}/value` с `"quality": "simulated"` теперь отклоняются (пока 500, как и любая ошибка валидации домена — маппинг в 400 вынесен в задачу 18)
  - тестовые данные: тег `is_cached` получил качество `uncertain`
- **Заглушки `Unknown` в доменных перечислениях** (`docs/adr/0001-no-unknown-enum-sentinels.md`) — удалены `TagQualityUnknown`, `TagTypeUnknown`, `ScriptLanguageUnknown`, `EventTypeUnknown`; нулевое значение перечисления невалидно (`String()` → `"invalid"`, `IsValid()` → `false`), разбор `"unknown"`, `""` и любой нестандартной строки возвращает ошибку

### Changed

- `internal/domain/tag/tag.go` — `NewTag` отклоняет невалидные тип и качество, `SetValue` — невалидное качество (раньше нулевое значение останавливал только CHECK в БД)
- `internal/domain/widget/widget_type.go` — `NewWidgetType` отклоняет невалидный `ScriptLanguage`
- `internal/domain/event/event_type.go` — `EventType` из `int` + `iota` стал struct value object с приватным полем, как остальные перечисления; снаружи пакета нельзя получить произвольное значение приведением
- **BREAKING: `InputPort` type hint** — новый value object `widget.PortTypeHint` (`internal/domain/widget/port_type_hint.go`): либо «любой тип» (`AnyTypeHint()`), либо конкретный `TagType` (`TypeHintFor`); методы `IsAny`, `TagType`, `Accepts`; «любой тип» в REST и в JSON-колонке `input_ports` — это `""`:
  - в ответах API «любой тип» теперь `"type_hint": ""` вместо `"unknown"`
  - запрос с `"type_hint": "unknown"` отклоняется (пока 500, см. задачу 18)
  - UI (`uidto.TypeHintLabel`, `project/render_properties.go`) больше не обрабатывает `"unknown"`
- Глоссарий (`GLOSSARY.md`, `AGENTS.md`): Quality — `Bad`/`Uncertain`/`Good`, TagType без `Unknown`, новый термин Device
- `internal/domain/event/event_type.go` — имена типов событий заданы одной таблицей `eventTypeNames`, из неё выводятся `AllEventTypes()`, `NewEventType()` и `String()` (вместо двух параллельных `switch` на 15 веток); добавлен `IsValid()`, как у остальных перечислений. Строковые имена и порядок `AllEventTypes()` не изменились. Round-trip тест теперь покрывает все значения `AllEventTypes()`, добавлена проверка уникальности значений и имён
- `docs/adr/0001-no-unknown-enum-sentinels.md` — оговорено исключение: нулевое значение `PortTypeHint{}` валидно и означает «любой тип» (это отдельный value object, а не перечисление)
- Форматирование `gofmt` в `internal/domain/tag/tag.go`, `internal/domain/widget/{input_port,port_binding}.go`, `internal/interface/ui/library/*.go`, `internal/interface/ui/project/{preview,project}.go` — без изменения поведения

## [0.0.24] - 2026-09-18

### Added

- **Протокол доменных событий клиент-сервер на базе SSE** — сервер транслирует все события `EventBus` подключённым клиентам через `GET /api/v1/events`, клиент отображает их в новой панели лога в статус-баре:
  - `internal/interface/restapi/event_hub.go` — `eventHub`: подписывается на `event.AllEventTypes()` ровно один раз при создании и раздаёт каждое опубликованное событие всем текущим SSE-клиентам (`broadcast`); у каждого клиента канал `messages` с буфером `eventClientBufferSize = 64`; при переполнении буфера клиент считается медленным и отключается (`closed = true`, канал закрывается), не блокируя `Publish` для остальной части приложения; `addClient`/`removeClient` учитывают лимит `maxClients`
  - `internal/interface/restapi/events.go` — `EventsHandlers.GetEvents`: хендлер `GET /api/v1/events`; проверяет лимит подключений до апгрейда до SSE (при превышении — обычный JSON 503, а не уже отправленный поток); пишет SSE-комментарий `: connected\n\n` сразу после заголовков; публикует `ClientConnectedEvent`/`ClientDisconnectedEvent` (через `defer`) при подключении/отключении; история не воспроизводится — только события, случившиеся после подключения
  - `internal/domain/client` (новый пакет) — `Client` — маркерный интерфейс-идентичность для событий жизненного цикла SSE-подключения; поведение агрегата пока не определено
  - `internal/domain/event/client_event.go`, `client_connected_event.go`, `client_disconnected_event.go` — `ClientEvent` (база с `ClientID() id.ID[client.Client]`), `ClientConnectedEvent`, `ClientDisconnectedEvent`; `EventType` расширен значениями `EventTypeClientConnected`/`EventTypeClientDisconnected`
  - `internal/domain/event/event_type.go` — `AllEventTypes()`: возвращает все известные `EventType` кроме `EventTypeUnknown`; используется хабом для универсальной подписки без ручного перечисления типов при добавлении новых
  - `internal/domain/event/event_bus.go` — `EventBus.Subscribe` теперь возвращает `Subscription` (непрозрачный хендл `{eventType, id}`), добавлен метод `Unsubscribe(Subscription)`; внутреннее хранилище обработчиков сменилось с `map[EventType][]EventHandler` на `map[EventType]map[uint64]EventHandler`, чтобы обработчики (не сравнимые как `func`) можно было адресно удалять; `internal/infrastructure/mqtt/event_bus_mqtt.go` обновлён под новую сигнатуру (декоратор проксирует `Subscribe`/`Unsubscribe` во внутреннюю шину)
  - `internal/interface/restapi/problems.go` — `NewServiceUnavailableError()`: RFC 7807 503 для случая превышения лимита одновременных SSE-подключений
  - `internal/interface/restapi/router.go` — `NewRouter` принимает `event.EventBus` и `maxSSEClients`; регистрирует `GET /api/v1/events`; `APIRouter.Close()` отписывает хаб от шины при graceful shutdown
  - `cmd/combined_server/main.go` — конфигурация через `github.com/alexflint/go-arg`: `serverArgs.MaxSSEClients` (env `MAX_SSE_CLIENTS`, по умолчанию 100); хаб зарегистрирован в `gracedownManager` как отдельный интерфейс `"Event Hub"` с таймаутом остановки 15s
  - `internal/interface/ui/eventlog` (новый пакет) — компонент **`Bar`**: статус-бар (текущее время, обновляется раз в секунду через `ctx.After`, + кнопка «Events») и раскрываемая панель лога событий под ним:
    - Подключение к `/api/v1/events` через нативный `EventSource` держится всё время жизни компонента независимо от того, открыта ли панель; браузер сам переподключается при разрыве, история не восстанавливается ни на клиенте, ни на сервере
    - Буфер `entries` ограничен `maxEntries = 500`; старые записи вытесняются независимо от активных фильтров (фильтры влияют только на отображение)
    - Панель — фиксированная высота на `visibleRows = 10` строк с внутренним скроллом, не меняется в зависимости от количества записей
    - Фильтры по категориям (`event_types.go`): `Tag`, `Widget`, `WidgetType`, `Scene`, `Value`, `Client`, `Other`; каждый тип события из `eventTypeInfoByType` маппится на категорию и человекочитаемый шаблон описания; по умолчанию скрыта только категория `Value` (поштучные обновления значений тегов иначе засоряют лог); состояние фильтров персистируется в `localStorage` (`eventlog:filters`)
    - `internal/interface/ui/root/root.go` — `eventlog.NewBar(r.apiServerURL)` смонтирован в `Root.Render()`
  - `tests/api/bruno_collections/growscada/get_events.yml` — запрос для ручной проверки SSE-потока
  - `internal/domain/event/event_test.go` — тесты на `Subscribe`/`Unsubscribe` (адресное снятие обработчика без затрагивания остальных подписчиков того же типа)
  - `internal/interface/restapi/events_test.go` — интеграционные тесты SSE-хендлера: подключение/формат сообщений, лимит `maxSSEClients` → 503, публикация `client_connected`/`client_disconnected`, отключение медленного клиента
  - `internal/interface/ui/eventlog/bar_test.go`, `event_types_test.go` — юнит-тесты фильтров по умолчанию и маппинга типов событий на категории/описания

### Changed

- `go.mod` — новая прямая зависимость `github.com/alexflint/go-arg` (+ indirect `github.com/alexflint/go-scalar`); `github.com/pressly/goose/v3`, `github.com/testcontainers/testcontainers-go`, `github.com/testcontainers/testcontainers-go/modules/postgres` переведены из indirect в прямые зависимости
- `Makefile` — добавлена цель `bench`: `go test -run=^$ -bench=. -benchmem ./...`

## [0.0.23] - 2026-09-14

### Changed

- **`Widget` стал entity внутри агрегата `Scene`** вместо самостоятельного aggregate root — устраняет несогласованность между доменной моделью и слоем событий при удалении сцены (виджеты удалялись каскадом в БД, но `WidgetDeletedEvent` не публиковался, ломая аудит в History). Единая версия `Scene.Version()` теперь служит границей optimistic concurrency и для сцены, и для всех её виджетов.
  - `internal/domain/widget/widget.go`: `Widget` лишился `SceneID()` и `Version()`; `NewWidget()` больше не принимает `sceneID`/`version`
  - `internal/domain/widget/widget_repository.go` удалён; ошибки `ErrWidgetNotFound`/`ErrWidgetInvalidInput` перенесены в новый `internal/domain/widget/errors.go` (`ErrWidgetConflict` убран — конфликт версии теперь выражается через `scene.ErrSceneConflict`)
  - `internal/domain/scene/scene.go`: `Scene` получил `Widgets() []widget.Widget`; `NewScene()` принимает список виджетов
  - `internal/domain/scene/scene_repository.go`: `SceneRepository` — единственная точка доступа к виджетам (`NextWidgetID`, `FindWidgetsBySceneID`, `FindWidgetByID`, `AddWidget`, `UpdateWidget`, `DeleteWidget`, `FindWidgetsByTypeID`); `AddWidget`/`UpdateWidget` проверяют версию сцены и атомарно её инкрементируют, `DeleteWidget` инкрементирует без проверки версии (удаление не рискует затереть чужие изменения)
  - `internal/infrastructure/postgres/repositories/widget_repository_postgres.go` удалён, логика объединена в `scene_repository_postgres.go`; общие функции сканирования/сериализации виджета вынесены в новый `widget_scan.go`. Точечные операции (`AddWidget`/`UpdateWidget`/`DeleteWidget`) делают целевой `UPDATE`/`INSERT`/`DELETE` по одной строке `widgets` + инкремент `scenes.version` в одной транзакции, не перечитывая все виджеты сцены. `DeleteByID` сцены читает её виджеты до каскадного удаления (для последующей публикации событий) в той же транзакции, что и сам `DELETE`
  - `internal/application/scene_application_service.go`: `SceneService` получил методы работы с виджетами (`FindWidgetsBySceneID`, `FindWidgetByID`, `CreateWidget`, `UpdateWidget`, `DeleteWidgetByID`), включая валидацию port bindings против `WidgetType` (перенесена из `WidgetService`). `DeleteSceneByID` публикует `WidgetDeletedEvent` для каждого виджета сцены перед `SceneDeletedEvent`
  - `internal/application/widget_application_service.go` удалён — `WidgetService` упразднён
  - `internal/application/widget_type_application_service.go`: `removeOrphanedPortBindings` теперь использует `SceneRepository.FindWidgetsByTypeID` (кросс-сценовый запрос, единственное легитимное исключение из правила "виджет только через сцену") вместо `WidgetRepository`
  - `internal/application/appdto/widget.go`: `Widget`/`CreateWidgetInput`/`UpdateWidgetInput` получили `SceneVersion` вместо `Version`; `SceneID` убран из `Create/UpdateWidgetInput` (передаётся отдельным параметром, приходит из URL)
  - REST API: `/api/v1/widgets/*` заменён на вложенный `/api/v1/scenes/{sceneId}/widgets` и `/api/v1/scenes/{sceneId}/widgets/{widgetId}` (breaking change); `internal/interface/restapi/widgets.go`, `router.go`, `restdto/widget.go` обновлены; тело запроса/ответа виджета несёт `scene_version` вместо `version`, `scene_id` в теле запроса убран (уже есть в пути)
  - `cmd/combined_server/main.go`: убрана отдельная wiring для `widgetRepository`/`widgetService`
  - UI (`internal/interface/ui/project/`): `dto.go`, `widget_ops.go` обновлены под вложенные URL и версионирование через сцену (`currentSceneVersion()`/`applyNewSceneVersion()` вместо версии на виджет); `scene_ops.go`/`render_scene.go` — удаление сцены теперь подтверждается диалогом с числом виджетов (`confirmDeleteScene()`)
  - `internal/infrastructure/postgres/migrations/20260601000002_drop_widgets_version.sql` — колонка `widgets.version` удалена как более не используемая (единственная версия — `scenes.version`); каскад `ON DELETE CASCADE` на `widgets.scene_id` не тронут
  - `internal/infrastructure/postgres/test_data/20990101000000_insert_test_data.sql`: `INSERT INTO widgets` больше не указывает колонку `version` — без этого seed падал с ошибкой (колонки не существует) и 3 примера виджетов (`status-circle-main`, `speedometer-max-connections`, `string-ticker-status`) не создавались при `make db_up`
  - `tests/api/bruno_collections/growscada/`: `post_widgets.yml`, `put_widget_by_id.yml`, `delete_widget_by_id.yml`, `get_widget_by_id.yml` переведены на вложенные URL и `scene_version`; `get_widgets.yml` (глобальный список виджетов) удалён — эндпоинта больше нет

### Removed

- Глобальные REST-эндпоинты `GET/POST /api/v1/widgets`, `GET/PUT/DELETE /api/v1/widgets/{id}` — виджет больше не адресуется независимо от сцены

## [0.0.22] - 2026-06-14

### Added

- `internal/interface/ui/uiutil/srcdoc.go` — новый пакет с общими утилитами для sandboxed-iframe превью:
  - **`BuildSrcdoc()`** — объединённая версия `buildSrcdoc()` из `library` и `project`; дополнительно инжектирует `<style>html,body{background:<bgColor>}</style>` в `<head>` iframe, чтобы фон совпадал с темой приложения; защищает от преждевременного закрытия `<style>` через `styleCloseRE.ReplaceAllString`
  - **`SetIframeSrcdoc()`** — общий хелпер для прямого DOM-присвоения `srcdoc` по `id`
  - **`IframeBgColor()`** — читает CSS-переменную `--bg` из `document.documentElement` через `getComputedStyle`; fallback `"#ffffff"`
  - **`IframeTextMuted()`** — читает `--text-muted`; fallback `"#888888"`
  - **`IsValidJSIdentifier()`** — проверяет, что строка соответствует `^[a-zA-Z_$][a-zA-Z0-9_$]*$`; используется для валидации имён портов перед добавлением
  - **`PortValueToJS()`** — перенесена из `library` и `project` (ранее приватная); логика без изменений

### Changed

- `internal/interface/ui/library/render_editor.go`:
  - Приватные `buildSrcdoc()` и `portValueToJS()` удалены — заменены вызовами `uiutil.BuildSrcdoc()` и `uiutil.PortValueToJS()`
  - `previewFrame.setSrcdoc()` удалена — `OnMount`/`OnUpdate` теперь вызывают `uiutil.SetIframeSrcdoc()`
  - iframe получает `allowtransparency="true"` и `background: transparent` для сквозной прозрачности
  - Валидация имени порта расширена: добавлена проверка `uiutil.IsValidJSIdentifier(name)` перед добавлением порта
  - Превью Library теперь передаёт `uiutil.IframeBgColor()` в `BuildSrcdoc` — фон iframe синхронизирован с темой

- `internal/interface/ui/project/preview.go`:
  - Приватные `buildSrcdoc()`, `portValueToJS()`, `setSrcdoc()` удалены из файла — заменены `uiutil`-аналогами
  - `widgetPreviewFrame` и `widgetThumbnailFrame` получают `allowtransparency="true"` и `background: transparent`

- `internal/interface/ui/project/render_scene.go`:
  - `renderSceneCanvas()` вычисляет `bg` и `textMuted` один раз через `uiutil.IframeBgColor()` / `uiutil.IframeTextMuted()` и передаёт их в `renderWidget()`
  - Подпись `renderWidget()` расширена параметрами `bg, textMuted string`; fallback-текст ненайденного виджета использует `textMuted` вместо захардкоженного `#888`

- `internal/interface/ui/project/render_widget_types.go`:
  - Миниатюры в панели типов строятся через `uiutil.BuildSrcdoc()` с явным `bg`

- `internal/interface/ui/project/render_properties.go`:
  - `buildSrcdoc()` в обработчике `OnChange` заменена на `uiutil.BuildSrcdoc()` с `uiutil.IframeBgColor()`

- `internal/interface/ui/library/library.go`:
  - Добавлено поле `ThemeMode string` (экспортированное — go-app отслеживает изменения полей компонента для планирования ре-рендера)
  - `OnMount` подписывается на состояние `"theme"` через `ctx.ObserveState`

- `internal/interface/ui/project/project.go`:
  - Аналогично `library`: добавлено `ThemeMode string` и `ctx.ObserveState("theme", &p.ThemeMode)`

- `internal/interface/ui/root/root.go`:
  - `setTheme()` теперь публикует тему в `ctx.SetState("theme", mode)`; то же происходит при `OnMount` для восстановления сохранённой темы — компоненты `Library` и `Project` получают уведомление и перерисовываются с актуальным фоном

## [0.0.21] - 2026-06-08

### Added

- `internal/interface/ui/project/preview.go` — три новых компонента и два вспомогательных функции:
  - **`widgetPreviewFrame`** — sandboxed-iframe для отображения виджета на холсте сцены; `pointer-events:none` пропускает мышиные события к прозрачному drag-оверлею; `OnMount`/`OnUpdate` устанавливают `srcdoc` через прямое DOM-присвоение (обход ограничения браузера на перезагрузку `<iframe srcdoc>` через `setAttribute`)
  - **`widgetThumbnailFrame`** — sandboxed-iframe для миниатюры в панели типов; рендерит виджет в нативном размере и уменьшает через CSS `transform: scale(N)` с `transform-origin: 0 0`; контейнер получает `overflow:hidden` с уже масштабированными размерами
  - **`simInputField`** — управляемое поле ввода для панели симуляции; `OnMount`/`OnUpdate` устанавливают DOM `value` через `getElementById` + `ctx.Defer` (без этого go-app vdom вызывает `removeAttribute` для пустой строки, оставляя поле с устаревшим текстом при переключении между виджетами)
  - **`buildSrcdoc()`** — формирует HTML-документ для `<iframe srcdoc>`: вставляет `htmlTemplate` в `<body>`, оборачивает `script` в `<script>`, генерирует JS-вызов `render(inputs)` на основе входных портов; добавляет `Content-Security-Policy: connect-src 'none'` для блокировки `fetch()`/XHR из iframe; экранирует `</script>` в пользовательском JS, чтобы содержимое поля не могло прервать `<script>` блок
  - **`portValueToJS()`** — конвертирует строковое значение из поля ввода в JS-литерал с учётом `typeHint` (`integer`, `boolean`, `string`); использует `strconv.ParseInt`, `json.Marshal` — инъекция невозможна

- `internal/interface/ui/project/render_properties.go` — панель «Simulate Inputs»:
  - Секция появляется для виджетов с входными портами; показывает поле `simInputField` на каждый порт с меткой типа
  - `OnChange` напрямую обновляет `srcdoc` у iframe `w-preview-<id>` через DOM без ожидания ре-рендера родительского компонента (в go-app v10 обработчики на дочернем компоненте не планируют ре-рендер родителя)
  - `capturedWT widgetTypeItem` захватывается один раз в начале `renderWidgetProperties` и используется в замыкании `OnChange`
  - Тестовые значения хранятся в `p.simInputs[widgetID][portName]` — не сохраняются на сервер

- `internal/interface/ui/project/project.go`:
  - Поле `simInputs map[string]map[string]string` — in-memory хранилище тестовых значений для симуляции
  - Метод `widgetTypeByID(id string) (widgetTypeItem, bool)` — линейный поиск по `p.widgetTypes`

### Changed

- `internal/interface/ui/project/render_scene.go` — виджеты на холсте теперь отображаются как sandboxed-iframe вместо текстовой метки:
  - Содержимое виджета: `widgetPreviewFrame` (`pointer-events:none`) + прозрачный `div`-оверлей для drag/click + рамка выделения (`pointer-events:none`, показывается при `isSelected`)
  - При ненайденном `WidgetType` (тип ещё не загружен или удалён) показывается имя виджета как текстовый fallback через экранированный HTML
  - Добавлен импорт пакета `"html"` для `html.EscapeString`

- `internal/interface/ui/project/render_widget_types.go` — панель типов виджетов теперь показывает миниатюры `widgetThumbnailFrame` вместо строк «имя + размеры»:
  - Масштаб миниатюры рассчитывается так, чтобы вписать виджет в ширину панели; при превышении `maxThumbH = 120` px пересчитывается scale и `thumbW` с повторной проверкой минимальной ширины 50 px
  - При отсутствии `HtmlTemplate` показывается пунктирный прямоугольник с текстом «no HTML»

- `internal/interface/ui/project/dto.go` — структура `widgetTypeItem` дополнена полями `HtmlTemplate string` и `Script string` для передачи шаблона и скрипта в компоненты предпросмотра

## [0.0.20] - 2026-06-07

### Added

- `internal/interface/ui/library/render_editor.go` — компонент **`inputDataField`**: тонкая обёртка над полем ввода в панели «Input Data»:
  - Решает проблему go-app vdom: при пустом значении фреймворк вызывает `removeAttribute`, а не присваивает `input.value = ""`, из-за чего браузер не очищает поле при переключении WidgetType
  - `OnMount` и `OnUpdate` присваивают свойство `value` напрямую через `getElementById` + `ctx.Defer`; `OnUpdate` пропускает DOM-запись если `Val` не изменился (`lastVal` guard)
  - Поле `Lib *Library` заменено на `OnChange func(string)` — компонент самодостаточен и не держит обратную ссылку на родителя

- `internal/interface/ui/library/render_editor.go` — компонент **`previewFrame`**: тонкая обёртка над sandboxed-iframe предпросмотра:
  - Решает ограничение браузера: обновление атрибута `srcdoc` через `setAttribute` не перезагружает iframe; работает только прямое присвоение JS-свойства `iframe.srcdoc`
  - `OnMount` и `OnUpdate` вызывают `setSrcdoc()`, которая делает `getElementById(p.ID).Set("srcdoc", p.Srcdoc)`
  - Поле `ID string` вынесено в структуру — исключает хардкод и позволяет иметь несколько экземпляров

- `internal/infrastructure/postgres/test_data` — новый тег `status_message` (`6ba7b819-...`): строковый тег для демонстрации виджета String Ticker

### Changed

- `internal/infrastructure/postgres/test_data/20990101000000_insert_test_data.sql` — примеры WidgetType заменены на наглядные JavaScript-виджеты:
  - **Boolean Circle** (`0001`) — SVG-круг, зеленеет при `inputs.state = true`
  - **Integer Speedometer** (`0002`) — SVG-спидометр 0–100: дуга и стрелка, цвет меняется по порогам (зелёный / оранжевый / красный)
  - **String Ticker** (`0003`) — бегущая строка с marquee-анимацией при переполнении контейнера
  - Удалены старые примеры: Pressure Gauge, Temperature Indicator, Boolean Lamp (CSS/div), Flow Meter (Python), Level Sensor (Lua)
  - DOWN-миграция расширена: теперь удаляет и старые ID (`0003`, `0004`, `0005-old`) — безопасно при их отсутствии

- `internal/interface/ui/library/render_editor.go` — `buildSrcdoc` теперь вызывает `render(inputs)` вместо `update(inputs)` (исправлено несоответствие с именем функции во всех примерах)

- `tests/api/bruno_collections` — приведено в соответствие с актуальными тестовыми данными:
  - `environments/localhost.yml` — `WIDGET_TYPE_ID` исправлен: `0005-...-0001` → `0001-...-0001` (Boolean Circle)
  - `post_widget_types.yml`, `put_widget_type_by_id.yml` — скрипты переименованы: `function update` → `function render`
  - `post_widgets.yml`, `put_widget_by_id.yml` — порт `pressure`/`temperature` → `state`; убрана несуществующая переменная `{{TAG_ID_2}}`

## [0.0.19] - 2026-06-04

### Added

- `internal/interface/ui/toast` — новый пакет **toast** для централизованного отображения ошибок (`toast.go`):
  - `Container` — фиксированный overlay, монтируется один раз в `root.go`; слушает глобальный экшн `toast.add` через `ctx.Handle`
  - `Problem` — структура RFC 9457: `Title`, `Status`, `Detail`, `Instance`, `Method` (HTTP-метод из `resp.Request`, не сериализуется)
  - `FromHTTPError(resp)` — читает и закрывает тело ответа, парсит как RFC 9457; при ошибке парсинга формирует синтетическую запись из `http.StatusText` и сырого тела
  - `NetworkError(err)` — обёртка для транспортных ошибок (connection refused, timeout и т.п.)
  - Анимация на трёх фазах (`entering → visible → exiting`) через вложенные `ctx.After`; CSS-кейфреймы инжектируются в `<head>` через `injectToastCSS()` один раз при монтировании `Root`
  - Защита от регрессии фазы: `setPhase` игнорирует любые переходы, если таблица уже в фазе `exiting`
  - Отображение: HTTP-метод (моноширинный), статус-код + заголовок, время создания (правый верхний угол), детальное сообщение, instance URI
  - Цвета статусов и фон таблички управляются CSS-переменными темы (`--toast-bg`, `--toast-shadow`, `--toast-err-*`, `--toast-warn-*`, `--toast-info-*`, `--toast-muted-*`) — корректно адаптируются при переключении светлой/тёмной/авто темы

- `internal/interface/ui/root` — добавлены переменные `--toast-*` в `lightVars()` и `darkVars()` (`root.go`); светлая тема: белый фон `rgba(252,252,252,0.98)`, тёмные цвета текста; тёмная тема: прежний полупрозрачный тёмный фон

### Changed

- `internal/interface/ui/library` (`library.go`, `render_list.go`, `widget_type_ops.go`) — поле `fetchErr string` и его inline-рендеринг удалены; все HTTP-ошибки (`http.Get`, `http.Post`, `http.DefaultClient.Do`) теперь диспатчат `toast.ActionAdd`
- `internal/interface/ui/project` (`project.go`, `scene_ops.go`, `tags_ops.go`, `tags_render.go`, `widget_ops.go`) — поля `fetchErr string` и `tagFetchErr string` удалены; все HTTP-ошибки переведены на `toast.ActionAdd`
- `internal/interface/ui/project/scene_ops.go` — при сетевой ошибке или ответе `>= 400` в `saveSceneProperties` и `commitSceneEdit` вызывается `p.loadScenes(ctx)` для отката оптимистичного обновления `p.scenes[i]`
- `internal/interface/ui/project/widget_ops.go` — при сетевой ошибке или ответе `>= 400` в `putWidget` вызывается `p.loadWidgets(ctx)` для отката оптимистичного обновления `p.widgets[idx]`; порог ошибки выровнен с остальными хэндлерами (`>= 400` вместо `< 200 || >= 300`)
- `internal/interface/ui/library/widget_type_ops.go` — порог ошибки в `applyChanges` выровнен: `>= 400` вместо `< 200 || >= 300`
- `Makefile` — `make run` открывает Chromium с флагом `--start-maximized`; `make full_restart` корректно останавливает Docker Compose и удаляет том перед пересозданием БД

## [0.0.18] - 2026-06-02

### Fixed

- `internal/interface/ui/project` — исправлено смещение виджета при изменении размера через хендл SE после поворота (`project.go`, `render_scene.go`):
  - **Причина**: CSS-матрица виджета содержит трансляцию `e = posX + ox·(1−cosA) + oy·sinA`, `f = posY − ox·sinA + oy·(1−cosA)`, где `ox = originX·W`, `oy = originY·H`. При изменении ширины/высоты `ox` и `oy` менялись, сдвигая левый-верхний угол виджета на экране — даже если `posX`/`posY` оставались прежними
  - При mousedown на хендле SE теперь фиксируется канвас-позиция левого-верхнего угла (`resizeStartE`, `resizeStartF`) и текущий `origin` (`resizeOriginX`, `resizeOriginY`)
  - В каждом mousemove после вычисления нового размера `posX`/`posY` пересчитываются так, чтобы `(e, f)` оставалась неизменной: `newPosX = E − newOx·(1−cosA) − sinA·newOy`, `newPosY = F + sinA·newOx − newOy·(1−cosA)`
  - Панель свойств обновляет поля X/Y синхронно с изменением размера

## [0.0.17] - 2026-06-01

### Added

- `internal/interface/ui/project` — панель **Properties** при отсутствии выбранного виджета теперь отображает свойства текущей сцены вместо текста «Select a widget» (`render_properties.go`, `scene_ops.go`, `project.go`):
  - Поля Name (text input), Size W × H (number spinbutton), Background HTML (textarea) — редактируемые, сохраняются через PUT `/api/v1/scenes/:id` при потере фокуса или нажатии Enter
  - Добавлены поля `editingScenePropsName`, `editingScenePropsWidth`, `editingScenePropsHeight`, `editingScenePropsBG` в структуру `Project`
  - `syncSceneEditingFields()` — синхронизирует поля панели с выбранной сценой; вызывается при загрузке, переключении вкладки сцены и переименовании через inline-редактор
  - `saveSceneProperties(ctx)` — отправляет PUT с оптимистичным обновлением in-memory + обновлением `Version` из ответа

### Fixed

- `internal/interface/ui/project` — исправлен сброс выбора виджета: один клик по холсту теперь снимает фокус (`project.go`, `render_scene.go`, `widget_ops.go`):
  - **Причина**: `OnMouseDown` на виджете сразу выставлял `draggingWidgetID`, из-за чего `finalizeAllDrags` при `mouseup` ставил `dragJustEnded = true` даже без реального перемещения — первый клик по сцене проглатывался
  - Добавлен флаг `dragDidMove bool`: выставляется в `OnMouseMove` только при наличии активного drag-операции; `finalizeAllDrags` теперь устанавливает `dragJustEnded = dragDidMove` (и сбрасывает `dragDidMove = false`) — подавление срабатывает только после реального перетаскивания, а не при обычном клике
  - Добавлен `OnMouseDown` на холст, сбрасывающий `dragJustEnded`: устраняет случай, когда после drag браузер не генерирует `click` (mousedown и mouseup на разных элементах) — флаг оставался `true` и следующий намеренный клик по сцене тоже проглатывался

## [0.0.16] - 2026-06-01

### Fixed

- `internal/interface/ui/project` — исправлен баг с перемещением точки **Origin** у повёрнутого виджета (`project.go`, `render_scene.go`):
  - При ненулевом угле поворота перетаскивание хендла Origin перемещало весь виджет вместо точки привязки. Три последовательно устранённые ошибки:
    1. Экранная дельта мыши не преобразовывалась в локальное пространство виджета; добавлено обратное вращение: `d_ox = cosA·dx + sinA·dy`, `d_oy = −sinA·dx + cosA·dy` с сохранением `originDragRotDeg` при начале drag
    2. Изменение `(ox, oy)` смещало компоненты трансляции CSS-матрицы (`e = posX + ox·(1−cosA) + oy·sinA`), визуально сдвигая виджет; добавлена компенсация позиции: `newPosX = startPosX − dOx·(1−cosA) − dOy·sinA`, `newPosY = startPosY − dOy·(1−cosA) + dOx·sinA`; добавлены поля `originStartPosX`, `originStartPosY`
    3. Два исправления объединены: дельта мыши сначала поворачивается в локальные оси виджета, затем кламп-скорректированные локальные дельты подставляются в формулу компенсации позиции — хендл точно следует за курсором при любом угле поворота, виджет остаётся на месте
- `internal/interface/ui/project` — исправлен сброс выбора (selection) виджета при отпускании хендлов вращения и Origin над холстом (`render_scene.go`, `widget_ops.go`, `project.go`):
  - После завершения drag браузер генерировал отдельное событие `click`, которое поднималось к canvas (минуя контейнер виджета) и вызывало `clearWidgetSelection`
  - Добавлен `OnClick` → `stopPropagation` на контейнер виджета: перехватывает клики по хендлам, когда мышь остаётся в пределах виджета
  - Добавлен флаг `dragJustEnded bool`: `finalizeAllDrags` выставляет его при завершении любого drag; canvas `OnClick` при виде флага сбрасывает его и пропускает очистку selection — покрывает случай отпускания мыши над canvas вне контейнера виджета

## [0.0.15] - 2026-05-29

### Added

- `internal/domain/widget` — **InputPort** и **PortBinding** Value Objects:
  - `InputPortName` — валидируется как JS-идентификатор (`^[a-zA-Z_$][a-zA-Z0-9_$]*$`); `NewInputPortNameFromStorage` пропускает regex для доверенных данных из БД
  - `InputPort` — именованный типизированный слот ввода на WidgetType; поля `Name`, `Description`, `TypeHint` (`tag.TagType`); `TypeHint == TagTypeUnknown` означает «любой тип тега»
  - `PortBinding` — связь порта (по имени) с конкретным экземпляром `Tag` (через `id.ID[tag.Tag]`)
  - `NewWidgetType` возвращает `(WidgetType, error)` и проверяет уникальность имён портов; при дубликате — `ErrDuplicateInputPortName`
  - `WidgetType.InputPorts()` — возвращает защитную копию среза
  - `Widget.PortBindings()` заменяет `Widget.TagIDs()` — виджет теперь хранит именованные привязки портов вместо плоского списка UUID тегов
- `internal/application` — `InputPortDTO`, `PortBindingDTO` добавлены в `appdto/widget.go` и `appdto/widget_type.go`; application-сервисы `WidgetService` и `WidgetTypeService` маппят порты и привязки через `NewInputPort` / `NewPortBinding`
- `internal/infrastructure/postgres/migrations`:
  - `20260601000000_add_input_ports_to_widget_types.sql` — колонка `input_ports JSONB NOT NULL DEFAULT '[]'` в `widget_types`; элемент: `{"name":"…","description":"…","type_hint":"…"}`
  - `20260601000001_replace_tag_ids_with_port_bindings_in_widgets.sql` — добавляет `port_bindings JSONB`, мигрирует существующие `tag_ids` в синтетические имена `port_0`, `port_1`, …, затем удаляет колонку `tag_ids`
- `internal/infrastructure/postgres/repositories` — репозитории `widget_repository_postgres` и `widget_type_repository_postgres` сериализуют/десериализуют `InputPort[]` и `PortBinding[]` как JSONB; `widget_type_repository_postgres_test.go` — новый набор интеграционных тестов
- `internal/interface/restapi/restdto` — `InputPortDTO` и `PortBindingDTO` добавлены в REST-DTO; поле `tag_ids []uuid` заменено на `port_bindings []PortBindingDTO` в `WidgetRequest/Response`; `input_ports []InputPortDTO` добавлено в `WidgetTypeRequest/Response`
- `internal/interface/ui/library` — редактор типов виджетов расширен колонкой **Input Ports**:
  - список существующих портов с кнопкой удаления (✕)
  - форма добавления: поля Name (JS identifier), Description (опционально), Type Hint (any / string / boolean / integer)
  - порты сохраняются через PUT вместе с остальными полями типа виджета
- `internal/interface/ui/project` — панель свойств виджета заменяет тег-список секцией **Port Bindings**:
  - для каждого входного порта WidgetType отображается `<select>` с тегами, отфильтрованными по `type_hint`; текущая привязка подсвечивается; можно сменить тег или снять привязку (`— unbound —`)
  - изменения сохраняются через PUT виджета
- `internal/interface/ui/uidto` — новый пакет `uidto` с `InputPortDTO`, общим для пакетов `library` и `project`, устраняет дублирование DTO
- `internal/interface/ui/root` — система тем оформления:
  - CSS custom properties (`var(--bg)`, `var(--accent)`, `var(--error)`, …) инжектируются через `<style id="gs-theme-vars">` в `<head>` при каждой смене темы
  - Три режима: `light`, `dark`, `auto` (авто использует `@media (prefers-color-scheme: dark)`)
  - Кнопка переключения темы: одиночный cycling-значок `◑` (auto) → `☀` (light) → `☾` (dark); режим сохраняется в `localStorage` (`root:theme`)
  - CSS-сброс `html, body { margin: 0; overflow: hidden }` устраняет постоянно видимый вертикальный scrollbar
- `internal/interface/ui/project` — изменяемые ширины панелей в Scenes:
  - Левая панель Widget Types и правая панель Properties разделены 5px-разделителями с `cursor: col-resize`
  - Drag-изменение ширины: левая [100, 480] px, правая [160, 600] px; ширины персистируются в `localStorage` (`project:widgetTypeWidth`, `project:propertiesWidth`)
- Диалоги подтверждения (`app.Window().Call("confirm", …).Bool()`) перед всеми деструктивными операциями в Library и Project
- Персистентность состояния UI в `localStorage`: активная вкладка навигации (`root:mode`), активная сцена (`project:sceneID`), активная под-вкладка Scenes/Tags (`project:subTab`), выбранный тип виджета в Library (`library:selectedID`), выбранный виджет на холсте
- Юнит-тесты `internal/domain/widget/input_port_test.go` — `TestNewInputPortName_*`, `TestNewInputPort_*`, `TestNewWidgetType_DuplicateInputPortName`, `TestNewWidgetType_UniqueInputPorts_OK`, `TestNewWidgetType_InputPortsCopied`

### Changed

- `Widget.TagIDs() []id.ID[tag.Tag]` → `Widget.PortBindings() []PortBinding` во всём стеке (домен → application → инфраструктура → REST → UI)
- `NewWidgetType` сигнатура: добавлен параметр `someInputPorts []InputPort`; возврат изменён с `WidgetType` на `(WidgetType, error)`
- `tag.NewTagType` принимает `"unknown"` и `""` как синонимы `TagTypeUnknown` без ошибки — упрощает маппинг `type_hint` из JSON
- `internal/interface/restapi/restdto/widget_type.go` — `WidgetTypeResponse.InputPorts` ранее отсутствовало и добавлено в Put-запрос; имя поля `input_ports` унифицировано с JSONB-схемой БД
- Все UI-компоненты Library и Project переведены с жёстко заданных hex-цветов на CSS custom properties; кнопки Delete стилизованы красным (`var(--error-*)`), Create — приглушённым акцентом (`var(--accent-*)`)

### Fixed

- Виджет не пропадал с холста после удаления: `loadWidgets` внутри `ctx.Dispatch` запускал `ctx.Async` в запрещённом контексте; заменено прямой фильтрацией среза `p.widgets` (оптимистичное обновление UI) — `widget_ops.go`
- Перетаскивание точки Origin перемещало виджет: удалена компенсация `Position.X/Y`; теперь изменяются только `Origin.X/Y` — `render_scene.go`, `project.go`
- `WidgetTypeService.UpdateWidgetType` при пустом срезе `InputPorts` в запросе обнулял порты вместо сохранения существующих; исправлено явной передачей портов из request — `widget_type_application_service.go`
- `WidgetService.UpdateWidget` аналогично игнорировал `PortBindings` при обновлении позиции/размера — `widget_application_service.go`, `widget_ops.go`
- UI Library: форма добавления порта не очищалась после добавления и не валидировала пустое имя — `render_editor.go`

## [0.0.14] - 2026-05-28

### Added

- `internal/interface/ui/project` — вкладка **Tags** в Project-компоненте:
  - `projectSubTab` — тип-перечисление (`scenes` / `tags`); `activeSubTab` хранит активную вкладку; `renderSubTabs()` рендерит таб-бар; `subTab()` — helper для отдельного таба с подсветкой активного состояния
  - `tags_render.go` — рендеринг тегов: `renderTagsPanel` (две колонки), `renderTagListColumn` (список с кнопками Create/Delete), `renderTagList` (кликабельные строки с именем и типом), `renderTagPropertiesPanel` (панель свойств выбранного тега: Name, Type, Value, Quality), `renderTagCreateForm` (форма с полями Name и Type, кнопки Create/Cancel)
  - `tags_ops.go` — сетевые операции: `createTag` (POST `/api/v1/tags`), `deleteTag` (DELETE `/api/v1/tags/{id}`); вспомогательная `defaultTagValue` возвращает начальное значение по типу тега
- `Makefile` — цель `full_restart`: `db_down` → `db_up` → `run`

### Changed

- `dto.go` — `tagItem` расширен полями `Type`, `Value`, `Quality`, `Version`; добавлены `createTagRequest` и `createTagResponse`
- `docs/ubiquitous_language/ubiquitous_language.md` — термин `Alarm` переименован в `Alert` во всех вхождениях

## [0.0.13] - 2026-05-27

### Fixed

- Оптимистичная блокировка виджетов: `UpdateWidget` с `version=0` в теле запроса тихо уходил в INSERT вместо UPDATE, вызывая ошибку PK-ограничения (HTTP 500 вместо 409). Исправлено: application-сервис выполняет `FindByID` + сравнение версий до `Save`; репозиторий добавил ветку `IsCommitted()` и fallback-ошибку для неопределённых состояний версии
- Оптимистичная блокировка сцен: `UpdateSceneInput` не содержал поле `Version`, что делало OCC структурно невозможным. Исправлено: добавлено поле `Version int` в `UpdateSceneInput`, `UpdateSceneRequest` и `UpdateSceneResponse`; `UpdateScene` проверяет `found.Version().Number() != input.Version` → `ErrSceneConflict`
- Оптимистичная блокировка типов виджетов: `UpdateWidgetType` игнорировал версию клиента — конкурентные правки перезаписывали друг друга без 409. Исправлено аналогичной проверкой версии в application-сервисе
- Nil-UUID валидация: `CreateWidget`/`UpdateWidget` с отсутствующим `scene_id` или `type_id` в JSON тихо сохраняли виджет с произвольным UUID. Исправлено явными проверками `== uuid.Nil` → `ErrWidgetInvalidInput` в application-сервисе
- `PostScenes` возвращал HTTP 500 при ошибках валидации домена; теперь возвращает 422 Unprocessable Entity при `ErrSceneValidation`
- `PostWidgets` возвращал HTTP 500 при невалидном вводе; теперь возвращает 400 Bad Request при `ErrWidgetInvalidInput`

### Changed

- `classifyUpdateConflict` — вынесена из четырёх репозиториев в единую пакетную generic-функцию `classifyUpdateConflict[T]` в `update_conflict_classification.go`; введён тип `tableName` с константами `tableNameTags`, `tableNameScenes`, `tableNameWidgets`, `tableNameWidgetTypes`, исключающий передачу произвольных строк
- `renderSceneCanvas` — удалён избыточный клиентский фильтр `w.SceneID != p.selectedSceneID`; `FindBySceneID` уже ограничивает выборку на уровне БД
- `finalizeAllDrags` — четыре одинаковых if-блока заменены циклом по `[]*string` с указателями на поля структуры
- Тесты: удалена функция-заглушка `mustTagIDFromUUID` (возвращала аргумент без изменений); удалена мёртвая переменная `_ = i` в range-цикле `widget_application_service_test.go`

## [0.0.12] - 2026-05-27

### Added

- `internal/domain/scene` — новый пакет домена для сцен:
  - `Scene` — агрегат (интерфейс + `sceneImpl`): `ID`, `Name`, `Size`, `BackgroundHTML`, `Version`
  - `SceneName` — Value Object; валидация: пустая строка возвращает ошибку
  - `SceneSize` — Value Object; ширина и высота в пикселях; `NewSceneSize` возвращает ошибку при неположительных значениях
  - `BackgroundHTML` — Value Object-обёртка над строкой статического HTML-фона сцены
  - `SceneRepository` — интерфейс с методами `NextID`, `Save`, `FindByID`, `FindAll`, `DeleteByID`; sentinel-ошибки `ErrSceneNotFound`, `ErrSceneConflict`, `ErrSceneValidation`
- `internal/domain/widget` — расширен агрегатом экземпляра виджета:
  - `Widget` — агрегат (интерфейс + `widgetImpl`): экземпляр `WidgetType`, размещённый на `Scene`; поля: `ID`, `Name`, `Position`, `Size`, `Origin`, `Rotation`, `TransformationMatrix`, `TypeID`, `SceneID`, `Labels`, `TagIDs`, `Version`
  - `WidgetName` — Value Object; валидация: пустая строка возвращает ошибку
  - `Position` — Value Object; координаты на холсте (`x`, `y` float64, `z` int для z-index)
  - `Size` — Value Object; ширина и высота в пикселях; `NewSize` возвращает ошибку при неположительных значениях
  - `Origin` — Value Object; нормализованная точка привязки трансформации (`0.0`–`1.0` по каждой оси)
  - `Rotation` — Value Object; угол поворота в градусах (float64)
  - `TransformationMatrix` — Value Object; 2D аффинная матрица CSS `matrix(a,b,c,d,e,f)`, вычисляется из `Position`, `Origin`, `Rotation`, `Size` с учётом точки привязки; метод `CSS()` возвращает строку для `transform:`
  - `WidgetRepository` — интерфейс с методами `NextID`, `Save`, `FindByID`, `FindAll`, `FindBySceneID`, `DeleteByID`; sentinel-ошибки `ErrWidgetNotFound`, `ErrWidgetConflict`, `ErrWidgetInvalidInput`
- `internal/domain/id` — новый пакет обобщённого Value Object `ID[T]`:
  - `ID[T]` — UUID-обёртка с типовым параметром агрегата; исключает перепутывание ID разных агрегатов на этапе компиляции
  - `NewID[T]`, `IDWithUUID[T]`, `IDOption[T]`; при создании без опций генерирует новый UUID автоматически
- `internal/domain/event` — события для жизненного цикла `Widget` и `Scene`:
  - `WidgetCreatedEvent`, `WidgetUpdatedEvent`, `WidgetDeletedEvent`
  - `SceneCreatedEvent`, `SceneUpdatedEvent`, `SceneDeletedEvent`
  - `EventType` расширен 6 новыми значениями
- `internal/application/widget_application_service.go` — `WidgetService` с методами `FindAllWidgets`, `FindWidgetsBySceneID`, `FindWidgetByID`, `CreateWidget`, `UpdateWidget`, `DeleteWidgetByID`; валидация обязательных полей `scene_id` и `type_id` на уровне сервиса; публикует события через `EventBus`
- `internal/application/scene_application_service.go` — `SceneService` с методами `FindAllScenes`, `FindSceneByID`, `CreateScene`, `UpdateScene`, `DeleteSceneByID`; публикует события через `EventBus`
- `internal/application/appdto/widget.go` — DTO `Widget`, `CreateWidgetInput`, `UpdateWidgetInput`; фабрики `NewWidget`, `NewWidgetList`
- `internal/application/appdto/scene.go` — DTO `Scene`, `CreateSceneInput`, `UpdateSceneInput`; фабрики `NewScene`, `NewSceneList`
- `internal/infrastructure/postgres/repositories/widget_repository_postgres.go` — PostgreSQL-реализация `WidgetRepository`; INSERT при `version == Initial`, UPDATE с оптимистичной блокировкой; `classifyUpdateConflict` определяет `ErrWidgetNotFound` vs `ErrWidgetConflict`
- `internal/infrastructure/postgres/repositories/scene_repository_postgres.go` — PostgreSQL-реализация `SceneRepository`; аналогичная стратегия оптимистичной блокировки
- Миграции:
  - `20260516000000_create_widgets.sql` — DDL таблицы `widgets`
  - `20260516000001_create_scenes.sql` — DDL таблицы `scenes`
  - `20260516000002_add_scene_id_to_widgets.sql` — добавлен FK `scene_id` в `widgets`
  - `20260518000001_add_size_to_widgets.sql` — добавлены колонки `width`, `height` в `widgets`
  - `20260526000000_add_transform_to_widgets.sql` — добавлены `origin_x`, `origin_y`, `rotation_degrees` в `widgets`
  - `20260527000000_widget_scene_id_not_null.sql` — `scene_id` переведён в `NOT NULL`
- `internal/interface/restapi/widgets.go` — REST-хэндлеры CRUD для экземпляров виджетов:
  - `GET /api/v1/widgets` — список всех виджетов
  - `GET /api/v1/widgets/{id}` — виджет по ID; 404 при отсутствии
  - `POST /api/v1/widgets` — создание; 201 с объектом; 400 при невалидном вводе
  - `PUT /api/v1/widgets/{id}` — обновление; 404 при отсутствии, 409 при конфликте версий
  - `DELETE /api/v1/widgets/{id}` — удаление; 200 с телом удалённого объекта, 404 при отсутствии
  - `GET /api/v1/scenes/{id}/widgets` — виджеты по сцене (вложенный ресурс)
- `internal/interface/restapi/scenes.go` — REST-хэндлеры CRUD для сцен:
  - `GET /api/v1/scenes` — список всех сцен
  - `GET /api/v1/scenes/{id}` — сцена по ID; 404 при отсутствии
  - `POST /api/v1/scenes` — создание; 201; 422 при ошибке валидации домена
  - `PUT /api/v1/scenes/{id}` — обновление; 404, 409 (конфликт версий), 422 (валидация)
  - `DELETE /api/v1/scenes/{id}` — удаление; 200 с телом удалённой сцены
- `internal/interface/restapi/restdto/widget.go` — HTTP-DTO `WidgetResponse`, `GetWidgetsResponse`, `CreateWidgetRequest/Response`, `UpdateWidgetRequest/Response`
- `internal/interface/restapi/restdto/scene.go` — HTTP-DTO `SceneResponse`, `GetScenesResponse`, `CreateSceneRequest/Response`, `UpdateSceneRequest/Response`
- Bruno-коллекция — новые запросы: `get_scenes`, `get_scene_by_id`, `post_scenes`, `put_scene_by_id`, `delete_scene_by_id`, `get_widgets`, `get_widget_by_id`, `get_widgets_by_scene_id`, `post_widgets`, `put_widget_by_id`, `delete_widget_by_id`
- Юнит-тесты домена `widget` — `widget_test.go`, `widget_name_test.go`, `widget_position_test.go`, `widget_size_test.go`, `widget_origin_test.go`, `widget_rotation_test.go`, `widget_transform_matrix_test.go`
- Юнит-тесты домена `scene` — `scene_test.go`, `scene_name_test.go`, `scene_size_test.go`, `scene_background_html_test.go`
- Юнит-тесты `internal/domain/id/id_test.go` — генерация нового UUID, задание через `IDWithUUID`, `String`, тип-параметр как изолятор
- Интеграционные тесты `internal/application/widget_application_service_test.go` — полный CRUD, валидация обязательных полей, конфликт версий, публикация событий
- Интеграционные тесты `internal/infrastructure/postgres/repositories/widget_repository_postgres_test.go`
- Интеграционные тесты `internal/interface/restapi/widgets_test.go` и `scenes_test.go` — все хэндлеры включая 409 Conflict и 422 Unprocessable Entity

### Changed

- `internal/domain/id` — `TagID`, `WidgetTypeID` и все прочие агрегатные идентификаторы переведены на обобщённый `id.ID[T]` из нового пакета `internal/domain/id`; per-aggregate `TagID`, `WidgetTypeID` удалены как отдельные типы
- `internal/domain/version` — `Version` стал обобщённым `Version[T]`; конструкторы `Initial[T]()` и `Committed[T]()` получили тип-параметр агрегата; исключает применение версии одного агрегата к другому
- `internal/interface/ui/library` — `Library` разбита на отдельные файлы: `library.go` (структура и монтирование), `dto.go`, `render_list.go`, `render_editor.go`, `widget_type_ops.go` (сетевые операции)
- `internal/interface/ui/project` — монолитный `project.go` разбит на: `project.go` (корневая структура), `dto.go`, `matrix.go`, `render_scene.go`, `render_properties.go`, `render_widget_types.go`, `scene_ops.go`, `widget_ops.go`
- Bruno-коллекция — все имена файлов с пробелами переименованы в snake_case (например, `get tag by id.yml` → `get_tag_by_id.yml`)

## [0.0.11] - 2026-05-16

### Added

- `internal/domain/widget` — новый пакет домена для типов виджетов:
  - `WidgetType` — агрегат (интерфейс + `widgetTypeImpl`); immutable: поля не мутируются напрямую, обновление через создание нового экземпляра в репозитории
 - `WidgetTypeID` — Value Object на основе UUID; функциональные опции `WidgetTypeIDWithUUID`; `ParseWidgetTypeID`, `MustParseWidgetTypeID`
  - `WidgetTypeName` — Value Object; валидация: пустая строка возвращает ошибку
  - `HtmlTemplate`, `Script` — Value Object-обёртки над строками; валидация зарезервирована (TODO)
  - `ScriptLanguage` — enum-VO (`javascript`, `python`, `lua`); сериализуется строкой; `NewScriptLanguage` возвращает ошибку для неизвестных значений
  - `WidgetTypeRepository` — интерфейс репозитория с методами `NextID`, `Save`, `FindByID`, `FindAll`, `DeleteByID`; sentinel-ошибки `ErrWidgetTypeNotFound` и `ErrWidgetTypeConflict` (optimistic locking)
- `internal/domain/version` — новый пакет Value Object `Version` для оптимистичной блокировки: константы `Initial` (0) и `Committed` (1); методы `Next()`, `IsCommitted()`, `Number()`; конструктор с валидацией на отрицательные числа
- `internal/domain/event` — события для жизненного цикла `WidgetType`: `WidgetTypeCreatedEvent`, `WidgetTypeUpdatedEvent`, `WidgetTypeDeletedEvent`; `EventType` расширен тремя новыми значениями
- `internal/application/widget_type_application_service.go` — `WidgetTypeService` с методами `FindAllWidgetTypes`, `FindWidgetTypeByID`, `CreateWidgetType`, `UpdateWidgetType`, `DeleteWidgetTypeByID`; публикует события через `EventBus` после каждой успешной мутации
- `internal/application/appdto/widget_type.go` — DTO `WidgetType`, `CreateWidgetTypeInput`, `UpdateWidgetTypeInput`; фабрики `NewWidgetType`, `NewWidgetTypeList`
- `internal/infrastructure/postgres/repositories/widget_type_repository_postgres.go` — PostgreSQL-реализация `WidgetTypeRepository`; INSERT при `version == Initial`, UPDATE с `WHERE id = $X AND version = $Y` при `IsCommitted()`; при `rowsAffected == 0` метод `classifyUpdateConflict` делает `SELECT EXISTS` и возвращает `ErrWidgetTypeNotFound` или `ErrWidgetTypeConflict`
- `internal/infrastructure/postgres/migrations/20260515000000_create_widget_types.sql` — DDL таблицы `widget_types`
- `internal/infrastructure/postgres/test_data/20260515000001_insert_test_data.sql` — тестовые данные для `widget_types`
- `internal/interface/restapi/widget_types.go` — REST-хэндлеры CRUD:
  - `GET /api/v1/widget-types` — список всех типов виджетов
  - `GET /api/v1/widget-types/{id}` — тип виджета по ID; 404 при отсутствии
  - `POST /api/v1/widget-types` — создание; 201 с ID
  - `PUT /api/v1/widget-types/{id}` — обновление; 404 при отсутствии, 409 при конфликте версий
  - `DELETE /api/v1/widget-types/{id}` — удаление; 200 с телом удалённого объекта, 404 при отсутствии
- `internal/interface/restapi/restdto/widget_type.go` — HTTP-DTO `WidgetTypeResponse`, `GetWidgetTypesResponse`, `CreateWidgetTypeRequest/Response`, `UpdateWidgetTypeRequest/Response`
- `internal/interface/ui/library/library.go` — UI-компонент `Library` для управления типами виджетов: список, создание, редактирование, удаление через REST API
- Bruno-коллекция: запросы `GET /widget-types`, `GET /widget-types/{{ID}}`, `POST /widget-types`, `PUT /widget-types/{{ID}}`, `DELETE /widget-types/{{ID}}`
- Юнит-тесты домена `widget` — `widget_type_test.go`, `widget_type_id_test.go`, `widget_type_name_test.go`, `widget_type_script_language_test.go`
- Юнит-тесты `internal/domain/version/version_test.go` — `New` с валидными значениями, отрицательное → ошибка, константы, `Next`, `IsCommitted`, `String`
- Интеграционные тесты `internal/application/widget_type_application_service_test.go` — CRUD, not-found, конфликт версий, публикация событий
- Интеграционные тесты `internal/interface/restapi/widget_types_test.go` — все CRUD-хэндлеры включая 409 Conflict

### Changed

- `internal/domain/version` — версия тега перенесена из `int` в `version.Version`; `Tag.Version()` теперь возвращает `version.Version`; `tag.TagVersionInitial`/`TagVersionCommitted` удалены
- `Tag.IncrementVersion()` — удалён из интерфейса и реализации; инкремент версии выполняется в репозитории через `version.Next()`
- `TagRepository.Save` — сигнатура изменена с `error` на `(Tag, error)`; репозиторий возвращает сохранённый агрегат с обновлённой версией
- `TagID`, `TagName`, `TagQuality`, `TagType` — переведены с type alias / primitive (`type Foo int`, `type Foo string`) на `struct { field T }`; исключает нечаянные приведения типов
- `TagQuality.IsValid()`, `TagType.IsValid()` — удалены; валидность гарантируется конструктором (`NewTagQuality`, `NewTagType` возвращают ошибку для неизвестных строк)
- `restapi/tags.go`, `restapi/widget_types.go` — 500-ошибки переведены на `sendInternalError(w, r, err)`: внутренние детали (SQL, стек) логируются, но не попадают в тело HTTP-ответа
- Тесты `tag_quality_test.go`, `tag_type_test.go` — граничный кейс `default` в `String()` восстановлен через `TagQuality{quality: 99}` / `TagType{tagType: 99}` вместо ранее использовавшихся raw-приведений типов

## [0.0.10] - 2026-05-12

### Added

- `internal/application/app_dto` — новый пакет с DTO уровня прикладного сервиса: `Tag`, `CreateTagInput`, `UpdateTagInput`; поля не имеют JSON-тегов, сериализация делегирована интерфейсному слою; фабричные функции `NewTag` и `NewTagList` конвертируют доменные агрегаты в DTO
- `internal/interface/restapi/rest_dto` — новый пакет с HTTP-DTO: `TagResponse`, `GetTagsResponse`, `CreateTagRequest`, `CreateTagResponse`, `UpdateTagRequest`, `UpdateTagResponse`; конвертеры `NewTagResponse`, `NewGetTagsResponse`, `NewCreateTagResponse`, `NewUpdateTagResponse`, `NewCreateTagInput`, `NewUpdateTagInput` изолируют маппинг между слоями

### Removed

- `internal/application/dto` — удалён единый DTO-пакет, смешивавший JSON-сериализацию и application-слой; его ответственность разделена между `app_dto` и `rest_dto`

### Changed

- `TagService` — все методы переведены на `app_dto`: `FindAllTags` возвращает `[]app_dto.Tag` вместо `dto.FindAllTagsResponse`; `FindTagByID` возвращает `app_dto.Tag` вместо `dto.Tag`; `CreateTag` принимает `app_dto.CreateTagInput` вместо `dto.CreateTagRequest` и возвращает полный `app_dto.Tag` вместо `dto.CreateTagResponse{ID}`; `DeleteTagByID` возвращает плоский `app_dto.Tag` вместо `dto.DeleteTagResponse{Tag: dto.Tag}`; `SetTagValueByID` принимает `app_dto.UpdateTagInput` вместо `dto.UpdateTagRequest` и возвращает `app_dto.Tag` вместо `dto.UpdateTagResponse{Version}`
- `restapi/tags.go` — хендлеры переведены на `rest_dto`: каждый хендлер конвертирует результат сервиса через соответствующую фабрику `rest_dto.New*`; удалены промежуточные переменные для ответов с ошибками; `UpdateTagRequest` больше не содержит поле `ID` — идентификатор передаётся только через path-параметр и подставляется в `NewUpdateTagInput`
- Тесты `tag_application_service_test.go` и `tags_test.go` — обновлены импорты и типы: `dto.CreateTagRequest` → `app_dto.CreateTagInput`, `dto.UpdateTagRequest` → `app_dto.UpdateTagInput`; тип возврата `createTagViaService` изменён с `dto.CreateTagResponse` на `app_dto.Tag`; декодирование HTTP-ответов использует `rest_dto.*` вместо `dto.*`
- Все doc-комментарии в `app_dto` и `rest_dto` переведены на английский язык

## [0.0.9] - 2026-05-08

### Added

- `EventBus` — реализация интерфейса `event.EventBus` в `internal/domain/event/event_bus.go`: хранилище подписчиков `map[EventType][]EventHandler`, потокобезопасность через `sync.RWMutex`; при `Publish` список обработчиков копируется под `RLock` и вызывается без блокировки, что исключает дедлок при вызове `Subscribe` из обработчика
- `MQTTEventBus` — декоратор над `event.EventBus` в `internal/infrastructure/mqtt/event_bus_mqtt.go`; при `Publish` вызывает внутреннюю шину и публикует событие в MQTT-брокер с топиком `events/<event_type>`; `MQTTClient` — порт-интерфейс с методом `Publish(topic string, payload []byte) error`, позволяющий подключить любую MQTT-библиотеку без изменения кода декоратора
- `NewTagService` — добавлен параметр `anEventBus event.EventBus`; `tagServiceImpl` хранит ссылку на шину и публикует события после успешных мутирующих операций: `TagCreatedEvent` в `CreateTag`, `TagDeletedEvent` в `DeleteTagByID`, `TagUpdatedEvent` в `SetTagValueByID`
- Юнит-тесты пакета `event` (`internal/domain/event/event_test.go`) — 33 теста:
  - `EventTimestamp`: создание без опций (метка близка к `now`), с фиксированным временем (`EventTimestampWithTime`), `String`, `ParseEventTimestamp` (валидная строка RFC3339 / невалидная → ошибка), `MustParseEventTimestamp` (валидная / паника)
  - `EventType`: `NewEventType` для всех 4 строк, для неизвестной строки → ошибка, round-trip `String` → `NewEventType` для каждого значения
  - `EventBus`: вызов подписчика при совпадении типа, изоляция по типу (другие подписчики не вызываются), несколько подписчиков для одного типа вызываются все, `Publish` без подписчиков не паникует
  - `SystemReadyEvent`, `TagCreatedEvent`, `TagUpdatedEvent`, `TagDeletedEvent`: тип, `TagID`, метка времени (не нулевая), `String` (не пустая), опция `WithTimestamp`
- Интеграционные тесты прикладного сервиса — 6 новых тестов на публикацию событий через `bus.Subscribe`:
  - `TestCreateTag_Success_PublishesTagCreatedEvent` — проверка типа события и `TagID`
  - `TestCreateTag_InvalidRequest_NoEventPublished` — при ошибке до `Save` событие не публикуется
  - `TestDeleteTagByID_Success_PublishesTagDeletedEvent` — проверка типа события и `TagID`
  - `TestDeleteTagByID_NotFound_NoEventPublished`
  - `TestSetTagValueByID_Success_PublishesTagUpdatedEvent` — проверка типа события и `TagID`
  - `TestSetTagValueByID_InvalidRequest_NoEventPublished`

### Fixed

- `SetTagValueByID` публиковал `TagDeletedEvent` вместо `TagUpdatedEvent` — исправлено

### Changed

- `NewTagService` — сигнатура расширена вторым параметром `anEventBus event.EventBus`; тесты в `tag_application_service_test.go` и `restapi/tags_test.go` обновлены: теперь передают `event.NewEventBus()`; вспомогательная функция `newServiceWithBus()` заменила `newServiceWithSpy()` — шина берётся из пакета `event`, подписка через `bus.Subscribe`
- `TagType` — добавлено нулевое значение `TagTypeUnknown TagType = iota` первым в блоке констант; `String()` получил явный `case TagTypeUnknown`; `NewTagType` возвращает `TagTypeUnknown` вместо `-1` при неизвестной строке; `IsValid()` исключает `TagTypeUnknown`
- `TagQuality` — добавлено нулевое значение `TagQualityUnknown TagQuality = iota` первым в блоке констант; аналогичные правки в `String()`, `NewTagQuality` и `IsValid()`
- `EventType` — добавлено нулевое значение `EventTypeUnknown EventType = iota` первым в блоке констант; `String()` получил явный `case EventTypeUnknown`; `NewEventType` возвращает `EventTypeUnknown` вместо `0` при неизвестной строке
- `Mode` — добавлена константа `ModeUnknown Mode = ""` первой в блоке констант (документирует нулевое значение типа `string`)
- Тесты `tag_type_test.go` — `TagType(-1)` в String-кейсе заменён на `TagTypeUnknown`; out-of-range значение `TagType(99)` покрывает `default`; `TagTypeUnknown` добавлен в список невалидных в `IsValid`-тесте
- Тесты `tag_quality_test.go` — аналогичные правки: `TagQuality(-1)` → `TagQualityUnknown`, добавлен `TagQuality(99)` для `default`, `TagQualityUnknown` в список невалидных
- Тесты `event_test.go` — `TestNewEventType_UnknownString_ReturnsError` расширен проверкой возвращаемого `EventTypeUnknown`; добавлен тест `TestEventType_String_Unknown`

## [0.0.8] - 2026-05-03

### Changed

- `TagKind` переименован в `TagType` во всём проекте: тип, константы (`TagTypeString`, `TagTypeBoolean`, `TagTypeInteger`), конструктор `NewTagType`
- `tag_kind.go` → `tag_type.go`, `tag_kind_test.go` → `tag_type_test.go`
- Метод `Tag.Kind() TagKind` переименован в `Tag.Type() TagType` в доменном интерфейсе и реализации
- Колонка `kind` переименована в `type` в схеме БД: обновлены DDL, индексы (`idx_tags_type`, `idx_tags_name_type`) и constraint (`chk_tags_type`)
- Все SQL-запросы в репозитории обновлены: `kind` → `type` в INSERT, UPDATE, SELECT, RETURNING
- DTO: поле `Kind string json:"kind"` переименовано в `Type string json:"type"` в `Tag` и `CreateTagRequest`
- Тесты: имена функций (`InvalidKind` → `InvalidType`, `ByKind` → `ByType`, `ForKind` → `ForType`, `UnknownKind` → `UnknownType`), переменные (`kind` → `tagType`, `newKind` → `newType`) и строки в логах (`"Kind:"` → `"Type:"`) обновлены во всех пакетах

## [0.0.7] - 2026-05-03

### Changed

- `cmd/combined_server/main.go`: все вызовы `fmt.Printf`/`fmt.Println` заменены на `slog.Error`/`slog.Info` со структурированными key-value аргументами
- `slogAdapter` — добавлен адаптер, реализующий интерфейс `gracedown.Logger` (`Println(v ...any)`); внутренние сообщения `gracedown` теперь выводятся через `slog.Info`
- `gracedown.NewManager` — передаётся `WithLogger(slogAdapter{})`, `gracedown` больше не использует `fmt.Println` напрямую

## [0.0.6] - 2026-05-03

### Added

- `DeleteTagByID` — метод удаления тега по ID в `TagService`; возвращает удалённый тег в `DeleteTagResponse`
- `SetTagValueByID` — метод обновления значения и качества тега в `TagService`; возвращает новую версию в `UpdateTagResponse`
- `DeleteByID` — метод удаления тега в `TagRepository`; возвращает `(Tag, error)`; реализация использует `DELETE ... RETURNING id, name, kind, value, quality, version`, что исключает лишний `SELECT`
- REST-хэндлер `DeleteTagByID` (`DELETE /api/v1/tags/{id}`) — удаление тега; 200 с телом удалённого тега, 404 при отсутствии, 400 при невалидном UUID
- REST-хэндлер `PatchTagValue` (`PATCH /api/v1/tags/{id}/value`) — обновление значения и качества; 200 с новой версией, 404 при отсутствии, 400 при невалидном UUID или теле
- Маршруты `DELETE /api/v1/tags/{id}` и `PATCH /api/v1/tags/{id}/value` зарегистрированы в роутере
- Bruno-коллекция: запросы `DELETE /api/v1/tags/{{TAG_ID}}` и `PATCH /api/v1/tags/{{TAG_ID}}/value`
- Интеграционные тесты репозитория — 4 теста для `DeleteByID`:
  - `TestDeleteByID_ExistingTag_ReturnsDeletedTag` — проверка всех полей возвращённого тега
  - `TestDeleteByID_ExistingTag_TagIsRemovedFromDB` — тег отсутствует в БД после удаления
  - `TestDeleteByID_ExistingTag_OtherTagsAreUnaffected` — прочие теги не затрагиваются
  - `TestDeleteByID_NotFound_ReturnsErrTagNotFound` — возвращает `ErrTagNotFound`
- Интеграционные тесты прикладного сервиса — 8 тестов:
  - `TestDeleteTag_ExistingTag_ReturnsDeletedTag`, `TestDeleteTag_ExistingTag_TagIsRemovedFromDB`, `TestDeleteTag_NotFound_ReturnsWrappedErrTagNotFound`
  - `TestSetTagValueByID_ValidUpdate_ReturnsIncrementedVersion`, `TestSetTagValueByID_ValidUpdate_ValueAndQualityAreUpdated`, `TestSetTagValueByID_NotFound_ReturnsWrappedErrTagNotFound`, `TestSetTagValueByID_InvalidQuality_ReturnsError`, `TestSetTagValueByID_InvalidValueForKind_ReturnsError`
- Интеграционные тесты HTTP-хэндлеров — 9 тестов:
  - `TestDeleteTagByID_ExistingTag_Returns200WithDeletedTag`, `TestDeleteTagByID_ExistingTag_TagIsRemovedFromDB`, `TestDeleteTagByID_NotFound_Returns404WithProblemDetails`, `TestDeleteTagByID_InvalidUUID_Returns400WithProblemDetails`
  - `TestPatchTagValue_ValidUpdate_Returns200WithVersion`, `TestPatchTagValue_ValidUpdate_ValueAndQualityAreUpdated`, `TestPatchTagValue_NotFound_Returns404WithProblemDetails`, `TestPatchTagValue_InvalidUUID_Returns400WithProblemDetails`, `TestPatchTagValue_InvalidJSON_Returns400WithProblemDetails`

### Changed

- `Tag.UpdateValue` переименован в `Tag.SetValue` в доменном интерфейсе и реализации; тест-функции переименованы соответственно (`TestTag_SetValue_*`)
- `TagService.DeleteTag` переименован в `TagService.DeleteTagByID`
- `TagRepository.Delete` переименован в `TagRepository.DeleteByID`; сигнатура изменена с `error` на `(Tag, error)` — сервис делает один вызов репозитория вместо двух (`FindByID` + `Delete`)
- `Save` в PostgreSQL-репозитории: добавлена явная обработка ошибки `RowsAffected()` для INSERT и UPDATE (для postgres всегда `nil`)
- `.github/workflows/go.yaml`: путь сборки исправлен с `./cmd/combined/` на `./cmd/combined_server/` — директория была переименована

## [0.0.5] - 2026-05-01

### Added

- Интеграционные тесты прикладного сервиса `tag` (`internal/application/tag_application_service_test.go`) — 10 тестов:
  - `TestCreateTag_ValidIntegerTag_ReturnsID` — создание тега типа `integer`, проверка ненулевого UUID в ответе
  - `TestCreateTag_ValidStringTag_ReturnsID` — создание тега типа `string`
  - `TestCreateTag_ValidBooleanTag_ReturnsID` — создание тега типа `boolean`
  - `TestCreateTag_InvalidKind_ReturnsError` — невалидный kind возвращает ошибку
  - `TestCreateTag_InvalidQuality_ReturnsError` — невалидное quality возвращает ошибку
  - `TestCreateTag_InvalidValueForKind_ReturnsError` — значение несовместимо с типом (строка вместо числа) возвращает ошибку
  - `TestFindAllTags_EmptyDB_ReturnsEmptyList` — пустая БД возвращает пустой список
  - `TestFindAllTags_MultipleTags_ReturnsAll` — возвращает все сохранённые теги
  - `TestFindTagByID_ExistingTag_ReturnsTag` — возвращает тег с корректными полями в DTO
  - `TestFindTagByID_NotFound_ReturnsWrappedErrTagNotFound` — возвращает ошибку с обёрнутым `ErrTagNotFound`
- Интеграционные тесты HTTP-хэндлеров `tag` (`internal/interface/restapi/tags_test.go`) — 8 тестов; запросы проходят через `router.ServeMux().ServeHTTP()`, что корректно заполняет path-параметры:
  - `TestGetTags_EmptyDB_Returns200WithEmptyList` — пустая БД, статус 200, пустой массив тегов
  - `TestGetTags_WithTags_Returns200WithAll` — теги в БД, статус 200, все теги в ответе
  - `TestGetTagsByID_ExistingTag_Returns200WithTag` — существующий тег, статус 200, корректные поля
  - `TestGetTagsByID_NotFound_Returns404WithProblemDetails` — несуществующий UUID, статус 404, тело в формате RFC 9457
  - `TestGetTagsByID_InvalidUUID_Returns400WithProblemDetails` — невалидная строка в `{id}`, статус 400, тело RFC 9457
  - `TestPostTags_ValidBody_Returns201WithID` — корректный JSON, статус 201, ненулевой UUID в ответе
  - `TestPostTags_InvalidJSON_Returns400WithProblemDetails` — невалидный JSON, статус 400, тело RFC 9457
  - `TestPostTags_InvalidKind_Returns500WithProblemDetails` — невалидный kind, статус 500, тело RFC 9457
- Юнит-тесты `ProblemDetails` (`internal/interface/restapi/problems_test.go`) — 11 тестов:
  - `TestProblemDetails_MarshalJSON_*` — 4 теста: стандартные поля попадают в JSON, пустой `detail` опускается, extensions сериализуются, extension не перебивает стандартное поле
  - `TestProblemDetails_UnmarshalJSON_*` — 2 теста: стандартные поля парсятся, неизвестные поля попадают в `Extensions`
  - Фабрики: `TestNewBadRequest_*`, `TestNewNotFound_*`, `TestNewConflict_*`, `TestNewValidationError_*`, `TestNewInternalError_*` — проверка статуса, типа, полей и extensions

## [0.0.4] - 2026-04-30

### Added

- Интеграционные тесты для инфраструктурного репозитория `tag` (`internal/infrastructure/postgres/repositories/tag_repository_postgres_test.go`) — 8 тестов:
  - `TestNextID_ReturnsUniqueIDs` — генерирует уникальные идентификаторы
  - `TestSave_NewTag_InsertsSuccessfully` — INSERT новой записи без ошибок
  - `TestSave_DuplicateID_ReturnsError` — повторный INSERT с тем же ID возвращает ошибку
  - `TestSave_ExistingTag_UpdatesSuccessfully` — UPDATE существующей записи, проверка новых значений
  - `TestSave_StaleVersion_ReturnsError` — оптимистичная блокировка: UPDATE с несовпадающей версией возвращает ошибку
  - `TestFindByID_ExistingTag_ReturnsTag` — возвращает тег с корректными полями
  - `TestFindByID_NotFound_ReturnsErrTagNotFound` — возвращает `ErrTagNotFound` при отсутствии записи
  - `TestFindAll_EmptyDB_ReturnsEmptySlice` — возвращает пустой срез при пустой таблице
  - `TestFindAll_MultipleTags_ReturnsAll` — возвращает все сохранённые теги
- Зависимость `github.com/pressly/goose/v3 v3.27.1` — применение SQL-миграций в тестовом окружении
- Тесты используют `testcontainers-go/modules/postgres`: контейнер поднимается в `TestMain`, миграции из `internal/infrastructure/postgres/migrations/` применяются через goose

## [0.0.3] - 2026-04-30

### Added

- Юнит-тесты для доменного агрегата `tag` (`internal/domain/tag/`) — 9 файлов, 63 теста:
  - `tag_id_test.go` — генерация UUID, парсинг, паника для невалидных строк, roundtrip
  - `tag_name_test.go` — создание имени, `String()`
  - `tag_kind_test.go` — парсинг строк, `String()`, `IsValid()`
  - `tag_quality_test.go` — парсинг строк, `String()`, `IsValid()`
  - `tag_value_boolean_test.go` — конструктор из `string`/`bool`/`int`, ошибки, `Value()`, `String()`
  - `tag_value_integer_test.go` — конструктор из `string`/`bool`/`int`, ошибки парсинга, `Value()`, `String()`
  - `tag_value_string_test.go` — конструктор из `string`/`bool`/`int`, конвертации, `Value()`, `String()`
  - `tag_value_test.go` — фабрика `NewTagValue`: диспетчеризация по `TagKind`, неизвестный kind, несовместимый тип
  - `tag_test.go` — создание агрегата, все геттеры, `IncrementVersion`, `UpdateValue`
- `.github/workflows/go.yaml` — GitHub Actions CI: запуск тестов и сборка при push/PR в ветку `path`

### Changed

- `.gitverse/workflows/go.yaml` — исправлены три проблемы: триггер `workflow_dispatch` заменён на `push`/`pull_request` для ветки `path`; версия Go зафиксирована через `go-version-file: go.mod` вместо явного `1.23`; путь сборки `cmd/server/main.go` (несуществующий) исправлен на `./cmd/combined/`

## [0.0.2] - 2026-04-28

### Added

- `FindTagByID` — реализован метод получения тега по UUID в сервисном слое (`TagService`)
- `FindByID` — реализован метод в репозитории PostgreSQL: SQL-запрос по `id`, маппинг строки в доменные value objects, возврат `ErrTagNotFound` при отсутствии записи
- `ErrTagNotFound` — добавлена сентинель-ошибка в доменный пакет `tag`
- Bruno-коллекция: добавлен запрос `GET /api/v1/tags/{{TAG_ID}}`
- `emergencyExit` — вспомогательная функция для аварийного завершения с корректным shutdown всех компонентов

### Changed

- `TagService.FindTagByID` — тип параметра изменён с `int` на `tag.TagID`
- `gracedown` обновлён до версии `v0.0.0-20260423215449-38425123f880`; переход с `WaitForSignal` на `WaitForSignalAndShutdown`, коды выхода вынесены в пакет `gracedown`
- Порядок инициализации в `main.go`: инфраструктурные компоненты (БД) поднимаются раньше интерфейсных (HTTP-серверы); регистрация shutdown-хука для БД перенесена до `Ping()`, чтобы соединение закрывалось при аварийном завершении
- `sendJSONResponse` — сериализация тела теперь выполняется до отправки заголовков, что позволяет вернуть `500` при ошибке маршалинга
- `GetTags`, `GetTagsByID` — упрощены: ручной `json.Marshal` + `w.Write` заменены на `sendJSONResponse`
- `PostTags` — исправлен отсутствующий `return` после ошибки `CreateTag`; без него параллельно уходили ответы `500` и `201`
- `GetTagsByID` зарегистрирован в роутере: `GET /api/v1/tags/{id}`

## [0.0.1] - 2026-04-22

- Basic tag functionality
