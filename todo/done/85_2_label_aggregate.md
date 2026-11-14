# Лейбл: агрегат, CRUD API, стартовые лейблы и отдельное окно

Status: ready-for-agent
Priority: 1
Parent: 85_labels_instead_of_releases
Part: 2/8

Часть 2 из `85_labels_instead_of_releases.md`. Слой C+D. Термин — Label в
`GLOSSARY.md`; ADR 0025 и термины уже записаны, 85_8 их сверяет с кодом.

## Зачем

Инженеру нужна именованная метка с цветом, которой он отметит сохранённые
состояния Library и Project, чтобы потом фильтровать их в окнах Import и
Deploy. Лейбл существует независимо от сохранённого состояния: его можно
создать заранее, переименовать, перекрасить и удалить.

## Домен (`domain/label`)

- `Label` — **один** агрегат Engineering node. Назначения — его сущности
  (появятся в 85_3); отдельных агрегатов Assignment нет, чтобы удаление
  лейбла оставалось одной транзакцией (ADR 0008, ADR 0025).
  - `ID() id.ID[Label]`;
  - `OrganizationID() id.ID[organization.Organization]` — лейбл принадлежит
    Organization, а не Project или Library; на сервере сейчас одна Organization;
  - `Name() Name`, `Color() Color`, `CreatedAt() time.Time`;
  - `Rename(Name) (Label, error)`, `Recolor(Color) (Label, error)` — каждое
    событие `label_updated` (имя и/или цвет);
  - `Delete() Label` — событие `label_deleted`; назначения снимаются в той же
    транзакции (85_3);
  - `event.Recorder`.
- `Name` — value object: `strings.TrimSpace`, 1–100 рун, без переводов строк;
  пустое/длинное/с переводом строки — `ErrInvalidLabelName`. Уникальность —
  в репозитории без учёта регистра в пределах Organization (`Prod` и `prod` —
  одно имя); переименование, меняющее только регистр, не конфликтует с самим
  собой.
- `Color` — необязательный value object: пусто (нейтральный цвет темы) или
  `#RRGGBB` (6 hex-цифр, ведущий `#`). Неверный формат —
  `ErrInvalidLabelColor`. Цвет не является единственным различием чипа: имя
  всегда показано.
- `CreateLabel(organizationID, name, color, at)` — событие `label_created`;
  `ReconstituteLabel` — без события.
- Ошибки: `ErrLabelNotFound`, `ErrLabelNameTaken`, `ErrInvalidLabelName`,
  `ErrInvalidLabelColor`.
- `Repository` (вне Draft, без Version):
  - `FindAll(ctx, organizationID) ([]Label, error)` — по имени без учёта
    регистра;
  - `FindByID(ctx, organizationID, id) (Label, error)`;
  - `Create`/`Update`/`Delete` — с outbox-событиями. Гонка двух одинаковых
    имён — unique-индекс → `ErrLabelNameTaken`; удаление с назначениями
    каскадно.

## Хранение

Миграция `engineering`:

```sql
CREATE TABLE engineering.labels (
    id              UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    color           TEXT NULL,
    created_at      TIMESTAMPTZ NOT NULL,
    CONSTRAINT ck_labels_name CHECK (name = btrim(name) AND length(name) BETWEEN 1 AND 100),
    CONSTRAINT ck_labels_color CHECK (color IS NULL OR color ~ '^#[0-9A-Fa-f]{6}$')
);
CREATE UNIQUE INDEX uq_labels_organization_name
    ON engineering.labels (organization_id, lower(name));
CREATE INDEX idx_labels_organization_name
    ON engineering.labels (organization_id, lower(name));
```

- Стартовые лейблы в той же миграции:
  `INSERT ... ('ready_for_import', NULL), ('ready_for_deploy', NULL)`
  в единственную Organization, `ON CONFLICT DO NOTHING`. Они обычные:
  переименовываются, перекрашиваются и удаляются; миграция их не
  восстанавливает.
- Логически лейбл принадлежит Organization; физически таблица в `engineering`,
  потому что пишет и читает её только Engineering node, а назначения
  ссылаются на `engineering.commits`. В комментарии миграции и ADR 0025:
  при появлении выбора Organization меняется только путь API и выборка по
  `organization_id`.
- Порядок — по имени без учёта регистра.

## Application

- `LabelService` (`application/label_application_service.go`):
  - `List(ctx) ([]appdto.Label, error)`;
  - `Create(ctx, name, color) (appdto.Label, error)`;
  - `Update(ctx, id, name, color) (appdto.Label, error)` — отсутствующее поле
    сохраняет текущее; пустой цвет очищает;
  - `Delete(ctx, id) error`;
  - Organization берётся из `organization.FindSoleOrganization`.
- `appdto.Label`: `ID`, `Name`, `Color`, `CreatedAt`, `AssignedCount`
  (в 85_2 всегда 0, в 85_3 — реальное).
- Сервис не читает Draft, не требует Version и не пишет DraftChange.

## REST (`contract/api/v0`, MINOR, `make schemas`)

Новый `contract/api/v0/label.go`:

- `GET /api/v0/labels` → 200 `{labels: [LabelResponse]}`, по имени;
- `POST /api/v0/labels` `{name, color?}` → 201 `LabelResponse`; 400; 409
  `label_name_taken`;
- `PATCH /api/v0/labels/{id}` `{name?, color?}` → 200 `LabelResponse`; 400;
  404 `label_not_found`; 409 `label_name_taken`;
- `DELETE /api/v0/labels/{id}` → 204; 404.
- `LabelResponse{id, name, color, assigned_count, created_at}`.
- События `label_created`, `label_updated`, `label_deleted` — в серверный
  поток. `eventlog` получает типы, категорию и `ReloadTargets.Labels`.
- `apiclient`: `Labels`, `CreateLabel`, `UpdateLabel`, `DeleteLabel`.
- `growctl` (минимально): `labels` (list), `labels create <name> [--color
  #RRGGBB]`, `labels rename <old> <new>`, `labels color <name> <#RRGGBB|none>`,
  `labels delete <name>`; вывод — имя, цвет, assigned_count.

## UI

- Новый пакет `ui/labels` — глобальное окно (dialog):
  - строка: цветной маркер, имя (карандаш → поле + ✓/✕, Enter сохраняет,
    Esc отменяет), `assigned_count` серым, кнопка цвета (палитра пресетов +
    hex-поле + «No color»), кнопка удаления;
  - удаление — подтверждение с именем и предупреждением, что метка снимется
    с N сохранённых состояний (в 85_2 — 0); после удаления список
    перечитывается;
  - внизу — «New label» + необязательный цвет + Add; ошибка сервера под
    полем, набранный текст сохраняется;
  - `Ctrl/Cmd+Enter` добавляет; `Esc` закрывает.
- Кнопка **Labels** (иконка тега) — в верхней панели `Root` рядом с
  переключателем темы, доступна во всех режимах; окно одно, состояние — в
  `Root`.
- На монтировании, по `label_*` и `EventResync` список перечитывается.
- Ошибки — `uiutil.Call`/`toast.FromError`; 409 имени — сообщение под полем.
- Чипы цветов: фон/рамка из `color`, текст темы; пустой цвет — нейтральный
  `var(--border)`/`var(--text-muted)`.

## Тесты

- Домен: имя (trim, 1/100/101, перевод строки), цвет (пусто, `#a1B2c3`,
  неверный), `Rename`/`Recolor`/`Delete`, события.
- Репозиторий (testcontainers): CRUD, регистронезависимая уникальность,
  seed-лейблы на чистой БД, CASCADE Organization, outbox.
- Application/REST: list/create/update/delete, 400/404/409, `assigned_count`,
  события в серверном потоке, seed-лейблы видны на чистой БД.
- UI (`uitest`): окно, add/rename/recolor/delete, подтверждение, 409,
  событие другого клиента.
- growctl: `labels` CRUD, e2e на чистой БД.
- Bruno: `get_labels` (seed), `post_label`, `patch_label` (имя и цвет),
  `delete_label`, 409; `requestBodies`; `seq`.
- `make build`, `make test`, `make bruno_test`, ручной проход PWA.

## Документы

- `GLOSSARY.md`: Label — имя + необязательный цвет; принадлежит Organization,
  владеет назначениями; не content.
- Комментарий к таблице и сервису: «Organization-level; API server-wide while
  one Organization exists».
- ADR 0025 и GLOSSARY уже обновлены; AGENTS, CHANGELOG и сверка — в 85_8.

## Вне объёма

- Назначение лейблов сохранённым состояниям (85_3).
- Фильтры в Import/Deploy (85_5/85_6).
- Права и автор изменений (задачи 49/50).

## Comments

- 2026-11-14: выделено из 85; `organization_id` сразу, API серверный.
- 2026-11-14 (грилинг): имя + необязательный hex-цвет; уникальность без учёта
  регистра; два стартовых лейбла; окно глобальное; назначения — сущности
  Label (85_3).
