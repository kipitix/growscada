# Назначение лейблов сохранённым состояниям и фильтр Commit'ов

Status: ready-for-agent
Priority: 1
Blocked by: 85_2_label_aggregate

Часть 3 из `85_labels_instead_of_releases.md`. Слой C+D. Продолжает 85_2:
лейблы уже можно создавать, переименовывать и удалять.

## Зачем

Лейбл бесполезен, пока его нельзя навесить на сохранённое состояние. Здесь
появляется связь «лейбл — saved state» (many-to-many), чипы в History и
серверный фильтр `?label=`, которым воспользуются окна Deploy (85_5) и Import
(85_6).

## Домен

- В пакете `domain/label` — второй агрегат `Assignment` (связь, не часть
  Draft и не часть Commit):
  - `LabelID() id.ID[Label]`, `CommitID() id.ID[draft.Commit]`,
    `Scope() draft.Scope`, `CommitNumber() int`;
  - `Assign(label, commit, at)` и `Unassign(...)` — чистые конструкторы с
    событиями `label_assigned` / `label_unassigned`;
  - `Repository`:
    - `Assign(ctx, labelID, commitID) error` — идемпотентно; отсутствие лейбла
      или Commit'а — `label.ErrLabelNotFound` / `draft.ErrCommitNotFound`;
    - `Unassign(ctx, labelID, commitID) error` — идемпотентно;
    - `LabelsOfCommits(ctx, commitIDs []uuid.UUID) (map[uuid.UUID][]Label, error)`
      — один запрос на список History;
    - `FindCommitsByLabel(ctx, scope, labelID) ([]draft.Commit, error)` — для
      фильтра (или фильтр делает `draft.Repository.FindCommits` join'ом; выбрать
      одно и покрыть тестом).
- Назначение **не** меняет Commit, Draft, Version и ProjectFile; правка
  лейбла не пишет DraftChange и не попадает в undo/redo.
- Событие назначения называет Scope и номер Commit'а (как `commit_created`):
  Project'а — в поток Project'а, Library — в серверный поток. В `apiv0.Event`
  добавить `label` (UUID лейбла) и использовать существующий `commit`; `ID` —
  ID Project'а/Library. События лейблов (`label_created/renamed/deleted`) —
  в серверный поток.

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
  лейблов списка сохранённых состояний.
- Удаление лейбла (`ON DELETE CASCADE`) снимает назначения; `label_deleted`
  заставляет клиентов перечитать историю.

## Application

- `LabelAssignmentService` (`application/label_assignment_application_service.go`):
  - `Assign(ctx, scope draft.Scope, commitNumber int, labelID uuid.UUID)
    ([]appdto.Label, error)`;
  - `Unassign(ctx, scope, commitNumber, labelID) ([]appdto.Label, error)`;
  - `LabelsOf(ctx, scope) (map[int][]appdto.Label, error)` или загрузка лейблов
    в `CommitService.List`/`Find` (одно место, не дублировать).
- Проверки: Project/Library жив; Commit существует в этой области; лейбл
  существует в Organization; номер Commit'а > 0; неверный UUID — 400.
- В `CommitService` (или его read-модели) `appdto.Commit.Labels` заполняется
  для списка и одного сохранения; `LabelService.List` теперь считает
  `assigned_count` по `commit_labels` (или кэширует COUNT в том же запросе).
- `FindCommits`/`List` принимает необязательный `labelID`; фильтр — SQL
  `JOIN engineering.commit_labels` по `commit.id`, не фильтрация в памяти.

## REST (`contract/api/v0`, MINOR, `make schemas`)

Под Project'ом и Library:

- `PUT …/projects/{id}/commits/{n}/labels/{label_id}` → 200
  `{labels: [LabelResponse]}` (или 204; выбрать одно и зафиксировать в
  контракте); 404 `commit_not_found` / `label_not_found`; 400.
- `DELETE …/projects/{id}/commits/{n}/labels/{label_id}` → 200
  `{labels: [...]}`; 404; 400. Идемпотентность: повторный PUT/DELETE не 409.
- То же под `…/libraries/{id}/commits/{n}/labels/{label_id}`.
- `GET …/commits?label=<uuid>` (оба scope) — только сохранённые состояния с
  этим лейблом; без параметра — все; невалидный UUID — 400.
- `CommitResponse.Labels []LabelResponse` (`label` — id и name), `omitempty`;
  поле `Releases`/`Released` пока остаётся до 85_4/85_7.
- `apiclient`: `AssignLabel`, `UnassignLabel`, `Commits` с опцией фильтра,
  лейблы в `CommitResponse`.
- `eventlog`: типы `label_assigned`/`label_unassigned`, `ReloadTargets`
  (Commits + Labels), категория History.

## UI (History)

- `ui/savedstates.Row` получает `Labels []apiv0.LabelResponse`; `Screen`
  получает `AllLabels []apiv0.LabelResponse` и callbacks `OnAssign(number,
  labelID)`, `OnUnassign(number, labelID)`, `OnManageLabels()`.
- В строке сохранения: чипы лейблов (маленькие, с `data-label-id`), рядом
  кнопка «+ label». Она раскрывает список всех лейблов с галочками у
  назначенных; клик ставит/снимает; пустой список — «No labels yet» и кнопка,
  открывающая окно лейблов (85_2).
- После назначения строка обновляется ответом (или перечитыванием истории);
  ошибка — toast, назначение не оптимистично (лейбл — общие данные).
- Оба режима (Library и Project) загружают `GET /labels` при открытии History
  и перечитывают историю по `label_assigned`, `label_unassigned`,
  `label_deleted`, `label_renamed`.
- Лейблы не показываются в Draft/редакторе и не участвуют в Save.

## Тесты

- Домен: `Assign`/`Unassign`, события, Scope Project/Library, чужой Commit.
- Репозиторий: PK/идемпотентность, CASCADE лейбла и Commit'а, чтение лейблов
  пачки, фильтр по лейблу, `assigned_count`.
- Application/REST: PUT/DELETE (200, 404, 400), фильтр `?label=`, лейблы в
  ответе Commit, события в правильном потоке.
- UI (`uitest`): чипы, «+ label», назначение/снятие, пустой список, ошибка,
  событие другого клиента.
- Bruno: `put_commit_label`, `delete_commit_label`, `get_commits_by_label`,
  невалидный label (400), лейбл в `get_commit`; `requestBodies`; `seq` после
  лейблов (85_2) и Commit'ов.
- `make build`, `make test`, `make bruno_test`.

## Документы

- `GLOSSARY.md`: **Label assignment** — связь лейбла с сохранённым состоянием;
  не DraftChange, не Version, не часть ProjectFile.
- ADR 0025 — в 85_8.

## Вне объёма

- Фильтры в окнах Deploy/Import (85_5/85_6).
- Назначение лейбла работающей Deployment или Draft'у — нет, только Commit.
- Массовое назначение и цвета лейблов.

## Comments

- 2026-11-14: выделено из 85. Назначение — отдельный агрегат, чтобы не менять
  ни Commit, ни Draft и не заводить Version там, где её нет.
