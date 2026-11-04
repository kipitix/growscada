# Rollback и список Revisions
Status: ready-for-agent

Слой B. Термины — `GLOSSARY.md` (Revision, Rollback, Deploy, Discard); решения — ADR 0014 и поправка 2026-11-04 к ADR 0013. Работает на Runtime-ноде без Engineering-ноды: Rollback не читает Draft.

## Что сделать

### Хранение и домен Runtime-ноды

- Миграция схемы `runtime`: `runtime.revisions` получает `cause text NOT NULL` (`deploy` | `rollback`) и `rolled_back_from integer NULL`; существующие строки получают `cause = 'deploy'`, затем колонка становится `NOT NULL` (без default — новый писатель обязан назвать причину). `CHECK (cause IN ('deploy','rollback'))`, `CHECK ((cause = 'deploy' AND rolled_back_from IS NULL) OR (cause = 'rollback' AND rolled_back_from IS NOT NULL))`; самоссылочный FK `(project_id, rolled_back_from) → runtime.revisions(project_id, n)` с `ON DELETE CASCADE` (уникальный ключ уже есть). Номер Revision по-прежнему `MAX(n)+1` под advisory-lock `hashtextextended(project_id, 0)`.
- `application/runtime.Revision` получает `Cause` и `RolledBackFrom`; `RevisionRepository`:
  - `FindAll(ctx, projectID) ([]RevisionSummary, error)` — сводки новыми первыми, без документов; пустой список — не ошибка;
  - `FindByNumber(ctx, projectID, n) (Revision, error)` — полный документ; нет строки — `ErrRevisionNotFound`;
  - `Rollback(ctx, projectID, n, now) (Revision, error)` — в одной транзакции: advisory-lock проекта; `SELECT … FROM public.projects WHERE id = $1 AND deleted_at IS NULL FOR SHARE` (нет строки — `project.ErrProjectNotFound`; `discarding` — `project.ErrProjectDiscarding`); прочитать текущую Revision (`ORDER BY n DESC LIMIT 1`) и целевую; сравнить содержимое (jsonb-равенство `config`) — совпало — `ErrNothingToRollback`; вставить `n = max+1`, `cause = 'rollback'`, `rolled_back_from = n целевой`, `deployed_at = now`, `config` целевой без изменений; `refreshCopies` по декодированному документу (тот же дифф, что Deploy: состояния совпавших Tag'ов и Device Tokens живут, исчезнувшие состояния уходят, новые/со сменённым TagType — initial); коммит.
- `application/runtime.RevisionService`: `List`, `Find` (по номеру), `Rollback`; после коммита — тот же `revision_created` (номер, время, проект), без outbox и без новых полей события. Ошибки: `ErrRevisionNotFound` (404), `ErrNothingToRollback` (409); проект не жив — `project.ErrProjectNotFound`; идёт Discard — `project.ErrProjectDiscarding`.
- `appdto.Revision` (граница ролей) и `inprocess` переносят `Cause`/`RolledBackFrom`, чтобы `deploy/preview.current_revision` нёс причину; `Deploy` заполняет `cause = 'deploy'`.

### REST (маршруты Runtime-ноды)

- `GET /api/v0/projects/{id}/revisions` → 200 `{revisions: [{number, deployed_at, cause, rolled_back_from?}]}`, новыми первыми; у проекта без Revision — 200 с пустым массивом; удалённый проект — 404 `project_not_found`.
- `GET /api/v0/projects/{id}/revisions/{revision_number}` → 200 `RevisionResponse` (тот же shape, что `GET …/revision`, плюс `cause`/`rolled_back_from` и `content`); нет номера или проект не жив — 404 `revision_not_found` / `project_not_found`; нечисловой номер — 400.
- `POST /api/v0/projects/{id}/revisions/{revision_number}/rollback` без тела → 201 `{number, deployed_at, cause, rolled_back_from}`; 404 цели; 409 `nothing_to_rollback`, если содержимое равно текущей Revision, и `project_discarding`, если идёт Discard. Никаких обращений к Engineering-ноде.
- Контракт `contract/api/v0`: тип `RevisionCause` (`deploy`, `rollback`), поля `cause` и `rolled_back_from` в `RevisionSummaryResponse` и `RevisionResponse`, `ListRevisionsResponse`; `SchemaVersion` — MINOR (0.12 → 0.13), `make schemas`. `contract/project/v0` не меняется: документ копируется как есть, `schema_version` не перештамповывается и не проверяется (пока MAJOR 0 совместимость не обещана).
- `restapi`: `revisionResource` (`revision_number`), парсер целого номера, маршруты в `routes()`, маппинг `ErrRevisionNotFound`/`ErrNothingToRollback`; каждый маршрут — в Bruno и в `requestBodies`, если понадобится тело.

### Чистка Engineering-ноды: копия ProjectFile уходит

- Rollback делает копию «последний отправленный Engineering-нодой ProjectFile» устаревшей; её читателей нет: Project-режим берёт Revision из `deploy/preview`, а `DeployService.State` никем не вызывается. Удалить `engineering.project_files` (миграция цепочки `engineering`), `ProjectFileCopies`, `ProjectFileRepositoryPostgres`, `appdto.DeployState`, `DeployService.State`, `ErrRevisionCopyNotSaved` и его 500-маппинг; `Deploy` сравнивает Draft с Revision через `RuntimeNode` и при равенстве всегда отвечает 409 `nothing_to_deploy` (никакой «починки копии» и 201 на повторный Deploy). `Preview` не меняется. `servertest`, тесты и `main.go` обновить.

### apiclient и PWA (Project-режим)

- `apiclient.ProjectClient`: `Revisions`, `RevisionByNumber`, `Rollback`; контрактные типы как есть.
- В deploy-баре Project-режима — кнопка `Revisions`, всегда доступная (у неразвёрнутого проекта диалог пуст). Диалог: список новыми первыми, номер, время, причина (`Deployed` / `Rolled back from #N`), текущая Revision помечена и её откат недоступен; выбор строки подгружает `GET …/revisions/{n}` и показывает сводку для сравнения (счётчики Scenes, Widgets, Tags, Devices, Environments), затем `Roll back` с `confirm()`; успех — закрыть диалог, перезагрузить preview и список; ошибки (`nothing_to_rollback`, 404, 409 Discard) — toast. `revision_created` и resync перезагружают открытый список; кнопки отключены, пока идёт Deploy/Discard.
- UI-тесты (`ui/uitest`): список Revisions после Deploy, откат создаёт новую Revision и обновляет бейдж, пустой список, ошибка 409 видна; текущая строка не откатывается.

### Bruno, тесты, документация

- Bruno-коллекция сервера: `GET …/revisions` (после Deploy — одна Revision), `GET …/revisions/{n}` с содержимым, `POST …/revisions/{n}/rollback` — 409 на текущую; для успешного отката сделать Draft-изменение (например, временный Tag), второй Deploy, откат к первой Revision (201, `cause=rollback`, `rolled_back_from=1`), затем вернуть Draft в исходное состояние, не ломая порядок `seq` и ожидания последующих запросов. Ассерты: статус, заголовок схемы, `application/problem+json` на ошибках, ключевые поля.
- Интеграционные тесты репозитория: нумерация и `cause`/`rolled_back_from`; сохранение состояний Tag и Device Tokens; сброс состояния исчезнувшего Tag и смена TagType; отказ при равном содержимом; отказ при Discard; 404 чужого/несуществующего номера; два параллельных отката/Deploy сериализуются.
- Тесты сервиса: публикация `revision_created` после коммита и только при успехе; отсутствие публикации при 409/404. REST-тесты: все три маршрута, 200 `[]`, 404, 400, 409; UI-тесты выше.
- Документация: ADR 0014 (уже записан), поправка к ADR 0013 (уже записана), `GLOSSARY.md` (Revision/Rollback — уже), `AGENTS.md` (маршруты, Revision list/detail/rollback, удалённая копия и `State`, Project-режим), `CHANGELOG.md`. Запись Rollback в Journal — задача 42, здесь только отметить.

## За рамками

- Journal (задача 42), History (45), Player/Operation (43), доступность UI отката на Runtime-only узле (43, 51), права доступа (49, 50), пагинация списка, рендер содержимого старой Revision, конвертеры `schema_version`.

## Comments

- 2026-10-04: бывший тикет 07 задачи 36. Решение Q30: Rollback на Runtime node создаёт новую Revision со старым содержимым.
- 2026-11-04 (grilling, Q1–Q12). Решения:
  - **Поверхность (Q1).** Runtime-нода + REST + apiclient + Bruno + тесты + UI в Project-режиме; Operation не трогаем (read-only, переделывается задачей 43).
  - **Причина Revision (Q2).** `cause` = `deploy` | `rollback`, у отката `rolled_back_from` — номер скопированной Revision. Имя `origin` не используется: оно уже занято Widget'ом (anchor) и термином глоссария про Tag/Widget.
  - **REST (Q3).** `GET …/revisions` (новыми первыми, без пагинации), `GET …/revisions/{n}` (полный документ), `POST …/revisions/{n}/rollback` без тела → 201 со сводкой. У проекта без Revision список — 200 `[]`.
  - **Откат в никуда (Q4).** Если содержимое выбранной Revision равно текущей — 409 `nothing_to_rollback`, новая Revision не создаётся (сравнивается документ, не номер).
  - **Discard (Q5).** Пока Project помечен на Discard, откат — 409 `project_discarding`, как Deploy: политика Discard читает текущую Revision.
  - **Состояния Tag (Q6).** Тот же дифф, что Deploy: совпавшие Tag'и сохраняют значение и `seq`, исчезнувшие теряют состояние, новые/со сменённым TagType — initial; Device Tokens не трогаются.
  - **Копия ProjectFile у Engineering (Q7).** Удаляется вместе с `State`/`DeployState`, `ProjectFileCopies` и `ErrRevisionCopyNotSaved`: после чужого Rollback она не может значить «текущая Revision», а её читателей нет. Повторный Deploy того же Draft — всегда 409.
  - **Версия документа (Q8).** Копируется как есть, `schema_version` не перештамповывается и не проверяется (MAJOR 0; при MAJOR 1 документы в БД мигрируются).
  - **Событие (Q9).** Тот же `revision_created`, без новых полей и типов.
  - **UI (Q10).** Кнопка `Revisions` в deploy-баре Project-режима, диалог со списком, просмотром выбранной Revision (полный документ → сводка) и подтверждением отката.
  - **Пустой список (Q11).** 200 `{revisions: []}`; `GET …/revision` по-прежнему 404 до первого Deploy.
  - **Живость проекта (Q12).** Новые маршруты (список, деталь, откат) требуют живого проекта: удалённый — 404; в откате это та же блокировка строки `public.projects`, что у Deploy. `GET …/revision` не меняется.
- 2026-11-04 (аудит DDD). Расхождений нет: Rollback — операция Runtime-ноды (`application/runtime`, REST без Engineering-импортов, `roles_test` зелёный), одна транзакция на Revision и её производные копии — та же документированная исключение-транзакция, что у Deploy (ADR 0013); агрегатов Draft не касается; термины глоссария уточнены; ограничение «UI отката только в Project-режиме» осознано (Runtime-only узел — REST, UI решит задача 43/51).
