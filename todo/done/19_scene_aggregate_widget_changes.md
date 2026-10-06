# Изменения Widget через агрегат Scene
Status: needs-triage

Появилось в архитектурном ревью 2026-10-05. По `CONTEXT.md` Scene — единственная граница консистентности для себя и своих Widget. На деле изменения Widget идут мимо агрегата: это отдельные методы репозитория, и правила Scene разнесены по трём модулям.

## Что не так сейчас

- В `scene.Scene` нет поведения для Widget. `SceneRepository` (`domain/scene/scene_repository.go`) — 12 методов, среди них `AddWidget`, `UpdateWidget`, `DeleteWidget`, `FindWidgetsByTypeID`.
- Правила Scene живут в трёх местах:
  - `SceneService` (`application/scene_application_service.go`) проверяет порты по WidgetType и сверяет ожидаемую Version;
  - `scene_repository_postgres.go` повышает Version (`bumpSceneVersion`) и переводит нарушение FK в `ErrWidgetTypeNotFound`;
  - сервис ещё раз оборачивает эту ошибку в invalid input.
- `DeleteWidget` повышает Version без CAS: `UPDATE scenes SET version = version + 1 WHERE id = $1`. Параллельная правка Scene между чтением и записью не обнаруживается.
- `FindByID` читает Scene и её Widget двумя запросами вне транзакции и может увидеть Scene в разные моменты времени.
- У `FindWidgetsBySceneID` и `FindWidgetByID` в репозитории нет вызывающих, кроме тестов.
- `Widget` лежит в `domain/widget` рядом с WidgetType, хотя это сущность агрегата Scene: `removeOrphanedPortBindings` в сервисе WidgetType собирает Widget через `widget.NewWidget(...)` в обход Scene.

Замечание: две сверки Version в `UpdateScene` (в сервисе и в SQL) — не дубль. Сервис ловит **конфликт правки** (ожидаемая Version клиента ≠ текущей), SQL-CAS — **гонку записи** (текущая Version сменилась между чтением и записью). Нужны обе; меняется только место первой.

## Решения

- **Scene неизменяемая.** Методы агрегата возвращают новую Scene, исходная не меняется: «до» — исходная, «после» — результат. Отдельного типа «изменение Scene» нет; форму DraftChange проектирует задача 38.
- **WidgetType передаётся аргументом.** Сервис загружает Scene (нет → 404), затем WidgetType (нет → 400 invalid input) и передаёт его в метод Scene. Агрегат проверяет, что `TypeID` Widget совпадает с WidgetType и PortBinding ссылаются только на его InputPort.
- **FK `fk_widgets_type_id` ловит гонку с удалением WidgetType.** Если WidgetType удалён между загрузкой и `Save`, репозиторий переводит нарушение FK в `ErrWidgetTypeNotFound`, а `widgetSaveError` в сервисе — в 400 `type_id`, как при отсутствии типа до загрузки.
- **Конфликт правки ловит агрегат.** Методы изменения принимают ожидаемую Version; несовпадение с текущей → `ErrSceneConflict`. Сервис своей сверки не делает, но вызывает `Scene.CheckVersion` до загрузки WidgetType, чтобы устаревшая запись получала 409 раньше 400 по `type_id` и не тратила запрос к `widget_types`. Порядок ошибок в `UpdateWidget`: Version (409) → Widget из URL (404) → WidgetType (400).
- **Гонку записи ловит `Save`**: CAS `WHERE version = <текущая>`, Version повышается один раз на сохранение.
- **Удаление Widget — без ожидаемой Version**, как удаление любого агрегата (`DELETE /tags/{id}`, `/widget-types/{id}`, `/scenes/{id}`). Контракт не меняется.
- **Widget и его value objects** (Position, Origin, Rotation, TransformMatrix, PortBinding, WidgetName…) переезжают в `domain/scene`. Пакет `domain/widget` (остаются WidgetType, InputPort, PortTypeHint, шаблон, скрипт) переименовывается в `domain/library` — под будущий агрегат Library (задача 33). Размер Widget — общий с WidgetType `library.Size`: одно правило (положительные ширина и высота, 100×100 по умолчанию) для размера по умолчанию и размера на Scene.

## Что сделать

- Методы агрегата Scene:
  - `AddWidget(expected, w, wt)`, `UpdateWidget(expected, w, wt)` — проверка TypeID и InputPort, конфликт правки, Widget с таким ID есть/нет → ошибка;
  - `RemoveWidget(widgetID)` — возвращает и удалённый Widget; нет такого Widget → `ErrWidgetNotFound`;
  - `CheckVersion(expected)` — конфликт правки до загрузки того, что нужно изменению;
  - изменение имени, размера и фона Scene с ожидаемой Version;
  - `ReconcileWith(wt)` — удаляет PortBinding на InputPort, которых больше нет у WidgetType; сообщает, изменилось ли что-то.
- `SceneRepository` сократить до: `NextID`, `NextWidgetID`, `FindByID`, `FindAll`, `FindByWidgetTypeID` (→ `[]Scene` целиком), `Save`, `DeleteByID`. Удалить `AddWidget`, `UpdateWidget`, `DeleteWidget`, `FindWidgetsBySceneID`, `FindWidgetByID`, `FindWidgetsByTypeID`, `WidgetInScene`, `bumpSceneVersion`.
- `Save(after)` в одной транзакции: CAS по строке `scenes` (0 строк → 404/409 через `classifyUpdateConflict`), чтение текущих Widget Scene, INSERT новых / UPDATE изменённых / DELETE исчезнувших. Возвращает Scene с Widget, перечитанными в той же транзакции: ответ API совпадает с последующим GET.
- Чтение (`FindByID`, `FindAll`, `FindByWidgetTypeID`) — в одной read-only транзакции `REPEATABLE READ`.
- `SceneService`: все изменения Widget и Scene — load → метод агрегата → `Save`. Удалить `validateWidgetType`; `widgetSaveError` оставить (см. «Решения»). `DELETE` Widget при гонке записи — 409.
- `removeOrphanedPortBindings` (`application/widget_type_application_service.go`): `FindByWidgetTypeID` → для каждой Scene `ReconcileWith(wt)` → `Save`, если изменилась. Согласование не опирается на то, что видел клиент, поэтому при `ErrSceneConflict` Scene перечитывается и согласуется заново (до 3 попыток), Scene, удалённая тем временем, пропускается. Атомарность и события — задача 21.
- Переезд Widget в `domain/scene` и переименование `domain/widget` → `domain/library`.
- Тесты:
  - правила Widget и конфликт правки тестируются на агрегате, без Postgres;
  - тест: метод агрегата не меняет исходную Scene;
  - репозиторий тестируется на сохранении и чтении Scene целиком (testcontainers), включая гонку записи;
  - гонки в сервисах: удаление WidgetType между загрузкой и `Save` → 400, параллельная правка Scene во время согласования → привязки очищены, правка сохранена, `DELETE` Widget при гонке → 409;
  - тесты удалённых методов репозитория удаляются, а не дублируются.

## Вне задачи

- Атомарное согласование Widget при изменении WidgetType и события затронутых Scene — задача 21.
- Единица работы и in-memory репозитории — задача 20.
- Проверка типа PortBinding по TagType — задача 25.
- Ожидаемая Version при удалении — если понадобится, единообразно для всех агрегатов отдельной задачей.

## Приёмка

- REST API и контракт `contract/api/v0` не меняются, Bruno-коллекция проходит как раньше.
- Изменение Widget с ожидаемой Version, отличной от текущей, возвращает 409 (как сейчас).
- Удаление Widget повышает Version Scene один раз через CAS и не затирает параллельную правку Scene: при гонке записи — 409.
- В `domain/widget` ничего не осталось; Widget — в `domain/scene`, WidgetType — в `domain/library`.
- `make build` и `make test` проходят.
