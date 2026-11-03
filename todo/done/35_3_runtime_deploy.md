# Runtime-нода: Deploy, текущая Revision, состояния Tag, конфиг Gateway
Status: ready-for-agent
Blocked by: 35_2

Часть 3 задачи 35. Runtime-нода принимает ProjectFile, создаёт Revision и переключает на неё собственные чтения; Engineering-нода пока не умеет деплоить, мост Draft→Runtime ещё жив.

Источник истины — `todo/backlog/35_projectfile_deploy_revision.md` (гриллинг 2026-11-03, Q4, Q5, Q8, Q11): это **спецификация**, а не отдельная работа.

## Что сделать

### Порт `application.RuntimeNode`

- `Deploy(ctx, projectID uuid.UUID, file projectv0.ProjectFile) (appdto.Revision, error)` — отдать Runtime-ноде конфигурацию; номер и время назначает она. `Header.ProjectID` документа обязан совпадать с `projectID`, иначе ошибка (чужой документ).
- `FindCurrentRevision(ctx, projectID uuid.UUID) (appdto.Revision, bool, error)` — нужен Engineering-ноде для «нечего деплоить», preview и Discard (части 4–5).
- Существующие методы остаются; комментарий порта больше не говорит «until Deploy exists» — теперь копии приходят из Revision.

### `application/runtime.RevisionService` и `inprocess`

- `Deploy`: `RevisionRepository.CreateRevision` (вся транзакция — в репозитории, часть 2), затем **после коммита** опубликовать событие `revision_created` в EventBus через `eventbus.AsyncPublisher` — так же, как `tag_updated`, без outbox (ADR 0011, Q11).
- `FindCurrent`. Сервис не читает `engineering.*` и не импортирует Engineering-пакеты (`roles_test`).
- `inprocess.NewRuntimeNode` получает методы; `main.go` прокидывает сервис (пока без REST).
- Правила состояний Tag (Q4) реализуются репозиторием из части 2 и проверяются тестами сервиса: новый Tag — initial, оставшийся — значение сохраняется, исчезнувший — удаляется, смена TagType — initial.

### Мост Draft→Runtime и Deploy пишут одни и те же копии

- Advisory-lock берёт только `CreateRevision` (часть 2), а мост (`UpsertDefinitions` в `CreateTag`, `Delete` у `DeleteTagByID`, policy `sync_runtime_tag_definitions`) и `SetTagValue` его не берут. Лок и не закрывает гонку: Tag, созданный мостом сразу после коммита Deploy, остаётся копией вне работающей Revision, а его значения Operation не увидит.
- Решить здесь, а не оставлять на часть 4/5: либо `Deploy` сверяет документ с Draft через порт после коммита (Tag, созданный во время Deploy, попадает в следующую ревизию, а запись значения до неё — 404), либо мост перестаёт писать копии в этой части. Решение записать в ADR 0013 (часть 5) и в этот пункт, с тестом на тег, созданный во время Deploy.

### Событие `revision_created`

- Новый `event.EventType` (`revision_created`), ContentEvent проекта: несёт `project_id`, номер ревизии и `deployed_at`.
- `contract/api/v0`: новый тип события и его разбор в `event.go`; `SchemaVersion` api/v0 — MINOR, `make schemas`.
- Трансляция в `restapi.newAPIEvent` (поля события — по месту использования в части 5).
- Тест на форму события и на то, что его получает только поток своего проекта.

### Конфиг Device Gateway — из Revision

- `DeviceConfigService` переезжает из `application` в `application/runtime`: конфигурация работающей Revision — данные Runtime-ноды (Q5, аудит DDD). Читает `runtime.devices`, `runtime.environments`, пару `runtime.environment_devices` и `runtime.tags` (поставщик = Device), Draft не участвует.
- Нужен репозиторий чтения копий (`postgres/runtime`): Device по ID, Environment по ID, пара, Tag'и по поставщику — в одной транзакции на запрос (или одним запросом с JOIN, как удобнее).
- Семантика ошибок сохраняется: нет Device в Revision (спящий токен) → 403 в REST, нет Environment или пары → 404; токен не удаляется и не меняется — он живёт вне Revision (ADR 0004).
- Обновить вызовы в `main.go` и тесты; комментарий «Until Deploy exists» уходит.

## Проверка

- `make build`, `make test`.
- Тесты: Deploy двух ревизий подряд (номера 1 и 2, `deployed_at` растёт, `UNIQUE` держится при параллельном вызове); событие публикуется после коммита и не публикуется при ошибке; конфиг Gateway отвечает по копиям и не видит Device, которого нет в Revision, но чей токен жив.
- `go test ./internal/server/roles_test.go` — импорты ролей не нарушены.

## Вне объёма

- Engineering Deploy/preview/Discard, снятие моста, обновление тестов, которые пишут значения Tag — часть 4.
- REST-маршруты, apiclient, UI, Bruno, документация — часть 5.

## Comments

- 2026-11-04 (реализация). **Гонка моста и Deploy** решена инвариантом «копии Runtime-схемы принадлежат работающей Revision»: `UpsertDefinitions` и `Delete` (`postgres/runtime`) берут тот же advisory-lock, что `CreateRevision`, и в той же транзакции проверяют, есть ли у проекта Revision; если есть — мост молчит. Поэтому Tag, созданный во время Deploy, попадает в **следующую** Revision, а запись его значения до неё — 404: копии вне Revision не появляется ни в одном порядке (мост до Deploy — дифф `CreateRevision` удаляет копию, которой нет в документе; мост после коммита — видит Revision и не пишет). Сверка документа с Draft после коммита остаётся за Engineering-нодой (индикатор неразвёрнутых изменений и следующий Deploy, часть 4); в части 3 для неё готовы `FindCurrentRevision` и сам Deploy. Тесты: `TestUpsertDefinitions_AfterARevision_WritesNoCopyOutsideIt`, `TestDelete_AfterARevision_KeepsTheRunningCopy`, `TestRevisionService_Deploy_KeepsTheTagStatesOfTheRunningRevision` (поздний Tag, запись — `tag.ErrTagNotFound`).
- 2026-11-04 (реализация). Порт `RuntimeNode` получил `Deploy` и `FindCurrentRevision`; `application/runtime.RevisionService` публикует `revision_created` после коммита через тот же `eventbus.AsyncPublisher`, что и `tag_updated` (без outbox). `DeviceConfigService` переехал в `application/runtime` и читает `runtime.devices`/`environments`/`environment_devices`/`tags` одним READ ONLY-транзакционным запросом; `appdto.DeviceConfig` удалён. `revision_created` — ContentEvent проекта, в API — `{id: project_id, revision, deployed_at}`, api/v0 0.10. Симулятор и его e2e-тесты деплоят проект через новый `servertest.DeployProject` (конфигурация Gateway теперь только из Revision).
- 2026-11-04 (ревью). Остаточное окно оговорено: до первой Revision мост по-прежнему пишет копии, поэтому Q4 «запись значения вне работающей Revision — 404» действует только после первого Deploy; это записано в ADR 0011 (поправка 2026-11-04), ссылку на ADR 0013 даст часть 5. Гонка моста и Deploy теперь покрыта конкурентным `TestUpsertDefinitions_ConcurrentWithADeploy_WritesNoCopyOutsideIt`, а не только последовательными тестами. Сид дополнен копиями Gateway-фикстур (`20990101000003_runtime_device_gateway_copies.sql`), иначе `GET /device/config` на чистой БД отвечал 403.
