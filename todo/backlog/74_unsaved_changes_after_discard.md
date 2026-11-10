# Бейдж «Unsaved changes» не исчезает после Discard к последнему Commit

Status: ready-for-agent
Priority: 3
Absorbs: 75

Появилось при ревью задачи 73 (находки Bugs 3 / Spec 2). Приёмочный п. 5
задачи 73 требует «после Save/Discard бейдж исчезает», а решение Q2 той же
задачи — считать `has_unsaved_changes` по `Version` Draft и
`last_commit_draft_version` последнего Commit. Два требования несовместимы:
`Discard` (`draft.Repository.Replace`) поднимает Version Draft, поэтому после
сброса к последнему Commit'у флаг остаётся истинным. То же у Library: после
Discard шапка редактора гасит бейдж только памятью вкладки
(`cleanDraftVersion`), а список, другая вкладка и перезагрузка страницы — нет.

## Решения грилинга (2026-11-10)

1. **Механизм — маркер «чистой версии», а не сравнение содержимого.** Draft
   помнит Version, на которой его содержимое последний раз стало равным
   последнему Commit'у. Флаг = `Version > CleanVersion`. Сравнение содержимого
   на каждом чтении отвергнуто: оно требует сборки всего ProjectFile
   (6 `FindAll` + копии WidgetType'ов) на каждое событие содержимого в редакторе
   и на каждую «грязную» строку страницы выбора.
   Известное ограничение принято: если содержимое вернулось к сохранённому
   **без** Save и без Discard (Undo/Redo к сохранённому состоянию, Open файла,
   канонически равного Commit'у, правка «туда-обратно»), флаг говорит
   «unsaved changes», пока не будет Discard'а или Save.
2. **Понятие — часть агрегата Draft.** `engineering.drafts.clean_version`
   (`INTEGER NOT NULL DEFAULT 0`), `draft.Draft.CleanVersion()` и
   `Draft.HasUnsavedChanges()`; проверка `nothing_to_save` живёт там же, в
   репозитории Draft'а, — правило в одном месте. `Repository.Replace` получает
   признак «записанное содержимое равно последнему Commit'у» от вызывающего,
   у которого оба документа уже в руках.
3. **Revert решается равенством содержимого, не провенансом.** Project:
   `len(warnings) == 0 && sameProjectFile(deployment.Content, lastCommitFile)`
   (хелпер уже есть, `project_file_builder.go`); Library: копии WidgetTypes
   последнего Release равны копиям последнего Commit'а. Так чистым считается и
   Deploy файла, и Rollback, а старый Release — нет.
   **Discard или Revert, выбросивший пины с их виджетами (warnings), чистым не
   считается** — содержимое действительно не равно Commit'у.
4. **`nothing_to_save` — по тому же маркеру.** Отказ ⟺ `Version == CleanVersion`.
   После Discard сохранить нельзя; после Undo к сохранённому состоянию — можно,
   и это создаст Commit с тем же содержимым (безвредная точка истории).
   Точное сравнение в Save отвергнуто: оно разошлось бы с бейджем (кнопка
   активна, а сервер отвечает 409).
5. **Флаг отдаёт сервер, UI только рисует.** Он едет в `DraftStateResponse`
   (`GET …/draft`): «состояние Draft'а — версия и есть ли несохранённое». UI
   удаляет `cleanDraftVersion` и сравнение с `lastCommitVersion` в обоих
   редакторах. `GET …/draft-changes` остаётся состоянием стека.
6. **Задача 75 поглощается.** «Draft без строки и Draft без правок — чистые» —
   это та же семантика: `CreateLibrary` перестаёт отдавать
   `ReleaseState{UnsavedChanges: true}`, и все состояния страницы выбора
   становятся достижимы без правок `ui/picker`. Файл 75 перенесён в `todo/done/`
   с пометкой «поглощена задачей 74».

## Что меняется

### Домен (`internal/server/domain/`)

- `draft`: `Draft` получает `CleanVersion() version.Version[Draft]` и
  `HasUnsavedChanges() bool`; `WithCleanVersion` — как `WithVersion`.
  Комментарии `SaveCommit`/`Replace` в `repository.go` описывают маркер.
- `draft.Repository.Replace` получает признак «цель равна последнему Commit'у»;
  `SaveCommit` отказывает `nothing_to_save` на чистой версии и ставит маркер.
- `library.ReleaseStateOf` принимает рассчитанный флаг вместо
  `(draftVersion, lastCommitDraftVersion)`; пакет по-прежнему не зависит от
  `draft`.

### Репозиторий и миграция (`infrastructure/postgres/`)

- Миграция `engineering/20261113000000_draft_clean_version.sql`:
  `ALTER TABLE engineering.drafts ADD COLUMN clean_version INTEGER NOT NULL
  DEFAULT 0`; бэкфилл
  `UPDATE engineering.drafts SET clean_version = version
   WHERE last_commit_draft_version = version` (остальные остаются 0 — «чистой
  версии не было», флаг честно «грязный»).
- `selectDraftColumns`, `draftRow`, `scanDraftRow`, `draftOf` читают маркер.
- `SaveCommit`: `clean_version = expected` в том же `UPDATE`, что и
  `last_commit_*`; отказ по `row.cleanVersion == row.version`.
- `Replace`: `clean_version = next` при чистой замене, иначе значение не
  трогается (версия выше — флаг истинен). `Update`, `Undo`, `Redo`, `Purge`,
  `ClearStack` маркер не пишут.

### Application (`internal/server/application/`)

- `ProjectService.draftState` читает только строку Draft'а (без
  `FindLastCommit`); `appdto.ProjectDraftState` → `{DraftVersion,
  HasUnsavedChanges}`, `appdto.NewProject` берёт флаг из него; Draft без строки —
  чисто.
- `LibraryService.releaseState` считает флаг из того же чтения Draft'а;
  `LibraryContent.Version` остаётся токеном записи (1 для Draft'а без строки), но
  флаг из него больше не выводится; `CreateLibrary` отдаёт `UnsavedChanges:
  false`.
- `DiscardService.Discard` — чисто при `len(warnings) == 0`; `Revert` — при
  `len(warnings) == 0 && sameProjectFile(...)`.
- `LibraryService.Discard` — чисто; `RevertToLatestRelease` — чисто, только если
  последний Release опубликовал последний Commit (сравнение копий WidgetTypes).
- `appdto.DraftState` получает `HasUnsavedChanges`; его заполняют `GET …/draft`,
  `Open`, `Discard`/`Revert` у Library. Ответы Project'а на Discard/Revert
  (`DiscardedDraft{version, warnings}`) не меняются — UI перечитывает `/draft`.
- `ProjectFileService.Open` идёт через `Update`, маркер не трогает: после Open
  флаг «грязный», даже если файл равен Commit'у (принятое ограничение).

### Контракт и REST

- `apiv0.DraftStateResponse` получает `has_unsaved_changes bool`; `api/v0`
  0.32 → 0.33, `make schemas` (0.32 уходит в `frozen.sha256`). `DiscardLibrary`
  и `RevertLibrary` — алиасы этого типа и получают поле автоматически; в
  `Problem.draft` (409 `draft_conflict`) значим только `version` — это
  записывается в комментарий типа.
- Bruno (`tests/api/bruno_collections/growscada_server/`): `get_project_draft`,
  `get_library_draft`, ответы Discard/Revert Library проверяют поле; порядок
  `seq` и `requestBodies` не меняются.

### UI (`internal/server/interface/ui/`)

- `ui/project` и `ui/library`: удаляются `cleanDraftVersion` и
  `hasUnsavedChanges()` из `draftVersion`/`lastCommitVersion`; шапка (Save,
  «Unsaved changes»/«Saved», строка статуса, `Ctrl+S`) читает флаг из состояния
  Draft'а.
- `loadDraftState` читается на mount, на `EventResync`, после каждого события
  содержимого и после каждой команды — те же точки, что у `loadDraftChanges`, —
  и применяет ответ под той же защитой от отставшей версии, что
  `applyDraftChanges` (иначе поздний ответ погасит бейдж).
- `doDiscard`/`doRevert` больше ничего не выставляют; `releaseStateOf` для
  выбранной библиотеки перестаёт подменять серверный флаг локальным.
  `ui/picker` не меняется.

### Тесты

- Домен: переходы `HasUnsavedChanges` (Save, запись содержимого, Discard,
  `Replace` с чистым и нечистым признаком, Undo); отказ `SaveCommit` на чистой
  версии.
- Репозиторий: маркер после Save/Discard/Revert; `nothing_to_save` после
  Discard; Revert к Release последнего Commit'а чист, к старому — нет.
- Application: флаг в `GET /projects`, `GET /libraries`, `GET …/draft` для
  «не писали», «писали, не сохраняли», «сохранили», «Discarded», «Reverted»,
  «Discard с warnings»; новорождённая Library — «never saved».
- UI: бейдж и кнопка Save в обоих редакторах после Save/Discard/Revert и после
  чужого Discard; `TestPicker_LibraryStates` — все пять состояний.

### Документы

- ADR 0016 — поправка 2026-11-10 (Clean Version, кто ставит, ограничение);
  ADR 0021 — поправка 2026-11-10 (отказ Save по чистой версии); `GLOSSARY.md` —
  термин **Clean Version** и уточнения Save/Discard/Revert (уже записаны).
- `AGENTS.md`: абзацы о Projects (`has_unsaved_changes` по версии и Commit'у),
  Libraries (`ReleaseStateOf`), Commits (`ErrNothingToSave`), Discard/Revert,
  шапках редакторов (`cleanDraftVersion` задач 72_6/72_7) и список миграций
  engineering; `CHANGELOG.md` — строка задачи.

## Приёмка

1. Новый проект/библиотека без единой записи — бейджа нет; после записи
   содержимого — бейдж есть; после Save — исчез; после Discard к последнему
   Commit — исчез; после Revert к Deployment/Release последнего Commit'а — исчез.
2. После Discard к Commit'у и последующей новой правки бейдж появляется снова.
3. Discard или Revert, выбросивший пины с их виджетами (warnings), оставляет
   бейдж: содержимое не равно Commit'у.
4. Флаг одинаково верен в `GET /projects`, `GET /libraries` и в обоих редакторах
   — в том числе после чужого Discard (событие) и после перезагрузки страницы.
5. `Save` на чистой версии — 409 `nothing_to_save`; после Discard кнопка Save
   выключена и Commit с тем же содержимым не создаётся.
6. Новорождённая Library на странице выбора — «never saved»; после первой правки
   — «Unsaved changes»; после Save — «unpublished commit»; после Release —
   «published»; у Library со старым Release без Commit — «legacy» (приёмка 75).
7. `make build`, `make test`, `make bruno_test` зелёные; `make schemas` применён,
   `api/v0` 0.33; проверка в браузере пройдена (правило 9).

## Связанное

- Задача 73 — ввод `has_unsaved_changes` у Project и страницы выбора.
- Задача 72_6, 72_7 — индикатор и `cleanDraftVersion` у Project и Library.
- Задача 75 — поглощена этой (семантика `has_unsaved_changes` у Library).
- ADR 0016 (поправка 2026-11-10), ADR 0021 (поправка 2026-11-10), ADR 0008,
  ADR 0011, ADR 0022; `GLOSSARY.md` — **Clean Version**.

## Comments

- 2026-11-10: заведено по итогам ревью задачи 73. Решение требовало выбора
  механизма, поэтому `needs-triage`.
- 2026-11-10: грилинг. Q1 пересматривался дважды: обсуждалось сравнение
  содержимого без нового состояния (точнее, но сборка Project'а на каждое
  чтение флага) против маркера — выбран маркер (Q1). Маркер — поле агрегата
  Draft, а не read-model application (Q2). Revert считается чистым по равенству
  содержимого, а не по провенансу Release (Q3). `nothing_to_save` — по тому же
  маркеру, без второго определения (Q4). Флаг едет в `DraftStateResponse`, UI его
  только рисует (Q5). Задача 75 влита: семантика «Draft без строки — чистый» и
  отказ `CreateLibrary` от `UnsavedChanges: true` входят сюда (Q6).
- 2026-11-10: аудит по DDD (правило 8) — расхождений нет: маркер пишется в
  транзакции самого Draft'а (ADR 0008), Runtime-роль не меняется (ADR 0011),
  термины и поправки ADR записаны. Статус — `ready-for-agent`.
