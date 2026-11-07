# Deploy Project Release, работающая Deployment и Rollback

Status: ready-for-agent
Priority: 1
Blocked by: 72_3, 72_9

Слой C. Термины — `GLOSSARY.md` (Release, Deploy, Deployment, Rollback). Часть 4 из `72_save_release_deploy.md`. Термин Revision уже переименован в Deployment задачей 72_9.

## Задача

Deploy активирует **Project Release**, а не Draft. Runtime node по-прежнему хранит запись о запуске — теперь **Deployment**, — но знает, из какой Release она сделана (`release_number`). Список Release проекта помечает работающую; любую Release можно выбрать и задеплоить. Старый Deploy Draft уходит.

## Runtime node

- Миграция `runtime`: `ALTER TABLE runtime.deployments ADD COLUMN release_number INTEGER` с `CHECK (release_number IS NULL OR release_number > 0)`. Старые Deployment получают `NULL`; индекс `(project_id, release_number)` не уникальный — одну Release можно задеплоить повторно (Rollback/повторный выбор).
- `runtime.Deployment` и `DeploymentSummary` получают `ReleaseNumber *int` (nil — Deployment из файла или до появления Releases).
- `DeploymentCause` остаётся `deploy`/`rollback`/`file`; `deploy` теперь означает «Deploy Release».
- `DeploymentRepository.CreateDeployment` получает `releaseNumber *int`:
  - `Deploy Release` — `cause=deploy`, `release_number=n`;
  - `Deploy file` — `cause=file`, `release_number=nil`;
  - `Rollback` — копирует `release_number` целевой Deployment (файловая остаётся с nil), `rolled_back_from` как сегодня.
- В транзакции Deploy Release проверить текущую Deployment: если её `release_number` равен запрошенному — `ErrNothingToDeploy` (409). Проверка под тем же advisory lock, что нумерация, иначе два одновременных Deploy одной Release создадут две Deployment.
- `DeploymentService`:
  - `DeployRelease(ctx, projectID, releaseNumber int, file projectv0.ProjectFile) (Deployment, error)`;
  - `DeployFile` остаётся, но не создаёт Release (см. ниже) и ставит `nil`;
  - `FindCurrent`, `List`, `Find`, `Rollback` отдают `ReleaseNumber`.
- `deployment_created`: добавить `release_number` (nullable) в событие, чтобы Project-режим мог перечитать список Release. `apiv0.DeploymentResponse` и `DeploymentSummaryResponse` получают `release_number` (MINOR).
- Journal (`contract/record/v0`): запись `deployment` получает `release_number` (nullable, MINOR). Старые записи без поля декодируются как nil; `journal_codec.go` обновить.
- Operation/Player не меняют поведение: `GET …/deployment` отдаёт дополнительное поле, Player его игнорирует.

## Engineering node

- `ReleaseService.Deploy(ctx, projectID, releaseNumber int, author string) (appdto.Deployment, error)`:
  1. Project жив; Release существует и принадлежит Project (`ErrReleaseNotFound`);
  2. берёт `Content` Release (ProjectFile);
  3. `RuntimeNode.DeployRelease(ctx, projectID, releaseNumber, file)`;
  4. `ErrNothingToDeploy` (409), если эта Release уже работает; 404 — нет Release; ошибки Runtime пробрасываются.
- `ReleaseService.List` **не** ходит в Runtime: список Release — данные Engineering node и работает, даже если Runtime недоступен. Работающую Release UI определяет двумя чтениями: `GET …/releases` + `GET …/deployment`; сравнивает `Deployment.release_number` с номером Release. Если Runtime недоступен, список виден, а маркер — «не удалось проверить»; если Deployment без `release_number`, маркера нет (строка «работает Deployment N (не Release)»).
- `ReleaseService.CreateFromFile(ctx, projectID, file, author, title string) (Release, error)` для Deploy file:
  - валидирует документ тем же валидатором, что `POST …/projectfile/deploy` (72_4 не меняет правила валидации);
  - создаёт Release с `origin=file`, `commit_number=nil`, заголовком (по умолчанию имя файла или «Файл»), автором (по умолчанию `file`, если не передан), копией ProjectFile;
  - затем деплоит её. Так у работающей Deployment появляется Release, и список Release показывает её как работающую.
  - Если Deploy после создания Release упал, Release остаётся неработающей и доступна для повторного Deploy; это осознанная цена двух шагов (Release-агрегат и Runtime-узел). Не откатывать Release удалением — Release неизменяем.
  - Если файл деплоится на узле без Engineering node (задача 52) — допустим прямой `DeployFile` с `release_number=nil`; UI показывает «работает Deployment N (не Release)».
- `POST …/projects/{id}/releases/{n}/deploy` → 201 `{number, deployed_at}`; 409 `nothing_to_deploy`; 404; 400.
- `POST …/deploy` (Deploy Draft) удаляется и отвечает 410 `gone` с подсказкой «Save → Release → Deploy». `GET …/deploy/preview` удаляется; его роль выполняют `GET …/deployment` (что работает) и список Release. `DeployService`, `Preview`, `DeployState` и их тесты удалить, если больше нет читателей.
- `POST …/deployments/{n}/rollback` остаётся (History/Runtime), но UI Project переходит на Deploy Release. Rollback копирует Deployment и сохраняет `release_number`: Release снова становится работающей.
- **Discard** Project сбрасывает Draft к **последнему Commit'у** (отменить несохранённое), а не к работающей Deployment. Старое поведение «сброс к работающей Deployment» становится отдельным `POST …/projects/{id}/revert-to-deployed` (Revert to deployed). Оба — одна транзакция над Draft (ADR 0016), чистят стек undo/redo, не трогают Commit/Release/Deployment. `GET …/deploy/preview` после Revert показывает, что Draft снова совпадает с работающей Deployment.
- Если Commit'ов нет (Draft ещё не сохраняли), Discard — 409 `nothing_to_discard`; Revert работает, если есть работающая Deployment, иначе 409.
- Старый `POST …/discard` меняет смысл на «к последнему Commit»; в CHANGELOG и Bruno это отметить (пока MAJOR 0).
- `POST …/projectfile/deploy` использует `CreateFromFile`; автор/заголовок — `?author=&title=` (необязательные) или значения по умолчанию. Контракт — MINOR.

## REST и контракт

- `api/v0`: `DeploymentResponse` + `release_number`, `DeploymentSummaryResponse` + `release_number`, событие `deployment_created` + `release_number`; новый `POST …/releases/{n}/deploy`; удалён `POST …/deploy` (410). MINOR (MAJOR = 0), `make schemas`.
- `apiclient`: `DeployRelease(projectID, n)`, `DeploymentState.ReleaseNumber`; удалить `DeployDraft`, `DeployPreview`, если они больше не нужны.
- Bruno: заменить `post_deploy` на `post_release_deploy`; добавить проверку 409 повторного Deploy, 404, 410 старого `post_deploy`; обновить `requestBodies`; `seq`: Save → Release → Deploy → `PATCH …/value`.
- Симулятор/Bruno симулятора: деплоят Release перед запуском драйвера.

## UI (минимально; полный Project UI — 72_6)

- Список Release получает кнопку **Deploy** у каждой неработающей Release и пометку **Работает** у работающей. Подтверждение: «Сделать Release N работающей? Operation переключится на неё».
- Кнопка Deploy в шапке Project открывает список Release, а не диалог Deploy Draft.
- После успеха — toast, перечитать список Release и `GET …/deployment`; событие `deployment_created` делает то же в других вкладках.
- Если Deployment работает без Release (`release_number=nil`), список показывает строку «Работает Deployment N (не Release)» и предлагает выбрать Release.
- Старый диалог Deploy Draft и кнопка Rollback в Project убираются (Rollback остаётся в API/History).

## Тесты

- Runtime: миграция и `release_number`; Deploy Release создаёт Deployment с номером; повторный Deploy той же Release — `ErrNothingToDeploy`; две конкурентные Deploy — одна Deployment; Deploy file — nil; Rollback сохраняет номер Release; `FindCurrent`/`List`/`Find` отдают номер; Journal-запись с `release_number`.
- Engineering: Deploy Release, 409/404; `GET …/releases` работает без Runtime; UI-маркер по `GET …/deployment` (совпадает/не совпадает/nil/нет Deployment/ошибка Runtime); Release из файла создаётся и деплоится; Discard к последнему Commit'у; Revert to deployed; `POST …/deploy` — 410.
- Operation/Player: `GET …/deployment` с новым полем; Player не падает.
- UI (`uitest`): Deploy из списка, пометка работающей переезжает, повторный Deploy — toast, Deployment без Release — строка.
- Bruno: новый flow, 410 старого маршрута, `requestBodies`.
- `make build`, `make test`, `make bruno_test`.

## Документы

- ADR 0013 (поправка: Deploy берёт Release, а не Draft; Runtime-запись — Deployment), ADR 0014 (Rollback сохраняет Release), ADR 0018 (Deploy file создаёт Release), ADR 0019 (Journal: `deployment` + `release_number`), ADR 0020 (Operation читает Deployment).
- `GLOSSARY.md`: Deploy, Release, Deployment, Rollback.
- `AGENTS.md`, `CHANGELOG.md`.

## Вне объёма

- Права на Deploy — задача 50.
- History/PlaybackFile — задачи 45/46 (они получат `release_number`).
- Обратное переименование Deployment → Revision не делается.

## Comments

- 2026-11-07: выделено из 72. `release_number` nullable — компромисс с уже работающими Deployment и Deploy file; UI честно показывает «не Release». Зависит от 72_9 (переименование Revision → Deployment).
