# Назначение лейблов сохранённым состояниям и фильтр Commit'ов

Status: ready-for-agent
Priority: 1
Parent: 85_labels_instead_of_releases
Part: 3/8
Blocked by: 85_2_label_aggregate

Часть 3 из `85_labels_instead_of_releases.md`. Слой C+D. Продолжает 85_2:
лейблы уже можно создавать, переименовывать, перекрашивать и удалять.

## Зачем

Лейбл бесполезен, пока его нельзя навесить на сохранённое состояние. Здесь
появляются назначения (сущности агрегата Label), чипы в History и серверный
фильтр `?label=`, которым воспользуются окна Deploy (85_5) и Import (85_6).

## Домен

- `Label` (85_2) получает назначения как **сущности агрегата**, а не отдельный
  агрегат:
  - `Assign(commitID, scope, commitNumber, at) (Label, error)` — идемпотентно;
    событие `label_assigned`;
  - `Unassign(commitID) (Label, error)` — идемпотентно; событие
    `label_unassigned`;
  - `Assignments() []Assignment` — read-модель для UI;
  - `Delete()` снимает назначения в той же транзакции: каскад внутри агрегата,
    ADR 0008 соблюдён.
- `Assignment` — value/entity: `CommitID() id.ID[draft.Commit]`,
  `Scope() draft.Scope`, `CommitNumber() int`, `AssignedAt() time.Time`.
- Назначение **не** меняет Commit, Draft и Version; не пишет DraftChange и не
  попадает в undo/redo.
- События `label_assigned`/`label_unassigned` записывает Label. В
  `apiv0.Event` добавить `label` (UUID лейбла) и использовать существующий
  `commit`; `ID` — ID Project'а/Library, payload называет scope и номер
  Commit'а.
- **Маршрутизация событий — серверный поток.** Назначение Project'а не идёт
  только в поток Project'а: глобальное окно Labels и History в другом режиме
  должны видеть `assigned_count` и чипы. Bar с `?project=` получает и серверный
  поток, поэтому Project-режим ничего не теряет.
- Удаление Project/Library каскадно снимает назначения его Commit'ов через БД,
  без `label_unassigned`; клиенты перечитывают историю по
  `project_deleted`/`library_deleted`. Это исключение ADR 0008 в ADR 0025.

## Хранение

Миграция `engineering`:

```sql
CREATE TABLE engineering.commit_labels (
    label_id  UUID NOT NULL REFERENCES engineering.labels(id) ON DELETE CASCADE,
    commit_id UUID NOT NULL REFERENCES engineering.commits(id) ON DELETE CASCADE,
    assigned_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (label_id, commit_id)
);
CREATE INDEX idx_commit_labels_commit ON engineering.commit_labels (commit_id);
```

- PK делает назначение идемпотентным; индекс по `commit_id` — для чтения
  лейблов пачки сохранений.
- FK `ON DELETE CASCADE` снимает назначения при удалении лейбла (внутри
  агрегата) и при удалении Commit'а вместе с Project/Library (исключение).

## Application

- `LabelService` расширяется (`application/label_application_service.go`):
  - `Assign(ctx, scope draft.Scope, commitNumber int, labelID uuid.UUID)
    ([]appdto.Label, error)`;
  - `Unassign(ctx, scope, commitNumber, labelID) ([]appdto.Label, error)`;
  - `LabelsOf(ctx, scope) (map[int][]appdto.Label, error)` — для списка
    Commit'ов;
  - `List` считает `assigned_count` по `commit_labels`.
- Проверки: Project/Library жив; Commit существует в этой области; лейбл
  существует в Organization; номер Commit'а > 0; неверный UUID — 400.
  `Assign`/`Unassign` загружают Label и Commit, пишут назначение и outbox в
  одной транзакции; гонка удаления лейбла и назначения сериализуется на строке
  Label (`FOR UPDATE` или FK-ошибка → 404/409).
- `CommitService.List`/`Find` заполняют `appdto.Commit.Labels` одним запросом
  на пачку; `FindCommits` принимает необязательный `labelID` и фильтрует SQL
  `JOIN engineering.commit_labels`, не в памяти.
- `growctl`: `label assign <project|library> --commit N --label <name|id>
  [--library]`, `label unassign ...`; `commits` печатает имена/цвета лейблов.

## REST (`contract/api/v0`, MINOR, `make schemas`)

Под Project'ом и Library:

- `PUT …/projects/{id}/commits/{n}/labels/{label_id}` → 200
  `{labels: [LabelResponse]}`; 404 `commit_not_found`/`label_not_found`; 400.
- `DELETE …/projects/{id}/commits/{n}/labels/{label_id}` → 200
  `{labels: [...]}`; 404; 400. Повторный PUT/DELETE не 409.
- То же под `…/libraries/{id}/commits/{n}/labels/{label_id}`.
- `GET …/commits?label=<uuid>` (оба scope) — только сохранения с лейблом;
  без параметра — все; невалидный UUID — 400.
- `CommitResponse.Labels []LabelResponse` (`id`, `name`, `color`),
  `omitempty`; `Released` пока остаётся до 85_4/85_7.
- `apiclient`: `AssignLabel`, `UnassignLabel`, `Commits` с фильтром, лейблы в
  `CommitResponse`.
- `eventlog`: `label_assigned`/`label_unassigned`, `ReloadTargets`
  (Commits + Labels), категория History; события — серверные.

## UI (History)

- `ui/savedstates.Row` получает `Labels []apiv0.LabelResponse`; `Screen`
  получает `AllLabels []apiv0.LabelResponse` и callbacks `OnAssign(number,
  labelID)`, `OnUnassign(number, labelID)`, `OnManageLabels()`.
- В строке сохранения: цветные чипы (маленькие, `data-label-id`), рядом
  кнопка «+ label». Она раскрывает список всех лейблов с галочками у
  назначенных; клик ставит/снимает; пустой список — «No labels yet» и кнопка,
  открывающая окно лейблов.
- После назначения строка обновляется ответом; ошибка — toast; назначение не
  оптимистично.
- Оба режима (Library и Project) загружают `GET /labels` при открытии History
  и перечитывают историю по `label_assigned`, `label_unassigned`,
  `label_deleted`, `label_updated`.
- Лейблы не показываются в Draft/редакторе и не участвуют в Save.

## Тесты

- Домен: `Assign`/`Unassign` (идемпотентность), события, Scope
  Project/Library, чужой Commit, удаление лейбла с назначениями.
- Репозиторий: PK, CASCADE лейбла и Commit'а, чтение пачки, фильтр,
  `assigned_count`, гонка assign/delete.
- Application/REST: PUT/DELETE (200, 404, 400), `?label=`, лейблы в ответе,
  события в серверном потоке.
- UI (`uitest`): чипы и цвета, «+ label», назначение/снятие, пустой список,
  ошибка, событие другого клиента.
- growctl: assign/unassign, вывод `commits` с лейблами.
- Bruno: `put_commit_label`, `delete_commit_label`, `get_commits_by_label`,
  невалидный label (400), лейбл в `get_commit`; `requestBodies`; `seq`.
- `make build`, `make test`, `make bruno_test`.

## Документы

- `GLOSSARY.md`: Label assignment — сущность Label, не DraftChange, не Version,
  не часть ProjectFile.
- ADR 0025 фиксирует границу агрегата и каскад при удалении Project/Library;
  85_8 сверяет документы с кодом.

## Вне объёма

- Фильтры в окнах Deploy/Import (85_5/85_6).
- Назначение лейбла Draft'у или Deployment — только Commit.
- Массовое назначение и мультифильтр.

## Comments

- 2026-11-14: выделено из 85.
- 2026-11-14 (грилинг): назначения — сущности Label, не отдельный агрегат;
  удаление каскадное; события — в серверный поток; назначение в History.
