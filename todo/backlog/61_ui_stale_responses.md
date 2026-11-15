# Устаревшие ответы в живой перезагрузке UI
Status: ready-for-agent
Priority: 1

Найдено в ревью задачи 29. Все ошибки старые: рефакторинг задачи 29 перенёс их без изменений. Общая причина — ответ, запрошенный до записи, применяется после неё. После 72_6/72_7 к этому добавляются гонки списка saved states, запрошенного до Save/Deploy, и чипов лейблов (85_3).

## Находки

- Список, запрошенный до создания, считает новый объект удалённым. Для Tag это закрыто (`tagLists`/`tagCreated`, `project/load.go`), для остальных нет:
  - `library/load.go` `setWidgetTypes`: только что созданный WidgetType пропадает из списка, выбор и LocalStorage сбрасываются (`clearSelection`).
  - `project/load.go` `setWidgets`: при быстром добавлении двух виджетов со второго снимается выбор (`pruneSelection`).
  - `project/load.go` `setScenes`: только что созданная Scene пропадает, выбор переходит на `scenes[0]`. `draftVersions.Advance` защищает от отката версии, но список всё равно строится из ответа, в котором нового ID нет.
- Library и Picker не защищены от списка, запрошенного до переименования: слияние откатывает имя в списке и в редакторе, следующий Apply молча отправляет старое имя. У Scene это делает `draftVersions.Advance`.
- Смена выбранной Scene (удалили выбранную, `selectScene`, сцен не осталось): `setScenes` очищает `p.widgets`, но `selectScene` до ответа оставляет старый список и рисует виджеты прошлой Scene.
- `project/scene_versions.go` `loadStaleScenes`: `staleScenes` очищается до GET. После сетевой ошибки повтора нет, следующая правка этой Scene получает ложный 409.
- `project/edit_fields.go` `widgetEditFieldsOf` округляет геометрию (`%.1f`, `%.3f`), а `saveWidgetGeometry` разбирает текст редактора. Сохранение из панели округляет и поля, которых пользователь не трогал (поворот 37.2846 → 37.3).
- `project/widget_ops.go` `finalizeAllDrags`: **закрыто** — отложенный `loadWidgets` ждёт отправленный PATCH (`waitMoves`), а не идёт с ним одновременно.

## Решения

- Общий механизм в `uiutil.Versions`: у каждой метки есть номер запроса, и `Versions` помнит, когда ID последний раз был в ответе или записан клиентом. Если это позже метки списка, отсутствие ID в списке не значит удаление.
- Списки Project и Library не заменяются ответом целиком: при ответе, который старше известного состояния, локальные копии остаются, а отсутствующие ID добавляются, если они известны после метки списка.
- Перетаскивание: сделано через `waitMoves`, менять не нужно.
- Смена выбранной Scene: `p.widgets` очищается сразу при выборе другой Scene, до ответа на холсте — «Loading widgets…».
- `loadStaleScenes`: при ошибке GET (кроме 404) ID возвращается в `staleScenes` и перечитывается при следующей загрузке; таймеров нет.
- Геометрия: поле, чей текст равен `widgetEditFieldsOf(текущий виджет)`, отправляется точным значением из виджета, иначе — разобранным текстом.
- Saved states: список, запрошенный до Save/Deploy, не должен откатывать только что созданную строку; чипы лейблов — тем же механизмом.

## План реализации

### Шаг 1. `uiutil.Versions` — метки запросов и «писано после»

`internal/server/interface/ui/uiutil/versions.go`, `versions_test.go`.

- `Mark` сейчас — это epoch, который растёт только на `Forget`/`ForgetID`; для порядка запросов он не годится. Ввести монотонный `seq`: `Mark()` увеличивает и возвращает номер; `Forget`/`ForgetID` тоже увеличивают его и записывают номер-барьер в `forgotAll`/`forgotID`; `Forgotten(m, id)` остаётся `m < барьер`.
- Добавить `seen map[uuid.UUID]Mark` — метка последнего ответа, который назвал ID (обновляется в `Advance`/`Observe`, даже если версия не новее: `seen[id] = max(seen[id], m)`).
- Добавить `Touch(id uuid.UUID)` — ставит `seen[id]` в `Mark()` на момент ответа записи/создания; для списка это «этот ID точно существует с этого момента».
- Добавить `KnownAfter(m Mark, id uuid.UUID) bool` — `seen[id] > m`; неизвестный ID — `false`.
- `Forget` очищает `seen`, `ForgetID` удаляет ID.
- Тесты: метки растут между запросами; ответ, отправленный до `Forget`, по-прежнему отбрасывается; `KnownAfter` истинно после `Touch`/`Observe` с более поздней меткой и ложно после более позднего списка, который ID не назвал; `ForgetID` снимает защиту.

### Шаг 2. Project — списки, выбор, создание

`internal/server/interface/ui/project/load.go`, `scene_ops.go`, `selection.go`, `tags_ops.go`, `widget_ops.go`.

- `setScenes`: `fresh` (через `Advance`) уже есть. При `!fresh` список не заменяет локальные Scene: для каждой Scene из ответа берётся локальная копия, если её версия не новее; Scene, которой нет в ответе, добавляется в конец, если `p.draftVersions.KnownAfter(mark, id)`; иначе считается удалённой. Выбор: выбранная Scene остаётся, если она в списке или `KnownAfter`; иначе — `scenes[0]`/`uuid.Nil`.
- `setWidgets`: та же логика для виджетов; `fresh` — по версии первого виджета, а для пустого списка — по версии выбранной Scene (или пропустить проверку, как сейчас). `pruneSelection` вызывать только для ID, которых нет ни в ответе, ни в `KnownAfter`.
- `createScene`, `createWidget`: после успешного ответа — `p.draftVersions.Touch(newID)` (и `Observe` версии, если она есть).
- `createTag`: `p.tagVersions.Touch(newID)`; `setTags` перестаёт использовать `tagLists`/`tagCreated` и сбрасывает выбор, только если Tag'а нет и он не `KnownAfter(mark)`. Поля `tagLists`/`tagCreated` удалить.
- `tagstate.Tags` не трогать: там свои `Mark`/`Merge` для `seq` состояния.

### Шаг 3. Library и Picker — имя и выбор

`internal/server/interface/ui/library/load.go`, `library/*_ops.go`, `internal/server/interface/ui/picker/picker.go`.

- `setWidgetTypes`: `fresh := len(items) > 0 && l.draftVersions.Advance(mark, l.libraryID, items[0].Version)`; при `fresh` — брать список, иначе сливать: локальные копии остаются, отсутствующие ID добавляются, если `l.draftVersions.KnownAfter(mark, id)`; выбор не сбрасывать для только что созданного WidgetType. `clearSelection` — только для действительно удалённого.
- Создание WidgetType: `createItem` в `library/widget_type_ops.go` — `l.draftVersions.Touch(newID)` после ответа.
- `Picker`: добавить `versions uiutil.Versions`; `load` берёт `mark` и для каждого `Item` вызывает `Advance(mark, item.ID, item.Version)`: ответ с более старой версией не заменяет локальное имя/описание. После успешного `commitEdit` обновить `p.items` и `selectedName` локально и вызвать `Touch(id)`; после создания — `Touch(newID)`. `reconcile` не снимает выбор, если `KnownAfter(mark, id)`.
- `lookup`/`loadOrder` не трогать: `orderSeq` уже отбрасывает старый ответ.

### Шаг 4. Смена Scene и `loadStaleScenes`

`internal/server/interface/ui/project/scene_ops.go`, `render_scene.go`, `scene_versions.go`.

- `selectScene`: очистить `p.widgets` и выбор до `loadWidgets`; в `canvasLayers`/`renderSceneCanvas` добавить слой «Loading widgets…», пока `p.widgetsReloader.Loading()` и `p.widgets == nil`; стиль — как у «Loading saved states…».
- `setScenes` при исчезновении выбранной Scene уже чистит `p.widgets` — сохранить.
- `loadStaleScenes`: в обработчике ошибки (кроме `apiclient.ErrNotFound`) вернуть `sceneID` в `staleScenes` и сделать `ctx.Update()`; повтор — при следующей загрузке, без таймеров.

### Шаг 5. Геометрия из панели

`internal/server/interface/ui/project/edit_fields.go`, `widget_ops.go`.

- В `saveWidgetGeometry` до разбора взять `fields := widgetEditFieldsOf(current)`; для каждого float-поля (`posX`, `posY`, `originX`, `originY`, `rotation`): если текст в `p.widgetEdit` равен `fields.<field>`, брать точное значение из `current`, иначе — `nf.float(...)`. `posZ`, `width`, `height` — целые, не округляются.
- Тест: виджет с поворотом 37.2846, пользователь меняет только X — после сохранения поворот 37.2846, а не 37.3.

## Порядок работ

Три коммита, каждый с зелёными `make build`/`make test`:

1. шаги 1–2 (`uiutil.Versions` + списки и создание в Project, включая Tag);
2. шаг 3 (Library и Picker);
3. шаги 4–5 (смена Scene, `loadStaleScenes`, геометрия).

## Тесты

- `uiutil/versions_test.go`: шаг 1.
- `ui/project`: отложенный список до создания Scene/Widget/Tag не убирает созданный объект и не снимает выбор; отложенный список до переименования не откатывает имя; `selectScene` очищает холст и показывает загрузку; `loadStaleScenes` повторяет после 500; сохранение геометрии не округляет нетронутые поля.
- `ui/library`: отложенный список до создания WidgetType сохраняет выбор; отложенный ответ после переименования не откатывает имя.
- `ui/picker`: stale list после rename и после create.
- Fakes из `uitest`: отложенные ответы (как в существующих тестах на события) и счётчики запросов.

## Приёмка

- Список, запрошенный до создания, не удаляет созданный объект и не снимает выбор; список до переименования не откатывает имя; смена Scene не рисует виджеты прошлой Scene; нетронутое поле геометрии сохраняется точным; `loadStaleScenes` повторяет после сетевой ошибки.
- `make build`, `make test`, `make bruno_test` проходят; поведение API не меняется.

## Comments

- 2026-11-15: сверено с кодом после цепочки 85. Список Release стал списком saved states; `finalizeAllDrags` уже закрыт (`waitMoves`), остальное воспроизводится как описано (`tagLists`/`tagCreated` живы, `setScenes` по-прежнему строит список из ответа, у Library нет `Advance`, `loadStaleScenes` чистит карту до GET, сохранение геометрии округляет). Зависимости 72_6/72_7 закрыты — `Blocked by` убран.
- 2026-11-15: задача подготовлена к разработке: расписаны шаги 1–5, файлы и тесты, `Priority: 1` (первая в P1). Отдельно отмечено, что `uiutil.Mark` придётся сделать монотонным: сейчас это epoch, растущий только на `Forget`, поэтому механизм «ответ позже метки» на нём не построить.
