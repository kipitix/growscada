# Удалить Release у Project и Library

Status: ready-for-agent
Priority: 1
Parent: 85_labels_instead_of_releases
Part: 7/8
Blocked by: 85_5_deploy_window, 85_6_import_library_state

Часть 7 из `85_labels_instead_of_releases.md`. Слой C+D. После 85_4–85_6
Deploy идёт по Commit'у, а импорт — по сохранённому состоянию Library; Release
больше никем не используется. Здесь он удаляется полностью.

## Что удаляется

### Project Release

- `domain/release` целиком (`Release`, `Number`, `Note`, `Author`, `Origin`,
  `CreateRelease`, `CreateReleaseFromFile`, события `release_created`,
  `release_note_updated`).
- `application/release_application_service.go`, `appdto.Release`,
  `project_release_repository_postgres.go`, `project_release_library_pins`.
- Маршруты удаляются полностью (404, без заглушек):
  - `GET/POST …/projects/{id}/releases`
  - `GET/PATCH …/projects/{id}/releases/{n}`
  - `GET …/projects/{id}/releases/{n}/projectfile`
  - `POST …/projects/{id}/releases/{n}/deploy`
- `CommitResponse.released` и `ReleasedCommits` репозитория Release;
  `NewCommitService` больше не принимает Release-репозиторий.
- `eventlog`: `release_created`, `release_note_updated`,
  `ReloadTargets.Releases`; `apiv0.Event.Release` удаляется.
- `apiclient`: `CreateRelease`, `Releases`, `Release`, `UpdateReleaseNote`,
  `ReleaseProjectFile`, `DeployRelease` удаляются.
- Миграция `engineering`: `DROP TABLE engineering.project_release_library_pins`,
  `DROP TABLE engineering.project_releases`.
- Deploy file уже не создаёт Release (85_4); маршрут
  `POST …/projectfile/deploy` остаётся и отвечает `{number, deployed_at}`.

### Library Release

- `domain/library/library_release.go` (`LibraryRelease`, `ReleaseNumber`,
  `ReleaseState`, `ReleaseStateOf`), `library_release_repository_postgres.go`,
  `ReleaseNumber`/`Origin`/`Author` VOs, если больше не используются.
- `LibraryService`: `Release`, `FindReleases`, `FindRelease`,
  `UpdateReleaseNote`, `RevertToLatestRelease`; `NewLibraryService` больше не
  принимает Release-репозиторий.
- `appdto.Library` поля `LatestRelease`, `HasUnpublishedChanges`;
  `LibraryResponse` — те же поля; `CreateReleaseRequest`,
  `LibraryReleaseResponse`; `RevertToReleasePath`.
- Маршруты удаляются полностью (404, без заглушек):
  - `GET/POST …/libraries/{id}/releases`
  - `GET/PATCH …/libraries/{id}/releases/{n}`
  - `POST …/libraries/{id}/revert-to-release`
- События `library_released`, `library_release_note_updated`; `Event.Release`.
- `NewLibraryCleanupPolicy` больше не удаляет Release: удаление Library
  каскадно удаляет её Commit'ы, Draft и WidgetTypes.
- Миграция `engineering`: `DROP TABLE engineering.released_widget_types`,
  `DROP TABLE engineering.library_releases` (после 85_6 Release больше не
  источник импортов). Down-миграция данные не возвращает — записать в файле.

### Остатки термина

- Механическое переименование `library.ReleasedWidgetType` →
  `library.WidgetTypeCopy`, `apiv0.ReleasedWidgetTypeResponse` →
  `apiv0.PinnedWidgetTypeResponse` (или `WidgetTypeCopyResponse`), методов
  `NewReleasedWidgetType*`/`ReleasedWidgetType*`; смысл — «копия WidgetType в
  сохранённом состоянии/ProjectFile». Поведение не меняется, контракт —
  MINOR. Если переименование разрастается, разрешено оставить внутренний тип,
  но публичный DTO переименовать обязательно и записать решение в Comments.

## UI

- `ui/savedstates.Screen` окончательно теряет Release: `Row.Released`,
  `Row.Release`, `Row.Note`, `OnRelease`, `OnNote`, `renderRelease`; строка —
  время, название, автор, лейблы (85_3), Load.
- Library mode: удалить `releases`, `loadReleases`, `releaseDialog`,
  `releaseCommit`, `releaseNote*`, `releasing`, `editingRelease`, `render*`
  Release; `Make Release` исчезает и у Library.
- Project mode: остатки Release-полей/тестов из 85_5 удалить.
- Выбор Library (`ui/picker`): состояния
  - `never saved`, `unsaved changes`, `saved` (есть сохранённые состояния);
  - убрать `unpublished saved state`, `published`, `legacy`;
  - статус: «Saved state #N» вместо «Release N · published»;
  - тесты состояний и скриншоты обновить.
- Library header: только unsaved-индикатор, без Release.
- `revert-to-release` уходит из API и UI; `Discard` (к последнему Commit)
  остаётся. `revert-to-deployed` у Project остаётся.
- History: `Load` — единственный способ вернуть состояние в Draft (task 84).

## growctl

- Удалить `release` и `releases` команды, `release` из e2e/документации;
  `deploy` принимает `--commit` (маршрут 85_4), не `--release`.
- `save` остаётся; `commits` показывает лейблы, если они есть.
- Library-команды: `release`/`releases` удаляются; `save` и `apply`
  остаются. Если у growctl есть pin/import — перевести на `commit` (проверить
  `internal/growctl/library.go`).
- `--library` / `releases --library` в справке и тестах убрать.

## Тесты и проверки

- Удалить/переписать все тесты Release (application, repository, REST, UI,
  growctl, servertest); `go build ./...` не должен знать `release`.
- Проверить, что удалённые маршруты больше не зарегистрированы и отвечают 404
  (заглушек нет); `routes()` и статический Bruno-тест не должны их знать.
- Library: удаление Library с Commit'ами, пинами и без них; cleanup policy.
- Picker: три состояния, тексты, тесты.
- Project: History без Make Release; Deploy saved state (85_5) зелёный.
- Bruno: старые release-запросы удалить из коллекции; сценарий без Release:
  Save Library → Import → Save Project → Deploy; `requestBodies` почистить.
- `make build`, `make test`, `make bruno_test`, ручной проход PWA.

## Документы

- ADR 0007/0012/0021, GLOSSARY, AGENTS, CHANGELOG, схемы — 85_8.
- Здесь — комментарии миграции о необратимости Down и о том, что Release
  заменён Commit'ом/deploy и Import.

## Вне объёма

- Обратная совместимость старых `/releases` и старых ProjectFile с
  `library_pins`/`release` (85_6 их уже отклоняет).
- Возврат Release в любой форме.

## Comments

- 2026-11-14: выделено из 85. Удаление Release — последний шаг, когда ни
  Deploy, ни Import, ни UI его уже не используют; до этого приложение
  остаётся работоспособным.
