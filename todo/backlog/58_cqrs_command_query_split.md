# Разделение команд и запросов (CQRS, ступень 1)
Status: needs-triage
Priority: 3

Появилось в grilling задачи 21 (2026-10-11). Сейчас модель одна: запросы идут через те же агрегаты и репозитории, что и команды. Ступень 1 разводит модели записи и чтения в одной БД — синхронно, без проекций и без eventual consistency.

## Что не так сейчас

- Сервисы application (`TagService`, `SceneService`, `WidgetTypeService`, `DeviceService`, `EnvironmentService`) смешивают `Find…` и мутации в одном интерфейсе.
- Репозитории агрегатов обслуживают обе стороны. `SceneRepository.FindAll` поднимает полные Scene со всеми Widgets, хотя списку Scenes нужны только id, имя и Version. `FindWidgetsBySceneID` грузит Scene целиком ради её Widgets; `TagRepository.FindAll` и `FindAllInLiveProject` читают определения Tag'ов, чтобы отдать список.
- Чтение проходит через `Reconstitute` и value objects — инварианты записи оплачиваются на каждом GET.
- Форма ответа выводится из агрегата (`appdto`), а не из нужд экрана. Редактор Project собирает экран из `/libraries/{id}/widget-types`, `/projects/{id}/scenes`, `…/scenes/{id}/widgets` и `/projects/{id}/tags`; Library — из своего Draft'а и списка WidgetTypes.

## Что сделать

- Application делится на команды (нынешние мутации: загрузить → метод агрегата → `Save`/`Delete`) и запросы (`application/query`: интерфейсы под экраны, например `SceneQueries.List`, `SceneQueries.Widgets(sceneID)`).
- Реализация запросов — `infrastructure/postgres/readmodel`: прямой SQL в read-DTO, без агрегатов и `Reconstitute`. Один запрос может соединять несколько таблиц, если так нужно экрану.
- Репозитории агрегатов сужаются до потребностей команд: `NextID`, `FindByID`, `Save`, `Delete` и то, что нужно политикам (`FindByWidgetTypeID`). `FindAll` и поиск по шаблону уходят на сторону чтения. Исключение — то, что нужно командам для проверок (например, уникальность имени Tag), остаётся в репозитории.
- REST: GET-хендлеры — в запросы, мутирующие — в команды. Контракт `contract/api/v0` не меняется, SchemaVersion не растёт.
- Ответ мутаций (полный DTO) остаётся: строгий CQS не цель.
- Тесты запросов — на testcontainers, как у репозиториев.

## Ограничения

- Ступень 2 (отдельные read-таблицы, обновляемые проектором по событиям) — не здесь. Если понадобится, проектор строится на механизме `policy_jobs` (задача 21, ADR 0008): задание с ключом агрегата пересобирает строку проекции из текущего состояния (Event Notification, не event sourcing — ADR 0002).
- Уже отдельные модели чтения: Operation (`GET …/operation`, `OperationStateService`, ADR 0020) и History/Player (Journal + Checkpoint, ADR 0019/0023); Deployment/ProjectFile (модель чтения Runtime node); сохранённые состояния с лейблами и фильтром (`GET …/commits`, ADR 0021/0025) — но они читаются репозиторием Draft'а, а не `readmodel`. Эта задача касается модели Draft на Engineering node и экранов поверх текущих таблиц.

## Приёмка

- Ни один GET-хендлер не вызывает репозиторий агрегата.
- Поведение REST API не меняется: существующие тесты и Bruno-коллекции проходят без правок.
- `make build` и `make test` проходят; `CHANGELOG.md`.

## Comments

- 2026-10-11 (grilling задачи 21): делать, когда первому экрану станет неудобна форма агрегата — вероятнее всего, Operation/Player (задача 43). До тех пор выгода мала: объёмы CRUD-ные.
- 2026-11-15: сверено с кодом после цепочки 85. Operation получил собственную read-модель (`GET /projects/{id}/operation`) и больше не собирает сцену из четырёх запросов — прежний пример устарел. Оставшаяся боль — экраны Draft'а (Project и Library) и `FindAll` репозиториев, поэтому задача в силе, но мотивация теперь у них, а не у Operation.
