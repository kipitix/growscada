# Новое окно Deploy по сохранённым состояниям с фильтром по лейблу

Status: ready-for-agent
Priority: 1
Blocked by: 85_3_label_assignments, 85_4_deploy_saved_state

Часть 5 из `85_labels_instead_of_releases.md`. Слой D, режим Project.

## Зачем

Deploy теперь запускает Commit (85_4), но окно всё ещё показывает Release'ы.
Нужно окно, где инженер видит сохранённые состояния Project'а, может отфильтровать
их по лейблу и задеплоить выбранное. Release из Project-режима исчезает.

## Что должно быть в окне

Кнопка **Deploy** в панели Project открывает диалог «Deploy a saved state»:

- шапка: заголовок и фильтр по лейблу — `All labels` (по умолчанию) + каждый
  лейбл из `GET /labels`; при выборе лейбла список перечитывается с
  `GET …/commits?label=<id>`, а не фильтруется в памяти;
- если лейблов нет — фильтра нет, подсказка «No labels yet» с кнопкой,
  открывающей окно лейблов (85_2);
- список сохранённых состояний, новые первыми: `#N`, название, автор, время,
  счётчики (scenes/widgets/tags/devices/environments), чипы лейблов;
- на строке: **Deploy** (кроме работающей) и **Project file** — скачивание
  `GET …/commits/{n}/projectfile` (маршрут уже есть);
- работающая строка помечена **Running**, если
  `deployment.commit_number == commit.number`; если Deployment без Commit
  (файл, старый), над списком предупреждение «Running Deployment N (no saved
  state): choose a saved state to deploy.»;
- если `GET …/deployment` не ответил — список остаётся, маркер «running
  cannot be checked» (как сегодня `deploymentFailed`);
- пусто: «Nothing saved yet. Save the Draft first: a saved state is what gets
  deployed.»;
- Deploy: подтверждение «Make saved state N the running one? Operation switches
  to it.», вызов `pc.DeployCommit(ctx, N)`, toast «Saved state N deployed» +
  «Deployment M is running», перечитать список, `GET …/deployment` и список
  Deployments; 409 `nothing_to_deploy` — toast «This saved state is running
  already»; остальное — `uiutil.ShowError`.

## Правки режима Project

- Убрать из `project.Project` поля и загрузку Release: `releases`,
  `releasesLoaded`, `releasesReloader`, `releaseDialog`, `releaseCommit`,
  `releaseNote`, `releaseAuthor`, `releasing`, `editingRelease`,
  `releaseNoteDraft`; удалить `project/releases.go`, вызовы `loadReleases` из
  `load.go`, `project.go`, `server_events.go`, `projectfile.go`.
- `deploy.go`/`releases.go`: `openDeployDialog` грузит `loadCommits`,
  `loadDeploy`, `loadLabels`; `deployRelease` → `deploySavedState`.
- Badge панели: `CommitNumber != nil` → `Saved state #N · running`; file/старый
  → `Deployment N · no saved state`; `deploymentFailed` → `Deployment unknown`.
- `deployButtonTitle`: нет сохранений → «Save the Draft first: only a saved
  state is deployed»; иначе «Choose a saved state to deploy».
- Deployments dialog: строка и detail показывают «Saved state #N» по
  `commit_number`, cause — как сейчас.
- History: `OnRelease` у Project больше не передаётся (кнопка Make Release
  исчезает из Project-строк); `ui/savedstates` получает флаг/пустой callback,
  чтобы Library временно сохранила кнопку до 85_7. Строки Project показывают
  лейблы (85_3).
- События: `release_created`/`release_note_updated` больше не перечитывают
  Project-режим; `ReloadTargets.Releases` остаётся только для Library до 85_7.
  `commit_created`, `label_*`, `label_assigned/unassigned` перечитывают
  окно/историю.
- `deployment_created` перечитывает `GET …/deployment` и, если открыт,
  список Deployments и окно Deploy.

## apiclient

- `ProjectClient.DeployCommit(ctx, commitNumber) (apiv0.DeployResponse, error)`
  (маршрут из 85_4); `ReleaseDeploy` остаётся только для Library? У Library
  своего Deploy нет — удалить из `apiclient` методы Project Release, если они
  больше нигде не используются (`CreateRelease`, `Releases`, `Release`,
  `UpdateReleaseNote`, `ReleaseProjectFile`, `DeployRelease`) — удаление
  серверных маршрутов в 85_7, но клиент Project их уже не зовёт.
- `Commits` с фильтром по лейблу.

## Тесты

- UI (`uitest`, Project):
  - окно показывает сохранения, а не Release; пустое состояние;
  - фильтр «All» и по лейблу перечитывает список с `?label=`;
  - удалённый выбранный лейбл сбрасывает фильтр на All;
  - Running по `commit_number`; Deployment без Commit — предупреждение;
  - Deploy: подтверждение, успех, 409, ошибка сети;
  - скачивание ProjectFile сохранения;
  - `deployment_created` другого клиента обновляет маркер.
- History Project: Make Release больше нет; лейблы видны; Load работает.
- Существующие тесты `releases_test.go`, `deploy_test.go` переписать на
  Commit'ы; `ops_test`/`deployments_test` обновить.
- Bruno: сценарий Deploy теперь `post_commit` → `post_commit_deploy`
  (85_4); Project Release запросы остаются до 85_7.
- `make build`, `make test`, `make bruno_test`, ручной проход PWA.

## Документы

- UI-тексты: Release → saved state; «Deploy a Release» → «Deploy a saved
  state»; «Make Release» в Project исчезает.
- ADR/GLOSSARY/AGENTS — 85_8.

## Вне объёма

- Library Release UI и API (85_7).
- Изменение состава Commit'ов, retention.
- Фильтр по нескольким лейблам.

## Comments

- 2026-11-14: выделено из 85. Фильтр серверный (`?label=`), чтобы он же
  работал в growctl и Bruno; окно Project перестаёт знать о Release.
