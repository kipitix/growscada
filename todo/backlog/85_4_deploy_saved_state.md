# Deploy сохранённого состояния: commit_number вместо release_number

Status: ready-for-agent
Priority: 1
Parent: 85_labels_instead_of_releases
Part: 4/8

Часть 4 из `85_labels_instead_of_releases.md`. Слой C (Runtime node и
Engineering node). Меняет ADR 0014/0021: Deployment помнит, какое **saved
state** (Commit) запущено, а не какой Release.

## Зачем

Release — лишний уровень между Commit'ом и Deployment'ом: Release копирует
Commit, но Deploy всё равно запускает содержимое, а не Release как таковой.
Убираем копию: Deploy берёт Commit напрямую. Это часть 1 из двух — UI нового
окна Deploy и удаление Release придут в 85_5/85_7; здесь меняется носитель и
серверные маршруты, приложение остаётся зелёным.

## Runtime node

Миграция `runtime` (одна, после `20261111000000_checkpoints`):

1. `ALTER TABLE runtime.deployments ADD COLUMN commit_number INTEGER` с
   `CHECK (commit_number IS NULL OR commit_number > 0)`.
2. Backfill: `commit_number` из `engineering.project_releases` по
   `(project_id, number)` → `release_number`, где `commit_number IS NOT NULL`.
   Deployment'ы из файла и старые остаются `NULL`.
3. Переписать `runtime.journal` строки типа `deployment`: взять
   `payload->>'release_number'`, найти Project Release, заменить
   `release_number` на `commit_number` (для file/legacy — просто убрать
   `release_number`), поднять `schema_version` этих строк до нового
   `record/v0`.
4. Переписать `runtime.checkpoints`: в `payload->'deployment'` сделать то же
   (`release_number` → `commit_number`, file/legacy — без поля).
5. Удалить `runtime.deployments.release_number` и его индекс; добавить
   `INDEX (project_id, commit_number)`.
6. Down-миграция восстанавливает `release_number` из
   `engineering.project_releases` (таблица ещё есть до 85_7) и удаляет
   `commit_number`.

Код:

- `application/runtime.Deployment`, `DeploymentSummary`:
  `CommitNumber *int` вместо `ReleaseNumber`.
- `DeploymentRepository.CreateDeployment(ctx, projectID, file, cause,
  commitNumber *int, now)`; `Rollback` копирует `commit_number` целевой
  Deployment; `DeployFile` — `nil`.
- `DeploymentService.DeployCommit(ctx, projectID, commitNumber int, file)`,
  проверка `commitNumber > 0`, публикация `deployment_created`; внутри —
  общий `deployContent(ctx, projectID, file, commitNumber *int, cause)`.
- `DeployFile` идёт через тот же `deployContent` с `nil` и причиной `file`.
- `ErrNothingToDeploy` — если у текущей Deployment тот же `commit_number`
  (только когда `commitNumber != nil`); проверка под тем же advisory lock,
  что нумерация (как сегодня).
- `deployment_created` (`application/runtime/deployment_event.go`):
  `CommitNumber() *int`.
- `contract/record/v0` (`SchemaVersion` 0.6, MINOR): `recordv0.Deployment`
  получает `CommitNumber *int json:"commit_number,omitempty"` вместо
  `ReleaseNumber`; `journal_codec.go` и `checkpoint_codec.go` обновить.
  Старые PlaybackFile с `release_number` не пересобираются (вне объёма), но
  строки Journal/Checkpoint миграция переписывает.

## Engineering node

- Порт `application.RuntimeNode`:
  `DeployCommit(ctx, projectID, commitNumber int, file projectv0.ProjectFile)
  (appdto.Deployment, error)` вместо `DeployRelease`; `DeployFile` остаётся.
  `internal/server/inprocess` переводит вызов в
  `runtime.DeploymentService.DeployCommit`.
- Новый `application.DeployService` (или метод `CommitService.Deploy` —
  выбрать одно, не дублировать):
  - `Deploy(ctx, projectID, commitNumber) (appdto.Deployment, error)`:
    Project жив и принадлежит Organization; Commit существует
    (`draft.ErrCommitNotFound` → 404); берёт `commit.Content()` как
    `ProjectFile`; зовёт `RuntimeNode.DeployCommit`; `ErrNothingToDeploy` →
    409 `nothing_to_deploy`; остальное — как сейчас.
  - Никакого Draft Version: Commit неизменяем, Deploy не читает Draft.
- `ProjectFileService.Deploy` (Deploy file) больше **не** создаёт Release:
  зовёт `RuntimeNode.DeployFile` с причиной `file` и `commit_number = nil`;
  ответ `{number, deployed_at}`. Это убирает `ReleaseService.CreateFromFile`
  из этого пути.
- `ReleaseService.Deploy` временно остаётся обёрткой для старого UI:
  читает Release; если `origin = commit`, зовёт `DeployService.Deploy` с его
  `commit_number`; если `origin = file` — отправляет содержимое с
  `commit_number = nil` (без проверки «уже работает»). Маршрут
  `POST …/releases/{n}/deploy` работает до 85_5; в 85_7 маршрут удаляется
  вместе с остальными `/releases` (404, без заглушки).
- `ReleaseService` (создание, список, заметка) не трогается — до 85_5/85_7.

## REST (`contract/api/v0`, MINOR, `make schemas`)

- Новый `POST /api/v0/projects/{id}/commits/{n}/deploy` → 201
  `DeployResponse{number, deployed_at}`; 404 `project_not_found` /
  `commit_not_found`; 409 `nothing_to_deploy`; 400 для нечислового `n`.
- `DeploymentResponse` и `DeploymentSummaryResponse`:
  `commit_number *int` вместо `release_number`; комментарии — «saved state the
  Deployment came from, nil for a file deploy or a Deployment made before
  saved states were deployed».
- `apiv0.Event`: `CommitNumber *int json:"commit_number"` вместо
  `ReleaseNumber` на `deployment_created`.
- `apiclient`: `DeployCommit(ctx, commitNumber)`, поля Deployment
  `CommitNumber`.
- `eventlog`/Project mode: `deployment_created` перечитывает Deployment;
  `deployment_commit_number` доступен.

## Минимальные правки UI (чтобы часть была зелёной)

Полное новое окно — 85_5; здесь только компиляция и корректные подписи:

- Project badge: `deployment.CommitNumber != nil` → `Saved state #N · running`;
  иначе `Deployment N · no saved state`.
- Маркер running в списке Release (он ещё существует): сравнивать
  `deployment.CommitNumber` с `release.CommitNumber` (`ReleaseResponse` уже
  несёт `commit_number`), а не `release_number`.
- Deployments list/detail: `commit_number`, подпись «Saved state #N».
- Toast Deploy Release остаётся до 85_5.

## Тесты

- Runtime: миграция backfill (Deploy, file, rollback), `DeployCommit`,
  `ErrNothingToDeploy` под гонкой двух Deploy, Rollback копирует
  `commit_number`, file deploy без номера, Journal-запись с `commit_number`,
  Checkpoint с `commit_number`, чтение старых строк после миграции.
- Engineering/inprocess: Deploy Commit (404/409/400), обёртка Release →
  Commit, Deploy file не создаёт Release.
- REST: новый маршрут, поля `commit_number` в deployment/list/event, старый
  `POST …/releases/{n}/deploy` всё ещё работает.
- UI: badge и маркер running по `commit_number`; существующие тесты
  Deploy Release обновлены.
- `servertest` (хелпер Deploy) и тесты, которые деплоят Release, переводятся
  на Commit, где это возможно без 85_5.
- Bruno: `post_commit_deploy`, `post_commit_deploy_unchanged` (409),
  `get_deployment` с `commit_number`; старые release-deploy запросы остаются.
- `make build`, `make test`, `make bruno_test`.

## Документы

- `CHANGELOG` и ADR 0021 — в 85_8; здесь только комментарии кода и миграции.
- `GLOSSARY.md`: Deployment — «номер Commit'а, который запущен»; Release пока
  остаётся, будет удалён в 85_7.

## Вне объёма

- Новое окно Deploy с фильтром по лейблу — 85_5.
- Удаление Project/Library Release из API и БД — 85_7.
- growctl: `deploy --release` продолжает работать через обёртку; `deploy
  --commit` и удаление `release` — 85_7/85_8.

## Comments

- 2026-11-14: выделено из 85. Решение — Deployment помнит Commit, Release
  исчезает; старые данные backfill'ятся, file-deploy больше не порождает
  Release.
