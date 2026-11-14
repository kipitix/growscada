# Импорт сохранённого состояния Library в Project вместо pin по Release

Status: ready-for-agent
Priority: 1
Blocked by: 85_3_label_assignments, 85_4_deploy_saved_state

Часть 6 из `85_labels_instead_of_releases.md`. Слой C+D. Заменяет решение
ADR 0007/0012 о «закреплении Release» на импорт **сохранённого состояния**
Library; UI-процесс называется **Import**.

## Зачем

Project видит WidgetTypes Library только через выбранное сохранённое состояние.
Release как промежуточная копия больше не нужна: Commit неизменяем, его
содержимое можно читать напрямую. Окно Import должно позволить выбрать любое
сохранённое состояние Library и отфильтровать их по лейблу.

## Домен

- `librarypin.LibraryPin`:
  - `Release() library.ReleaseNumber` → `CommitNumber() int` (номер Commit'а
    в Draft'е Library); при необходимости — `CommitID() id.ID[draft.Commit]`;
  - `CreateLibraryPin(projectID, libraryID, commitNumber)` и
    `ReconstituteLibraryPin(projectID, libraryID, commitNumber)`;
  - `Switch(commit draft.Commit)` проверяет, что Commit — из той же Library;
  - события `library_pin_created/updated/deleted` остаются (UI переименует
    подписи в Import; переименование самих event-имён — вне объёма).
- `draft.Session.PutLibraryPin(ctx, libraryID, commitNumber)`:
  - читает Commit Library (`draft.Repository`/loader; Commit неизменяем);
  - проверяет, что Commit принадлежит Draft'у этой Library;
  - `ImpactOf` считает по `commit.Content().WidgetTypes()`;
  - `reconcileScenes` берёт WidgetTypes из Commit'а, а не из
    `library.LibraryRelease`.
- `LibraryPin` блокирует удаление Library, как раньше; удаление Commit'а
  невозможно (только вместе с Library), поэтому висячих пинов нет.
- `library.LibraryRelease` пока остаётся для чтения старых данных и будет
  удалён в 85_7; новые пины его не используют.

## Хранение и миграция

Миграция `engineering`:

1. **Синтетические Commit'ы для legacy Release.** Каждый
   `engineering.library_releases` с `origin = 'legacy'` (без Commit'а)
   становится Commit'ом соответствующей Library:
   - `draft_id` — Draft Library;
   - `number` — следующий после `drafts.last_commit_number` (по возрастанию
     release number);
   - `title = 'Legacy release N'`, `author = 'system'`,
     `created_at = release.created_at`,
     `draft_version = drafts.last_commit_draft_version`;
   - `content` — `{format: 1, kind: "library", widget_types: [...]}` из
     `engineering.released_widget_types` этого Release (поле `release_number`
     заменить на `commit_number` при сборе).
   - Поднять `last_commit_number` Draft'а; `clean_version` не трогать.
2. Построить соответствие `(library_id, release_number) → commit_number`:
   `library_releases.commit_number` для commit-backed, номер синтетического
   Commit'а для legacy.
3. `engineering.library_pins`: `release_number` → `commit_number` (и
   `commit_id` с FK на `engineering.commits(id)` — добавить), по карте.
4. `engineering.commit_library_pins` (пины Project'а): `release_number` →
   `commit_number` Library по карте.
5. `engineering.project_release_library_pins` (ещё существует до 85_7) — то же
   соответствие, чтобы старые Project Release не сломались до удаления.
6. JSONB документов:
   - `engineering.commits.content#>{project_file,library_pins}`:
     `[{"library_id":...,"release":N}]` → `[{"library_id":...,"commit":M}]`;
   - `runtime.deployments.config.library_pins` — то же;
   - если пина нет в карте — оставить строку и записать в лог миграции
     (такого быть не должно: FK держал).
7. **Стеки undo.** Снапшоты `engineering.draft_changes` хранят
   `LibraryPinState{release}` и после смены Release больше не применимы.
   Очистить `engineering.draft_changes` и выставить
   `engineering.drafts.stack_position = 0` (одноразовая потеря undo-истории;
   записать в CHANGELOG). Контент, Commit'ы и Deployment'ы не страдают.
8. Down-миграция: обратная карта невозможна без потерь; Down очищает
   `library_pins`/стек и не восстанавливает Release-пины. Это допустимо для
   миграции модели, но отметить в файле.

Проверить миграцию на контейнере с данными трёх видов: Release of Commit,
legacy Release, пин без Release (быть не должно) — тест в
`repositories/migrations_test.go`.

## ProjectFile (`contract/project/v0`, MINOR, `make schemas`)

- `LibraryPin{ LibraryID uuid.UUID; Commit int }`, JSON `commit`.
- Для чтения старых файлов: необязательное поле `Release int
  json:"release,omitempty"` (deprecated, только декодирование); после
  декодирования нормализуется в `Commit`; канонизация и Builder пишут только
  `commit`. Валидатор не принимает документ, где у пина нет `commit` после
  нормализации.
- `WidgetType`-копии и `LibraryPin` остаются provenance, Runtime их не читает.
- `project_file_builder.go`, `project_file_validator.go`, все тесты
  ProjectFile — на `commit`.
- Скачанные старые ProjectFile с `release` открываются (legacy-поле), но
  после Open сохраняются с `commit`.

## Application и REST (`contract/api/v0`, MINOR, `make schemas`)

- `appdto.LibraryPin`: `LibraryID`, `CommitNumber`, `Title`, `Author`,
  `SavedAt`, `Labels`, `Version`. Убрать `Release`, `Origin`, `Note`.
- `LibraryPinResponse`:
  `{library_id, commit_number, title, author, saved_at, labels, version}`;
  `PutLibraryPinRequest{commit, version}`; превью —
  `GET …/library-pins/{library_id}/impact?commit=N`.
- `LibraryPinService.PutLibraryPin(ctx, projectID, libraryID, commitNumber,
  version)`, `DeleteLibraryPin`, `FindLibraryPins`; проверки:
  Library жива, Commit существует в её Draft'е (404 `commit_not_found`),
  Draft Version (409 `draft_conflict`), impact (409 `orphaned_widgets`,
  400 `invalid_input`).
- `FindPinnedWidgetTypes`/`FindPinnedWidgetType`: WidgetTypes из Commit'а
  (`Commit.Content().WidgetTypes()`), а не из `released_widget_types`.
- `project_file_builder.go` пишет `commit`; `library_pins_of_deployment.go` и
  предупреждения Discard/Load говорят `commit` (текст — saved state).
- `RuntimeNode.DeploymentNamesLibrary` не меняется: он смотрит `library_id`
  пинов в документе.
- `apiclient`: `PinLibrary`/`UnpinLibrary`/`LibraryPinImpact` принимают
  commit; поля ответа.

## UI: окно Import

- Панель Project вместо inline-списка пинов: заголовок **Libraries**,
  список импортированных (имя Library, `saved state #N`, название, автор,
  чипы лейблов) и кнопка **Import libraries…**.
- Диалог Import libraries:
  - список всех Library (имя, последнее сохранение); у каждой — текущий
    импорт или «Not imported»;
  - выбор Library раскрывает её сохранённые состояния, новые первыми:
    `#N`, название, автор, время, labels, число WidgetTypes; **Import** на
    строке (для импортированной — **Switch**);
  - сверху — фильтр по лейблу: `All labels` + каждый лейбл; выбранный лейбл
    перечитывает сохранения выбранной Library с `?label=` (или фильтрует
    загруженный список; выбрать одно и закрепить тестом);
  - у импортированной Library — **Remove** (с тем же impact-превью);
  - impact: orphaned Widgets — отказ с именами; broken PortBindings —
    подтверждение; после успеха перечитать пины, WidgetTypes, Draft state;
  - Library без сохранений: «No saved states: save the library first.»;
  - пустой список Library: «No libraries yet.»;
  - ошибки 404/409 — toast; 409 `draft_conflict` — перечитать Draft и пины.
- Тексты: Import, Imported, Change import, Remove import; слово pin в UI не
  используется.
- История Load/Discard: предупреждения о недоступном импорте говорят
  `library <id> saved state N: K widgets dropped`.

## Тесты

- Домен: создание/переключение пина по Commit, чужой Commit, impact,
  reconcile bindings, удаление.
- Репозиторий: чтение пина с Commit'ом, FK, миграция (legacy Release →
  Commit, rewrite пинов/JSONB, очистка стеков), ProjectFile round-trip.
- Application/REST: import/switch/remove, 404 Commit, 409 Version/impact,
  `?commit=` preview, пины в ProjectFile, предупреждения Load.
- UI (`uitest`): окно Import, фильтр по лейблу, import/switch/remove,
  orphaned/broken confirm, Library без сохранений, событие другого клиента.
- Bruno: `put_library_pin` `{commit}`, `get_library_pins`, impact, switch,
  delete; сид-сценарий: Save Library → Import её saved state → Save Project →
  Deploy Project saved state (85_4).
- `make build`, `make test`, `make bruno_test`, ручной проход PWA.

## Документы

- `GLOSSARY.md`: **Import** — выбор сохранённого состояния Library, которое
  Project использует; **Library Pin** остаётся внутренним именем механизма.
- ADR 0007/0012 — поправки в 85_8.
- CHANGELOG: потеря undo-истории при миграции и смена семантики пина.

## Вне объёма

- Удаление `library_releases`/`released_widget_types` и API Release — 85_7.
- Переименование домена `librarypin` → `libraryimport` и маршрутов
  `library-pins` → `imports` (UI уже говорит Import).
- Импорт Project в Project.

## Comments

- 2026-11-14: выделено из 85. Legacy Release превращается в синтетический
  Commit, иначе его содержимое — единственная копия — потерялось бы при
  удалении Release. Стеки undo чистятся, потому что их снапшоты пинов
  ссылаются на исчезнувшую нумерацию.
