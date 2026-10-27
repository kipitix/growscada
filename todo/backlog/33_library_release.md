# Library и Library Release
Status: ready-for-agent
Blocked by: 32

Слой B. Термины — `CONTEXT.md` (Library, Library Release, Library Pin, Release, Draft, Discard, WidgetType); решения — ADR 0007, ADR 0012.

Вне задачи: EditLock (задача 37), DraftChange и undo/redo (задача 38), копирование в Revision/ProjectFile (задача 35), публичность и права доступа к Library (задача 50). До задачи 35 Draft проекта и Operation берут WidgetTypes из закреплённых Library Release.

## Модель

- **Library** — агрегат (имя, Organization, Version) в `engineering.*`; имя уникально среди живых Library своей Organization. Пока любой Project сервера может закрепить релиз любой Library. Удаление — пометка + политика очистки (как Project), только пока ни один живой Project её не закрепил (иначе 409); Library Pin держит строку живой Library `FOR SHARE`.
- **WidgetType** — отдельный агрегат с LibraryID (как Tag с ProjectID): репозитории открываются по Library (как `ProjectScopes`). Имя уникально в Library; (Library, имя) — естественный ключ. Удаление WidgetType из Draft Library не затрагивает ни одной Scene и больше не проверяет Widgets.
- **Library Release** — неизменяемый агрегат, никогда не удаляется: копии всех WidgetTypes Draft под теми же ID (объект-значение, отдельный доменный тип, например `ReleasedWidgetType`: без Version), номер 1, 2, 3… в Library (последний + 1, уникальность `(library_id, n)`, повтор при гонке — Library не меняется), время, необязательная заметка. Release отклоняется (409), если Draft не изменился с последнего релиза; сравнение Draft с последним релизом — чистая доменная функция (она же даёт признак «есть невыпущенные изменения»).
- **Library Pin** — агрегат на пару (Project, Library): номер Library Release + Version, содержимое Draft проекта (`ContentEvent`, через `ProjectScopes`). Добавить — создать (у Library должен быть хотя бы один релиз), сменить — с expected Version, убрать — удалить. Очистка Project удаляет и его Library Pins.
- **Widget** по-прежнему хранит только WidgetTypeID; его WidgetType — из того закреплённого релиза, где он есть.

## Смена Library Pin (ADR 0012)

- Одна доменная функция считает последствия: Widgets, оставшиеся без WidgetType, и PortBindings, которые станут невалидны. Её используют и предпросмотр (`impact`), и проверка.
- Смена/снятие отклоняются (409 со списком Widgets), если Widgets остались бы без WidgetType.
- Сломанные PortBindings удаляет политика согласования: по событию смены Library Pin — все Scenes проекта; по `widget_created`/`widget_updated` — Scene этого Widget, против закреплённых релизов. Джоб по `widget_type_updated` удаляется.
- Гонки: запись Scene, добавляющая Widget или меняющая его WidgetType, берёт строку Library Pin `FOR SHARE` и в той же транзакции проверяет WidgetType в закреплённом релизе; смена/снятие берёт её `FOR UPDATE` и проверяет Widgets проекта. Переходное решение до Draft-агрегата (задача 38).

## Discard Library (ADR 0012)

- Отклоняется (409), если релизов ещё нет.
- Ставит на Library пометку «сброс к релизу N»; политика приводит каждый WidgetType к последнему релизу в своей транзакции (вернуть удалённые с тем же ID, удалить новые, восстановить изменённые) и последней снимает пометку.
- Пока пометка стоит, правки WidgetTypes этой Library, Release и повторный Discard отклоняются (409): запись WidgetType берёт строку Library `FOR SHARE` и проверяет пометку.

## REST (`contract/api/v0`, MINOR; `make schemas`)

- Серверные: `/libraries` (CRUD, `?name=`), `/libraries/{id}/widget-types` (Draft, CRUD), `POST /libraries/{id}/releases` (Release: заметка + Version Library), `GET /libraries/{id}/releases[/{n}]`, `POST /libraries/{id}/discard`.
- Проектные: `GET …/library-pins`, `PUT …/library-pins/{library_id}` (номер релиза + Version), `DELETE …/library-pins/{library_id}`, `GET …/library-pins/{library_id}/impact?release=n`, `GET …/widget-types` (WidgetTypes всех закреплённых релизов — палитра Project и Operation).
- Старый `/widget-types` удаляется.
- События Library и Library Release — в серверный поток; события Library Pin — в поток проекта.

## growctl (`contract/manifest/v0`)

- Новые виды манифеста `Library` (имя) и `WidgetType` (Library по имени, имя, html/script, размер, InputPorts); apply декларативно правит Draft.
- Library Pins — поле манифеста `Project` (Library → номер релиза).
- Императивные команды `growctl release <library> [--note]` и `growctl discard <library>`.

## UI

- Library: список библиотек (создать/переименовать/удалить), Draft выбранной — текущий редактор WidgetTypes, признак «есть невыпущенные изменения», Release (с заметкой), Discard (с подтверждением; «идёт сброс», Draft только для чтения), история релизов с просмотром содержимого только для чтения.
- Project: панель Library Pins (добавить/сменить/убрать; перед сменой — предпросмотр: мешающие Widgets блокируют, теряемые PortBindings требуют подтверждения); палитра — WidgetTypes закреплённых релизов, сгруппированные по Library.

## Миграция и seed

- Миграция: Library «Main» в стартовой Organization со всеми существующими WidgetTypes (дубли имён — суффикс), Library Release 1, закреплён в каждом живом Project.
- Dev-seed: вторая Library с релизом — для проверки нескольких закреплений.

## Прочее

- Bruno (новые маршруты, `requestBodies`), тесты, `CHANGELOG.md`.

## Comments

- 2026-10-04: бывшая задача 18. Заменяет решение Q7 бывшей задачи 36 («Library — часть Draft проекта»): Library отдельна и общая по ссылке на релиз, а не копия в проекте.
- 2026-10-06 (DDD-ревью задачи 20): согласование Widgets с WidgetType (удаление привязок к исчезнувшим портам) переезжает сюда — в DraftChange смены закреплённого Library Release внутри Draft проекта. Политика по `widget_type_updated` из задачи 21 после этого удаляется: правка WidgetType в Draft Library не меняет ни одной Scene.
- 2026-10-27 (grilling): объём — без EditLock, DraftChange и undo (37, 38); границы агрегатов — как Project и Tag (Library, WidgetType, Library Release, Library Pin), укрупнение до Draft-агрегата — в 38; Widget ссылается только по WidgetTypeID; сироты отклоняются, привязки чинит политика; Discard — политикой с Draft только для чтения; публичности нет до задачи 50; снятие EditLock переименовано в Check in, чтобы «Release» осталось за Library. ADR 0012.
