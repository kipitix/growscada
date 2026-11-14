# Импорт сохранённого состояния Library вместо pin по Release

Status: ready-for-agent
Priority: 1
Parent: 85_labels_instead_of_releases
Part: 6/8
Blocked by: 85_3_label_assignments, 85_4_deploy_saved_state

Часть 6 из `85_labels_instead_of_releases.md`. Слой C+D. Заменяет решение
ADR 0007/0012 о «закреплении Release» на импорт **сохранённого состояния**
Library. Переименование полное: домен, таблицы, события, API, ProjectFile,
манифест growctl и UI говорят **Import**.

## Зачем

Project видит WidgetTypes Library только через выбранное сохранённое состояние.
Release как промежуточная копия больше не нужна: Commit неизменяем, его
содержимое можно читать напрямую. Окно Import позволяет выбрать любое
сохранённое состояние Library и отфильтровать их по лейблу.

## Домен

- Пакет `domain/librarypin` → `domain/libraryimport`:
  - `LibraryImport` (бывший LibraryPin) — сущность Draft'а Project:
    `ProjectID()`, `LibraryID()`, `CommitNumber()`, `CommitID()`, `Switch`,
    `Delete`;
  - `CreateLibraryImport(projectID, libraryID, commit)`,
    `ReconstituteLibraryImport(...)`;
  - `ImportImpactOf` (бывший `ImpactOf`) считает по
    `commit.Content().WidgetTypes()`;
  - события `library_import_created/updated/deleted` (бывшие
    `library_pin_*`);
  - снапшоты DraftChange: `KindLibraryImport`,
    `LibraryImportState{LibraryID, Commit}` (стеки всё равно очищаются
    миграцией).
- `draft.Session.PutLibraryImport(ctx, libraryID, commitNumber)`:
  - читает Commit Library (Commit неизменяем), проверяет, что он из этой
    Library;
  - `ImportImpactOf` и `reconcileScenes` берут WidgetTypes из Commit'а;
  - `DeleteLibraryImport` — прежний unpin.
- Импорт блокирует удаление Library, как раньше. Удалить Commit нельзя иначе
  как вместе с Library, поэтому висячих импортов нет.
- `library.LibraryRelease` пока остаётся для commit-backed Release до 85_7;
  новые импорты его не используют.

## Хранение и миграция

Миграция `engineering` (после 85_4; project_releases ещё существуют):

1. **Карта commit-backed Release.** `(library_id, release_number) →
   commit_number` из `engineering.library_releases` с
   `commit_number IS NOT NULL`.
2. **Импорты.** `engineering.library_pins` переименовать в
   `engineering.library_imports`; `release_number` → `commit_number`; добавить
   `commit_id UUID NOT NULL REFERENCES engineering.commits(id)`.
   - commit-backed пин: `commit_number`/`commit_id` по карте;
   - legacy пин (Release без Commit): удалить строку; запомнить
     `(project_id, library_id, release_number)` для шага 4.
3. **Provenance.** `engineering.commit_library_pins` переименовать в
   `commit_library_imports`, `release_number` → `commit_number` по карте.
   `engineering.project_release_library_pins` — обновить по карте; legacy-пины
   удалить (таблица исчезнет в 85_7).
4. **Legacy Release: удалить вместе с импортами и Widget'ами.**
   - Для каждого legacy Release взять `released_widget_types` (type IDs).
   - В Draft'ах Project'ов, которые импортировали этот Release, удалить
     Widget'ы этих TypeID (и их PortBindings — они часть Widget'а).
   - Удалить сами импорты (шаг 2).
   - Удалить legacy Release из `engineering.library_releases` и его
     `released_widget_types`.
   - Синтетических Commit'ов нет. Это осознанная потеря данных: CHANGELOG.
5. **ProjectFile-документы переписать.**
   - `engineering.commits.content#>{project_file,library_pins}` →
     `library_imports`, `release` → `commit` по карте;
   - legacy-пины удалить из массива; копии WidgetTypes удалённых Release и
     Widget'ы этих типов удалить из документов;
   - то же для `runtime.deployments.config` — **включая работающую
     Deployment**. Operation сразу перестанет показывать эти Widget'ы.
   - Down-миграция данные не возвращает; записать в файле.
6. **Стеки undo.** `DELETE FROM engineering.draft_changes`;
   `UPDATE engineering.drafts SET stack_position = 0`.
7. **Версия Draft'ов, чей контент изменился** (удалены импорты/Widget'ы):
   `version = version + 1`, `clean_version = 0` — Draft честно показывает
   несохранённые изменения; клиенты не считают его равным Commit'у.
8. **Внешние старые ProjectFile.** Документ с `library_pins`/`release`
   отклоняется 400 `file_issues` (формат пина сменился); новый формат —
   `library_imports`/`commit`. Это осознанный разрыв при MAJOR 0; CHANGELOG.

Тест миграции на контейнере: commit-backed Release, legacy Release, импорт без
Release (быть не должно), Project с Widget'ами legacy-типов, работающая
Deployment с legacy-пином.

## ProjectFile (`contract/project/v0`, MINOR, `make schemas`)

- `LibraryPin` → `LibraryImport`:
  `{LibraryID uuid.UUID; Commit int}`; JSON `library_imports`, элемент
  `{"library_id":..., "commit":N}`.
- `ProjectFile.LibraryPins` → `LibraryImports`.
- Старый `library_pins`/`release` только диагностируется при Open
  (`file_issues`), не нормализуется.
- `WidgetType`-копии и `LibraryImport` остаются provenance, Runtime их не
  читает.
- Builder, validator, канонизация и тесты ProjectFile — на imports/commit.

## Application и REST (`contract/api/v0`, MINOR, `make schemas`)

- `appdto.LibraryPin` → `appdto.LibraryImport`: `LibraryID`, `CommitNumber`,
  `Title`, `Author`, `SavedAt`, `Labels`, `Version`.
- `LibraryImportResponse{library_id, commit_number, title, author, saved_at,
  labels, version}`; `PutLibraryImportRequest{commit, version}`; превью —
  `GET …/library-imports/{library_id}/impact?commit=N`.
- Маршруты (полное переименование):
  - `GET/PUT/DELETE …/projects/{id}/library-imports[/{library_id}]`
  - `GET …/projects/{id}/library-imports/{library_id}/impact`
  - `GET …/projects/{id}/library-imports/widget-types` (палитра).
- `LibraryImportService` (бывший `LibraryPinService`): `FindLibraryImports`,
  `PutLibraryImport`, `DeleteLibraryImport`, `FindImportedWidgetTypes`,
  `ImportImpact`; проверки: Library жива, Commit существует в её Draft'е (404
  `commit_not_found`), Draft Version (409 `draft_conflict`), impact (409
  `orphaned_widgets`, 400 `invalid_input`).
- `FindImportedWidgetTypes` берёт WidgetTypes из Commit'а.
- `project_file_builder.go` пишет `library_imports`/`commit`;
  `library_pins_of_deployment.go` → `library_imports_of_deployment.go`;
  предупреждения Discard/Load говорят `saved state N`.
- `RuntimeNode.DeploymentNamesLibrary` читает `library_imports`.
- `apiclient`: `Imports`, `ImportLibrary`, `RemoveImport`, `ImportImpact`,
  `ImportedWidgetTypes`; поля ответа.

## growctl

- Манифест: `spec.libraryPins` → `spec.libraryImports`, элемент
  `{library: <name>, commit: N}`; `plan.go`, `manifest.go`, тесты и
  e2e-манифесты обновить. `apply` больше не публикует Release ради пина.
- Отдельных команд импорта нет (как и раньше у пинов); импорт задаётся
  манифестом.
- `commits` печатает лейблы; `deploy --commit` — в 85_4/85_7.

## UI: окно Import

- Панель Project вместо inline-списка: заголовок **Imported libraries**,
  список импортированных (имя Library, `saved state #N`, название, автор,
  цветные чипы лейблов) и кнопка **Import libraries…**.
- Диалог Import libraries:
  - список всех Library (имя, последнее сохранение); у каждой — текущий
    импорт или «Not imported»;
  - выбор Library раскрывает её сохранённые состояния, новые первыми:
    `#N`, название, автор, время, лейблы, число WidgetTypes; **Import** на
    строке (для импортированной — **Switch**);
  - сверху — фильтр по лейблу: `All labels` + каждый лейбл; выбранный лейбл
    перечитывает сохранения выбранной Library с `?label=`; фильтр не сужает
    список Library;
  - у импортированной Library — **Remove** с impact-превью;
  - impact: orphaned Widgets — отказ с именами; broken PortBindings —
    подтверждение; после успеха перечитать импорты, WidgetTypes, Draft state;
  - Library без сохранений: «No saved states: save the library first.»;
  - пустой список Library: «No libraries yet.»;
  - 404/409 — toast; 409 `draft_conflict` — перечитать Draft и импорты.
- Тексты: Import, Imported, Change import, Remove import; слово pin в UI не
  используется.
- История Load/Discard: предупреждения — `library <id> saved state N:
  K widgets dropped`.

## Тесты

- Домен: создание/переключение импорта по Commit, чужой Commit, impact,
  reconcile bindings, удаление.
- Репозиторий: чтение импорта с Commit'ом, FK, миграция (карта, legacy
  удаление, перепись JSONB и работающей Deployment, очистка стеков, bump
  версии), ProjectFile round-trip.
- Application/REST: import/switch/remove, 404 Commit, 409 Version/impact,
  `?commit=` preview, импорты в ProjectFile, предупреждения Load.
- UI (`uitest`): окно Import, фильтр, import/switch/remove,
  orphaned/broken confirm, Library без сохранений, событие другого клиента.
- growctl: манифест `libraryImports`, e2e.
- Bruno: `put_library_import` `{commit}`, `get_library_imports`, impact,
  switch, delete; сид: Save Library → Import → Save Project → Deploy Project.
- `make build`, `make test`, `make bruno_test`, ручной проход PWA.

## Документы

- `GLOSSARY.md`: **Library Import** вместо Library Pin.
- ADR 0007/0012 — поправки в 85_8; ADR 0021 — исключение неизменяемости.
- CHANGELOG: потеря legacy Release, перепись документов, удаление импортов,
  разрыв формата ProjectFile.

## Вне объёма

- Удаление commit-backed `library_releases` и API Release — 85_7.
- Переименование `ReleasedWidgetType` → `WidgetTypeCopy` — 85_7.
- Импорт Project в Project.

## Comments

- 2026-11-14: выделено из 85.
- 2026-11-14 (грилинг): полное переименование pin → import; legacy Release
  удаляется с импортами и Widget'ами, без синтетических Commit'ов; старые
  Commit/Deployment/работающая Deployment переписываются; все стеки undo
  очищаются; внешние старые ProjectFile отклоняются.
