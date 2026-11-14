# Лейбл: агрегат, CRUD API и отдельное окно

Status: ready-for-agent
Priority: 1

Часть 2 из `85_labels_instead_of_releases.md`. Слой C+D. Термин — Label в
`GLOSSARY.md` (запись добавляется здесь же; ADR 0025 пишется в 85_8).

## Зачем

Инженеру нужна именованная метка, которой он отметит сохранённые состояния
Library и Project, чтобы потом фильтровать их в окнах Import и Deploy. Лейбл
существует независимо от сохранённого состояния: его можно создать заранее,
переименовать и удалить.

## Домен (`domain/label`)

- `Label` — отдельный агрегат Engineering node:
  - `ID() id.ID[Label]`;
  - `OrganizationID() id.ID[organization.Organization]` — лейбл принадлежит
    Organization, а не Project или Library; на сервере сейчас одна Organization;
  - `Name() Name`, `CreatedAt() time.Time`;
  - `Rename(Name) (Label, error)` — новое имя, событие `label_renamed`;
  - `event.Recorder`.
- `Name` — value object: `strings.TrimSpace`, 1–100 рун, без переводов строк;
  пустое, слишком длинное или с переводом строки — `ErrInvalidLabelName`.
  Уникальность — в репозитории: имена сравниваются без учёта регистра в
  пределах Organization (`Prod` и `prod` — одно имя).
- `CreateLabel(organizationID, name, at)` — чистый конструктор, событие
  `label_created`; `ReconstituteLabel` — без события, для репозитория.
- `Delete()` возвращает копию с событием `label_deleted`; удаление каскадно
  снимает назначения (таблица `commit_labels`, 85_3) и не требует читать
  сохранённые состояния.
- Ошибки: `ErrLabelNotFound`, `ErrLabelNameTaken`, `ErrInvalidLabelName`.
- `Repository` (вне Draft, без Version):
  - `FindAll(ctx, organizationID) ([]Label, error)` — по имени без учёта
    регистра;
  - `FindByID(ctx, organizationID, id) (Label, error)`;
  - `Create`/`Rename`/`Delete` — с outbox-событиями, как у агрегатов
    Engineering node. Гонка двух одинаковых имён — unique-индекс →
    `ErrLabelNameTaken`.

## Хранение

Миграция `engineering`:

```sql
CREATE TABLE engineering.labels (
    id              UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL,
    CONSTRAINT ck_labels_name CHECK (name = btrim(name) AND length(name) BETWEEN 1 AND 100)
);
CREATE UNIQUE INDEX uq_labels_organization_name
    ON engineering.labels (organization_id, lower(name));
CREATE INDEX idx_labels_organization_name
    ON engineering.labels (organization_id, lower(name));
```

- Логически лейбл принадлежит Organization; физически таблица в `engineering`,
  потому что пишет и читает её только Engineering node, а назначения будут
  ссылаться на `engineering.commits`. В комментарии миграции и в ADR 0025
  зафиксировать: при появлении выбора Organization меняется только путь API и
  выборка по `organization_id`, таблицы и агрегат остаются.
- Отдельного поля цвета/иконки нет. Порядок — по имени.

## Application

- `LabelService` (`application/label_application_service.go`):
  - `List(ctx) ([]appdto.Label, error)`;
  - `Create(ctx, name) (appdto.Label, error)`;
  - `Rename(ctx, id, name) (appdto.Label, error)`;
  - `Delete(ctx, id) error`;
  - Organization берётся из `organization.FindSoleOrganization` (как в
    `main.go`); ошибка «нет ровно одной Organization» — отказ старта, не 404.
- `appdto.Label`: `ID`, `Name`, `CreatedAt`, `AssignedCount` (число
  сохранённых состояний с этим лейблом; в 85_2 всегда 0, в 85_3 — реальное).
- Сервис не читает Draft, не требует Version и не пишет DraftChange.

## REST (`contract/api/v0`, MINOR, `make schemas`)

Новый `contract/api/v0/label.go`:

- `GET /api/v0/labels` → 200 `{labels: [LabelResponse]}`, по имени;
- `POST /api/v0/labels` `{name}` → 201 `LabelResponse`; 400; 409
  `label_name_taken`;
- `PATCH /api/v0/labels/{id}` `{name}` → 200 `LabelResponse`; 400; 404
  `label_not_found`; 409 `label_name_taken`;
- `DELETE /api/v0/labels/{id}` → 204; 404.
- `LabelResponse{id, name, assigned_count, created_at}`.
- События `label_created`, `label_renamed`, `label_deleted` идут в серверный
  поток (`/api/v0/events`): лейблы не принадлежат Project'у. `eventlog`
  получает типы, категорию и `ReloadTargets.Labels`.
- `apiclient`: `Labels`, `CreateLabel`, `RenameLabel`, `DeleteLabel`.
- Ручки удалённого лейбла: назначения удаляются каскадом; UI History
  перечитывает сохранённые состояния по `label_deleted`.

## UI

- Новый пакет `ui/labels` — окно (dialog) со списком лейблов:
  - строка: имя (карандаш → поле + ✓/✕, Enter сохраняет, Esc отменяет,
    пустое имя не отправляется), `assigned_count` серым, кнопка удаления;
  - удаление — подтверждение с именем лейбла и предупреждением, что метка
    снимется с N сохранённых состояний; после удаления список перечитывается;
  - внизу — поле «New label» + Add; ошибка сервера показывается под полем, а
    набранный текст сохраняется (сервер — единственный валидатор);
  - `Ctrl/Cmd+Enter` в поле добавляет; `Esc` закрывает окно.
- Кнопка **Labels** (иконка тега) — в верхней панели `Root` рядом с
  переключателем темы, доступна во всех режимах; окно одно на всё приложение,
  открывается поверх текущего режима. Состояние (открыто/закрыто, черновики
  имён) — в `Root`, не в модах.
- На монтировании и по событиям `label_*` список перечитывается; `EventResync`
  тоже.
- Ошибки — `uiutil.Call`/`toast.FromError`; 409 имени — сообщение под полем.

## Тесты

- Домен: имя (trim, 1/100/101, перевод строки), `Rename`, события, удаление.
- Репозиторий (testcontainers): CRUD, регистронезависимая уникальность в
  Organization, две Organization не конфликтуют, CASCADE при удалении
  Organization, outbox-события.
- Application/REST: list/create/rename/delete, 400/404/409, `assigned_count`,
  события в серверном потоке.
- UI (`uitest`): открытие окна, добавление, переименование на месте, удаление
  с подтверждением, ошибка 409, событие другого клиента.
- Bruno: `get_labels`, `post_label`, `patch_label`, `delete_label`, повторное
  имя (409); `requestBodies`; `seq` — CRUD до использования в 85_3.
- `make build`, `make test`, `make bruno_test`, ручной проход в PWA.

## Документы

- `GLOSSARY.md`: **Label** — именованная метка Organization, назначаемая
  сохранённым состояниям; **Label assignment** появится в 85_3.
- Комментарий к таблице и в сервисе: «Organization-level; API is server-wide
  while one Organization exists».
- ADR 0025, AGENTS, CHANGELOG — в 85_8.

## Вне объёма

- Назначение лейблов сохранённым состояниям (85_3).
- Фильтры в Import/Deploy (85_5/85_6).
- Права и автор изменений (задачи 49/50); сейчас лейблы может менять любой
  клиент сервера.

## Comments

- 2026-11-14: выделено из 85. Организация уже есть как identity marker, поэтому
  `organization_id` пишется сразу; серверным остаётся только API.
