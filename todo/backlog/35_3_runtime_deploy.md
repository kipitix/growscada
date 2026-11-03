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
