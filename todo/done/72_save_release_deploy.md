# Save → Commit → Release → Deployment

Status: ready-for-agent
Priority: 1
Parts: 72_1_remove_editlock, 72_2_commit_save, 72_3_project_releases, 72_4_deploy_release, 72_5_library_commit_release, 72_6_project_ui_save_releases, 72_7_library_ui_save_releases, 72_8_migration_tools_docs, 72_9_rename_revision_to_deployment

Слой C (Draft и публикация). Термины — `GLOSSARY.md` (Draft, DraftChange, Commit, Release, Deploy, Deployment).
Меняет решения ADR 0013, 0014, 0015, 0016, 0017, 0018, 0019, 0020 и 0007; новые ADR 0021 и 0022 пишутся в 72_1/72_8. Runtime-сущность Revision переименовывается в Deployment задачей 72_9.

## Зачем

Сейчас редактирование устроено как «взял замок — правишь — отдал»: Check out, Check in, Take over, read-only без замка. Для инженера это неочевидно: правка Draft уже сохраняется на сервере после каждого действия, а «Check in» ничего не публикует и не сбрасывает — это только координация. Пользователь просит редакторскую модель: **Edit и Save**, без Check in/out, с историей сохранений.

Публикация тоже перевёрнута: сейчас Deploy берёт Draft целиком и делает Revision (в 72_9 переименована в Deployment); Release есть только у Library. Нужна явная лестница:

**Draft (правится) → Save (снимок с заголовком и автором) → Release (публикация снимка) → Deploy (какая Release работает).**

## Целевая модель (рекомендация)

### Уровни

| Уровень | Что это | Кто создаёт | Изменяемость |
|---|---|---|---|
| **Draft** | рабочее состояние Project или Library; каждое действие — DraftChange | инженер, сразу | изменяемый, Version + undo/redo |
| **Commit** (решение 2026-11-07) | снимок всего Draft на момент Save: содержимое, номер, заголовок, автор, время, версия Draft | Save | содержимое неизменяемо; заголовок меняется отдельной кнопкой |
| **Release** | публикация Commit: номер, заметка, автор, время, провенанс Commit и **копия содержимого** | Release | содержимое неизменяемо, не удаляется; заметка редактируется |
| **Deployment** (Runtime node) | запись о запуске конфигурации Project: неизменяемый ProjectFile, номер, время, причина, номер Release | Deploy Release, Rollback, Deploy file | неизменяем; Operation всегда работает ровно с одним Deployment |

- **Project Release** можно сделать Deploy. Список всех Release выбранного Project показывает, какая Release работает сейчас (Release других Project'ов в этом списке не показываются). Список — данные Engineering node и читается без Runtime; работающую Release UI определяет по `GET …/deployment` и сравнивает `release_number`; если Runtime недоступен, список остаётся, а маркер — «не удалось проверить».
- **Library Release** виден в списках Project (закрепление Library Pin) и не деплоится: Project видит WidgetType только из закреплённой Release.
- **Deployment** — запись Runtime node о запуске; прежнее имя — Revision (переименовано в 72_9). Это не Commit и не Release.
- «Работает сейчас» = у текущей Deployment есть `release_number`; список Release помечает эту Release. Deployment, созданная до появления Releases или из файла, может не иметь Release — тогда список честно показывает «работает Deployment N (не Release)».

### Save

- Правки Draft по-прежнему сохраняются на сервере после каждого действия (DraftChange); Save **не** является единственной точкой сохранения и не несёт риска потерять работу.
- Save — явное действие: заголовок (обязателен), автор (объявленное имя; User появится в задаче 49), проверка Version Draft. Каждый Save создаёт новый Commit со следующим номером.
- Save не меняет содержимое Draft, не поднимает Version Draft, не пишет DraftChange и **не чистит стек undo/redo**. «Есть несохранённые изменения» = Version Draft больше `draft_version` последнего Commit.
- Save без изменений с последнего Commit — 409 `nothing_to_save`. Draft, который сборщик ProjectFile отвергает, — 409 `draft_not_saveable` с причиной (как `draft_not_deployable` сегодня).
- Commit — сущность агрегата Draft: `engineering.commits` с `draft_id`, уникальным `(draft_id, number)`, содержимым JSONB и метаданными. Save собирает содержимое по Version клиента, затем в одной транзакции над Draft блокирует строку, сверяет Version, вставляет Commit и `last_commit_number`; расхождение — 409. Это не отдельный агрегат и не межтранзакционная запись.
- Содержимое Commit: для Project — `projectv0.ProjectFile` (тот же сборщик, что у Deploy); для Library — снимок всех WidgetType Draft (те же `ReleasedWidgetType`, что в Release).
- Заголовок Commit меняется отдельным действием `PATCH …/commits/{n}` `{title}`. Содержимое не меняется. Провенанс Release, уже созданной из Commit, не переписывается задним числом.

### Release

- Release делается **из Commit**, не из Draft. Release копирует содержимое Commit, поэтому переживает удаление/обрезку Commit и остаётся самодостаточной.
- Release — отдельный агрегат области с общей доменной моделью и общим `appdto.Release`; хранение — по областям, чтобы не ломать существующие закрепления:
  - Project Release — новая таблица `engineering.project_releases` (`project_id`, `number`, `commit_number` nullable, `commit_title`, `note`, `author`, `created_at`, `origin`, `content jsonb` с `ProjectFile`);
  - Library Release — существующая `engineering.library_releases` расширяется `commit_number` (nullable), `author`, `origin`; содержимое остаётся в `engineering.released_widget_types` (его читают закрепления и палитра Project), а Commit/Release копируют WidgetTypes туда.
  - Уникальность `(scope, number)` и «один Commit — одна Release» — в каждой таблице.
- Один Commit — не больше одной Release: повторная публикация того же Commit — 409 `commit_already_released`. Нужна новая Release — сначала новый Save (даже если содержимое не менялось, это осознанное решение; иначе номер Release и заметка теряют смысл).
- Release не удаляется; содержимое неизменяемо, **заметка редактируется отдельной кнопкой** (`PATCH …/releases/{n}`) и не является содержимым. Заголовок Release — снимок заголовка Commit'а.
- Project Release и Library Release — один доменный тип `Release` со `Scope`; разница в том, что Project Release деплоится, а Library Release закрепляется.
- Старые Library Release (до этой задачи) получают `commit_number = NULL` и `origin = legacy`; их содержимое остаётся рабочим.

### Deploy

- Deploy активирует **Project Release**, а не Draft. `POST …/projects/{id}/releases/{n}/deploy` отправляет содержимое Release на Runtime node.
- Runtime node хранит запись о запуске — **Deployment** (`runtime.deployments`, создана переименованием в 72_9), — с `release_number` (nullable) и причиной `deploy`/`rollback`/`file`. Operation, Player, Journal и History читают Deployment; `deployment_created` — единственный сигнал «конфигурация сменилась».
- Повторный Deploy работающей Release (и Deploy Draft, если такой маршрут останется) — 409 `nothing_to_deploy`. Проверка «какая Release работает» — в транзакции Runtime node, а не только в UI.
- Rollback остаётся способом сделать работающей более раннюю Deployment; новая Deployment копирует документ и сохраняет `release_number` исходной. В UI Project «Rollback» заменяется выбором любой Release и Deploy; отдельный маршрут Rollback можно оставить для совместимости/History.
- Deploy file (`POST …/projectfile/deploy`) не трогает Draft. Чтобы у работающей Deployment был Release, файл создаёт **Release напрямую** (`origin = file`, `commit_number = NULL`, заголовок/имя файла, автор из запроса) и деплоит её. Если файл деплоится на узле без Engineering node, допустим `release_number = NULL`; UI показывает «работает Deployment N (не Release)».
- **Discard** возвращает Draft к **последнему Commit'у** (отменить несохранённое) в обоих режимах. Отдельно есть **Revert to deployed** для Project (сброс Draft к работающей Deployment) и **Revert to latest Release** для Library (сброс к последней опубликованной Release). Оба Revert — одна транзакция над Draft, как Discard; Commit/Release/Deployment не удаляются. Стек undo/redo чистит Discard и Revert.
- «Deploy a Draft» как отдельный акт уходит: путь публикации — Save → Release → Deploy. Старый `POST …/deploy` можно оставить только как шим, который отвечает 410 с подсказкой; в UI его нет.

### Без EditLock

- Check out, Check in, Take over, EditLock, `edit_locks`, reaper, presence по SSE, `X-GrowSCADA-Client`, `?client=`, `Problem.edit_lock` и события `edit_lock_*` удаляются.
- Конкурентность — только Version Draft: каждая запись состояния несёт Version, устаревшая — 409 `draft_conflict`, UI перечитывает содержимое и стек. Deploy/Release больше не требуют замка: Deploy читает Release, Release читает Commit.
- Автор Commit/Release/Deploy — объявленное имя (как сегодняшнее имя EditLock), хранится в LocalStorage UI. Это coordination, не аутентификация; задача 49 заменит имя на User, задача 50 добавит права.
- Undo/redo остаются общими для Draft: без замка два инженера могут отменять чужие DraftChange. Это осознанный компромисс до задачи 49/50; в spec 72_2 фиксируется, что история undo — Draft, а не автора.

### ProjectFile и контракты

- `contract/project/v0` не меняется: Release хранит тот же `ProjectFile`; провенанс (номер Commit/Release, автор, заметка) — метаданные Engineering node, не документ.
- `contract/api/v0` растёт: `CommitResponse`, `ReleaseResponse`, `DeploymentResponse`, `POST …/commits`, `GET …/commits`, `PATCH …/commits/{n}`, `POST …/releases`, `GET …/releases`, `POST …/releases/{n}/deploy`, `GET …/deployment`, `GET …/deployments`; из него уходят edit-lock DTO/маршруты/события. Пока MAJOR = 0 — MINOR и `make schemas`.
- `contract/record/v0` (Journal): запись `deployment` (прежнее имя `revision`) с `release_number` (MINOR), чтобы History/PlaybackFile видели, какая Release работала; старые записи `revision` читаются кодеком.
- События: `commit_created`, `commit_title_updated`, `release_created`, `release_note_updated` (у Library — `library_released`/`library_release_note_updated`); Deploy публикует `deployment_created`. Project-события — в поток Project, Library-события — в серверный поток.

## Варианты имени сущности Save

| Вариант | Плюсы | Минусы |
|---|---|---|
| **A. Commit** (рекомендуется) | снимок не конфликтует с Runtime-сущностью; Git-подобная лестница Commit → Release → Deployment; коротко | слово «коммит» менее привычно инженеру; в UI можно подписать «Сохранение»/«Версия» |
| **B. Revision** | совпадает с формулировкой пользователя; привычно по документации | текущую Runtime Revision придётся переименовать в Deployment (`runtime.revisions`, API, `revision_created`, Journal, History, PlaybackFile, ADR 0013/0014/0019/0020) — большой механический рефакторинг и риск |
| **C. Snapshot / Снимок** | нейтрально и понятно | пересекается с внутренним `draft.Snapshot` (DraftChange) и с «снимком» Checkpoint |

Решение (2026-11-07): **A, Commit** для снимка Save; слово «Ревизия» за снимком не остаётся. Отдельным решением (2026-11-07) Runtime-сущность **Revision переименована в Deployment** (72_9): модель — Commit → Release → Deployment. Варианты B и C не выбраны.

## Разбиение

| Часть | Файл | Что | Blocked by |
|---|---|---|---|
| 1 | `72_1_remove_editlock.md` | Убрать EditLock, Check in/out, Take over, reaper, presence, заголовок и гейт; конкурентность только Version Draft; автор — объявленное имя; ADR 0022, GLOSSARY, API, UI, growctl, Bruno | — |
| 2 | `72_2_commit_save.md` | Commit как сущность Draft (Project и Library), Save, список, заголовок отдельной кнопкой, unsaved-индикатор, миграция `engineering.commits`, REST/UI/Bruno | 72_1 |
| 3 | `72_3_project_releases.md` | Project Release из Commit, список, провенанс, уникальность «один Commit — одна Release», миграция `engineering.project_releases`, REST/UI | 72_2 |
| 4 | `72_4_deploy_release.md` | Deploy Project Release, `release_number` в Deployment, running-маркер, Deploy file → Release, Rollback/Discard, Journal, Operation/Player, Bruno | 72_3, 72_9 |
| 5 | `72_5_library_commit_release.md` | Library Release из Commit, старые Release как legacy, закрепления и палитра Project, защита Release от удаления Library (задача 71) | 72_2, 71 |
| 6 | `72_6_project_ui_save_releases.md` | Project UI: Save, имя автора, заголовок, список Commit, Release из Commit, список Release с running, Deploy по Release, удаление старого Deploy/Rollback UI | 72_4 |
| 7 | `72_7_library_ui_save_releases.md` | Library UI: Save, Commit, Release из Commit, история Release, провенанс в Project-пинах | 72_5, 72_1 |
| 8 | `72_8_migration_tools_docs.md` | Миграция/совместимость, growctl (save/release/deploy), Bruno-коллекции, ADR 0021/0022 и поправки, GLOSSARY, AGENTS, CHANGELOG, схемы | 72_1–72_7, 72_9 |
| 9 | `72_9_rename_revision_to_deployment.md` | Переименовать Runtime Revision → Deployment: домен, БД, API, события, Journal, Operation/Player, History/PlaybackFile, Bruno | — |

72_9 можно делать параллельно с 72_1/72_2, но до 72_4. 72_3 и 72_5 можно вести параллельно после 72_2, но 71 (защита Library) требует 72_2+72_3 и блокирует 72_5; 72_6 и 72_7 — параллельно после своих серверных частей. Все части — `Priority: 1`; 72_1 начинает ветку `72_save_release_deploy`.

## Влияние на уже заведённые задачи

- **44 Checkpoint** — Checkpoint привязан к Deployment; добавить номер Release и обновить «Deploy/Rollback» → «Deploy Release/Rollback». `Blocked by` дополняется 72_4/72_9.
- **45 History**, **46 PlaybackFile**, **47 hover tracking** — читают Deployment; в PlaybackFile — Deployment/Release, а не «ревизии Draft».
- **48 Adopt** — Origin=Project после Deploy Release, а не после Deploy Draft; текущая конфигурация — Deployment.
- **49 User** — User становится автором Commit/Release/Deploy и носителем прав, а не EditLock; зависимость от 72_2.
- **50 Access rights** — права на Save, Release, Deploy, Rollback вместо прав на Check out.
- **51 Runtime embedded Deployment** — вшивается работающая Deployment, а не Draft.
- **52 Network nodes** — порт Engineering→Runtime деплоит Release; «текущая Deployment» приходит с `release_number`.
- **58 CQRS** — списки Commit/Release и running-маркер — новые read-модели.
- **63, 64** — списки Library Release и код Release меняются; часть работы уходит в 72_5/72_7.
- **68 Tag supplier change** — мутация Tag'а ожидает Version Draft; EditLock больше нет.
- **71 Release pinned by history** — становится критичной: теперь **каждый Commit** называет закреплённые Library Release, а не только работающая Deployment. Решение (2026-11-07) — **гибрид**: живой пин и работающая Deployment блокируют удаление Library; старая история (Commit/Project Release) не блокирует, а Discard к старой Deployment пропускает исчезнувшую Release с предупреждением и не ретраится вечно. `Priority: 1`, `Blocked by: 72_2, 72_3, 72_9`.
- **61 UI stale responses** — гонки «ответ до Save/Release» добавляются к списку; проверять после 72_6/72_7.

## Вне объёма

- Retention/обрезка Commit'ов (полные снимки на каждый Save; при необходимости — отдельная задача после появления реального объёма).
- Совместное редактирование в реальном времени и per-user undo; до задачи 49/50 история undo остаётся общей.
- Права доступа и аутентификация автора (задачи 49, 50).
- Обратное переименование Deployment → Revision не делается; переименование выполняется в 72_9.

## Comments

- 2026-11-07: постановка пользователя. Убрать Check in/out, редакторская модель Edit/Save; каждый Save — снимок с заголовком, временем и автором; из снимка — Release; Library Release виден в Project; Project Release деплоится; список всех Release проекта с пометкой работающей. Разбиение — 9 частей, все Priority: 1; существующие 44–70 остаются P3, 71 — P1.
- 2026-11-07: имя снимка выбрано — **Commit**.
- 2026-11-07: отдельным решением Runtime-сущность Revision переименована в **Deployment** (72_9). Итоговая модель: Draft → Save/Commit → Release → Deploy → Deployment.
- 2026-11-07 (гриллинг готовности к auto):
  - Library и история (71) — **гибрид**: живой пин и работающая Deployment блокируют удаление; старая история — нет; восстановление пинов из старой истории пропускает исчезнувшую Release с предупреждением и не ретраится вечно.
  - Discard в обоих режимах — к последнему Commit'у; Revert к работающей Deployment / последней Release — отдельные действия.
  - Список Release читается без Runtime; работающая Release определяется отдельным `GET …/deployment`; при недоступности Runtime список остаётся, маркер «не удалось проверить».
  - Один Commit — одна Release; содержимое Release неизменяемо, заметка редактируется отдельной кнопкой.
  - Deploy до 72_4 защищён проверкой Version вокруг сборки: чтение V → сборка → проверка V в транзакции → отправка; иначе 409, смешанный ProjectFile не уходит.
